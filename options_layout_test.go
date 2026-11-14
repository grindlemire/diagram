package diagram

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/icons/generic"
	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/themes"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func tok(c Color) (themes.Token, bool) { return c.Token() }

func TestIconColor(t *testing.T) {
	d := New("t")
	plain := d.Node("Plain", Icon(generic.Server))
	stroked := d.Node("Stroked", Icon(generic.Server), Stroke(Red))
	colored := d.Node("Coloured", Icon(generic.Server), IconColor(Green), Stroke(Red))
	g := d.Column("Group", Icon(generic.Cloud), IconColor(Hex("#ff6600")), Stroke(Blue))
	g.Node("Child")
	res := lay(t, d)
	look := func(e Endpoint) layout.Look { return res.ByEl[e.element()].Look }

	theme := d.m.Theme.IconColor
	if look(plain).IconColor != theme {
		t.Errorf("plain icon colour %v, want theme %v", look(plain).IconColor, theme)
	}
	if look(stroked).IconColor != theme {
		t.Errorf("Stroke recoloured the icon: %v", look(stroked).IconColor)
	}
	if tk, _ := tok(look(stroked).Stroke); tk != themes.Red {
		t.Errorf("Stroke lost from the border")
	}
	if tk, _ := tok(look(colored).IconColor); tk != themes.Green {
		t.Errorf("IconColor(Green) gave %v", look(colored).IconColor)
	}
	if got := look(g).IconColor.Resolve(d.m.Theme); themes.HexString(got) != "#ff6600" {
		t.Errorf("group IconColor gave %s", themes.HexString(got))
	}

	bad := New("t")
	bad.Node("A", IconColor(Hex("#nope")))
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), `node "A": IconColor: invalid hex colour`) {
		t.Errorf("bad IconColor: %v", err)
	}
}

func TestUniformKeepsIntrinsicShapes(t *testing.T) {
	d := New("t")
	r := d.Row("", Uniform)
	circle := r.Node("Circle", Circle)
	box := r.Node("A box with a much longer label", Desc("sets the size"))
	person := r.Node("Person", Person)
	icon := r.Node("Icon only", Icon(generic.Server), IconOnly)
	pill := r.Node("Pill", Pill)
	res := lay(t, d)
	b := func(n *Node) *layout.Box { return res.ByEl[n.el] }

	for _, n := range []*Node{circle, person, icon, pill} {
		if !near(b(n).Rect.W, b(box).Rect.W) || !near(b(n).Rect.H, b(box).Rect.H) {
			t.Errorf("%s slot %vx%v, want %vx%v", n.el.Label, b(n).Rect.W, b(n).Rect.H, b(box).Rect.W, b(box).Rect.H)
		}
		if !b(n).Rect.Contains(b(n).Shape) {
			t.Errorf("%s shape %+v outside slot %+v", n.el.Label, b(n).Shape, b(n).Rect)
		}
	}
	if s := b(box).Shape; s != b(box).Rect {
		t.Errorf("box shape %+v should fill its slot %+v", s, b(box).Rect)
	}
	if s := b(pill).Shape; s != b(pill).Rect {
		t.Errorf("pill shape %+v should fill its slot %+v", s, b(pill).Rect)
	}
	cs := b(circle).Shape
	if !near(cs.W, cs.H) {
		t.Errorf("circle drawn as %vx%v", cs.W, cs.H)
	}
	if c, s := b(circle).Rect.Center(), cs.Center(); !near(c.X, s.X) || !near(c.Y, s.Y) {
		t.Errorf("circle not centred: slot centre %v, shape centre %v", c, s)
	}
	// Person and IconOnly keep the size they have outside Uniform.
	alone := New("t")
	p := alone.Node("Person", Person)
	ic := alone.Node("Icon only", Icon(generic.Server), IconOnly)
	ares := lay(t, alone)
	for _, pair := range [][2]*layout.Box{{b(person), ares.ByEl[p.el]}, {b(icon), ares.ByEl[ic.el]}} {
		got, want := pair[0].Shape, pair[1].Shape
		if !near(got.W, want.W) || !near(got.H, want.H) {
			t.Errorf("%s figure %vx%v under Uniform, %vx%v alone", pair[0].El.Label, got.W, got.H, want.W, want.H)
		}
		// Label centred under the figure.
		if ln := pair[0].Lines[0]; !near(ln.X, got.Center().X) {
			t.Errorf("%s label at x %v, figure centre %v", pair[0].El.Label, ln.X, got.Center().X)
		}
	}
	checkGeometry(t, res)
}

// justified lays out a column stretched by a tall sibling and returns the
// column's content box and its children's rects.
func justified(t *testing.T, opts ...GroupOption) (layout.Rect, []layout.Rect) {
	t.Helper()
	d := New("t")
	d.Node("Tall", Size(80, 400))
	col := d.Column("Column", opts...)
	a, b, c := col.Node("A"), col.Node("B"), col.Node("C")
	res := lay(t, d)
	checkGeometry(t, res)
	var rs []layout.Rect
	for _, n := range []*Node{a, b, c} {
		rs = append(rs, res.ByEl[n.el].Rect)
	}
	return res.ByEl[col.el].Content, rs
}

func TestJustify(t *testing.T) {
	gap := themes.Light.Gap
	content, start := justified(t)
	if !near(start[0].Y, content.Y) || !near(start[1].Y-start[0].Bottom(), gap) {
		t.Errorf("default should be JustifyStart: content %+v, children %+v", content, start)
	}
	_, explicit := justified(t, JustifyStart)
	if explicit[0] != start[0] {
		t.Errorf("JustifyStart %+v differs from the default %+v", explicit[0], start[0])
	}
	content, end := justified(t, JustifyEnd)
	if !near(end[2].Bottom(), content.Bottom()) || !near(end[2].Y-end[1].Bottom(), gap) {
		t.Errorf("JustifyEnd: content %+v, children %+v", content, end)
	}
	content, mid := justified(t, JustifyCenter)
	if top, bottom := mid[0].Y-content.Y, content.Bottom()-mid[2].Bottom(); !near(top, bottom) || top < 1 {
		t.Errorf("JustifyCenter: %v above, %v below", top, bottom)
	}
	content, sb := justified(t, JustifySpaceBetween)
	g1, g2 := sb[1].Y-sb[0].Bottom(), sb[2].Y-sb[1].Bottom()
	if !near(sb[0].Y, content.Y) || !near(sb[2].Bottom(), content.Bottom()) || !near(g1, g2) || g1 <= gap {
		t.Errorf("JustifySpaceBetween: content %+v, children %+v", content, sb)
	}

	// Rows justify horizontally.
	d := New("t", Vertical)
	d.Node("A very wide node that stretches the row below it", MaxWidth(1000))
	row := d.Row("Row", JustifyEnd)
	x := row.Node("X")
	res := lay(t, d)
	if rb, rx := res.ByEl[row.el].Content, res.ByEl[x.el].Rect; !near(rx.Right(), rb.Right()) {
		t.Errorf("row JustifyEnd: content %+v, X %+v", rb, rx)
	}

	bad := New("t")
	bad.Grid("G", 2, JustifyCenter)
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), `grid "G": Justify applies only to a Row or Column`) {
		t.Errorf("Justify on a grid: %v", err)
	}
}

func TestCanvasOverlapIsAnError(t *testing.T) {
	d := New("t")
	c := d.Canvas("Hand placed")
	la := line() + 1
	c.Node("A", 0, 0)
	lb := line() + 1
	c.Node("B", 60, 20)
	c.Node("Clear", 400, 0)
	want := fmt.Sprintf(`options_layout_test.go:%d: node "B" overlaps node "A" (options_layout_test.go:%d) in canvas "Hand placed"`, lb, la)

	err := d.Validate()
	if err == nil || err.Error() != want {
		t.Errorf("Validate:\n got %v\nwant %s", err, want)
	}
	var sb strings.Builder
	if err := d.RenderSVG(&sb); err == nil || err.Error() != want {
		t.Errorf("RenderSVG:\n got %v\nwant %s", err, want)
	}

	ok := New("t")
	oc := ok.Canvas("Stacked", AllowOverlap)
	oc.Node("A", 0, 0)
	oc.Node("B", 60, 20)
	if err := ok.Validate(); err != nil {
		t.Errorf("AllowOverlap: %v", err)
	}
	if err := ok.RenderSVG(&sb); err != nil {
		t.Errorf("AllowOverlap render: %v", err)
	}

	bad := New("t")
	bad.Row("R", AllowOverlap)
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), `row "R": AllowOverlap applies only to a Canvas`) {
		t.Errorf("AllowOverlap on a row: %v", err)
	}
}

func TestLayoutErrorsJoinBuilderErrors(t *testing.T) {
	d := New("t")
	g := d.Grid("G", 2)
	g.Node("Out", 0, 1, Span(1, 2)) // outside the grid: layout must not panic
	c := d.Canvas("C")
	c.Node("A", 0, 0)
	c.Node("B", 10, 10)
	err := d.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	for _, sub := range []string{"outside 2-column grid", `node "B" overlaps node "A"`} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("missing %q in:\n%s", sub, err)
		}
	}
}

func TestCrossGroups(t *testing.T) {
	d := New("t")
	a, b := d.Node("A"), d.Node("B")
	e := d.Connect(a, b, CrossGroups)
	plain := d.Connect(b, a)
	if !e.e.CrossGroups || plain.e.CrossGroups {
		t.Errorf("CrossGroups: %v, plain %v", e.e.CrossGroups, plain.e.CrossGroups)
	}
}

func TestSelfLoopReservesSpace(t *testing.T) {
	d := New("t")
	row := d.Row("", Gap(24))
	a := row.Node("A")
	retry := row.Node("Retry", Pill)
	poller := row.Node("Poller", Circle)
	plain := row.Node("Plain")
	d.Connect(a, a)
	d.Connect(a, a, Label("again"))
	d.Connect(poller, poller, Exit(Top), Label("every 5s"))
	res := lay(t, d)
	checkGeometry(t, res)
	b := func(n *Node) *layout.Box { return res.ByEl[n.el] }
	th := d.m.Theme

	// In a row, unpinned loops go above the node, away from its
	// neighbours on either side.
	ba := b(a)
	if ba.LoopSide != model.SideTop {
		t.Errorf("A loop side %v, want top", ba.LoopSide)
	}
	band := ba.Shape.Y - ba.Rect.Y
	if lh := b(a).Rect.H - b(a).Shape.H; band < 2*(th.EdgeClearance+8)+th.EdgeClearance || !near(band, lh) {
		t.Errorf("A band %v (extent %+v, shape %+v)", band, ba.Rect, ba.Shape)
	}
	if !near(ba.Rect.Bottom(), ba.Shape.Bottom()) {
		t.Errorf("A shape %+v should end at its extent %+v", ba.Shape, ba.Rect)
	}
	// The neighbour starts after A's extent.
	if b(retry).Rect.X < ba.Rect.Right() {
		t.Errorf("Retry %+v inside A's extent %+v", b(retry).Rect, ba.Rect)
	}
	// Text stays centred on the drawn shape.
	if ln := ba.Lines[0]; !near(ln.X, ba.Shape.Center().X) {
		t.Errorf("A label at %v, shape centre %v", ln.X, ba.Shape.Center().X)
	}

	bp := b(poller)
	if bp.LoopSide != model.SideTop {
		t.Errorf("Poller loop side %v, want top", bp.LoopSide)
	}
	if bp.Rect.Y >= bp.Shape.Y || !near(bp.Rect.Bottom(), bp.Shape.Bottom()) {
		t.Errorf("Poller band should be above: extent %+v shape %+v", bp.Rect, bp.Shape)
	}
	if !near(bp.Shape.W, bp.Shape.H) {
		t.Errorf("Poller circle drawn %vx%v", bp.Shape.W, bp.Shape.H)
	}
	if ln := bp.Lines[0]; !near(ln.X, bp.Shape.Center().X) {
		t.Errorf("Poller label at %v, shape centre %v", ln.X, bp.Shape.Center().X)
	}
	if b(plain).LoopSide != model.SideAuto || b(plain).Rect != b(plain).Shape {
		t.Errorf("Plain has a loop band: %+v %+v", b(plain).Rect, b(plain).Shape)
	}
}

func TestSelfLoopBandUnderUniformGridAndCanvas(t *testing.T) {
	d := New("t", Vertical)
	g := d.Grid("G", 2, Uniform)
	a := g.Node("A", 0, 0)
	other := g.Node("Longer label", 0, 1)
	below := g.Node("Below", 1, 0)
	d.Connect(a, a, Exit(Bottom))
	c := d.Canvas("C")
	l := c.Node("Left", 100, 0)
	d.Connect(l, l, Exit(Left))
	res := lay(t, d)
	checkGeometry(t, res)
	ba, bo, bb := res.ByEl[a.el], res.ByEl[other.el], res.ByEl[below.el]
	if !near(ba.Shape.W, bo.Shape.W) || !near(ba.Shape.H, bo.Shape.H) {
		t.Errorf("Uniform: A drawn %vx%v, other %vx%v", ba.Shape.W, ba.Shape.H, bo.Shape.W, bo.Shape.H)
	}
	if ba.Rect.H <= ba.Shape.H || !near(ba.Rect.Y, ba.Shape.Y) {
		t.Errorf("A band should be below: extent %+v shape %+v", ba.Rect, ba.Shape)
	}
	if bb.Rect.Y < ba.Rect.Bottom() {
		t.Errorf("row 1 %+v overlaps A's band %+v", bb.Rect, ba.Rect)
	}
	// A canvas node's coordinates place its drawn box; the band reaches
	// to its left.
	bl, bc := res.ByEl[l.el], res.ByEl[c.el]
	if bl.Rect.X >= bl.Shape.X || !near(bl.Shape.X, bc.Content.X+100) || !near(bl.Rect.Right(), bl.Shape.Right()) {
		t.Errorf("canvas band: extent %+v shape %+v content %+v", bl.Rect, bl.Shape, bc.Content)
	}
}

func TestReadableTextOnLiteralFills(t *testing.T) {
	hexOf := func(t *themes.Theme, c Color) string { return themes.HexString(c.Resolve(t)) }

	dark := New("t", Theme(themes.Dark))
	light := dark.Node("Cache", Fill(Hex("#dcfce7")), Desc("literal fill"), Icon(generic.Server))
	tokenFill := dark.Node("Token", Fill(Surface), Desc("token fill"))
	own := dark.Node("Own", Fill(Hex("#dcfce7")), TextColor(Red))
	res := lay(t, dark)
	bl := res.ByEl[light.el]
	if got := hexOf(themes.Dark, bl.Lines[0].Color); got != "#0f172a" {
		t.Errorf("light literal fill in Dark: label %s, want #0f172a", got)
	}
	if _, isTok := bl.Lines[1].Color.Token(); isTok {
		t.Errorf("description on a literal fill should be a literal, got a token")
	}
	if c := bl.Lines[1].Color.Resolve(themes.Dark); c == bl.Lines[0].Color.Resolve(themes.Dark) {
		t.Errorf("description %s should be muted", themes.HexString(c))
	}
	if _, isTok := bl.Look.IconColor.Token(); isTok {
		t.Errorf("theme icon colour kept on a light fill in Dark")
	}
	bt := res.ByEl[tokenFill.el]
	if tk, ok := bt.Lines[0].Color.Token(); !ok || tk != themes.Foreground {
		t.Errorf("token fill changed the label colour to %v", bt.Lines[0].Color)
	}
	if tk, ok := bt.Lines[1].Color.Token(); !ok || tk != themes.Muted {
		t.Errorf("token fill changed the description colour to %v", bt.Lines[1].Color)
	}
	if tk, ok := res.ByEl[own.el].Lines[0].Color.Token(); !ok || tk != themes.Red {
		t.Errorf("explicit TextColor lost: %v", res.ByEl[own.el].Lines[0].Color)
	}

	lt := New("t")
	navy := lt.Node("Navy", Fill(Hex("#1e293b")), Desc("dark literal"))
	g := lt.Column("Dark group", Fill(Hex("#111111")), Desc("header"))
	g.Node("Inside")
	lres := lay(t, lt)
	if got := hexOf(themes.Light, lres.ByEl[navy.el].Lines[0].Color); got != "#ffffff" {
		t.Errorf("dark literal fill in Light: label %s, want #ffffff", got)
	}
	if got := hexOf(themes.Light, lres.ByEl[g.el].Lines[0].Color); got != "#ffffff" {
		t.Errorf("dark literal group fill: header %s, want #ffffff", got)
	}

	// Under Auto the colours are literals, so both palettes agree.
	auto := New("t", Theme(themes.Auto(themes.Light, themes.Dark)))
	n := auto.Node("Cache", Fill(Hex("#dcfce7")), IconColor(Purple), Icon(generic.Server))
	ares := lay(t, auto)
	ab := ares.ByEl[n.el]
	if _, isTok := ab.Lines[0].Color.Token(); isTok {
		t.Errorf("Auto: label colour is a token")
	}
	if tk, _ := ab.Look.IconColor.Token(); tk != themes.Purple {
		t.Errorf("explicit IconColor lost: %v", ab.Look.IconColor)
	}
}

// TestLoopedNodesStayInLine checks that a loop band reserves space without
// moving a node's drawn body out of line with its siblings.
func TestLoopedNodesStayInLine(t *testing.T) {
	for _, side := range []Side{Top, Right, Bottom, Left} {
		t.Run(fmt.Sprint(side), func(t *testing.T) {
			d := New("t", Vertical)
			row := d.Row("Row")
			ra := row.Node("A")
			rl := row.Node("Looped")
			rc := row.Node("C", Desc("taller"))
			d.Connect(rl, rl, Exit(side), Label("retry"))
			col := d.Column("Column")
			ca := col.Node("A wide node in the column")
			cl := col.Node("Looped")
			cc := col.Node("C")
			d.Connect(cl, cl, Exit(side), Label("again"))
			g := d.Grid("Grid", 2)
			ga := g.Node("A", 0, 0)
			gl := g.Node("Looped", 0, 1)
			gb := g.Node("Below", 1, 1, Desc("wider than looped"))
			d.Connect(gl, gl, Exit(side))
			res := lay(t, d)
			checkGeometry(t, res)
			s := func(n *Node) layout.Rect { return res.ByEl[n.el].Shape }
			for _, n := range []*Node{ra, rc} {
				if !near(s(n).Center().Y, s(rl).Center().Y) {
					t.Errorf("row: %s centre y %v, looped %v", n.el.Label, s(n).Center().Y, s(rl).Center().Y)
				}
			}
			for _, n := range []*Node{ca, cc} {
				if !near(s(n).Center().X, s(cl).Center().X) {
					t.Errorf("column: %s centre x %v, looped %v", n.el.Label, s(n).Center().X, s(cl).Center().X)
				}
			}
			if !near(s(ga).Center().Y, s(gl).Center().Y) || !near(s(gb).Center().X, s(gl).Center().X) {
				t.Errorf("grid: A %v, looped %v, below %v", s(ga).Center(), s(gl).Center(), s(gb).Center())
			}
			// The band is real space: the extent stays clear of every
			// sibling and inside the container's content.
			for _, n := range []*Node{rl, cl, gl} {
				b := res.ByEl[n.el]
				if b.Rect == b.Shape {
					t.Errorf("%s has no band", n.el.Describe())
				}
			}
		})
	}
}

func TestLoopBandAlignStart(t *testing.T) {
	d := New("t")
	row := d.Row("Row", AlignStart)
	a := row.Node("A", Desc("taller"))
	l := row.Node("Looped")
	d.Connect(l, l, Exit(Top))
	res := lay(t, d)
	checkGeometry(t, res)
	sa, sl := res.ByEl[a.el].Shape, res.ByEl[l.el].Shape
	if !near(sa.Y, sl.Y) {
		t.Errorf("AlignStart: A top %v, looped top %v", sa.Y, sl.Y)
	}
	if rl := res.ByEl[l.el].Rect; rl.Y >= sl.Y {
		t.Errorf("band above the looped node missing: extent %+v shape %+v", rl, sl)
	}
}
