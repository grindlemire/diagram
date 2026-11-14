package diagram

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"math"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/themes"
)

// ---- Path, Fan, FanIn, With ----

func ends(t *testing.T, d *Diagram) []string {
	t.Helper()
	var out []string
	for _, e := range d.m.Edges {
		out = append(out, e.From.Label+"→"+e.To.Label)
	}
	return out
}

func TestPathAndFan(t *testing.T) {
	d := New("t")
	a, b, c, x := d.Node("A"), d.Node("B"), d.Node("C"), d.Node("X")
	p := d.Path(a, b, c)
	f := d.Fan(x, a, b)
	in := d.FanIn(x, b, c)
	if got, want := strings.Join(ends(t, d), " "), "A→B B→C X→A X→B B→X C→X"; got != want {
		t.Errorf("edges %s, want %s", got, want)
	}
	if len(p.All()) != 2 || len(f.All()) != 2 || len(in.All()) != 2 {
		t.Errorf("set sizes %d %d %d, want 2 each", len(p.All()), len(f.All()), len(in.All()))
	}
	if p.All()[1].e != d.m.Edges[1] {
		t.Errorf("All doesn't return the set's edges in order")
	}
	if got := p.With(Dashed, Label("sync")); got != p {
		t.Errorf("With should return its set")
	}
	for _, e := range p.All() {
		if e.e.Style.Dash != model.DashDashed || e.e.Label != "sync" {
			t.Errorf("With didn't reach %s", e.e.Describe())
		}
	}
	if d.m.Edges[2].Label != "" {
		t.Errorf("With reached an edge outside its set")
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPathAndFanErrors(t *testing.T) {
	d := New("t")
	a, b := d.Node("A"), d.Node("B")
	other := New("other").Node("Elsewhere")
	l1 := line() + 1
	d.Path(a)
	l2 := line() + 1
	d.Fan(a)
	l3 := line() + 1
	d.FanIn(a)
	l4 := line() + 1
	d.Path(a, b, other)
	l5 := line() + 1
	d.Fan(a, nil)
	set := d.Fan(a, b)
	l6 := line() + 1
	set.With(Exit(Side(9)))
	lg := d.Legend("Legend")
	l7 := line() + 1
	d.Connect(a, lg)

	err := d.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	msgs := strings.Split(err.Error(), "\n")
	want := []string{
		fmt.Sprintf("conveniences_test.go:%d: Path: want at least 2 endpoints, got 1", l1),
		fmt.Sprintf("conveniences_test.go:%d: Fan: want at least one endpoint to connect to", l2),
		fmt.Sprintf("conveniences_test.go:%d: FanIn: want at least one endpoint to connect from", l3),
		fmt.Sprintf(`conveniences_test.go:%d: edge B → Elsewhere: node "Elsewhere" belongs to a different diagram (conveniences_test.go:%d)`, l4, l1-2),
		fmt.Sprintf("conveniences_test.go:%d: Fan: nil endpoint", l5),
		fmt.Sprintf("conveniences_test.go:%d: edge A → B: Exit(9): not a side", l6),
		fmt.Sprintf(`conveniences_test.go:%d: edge A → Legend: column "Legend" is a legend, which can't be an edge endpoint`, l7),
	}
	if len(msgs) != len(want) {
		t.Fatalf("got %d errors, want %d:\n%s", len(msgs), len(want), err)
	}
	for i := range want {
		if msgs[i] != want[i] {
			t.Errorf("error %d:\n got %s\nwant %s", i, msgs[i], want[i])
		}
	}
	// The valid edge of the cross-diagram path is still made.
	if got := strings.Join(ends(t, d), " "); got != "A→B A→B" {
		t.Errorf("edges %s", got)
	}
}

// ---- Named styles and legends ----

// legendDiagram uses named styles on an edge, a node, a group, a node and
// an edge both, and through Defaults; "unused" is never applied.
func legendDiagram() (*Diagram, *Column) {
	d := New("Legend")
	crit := Named("critical path", Stroke(Red), StrokeWidth(2))
	ext := Named("external", Dashed, Fill(Hex("#f8fafc")))
	zone := Named("trust zone", Stroke(Orange))
	both := Named("both", Stroke(Teal))
	_ = Named("unused", Stroke(Pink))
	dflt := Named("default", Bold)

	a := d.Node("A", ext)
	g := d.Column("Zone", zone, Defaults(dflt))
	b := g.Node("B", both)
	c := g.Node("C")
	d.Connect(a, b, crit)
	d.Connect(b, c, both)
	return d, d.Legend("Key")
}

func TestLegendEntries(t *testing.T) {
	d, lg := legendDiagram()
	res := lay(t, d)
	box := res.ByEl[lg.el]
	if box == nil || !box.Shown {
		t.Fatal("legend not laid out")
	}
	type entry struct {
		name string
		kind layout.SampleKind
	}
	var got []entry
	for _, row := range box.Kids() {
		if !row.El.Generated || len(row.Kids()) != 2 {
			t.Fatalf("legend child %s is not a generated entry", row.El.Describe())
		}
		s, n := row.Kids()[0], row.Kids()[1]
		if s.Sample == nil || n.Sample != nil || len(n.Lines) != 1 || n.Lines[0].Text != s.Sample.Name {
			t.Fatalf("entry %s: sample %+v, name %+v", row.El.ID, s.Sample, n.Lines)
		}
		got = append(got, entry{s.Sample.Name, s.Sample.Kind})
		if !box.Content.Contains(row.Rect) {
			t.Errorf("entry %s %+v outside the legend %+v", s.Sample.Name, row.Rect, box.Content)
		}
	}
	// First-use order: ext (A), zone and default (the group), both (B),
	// critical path (the first edge).
	want := []entry{
		{"external", layout.SampleNode},
		{"trust zone", layout.SampleGroup},
		{"default", layout.SampleNode},
		{"both", layout.SampleEdge},
		{"critical path", layout.SampleEdge},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("entries\n got %v\nwant %v", got, want)
	}
	// Samples line up, so the names do too.
	var x float64
	for i, row := range box.Kids() {
		nx := row.Kids()[1].Rect.X
		if i > 0 && !near(nx, x) {
			t.Errorf("entry %d's name at x=%v, want %v", i, nx, x)
		}
		x = nx
	}
	crit := box.Kids()[4].Kids()[0].Sample
	if tok, _ := crit.Color.Token(); tok != themes.Red || crit.Width != 2 {
		t.Errorf("critical path sample %+v", crit)
	}
	ext := box.Kids()[0].Kids()[0]
	if ext.Look.Dash != model.DashDashed || ext.Look.Fill != Hex("#f8fafc") {
		t.Errorf("external sample look %+v", ext.Look)
	}
	// The legend keeps its natural size instead of stretching to the
	// row's height.
	if box.Rect.H >= res.ByEl[d.m.Root.Children[1]].Rect.H+40 {
		t.Errorf("legend %+v stretched", box.Rect)
	}
	// Laying out again rebuilds the same entries without touching the model.
	if len(lg.el.Children) != 0 {
		t.Errorf("layout added %d children to the model", len(lg.el.Children))
	}
	again := lay(t, d)
	if n := len(again.ByEl[lg.el].Kids()); n != 5 {
		t.Errorf("second layout has %d entries", n)
	}
}

func TestLegendUserChildrenFirst(t *testing.T) {
	d := New("t")
	a := d.Node("A")
	d.Connect(a, d.Node("B"), Named("async", Dashed))
	lg := d.Legend("Key")
	lg.Node("Note", Text)
	res := lay(t, d)
	kids := res.ByEl[lg.el].Kids()
	if len(kids) != 2 || kids[0].El.Label != "Note" || !kids[1].El.Generated {
		t.Fatalf("legend children %v", kids)
	}
}

func TestEmptyLegend(t *testing.T) {
	d := New("t")
	a := d.Node("A")
	b := d.Node("B", Fill(Red))
	d.Connect(a, b)
	lg := d.Legend("Key")
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	res := lay(t, d)
	if res.ByEl[lg.el] != nil {
		t.Errorf("empty legend was laid out")
	}
	// It takes no space: the diagram is as wide as without it.
	plain := New("t")
	plain.Connect(plain.Node("A"), plain.Node("B", Fill(Red)))
	if w := lay(t, plain).Width; !near(w, res.Width) {
		t.Errorf("width %v with an empty legend, %v without", res.Width, w)
	}
	var svg, dio bytes.Buffer
	if err := d.RenderSVG(&svg); err != nil {
		t.Fatal(err)
	}
	if err := d.RenderDrawio(&dio); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(svg.String(), "legend") || strings.Contains(dio.String(), `"key"`) {
		t.Errorf("empty legend rendered")
	}
}

func TestNamedStyleErrors(t *testing.T) {
	d := New("t")
	l1 := line() + 1
	red := Named("hot", Stroke(Red))
	blue := Named("hot", Stroke(Blue))
	same := Named("hot", Stroke(Red))
	d.Node("A", red)
	d.Node("B", same)
	l2 := line() + 1
	d.Node("C", blue)
	l3 := line() + 1
	d.Node("D", Named(" ", Fill(Hex("#zz"))))
	err := d.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	for _, sub := range []string{
		fmt.Sprintf(`conveniences_test.go:%d: node "C": Named("hot"): a different style has this name (conveniences_test.go:%d)`, l2, l1),
		fmt.Sprintf(`conveniences_test.go:%d: node "D": Fill: invalid hex colour "#zz"`, l3),
		fmt.Sprintf(`conveniences_test.go:%d: node "D": Named(" "): want a name`, l3),
	} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("missing %q in:\n%s", sub, err)
		}
	}
	if n := strings.Count(err.Error(), "\n") + 1; n != 3 {
		t.Errorf("got %d errors, want 3:\n%s", n, err)
	}
}

func TestNamedStyleClasses(t *testing.T) {
	d, _ := legendDiagram()
	var svg bytes.Buffer
	if err := d.RenderSVG(&svg); err != nil {
		t.Fatal(err)
	}
	s := svg.String()
	for _, want := range []string{
		`id="a" class="node style-external"`,
		`id="zone" class="group style-trust-zone"`,
		`id="zone/b" class="node style-default style-both"`,
		`id="zone/c" class="node style-default"`,
		`class="edge style-critical-path"`,
		`class="edge style-both"`,
		`class="group legend"`,
		`class="legend-entry style-critical-path"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("SVG lacks %s", want)
		}
	}
	page := renderHTML(t, d)
	if !strings.Contains(page, `class="node style-external"`) || !strings.Contains(page, `class="edge style-critical-path"`) {
		t.Errorf("HTML lacks named style classes")
	}
	// Without Named there are no style classes at all.
	for _, c := range cases {
		var b bytes.Buffer
		if err := c.build().RenderSVG(&b); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(b.String(), "style-") {
			t.Errorf("%s: style class without Named", c.name)
		}
	}
}

// ---- Badges and status dots ----

func badgeDiagram(th *themes.Theme) (*Diagram, *Node, *Node, *Node) {
	d := New("Badges", Theme(th))
	// Top-left of the diagram, so the marks reach past the layout.
	a := d.Node("A", Badge("×3"), Badge("beta"), Status(Green))
	g := d.Column("Group")
	b := g.Node("B", Cylinder, Status(Red), Badge("v2"))
	c := d.Node("C", Circle, Badge("1"))
	d.Connect(a, b, Label("calls"))
	d.Connect(b, c)
	return d, a, b, c
}

func TestBadgeAndStatusGeometry(t *testing.T) {
	d, a, b, c := badgeDiagram(themes.Light)
	res := lay(t, d)
	canvas := layout.Rect{W: res.Width, H: res.Height}

	ba := res.ByEl[a.el]
	if len(ba.Badges) != 2 || ba.Badges[0].Line.Text != "×3" || ba.Badges[1].Line.Text != "beta" {
		t.Fatalf("badges %+v", ba.Badges)
	}
	first, second := ba.Badges[0].Rect, ba.Badges[1].Rect
	sh := ba.Shape
	// The first badge straddles the top-right corner; the next sits to
	// its left on the same line.
	if !(first.X < sh.Right() && first.Right() > sh.Right() && first.Y < sh.Y && first.Bottom() > sh.Y) {
		t.Errorf("first badge %+v doesn't straddle the corner of %+v", first, sh)
	}
	if !near(first.Y, second.Y) || second.Right() > first.X-2 {
		t.Errorf("second badge %+v isn't left of the first %+v", second, first)
	}
	// Its middle is on the rounded corner, just below the top edge.
	if mid := first.Y + first.H/2; mid < sh.Y || mid > sh.Y+3 {
		t.Errorf("badge centre %v, want on the corner by the top edge %v", mid, sh.Y)
	}
	if bg := ba.Badges[0]; bg.Line.X < first.X || bg.Line.X > first.Right() || bg.Line.Y < first.Y || bg.Line.Y > first.Bottom() {
		t.Errorf("badge text %+v outside its pill %+v", bg.Line, first)
	}
	dot := ba.Status
	if dot == nil || math.Abs(dot.CX-sh.X) > 4 || math.Abs(dot.CY-sh.Y) > 4 {
		t.Fatalf("status %+v not on the top-left corner of %+v", dot, sh)
	}
	if tok, _ := dot.Color.Token(); tok != themes.Green {
		t.Errorf("status colour %v", dot.Color)
	}
	// Rings take the colour behind the node: the background at the top
	// level, the group's fill inside it.
	if tok, _ := ba.Ring.Token(); tok != themes.Background {
		t.Errorf("top-level ring %v", ba.Ring)
	}
	bb := res.ByEl[b.el]
	if bb.Ring != res.ByEl[b.el.Parent].Look.Fill {
		t.Errorf("ring in a group %v, want the group's fill", bb.Ring)
	}
	// A cylinder's marks sit on its lid's widest points.
	if !near(bb.Status.CY, bb.Shape.Y+bb.Cap) || !near(bb.Badges[0].Rect.Y+bb.Badges[0].Rect.H/2, bb.Shape.Y+bb.Cap) {
		t.Errorf("cylinder marks %+v %+v, lid at %v", bb.Status, bb.Badges[0].Rect, bb.Shape.Y+bb.Cap)
	}
	// A circle's badge sits on the outline, at 45°.
	bc := res.ByEl[c.el]
	cx, cy, r := bc.Shape.X+bc.Shape.W/2, bc.Shape.Y+bc.Shape.H/2, bc.Shape.W/2
	br := bc.Badges[0].Rect
	if px, py := br.Right()-br.H/2, br.Y+br.H/2; math.Abs(math.Hypot(px-cx, py-cy)-r) > 0.5 {
		t.Errorf("circle badge anchor (%v, %v) is %v from the centre, radius %v", px, py, math.Hypot(px-cx, py-cy), r)
	}
	// Everything stays on the canvas.
	for _, bx := range res.Boxes {
		for _, m := range bx.Marks() {
			if !canvas.Contains(m) {
				t.Errorf("%s: mark %+v outside the canvas %+v", bx.El.Describe(), m, canvas)
			}
		}
	}
	// Edge labels keep off marks.
	for _, e := range res.Edges {
		for _, bx := range res.Boxes {
			for _, m := range bx.Marks() {
				if e.LabelRect.W > 0 && e.LabelRect.Overlaps(m) {
					t.Errorf("label of %s covers a mark of %s", e.E.Describe(), bx.El.Describe())
				}
			}
		}
	}
}

func TestPortsAvoidBadges(t *testing.T) {
	d := New("t")
	c := d.Canvas("")
	// Each upper node lines up with a mark, where the edge would
	// otherwise attach.
	right := c.Node("R", 150, 0, Size(60, 40))
	left := c.Node("L", -120, 82, Size(40, 40))
	n := c.Node("Wide service", 0, 100, Size(200, 60), Badge("beta"), Badge("×3"), Status(Green))
	d.Connect(right, n, Exit(Bottom), Enter(Top))
	d.Connect(left, n, Exit(Right), Enter(Left))
	res := lay(t, d)
	bn := res.ByEl[n.el]
	sh := bn.Shape
	for _, e := range res.Edges {
		p := e.Points[len(e.Points)-1]
		for _, m := range bn.Marks() {
			var under bool
			switch {
			case near(p.Y, sh.Y): // top
				under = p.X >= m.X && p.X <= m.Right()
			case near(p.X, sh.X), near(p.X, sh.Right()): // left, right
				under = p.Y >= m.Y && p.Y <= m.Bottom() && p.X >= m.X && p.X <= m.Right()
			}
			if under {
				t.Errorf("%s enters at %+v, under a mark %+v", e.E.Describe(), p, m)
			}
		}
	}
}

func TestBadgeTextContrast(t *testing.T) {
	lum := func(c color.RGBA) float64 {
		f := func(v uint8) float64 {
			s := float64(v) / 255
			if s <= 0.04045 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
	}
	ratio := func(a, b color.RGBA) float64 {
		x, y := lum(a), lum(b)
		if x < y {
			x, y = y, x
		}
		return (x + 0.05) / (y + 0.05)
	}
	pale := *themes.Light
	pale.Palette[themes.Accent] = color.RGBA{0xf1, 0xf5, 0xf9, 0xff} // close to the white background
	for _, th := range []*themes.Theme{themes.Light, themes.Dark, themes.Blueprint, themes.Auto(themes.Light, themes.Dark), &pale} {
		d, a, _, _ := badgeDiagram(th)
		bg := lay(t, d).ByEl[a.el].Badges[0]
		pals := []*themes.Palette{&th.Palette}
		if th.Dark != nil {
			pals = append(pals, th.Dark)
		}
		for _, p := range pals {
			resolve := func(c Color) color.RGBA {
				if tok, ok := c.Token(); ok {
					return p[tok]
				}
				return c.Resolve(th)
			}
			if r := ratio(resolve(bg.Fill), resolve(bg.Line.Color)); r < 4.5 {
				t.Errorf("%s: badge text contrast %.2f", th.Name, r)
			}
		}
	}
}

func TestMarksInEveryFormat(t *testing.T) {
	d, _, _, _ := badgeDiagram(themes.Light)
	d.Connect(d.Node("D"), d.Node("E"), Named("async", Dashed))
	d.Legend("Key")

	var dio bytes.Buffer
	if err := d.RenderDrawio(&dio); err != nil {
		t.Fatal(err)
	}
	x := dio.String()
	for _, want := range []string{
		`id="a#status" value="" style="ellipse;fillColor=#16a34a;strokeColor=#ffffff;strokeWidth=2;connectable=0;" vertex="1" parent="a"`,
		`id="a#badge-1" value="×3"`,
		`id="a#badge-2" value="beta"`,
		`id="group/b#badge-1" value="v2"`,
		`id="key" value="Key"`,
		`id="key/style-async#sample" value="" style="edgeStyle=none;`,
		`edge="1" parent="key"`,
		`id="key/style-async#name" value="async" style="text;`,
	} {
		if !strings.Contains(x, want) {
			t.Errorf("draw.io lacks %s", want)
		}
	}

	var svg bytes.Buffer
	if err := d.RenderSVG(&svg); err != nil {
		t.Fatal(err)
	}
	if s := svg.String(); !strings.Contains(s, `<g class="marks">`) || !strings.Contains(s, ">beta</text>") {
		t.Errorf("SVG lacks marks")
	}

	res := lay(t, d)
	var buf bytes.Buffer
	if err := d.RenderPNG(&buf, Scale(1)); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	at := func(x, y float64) color.RGBA {
		const m = 24 // default margin
		return color.RGBAModel.Convert(img.At(int(math.Round(x+m)), int(math.Round(y+m)))).(color.RGBA)
	}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	for _, bx := range res.Boxes {
		if dot := bx.Status; dot != nil {
			want := dot.Color.Resolve(themes.Light)
			if got := at(dot.CX, dot.CY); !nearColor(got, want) {
				t.Errorf("%s: dot pixel %v, want %v", bx.El.Describe(), got, want)
			}
		}
		for _, bg := range bx.Badges {
			// The pill's end is filled with the accent colour.
			want := bg.Fill.Resolve(themes.Light)
			if got := at(bg.Rect.X+3, bg.Rect.Y+bg.Rect.H/2); !nearColor(got, want) {
				t.Errorf("%s: badge pixel %v, want %v", bx.El.Describe(), got, want)
			}
		}
	}
	// The legend's edge sample draws a line.
	var lgBox *layout.Box
	for _, bx := range res.Boxes {
		if bx.Sample != nil && bx.Sample.Kind == layout.SampleEdge {
			lgBox = bx
		}
	}
	if lgBox == nil {
		t.Fatal("no edge sample")
	}
	r := lgBox.Shape
	if got := at(r.X+3, r.Y+r.H/2); nearColor(got, white) {
		t.Errorf("edge sample not drawn at %+v", r)
	}
}

func nearColor(a, b color.RGBA) bool {
	d := func(x, y uint8) bool { return math.Abs(float64(x)-float64(y)) <= 24 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B)
}

func TestConveniencesDeterministic(t *testing.T) {
	build := func() *Diagram {
		d, _ := legendDiagram()
		d.Node("Z", Badge("beta"), Status(Yellow))
		return d
	}
	for _, f := range []func(*Diagram, *bytes.Buffer) error{
		func(d *Diagram, b *bytes.Buffer) error { return d.RenderSVG(b) },
		func(d *Diagram, b *bytes.Buffer) error { return d.RenderDrawio(b) },
		func(d *Diagram, b *bytes.Buffer) error { return d.RenderPNG(b) },
	} {
		var a, b bytes.Buffer
		if err := f(build(), &a); err != nil {
			t.Fatal(err)
		}
		if err := f(build(), &b); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Errorf("two renders differ")
		}
	}
}

func TestBadgeErrors(t *testing.T) {
	d := New("t")
	l := line() + 1
	d.Node("A", Badge(""), Status(Color{}), Status(Hex("#zz")))
	d.Node("B", Badge("🚀"))
	err := d.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	for _, sub := range []string{
		fmt.Sprintf(`conveniences_test.go:%d: node "A": Badge(""): want some text`, l),
		fmt.Sprintf(`conveniences_test.go:%d: node "A": Status: want a colour`, l),
		fmt.Sprintf(`conveniences_test.go:%d: node "A": Status: invalid hex colour "#zz"`, l),
		`node "B": no font has a glyph for '🚀'`,
	} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("missing %q in:\n%s", sub, err)
		}
	}
}
