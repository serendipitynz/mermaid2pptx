# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository.
日本語版: [AGENTS.ja.md](AGENTS.ja.md).

## What this project is

`mermaid2pptx` converts a Mermaid-rendered SVG (or a `.mmd` file, via
`mermaid-cli`) into a PowerPoint `.pptx` made of **native, editable objects** —
preset shapes, connectors bound to those shapes, and text boxes. It is not an
image exporter. The goal is that a user can open the result in PowerPoint and
move/edit every element.

Read `README.md` for user-facing behavior before changing conversion output.

## Build, test, verify

```sh
go build -o mermaid2pptx ./cmd/mermaid2pptx   # build
go test ./...                                 # unit + package tests
go vet ./... && gofmt -l .                    # must be clean (gofmt prints nothing)
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/mermaid2pptx  # cross-compile check
```

Always run `gofmt -l .`, `go vet ./...`, and `go test ./...` before finishing a
change. If a change affects the generated `.pptx`, also verify the output (see
below) — a well-formed XML test passing does not prove the slide looks right.

### Verifying generated output

Tests already assert: element counts per sample, node-shape kinds, connector
endpoint geometry, and that every packaged XML part is well-formed and no shape
lands off-slide. When you change the generator, re-run the samples and inspect
with `python-pptx` (available in the environment):

```sh
./mermaid2pptx -f sample/graph*.svg
python3 -c "from pptx import Presentation; p=Presentation('sample/graph1.pptx'); print(len(p.slides[0].shapes))"
```

The strongest check we have is `TestConnectorEndpointsMatchSVG`, which
reconstructs every edge connector's endpoints from the generated XML (undoing
the flip/rotation) and asserts they match the SVG `data-points` transformed
through the same px->EMU fit, within ~1 EMU. `fitTransform` in `slide.go` is
the single source of truth for that transform, shared by the generator and the
test. If you touch geometry, this test is the guard — keep it green.

## Architecture

Pipeline: **parse SVG → `Diagram` model → emit `slide1.xml` (DrawingML) → zip
into a `.pptx`**. The CLI only wires files together and, for `.mmd`, shells out
to `mmdc` to produce a temporary SVG first.

- `cmd/mermaid2pptx/main.go` — flags, `.mmd`→SVG rendering (`mmdc`), file I/O.
- `internal/convert/model.go` — the intermediate types: `Diagram`, `Node`,
  `Cluster`, `Edge`, `EdgeLabel`, `Line`, `TextBox`, plus `ShapeKind`. Also the
  Mermaid default theme colors.
- `internal/convert/svg.go` — the shared parser. `ParseMermaidSVG` detects the
  diagram type from the root `aria-roledescription` and dispatches.
  `parseGraphDiagram` handles the dagre-based types (flowchart, state, class,
  er) that share the `nodes / edgePaths / edgeLabels / clusters` DOM.
- `internal/convert/compartment.go` — class/ER boxes (title/member/attribute
  compartments) decomposed into a box + text boxes + divider lines.
- `internal/convert/sequence.go` — the sequence diagram parser (flat layout of
  rects, lines, and text; text is attached to the smallest enclosing rect).
- `internal/convert/geom.go` — connector placement math: which side an edge
  leaves/enters, and the flip/rotate transform to fit a preset connector.
- `internal/convert/slide.go` — DrawingML emission (`GenerateSlideXML`) and the
  px→EMU transform. `Options` holds font + margin.
- `internal/convert/skeleton.go` — the static parts of the `.pptx` package
  (everything except `slide1.xml`) and the zip writer.

### Coordinate systems

- SVG pixels, y-down. The viewBox origin (`MinX`/`MinY`) can be non-zero
  (sequence diagrams) — always subtract it.
- EMU for OOXML: `9525 EMU = 1 px` at 96 dpi; slide is 12192000 x 6858000 EMU.
- Composite states and other nested content live inside a translated
  `<g class="root">`; the parser accumulates ancestor `translate()` while
  walking, and each `parseX` takes `dx, dy` offsets. Do not assume a node's
  transform is absolute.

## Conventions

- **Standard library only.** No production dependencies. Do not add a module
  require without asking the user first. (`python-pptx` and `mermaid-cli` are
  external *dev/verify* tools, not Go deps.)
- Code comments in English. Keep them at the density of the surrounding code
  and explain *why*, not *what*.
- Prefer small, focused functions that append to the `Diagram` model; keep XML
  string-building confined to `slide.go` / `skeleton.go`.
- Emitted XML is built with `fmt.Fprintf` into a `strings.Builder`; always run
  user text through `xmlEsc`.

## Gotchas (learned the hard way)

- **Mermaid's DOM changes between versions.** Samples were rendered with
  mermaid-cli 11.15. Shapes are detected structurally, not by a fixed path —
  e.g. stadium nodes are a `<g>`-wrapped Bézier `<path>`, not a rect; the
  diamond `<polygon>` carries its own `translate()` on top of the node's.
  When output breaks after a mermaid upgrade, re-inspect the SVG structure
  before "fixing" the code.
- **Some line ends have no OOXML equivalent** (hollow inheritance triangle,
  crow's foot). They are deliberately approximated in `arrowType`.
- **Preset connectors can't express every route.** U-turns and multi-bend
  paths fall back to a freeform polyline; endpoints stay exact but the curve
  differs from Mermaid.
- Edge endpoints are resolved first from the edge id (`L_from_to_n` /
  `id_from_to_n`), then by nearest-boundary geometry — node ids may contain
  underscores, so id splitting is validated against the known-id set.
- **Nothing in the pipeline measures the output font** (no font metrics:
  standard library only). Widths come from Mermaid's browser measurements,
  taken with the browser's own font stack, except in `sequence.go`, where
  `estTextWidth` estimates them per character because the SVG has no measured
  box to read. `labelWidthSafety` in `slide.go` compensates for the default
  font's extra width; a `-font` far wider than that (a monospace face) will
  overflow its box, in node labels as much as in edge labels. Widening the
  constant to cover any font would make the default output permanently loose,
  so it is tuned for the default.

## Adding a new diagram type

1. Render a sample with `mmdc` and inspect the SVG structure (root
   `aria-roledescription`, node/edge classes, whether it uses `data-points`).
2. If it is dagre-based (shares `nodes`/`edgePaths`), extend `parseGraphDiagram`
   and reuse the existing node/edge parsing. Otherwise write a dedicated parser
   like `sequence.go`.
3. Map its shapes to `ShapeKind` / `Line` / `TextBox` in the model; add
   rendering in `slide.go` only if a new geometry is needed.
4. Add a `sample/graphN.mmd`, render it, and add a `TestParseGraphN` plus the
   file to `TestGeneratePackage`.

## Repository notes

- `_sandbox/` holds internal material and is git-ignored — never commit it or
  copy its contents into tracked files or generated output.
- `sample/*.svg` are test fixtures (referenced by the tests); `sample/*.mmd`
  are their sources; `sample/*.pptx` are demonstration outputs. Keep them in
  sync when you change the generator.
- Do not include customer names, account numbers, or other PII in code,
  samples, or generated artifacts.

<!-- BACKLOG.MD GUIDELINES START -->
<!-- backlog.md-instructions-version: 1.50.1 -->
<CRITICAL_INSTRUCTION>

## Backlog.md Workflow

This project uses Backlog.md for task and project management.

Use `backlog <command> --help` before running unfamiliar commands. Help shows options, fields, and examples.

Do not edit Backlog task, draft, document, decision, or milestone markdown files directly. Use the `backlog` CLI so metadata, relationships, and history stay consistent.

</CRITICAL_INSTRUCTION>
<!-- BACKLOG.MD GUIDELINES END -->
