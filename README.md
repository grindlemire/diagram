# diagram

System diagrams as Go code. Declare nodes, groups and edges; get SVG, PNG,
interactive HTML or an editable draw.io file.

- Pure Go: the standard library plus `golang.org/x/image`. No Graphviz, no
  browser, no cgo.
- Manual layout with typed containers (`Row`, `Column`, `Grid`, `Canvas`), so
  the picture comes out the way you arranged it, every time.
- Orthogonal edge routing around nodes and groups, with pinned ports and
  waypoints when you need them.
- Themes, composable styles, bundled icons, and your own SVG or PNG icons.

```go
package main

import (
	"log"

	"github.com/grindlemire/diagram"
	"github.com/grindlemire/diagram/icons/brands"
	"github.com/grindlemire/diagram/icons/generic"
)

func main() {
	d := diagram.New("Payments")

	web := d.Node("Web app", diagram.Desc("Next.js"), diagram.Icon(generic.Browser))
	vpc := d.Column("Production VPC", diagram.Desc("us-east-1"), diagram.Icon(generic.Cloud))
	api := vpc.Node("API", diagram.Desc("Go · 3 replicas"), diagram.Icon(brands.Go), diagram.IconLeft)
	workers := vpc.Grid("Workers", 2, diagram.Uniform)
	ledger := workers.Node("Ledger worker", 0, 0)
	payout := workers.Node("Payout worker", 0, 1)
	db := d.Node("Postgres", diagram.Cylinder, diagram.Desc("primary"))

	critical := diagram.Style(diagram.Stroke(diagram.Red), diagram.StrokeWidth(2))
	d.Connect(web, api, diagram.Label("HTTPS"))
	d.Connect(api, ledger)
	d.Connect(api, payout, diagram.Dashed)
	d.Connect(ledger, db, critical, diagram.Label("writes"))

	if err := d.WriteFile("payments.svg"); err != nil {
		log.Fatal(err)
	}
}
```

```
go get github.com/grindlemire/diagram
```

Requires Go 1.26 or later.

## Layout

The container you add something to decides where it goes. The container kind
is part of the Go type, so placing something wrongly doesn't compile.

| Container | Places children | Child constructor |
|---|---|---|
| `Row` | left to right | `row.Node(label, opts...)` |
| `Column` | top to bottom | `col.Node(label, opts...)` |
| `Grid` | in cells; `Span(rows, cols)` covers several | `grid.Node(label, row, col, opts...)` |
| `Canvas` | at coordinates relative to the canvas | `canvas.Node(label, x, y, opts...)` |

Containers nest in any combination. A container with a label draws a box with
a header; one with an empty label is invisible and only arranges its children.
Nested boxes get progressively darker backgrounds. The diagram itself is a
`Row`, or a `Column` with `diagram.Vertical`.

Container options: `Gap`, `Padding`, `AlignStart`/`AlignCenter`/`AlignEnd`
(across the flow), `JustifyStart` (default)/`JustifyCenter`/`JustifyEnd`/
`JustifySpaceBetween` (along it), `Uniform` (equal-sized child nodes; circles,
people and icon-only nodes keep their proportions), `Invisible`, `Visible`,
`HeaderTop`, `Defaults(styles...)`, and `AllowOverlap` for a `Canvas` whose
children are meant to overlap (otherwise overlap is an error).

## Nodes

A node has a label, an optional description (`Desc`) and an optional icon
(`Icon`, placed with `IconTop`, `IconLeft` or `IconOnly`, sized with
`IconSize`, coloured with `IconColor`). Labels wrap at `MaxWidth`.
`Tooltip(text)` adds hover text and `Link(url)` makes a node or group link
somewhere (http, https, mailto or a relative URL).
`Size(w, h)` fixes a node's size. `Badge("×3")` adds a small pill on the
top-right corner (several stack leftwards) and `Status(diagram.Green)` a
status dot on the top-left.

Shapes: `Box` (default), `Rect`, `Pill`, `Circle`, `Cylinder`, `Queue`,
`Document`, `Person`, `Note`, `Text`.

## Edges

```go
d.Connect(from, to, opts...)
```

Either end can be a node or a container, or the same node twice for a
self-loop (layout reserves room for it). Edges are routed with right angles
around everything that isn't their endpoint. Options:

- `Label(s)`, `LabelAt(diagram.Start | diagram.Middle | diagram.End)`
- `Exit(side)`, `Enter(side)` to pin ports (`diagram.Top`, `Right`, `Bottom`, `Left`)
- `Via(diagram.Pt(x, y), ...)` for waypoints
- `Tooltip(text)` for hover text
- `Straight` for a direct line
- `CrossGroups` to let the edge pass through groups that hold neither end
  (by default it routes around them)
- `Arrow` (default), `ArrowBoth`, `NoArrow`, `Head(kind)`, `Tail(kind)` with
  `Triangle`, `Open`, `Diamond`, `Dot`

Several edges at once:

```go
d.Path(web, gql, api)                     // web → gql → api
d.Fan(lb, w1, w2, w3).With(diagram.Dashed) // lb → each worker
d.FanIn(db, w1, w2)                       // each worker → db
```

`With` applies options to every edge in the set; `All()` returns them.

Gaps make room for edges automatically. Wherever edges connect, a gap is at
least wide enough for one clean lane and the label on it, and after routing,
gaps whose edges don't fit side by side are widened and the diagram laid out
again. A `Gap` you set is a minimum. It only ever adds space; a gap with no
edges through it stays exactly as written. Use
`diagram.New(title, diagram.ExactSpacing)` when positions must match a design
exactly; edges then squeeze or detour to fit.

## Styles and themes

Style options are values. Bundle and reuse them:

```go
async := diagram.Style(diagram.Dashed, diagram.Stroke(diagram.Muted))
critical := diagram.Style(diagram.Stroke(diagram.Red), diagram.StrokeWidth(2))
d.Connect(api, queue, async)
```

Give a style a name when it means something, and add a legend:

```go
critical := diagram.Named("critical path", diagram.Stroke(diagram.Red), diagram.StrokeWidth(2))
async := diagram.Named("async", diagram.Dashed)
d.Connect(api, db, critical)
d.Connect(api, queue, async)
d.Legend("Legend")
```

The legend lists every named style the diagram uses, with a sample line for
edge styles or a swatch for node and group styles. It's a container like any
other (`Legend` exists on the diagram and on every container), sizes to its
content, and draws nothing if no named styles are used. In SVG and HTML,
elements using a named style get the class `style-<name>` (`style-critical-path`).
Two different styles with the same name are an error.

`Fill`, `Stroke`, `TextColor`, `StrokeWidth`, `Radius`, `FontSize`, `Bold`,
`Solid`, `Dashed` and `Dotted` work on nodes, groups and edges. Later options
win. Nothing is inherited implicitly: a group's style applies to its own box,
and `Defaults` applies styles to the nodes created in it.

On a literal fill such as `Fill(diagram.Hex("#dcfce7"))`, text switches to
dark or white automatically so it stays readable in every theme.

Colours are theme tokens (`diagram.Accent`, `Red`, `Muted`, ...) that follow
the theme, or literals (`diagram.Hex("#ff6600")`). Themes are in package
`themes`: `Light` (default), `Dark`, `Blueprint`, and `Auto(light, dark)`,
which makes SVG output follow the viewer's `prefers-color-scheme`.

```go
d := diagram.New("Docs", diagram.Theme(themes.Auto(themes.Light, themes.Dark)))
```

## Icons

Bundled sets, each its own package so you only carry what you import:

- `icons/generic`: 47 infrastructure icons (server, database, queue, user...)
  from [Lucide](https://lucide.dev)
- `icons/network`: 20 network icons from Lucide
- `icons/brands`: 26 technology logos from
  [Simple Icons](https://simpleicons.org), with brand colours
  (`brands.PostgresColor`)

Monochrome icons use the theme's icon colour, or `IconColor(c)`. Your own
icons:

```go
logo, err := icons.Open("logo.svg") // or .png; also icons.FromSVG / FromPNG
d.Node("Our service", diagram.Icon(logo), diagram.IconSize(48))
```

Any well-formed SVG works for SVG and draw.io output. PNG output draws a
subset of SVG (shapes, paths, transforms, `use`, linear and radial gradients)
and reports anything else as an error naming the icon. `icon.CheckPNG()`
checks early.

## Fonts

Text uses the embedded Inter font. For scripts Inter doesn't cover, register
TrueType fallback fonts once, before rendering:

```go
//go:embed NotoSansSC-Regular.ttf
var notoSC []byte

func init() { diagram.MustRegisterFont(notoSC) }
```

Each character uses the first font that has it. SVG output embeds a subset
of every font it used; PNG draws them directly. Text that no registered font
can draw is reported as an error rather than drawn as boxes.

## Output

```go
d.WriteFile("out.svg")    // format from the extension: .svg, .png, .html, .drawio
d.RenderSVG(w, opts...)
d.RenderPNG(w, opts...)   // 2× pixel density by default
d.RenderHTML(w, opts...)  // self-contained interactive page
d.RenderDrawio(w, opts...)
diagram.WriteDrawio(w, []*diagram.Diagram{d1, d2}) // one page per diagram
```

Options: `Margin(px)`, `WithTheme(t)` (all formats); `Scale(n)` (PNG);
`Transparent` (SVG and PNG); `EmbedFont` (default) or `FontByName` (SVG and
HTML).

- **SVG** embeds only the glyphs it uses from Inter, so it looks the same in
  every viewer and stays small.
- **PNG** is rasterized in Go and matches the SVG.
- **HTML** is one file with no network requests: the SVG plus pan and zoom
  (wheel, drag, pinch, `+ − 0`, arrow keys), hover highlighting of each
  element's connections, click to focus an element (Esc clears), tooltips
  with descriptions, `Tooltip` text and links (an "Open ↗" link in the
  tooltip, or Cmd/Ctrl-click), Tab between nodes, `/` to search, and a
  light/dark toggle, starting from the viewer's system setting, when the theme
  is `themes.Light` or `themes.Auto`.
  Deep links: `page.html#vpc/api` opens focused on that element, and
  `#from=<id>&to=<id>` on a traced path; focus a node, then Shift-click another
  to highlight the route between them. "Copy link" (or `C`) copies the current
  focus or path. `?` lists every shortcut.
  `Tooltip` and `Link` also work in SVG (as `<title>` and `<a>`) and draw.io
  (as the cell's tooltip and link).
- **Embedding.** `d.HTMLFragment()` (html/template), `@diagram.Embed(d)`
  (templ, with no templ dependency) or `d.RenderEmbed(w)` writes a
  `<diagram-view>` element with the same interactions, isolated in a shadow
  root, so several diagrams can share a page and htmx swaps work. It fills its
  container's width at the diagram's aspect ratio, or `Height("60vh")`. Plain
  scrolling scrolls the page; ⌘/Ctrl-scroll zooms. Each fragment carries the
  script by default (defined once per page); to serve it once instead, use
  `http.Handle("/diagram.js", diagram.ScriptHandler())` and pass
  `diagram.NoScript`, and `Nonce(n)` under a Content-Security-Policy. Style it
  with `--diagram-background`, `--diagram-font` and `theme="dark"`
  (or `EmbedTheme("dark")`); viewers can switch the theme from the toolbar,
  as on the page, and `Toolbar(false)` hides it. Deep links are
  `#<name>:<element-id>` (`EmbedName`).

  ```go
  // templ
  @diagram.Embed(docs.Payments(), diagram.Height("480px"))
  ```
- **draw.io** files are uncompressed and editable: groups are containers,
  nodes are native draw.io shapes, and edges are attached to their endpoints,
  so moving a box in draw.io drags its edges along.

## Errors

Builder methods don't return errors. Mistakes the compiler can't catch, such
as two nodes in one grid cell, are recorded with the file and line of the
call. `Validate()` and every render method return all of them at once:

```
payments.go:9: node "Worker 2": cell (0,1) already occupied by node "Worker 1" (payments.go:8)
payments.go:14: edge API → Worker 2: no route from exit top
payments.go:20: node "B" overlaps node "A" (payments.go:19) in canvas "Legacy"
payments.go:23: node "Launch 🚀": no font has a glyph for '🚀' (U+1F680); register one with diagram.RegisterFont
```

`Validate` checks everything except routing, which runs when you render.

## Command line

```
go install github.com/grindlemire/diagram/cmd/diagram@latest
```

Any exported function `func() *diagram.Diagram` in a package is a diagram. Its
file name is the function name in kebab case: `PaymentsFlow` is written as
`payments-flow.svg`.

```
diagram list   [pkg]                                   # names, functions, positions
diagram render [pkg] [-o diagrams] [-f svg,png,html,drawio] [-only name,...]
diagram watch  [pkg] [-addr 127.0.0.1:0] [-o dir] [-f svg]
```

`render` writes every diagram (SVG by default) and reports a broken diagram's
errors with file and line without stopping the others; it exits 1 if any
failed. `watch` serves an index of the package's diagrams and each one as an
interactive page, and reloads open pages when you save. Compile and
validation errors appear in the page until you fix them. With `-o` it also
writes files after every run.

The package can't be `package main`. The command builds the functions by
generating a small program in `.diagram/` at your module root and running it
with `go run` from the package directory; add `.diagram/` to your `.gitignore`.

## Golden tests

```go
func TestPaymentsDiagram(t *testing.T) {
	diagramtest.Golden(t, docs.PaymentsFlow())
}
```

`Golden` fails the test if the diagram is invalid, renders it and compares it
with `testdata/<test name>.svg`. `diagramtest.Name("payments")` picks the file
name and `diagramtest.Formats("svg", "drawio")` adds formats (`html` and `png`
work too; PNG allows small anti-aliasing differences). Create or update golden
files with

```
go test ./... -diagram.update
DIAGRAM_UPDATE=1 go test ./...
```

On a mismatch the new output is written next to the golden file as
`testdata/<name>.got.svg`; open both in a browser to compare.

## Development

```
go test ./...
go test . -update                              # rewrite golden files
go test . -run TestGallery -gallery /tmp/g.html # review changes side by side
go generate ./icons/...                         # regenerate bundled icons
```

## Licences

diagram is MIT licensed (see `LICENSE`). Bundled assets keep their own licences, which ship next to them: Inter (SIL Open Font License, `internal/text/fonts`),
Lucide (ISC, with MIT for icons derived from Feather), Simple Icons (CC0;
the Kafka logo is Apache-2.0). Brand logos are trademarks of their owners.
