---
id: TASK-2
title: Stop PowerPoint from re-wrapping node labels Mermaid laid out on one line
status: To Do
assignee: []
created_date: '2026-09-08 21:29'
labels: []
milestone: m-3
dependencies: []
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
PowerPoint re-breaks node labels that Mermaid rendered on a single line, so the deck does not match the diagram it came from. Rendering the sample decks through PowerPoint itself shows three such labels:

- graph1 "エラー" — ellipse, node box 74.5px, Mermaid text width 47.5px, rendered as "エ" / "ラー"
- graph1 "入力チェック" — diamond, node box 178.8px, text 96px, rendered as "入力" / "チェック"
- graph2 "クライアント" — ellipse, node box 91.2px, text 96px, rendered on two lines

Mermaid's own line count is readable from each foreignObject's height in sample/*.svg (24px per line), so the expected result is checkable against the fixtures: all three are one line there, while "後処理バッチ" is two lines and is already reproduced correctly.

Cause: node labels call writeTxBody without noWrap, so bodyPr carries PowerPoint's default wrap="square". PowerPoint derives a non-rectangular preset's text area from an inscribed region much narrower than the bounding box — a diamond's is roughly half the box in each dimension — so a label that Mermaid measured against the full node box no longer fits and gets re-broken. The text insets are not to blame: they are already emitted as 18288 EMU (~2px), not PowerPoint's 91440 default. For "クライアント" even the full box (91.2px) is narrower than the text (96px); Mermaid lets a circle's label overflow the circle, whereas PowerPoint wraps it instead.

The mechanism to fix this already exists: txBody.noWrap emits wrap="none" and is used for edge labels and free text boxes, just not for node labels.

The catch is that turning wrap off everywhere would regress labels Mermaid itself wrapped. Long labels currently arrive as a single paragraph and rely on PowerPoint's wrapping to break them — no node label in the graph4 output has more than one paragraph today. So either Mermaid's own line breaks have to be reproduced as separate paragraphs before wrapping is disabled, or noWrap has to be applied only where Mermaid used a single line. The first is the more faithful of the two: Mermaid has already decided where the breaks go, and the generator's job is to carry that decision across rather than re-derive it.

Related: the note in AGENTS.md that nothing in the pipeline measures the output font. That warning is about label width and labelWidthSafety; this defect is a different mechanism (PowerPoint's own text area and wrap default), so it should be fixable without retuning that constant.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Every node label that Mermaid rendered on one line renders on one line in PowerPoint, across all samples
- [ ] #2 Labels Mermaid itself wrapped keep Mermaid's line breaks; graph4's long descriptive labels do not regress into a single overflowing line
- [ ] #3 A test asserts each node label's paragraph/line structure matches the line count implied by the source SVG's foreignObject height
- [ ] #4 labelWidthSafety is left unchanged, or any change to it is justified separately
- [ ] #5 gofmt -l ., go vet ./..., go test ./... all clean, and the samples are regenerated
<!-- AC:END -->
