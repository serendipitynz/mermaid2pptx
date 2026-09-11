package convert

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// xnode is a generic XML tree node keeping mixed content in order.
// tag == "" means a text node (text holds the character data).
type xnode struct {
	tag  string
	text string
	attr map[string]string
	kids []*xnode
}

func (n *xnode) get(name string) string { return n.attr[name] }

func (n *xnode) hasClass(c string) bool {
	for _, f := range strings.Fields(n.get("class")) {
		if f == c {
			return true
		}
	}
	return false
}

// walk visits every element node in depth-first order.
func (n *xnode) walk(fn func(*xnode)) {
	if n.tag != "" {
		fn(n)
	}
	for _, k := range n.kids {
		k.walk(fn)
	}
}

func parseXMLTree(r io.Reader) (*xnode, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	root := &xnode{tag: "#root", attr: map[string]string{}}
	stack := []*xnode{root}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el := &xnode{tag: t.Name.Local, attr: map[string]string{}}
			for _, a := range t.Attr {
				el.attr[a.Name.Local] = a.Value
			}
			top := stack[len(stack)-1]
			top.kids = append(top.kids, el)
			stack = append(stack, el)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			s := string(t)
			if s != "" {
				top := stack[len(stack)-1]
				top.kids = append(top.kids, &xnode{text: s})
			}
		}
	}
	if len(root.kids) == 0 {
		return nil, fmt.Errorf("empty XML document")
	}
	for _, k := range root.kids {
		if k.tag != "" {
			return k, nil
		}
	}
	return nil, fmt.Errorf("no root element")
}

var (
	reTranslate   = regexp.MustCompile(`translate\(\s*(-?[\d.eE+]+)[,\s]+(-?[\d.eE+]+)\s*\)`)
	reHexColor    = regexp.MustCompile(`#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})`)
	reRGBColor    = regexp.MustCompile(`rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)`)
	reNumber      = regexp.MustCompile(`-?[\d.]+(?:[eE][-+]?\d+)?`)
	reCylinderArc = regexp.MustCompile(`^M\s*0[,\s][\d.\s]+a\s*(-?[\d.]+)[,\s]+(-?[\d.]+)`)
)

// findShapeEl locates the geometry element of a node, descending into
// wrapper groups but skipping the label subtree (which holds an empty
// placeholder <rect/>).
func findShapeEl(g *xnode) *xnode {
	var shape *xnode
	var visit func(*xnode)
	visit = func(k *xnode) {
		if shape != nil {
			return
		}
		switch k.tag {
		case "rect", "polygon", "path", "circle", "ellipse":
			if k.tag == "rect" && k.get("width") == "" {
				return
			}
			shape = k
		case "g":
			if k.hasClass("label") {
				return
			}
			for _, c := range k.kids {
				visit(c)
			}
		}
	}
	for _, k := range g.kids {
		visit(k)
	}
	return shape
}

func parseTranslate(s string) (float64, float64, bool) {
	m := reTranslate.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	x, _ := strconv.ParseFloat(m[1], 64)
	y, _ := strconv.ParseFloat(m[2], 64)
	return x, y, true
}

// parseStyleDecls parses "fill:#fff !important; stroke: red" into a map.
func parseStyleDecls(s string) map[string]string {
	out := map[string]string{}
	for _, d := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(d, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(strings.ReplaceAll(v, "!important", ""))
		out[strings.TrimSpace(k)] = v
	}
	return out
}

// cssColorToHex converts #rgb/#rrggbb/rgb(...) to "RRGGBB". Empty when unknown.
func cssColorToHex(v string) string {
	v = strings.TrimSpace(v)
	if m := reHexColor.FindStringSubmatch(v); m != nil {
		h := m[1]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		return strings.ToUpper(h)
	}
	if m := reRGBColor.FindStringSubmatch(v); m != nil {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		return strings.ToUpper(fmt.Sprintf("%02x%02x%02x", r, g, b))
	}
	return ""
}

func styleColor(styles map[string]string, key string) string {
	if v, ok := styles[key]; ok {
		return cssColorToHex(v)
	}
	return ""
}

// extractLabel converts a foreignObject subtree (XHTML) into paragraphs and
// picks the innermost explicit text color if any.
func extractLabel(fo *xnode) ([]Para, string) {
	var paras []Para
	var cur Para
	color := ""
	flush := func() {
		if !cur.empty() {
			// trim outer whitespace of the paragraph
			cur.Runs[0].Text = strings.TrimLeft(cur.Runs[0].Text, " \n\t")
			last := len(cur.Runs) - 1
			cur.Runs[last].Text = strings.TrimRight(cur.Runs[last].Text, " \n\t")
			if !cur.empty() {
				paras = append(paras, cur)
			}
		}
		cur = Para{}
	}
	var visit func(n *xnode, bold, italic bool)
	visit = func(n *xnode, bold, italic bool) {
		if n.tag == "" {
			t := strings.ReplaceAll(n.text, "\n", " ")
			if t != "" {
				cur.Runs = append(cur.Runs, Run{Text: t, Bold: bold, Italic: italic})
			}
			return
		}
		if c := styleColor(parseStyleDecls(n.get("style")), "color"); c != "" {
			color = c
		}
		switch n.tag {
		case "br":
			flush()
			return
		case "p":
			flush()
		case "b", "strong":
			bold = true
		case "i", "em":
			italic = true
		}
		for _, k := range n.kids {
			visit(k, bold, italic)
		}
		if n.tag == "p" {
			flush()
		}
	}
	visit(fo, false, false)
	flush()
	return paras, color
}

// mermaid's own defaults, used when the SVG does not say otherwise.
const (
	defFontSizePx = 16.0
	defLineHeight = 1.5
)

// labelLayout is what mermaid's foreignObject records about how the label was
// laid out: the width the browser wrapped it at (0 when mermaid laid it out
// unwrapped) and the number of lines it ended up on.
type labelLayout struct {
	wrapW float64
	lines int
}

// findLabel finds the label group / foreignObject under an element and
// returns paragraphs, explicit text color, and that layout. fontPx is the
// diagram's font size, which the line count is read against.
func findLabel(el *xnode, fontPx float64) ([]Para, string, labelLayout) {
	var fo *xnode
	el.walk(func(n *xnode) {
		if fo == nil && n.tag == "foreignObject" {
			fo = n
		}
	})
	if fo == nil {
		return nil, "", labelLayout{}
	}
	paras, color := extractLabel(fo)
	if color == "" {
		// color may be on the surrounding g.label element
		el.walk(func(n *xnode) {
			if color == "" && n.hasClass("label") {
				color = styleColor(parseStyleDecls(n.get("style")), "color")
			}
		})
	}
	return paras, color, labelLayout{wrapW: labelWrapWidth(fo), lines: labelLineCount(fo, fontPx)}
}

// labelLineCount is how many lines mermaid rendered the label on: its
// foreignObject is one line high per line of text. The line is the diagram's
// font size times the label's own line-height, not a fixed 24px — mermaid
// takes a configured fontSize, and at 20px a two-line label is 60px high,
// which read as 24px lines would come out as three.
func labelLineCount(fo *xnode, fontPx float64) int {
	h, _ := strconv.ParseFloat(fo.get("height"), 64)
	if h <= 0 {
		return 0
	}
	lineH := labelFontSize(fo, fontPx) * labelLineHeight(fo)
	if lineH <= 0 {
		return 0
	}
	return int(math.Round(h / lineH))
}

// labelFontSize is the size the label itself declares, falling back to the
// diagram's. mermaid's classDef can set a font-size per node, which makes that
// label's lines taller than the rest of the diagram's; where it reaches the
// label as an inline style, this picks it up. The innermost declaration wins,
// as it does for the label's color.
//
// A classDef mermaid emits as a stylesheet rule instead of an inline style is
// not resolved: that needs the rule matched against the node's classes, and
// there is no fixture rendered from such a diagram to check the shape against.
// mermaid's DOM differs between versions, so a guess at it is what breaks
// silently on the next upgrade.
func labelFontSize(fo *xnode, docPx float64) float64 {
	px := 0.0
	fo.walk(func(n *xnode) {
		v, ok := parseStyleDecls(n.get("style"))["font-size"]
		if !ok {
			return
		}
		if f := cssLengthPx(v); f > 0 {
			px = f
		}
	})
	if px <= 0 {
		return docPx
	}
	return px
}

// labelLineHeight reads the unitless line-height mermaid sets on the label's
// div, defaulting to the 1.5 it has always emitted.
func labelLineHeight(fo *xnode) float64 {
	ratio := 0.0
	fo.walk(func(n *xnode) {
		if ratio > 0 {
			return
		}
		if v, ok := parseStyleDecls(n.get("style"))["line-height"]; ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 {
				ratio = f
			}
		}
	})
	if ratio <= 0 {
		return defLineHeight
	}
	return ratio
}

// docFontSize is the font size mermaid's own stylesheet sets on the diagram
// root, which every label is laid out against. Diagrams rendered with a
// configured fontSize carry it here rather than in any label's own style.
func docFontSize(root *xnode) float64 {
	id := root.get("id")
	if id == "" {
		return defFontSizePx
	}
	var css strings.Builder
	root.walk(func(n *xnode) {
		if n.tag == "style" {
			for _, k := range n.kids {
				css.WriteString(k.text)
			}
		}
	})
	re := regexp.MustCompile(`#` + regexp.QuoteMeta(id) + `\s*\{[^}]*?font-size:\s*([0-9.]+(?:px|pt))`)
	if m := re.FindStringSubmatch(css.String()); m != nil {
		if f := cssLengthPx(m[1]); f > 0 {
			return f
		}
	}
	return defFontSizePx
}

// cssLengthPx converts a CSS font-size to px. Only the absolute units are
// converted: a relative one (em, rem, %) resolves against the cascade this
// parser does not build, and a number read out of one as if it were px would
// be worse than falling back to the diagram's own size.
func cssLengthPx(v string) float64 {
	v = strings.TrimSpace(v)
	for _, u := range []struct {
		suffix string
		px     float64
	}{
		{"px", 1},
		{"pt", 96.0 / 72.0},
	} {
		rest, ok := strings.CutSuffix(v, u.suffix)
		if !ok {
			continue
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(rest), 64); err == nil && f > 0 {
			return f * u.px
		}
		return 0
	}
	return 0
}

// labelWrapWidth reports the pixel width the browser wrapped a label at, or 0
// when mermaid declared it unwrappable (white-space: nowrap). The distinction
// decides who owns the line breaks: a nowrap label carries every break in the
// DOM as a <br>, so its paragraphs already are the rendered lines, while a
// wrapped one records only its width and height and the break positions are
// gone.
func labelWrapWidth(fo *xnode) float64 {
	nowrap := false
	fo.walk(func(n *xnode) {
		if parseStyleDecls(n.get("style"))["white-space"] == "nowrap" {
			nowrap = true
		}
	})
	if nowrap {
		return 0
	}
	w, _ := strconv.ParseFloat(fo.get("width"), 64)
	return w
}

// applyLabelWrap carries mermaid's line breaking across, so that the emitted
// paragraphs are the lines mermaid rendered. A label mermaid did not wrap
// already has every break in the DOM as a <br>; one the browser wrapped keeps
// only its width and line count, so it is re-broken here to that same count.
//
// The line breaks are then the generator's to own: every label is emitted with
// PowerPoint's wrapping switched off. Leaving it on re-breaks these lines
// against a text area much narrower than the node box, which is the whole
// defect for a one-line label and re-breaks a wrapped one just as badly
// (a 2-line diamond label came back out of PowerPoint as 4 lines).
func applyLabelWrap(paras []Para, lay labelLayout) []Para {
	if lay.wrapW <= 0 {
		return paras
	}
	return fitLineCount(paras, lay.wrapW, lay.lines)
}

// diagramType maps the SVG root's aria-roledescription to a Diagram.Type.
func diagramType(root *xnode) string {
	switch role := root.get("aria-roledescription"); {
	case strings.HasPrefix(role, "flowchart"):
		return "flowchart"
	case strings.HasPrefix(role, "stateDiagram"):
		return "state"
	case role == "class" || strings.HasPrefix(role, "classDiagram"):
		return "class"
	case role == "er" || strings.HasPrefix(role, "erDiagram"):
		return "er"
	case role == "sequence":
		return "sequence"
	default:
		return role
	}
}

// ParseMermaidSVG parses a mermaid SVG (flowchart, stateDiagram, class, er,
// sequence) into a Diagram.
func ParseMermaidSVG(r io.Reader) (*Diagram, error) {
	root, err := parseXMLTree(r)
	if err != nil {
		return nil, err
	}
	if root.tag != "svg" {
		return nil, fmt.Errorf("not an SVG document (root <%s>)", root.tag)
	}
	d := &Diagram{Type: diagramType(root)}
	vb := reNumber.FindAllString(root.get("viewBox"), -1)
	if len(vb) == 4 {
		d.MinX, _ = strconv.ParseFloat(vb[0], 64)
		d.MinY, _ = strconv.ParseFloat(vb[1], 64)
		d.W, _ = strconv.ParseFloat(vb[2], 64)
		d.H, _ = strconv.ParseFloat(vb[3], 64)
	}
	if d.W == 0 || d.H == 0 {
		return nil, fmt.Errorf("missing or invalid viewBox")
	}

	switch d.Type {
	case "flowchart", "state", "class", "er":
		parseGraphDiagram(root, d)
	case "sequence":
		parseSequence(root, d)
	default:
		return nil, fmt.Errorf("unsupported diagram type %q (supported: flowchart, stateDiagram, class, er, sequence)", d.Type)
	}
	if len(d.Nodes) == 0 {
		return nil, fmt.Errorf("no nodes found in %s diagram", d.Type)
	}
	return d, nil
}

// parseGraphDiagram handles the dagre-based diagram types that share the
// nodes / edgePaths / edgeLabels / clusters DOM structure. Composite states
// nest their content in a translated <g class="root">, so ancestor
// translates are accumulated while walking.
func parseGraphDiagram(root *xnode, d *Diagram) {
	fontPx := docFontSize(root)
	var visit func(n *xnode, dx, dy float64)
	visit = func(n *xnode, dx, dy float64) {
		if n.tag == "" || n.tag == "defs" {
			return
		}
		switch {
		case n.tag == "g" && (n.hasClass("cluster") || n.hasClass("statediagram-cluster")):
			if c, ok := parseCluster(n, fontPx, dx, dy); ok {
				d.Clusters = append(d.Clusters, c)
			}
			return
		case n.tag == "g" && n.hasClass("node"):
			if d.Type == "class" || d.Type == "er" {
				parseCompartmentNode(n, d, dx, dy)
			} else if nd, ok := parseNode(n, d.Type, fontPx, dx, dy); ok {
				d.Nodes = append(d.Nodes, nd)
			}
			return
		case n.tag == "path" && n.get("data-edge") == "true":
			if e, ok := parseEdge(n, dx, dy); ok {
				d.Edges = append(d.Edges, e)
			}
			return
		case n.tag == "g" && n.hasClass("edgeLabel"):
			if l, ok := parseEdgeLabel(n, fontPx, dx, dy); ok {
				d.Labels = append(d.Labels, l)
			}
			return
		}
		if tx, ty, ok := parseTranslate(n.get("transform")); ok {
			dx, dy = dx+tx, dy+ty
		}
		for _, k := range n.kids {
			visit(k, dx, dy)
		}
	}
	visit(root, 0, 0)
	resolveEdgeEndpoints(d)
}

func parseCluster(g *xnode, fontPx, dx, dy float64) (Cluster, bool) {
	c := Cluster{ID: orDefault(g.get("data-id"), clusterID(g.get("id")))}
	if tx, ty, ok := parseTranslate(g.get("transform")); ok {
		dx, dy = dx+tx, dy+ty
	}
	var rect *xnode
	g.walk(func(n *xnode) {
		if rect == nil && n.tag == "rect" && n.get("width") != "" {
			rect = n
		}
	})
	if rect == nil {
		return c, false
	}
	c.R.X, _ = strconv.ParseFloat(rect.get("x"), 64)
	c.R.Y, _ = strconv.ParseFloat(rect.get("y"), 64)
	c.R.W, _ = strconv.ParseFloat(rect.get("width"), 64)
	c.R.H, _ = strconv.ParseFloat(rect.get("height"), 64)
	c.R.X, c.R.Y = c.R.X+dx, c.R.Y+dy
	st := parseStyleDecls(rect.get("style"))
	c.Fill = styleColor(st, "fill")
	c.Stroke = styleColor(st, "stroke")
	var lay labelLayout
	c.Label, c.TextColor, lay = findLabel(g, fontPx)
	c.Label = applyLabelWrap(c.Label, lay)
	return c, c.R.W > 0 && c.R.H > 0
}

// nodeID extracts the mermaid node id from a DOM id, e.g. "CI" from
// "my-svg-flowchart-CI-0". The infix differs per diagram type.
func nodeID(id string) string {
	for _, prefix := range []string{"flowchart-", "state-", "classId-", "entity-"} {
		i := strings.Index(id, prefix)
		if i < 0 {
			continue
		}
		s := id[i+len(prefix):]
		if j := strings.LastIndex(s, "-"); j > 0 {
			return s[:j]
		}
		return s
	}
	return id
}

// clusterID extracts "LLM" from "my-svg-LLM".
func clusterID(id string) string {
	// mermaid emits "<svgid>-<subgraphid>"; svg id may itself contain dashes,
	// so take the last segment.
	if j := strings.LastIndex(id, "-"); j >= 0 {
		return id[j+1:]
	}
	return id
}

func parseNode(g *xnode, dtype string, fontPx, dx, dy float64) (Node, bool) {
	n := Node{ID: nodeID(g.get("id"))}
	tx, ty, ok := parseTranslate(g.get("transform"))
	if !ok {
		return n, false
	}
	tx, ty = tx+dx, ty+dy
	shape := findShapeEl(g)
	if shape == nil {
		return n, false
	}
	st := parseStyleDecls(shape.get("style"))
	n.Fill = styleColor(st, "fill")
	n.Stroke = styleColor(st, "stroke")

	// the shape element may carry its own translate on top of the node's
	sx, sy, _ := parseTranslate(shape.get("transform"))
	tx, ty = tx+sx, ty+sy

	attr := func(name string) float64 {
		v, _ := strconv.ParseFloat(shape.get(name), 64)
		return v
	}

	switch shape.tag {
	case "rect":
		w, h := attr("width"), attr("height")
		n.Kind = KindRect
		n.R = Rect{X: tx + attr("x"), Y: ty + attr("y"), W: w, H: h}
		// stadium nodes ([...]) are rects with rx = h/2
		if rx := attr("rx"); rx > 0 && math.Min(w, h) > 0 {
			n.Adj = int(math.Min(math.Round(rx*100000/math.Min(w, h)), 50000))
		}
	case "polygon":
		nums := reNumber.FindAllString(shape.get("points"), -1)
		if len(nums) < 6 {
			return n, false
		}
		var pts []Pt
		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		for i := 0; i+1 < len(nums); i += 2 {
			x, _ := strconv.ParseFloat(nums[i], 64)
			y, _ := strconv.ParseFloat(nums[i+1], 64)
			pts = append(pts, Pt{tx + x, ty + y})
			minX, maxX = math.Min(minX, tx+x), math.Max(maxX, tx+x)
			minY, maxY = math.Min(minY, ty+y), math.Max(maxY, ty+y)
		}
		n.R = Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
		n.Poly = pts
		switch {
		case len(pts) == 4 && isDiamond(pts, n.R):
			n.Kind = KindDiamond
		case len(pts) == 6:
			n.Kind = KindHexagon
		default:
			n.Kind = KindPolygon
		}
	case "circle":
		r := attr("r")
		n.Kind = KindEllipse
		n.R = Rect{X: tx + attr("cx") - r, Y: ty + attr("cy") - r, W: 2 * r, H: 2 * r}
	case "ellipse":
		rx, ry := attr("rx"), attr("ry")
		n.Kind = KindEllipse
		n.R = Rect{X: tx + attr("cx") - rx, Y: ty + attr("cy") - ry, W: 2 * rx, H: 2 * ry}
	case "path":
		d := shape.get("d")
		if m := reCylinderArc.FindStringSubmatch(d); m != nil && sx < 0 && sy < 0 {
			// cylinders start with "M0,ry a rx,ry ..." and are centered via
			// transform="translate(-w/2, -h/2)"
			w, h := -2*sx, -2*sy
			n.Kind = KindCylinder
			n.R = Rect{X: tx, Y: ty, W: w, H: h}
			// derive the "can" adjust: ry = ss*adj/200000
			ry, _ := strconv.ParseFloat(m[2], 64)
			if ss := math.Min(w, h); ss > 0 && ry > 0 {
				n.Adj = int(math.Round(ry * 200000 / ss))
			}
			break
		}
		// other path shapes (stadium pills, ...): bounding box from the
		// coordinate pairs of the path data, rendered as a full-round rect
		nums := reNumber.FindAllString(d, -1)
		if len(nums) < 4 {
			return n, false
		}
		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		for i := 0; i+1 < len(nums); i += 2 {
			x, _ := strconv.ParseFloat(nums[i], 64)
			y, _ := strconv.ParseFloat(nums[i+1], 64)
			minX, maxX = math.Min(minX, x), math.Max(maxX, x)
			minY, maxY = math.Min(minY, y), math.Max(maxY, y)
		}
		n.Kind = KindRect
		n.Adj = 50000
		n.R = Rect{X: tx + minX, Y: ty + minY, W: maxX - minX, H: maxY - minY}
	default:
		return n, false
	}
	var lay labelLayout
	n.Label, n.TextColor, lay = findLabel(g, fontPx)
	n.Label = applyLabelWrap(n.Label, lay)
	if dtype == "state" {
		applyStateNodeStyle(g, &n)
	}
	return n, n.R.W > 0 && n.R.H > 0
}

// applyStateNodeStyle fills start / end pseudo states solid dark, matching
// the mermaid rendering (they have no label).
func applyStateNodeStyle(g *xnode, n *Node) {
	marker := false
	g.walk(func(k *xnode) {
		if k.hasClass("state-start") || k.hasClass("state-end") {
			marker = true
		}
	})
	if marker || strings.HasPrefix(n.ID, "root_start") || strings.HasPrefix(n.ID, "root_end") {
		n.Fill = orDefault(n.Fill, defTextColor)
		n.Stroke = orDefault(n.Stroke, defTextColor)
		n.Label = nil
	}
}

// isDiamond reports whether a 4-vertex polygon has its vertices at the
// midpoints of its bounding-box edges (as opposed to e.g. a parallelogram).
func isDiamond(pts []Pt, r Rect) bool {
	tol := math.Max(r.W, r.H) * 0.05
	onMid := func(p Pt) bool {
		mx := math.Abs(p.X-r.Cx()) < tol && (math.Abs(p.Y-r.Y) < tol || math.Abs(p.Y-r.Y-r.H) < tol)
		my := math.Abs(p.Y-r.Cy()) < tol && (math.Abs(p.X-r.X) < tol || math.Abs(p.X-r.X-r.W) < tol)
		return mx || my
	}
	for _, p := range pts {
		if !onMid(p) {
			return false
		}
	}
	return true
}

// arrowType maps a mermaid marker reference to the closest OOXML line-end
// type. Hollow variants (extension, aggregation) and crow's feet have no
// exact OOXML equivalent and are approximated.
func arrowType(markerURL string) string {
	m := strings.ToLower(markerURL)
	switch {
	case m == "":
		return ""
	case strings.Contains(m, "cross"):
		return "arrow"
	case strings.Contains(m, "circle") || strings.Contains(m, "lollipop") ||
		strings.Contains(m, "zeroorone"):
		return "oval"
	case strings.Contains(m, "composition") || strings.Contains(m, "aggregation") ||
		strings.Contains(m, "diamond"):
		return "diamond"
	case strings.Contains(m, "dependency") || strings.Contains(m, "ormore"):
		return "arrow"
	case strings.Contains(m, "onlyone") || strings.Contains(m, "mdparent"):
		return ""
	default:
		// point / arrowhead / extension / barb / ...
		return "triangle"
	}
}

func parseEdge(p *xnode, dx, dy float64) (Edge, bool) {
	// Prefer data-id: it is the bare edge id (e.g. "L_A_B_0") without the
	// svg-root prefix, so splitEdgeID can't be thrown off by an "L_"/"id_"
	// substring that happens to appear in the root id.
	e := Edge{ID: orDefault(p.get("data-id"), p.get("id"))}
	cls := " " + p.get("class") + " "
	e.Dashed = strings.Contains(cls, "edge-pattern-dashed") || strings.Contains(cls, "edge-pattern-dotted")
	e.Thick = strings.Contains(cls, "edge-thickness-thick")
	e.StartArrow = arrowType(p.get("marker-start"))
	e.EndArrow = arrowType(p.get("marker-end"))
	if dp := p.get("data-points"); dp != "" {
		raw, err := base64.StdEncoding.DecodeString(dp)
		if err == nil {
			var pts []struct{ X, Y float64 }
			if json.Unmarshal(raw, &pts) == nil {
				for _, q := range pts {
					e.Points = append(e.Points, Pt{q.X + dx, q.Y + dy})
				}
			}
		}
	}
	if len(e.Points) < 2 {
		// fallback: collect coordinate pairs from the path data (M/C/L absolute)
		nums := reNumber.FindAllString(p.get("d"), -1)
		for i := 0; i+1 < len(nums); i += 2 {
			x, _ := strconv.ParseFloat(nums[i], 64)
			y, _ := strconv.ParseFloat(nums[i+1], 64)
			e.Points = append(e.Points, Pt{x + dx, y + dy})
		}
	}
	return e, len(e.Points) >= 2
}

func parseEdgeLabel(g *xnode, fontPx, dx, dy float64) (EdgeLabel, bool) {
	l := EdgeLabel{}
	cx, cy, ok := parseTranslate(g.get("transform"))
	if !ok {
		return l, false // labels of unlabeled edges have no transform
	}
	l.C = Pt{cx + dx, cy + dy}
	var fo *xnode
	g.walk(func(n *xnode) {
		if fo == nil && n.tag == "foreignObject" {
			fo = n
		}
	})
	if fo == nil {
		return l, false
	}
	l.W, _ = strconv.ParseFloat(fo.get("width"), 64)
	l.H, _ = strconv.ParseFloat(fo.get("height"), 64)
	var lay labelLayout
	l.Label, l.TextColor, lay = findLabel(g, fontPx)
	l.Label = applyLabelWrap(l.Label, lay)
	return l, l.W > 0 && l.H > 0 && len(l.Label) > 0
}

// resolveEdgeEndpoints fills Edge.From / Edge.To, first from the edge id
// ("...-L_<from>_<to>_<n>"), falling back to nearest-boundary geometry.
// Clusters count as connectable shapes too (composite states).
func resolveEdgeEndpoints(d *Diagram) {
	known := map[string]bool{}
	for _, n := range d.Nodes {
		known[n.ID] = true
	}
	for _, c := range d.Clusters {
		known[c.ID] = true
	}
	for i := range d.Edges {
		e := &d.Edges[i]
		if from, to, ok := splitEdgeID(e.ID, known); ok {
			e.From, e.To = from, to
			continue
		}
		e.From = nearestNodeID(d, e.Points[0])
		e.To = nearestNodeID(d, e.Points[len(e.Points)-1])
	}
}

// splitEdgeID parses "my-svg-L_CI_FILL_0" (flowchart) or
// "my-svg-id_Animal_Dog_1" (class) against the set of known node ids,
// handling node ids that themselves contain underscores.
func splitEdgeID(id string, known map[string]bool) (string, string, bool) {
	var rest string
	if i := strings.Index(id, "L_"); i >= 0 {
		rest = id[i+2:]
	} else if i := strings.Index(id, "id_"); i >= 0 {
		rest = id[i+3:]
	} else {
		return "", "", false
	}
	// strip trailing "_<number>"
	if j := strings.LastIndex(rest, "_"); j > 0 {
		if _, err := strconv.Atoi(rest[j+1:]); err == nil {
			rest = rest[:j]
		}
	}
	for j := 1; j < len(rest); j++ {
		if rest[j] != '_' {
			continue
		}
		a, b := rest[:j], rest[j+1:]
		if known[a] && known[b] {
			return a, b, true
		}
	}
	return "", "", false
}

// nearestNodeID returns the node or cluster whose boundary is closest to p.
func nearestNodeID(d *Diagram, p Pt) string {
	best, bestScore := "", math.Inf(1)
	consider := func(id string, r Rect) {
		if r.W == 0 || r.H == 0 {
			return
		}
		dx := math.Abs(p.X-r.Cx()) / (r.W / 2)
		dy := math.Abs(p.Y-r.Cy()) / (r.H / 2)
		// distance of the normalized max-metric from the boundary (=1)
		score := math.Abs(math.Max(dx, dy) - 1)
		if score < bestScore {
			best, bestScore = id, score
		}
	}
	for _, n := range d.Nodes {
		consider(n.ID, n.R)
	}
	for _, c := range d.Clusters {
		consider(c.ID, c.R)
	}
	if bestScore > 0.5 {
		return ""
	}
	return best
}
