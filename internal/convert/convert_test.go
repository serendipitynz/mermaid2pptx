package convert

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestParseStyleAndColors(t *testing.T) {
	st := parseStyleDecls("fill:#F8D7DA !important;stroke:#D9534F !important")
	if got := styleColor(st, "fill"); got != "F8D7DA" {
		t.Errorf("fill = %q", got)
	}
	if got := styleColor(st, "stroke"); got != "D9534F" {
		t.Errorf("stroke = %q", got)
	}
	if got := cssColorToHex("rgb(122, 26, 26)"); got != "7A1A1A" {
		t.Errorf("rgb = %q", got)
	}
	if got := cssColorToHex("#abc"); got != "AABBCC" {
		t.Errorf("short hex = %q", got)
	}
}

func TestExtractLabel(t *testing.T) {
	frag := `<foreignObject width="200" height="96"><div xmlns="http://www.w3.org/1999/xhtml" style="color: rgb(122, 26, 26) !important;"><span class="nodeLabel"><p>line one<br />line <b>two</b></p></span></div></foreignObject>`
	root, err := parseXMLTree(strings.NewReader(frag))
	if err != nil {
		t.Fatal(err)
	}
	paras, color := extractLabel(root)
	if len(paras) != 2 {
		t.Fatalf("paras = %d, want 2", len(paras))
	}
	if paras[0].Runs[0].Text != "line one" {
		t.Errorf("para0 = %q", paras[0].Runs[0].Text)
	}
	last := paras[1].Runs[len(paras[1].Runs)-1]
	if last.Text != "two" || !last.Bold {
		t.Errorf("bold run = %+v", last)
	}
	if color != "7A1A1A" {
		t.Errorf("color = %q", color)
	}
}

func TestSplitEdgeID(t *testing.T) {
	known := map[string]bool{"CI": true, "FILL": true, "A_B": true, "C": true}
	if f, to, ok := splitEdgeID("my-svg-L_CI_FILL_0", known); !ok || f != "CI" || to != "FILL" {
		t.Errorf("got %q %q %v", f, to, ok)
	}
	if f, to, ok := splitEdgeID("x-L_A_B_C_1", known); !ok || f != "A_B" || to != "C" {
		t.Errorf("underscore id: got %q %q %v", f, to, ok)
	}
}

func identityEMU(p Pt) (int64, int64) { return int64(p.X), int64(p.Y) }

func TestConnectorGeomPerpendicular(t *testing.T) {
	// exits right of source, arrives at top of target (down-right)
	g := connectorGeom(Pt{100, 50}, Pt{300, 200}, sideRight, sideTop, identityEMU)
	if !g.ok || g.prst != "curvedConnector2" {
		t.Fatalf("geom = %+v", g)
	}
	if g.rot != 0 || g.flipH || g.flipV {
		t.Errorf("unexpected orientation: %+v", g)
	}
	if g.offX != 100 || g.offY != 50 || g.cx != 200 || g.cy != 150 {
		t.Errorf("bbox: %+v", g)
	}
}

func TestConnectorGeomVerticalParallel(t *testing.T) {
	// CI -> FILL from graph1: exits bottom, arrives top, offset left
	g := connectorGeom(Pt{418, 159}, Pt{369, 233}, sideBottom, sideTop, identityEMU)
	if !g.ok || g.prst != "curvedConnector3" {
		t.Fatalf("geom = %+v", g)
	}
	if g.rot != 5400000 {
		t.Errorf("rot = %d", g.rot)
	}
	// pre-rotation frame: S maps to top-left, E to bottom-right -> no flips
	if g.flipH || g.flipV {
		t.Errorf("flips: %+v", g)
	}
	// pre-rot ext swaps w/h of the world box
	if g.cx != 74 || g.cy != 49 {
		t.Errorf("ext: %+v", g)
	}
}

func TestConnectorGeomStraight(t *testing.T) {
	g := connectorGeom(Pt{100, 200}, Pt{100, 100}, sideTop, sideBottom, identityEMU)
	if !g.ok || g.prst != "straightConnector1" {
		t.Fatalf("geom = %+v", g)
	}
	if !g.flipV || g.flipH {
		t.Errorf("upward line should flipV: %+v", g)
	}
}

func TestConnectorGeomUTurnFallsBack(t *testing.T) {
	// exits bottom, arrives at bottom of a target above: not representable
	g := connectorGeom(Pt{100, 200}, Pt{300, 150}, sideBottom, sideBottom, identityEMU)
	if g.ok {
		t.Errorf("expected fallback, got %+v", g)
	}
}

func mustParseFile(t *testing.T, path string) *Diagram {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("%s not available: %v", path, err)
	}
	defer f.Close()
	d, err := ParseMermaidSVG(f)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func nodeByID(d *Diagram, id string) *Node {
	for i := range d.Nodes {
		if d.Nodes[i].ID == id {
			return &d.Nodes[i]
		}
	}
	return nil
}

// graph1: LR flow covering the node shape variety.
func TestParseGraph1(t *testing.T) {
	d := mustParseFile(t, "../../sample/graph1.svg")
	if len(d.Nodes) != 8 {
		t.Errorf("nodes = %d, want 8", len(d.Nodes))
	}
	if len(d.Edges) != 8 {
		t.Errorf("edges = %d, want 8", len(d.Edges))
	}
	if len(d.Labels) != 2 {
		t.Errorf("labels = %d, want 2", len(d.Labels))
	}
	wantKinds := map[string]ShapeKind{
		"START": KindRect, // stadium -> full-round rect
		"INPUT": KindPolygon,
		"VALID": KindDiamond,
		"PROC":  KindPolygon,
		"ERR":   KindEllipse,
		"DB":    KindCylinder,
		"HEX":   KindHexagon,
		"DONE":  KindRect,
	}
	for id, kind := range wantKinds {
		n := nodeByID(d, id)
		if n == nil {
			t.Errorf("node %s not found", id)
			continue
		}
		if n.Kind != kind {
			t.Errorf("node %s kind = %d, want %d", id, n.Kind, kind)
		}
	}
	if n := nodeByID(d, "START"); n != nil && n.Adj != 50000 {
		t.Errorf("START (stadium) adj = %d, want 50000", n.Adj)
	}
	if n := nodeByID(d, "VALID"); n != nil && (n.Fill != "FFF3CD" || n.Stroke != "E0A800") {
		t.Errorf("VALID colors: fill=%s stroke=%s", n.Fill, n.Stroke)
	}
	thick := 0
	for _, e := range d.Edges {
		if e.Thick {
			thick++
		}
		if e.From == "" || e.To == "" {
			t.Errorf("unresolved edge %s", e.ID)
		}
	}
	if thick != 1 {
		t.Errorf("thick = %d, want 1", thick)
	}
}

// graph2: nested subgraphs, cylinders, dashed edges.
func TestParseGraph2(t *testing.T) {
	d := mustParseFile(t, "../../sample/graph2.svg")
	if len(d.Nodes) != 7 {
		t.Errorf("nodes = %d, want 7", len(d.Nodes))
	}
	if len(d.Clusters) != 3 {
		t.Errorf("clusters = %d, want 3", len(d.Clusters))
	}
	cylinders := 0
	for _, n := range d.Nodes {
		if n.Kind == KindCylinder {
			cylinders++
			if n.Adj <= 0 {
				t.Errorf("cylinder %s without adj", n.ID)
			}
		}
	}
	if cylinders != 2 {
		t.Errorf("cylinders = %d, want 2", cylinders)
	}
	dashed := 0
	for _, e := range d.Edges {
		if e.Dashed {
			dashed++
		}
	}
	if dashed != 2 {
		t.Errorf("dashed = %d, want 2", dashed)
	}
}

// graph4: curve=basis directive, wrappingWidth long labels, and a diamond
// gate standing between two clusters.
func TestParseGraph4(t *testing.T) {
	d := mustParseFile(t, "../../sample/graph4.svg")
	if len(d.Clusters) != 2 {
		t.Errorf("clusters = %d, want 2", len(d.Clusters))
	}
	if len(d.Nodes) != 6 {
		t.Errorf("nodes = %d, want 6", len(d.Nodes))
	}
	gate := nodeByID(d, "GATE")
	if gate == nil || gate.Kind != KindDiamond {
		t.Fatalf("GATE diamond not found: %+v", gate)
	}
	// regression: the diamond polygon carries its own translate; ignoring it
	// shifted the shape up-right by half its size. The bbox must be centered
	// on the node group's translate origin (198, 373).
	if math.Abs(gate.R.Cx()-198) > 1 || math.Abs(gate.R.Cy()-373) > 1 {
		t.Errorf("GATE center = (%g, %g), want (~198, ~373)", gate.R.Cx(), gate.R.Cy())
	}
	// wrapped long label stays one paragraph (no <br/>)
	form := nodeByID(d, "FORM")
	if form == nil || len(form.Label) != 1 {
		t.Fatalf("FORM label paras: %+v", form)
	}
	if n := len([]rune(form.Label[0].Runs[0].Text)); n < 30 {
		t.Errorf("FORM label too short (%d runes), wrapping sample broken?", n)
	}
	// the gate connects across both clusters
	deg := 0
	for _, e := range d.Edges {
		if e.From == "GATE" || e.To == "GATE" {
			deg++
		}
	}
	if deg != 3 {
		t.Errorf("GATE degree = %d, want 3", deg)
	}
}

// graph5: stateDiagram-v2 with a composite state.
func TestParseGraph5(t *testing.T) {
	d := mustParseFile(t, "../../sample/graph5.svg")
	if d.Type != "state" {
		t.Fatalf("type = %q", d.Type)
	}
	if len(d.Nodes) != 7 {
		t.Errorf("nodes = %d, want 7", len(d.Nodes))
	}
	if len(d.Clusters) != 1 || d.Clusters[0].ID != "Running" {
		t.Fatalf("clusters: %+v", d.Clusters)
	}
	// composite content is nested in a translated root; the cluster rect
	// (8,8) must be shifted by that root's translate (71.5, 178)
	c := d.Clusters[0]
	if math.Abs(c.R.X-79.5) > 0.1 || math.Abs(c.R.Y-186) > 0.1 {
		t.Errorf("Running cluster at (%g, %g), want (79.5, 186)", c.R.X, c.R.Y)
	}
	start := nodeByID(d, "root_start")
	if start == nil || start.Kind != KindEllipse || start.Fill != "333333" {
		t.Errorf("root_start: %+v", start)
	}
	for _, e := range d.Edges {
		if e.From == "" || e.To == "" {
			t.Errorf("unresolved edge %s", e.ID)
		}
		if e.EndArrow != "triangle" {
			t.Errorf("edge %s end arrow = %q", e.ID, e.EndArrow)
		}
	}
	if len(d.Edges) != 8 {
		t.Errorf("edges = %d, want 8", len(d.Edges))
	}
	if len(d.Labels) != 5 {
		t.Errorf("labels = %d, want 5", len(d.Labels))
	}
}

// graph6: sequence diagram (actors, lifelines, messages, activation, note).
func TestParseGraph6(t *testing.T) {
	d := mustParseFile(t, "../../sample/graph6.svg")
	if d.Type != "sequence" {
		t.Fatalf("type = %q", d.Type)
	}
	// 3 participants drawn top+bottom + 2 activations + 1 note
	if len(d.Nodes) != 9 {
		t.Errorf("nodes = %d, want 9", len(d.Nodes))
	}
	// 3 lifelines + 6 messages
	if len(d.Lines) != 9 {
		t.Errorf("lines = %d, want 9", len(d.Lines))
	}
	dashed, arrows, above := 0, 0, 0
	for _, l := range d.Lines {
		if l.Dashed {
			dashed++
		}
		if l.EndArrow != "" {
			arrows++
		}
		if l.Above {
			above++
		}
	}
	if dashed != 3 || arrows != 6 || above != 6 {
		t.Errorf("dashed=%d arrows=%d above=%d, want 3/6/6", dashed, arrows, above)
	}
	// message labels are free text boxes
	if len(d.TextBoxes) != 6 {
		t.Errorf("textboxes = %d, want 6", len(d.TextBoxes))
	}
	// actor label is merged into its box
	found := false
	for _, n := range d.Nodes {
		if n.ID == "B" && len(n.Label) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("actor B has no merged label")
	}
}

// graph7: class diagram (compartment boxes, realization / aggregation).
func TestParseGraph7(t *testing.T) {
	d := mustParseFile(t, "../../sample/graph7.svg")
	if d.Type != "class" {
		t.Fatalf("type = %q", d.Type)
	}
	if len(d.Nodes) != 4 {
		t.Errorf("nodes = %d, want 4", len(d.Nodes))
	}
	for _, n := range d.Nodes {
		if n.Kind != KindBox {
			t.Errorf("node %s kind = %d, want KindBox", n.ID, n.Kind)
		}
	}
	// 2 dividers per class
	if len(d.Lines) != 8 {
		t.Errorf("divider lines = %d, want 8", len(d.Lines))
	}
	if len(d.Edges) != 3 {
		t.Fatalf("edges = %d, want 3", len(d.Edges))
	}
	dashedTri, diamond := 0, 0
	for _, e := range d.Edges {
		if e.From == "" || e.To == "" {
			t.Errorf("unresolved edge %s", e.ID)
		}
		if e.Dashed && (e.StartArrow == "triangle" || e.EndArrow == "triangle") {
			dashedTri++
		}
		if e.StartArrow == "diamond" || e.EndArrow == "diamond" {
			diamond++
		}
	}
	if dashedTri != 2 || diamond != 1 {
		t.Errorf("realization=%d aggregation=%d, want 2/1", dashedTri, diamond)
	}
	if len(d.TextBoxes) < 12 {
		t.Errorf("textboxes = %d, want >= 12", len(d.TextBoxes))
	}
}

// graph8: ER diagram (entity tables, crow's foot approximation).
func TestParseGraph8(t *testing.T) {
	d := mustParseFile(t, "../../sample/graph8.svg")
	if d.Type != "er" {
		t.Fatalf("type = %q", d.Type)
	}
	if len(d.Nodes) != 4 {
		t.Errorf("entities = %d, want 4", len(d.Nodes))
	}
	if len(d.Edges) != 3 {
		t.Errorf("edges = %d, want 3", len(d.Edges))
	}
	for _, e := range d.Edges {
		if e.From == "" || e.To == "" {
			t.Errorf("unresolved edge %s", e.ID)
		}
		if e.EndArrow != "arrow" {
			t.Errorf("edge %s end = %q, want arrow (crow's foot)", e.ID, e.EndArrow)
		}
	}
	if len(d.Labels) != 3 {
		t.Errorf("relation labels = %d, want 3", len(d.Labels))
	}
	if len(d.TextBoxes) < 30 {
		t.Errorf("textboxes = %d, want >= 30", len(d.TextBoxes))
	}
}

// TestGeneratePackage builds a full pptx in memory and checks that every part
// is well-formed XML and shapes stay inside the slide.
func TestGeneratePackage(t *testing.T) {
	for _, name := range []string{
		"../../sample/graph1.svg",
		"../../sample/graph2.svg",
		"../../sample/graph3.svg",
		"../../sample/graph4.svg",
		"../../sample/graph5.svg",
		"../../sample/graph6.svg",
		"../../sample/graph7.svg",
		"../../sample/graph8.svg",
	} {
		d := mustParseFile(t, name)
		slideXML := GenerateSlideXML(d, Options{Font: "Noto Sans JP", MarginIn: 0.3})
		var buf bytes.Buffer
		if err := WritePPTX(&buf, slideXML); err != nil {
			t.Fatal(err)
		}
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatal(err)
		}
		if len(zr.File) != 13 {
			t.Errorf("%s: parts = %d, want 13", name, len(zr.File))
		}
		for _, zf := range zr.File {
			if !strings.HasSuffix(zf.Name, ".xml") && !strings.HasSuffix(zf.Name, ".rels") {
				continue
			}
			rc, err := zf.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			dec := xml.NewDecoder(bytes.NewReader(data))
			for {
				_, err := dec.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("%s: %s is not well-formed: %v", name, zf.Name, err)
				}
			}
		}
		if !strings.Contains(slideXML, "<p:cxnSp>") {
			t.Errorf("%s: no connectors emitted", name)
		}
		if strings.Contains(slideXML, `x="-`) {
			t.Errorf("%s: negative shape offset emitted", name)
		}
	}
}

// endpointTolEMU is the allowed error when reconstructing a connector's
// world-space endpoints. Measured worst case across all samples is < 1 EMU
// (float rounding via math.Round); a real geometry regression — a wrong
// attach side or flip — misplaces an endpoint by thousands of EMU, so this
// bound is tight enough to catch one yet immune to rounding noise.
const endpointTolEMU = 2.0

// TestConnectorEndpointsMatchSVG is the regression guard for connector
// geometry: every edge connector's reconstructed endpoints (undoing the
// xfrm flip/rotation) must land on some edge's SVG data-points, transformed
// through the same px->EMU fit. Re-run this whenever geom.go / slide.go
// placement math changes.
func TestConnectorEndpointsMatchSVG(t *testing.T) {
	for _, f := range []string{"1", "2", "3", "4", "5", "7", "8"} {
		name := "../../sample/graph" + f + ".svg"
		d := mustParseFile(t, name)
		scale, offX, offY := fitTransform(d, 0.3)
		emu := func(p Pt) (float64, float64) {
			return p.X*emuPerPx*scale + offX, p.Y*emuPerPx*scale + offY
		}
		// expected endpoint pairs (start, end) in EMU, one per edge
		type pair struct{ sx, sy, ex, ey float64 }
		var want []pair
		for _, e := range d.Edges {
			sx, sy := emu(e.Points[0])
			ex, ey := emu(e.Points[len(e.Points)-1])
			want = append(want, pair{sx, sy, ex, ey})
		}

		root, err := parseXMLTree(strings.NewReader(GenerateSlideXML(d, Options{Font: "X", MarginIn: 0.3})))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		checked := 0
		root.walk(func(nd *xnode) {
			if nd.tag != "cxnSp" {
				return
			}
			var name string
			var xfrm *xnode
			nd.walk(func(k *xnode) {
				if k.tag == "cNvPr" && name == "" {
					name = k.get("name")
				}
				if k.tag == "xfrm" && xfrm == nil {
					xfrm = k
				}
			})
			if !strings.HasPrefix(name, "edge ") || xfrm == nil {
				return // "line" connectors (lifelines, dividers) aren't data-points based
			}
			sx, sy, ex, ey := reconstructConnector(xfrm)
			best := math.Inf(1)
			for _, w := range want {
				d1 := max4(math.Abs(sx-w.sx), math.Abs(sy-w.sy), math.Abs(ex-w.ex), math.Abs(ey-w.ey))
				if d1 < best {
					best = d1
				}
			}
			if best > endpointTolEMU {
				t.Errorf("%s: connector %q endpoints off by %.1f EMU (tol %.0f)", name, name, best, endpointTolEMU)
			}
			checked++
		})
		if checked == 0 {
			t.Errorf("%s: no edge connectors to verify", name)
		}
	}
}

// reconstructConnector recovers the world-space start (local 0,0) and end
// (local cx,cy) of a preset connector from its xfrm, undoing flipH/flipV and
// the clockwise rotation about the box center.
func reconstructConnector(xfrm *xnode) (sx, sy, ex, ey float64) {
	atof := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	var ox, oy, cx, cy float64
	for _, k := range xfrm.kids {
		switch k.tag {
		case "off":
			ox, oy = atof(k.get("x")), atof(k.get("y"))
		case "ext":
			cx, cy = atof(k.get("cx")), atof(k.get("cy"))
		}
	}
	rot := atof(xfrm.get("rot")) / 60000 * math.Pi / 180
	fh, fv := xfrm.get("flipH") == "1", xfrm.get("flipV") == "1"
	world := func(px, py float64) (float64, float64) {
		if fh {
			px = cx - px
		}
		if fv {
			py = cy - py
		}
		mx, my := cx/2, cy/2
		dx, dy := px-mx, py-my
		return ox + mx + dx*math.Cos(rot) - dy*math.Sin(rot),
			oy + my + dx*math.Sin(rot) + dy*math.Cos(rot)
	}
	sx, sy = world(0, 0)
	ex, ey = world(cx, cy)
	return
}

func max4(a, b, c, d float64) float64 {
	return math.Max(math.Max(a, b), math.Max(c, d))
}

// The whole point of labelBoxSize is that the text still fits inside the
// roundRect's presetTextRectangle, which no XML well-formedness check catches.
func TestLabelBoxFitsPresetTextRectangle(t *testing.T) {
	cases := []struct{ w, h float64 }{
		{20, 24},   // "OK" - square-ish, the inset follows the width
		{45.7, 24}, // "HTTPS"
		{64, 24},   // "書き込み"
		{200, 48},  // wide, two lines
	}
	for _, c := range cases {
		w, h := labelBoxSize(c.w, c.h)
		inner := w - 2*roundRectInset*math.Min(w, h)
		want := c.w*labelWidthSafety + 2*labelPadPx
		if inner < want-1e-9 {
			t.Errorf("labelBoxSize(%g, %g) = %g x %g: text area %g < %g", c.w, c.h, w, h, inner, want)
		}
		if h < c.h {
			t.Errorf("labelBoxSize(%g, %g): height %g shrank below the text", c.w, c.h, h)
		}
	}
}
