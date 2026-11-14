# diagram — design

A Go library for drawing system diagrams as code. Declarative API, simple
composable styling, bundled and importable icons, output to SVG, PNG and
draw.io. No dependencies beyond the standard library and `golang.org/x`.

This document records the decisions behind the library. Change it when a
decision changes.

## Scope

Requires Go 1.26 (the minimum for golang.org/x/image v0.48).


In v1:

- Library only (`github.com/grindlemire/diagram`). A CLI may come later.
- Manual layout through nested, typed containers. No automatic layout.
- Orthogonal edge routing with obstacle avoidance, pinned ports and waypoints.
- SVG, PNG, interactive HTML and editable draw.io output.

Not in v1: automatic layout (planned as an `Auto` container kind), the CLI,
edge junctions and bundling, custom shape paths, complex text shaping
(Arabic, Indic, ligatures), cloud-vendor icon sets, mapping to draw.io's
native stencils, curved edges.

Versioning stays at `v0.x` until real use has settled the API.

## Example

```go
d := diagram.New("Payments", diagram.Theme(themes.Light))

web := d.Node("Web", diagram.Icon(generic.Browser))
vpc := d.Column("VPC")
api := vpc.Node("API", diagram.Icon(generic.Server), diagram.Desc("Go · 3 replicas"))
workers := vpc.Grid("Workers", 2)
w1 := workers.Node("Worker 1", 0, 0)
w2 := workers.Node("Worker 2", 0, 1)
db := d.Node("Postgres", diagram.Icon(generic.Database), diagram.Cylinder)

critical := diagram.Style(diagram.Stroke(diagram.Red), diagram.StrokeWidth(2))

d.Connect(web, api, diagram.Label("HTTPS"))
d.Connect(api, w1)
d.Connect(api, w2)
d.Connect(w1, db, critical)
d.Connect(w2, db, critical, diagram.Exit(diagram.Bottom))

if err := d.WriteFile("payments.svg"); err != nil {
	// payments.go:9: node "Worker 2": cell (0,1) already occupied by "Worker 1" (payments.go:8)
}
```

## API

### Containers are typed

The container kind is part of the Go type, so putting a child in the wrong
kind of container fails to compile.

| Type | Child constructors | Children placed by |
|---|---|---|
| `*Diagram` | same as its root (a `Row`, or a `Column` with `diagram.Vertical`) | flow |
| `*Row`, `*Column` | `Node(label, opts...)`, `Row/Column(label, opts...)`, `Grid(label, cols, opts...)`, `Canvas(label, opts...)` | flow, in the order declared |
| `*Grid` | `Node(label, row, col, opts...)`, `Row/Column/Grid/Canvas(label, row, col, ...)` | cell, with optional `Span(rows, cols)`; rows grow as needed |
| `*Canvas` | `Node(label, x, y, opts...)`, `Row/Column/Grid/Canvas(label, x, y, ...)` | absolute coordinates (of the node's drawn box) relative to the canvas origin; the canvas sizes itself to the bounding box of its children plus padding. Overlapping children are an error unless the canvas has `AllowOverlap` |

In a `Row` or `Column`, `Justify` places children along the main axis when
the container is larger than they need: `JustifyStart` (default),
`JustifyCenter`, `JustifyEnd`, `JustifySpaceBetween`. `AlignStart`,
`AlignCenter` (default) and `AlignEnd` place them across it. Nodes line up by
their drawn box, so reserved space around a node (for self-loops) doesn't
push it out of line.

A container with an empty label is invisible: it only does layout. A labelled
container draws a box with a header (label, optional icon and description) at
its top left, or centred with `HeaderTop`. Themes shade nested groups by depth.
Empty groups are allowed.

### Errors are collected

Builder calls return plain handles, never errors. Misuse (occupied cell,
duplicate `ID`, bad numbers, an option that doesn't apply where it's used,
an edge between two diagrams) is recorded with the file:line of the call,
taken with `runtime.Caller`. `Validate()` and every render call return all
of them joined with `errors.Join`, together with errors found by layout
(overlapping canvas children), by the glyph check (text no font can draw)
and, when rendering, by routing (an impossible route). `Validate` runs
layout but not routing.

### Identity

Elements are referenced by Go handles. Each one gets a deterministic ID from
its path in the tree (`vpc/workers/worker-1`), which is used for draw.io cell
IDs and SVG `id` attributes. `ID("w1")` overrides it.

### Options are typed

- `NodeOption`, `GroupOption`, `EdgeOption` and `DiagramOption` are separate
  interfaces.
- Style options (`Fill`, `Stroke`, `StrokeWidth`, `Dashed`, `Dotted`, `Radius`,
  `TextColor`, `FontSize`, `Bold`) implement all three element interfaces.
- `Style(...)` bundles style options into one reusable value. A bundle is
  itself a style option. Options apply in order and later ones win. Fields
  that are never set stay unset; there is no zero-value ambiguity.
- No implicit inheritance. A group's style affects only its own box.
  `Defaults(...)` on a group explicitly applies styles to the nodes created
  in it afterwards.

### Named styles, legends and marks

- `Named(name, opts...)` is a style bundle whose name is recorded on each
  element it's used on (`model.Style.Names`, with a per-diagram registry that
  rejects two different bundles under one name). SVG and HTML add the class
  `style-<slug>` only when it's used.
- `Legend(title)` is a container (on the diagram and every container type)
  whose entries are generated at layout time, without mutating the model:
  one per named style in first-use order, after any children the user added.
  Edge styles get a line sample, node and group styles a swatch. An empty
  legend takes no space. It can't be an edge endpoint.
- `Badge(text)` pills straddle a node's top-right corner (stacking leftwards)
  and `Status(c)` puts a dot on the top-left, both anchored to the shape's
  outline and ringed in the background colour. They don't change layout; the
  router treats them like group headers (solid, hard obstacles for labels) and
  keeps ports off them when enough of the side remains. draw.io gets them as
  child cells.

### Colours

`Color` is either a theme token (`Accent`, `Foreground`, `Muted`,
`Background`, `Surface`, `Border`, `Red`, `Orange`, `Yellow`, `Green`,
`Teal`, `Blue`, `Purple`, `Pink`, `Gray`) or a literal (`Hex("#ff6600")`,
`RGB(r, g, b)`). Tokens resolve against the active theme, so changing the
theme recolours the diagram.

### Nodes

- Anatomy: label (wraps at the theme's `MaxWidth`, `\n` forces a break),
  optional `Desc`, optional `Icon` with `IconTop` (the default when there is an
  icon), `IconLeft` or `IconOnly`, and `IconSize(n)`.
- Shapes (each has a native draw.io equivalent): `Box` (rounded, default),
  `Rect`, `Pill`, `Circle`, `Cylinder`, `Queue`, `Document`, `Person`,
  `Note`, `Text`.
- Size fits the content by default. `Size(w, h)` fixes it. `Uniform` on a
  container gives every direct child node the same slot; boxes and pills
  grow to fill it, while Circle, Person and IconOnly keep their proportions
  and are centred in it.
- Monochrome icons use `IconColor(c)`, or the theme's icon colour. Style
  options don't recolour icons.
- On a literal (non-token) fill, unless `TextColor`/`IconColor` say
  otherwise, the label is dark (#0f172a) or white, whichever contrasts more
  with the fill; the description is a muted mix that stays readable, and an
  icon colour under 3:1 contrast falls back to the label colour. These
  colours are literals, so they read the same in every theme and in both
  `Auto` palettes. Token fills keep theme colours.

### Edges

- `d.Connect(from, to, opts...)`. Either endpoint can be a node or a group.
- Orthogonal routing by default, `Straight` per edge.
- `Exit(side)` and `Enter(side)` pin ports; otherwise ports are chosen from
  the endpoints' relative positions. Edges that share a side spread along it.
- `Via(points...)` adds waypoints in diagram coordinates.
- `Label(s)` is placed on a pill at the middle of the longest segment by
  default, or with `LabelAt(Start|Middle|End)`. Labels are obstacles for
  other edges. Placement tries, in order: on the line; beside it and
  touching it; on or touching it while spilling over the borders of the
  edge's own two end nodes (never their text or icons); and only then away
  from the line. A label never covers another node or any text.
- Arrowheads: `Arrow` (target end, default), `ArrowBoth`, `NoArrow`,
  `Head(kind)`, `Tail(kind)` with `Triangle`, `Open`, `Diamond`, `Dot`.
- Parallel edges between the same endpoints are offset.
- `d.Path(...)`, `d.Fan(from, to...)` and `d.FanIn(to, from...)` return an
  `*Edges` set; `.With(opts...)` applies options to all of them, recording
  errors at the `With` call.
- Edges never pass through groups that hold neither endpoint, unless the
  edge has `CrossGroups`. Routes may leave the layout area as a last resort;
  the canvas grows to contain them.
- Self-loops get reserved space: layout widens a looped node's extent on its
  `LoopSide` (the first loop's pinned `Exit`/`Enter`, otherwise a side facing
  away from neighbours), and the router nests the loops and their labels in
  that band.
- Ports on Person and IconOnly nodes' bottom sides attach below the label,
  so edges never cross it.
- Spacing is automatic, and a user `Gap` is a minimum:
  - **Floor (before routing):** in every Row, Column and Grid whose subtree
    holds an edge endpoint, each gap is at least `2×EdgeClearance +
    ParallelOffset` (38px in Light); where a labelled edge connects
    neighbouring children, the gap also fits that label on the edge's line.
  - **Widening (after a first routing sketch):** gaps whose edges need more
    than `2×EdgeClearance + n×1.5×ParallelOffset` plus any label placed in
    them are widened once; the better-scoring sketch (internal/quality) is
    routed to completion. Diagrams that fit pay nothing extra.
  - Both only add space and never mutate the user's model.
    `ExactSpacing` turns both off, so gaps are exactly as written.
- Routing quality is tracked by `examples/quality_test.go` against
  `examples/testdata/quality.json` (bends, crossings, length ratio, segments
  through nodes or headers, label overlaps, shared line length, length
  outside the layout); regressions fail the test, `-update-quality`
  re-records.

### Output

```go
d.RenderSVG(w, opts...)
d.RenderPNG(w, opts...)       // Scale(2) by default, Transparent
d.RenderDrawio(w, opts...)
diagram.WriteDrawio(w, d1, d2) // one page per diagram
d.WriteFile("x.svg")           // picks the format from .svg/.png/.drawio
```

HTML output (`RenderHTML`, `.html`) is the SVG rendered in an internal
interactive mode, wrapped in a page with inline CSS and JS and no network
requests. Interactive mode adds data attributes from `scene.Group.Meta`
(`data-kind`, `data-id`, `data-parent`, `data-from`/`data-to` on edges,
`data-for` on edge labels, tooltip and link), focusable nodes with ARIA
labels, colours as CSS variables, and empty hidden groups for invisible
containers so every endpoint and parent ID resolves. The page offers pan and
zoom, connection highlighting, click-to-focus, tooltips (Cmd/Ctrl-click or the
tooltip's Open link follows `Link`), keyboard navigation, search, and a
light/dark toggle that starts from the viewer's system setting and returns
to following it when toggled back: `themes.Auto` uses its own dark palette,
`themes.Light` is paired with `themes.Dark`, and other themes get no switch.
The page script is a shared core (`view.js`) with two hosts: `page.js` for
the full page and `element.js` for `<diagram-view>` embeds. Deep links use
`#<id>` and `#from=<id>&to=<id>` (embeds prefix `<name>:`). Path tracing
takes the shortest path by hops along edge direction, then ignoring direction;
an edge to or from a group connects to the group and everything inside it.
Tooltips are placed at the candidate position that covers the fewest edge
labels and highlighted elements. `window.diagramView` exposes `get()`/`set()`
of the pan and zoom, and the page restores its view after a reload from
sessionStorage (`diagram-view:<title>:<viewBox>`), which `diagram watch`
relies on.

Embeds put the diagram in a shadow root, so IDs and CSS can't clash with the
host page or other embeds. Chrome ignores `@font-face` inside shadow roots, so
the script registers each embed's font subsets in the document under names
hashed from their bytes. The embed currently extracts the font faces and SVG
styles from the SVG renderer's output text; its tests fail if that format
changes.

Standalone SVG is unchanged except that `Tooltip` adds a `<title>` and `Link`
wraps the element in `<a>`; draw.io wraps such cells in `<UserObject>` with
`tooltip`/`link` attributes.

Render options: `Margin(n)`, `WithTheme(t)`, `Scale(n)` (PNG),
`Transparent` (PNG), `EmbedFont` (default) / `FontByName` (SVG).

`themes.Auto(light, dark)` makes SVG output follow `prefers-color-scheme`:
token colours become CSS classes whose values switch under a media query
(classes are more widely supported than CSS variables in SVG viewers).
Without `Auto`, colours are plain attributes. PNG and draw.io use the light
palette.

## Text

Inter Regular and SemiBold (OFL) are embedded. Measuring, wrapping and PNG
glyph outlines use `golang.org/x/image/font/sfnt`. SVG embeds a subset of the
font as an `@font-face` data URI by default. draw.io references Inter through
`fontSource`, with a little extra padding. v1 supports scripts that need no
complex shaping.

`RegisterFont(ttf)` adds fallback fonts (TrueType only; CFF `.otf`,
collections, WOFF and colour-emoji fonts are rejected with a clear error).
They form a global chain after Inter, in registration order; each rune uses
the first font with a glyph, and line metrics stay Inter's. Fonts sharing a
family name form one family, and each weight picks the closest
`usWeightClass`. SVG embeds a subset of every font that drew text, as
families `Inter`, `diagram-fallback-N`, listed in the font stack. PNG draws
fallback outlines directly. draw.io can't embed fonts, so fallback text uses
the viewer's system font. A rune no font can draw is a collected error
naming the element.

## Icons

- `icons.FromSVG(r)` accepts any well-formed SVG. `icons.FromPNG(r)` accepts
  PNGs. Icons can be placed at any size.
- SVG and draw.io output pass SVG icons through unchanged. `currentColor` is
  resolved to the node's icon colour (substituted into the bytes for draw.io).
- PNG output rasterizes a defined SVG subset: path, rect, circle, ellipse,
  line, polyline, polygon, g, use/defs, transforms, fill rules, opacity,
  stroke properties, linear and radial gradients. Anything outside it is an
  error at PNG render time naming the icon and the feature. `icon.CheckPNG()`
  validates early.
- PNG icons are resampled with `x/image/draw` (Catmull-Rom).
- Bundled sets live in their own subpackages (`icons/generic`,
  `icons/network`, `icons/brands`), generated as Go code by `cmd/gen-icons`
  (a separate module so its dependencies stay out of users' `go.mod`).
  Sources: Lucide (ISC) for generic and network icons, Simple Icons (CC0) for
  brands.

## draw.io

Editable and semantic, not just pixel-identical:

- Groups become container cells (`container=1`) with children parented to
  them.
- Nodes become vertices with styles converted to draw.io style strings.
  Icons become `shape=image` with data URIs.
- Edges are attached to source and target cells, use
  `edgeStyle=orthogonalEdgeStyle` with our waypoints, and carry
  `exitX/exitY/entryX/entryY` for pinned ports.
- XML is uncompressed so it diffs well. Small visual differences from the SVG
  are accepted.
- Ports are always written as `exitX/exitY/entryX/entryY`, pinned or not,
  so draw.io draws exactly the route we computed.
- `Person` maps to draw.io's `shape=actor` (head and shoulders), which
  matches our outline better than the UML stick figure.

## Architecture

```
public API (package diagram)
   │ builds
   ▼
internal/model   — element tree + edges + collected errors (no geometry)
   │
   ▼ internal/layout (uses internal/text)
laid-out model   — a box for every element, inner text/icon positions
   │
   ▼ internal/route
routed edges     — polylines, ports, label boxes
   ├──────────────► internal/render/drawio
   ▼ internal/scene
scene            — rects, paths, text runs, images; colours stay tokens
   ├──► internal/render/svg   (CSS classes for tokens under Auto)
   │      └──► internal/render/html  (svg in interactive mode + inline JS/CSS)
   └──► internal/render/png   (x/image/vector; internal/svgicon for icons)
```

Packages:

```
diagram/                  public API
  themes/                 Color, Theme, Light, Dark, Blueprint, Auto
  icons/                  Icon, FromSVG, FromPNG, CheckPNG
  icons/{generic,network,brands}/
  internal/model/
  internal/text/          sfnt measurement, wrapping, embedded Inter
  internal/layout/
  internal/route/
  internal/scene/
  internal/svgicon/       SVG subset → flat shapes (own path types: scene
                          imports icons, which imports svgicon)
  internal/render/{svg,png,html,drawio}/
  cmd/gen-icons/          separate module
```

## Determinism rules

Output must be byte-identical across runs and machines (PNG excepted):

- Never iterate over a map while producing output.
- Format every float with `strconv.FormatFloat(v, 'f', -1, 64)` after
  rounding to 2 decimals (`internal/num.Fmt`). Never `%v`.
- Children keep declaration order.

## Tooling

- `cmd/diagram` discovers exported `func() *diagram.Diagram` functions with
  go/parser (no registration), generates a runner in `<modroot>/.diagram/`, and
  runs it with `go run` from the package directory. `render` writes files;
  `watch` polls the package's files (stdlib only), re-runs on change, serves an
  index plus each diagram's HTML page, and live-reloads them over
  Server-Sent Events, showing compile and validation errors in the page.
- `diagramtest.Golden(t, d)` compares rendered output with
  `testdata/<name>.<ext>`, writes `.got` files on mismatch, and updates with
  `-diagram.update` or `DIAGRAM_UPDATE=1`.

## Testing

1. Geometry tests on layout and routing: no overlaps, children inside parent
   padding, routes avoid obstacles, ports respected. Compare at 0.01
   precision.
2. Byte-exact golden files for SVG and draw.io in `testdata/`, regenerated
   with `go test ./... -update`.
3. PNG golden files compared with a per-pixel tolerance (arm64 fuses
   multiply-add and amd64 does not).
4. A review gallery: `go test -run TestGallery` writes an HTML page showing
   every golden diagram next to its previous version.
5. Example tests double as documentation.

## Milestones

| # | Milestone |
|---|---|
| M0 | Model and API skeleton, typed containers and options, error collection, IDs, text measurement |
| M1 | Layout, scene, SVG renderer, `themes.Light`, straight edges |
| M2 | Orthogonal routing, ports, waypoints, labels, arrowheads, parallel edges, group endpoints |
| M3 | Icons: `FromSVG`/`FromPNG`, passthrough, `cmd/gen-icons`, `icons/generic`, `icons/network` |
| M4 | draw.io renderer |
| M5 | PNG renderer, SVG subset parser with gradients |
| M6 | Dark, Blueprint and Auto themes, `icons/brands`, gallery, examples, README, v0.1.0 |
