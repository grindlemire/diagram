package drawio

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"flag"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/route"
	"github.com/grindlemire/diagram/themes"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata")

// ---- diagram builders ----

type opt func(*model.Element)

func add(parent *model.Element, kind model.Kind, label string, opts ...opt) *model.Element {
	d := parent.Diagram
	depth := parent.Depth
	if parent != d.Root && parent.Visible() {
		depth++
	}
	e := &model.Element{Kind: kind, Label: label, Parent: parent, Diagram: d, Depth: depth}
	for _, o := range opts {
		o(e)
	}
	parent.Children = append(parent.Children, e)
	return e
}

func node(parent *model.Element, label string, opts ...opt) *model.Element {
	return add(parent, model.KindNode, label, opts...)
}

func desc(s string) opt           { return func(e *model.Element) { e.Desc = s } }
func shape(s model.Shape) opt     { return func(e *model.Element) { e.Shape = s } }
func icon(ic *icons.Icon) opt     { return func(e *model.Element) { e.Icon = ic } }
func place(p model.IconPlace) opt { return func(e *model.Element) { e.IconPlace = p } }
func cell(r, c int) opt           { return func(e *model.Element) { e.Row, e.Col = r, c } }
func cols(n int) opt              { return func(e *model.Element) { e.Cols = n } }
func styled(s model.Style) opt    { return func(e *model.Element) { e.Style = s } }
func invisible(e *model.Element)  { e.Invisible = model.Some(true) }
func headerTop(e *model.Element)  { e.HeaderTop = true }
func maxWidth(px float64) opt     { return func(e *model.Element) { e.MaxWidth = model.Some(px) } }
func iconSize(px float64) opt     { return func(e *model.Element) { e.IconSize = model.Some(px) } }
func connect(d *model.Diagram, from, to *model.Element, opts ...func(*model.Edge)) *model.Edge {
	e := &model.Edge{From: from, To: to}
	for _, o := range opts {
		o(e)
	}
	d.Edges = append(d.Edges, e)
	return e
}

var (
	monoIcon  = icons.MustSVG("box", `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18"/><path d="M9 21V9"/></svg>`)
	colorIcon = icons.MustSVG("dot", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 16"><rect width="32" height="16" rx="8" fill="#e11d48"/><circle cx="8" cy="8" r="5" fill="#fff"/></svg>`)
	pngIcon   = mustPNG("iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAIAAAAmkwkpAAAAQUlEQVR4nAA0AMv/AxajSt8H56F3RN8H5wPfB+csSz7UtcIsSz4AFqNK6lgMFqNK6lgMA98H5yxLPtS1wixLPgMARPsVqbolQpAAAAAASUVORK5CYII=")
)

func mustPNG(b64 string) *icons.Icon {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(err)
	}
	ic, err := icons.FromPNG(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return ic
}

// shapesDiagram has every shape and icon placement.
func shapesDiagram() *model.Diagram {
	d := model.New("Shapes")
	g := add(d.Root, model.KindGrid, "Shapes", cols(4), desc("every outline"))
	node(g, "Box", cell(0, 0), desc("rounded rectangle"))
	node(g, "Rect", cell(0, 1), shape(model.ShapeRect))
	node(g, "Pill", cell(0, 2), shape(model.ShapePill))
	node(g, "Circle", cell(0, 3), shape(model.ShapeCircle))
	node(g, "Cylinder", cell(1, 0), shape(model.ShapeCylinder), desc("primary"))
	node(g, "Queue", cell(1, 1), shape(model.ShapeQueue))
	node(g, "Document", cell(1, 2), shape(model.ShapeDocument))
	node(g, "A note that wraps over several lines", cell(1, 3), shape(model.ShapeNote), maxWidth(120))
	node(g, "Person", cell(2, 0), shape(model.ShapePerson))
	node(g, "Plain <text> & \"quotes\"", cell(2, 1), shape(model.ShapeText))
	node(g, "Styled", cell(2, 2), shape(model.ShapeRect), styled(model.Style{
		Fill: themes.Hex("#fef3c7"), Stroke: themes.Of(themes.Red), TextColor: themes.Of(themes.Red),
		StrokeWidth: model.Some(2.0), Dash: model.DashDashed, Radius: model.Some(4.0), Bold: model.Some(false),
	}))
	node(g, "Dotted", cell(2, 3), styled(model.Style{Dash: model.DashDotted, Fill: themes.RGBA(37, 99, 235, 64)}))

	ic := add(d.Root, model.KindColumn, "Icons", headerTop, icon(monoIcon))
	node(ic, "Icon top", icon(monoIcon), desc("monochrome"))
	node(ic, "Icon left", icon(monoIcon), place(model.IconLeft), iconSize(20), desc("smaller"))
	node(ic, "Icon only", icon(monoIcon), place(model.IconOnly))
	node(ic, "Colour SVG", icon(colorIcon), shape(model.ShapePill))
	node(ic, "PNG", icon(pngIcon), place(model.IconLeft))
	return d
}

// nestedDiagram nests groups three deep with invisible containers in
// between and connects across levels, to groups and to an invisible
// container, with every arrow kind and dash.
func nestedDiagram() *model.Diagram {
	d := model.New("Nested")
	client := node(d.Root, "Client", shape(model.ShapePerson))
	region := add(d.Root, model.KindColumn, "Region", desc("us-east-1"), icon(monoIcon))
	lb := node(region, "Load balancer", icon(monoIcon), place(model.IconLeft), iconSize(20))
	inner := add(region, model.KindRow, "")
	cluster := add(inner, model.KindRow, "Cluster")
	wrap := add(cluster, model.KindColumn, "", invisible)
	pod := add(wrap, model.KindColumn, "Pod", headerTop, icon(monoIcon))
	app := node(pod, "App", desc("Go"))
	side := node(pod, "Sidecar", shape(model.ShapePill))
	cache := node(cluster, "Cache", shape(model.ShapeCylinder))
	store := add(d.Root, model.KindColumn, "", func(e *model.Element) { e.Justify = model.JustifyCenter })
	db := node(store, "DB", shape(model.ShapeCylinder))
	node(store, "Bucket", shape(model.ShapeDocument))

	connect(d, client, lb, func(e *model.Edge) { e.Label = "HTTPS" })
	connect(d, lb, app, func(e *model.Edge) {
		e.Exit, e.Enter = model.SideBottom, model.SideTop
		e.Style.Dash = model.DashDashed
		e.Head = model.ArrowOpen
	})
	connect(d, app, side, func(e *model.Edge) {
		e.Style.Dash = model.DashDotted
		e.Head, e.Tail = model.ArrowDiamond, model.ArrowDot
	})
	connect(d, side, cache, func(e *model.Edge) {
		e.Head, e.Tail = model.ArrowNone, model.ArrowTriangle
		e.Style.Stroke = themes.Of(themes.Red)
		e.Style.StrokeWidth = model.Some(2.0)
	})
	connect(d, client, pod, func(e *model.Edge) { e.Label = "deploys"; e.LabelAt = model.LabelStart })
	connect(d, cache, store, func(e *model.Edge) { e.Label = "to <store> & co" })
	connect(d, app, db, func(e *model.Edge) {
		e.Via = []model.Point{{X: 520, Y: 20}, {X: 700, Y: 20}}
		e.Label = "writes"
		e.LabelAt = model.LabelEnd
		e.Enter = model.SideTop
	})
	connect(d, region, db, func(e *model.Edge) { e.Straight = true; e.Head = model.ArrowTriangle })
	return d
}

// lay lays d out and routes it with the real router.
func lay(t *testing.T, d *model.Diagram) *layout.Result {
	t.Helper()
	res, _ := layout.Layout(d, themes.Light)
	if err := route.Route(res); err != nil {
		t.Fatal(err)
	}
	return res
}

// layFixed lays d out and routes it with straight lines through any
// waypoints, so golden files only change when layout or this package do,
// not when the router does.
func layFixed(d *model.Diagram) *layout.Result {
	res, _ := layout.Layout(d, themes.Light)
	for _, e := range res.Edges {
		from, to := e.From.Shape, e.To.Shape
		first, last := to.Center(), from.Center()
		if len(e.E.Via) > 0 {
			first, last = e.E.Via[0], e.E.Via[len(e.E.Via)-1]
		}
		pts := []model.Point{border(from, e.E.Exit, first)}
		pts = append(pts, e.E.Via...)
		e.Points = append(pts, border(to, e.E.Enter, last))
		e.Rounded = false
		// Label in the middle of the first segment, or near an end.
		a, b := e.Points[0], e.Points[1]
		f := map[model.LabelPos]float64{model.LabelMiddle: 0.5, model.LabelStart: 0.25, model.LabelEnd: 0.75}[e.E.LabelAt]
		if e.E.LabelAt == model.LabelEnd {
			a, b = e.Points[len(e.Points)-2], e.Points[len(e.Points)-1]
		}
		layout.PlaceLabel(res.Theme, e, model.Point{X: a.X + (b.X-a.X)*f, Y: a.Y + (b.Y-a.Y)*f})
	}
	return res
}

// border returns the middle of a pinned side, or where the line from r's
// centre towards p crosses r's border.
func border(r layout.Rect, s model.Side, p model.Point) model.Point {
	c := r.Center()
	switch s {
	case model.SideTop:
		return model.Point{X: c.X, Y: r.Y}
	case model.SideBottom:
		return model.Point{X: c.X, Y: r.Bottom()}
	case model.SideLeft:
		return model.Point{X: r.X, Y: c.Y}
	case model.SideRight:
		return model.Point{X: r.Right(), Y: c.Y}
	}
	dx, dy := p.X-c.X, p.Y-c.Y
	k := math.Inf(1)
	if dx != 0 {
		k = r.W / 2 / math.Abs(dx)
	}
	if dy != 0 {
		k = math.Min(k, r.H/2/math.Abs(dy))
	}
	if math.IsInf(k, 1) {
		return c
	}
	return model.Point{X: c.X + dx*k, Y: c.Y + dy*k}
}

// elbows replaces every route with an orthogonal one, as the orthogonal
// router produces, so orthogonal edge styles are covered whatever the
// router currently does: an elbow between columns, a U-turn back to a
// node in the same column, and a detour that enters from above.
func elbows(res *layout.Result) {
	for i, e := range res.Edges {
		a, b := e.From.Shape, e.To.Shape
		s := model.Point{X: a.Right(), Y: a.Y + a.H/2}
		var lp model.Point
		switch {
		case i == 2: // detour over the target, entering its top
			t := model.Point{X: b.X + b.W/2, Y: b.Y}
			mx, top := s.X+20, math.Min(a.Y, b.Y)-20
			e.Points = []model.Point{s, {X: mx, Y: s.Y}, {X: mx, Y: top}, {X: t.X, Y: top}, t}
			e.ExitSide, e.EnterSide = model.SideRight, model.SideTop
			lp = model.Point{X: (mx + t.X) / 2, Y: top}
		case a.X == b.X: // U-turn into the target's right side
			t := model.Point{X: b.Right(), Y: b.Y + b.H/2}
			mx := math.Max(s.X, t.X) + 24
			e.Points = []model.Point{s, {X: mx, Y: s.Y}, {X: mx, Y: t.Y}, t}
			e.ExitSide, e.EnterSide = model.SideRight, model.SideRight
			lp = model.Point{X: mx, Y: (s.Y + t.Y) / 2}
		default:
			t := model.Point{X: b.X, Y: b.Y + b.H/2}
			mx := (s.X + t.X) / 2
			e.Points = []model.Point{s, {X: mx, Y: s.Y}, {X: mx, Y: t.Y}, t}
			e.ExitSide, e.EnterSide = model.SideRight, model.SideLeft
			lp = model.Point{X: mx, Y: (s.Y + t.Y) / 2}
		}
		e.Rounded = true
		layout.PlaceLabel(res.Theme, e, lp)
	}
}

func elbowDiagram() *model.Diagram {
	d := model.New("Elbows")
	col := add(d.Root, model.KindColumn, "")
	a := node(col, "Top")
	b := node(col, "Bottom")
	c := node(d.Root, "Right", shape(model.ShapeQueue))
	connect(d, a, c, func(e *model.Edge) { e.Label = "up" })
	connect(d, b, c, func(e *model.Edge) { e.Style.Dash = model.DashDashed })
	connect(d, a, c, func(e *model.Edge) { e.Label = "over"; e.Head = model.ArrowOpen })
	connect(d, a, b, func(e *model.Edge) { e.Label = "back" })
	return d
}

func goldenCases() map[string][]Page {
	elb := layFixed(elbowDiagram())
	elbows(elb)
	return map[string][]Page{
		"shapes":    {{Name: "Shapes", Result: layFixed(shapesDiagram()), Margin: 24}},
		"nested":    {{Name: "Nested", Result: layFixed(nestedDiagram()), Margin: 24}},
		"multipage": {{Name: "Elbows", Result: elb, Margin: 16}, {Name: "Nested & more", Result: layFixed(nestedDiagram()), Margin: 0}},
	}
}

func render(t *testing.T, pages []Page) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(&buf, pages); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGolden(t *testing.T) {
	for _, name := range []string{"shapes", "nested", "multipage"} {
		t.Run(name, func(t *testing.T) {
			got := render(t, goldenCases()[name])
			if again := render(t, goldenCases()[name]); !bytes.Equal(got, again) {
				t.Fatal("output differs between runs")
			}
			path := filepath.Join("testdata", name+".drawio")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				gl, wl := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
				for i := 0; i < len(gl) || i < len(wl); i++ {
					var g, w string
					if i < len(gl) {
						g = gl[i]
					}
					if i < len(wl) {
						w = wl[i]
					}
					if g != w {
						t.Fatalf("%s differs at line %d:\n got: %.300s\nwant: %.300s\n(run with -update to accept)", path, i+1, g, w)
					}
				}
			}
		})
	}
}

// ---- structural checks ----

type xFile struct {
	Diagrams []struct {
		ID    string  `xml:"id,attr"`
		Name  string  `xml:"name,attr"`
		Cells []xCell `xml:"mxGraphModel>root>mxCell"`
	} `xml:"diagram"`
}

type xCell struct {
	ID     string `xml:"id,attr"`
	Value  string `xml:"value,attr"`
	Style  string `xml:"style,attr"`
	Parent string `xml:"parent,attr"`
	Source string `xml:"source,attr"`
	Target string `xml:"target,attr"`
	Vertex string `xml:"vertex,attr"`
	Edge   string `xml:"edge,attr"`
	Geo    *struct {
		X        float64  `xml:"x,attr"`
		Y        float64  `xml:"y,attr"`
		W        float64  `xml:"width,attr"`
		H        float64  `xml:"height,attr"`
		Relative string   `xml:"relative,attr"`
		Points   []xPoint `xml:"Array>mxPoint"`
		Extra    []xPoint `xml:"mxPoint"`
	} `xml:"mxGeometry"`
}

type xPoint struct {
	X  float64 `xml:"x,attr"`
	Y  float64 `xml:"y,attr"`
	As string  `xml:"as,attr"`
}

func parseStyle(t *testing.T, s string) map[string]string {
	t.Helper()
	if !strings.HasSuffix(s, ";") {
		t.Errorf("style %.80q does not end in ;", s)
	}
	out := map[string]string{}
	for _, kv := range strings.Split(strings.TrimSuffix(s, ";"), ";") {
		k, v, _ := strings.Cut(kv, "=")
		if k == "" {
			t.Errorf("empty key in style %.80q", s)
		}
		out[k] = v
	}
	return out
}

func TestStructure(t *testing.T) {
	cases := goldenCases()
	// The real router, whatever it currently draws.
	cases["routed"] = []Page{
		{Name: "Shapes", Result: lay(t, shapesDiagram()), Margin: 24},
		{Name: "Nested", Result: lay(t, nestedDiagram()), Margin: 8},
	}
	for name, pages := range cases {
		t.Run(name, func(t *testing.T) {
			var f xFile
			if err := xml.Unmarshal(render(t, pages), &f); err != nil {
				t.Fatal(err)
			}
			if len(f.Diagrams) != len(pages) {
				t.Fatalf("got %d pages, want %d", len(f.Diagrams), len(pages))
			}
			pageIDs := map[string]bool{}
			for i, dg := range f.Diagrams {
				if pageIDs[dg.ID] {
					t.Errorf("duplicate page id %q", dg.ID)
				}
				pageIDs[dg.ID] = true
				checkPage(t, pages[i], dg.Cells)
			}
		})
	}
}

func checkPage(t *testing.T, p Page, cells []xCell) {
	byID := map[string]*xCell{}
	for i := range cells {
		c := &cells[i]
		if byID[c.ID] != nil {
			t.Errorf("duplicate cell id %q", c.ID)
		}
		byID[c.ID] = c
	}
	if byID["0"] == nil || byID["1"] == nil || byID["1"].Parent != "0" {
		t.Fatal("missing root cells 0 and 1")
	}
	// abs returns a vertex's absolute origin by walking up its parents.
	var abs func(c *xCell) (float64, float64)
	abs = func(c *xCell) (float64, float64) {
		if c.Vertex != "1" {
			return 0, 0
		}
		px, py := abs(byID[c.Parent])
		return px + c.Geo.X, py + c.Geo.Y
	}
	for i := range cells {
		c := &cells[i]
		if c.ID == "0" {
			continue
		}
		par := byID[c.Parent]
		if par == nil {
			t.Errorf("%s: parent %q does not exist", c.ID, c.Parent)
			continue
		}
		if c.ID == "1" {
			continue
		}
		st := parseStyle(t, c.Style)
		if img, ok := st["image"]; ok && (strings.Contains(img, ";base64") || !strings.HasPrefix(img, "data:image/")) {
			t.Errorf("%s: bad image %.60q", c.ID, img)
		}
		if c.Geo == nil {
			t.Errorf("%s: no geometry", c.ID)
			continue
		}
		switch {
		case c.Vertex == "1":
			if par.Vertex == "1" {
				const eps = 0.02
				if c.Geo.X < -eps || c.Geo.Y < -eps || c.Geo.X+c.Geo.W > par.Geo.W+eps || c.Geo.Y+c.Geo.H > par.Geo.H+eps {
					t.Errorf("%s: geometry %+v not inside parent %s (%vx%v)", c.ID, *c.Geo, par.ID, par.Geo.W, par.Geo.H)
				}
			}
		case c.Edge == "1":
			if c.Parent != "1" {
				t.Errorf("%s: edge parent %q, want 1", c.ID, c.Parent)
			}
			for _, end := range []string{c.Source, c.Target} {
				if end != "" && (byID[end] == nil || byID[end].Vertex != "1") {
					t.Errorf("%s: endpoint %q is not a vertex", c.ID, end)
				}
			}
			has := map[string]bool{}
			for _, pt := range c.Geo.Extra {
				has[pt.As] = true
			}
			if c.Source == "" && !has["sourcePoint"] || c.Target == "" && !has["targetPoint"] {
				t.Errorf("%s: unattached end without a terminal point", c.ID)
			}
		default:
			t.Errorf("%s: neither vertex nor edge", c.ID)
		}
	}

	// Every node and visible group sits where the layout put it.
	res := p.Result
	for _, b := range res.Boxes {
		c := byID[b.El.ID]
		if b.El.Kind != model.KindNode && !b.Shown {
			if c != nil {
				t.Errorf("invisible container %s was emitted", b.El.ID)
			}
			continue
		}
		if c == nil {
			t.Errorf("%s: not emitted", b.El.ID)
			continue
		}
		x, y := abs(c)
		want := b.Shape
		if !approx(x, want.X+p.Margin) || !approx(y, want.Y+p.Margin) || !approx(c.Geo.W, want.W) || !approx(c.Geo.H, want.H) {
			t.Errorf("%s: at (%v,%v %vx%v), want (%v,%v %vx%v)", b.El.ID, x, y, c.Geo.W, c.Geo.H, want.X+p.Margin, want.Y+p.Margin, want.W, want.H)
		}
		if ic := byID[b.El.ID+"#icon"]; ic != nil {
			x, y := abs(ic)
			if !approx(x, b.Icon.X+p.Margin) || !approx(y, b.Icon.Y+p.Margin) {
				t.Errorf("%s icon: at (%v,%v), want (%v,%v)", b.El.ID, x, y, b.Icon.X+p.Margin, b.Icon.Y+p.Margin)
			}
		}
	}
	// Edges attach to their endpoints' cells, and their fixed ports land
	// on the routed end points.
	for _, e := range res.Edges {
		c := byID[e.E.ID]
		if c == nil {
			t.Errorf("edge %s: not emitted", e.E.ID)
			continue
		}
		st := parseStyle(t, c.Style)
		ends := []struct {
			box    *layout.Box
			id     string
			prefix string
			pt     model.Point
		}{{e.From, c.Source, "exit", e.Points[0]}, {e.To, c.Target, "entry", e.Points[len(e.Points)-1]}}
		for _, end := range ends {
			if end.box.El.Kind != model.KindNode && !end.box.Shown {
				if end.id != "" {
					t.Errorf("edge %s: attached to invisible %s", e.E.ID, end.id)
				}
				continue
			}
			if end.id != end.box.El.ID {
				t.Errorf("edge %s: %s end is %q, want %q", e.E.ID, end.prefix, end.id, end.box.El.ID)
				continue
			}
			v := byID[end.id]
			x, y := abs(v)
			fx, _ := strconv.ParseFloat(st[end.prefix+"X"], 64)
			fy, _ := strconv.ParseFloat(st[end.prefix+"Y"], 64)
			px, py := x+fx*v.Geo.W, y+fy*v.Geo.H
			if math.Abs(px-(end.pt.X+p.Margin)) > 0.1 || math.Abs(py-(end.pt.Y+p.Margin)) > 0.1 {
				t.Errorf("edge %s: %s port at (%v,%v), want (%v,%v)", e.E.ID, end.prefix, px, py, end.pt.X+p.Margin, end.pt.Y+p.Margin)
			}
		}
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.02 }

// ---- helpers ----

func TestAlong(t *testing.T) {
	pts := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 100}}
	for _, tc := range []struct {
		p       model.Point
		x       float64
		offX, y float64
	}{
		{model.Point{X: 0, Y: 0}, -1, 0, 0},
		{model.Point{X: 100, Y: 0}, 0, 0, 0},
		{model.Point{X: 50, Y: -5}, -0.5, 0, -5},
		{model.Point{X: 103, Y: 100}, 1, 3, 0},
	} {
		x, off := along(pts, tc.p)
		if !approx(x, tc.x) || !approx(off.X, tc.offX) || !approx(off.Y, tc.y) {
			t.Errorf("along(%v) = %v, %v; want %v, (%v,%v)", tc.p, x, off, tc.x, tc.offX, tc.y)
		}
	}
}

func TestSimplify(t *testing.T) {
	got := simplify([]model.Point{{X: 0, Y: 0}, {X: 0, Y: 0}, {X: 10, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 10}})
	want := []model.Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 10}}
	if len(got) != len(want) {
		t.Fatalf("simplify = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("simplify = %v, want %v", got, want)
		}
	}
}

func TestIDsAvoidReserved(t *testing.T) {
	d := model.New("ids")
	a := node(d.Root, "A", func(e *model.Element) { e.ExplicitID = "1" })
	b := node(d.Root, "B", func(e *model.Element) { e.ExplicitID = "0" })
	connect(d, a, b)
	var f xFile
	if err := xml.Unmarshal(render(t, []Page{{Result: layFixed(d)}}), &f); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range f.Diagrams[0].Cells {
		if seen[c.ID] {
			t.Errorf("duplicate id %q", c.ID)
		}
		seen[c.ID] = true
	}
	if f.Diagrams[0].Name != "Page-1" {
		t.Errorf("unnamed page called %q", f.Diagrams[0].Name)
	}
}
