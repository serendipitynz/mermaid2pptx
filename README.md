# mermaid2pptx

*Read this in [日本語](README.ja.md).*

A CLI tool that converts Mermaid diagrams into `.pptx` files made of
**editable PowerPoint objects** (shapes, connectors, and text) — not a
flattened image. (This is not an official mermaid-js project.)

Supported diagrams: **flowchart / stateDiagram-v2 / classDiagram / erDiagram /
sequenceDiagram** (auto-detected from the SVG root's `aria-roledescription`).

Implemented with the Go standard library only, so it cross-compiles to a
single binary for macOS / Windows. Input is either the SVG that Mermaid emits
(no dependencies) or a `.mmd` file (rendered to SVG by calling the
`mermaid-cli` found on `PATH`).

## Layout

```
cmd/mermaid2pptx/ CLI entry point
internal/convert/ SVG parser, DrawingML generation, PPTX packaging
sample/           samples (.mmd / .svg rendered by mermaid-cli /
                  .pptx produced by this tool)
```

Sample coverage:

- graph1 — flowchart LR; full node-shape coverage (stadium, parallelogram,
  diamond, subroutine, circle, cylinder, hexagon); reverse edges; thick edge
- graph2 — flowchart with nested subgraphs; dotted edges; DB cylinder
- graph3 — flowchart with `classDef` colors; cyclic edges; edge labels
- graph4 — flowchart `%%{init}%%` (curve: basis / wrappingWidth); long-label
  auto-wrapping; a diamond gate standing between two clusters
- graph5 — stateDiagram-v2 (start/end states, composite state, transition labels)
- graph6 — sequenceDiagram (actors, lifelines, activation, note, dashed replies)
- graph7 — classDiagram (interface, realization, aggregation)
- graph8 — erDiagram (entity attribute tables, cardinality)

## Install

```sh
go install github.com/serendipitynz/mermaid2pptx/cmd/mermaid2pptx@latest
```

## Build

```sh
go build -o mermaid2pptx ./cmd/mermaid2pptx

# For Windows
GOOS=windows GOARCH=amd64 go build -o mermaid2pptx.exe ./cmd/mermaid2pptx
```

## Usage

```sh
./mermaid2pptx input.svg              # write input.pptx (won't overwrite an existing file)
./mermaid2pptx input.mmd              # convert straight from .mmd via mmdc
./mermaid2pptx -f input.svg           # allow overwriting
./mermaid2pptx -o out.pptx input.svg  # set the output path
./mermaid2pptx a.svg b.mmd            # convert several at once (mixed input is fine)
./mermaid2pptx -font "Meiryo" a.svg   # set the font (default: Noto Sans JP)
./mermaid2pptx -margin 0.5 a.svg      # slide margin in inches (default: 0.3)
./mermaid2pptx -mmdc /path/to/mmdc a.mmd  # point at a specific mermaid-cli
./mermaid2pptx -version               # print version information and exit

# Regenerate the samples
./mermaid2pptx -f sample/graph*.svg
```

Options may appear before, after, or between the input files
(`./mermaid2pptx a.svg -o out.pptx -f` works too).

`.mmd` input requires [mermaid-cli](https://github.com/mermaid-js/mermaid-cli)
(`npm install -g @mermaid-js/mermaid-cli`). SVG input has no external
dependencies.

## Rendering the sample SVGs

`sample/*.svg` is produced with mermaid-cli:

```sh
mmdc -i sample/graph1.mmd -o sample/graph1.svg -I my-svg -b white
```

## Conversion rules

Common:

- Slides are 16:9 (13.33 x 7.5 in). The whole diagram is fit into the margins
  with its aspect ratio preserved, and the font is scaled by the same factor
  (small diagrams are enlarged up to 1.25x).
- Fill, stroke, and text colors are carried over from the SVG styles verbatim.
- `<br/>` becomes a paragraph break; `<b>` / `<i>` become bold / italic.

flowchart / stateDiagram:

- Nodes map to PowerPoint preset shapes:
  - rectangle `[ ]` / subroutine → `roundRect`
  - diamond `{ }` → `diamond`
  - cylinder `[( )]` → `can`
  - circle `(( ))` / start & end states → `ellipse`
  - stadium `([ ])` → `roundRect` (fully rounded corners)
  - hexagon `{{ }}` → `hexagon`
  - other polygons (parallelogram `[/ /]`, etc.) → custom geometry (vertices
    emitted as-is)
- Edges become PowerPoint connectors (`curvedConnector2/3` /
  `straightConnector1`). The endpoints are bound to the nodes (composite
  states to the container) via `stCxn` / `endCxn`, so moving a shape drags its
  connectors along. Routes that no preset can express fall back to a freeform
  polyline.
- Edge labels → rounded rectangles with no border and a translucent background.
- Subgraphs / composite states → rounded rectangles with a top-anchored title.
- Dotted edges (`-.->`) become dashed; thick edges (`==>`) become thick lines.

classDiagram / erDiagram:

- A class / entity becomes one outer rectangle + independent text boxes
  (title, members, attribute rows) placed at the same positions as in the SVG
  + divider lines.
- Relationships become connectors (`stCxn` / `endCxn` bound). Arrowheads are
  approximated with OOXML line ends: inheritance/realization `<|--` → filled
  triangle, composition/aggregation `*--` / `o--` → diamond, dependency → open
  arrow, crow's foot `}o` / `}|` → open arrow, `|o` → oval.

sequenceDiagram:

- Actor boxes / activations / notes → rectangles (with their text pulled in).
- Lifelines → thin lines; messages → arrowed straight lines (dashed replies
  supported).
- Message labels → borderless text boxes (width estimated from the font).

## Limitations

- Assumes Mermaid `htmlLabels` (foreignObject labels); sequence uses SVG text.
- Edge curves are approximated by preset connectors and do not exactly match
  Mermaid's Bézier curves (the endpoints do match).
- Line ends with no OOXML equivalent (hollow inheritance triangle, crow's
  foot) are approximated.
- In class / ER diagrams the compartment text is separate from its box, so
  moving the box leaves the text behind (grouping is a future task).
- Sequence loop / alt frames go through the generic conversion (lines + text);
  there is no dedicated handling for them.
- Text is laid out at the sizes Mermaid measured in the browser, with its own
  font stack. Shapes and label backgrounds are sized from those measurements,
  so a `-font` whose metrics are much wider than the browser's (a monospace
  face, say) can push text past the box that holds it. The default font is
  what the sizing is tuned for.

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for
the development workflow and the checks CI runs.

## License

Licensed under the [Apache License, Version 2.0](LICENSE). See also [NOTICE](NOTICE)
and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) (the `sample/*.svg` fixtures
embed markup generated by Mermaid, which is MIT-licensed).

"Mermaid" belongs to the mermaid-js project and "PowerPoint" / `.pptx` to
Microsoft; this project is not affiliated with either and only interoperates
with those tools and formats.
