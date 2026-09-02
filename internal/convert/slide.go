package convert

import (
	"fmt"
	"math"
	"strings"
)

// Slide geometry constants (16:9 widescreen).
const (
	slideCx     = int64(12192000)
	slideCy     = int64(6858000)
	emuPerPx    = 9525.0 // 96 dpi
	lineWidth   = int64(12700)
	thickWidth  = int64(25400)
	clusterAdj  = 5844   // subtle corner rounding for subgraph containers
	lnSpcPct    = 150000 // matches mermaid's line-height 1.5
	basePt100   = 1200.0 // mermaid 16px = 12pt, in 1/100 pt
	labelPadPx  = 4.0    // extra padding around edge label text
	labelAlpha  = 80000  // edge label background opacity (SVG: rgba(...,0.8))
	textInsetsW = 18288  // ~2px horizontal text inset
	textInsetsH = 9144
	maxUpscale  = 1.25 // upper bound for enlarging small diagrams
	// roundRect's presetTextRectangle is inset on each side by
	// min(w,h) * adj * (1-cos45): the default adj is 16667, so ~4.9%.
	roundRectInset = 0.048815
	// Mermaid measures label widths with the browser's font stack, which is
	// narrower than the output font (notably for Latin text). Widen the
	// background box so it still covers what PowerPoint actually draws.
	labelWidthSafety = 1.12
)

// Options controls slide generation.
type Options struct {
	Font     string
	MarginIn float64
}

// slideGen holds transform state while emitting slide XML.
type slideGen struct {
	b        strings.Builder
	scale    float64 // px -> px on slide
	offX     float64 // EMU
	offY     float64 // EMU
	font     string
	sz       int // font size in 1/100 pt
	nextID   int
	spIDs    map[string]int // node ID -> cNvPr id
	clFill   string         // cluster default fill
	clStroke string         // cluster default stroke
}

func xmlEsc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fitTransform computes the px->EMU affine transform that fits the diagram
// into the slide margins with aspect ratio preserved. It is the single source
// of truth for the placement math (shared with tests).
func fitTransform(d *Diagram, marginIn float64) (scale, offX, offY float64) {
	margin := int64(marginIn * 914400)
	availW := float64(slideCx - 2*margin)
	availH := float64(slideCy - 2*margin)
	scale = math.Min(availW/(d.W*emuPerPx), availH/(d.H*emuPerPx))
	// don't blow up small diagrams (and their fonts) to fill the slide
	scale = math.Min(scale, maxUpscale)
	offX = float64(margin) + (availW-d.W*emuPerPx*scale)/2 - d.MinX*emuPerPx*scale
	offY = float64(margin) + (availH-d.H*emuPerPx*scale)/2 - d.MinY*emuPerPx*scale
	return
}

// GenerateSlideXML renders the diagram to a slide1.xml document.
func GenerateSlideXML(d *Diagram, opt Options) string {
	scale, offX, offY := fitTransform(d, opt.MarginIn)
	clFill, clStroke := d.clusterDefaults()
	g := &slideGen{
		scale: scale,
		offX:  offX,
		offY:  offY,
		font:  opt.Font,
		// floor, not round: a font larger than the geometric scale would
		// outgrow the boxes measured from the SVG.
		sz:       maxInt(100, int(math.Floor(basePt100*scale/50))*50),
		nextID:   2,
		spIDs:    map[string]int{},
		clFill:   clFill,
		clStroke: clStroke,
	}

	g.b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`)

	// z-order: clusters, under-lines, nodes, over-lines, connectors, texts
	nodeRects := map[string]Rect{}
	for _, c := range d.Clusters {
		g.writeCluster(c)
		nodeRects[c.ID] = c.R
	}
	for _, l := range d.Lines {
		if !l.Above {
			g.writeLine(l)
		}
	}
	for _, n := range d.Nodes {
		g.writeNode(n)
		if n.ID != "" {
			nodeRects[n.ID] = n.R
		}
	}
	for _, l := range d.Lines {
		if l.Above {
			g.writeLine(l)
		}
	}
	for _, e := range d.Edges {
		g.writeEdge(e, nodeRects)
	}
	for _, t := range d.TextBoxes {
		g.writeTextBox(t)
	}
	for _, l := range d.Labels {
		g.writeEdgeLabel(l)
	}

	g.b.WriteString(`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`)
	return g.b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (g *slideGen) id() int {
	v := g.nextID
	g.nextID++
	return v
}

func (g *slideGen) emuPt(p Pt) (int64, int64) {
	return int64(math.Round(p.X*emuPerPx*g.scale + g.offX)),
		int64(math.Round(p.Y*emuPerPx*g.scale + g.offY))
}

func (g *slideGen) emuLen(v float64) int64 {
	return int64(math.Round(v * emuPerPx * g.scale))
}

func (g *slideGen) writeXfrm(r Rect) {
	x, y := g.emuPt(Pt{r.X, r.Y})
	fmt.Fprintf(&g.b, `<a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`,
		x, y, g.emuLen(r.W), g.emuLen(r.H))
}

func (g *slideGen) writeFillLine(fill, stroke string, w int64, dashed bool) {
	fmt.Fprintf(&g.b, `<a:solidFill><a:srgbClr val="%s"/></a:solidFill>`, fill)
	fmt.Fprintf(&g.b, `<a:ln w="%d"><a:solidFill><a:srgbClr val="%s"/></a:solidFill>`, w, stroke)
	if dashed {
		g.b.WriteString(`<a:prstDash val="dash"/>`)
	}
	g.b.WriteString(`</a:ln>`)
}

// txBody holds the text body settings that differ between shape kinds.
type txBody struct {
	anchor string // "ctr" or "t"
	// noWrap disables word wrapping, for boxes whose text was laid out
	// upstream (free text boxes, edge labels).
	noWrap bool
	// noInsets drops the default text insets, for boxes whose padding is
	// already part of the shape geometry. The insets are a fixed EMU amount
	// and so do not follow the diagram scale.
	noInsets bool
}

func (g *slideGen) writeTxBody(paras []Para, color string, opt txBody) {
	if len(paras) == 0 {
		return
	}
	wrap := ""
	if opt.noWrap {
		wrap = ` wrap="none"`
	}
	insW, insH := int64(textInsetsW), int64(textInsetsH)
	if opt.noInsets {
		insW, insH = 0, 0
	}
	fmt.Fprintf(&g.b, `<p:txBody><a:bodyPr rtlCol="0" anchor="%s"%s lIns="%d" tIns="%d" rIns="%d" bIns="%d"/><a:lstStyle/>`,
		opt.anchor, wrap, insW, insH, insW, insH)
	for _, p := range paras {
		algn := "ctr"
		if p.Align != "" {
			algn = p.Align
		}
		fmt.Fprintf(&g.b, `<a:p><a:pPr algn="%s"><a:lnSpc><a:spcPct val="%d"/></a:lnSpc></a:pPr>`, algn, lnSpcPct)
		for _, r := range p.Runs {
			g.b.WriteString(`<a:r><a:rPr lang="ja-JP" altLang="en-US" sz="` + fmt.Sprint(g.sz) + `"`)
			if r.Bold {
				g.b.WriteString(` b="1"`)
			}
			if r.Italic {
				g.b.WriteString(` i="1"`)
			}
			g.b.WriteString(`>`)
			fmt.Fprintf(&g.b, `<a:solidFill><a:srgbClr val="%s"/></a:solidFill>`, color)
			fmt.Fprintf(&g.b, `<a:latin typeface="%[1]s"/><a:ea typeface="%[1]s"/>`, xmlEsc(g.font))
			g.b.WriteString(`</a:rPr><a:t>` + xmlEsc(r.Text) + `</a:t></a:r>`)
		}
		g.b.WriteString(`</a:p>`)
	}
	g.b.WriteString(`</p:txBody>`)
}

func (g *slideGen) openSp(id int, name string) {
	fmt.Fprintf(&g.b, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>`, id, xmlEsc(name))
}

func (g *slideGen) writeCluster(c Cluster) {
	id := g.id()
	g.spIDs[c.ID] = id
	g.openSp(id, "cluster "+c.ID)
	g.writeXfrm(c.R)
	fmt.Fprintf(&g.b, `<a:prstGeom prst="roundRect"><a:avLst><a:gd name="adj" fmla="val %d"/></a:avLst></a:prstGeom>`, clusterAdj)
	g.writeFillLine(orDefault(c.Fill, g.clFill), orDefault(c.Stroke, g.clStroke), lineWidth, false)
	g.b.WriteString(`</p:spPr>`)
	g.writeTxBody(c.Label, orDefault(c.TextColor, defTextColor), txBody{anchor: "t"})
	g.b.WriteString(`</p:sp>`)
}

func (g *slideGen) writeNode(n Node) {
	id := g.id()
	g.spIDs[n.ID] = id
	name := n.ID
	if name == "" {
		name = "box"
		if len(n.Label) > 0 && len(n.Label[0].Runs) > 0 {
			name = n.Label[0].Runs[0].Text
		}
	}
	g.openSp(id, name)
	g.writeXfrm(n.R)
	switch n.Kind {
	case KindDiamond:
		g.b.WriteString(`<a:prstGeom prst="diamond"><a:avLst/></a:prstGeom>`)
	case KindCylinder:
		adj := n.Adj
		if adj <= 0 {
			adj = 25000
		}
		fmt.Fprintf(&g.b, `<a:prstGeom prst="can"><a:avLst><a:gd name="adj" fmla="val %d"/></a:avLst></a:prstGeom>`, adj)
	case KindEllipse:
		g.b.WriteString(`<a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom>`)
	case KindHexagon:
		g.b.WriteString(`<a:prstGeom prst="hexagon"><a:avLst/></a:prstGeom>`)
	case KindPolygon:
		g.writePolygonGeom(n)
	case KindBox:
		g.b.WriteString(`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>`)
	default:
		if n.Adj > 0 {
			fmt.Fprintf(&g.b, `<a:prstGeom prst="roundRect"><a:avLst><a:gd name="adj" fmla="val %d"/></a:avLst></a:prstGeom>`, n.Adj)
		} else {
			g.b.WriteString(`<a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>`)
		}
	}
	g.writeFillLine(orDefault(n.Fill, defNodeFill), orDefault(n.Stroke, defNodeStroke), lineWidth, false)
	g.b.WriteString(`</p:spPr>`)
	g.writeTxBody(n.Label, orDefault(n.TextColor, defTextColor), txBody{anchor: "ctr"})
	g.b.WriteString(`</p:sp>`)
}

// writePolygonGeom emits a closed custom-geometry polygon for node shapes
// that have no matching PowerPoint preset (trapezoid, parallelogram, ...).
func (g *slideGen) writePolygonGeom(n Node) {
	cx := max64(g.emuLen(n.R.W), 1)
	cy := max64(g.emuLen(n.R.H), 1)
	fmt.Fprintf(&g.b, `<a:custGeom><a:avLst/><a:gdLst/><a:ahLst/><a:cxnLst/><a:rect l="0" t="0" r="%d" b="%d"/><a:pathLst><a:path w="%d" h="%d">`, cx, cy, cx, cy)
	for i, p := range n.Poly {
		px := g.emuLen(p.X - n.R.X)
		py := g.emuLen(p.Y - n.R.Y)
		if i == 0 {
			fmt.Fprintf(&g.b, `<a:moveTo><a:pt x="%d" y="%d"/></a:moveTo>`, px, py)
		} else {
			fmt.Fprintf(&g.b, `<a:lnTo><a:pt x="%d" y="%d"/></a:lnTo>`, px, py)
		}
	}
	g.b.WriteString(`<a:close/></a:path></a:pathLst></a:custGeom>`)
}

func (g *slideGen) writeEdgeLabel(l EdgeLabel) {
	id := g.id()
	name := "edge label"
	if len(l.Label) > 0 && len(l.Label[0].Runs) > 0 {
		name = l.Label[0].Runs[0].Text
	}
	g.openSp(id, name)
	w, h := labelBoxSize(l.W, l.H)
	r := Rect{X: l.C.X - w/2, Y: l.C.Y - h/2, W: w, H: h}
	g.writeXfrm(r)
	g.b.WriteString(`<a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>`)
	fmt.Fprintf(&g.b, `<a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill><a:ln><a:noFill/></a:ln>`,
		defLabelBg, labelAlpha)
	g.b.WriteString(`</p:spPr>`)
	// Mermaid's label div is white-space: nowrap, so every line break is
	// already a separate paragraph; wrapping here could only be an artifact.
	g.writeTxBody(l.Label, orDefault(l.TextColor, defTextColor), txBody{anchor: "ctr", noWrap: true, noInsets: true})
	g.b.WriteString(`</p:sp>`)
}

// labelBoxSize returns the roundRect background size that keeps a label of the
// given text size inside the shape's presetTextRectangle, in both directions.
func labelBoxSize(textW, textH float64) (w, h float64) {
	innerW := textW*labelWidthSafety + 2*labelPadPx
	// Half the horizontal padding: mermaid's measured height already carries
	// the 1.5 line-height leading, so the glyphs have room built in.
	innerH := textH + labelPadPx
	// The inset is the same on all four sides and follows min(w,h), so the
	// shorter side determines it: min(w,h) = min(innerW,innerH) + 2*inset*min(w,h).
	m := math.Min(innerW, innerH) / (1 - 2*roundRectInset)
	return innerW + 2*roundRectInset*m, innerH + 2*roundRectInset*m
}

func (g *slideGen) writeEdge(e Edge, nodeRects map[string]Rect) {
	S := e.Points[0]
	E := e.Points[len(e.Points)-1]
	w := lineWidth
	if e.Thick {
		w = thickWidth
	}

	srcR, okS := nodeRects[e.From]
	dstR, okE := nodeRects[e.To]
	if okS && okE {
		// Boundary position and travel direction can disagree on slanted
		// shapes (diamond faces); try position-based sides first, then
		// tangent-based ones.
		posS, posE := sideOf(srcR, S), sideOf(dstR, E)
		tanS := tangentSideOut(segmentDir(e.Points, false))
		tanE := tangentSideIn(segmentDir(e.Points, true))
		tried := map[[2]side]bool{}
		for _, c := range [][2]side{{posS, posE}, {posS, tanE}, {tanS, posE}, {tanS, tanE}} {
			if tried[c] {
				continue
			}
			tried[c] = true
			if cg := connectorGeom(S, E, c[0], c[1], g.emuPt); cg.ok {
				g.writeConnector(e, cg, c[0], c[1], w)
				return
			}
		}
	}
	g.writeFreeformEdge(e, w)
}

func (g *slideGen) writeConnector(e Edge, cg connGeom, sS, sE side, w int64) {
	id := g.id()
	name := fmt.Sprintf("edge %s-%s", e.From, e.To)
	fmt.Fprintf(&g.b, `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="%d" name="%s"/><p:cNvCxnSpPr><a:cxnSpLocks/>`, id, xmlEsc(name))
	if fromID, ok := g.spIDs[e.From]; ok {
		fmt.Fprintf(&g.b, `<a:stCxn id="%d" idx="%d"/>`, fromID, sS.cxnIdx())
	}
	if toID, ok := g.spIDs[e.To]; ok {
		fmt.Fprintf(&g.b, `<a:endCxn id="%d" idx="%d"/>`, toID, sE.cxnIdx())
	}
	g.b.WriteString(`</p:cNvCxnSpPr><p:nvPr/></p:nvCxnSpPr><p:spPr>`)
	g.b.WriteString(`<a:xfrm`)
	if cg.rot != 0 {
		fmt.Fprintf(&g.b, ` rot="%d"`, cg.rot)
	}
	if cg.flipH {
		g.b.WriteString(` flipH="1"`)
	}
	if cg.flipV {
		g.b.WriteString(` flipV="1"`)
	}
	fmt.Fprintf(&g.b, `><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`, cg.offX, cg.offY, cg.cx, cg.cy)
	fmt.Fprintf(&g.b, `<a:prstGeom prst="%s"><a:avLst/></a:prstGeom>`, cg.prst)
	g.writeEdgeLine(e, w)
	g.b.WriteString(`</p:spPr></p:cxnSp>`)
}

func (g *slideGen) writeEdgeLine(e Edge, w int64) {
	g.writeLnEnds(w, defEdgeColor, e.Dashed, e.StartArrow, e.EndArrow)
}

// writeLnEnds emits an <a:ln> with optional dash pattern and arrow ends.
func (g *slideGen) writeLnEnds(w int64, color string, dashed bool, startArrow, endArrow string) {
	fmt.Fprintf(&g.b, `<a:ln w="%d" cap="flat"><a:solidFill><a:srgbClr val="%s"/></a:solidFill>`, w, color)
	if dashed {
		g.b.WriteString(`<a:prstDash val="dash"/>`)
	}
	g.b.WriteString(`<a:round/>`)
	fmt.Fprintf(&g.b, `<a:headEnd type="%s"/><a:tailEnd type="%s"/>`,
		orDefault(startArrow, "none"), orDefault(endArrow, "none"))
	g.b.WriteString(`</a:ln>`)
}

// writeLine emits a straight connector for lifelines, dividers and messages.
func (g *slideGen) writeLine(l Line) {
	x1, y1 := g.emuPt(l.P1)
	x2, y2 := g.emuPt(l.P2)
	w := lineWidth
	if l.WidthPx > 0 {
		w = max64(int64(l.WidthPx*emuPerPx*g.scale), 6350)
	}
	id := g.id()
	fmt.Fprintf(&g.b, `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="%d" name="line"/><p:cNvCxnSpPr/><p:nvPr/></p:nvCxnSpPr><p:spPr>`, id)
	g.b.WriteString(`<a:xfrm`)
	if x1 > x2 {
		g.b.WriteString(` flipH="1"`)
	}
	if y1 > y2 {
		g.b.WriteString(` flipV="1"`)
	}
	fmt.Fprintf(&g.b, `><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`,
		min64(x1, x2), min64(y1, y2), max64(x1, x2)-min64(x1, x2), max64(y1, y2)-min64(y1, y2))
	g.b.WriteString(`<a:prstGeom prst="straightConnector1"><a:avLst/></a:prstGeom>`)
	g.writeLnEnds(w, orDefault(l.Color, defEdgeColor), l.Dashed, l.StartArrow, l.EndArrow)
	g.b.WriteString(`</p:spPr></p:cxnSp>`)
}

// writeTextBox emits a borderless, fill-less text shape at an exact position.
func (g *slideGen) writeTextBox(t TextBox) {
	id := g.id()
	name := "text"
	if len(t.Label) > 0 && len(t.Label[0].Runs) > 0 {
		name = t.Label[0].Runs[0].Text
	}
	g.openSp(id, name)
	g.writeXfrm(t.R)
	g.b.WriteString(`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/><a:ln><a:noFill/></a:ln></p:spPr>`)
	g.writeTxBody(t.Label, orDefault(t.Color, defTextColor), txBody{anchor: "ctr", noWrap: true})
	g.b.WriteString(`</p:sp>`)
}

// writeFreeformEdge draws the edge as a custom-geometry polyline (used when
// no preset connector matches, e.g. U-shaped routes).
func (g *slideGen) writeFreeformEdge(e Edge, w int64) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range e.Points {
		minX, maxX = math.Min(minX, p.X), math.Max(maxX, p.X)
		minY, maxY = math.Min(minY, p.Y), math.Max(maxY, p.Y)
	}
	offX, offY := g.emuPt(Pt{minX, minY})
	cx := max64(g.emuLen(maxX-minX), 1)
	cy := max64(g.emuLen(maxY-minY), 1)

	id := g.id()
	g.openSp(id, fmt.Sprintf("edge %s-%s", e.From, e.To))
	fmt.Fprintf(&g.b, `<a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`, offX, offY, cx, cy)
	fmt.Fprintf(&g.b, `<a:custGeom><a:avLst/><a:gdLst/><a:ahLst/><a:cxnLst/><a:rect l="0" t="0" r="0" b="0"/><a:pathLst><a:path w="%d" h="%d">`, cx, cy)
	for i, p := range e.Points {
		px := g.emuLen(p.X - minX)
		py := g.emuLen(p.Y - minY)
		if i == 0 {
			fmt.Fprintf(&g.b, `<a:moveTo><a:pt x="%d" y="%d"/></a:moveTo>`, px, py)
		} else {
			fmt.Fprintf(&g.b, `<a:lnTo><a:pt x="%d" y="%d"/></a:lnTo>`, px, py)
		}
	}
	g.b.WriteString(`</a:path></a:pathLst></a:custGeom><a:noFill/>`)
	g.writeEdgeLine(e, w)
	g.b.WriteString(`</p:spPr></p:sp>`)
}
