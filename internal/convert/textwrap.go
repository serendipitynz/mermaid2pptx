package convert

// runeWidthPx is a rune's advance at mermaid's default 16px font, the one
// width model the package uses where mermaid left no measurement to read.
//
// A single width for every latin glyph is not good enough here: it is wrong by
// up to half a glyph either way ("W" against "i"), which is enough to move a
// line break and, where the text is re-wrapped, to overflow the shape. So the
// latin range carries the real advances of "trebuchet ms", the first font in
// the stack mermaid's own stylesheet sets, taken from the font at 16px. Across
// the 62 latin labels in sample/*.svg, whose foreignObject widths are the
// browser's own measurements, this predicts the measured width with a median
// error of 0.02% (worst 4.7%, on the bold class titles).
//
// CJK falls back to a flat full-width 16px and anything else to a flat 9px,
// near the mean latin advance. Neither is measured.
func runeWidthPx(r rune) float64 {
	if r >= 0x20 && int(r) < 0x20+len(latinAdvance16) {
		if w := latinAdvance16[r-0x20]; w > 0 {
			return w
		}
	}
	if r > 0xFF {
		return 16
	}
	return 9
}

// latinAdvance16 holds U+0020..U+00FF advances in px at 16px "trebuchet ms";
// 0 marks a codepoint the font does not cover. See runeWidthPx.
var latinAdvance16 = [0xE0]float64{
	4.82, 5.875, 5.195, 8.391, 8.391, 9.602, 11.297, 2.555, // 0x20
	5.875, 5.875, 5.875, 8.391, 5.875, 5.875, 5.875, 8.391, // 0x28
	8.391, 8.391, 8.391, 8.391, 8.391, 8.391, 8.391, 8.391, // 0x30
	8.391, 8.391, 5.875, 5.875, 8.391, 8.391, 8.391, 5.875, // 0x38
	12.328, 9.438, 9.055, 9.57, 9.812, 8.57, 8.398, 10.82, // 0x40
	10.469, 4.453, 7.625, 9.211, 8.102, 11.352, 10.211, 10.781, // 0x48
	8.922, 10.812, 9.312, 7.695, 9.289, 10.375, 9.398, 13.633, // 0x50
	8.906, 9.125, 8.805, 5.875, 5.688, 5.875, 8.391, 8.391, // 0x58
	8.391, 8.406, 8.914, 7.922, 8.914, 8.727, 5.914, 8.031, // 0x60
	8.742, 4.562, 5.867, 8.07, 4.719, 13.281, 8.742, 8.586, // 0x68
	8.914, 8.914, 6.219, 6.477, 6.344, 8.742, 7.836, 11.906, // 0x70
	8.016, 7.891, 7.594, 5.875, 8.391, 5.875, 8.391, 0, // 0x78
	0, 0, 0, 0, 0, 0, 0, 0, // 0x80
	0, 0, 0, 0, 0, 0, 0, 0, // 0x88
	0, 0, 0, 0, 0, 0, 0, 0, // 0x90
	0, 0, 0, 0, 0, 0, 0, 0, // 0x98
	4.82, 5.875, 8.391, 8.391, 8.391, 9.125, 8.391, 7.258, // 0xa0
	8.391, 11.406, 5.875, 8.391, 8.391, 5.875, 11.406, 8.391, // 0xa8
	8.391, 8.391, 7.219, 7.258, 8.391, 8.742, 8.391, 5.875, // 0xb0
	8.391, 7.219, 5.875, 8.391, 13.031, 13.031, 13.031, 5.875, // 0xb8
	9.438, 9.438, 9.438, 9.438, 9.438, 9.438, 13.867, 9.57, // 0xc0
	8.57, 8.57, 8.57, 8.57, 4.453, 4.453, 4.453, 4.453, // 0xc8
	9.812, 10.211, 10.781, 10.781, 10.781, 10.781, 10.781, 8.391, // 0xd0
	10.508, 10.375, 10.375, 10.375, 10.375, 9.125, 8.891, 8.742, // 0xd8
	8.406, 8.406, 8.406, 8.406, 8.406, 8.406, 13.969, 7.922, // 0xe0
	8.727, 8.727, 8.727, 8.727, 4.562, 4.562, 4.562, 4.562, // 0xe8
	8.789, 8.742, 8.586, 8.586, 8.586, 8.586, 8.586, 8.391, // 0xf0
	8.727, 8.742, 8.742, 8.742, 8.742, 7.891, 8.852, 7.891, // 0xf8
}

// isCJK reports whether a rune may start a line without an intervening space,
// covering the CJK blocks mermaid labels use (kana, ideographs, fullwidth
// punctuation).
func isCJK(r rune) bool {
	return r > 0x2E80
}

// wrapParas re-breaks paragraphs so that no line exceeds width px, keeping the
// paragraphs it is given (a <br> break stays a break). It approximates the
// browser: CJK breaks between characters, latin text only at spaces.
//
// Callers use this only for labels mermaid let the browser wrap, where the SVG
// keeps the resulting line count and the wrap width but not the positions of
// the breaks. Those positions are estimated, so callers go through
// fitLineCount rather than calling this directly with the SVG's width.
func wrapParas(paras []Para, width float64) []Para {
	if width <= 0 {
		return paras
	}
	var out []Para
	for _, p := range paras {
		out = append(out, wrapPara(p, width)...)
	}
	return out
}

// piece is a run fragment: the text of one token, with the style of the run it
// came from. A token spans runs when a word is partly bold.
type piece struct {
	text   string
	bold   bool
	italic bool
}

// token is an unbreakable unit: one CJK rune, one latin word, or one run of
// spaces. Spaces are dropped when a break lands on them.
type token struct {
	pieces []piece
	w      float64
	space  bool
}

func wrapPara(p Para, width float64) []Para {
	var out []Para
	cur := Para{Align: p.Align}
	curW := 0.0
	for _, t := range tokenize(p) {
		if curW > 0 && curW+t.w > width {
			out = append(out, cur)
			cur = Para{Align: p.Align}
			curW = 0
			if t.space {
				continue
			}
		}
		if curW == 0 && t.space {
			continue
		}
		for _, pc := range t.pieces {
			cur.appendRun(pc)
		}
		curW += t.w
	}
	if !cur.empty() {
		out = append(out, cur)
	}
	if len(out) == 0 {
		return []Para{p}
	}
	return out
}

func (p *Para) appendRun(pc piece) {
	if n := len(p.Runs); n > 0 && p.Runs[n-1].Bold == pc.bold && p.Runs[n-1].Italic == pc.italic {
		p.Runs[n-1].Text += pc.text
		return
	}
	p.Runs = append(p.Runs, Run{Text: pc.text, Bold: pc.bold, Italic: pc.italic})
}

func tokenize(p Para) []token {
	var toks []token
	var cur token
	flush := func() {
		if len(cur.pieces) > 0 {
			toks = append(toks, cur)
		}
		cur = token{}
	}
	// a latin word or a space run continues across a run boundary, so the
	// token stays open and only gains a piece with the new style
	add := func(r rune, run Run) {
		if n := len(cur.pieces); n > 0 && cur.pieces[n-1].bold == run.Bold && cur.pieces[n-1].italic == run.Italic {
			cur.pieces[n-1].text += string(r)
		} else {
			cur.pieces = append(cur.pieces, piece{text: string(r), bold: run.Bold, italic: run.Italic})
		}
		cur.w += runeWidthPx(r)
	}
	for _, run := range p.Runs {
		for _, r := range run.Text {
			switch {
			case r == ' ':
				if !cur.space {
					flush()
					cur.space = true
				}
				add(r, run)
			case isCJK(r):
				flush()
				add(r, run)
				flush()
			default:
				if cur.space {
					flush()
				}
				add(r, run)
			}
		}
	}
	flush()
	return toks
}

// fitLineCount re-breaks paras onto want lines, correcting the wrap width
// until the line count matches. runeWidthPx follows the font mermaid laid the
// label out with, so wrapping at the SVG's own width usually lands on the
// browser's own line count — but the residue still matters: the label is
// emitted with wrapping disabled, so one line too few is text hanging outside
// the shape, with nothing left to re-break it.
//
// The line count is the one thing the SVG does record, so it corrects what the
// advances could not: a different font in the stack, a glyph outside the table,
// synthetic bold. Break positions stay approximate; the count does not.
func fitLineCount(paras []Para, width float64, want int) []Para {
	got := wrapParas(paras, width)
	if want <= 0 || len(got) == want {
		return got
	}
	// line count is non-increasing in width, so the widths yielding `want`
	// lines form one interval: bracket it and bisect for its lower edge
	lo, hi := 1.0, estTextWidth(paras)
	for range 40 {
		mid := (lo + hi) / 2
		if len(wrapParas(paras, mid)) > want {
			lo = mid
		} else {
			hi = mid
		}
	}
	fitted := wrapParas(paras, hi)
	if len(fitted) == want {
		return fitted
	}
	// `want` is unreachable: no width splits this text that way (a single
	// unbreakable token can force it). Keep whichever side lands closer, and
	// on a tie the narrower lines, which overflow less.
	alt := wrapParas(paras, lo)
	if absInt(len(alt)-want) < absInt(len(fitted)-want) {
		return alt
	}
	return fitted
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
