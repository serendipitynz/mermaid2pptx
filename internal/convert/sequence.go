package convert

import (
	"strconv"
	"strings"
)

// Sequence diagrams have a flat structure of absolutely-positioned rects
// (actors, notes, activations), lines (lifelines, messages) and texts. Texts
// whose anchor falls inside a rect become that shape's label; the rest become
// free-standing text boxes.

type seqText struct {
	anchor Pt // reference point used for the inside-a-rect test
	x      float64
	middle bool // text-anchor: middle
	color  string
	paras  []Para
}

func parseSequence(root *xnode, d *Diagram) {
	var texts []seqText

	var visit func(n *xnode)
	visit = func(n *xnode) {
		switch n.tag {
		case "defs":
			return // markers
		case "rect":
			if r, ok := parseSeqRect(n); ok {
				d.Nodes = append(d.Nodes, r)
			}
		case "line":
			d.Lines = append(d.Lines, parseSeqLine(n))
		case "text":
			if t, ok := parseSeqText(n); ok {
				texts = append(texts, t)
			}
		case "path":
			// self-messages and other decorated paths
			if n.get("data-et") == "message" {
				if e, ok := parseEdge(n, 0, 0); ok {
					d.Edges = append(d.Edges, e)
				}
			}
		}
		for _, k := range n.kids {
			visit(k)
		}
	}
	visit(root)

	// attach texts to the smallest enclosing rect, else emit as text boxes
	for _, t := range texts {
		if host := smallestEnclosing(d.Nodes, t.anchor); host != nil {
			host.Label = append(host.Label, t.paras...)
			host.TextColor = orDefault(host.TextColor, t.color)
			continue
		}
		w := estTextWidth(t.paras)
		x := t.x
		if t.middle {
			x -= w / 2
		}
		d.TextBoxes = append(d.TextBoxes, TextBox{
			R:     Rect{X: x, Y: t.anchor.Y - 12, W: w, H: 24 * float64(len(t.paras))},
			Color: t.color,
			Label: t.paras,
		})
	}
}

func parseSeqRect(n *xnode) (Node, bool) {
	attr := func(name string) float64 {
		v, _ := strconv.ParseFloat(n.get(name), 64)
		return v
	}
	w, h := attr("width"), attr("height")
	if w <= 0 || h <= 0 {
		return Node{}, false
	}
	nd := Node{
		ID:     n.get("name"),
		Kind:   KindRect,
		R:      Rect{X: attr("x"), Y: attr("y"), W: w, H: h},
		Fill:   elColor(n, "fill"),
		Stroke: elColor(n, "stroke"),
	}
	if rx := attr("rx"); rx > 0 {
		nd.Adj = int(rx * 100000 / minf(w, h))
	} else {
		nd.Kind = KindBox
	}
	return nd, true
}

func parseSeqLine(n *xnode) Line {
	attr := func(name string) float64 {
		v, _ := strconv.ParseFloat(n.get(name), 64)
		return v
	}
	width, _ := strconv.ParseFloat(strings.TrimSuffix(n.get("stroke-width"), "px"), 64)
	return Line{
		P1:         Pt{attr("x1"), attr("y1")},
		P2:         Pt{attr("x2"), attr("y2")},
		Color:      elColor(n, "stroke"),
		WidthPx:    width,
		Dashed:     strings.Contains(n.get("style"), "dasharray") || strings.Contains(n.get("class"), "dashed"),
		StartArrow: arrowType(n.get("marker-start")),
		EndArrow:   arrowType(n.get("marker-end")),
		Above:      n.get("data-et") == "message",
	}
}

func parseSeqText(n *xnode) (seqText, bool) {
	t := seqText{}
	x, _ := strconv.ParseFloat(n.get("x"), 64)
	y, _ := strconv.ParseFloat(n.get("y"), 64)
	t.x = x
	st := n.get("style")
	t.middle = strings.Contains(st, "text-anchor: middle") || n.get("text-anchor") == "middle"
	// "dy=1em" shifts the rendered line one line-height down
	if strings.Contains(n.get("dy"), "em") {
		y += 16
	}
	t.anchor = Pt{x, y}
	t.color = styleColor(parseStyleDecls(st), "fill")

	// direct character data and/or tspan children, one paragraph per tspan
	var cur Para
	flush := func() {
		if !cur.empty() {
			t.paras = append(t.paras, cur)
		}
		cur = Para{}
	}
	for _, k := range n.kids {
		if k.tag == "" {
			if s := strings.TrimSpace(k.text); s != "" {
				cur.Runs = append(cur.Runs, Run{Text: s})
			}
			continue
		}
		if k.tag == "tspan" {
			flush()
			txt := ""
			for _, c := range k.kids {
				if c.tag == "" {
					txt += c.text
				}
			}
			if s := strings.TrimSpace(txt); s != "" {
				cur.Runs = append(cur.Runs, Run{Text: s})
			}
		}
	}
	flush()
	return t, len(t.paras) > 0
}

// smallestEnclosing finds the smallest node rect containing p.
func smallestEnclosing(nodes []Node, p Pt) *Node {
	var best *Node
	for i := range nodes {
		n := &nodes[i]
		if !n.R.contains(p) {
			continue
		}
		if best == nil || n.R.W*n.R.H < best.R.W*best.R.H {
			best = n
		}
	}
	return best
}

// estTextWidth estimates the pixel width of the widest paragraph at the
// default 16px font.
func estTextWidth(paras []Para) float64 {
	widest := 0.0
	for _, p := range paras {
		w := 0.0
		for _, r := range p.Runs {
			for _, c := range r.Text {
				w += runeWidthPx(c)
			}
		}
		if w > widest {
			widest = w
		}
	}
	return widest + 8
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
