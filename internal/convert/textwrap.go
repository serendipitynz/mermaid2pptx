package convert

// runeWidthPx approximates a rune's advance at the default 16px font. Nothing
// in the pipeline measures the output font, so this is the one width model the
// package uses where mermaid left no measurement to read.
func runeWidthPx(r rune) float64 {
	if r > 0xFF {
		return 16
	}
	return 9
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
// the breaks — so these positions are estimated, and the caller leaves
// PowerPoint's own wrapping on as a fallback.
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
