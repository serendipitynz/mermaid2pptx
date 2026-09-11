---
id: TASK-3
title: >-
  Resolve a node's font size from mermaid's stylesheet rules, not only inline
  styles
status: To Do
assignee: []
created_date: '2026-09-11 11:31'
labels: []
milestone: m-4
dependencies: []
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
TASK-2 made the generator read each label's line count from its foreignObject height, which only means anything against the font size the label was laid out at. That size is resolved today by `docFontSize` (the rule mermaid writes for the diagram root) and `labelFontSize` (an inline style on the label itself), in `internal/convert/svg.go` — covering the default case and a classDef whose font-size reaches the label inline.

A classDef mermaid emits as a stylesheet rule instead is not resolved: the rule has to be matched against the node's own classes. Such a label's lines are taller than the document size implies, so its foreignObject height reads as more lines than mermaid drew — a two-line 20px label is 60px, which against 16px reads as three — and since labels are emitted with wrapping disabled, the deck then shows a line break mermaid never drew. Reaching the failure needs a node that is both custom-sized and long enough for the browser to wrap it, because the line count is only consulted for browser-wrapped labels.

This was left out of TASK-2 deliberately, not overlooked (PR #4, external review rounds 3-5, where the reviewer agreed to hold it open on this condition). mermaid-cli was not installed in the environment that work was done in, so no fixture could be rendered from a classDef diagram, and AGENTS.md requires shapes to be detected against a real fixture because mermaid's DOM changes between versions. A cascade resolver written against a guessed DOM is the failure mode that guidance exists to prevent.

So the first step is to render a sample with mmdc and inspect what mermaid actually emits for classDef font sizing — whether it lands on the node group, the label div, or only as a class rule. The code change follows from that, and may turn out smaller or differently shaped than assumed here.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A sample rendered with mermaid-cli from a diagram whose classDef sets a font-size is added under sample/, and the DOM mermaid uses to carry that size is recorded
- [ ] #2 A node whose font size comes from a stylesheet rule has its label line count read against that size rather than the document's
- [ ] #3 A test asserts the line count for such a node and fails against the current document-and-inline-only resolution
- [ ] #4 Relative units (em, rem, %) either stay unresolved and fall back, as cssLengthPx does today, or their resolution is justified separately
- [ ] #5 gofmt -l ., go vet ./..., go test ./... all clean, and any regenerated samples are committed with the change
<!-- AC:END -->
