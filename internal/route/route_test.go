package route_test

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/route"
	"github.com/grindlemire/diagram/themes"
)

// builder makes model trees directly. Everything sits on an invisible
// canvas at the origin, so element coordinates are diagram coordinates.
type builder struct {
	d    *model.Diagram
	root *model.Element
	line int
}

func newBuilder() *builder {
	d := model.New("test")
	c := &model.Element{Kind: model.KindCanvas, Parent: d.Root, Diagram: d}
	d.Root.Children = append(d.Root.Children, c)
	return &builder{d: d, root: c}
}

func (b *builder) add(parent *model.Element, e *model.Element) *model.Element {
	if parent == nil {
		parent = b.root
	}
	e.Parent, e.Diagram = parent, b.d
	parent.Children = append(parent.Children, e)
	return e
}

// node adds a fixed-size box at (x, y) in its parent.
func (b *builder) node(parent *model.Element, label string, x, y float64) *model.Element {
	return b.add(parent, &model.Element{Kind: model.KindNode, Label: label, X: x, Y: y,
		Width: model.Some(100.0), Height: model.Some(40.0)})
}

// group adds a labelled canvas at (x, y) in its parent.
func (b *builder) group(parent *model.Element, label string, x, y float64) *model.Element {
	return b.add(parent, &model.Element{Kind: model.KindCanvas, Label: label, X: x, Y: y})
}

func (b *builder) edge(from, to *model.Element, opts ...func(*model.Edge)) *model.Edge {
	b.line++
	e := &model.Edge{From: from, To: to, Src: model.Src{File: "x.go", Line: b.line}}
	for _, o := range opts {
		o(e)
	}
	b.d.Edges = append(b.d.Edges, e)
	return e
}

func exit(s model.Side) func(*model.Edge)  { return func(e *model.Edge) { e.Exit = s } }
func enter(s model.Side) func(*model.Edge) { return func(e *model.Edge) { e.Enter = s } }
func label(s string) func(*model.Edge)     { return func(e *model.Edge) { e.Label = s } }
func via(p ...model.Point) func(*model.Edge) {
	return func(e *model.Edge) { e.Via = append(e.Via, p...) }
}
func straightEdge(e *model.Edge) { e.Straight = true }

func (b *builder) route(t *testing.T) *layout.Result {
	t.Helper()
	res, _ := layout.Layout(b.d, themes.Light)
	if err := route.Route(res); err != nil {
		t.Fatalf("Route: %v", err)
	}
	return res
}

const eps = 0.01

// checkEdge asserts the geometry every orthogonal edge must have.
func checkEdge(t *testing.T, res *layout.Result, e *layout.Edge) {
	t.Helper()
	name := e.E.Describe()
	pts := e.Points
	if len(pts) < 2 {
		t.Fatalf("%s: %d points", name, len(pts))
	}
	if !e.Rounded {
		t.Errorf("%s: not rounded", name)
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		if math.Abs(a.X-b.X) > eps && math.Abs(a.Y-b.Y) > eps {
			t.Errorf("%s: segment %d %v→%v is not axis-aligned", name, i, a, b)
		}
		for _, n := range res.Boxes {
			if n.El.Kind != model.KindNode {
				continue
			}
			if crosses(n.Shape, a, b) {
				t.Errorf("%s: segment %d %v→%v crosses %s", name, i, a, b, n.El.Describe())
			}
		}
	}
	if e.From == e.To {
		return
	}
	// An edge between a group and something inside it meets the group's
	// border from the inside.
	checkPort(t, name+" exit", e.From.Shape, e.ExitSide, pts[0], pts[1], inside(e.To.El, e.From.El))
	checkPort(t, name+" entry", e.To.Shape, e.EnterSide, pts[len(pts)-1], pts[len(pts)-2], inside(e.From.El, e.To.El))
}

func inside(e, g *model.Element) bool {
	for x := e.Parent; x != nil; x = x.Parent {
		if x == g {
			return true
		}
	}
	return false
}

// checkPort asserts that p is on side s of r and that the segment to q
// is perpendicular to it, on the outside (or the inside when in is set).
func checkPort(t *testing.T, name string, r layout.Rect, s model.Side, p, q model.Point, in bool) {
	t.Helper()
	if in {
		q = model.Point{X: 2*p.X - q.X, Y: 2*p.Y - q.Y}
	}
	var onSide, perp bool
	switch s {
	case model.SideTop:
		onSide = math.Abs(p.Y-r.Y) < eps && p.X > r.X && p.X < r.Right()
		perp = math.Abs(p.X-q.X) < eps && q.Y < p.Y
	case model.SideBottom:
		onSide = math.Abs(p.Y-r.Bottom()) < eps && p.X > r.X && p.X < r.Right()
		perp = math.Abs(p.X-q.X) < eps && q.Y > p.Y
	case model.SideLeft:
		onSide = math.Abs(p.X-r.X) < eps && p.Y > r.Y && p.Y < r.Bottom()
		perp = math.Abs(p.Y-q.Y) < eps && q.X < p.X
	case model.SideRight:
		onSide = math.Abs(p.X-r.Right()) < eps && p.Y > r.Y && p.Y < r.Bottom()
		perp = math.Abs(p.Y-q.Y) < eps && q.X > p.X
	default:
		t.Errorf("%s: side %v", name, s)
		return
	}
	if !onSide {
		t.Errorf("%s: %v is not on the %v side of %v", name, p, s, r)
	}
	if !perp {
		t.Errorf("%s: segment %v→%v is not perpendicular to the %v side", name, p, q, s)
	}
}

// crosses reports whether segment ab passes through r's interior.
func crosses(r layout.Rect, a, b model.Point) bool {
	in := layout.Rect{X: r.X + eps, Y: r.Y + eps, W: r.W - 2*eps, H: r.H - 2*eps}
	x0, x1 := math.Min(a.X, b.X), math.Max(a.X, b.X)
	y0, y1 := math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)
	return x0 < in.Right() && x1 > in.X && y0 < in.Bottom() && y1 > in.Y
}

func checkAll(t *testing.T, res *layout.Result) {
	t.Helper()
	for _, e := range res.Edges {
		if !e.E.Straight {
			checkEdge(t, res, e)
		}
	}
}

func TestAboveBelowUsesBottomAndTop(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	c := b.node(nil, "B", 0, 120)
	b.edge(a, c)
	res := b.route(t)
	checkAll(t, res)
	e := res.Edges[0]
	if e.ExitSide != model.SideBottom || e.EnterSide != model.SideTop {
		t.Errorf("sides %v→%v, want bottom→top", e.ExitSide, e.EnterSide)
	}
	if len(e.Points) != 2 {
		t.Errorf("aligned boxes: got %v, want one straight segment", e.Points)
	}
}

func TestSideBySideUsesFacingSides(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	c := b.node(nil, "B", 200, 10)
	b.edge(a, c)
	res := b.route(t)
	checkAll(t, res)
	e := res.Edges[0]
	if e.ExitSide != model.SideRight || e.EnterSide != model.SideLeft {
		t.Errorf("sides %v→%v, want right→left", e.ExitSide, e.EnterSide)
	}
	if len(e.Points) != 2 {
		t.Errorf("overlapping spans: got %v, want one straight segment", e.Points)
	}
}

func TestAvoidsNodes(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	b.node(nil, "Wall", 200, -40)
	b.node(nil, "Wall 2", 200, 40)
	c := b.node(nil, "B", 400, 0)
	b.edge(a, c)
	b.edge(c, a)
	checkAll(t, b.route(t))
}

func TestAvoidsUnrelatedGroups(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 100)
	g := b.group(nil, "Group", 200, 0)
	b.node(g, "Inside", 0, 0)
	b.node(g, "Inside 2", 0, 160)
	c := b.node(nil, "B", 500, 100)
	b.edge(a, c)
	res := b.route(t)
	checkAll(t, res)
	gr := res.ByEl[g].Rect
	for _, e := range res.Edges {
		for i := 0; i+1 < len(e.Points); i++ {
			if crosses(gr, e.Points[i], e.Points[i+1]) {
				t.Errorf("%s enters %v: %v", e.E.Describe(), gr, e.Points)
			}
		}
	}
}

func TestEntersOnlyGroupsOfItsEnds(t *testing.T) {
	b := newBuilder()
	g := b.group(nil, "Left", 0, 0)
	a := b.node(g, "A", 0, 0)
	h := b.group(nil, "Right", 300, 0)
	c := b.node(h, "B", 0, 0)
	other := b.group(nil, "Other", 150, 200)
	b.node(other, "X", 0, 0)
	b.edge(a, c)
	res := b.route(t)
	checkAll(t, res)
	or := res.ByEl[other].Rect
	e := res.Edges[0]
	for i := 0; i+1 < len(e.Points); i++ {
		if crosses(or, e.Points[i], e.Points[i+1]) {
			t.Errorf("edge enters unrelated group: %v", e.Points)
		}
	}
}

func TestPinnedPorts(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	c := b.node(nil, "B", 300, 0)
	b.edge(a, c, exit(model.SideTop), enter(model.SideBottom))
	b.edge(a, c, exit(model.SideBottom), enter(model.SideRight))
	b.edge(a, c, exit(model.SideLeft))
	res := b.route(t)
	checkAll(t, res)
	want := [][2]model.Side{
		{model.SideTop, model.SideBottom},
		{model.SideBottom, model.SideRight},
		{model.SideLeft, 0},
	}
	for i, e := range res.Edges {
		if e.ExitSide != want[i][0] || want[i][1] != 0 && e.EnterSide != want[i][1] {
			t.Errorf("edge %d: sides %v→%v, want %v→%v", i, e.ExitSide, e.EnterSide, want[i][0], want[i][1])
		}
	}
}

func TestWaypointsVisitedInOrder(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	c := b.node(nil, "B", 400, 0)
	vias := []model.Point{{X: 150, Y: 150}, {X: 300, Y: -100}, {X: 250, Y: 200}}
	b.edge(a, c, via(vias...))
	res := b.route(t)
	checkAll(t, res)
	pts := res.Edges[0].Points
	// Walk the polyline, consuming waypoints as they appear.
	k := 0
	for i := 0; i+1 < len(pts) && k < len(vias); i++ {
		for k < len(vias) && onSegment(vias[k], pts[i], pts[i+1]) {
			k++
		}
	}
	if k != len(vias) {
		t.Errorf("visited %d of %d waypoints in order: %v", k, len(vias), pts)
	}
}

func onSegment(p, a, b model.Point) bool {
	return p.X >= math.Min(a.X, b.X)-eps && p.X <= math.Max(a.X, b.X)+eps &&
		p.Y >= math.Min(a.Y, b.Y)-eps && p.Y <= math.Max(a.Y, b.Y)+eps &&
		(math.Abs(a.X-b.X) < eps || math.Abs(a.Y-b.Y) < eps)
}

func TestParallelEdgesDontOverlap(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	c := b.node(nil, "B", 250, 150)
	b.edge(a, c)
	b.edge(a, c)
	b.edge(c, a)
	res := b.route(t)
	checkAll(t, res)
	for i, e := range res.Edges {
		for _, f := range res.Edges[i+1:] {
			if p, q, ok := overlapping(e.Points, f.Points); ok {
				t.Errorf("%v and %v overlap at %v–%v", e.Points, f.Points, p, q)
			}
		}
	}
	// Ports on a shared side are at least PortSpacing apart.
	p, q := res.Edges[0].Points[0], res.Edges[1].Points[0]
	if d := math.Hypot(p.X-q.X, p.Y-q.Y); res.Edges[0].ExitSide == res.Edges[1].ExitSide && d < themes.Light.PortSpacing-eps {
		t.Errorf("ports %v apart, want at least %v", d, themes.Light.PortSpacing)
	}
}

// overlapping reports whether two polylines share a collinear stretch
// longer than a pixel.
func overlapping(a, b []model.Point) (model.Point, model.Point, bool) {
	for i := 0; i+1 < len(a); i++ {
		for j := 0; j+1 < len(b); j++ {
			p, q, r, s := a[i], a[i+1], b[j], b[j+1]
			switch {
			case math.Abs(p.X-q.X) < eps && math.Abs(r.X-s.X) < eps && math.Abs(p.X-r.X) < 1:
				lo := math.Max(math.Min(p.Y, q.Y), math.Min(r.Y, s.Y))
				hi := math.Min(math.Max(p.Y, q.Y), math.Max(r.Y, s.Y))
				if hi-lo > 1 {
					return model.Point{X: p.X, Y: lo}, model.Point{X: p.X, Y: hi}, true
				}
			case math.Abs(p.Y-q.Y) < eps && math.Abs(r.Y-s.Y) < eps && math.Abs(p.Y-r.Y) < 1:
				lo := math.Max(math.Min(p.X, q.X), math.Min(r.X, s.X))
				hi := math.Min(math.Max(p.X, q.X), math.Max(r.X, s.X))
				if hi-lo > 1 {
					return model.Point{X: lo, Y: p.Y}, model.Point{X: hi, Y: p.Y}, true
				}
			}
		}
	}
	return model.Point{}, model.Point{}, false
}

func TestSharedSideSpreadsWithoutCrossing(t *testing.T) {
	b := newBuilder()
	hub := b.node(nil, "Hub", 195, 0)
	var leaves []*model.Element
	for i := range 4 {
		leaves = append(leaves, b.node(nil, fmt.Sprint("Leaf ", i), float64(i)*130, 160))
	}
	for _, l := range leaves {
		b.edge(hub, l)
	}
	res := b.route(t)
	checkAll(t, res)
	// The middle leaves hang off the bottom; the outer ones may use a side.
	var xs []float64
	for i, e := range res.Edges {
		if (i == 1 || i == 2) && e.ExitSide != model.SideBottom {
			t.Errorf("%s exits %v, want bottom", e.E.Describe(), e.ExitSide)
		}
		if e.ExitSide == model.SideBottom {
			xs = append(xs, e.Points[0].X)
		}
		if len(e.Points) > 4 {
			t.Errorf("%s: %d bends, want at most 2", e.E.Describe(), len(e.Points)-2)
		}
	}
	for i := 1; i < len(xs); i++ {
		if xs[i]-xs[i-1] < themes.Light.PortSpacing-eps {
			t.Errorf("ports not ordered left to right with spacing: %v", xs)
		}
	}
	for i, e := range res.Edges {
		for _, f := range res.Edges[i+1:] {
			if n := crossings(e.Points, f.Points); n > 0 {
				t.Errorf("%s and %s cross", e.E.Describe(), f.E.Describe())
			}
		}
	}
}

func crossings(a, b []model.Point) int {
	n := 0
	for i := 0; i+1 < len(a); i++ {
		for j := 0; j+1 < len(b); j++ {
			p, q, r, s := a[i], a[i+1], b[j], b[j+1]
			if math.Abs(p.Y-q.Y) < eps { // a horizontal
				p, q, r, s = r, s, p, q
			}
			// p-q vertical, r-s horizontal
			if math.Abs(p.X-q.X) > eps || math.Abs(r.Y-s.Y) > eps {
				continue
			}
			if p.X > math.Min(r.X, s.X)+eps && p.X < math.Max(r.X, s.X)-eps &&
				r.Y > math.Min(p.Y, q.Y)+eps && r.Y < math.Max(p.Y, q.Y)-eps {
				n++
			}
		}
	}
	return n
}

func TestLabelsAvoidNodesAndEachOther(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	c := b.node(nil, "B", 0, 100)
	d := b.node(nil, "C", 200, 0)
	b.edge(a, c, label("a fairly long label"))
	b.edge(a, d, label("second"))
	b.edge(c, d, label("third label"))
	b.edge(d, a, label("back"), func(e *model.Edge) { e.LabelAt = model.LabelEnd })
	res := b.route(t)
	checkAll(t, res)
	for i, e := range res.Edges {
		if e.LabelRect.W == 0 {
			t.Fatalf("%s: no label placed", e.E.Describe())
		}
		for _, n := range res.Boxes {
			if n.El.Kind == model.KindNode && e.LabelRect.Overlaps(n.Shape) {
				t.Errorf("%s: label %v overlaps %s", e.E.Describe(), e.LabelRect, n.El.Describe())
			}
		}
		for _, f := range res.Edges[i+1:] {
			if e.LabelRect.Overlaps(f.LabelRect) {
				t.Errorf("labels of %s and %s overlap", e.E.Describe(), f.E.Describe())
			}
		}
	}
}

func TestStraightEdges(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	c := b.node(nil, "B", 300, 200)
	b.edge(a, c, straightEdge)
	b.edge(a, c, straightEdge)
	res := b.route(t)
	e, f := res.Edges[0], res.Edges[1]
	if e.Rounded || len(e.Points) != 2 {
		t.Errorf("straight edge: rounded=%v points=%v", e.Rounded, e.Points)
	}
	if reflect.DeepEqual(e.Points, f.Points) {
		t.Errorf("parallel straight edges coincide: %v", e.Points)
	}
}

func TestSelfLoop(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	b.edge(a, a)
	b.edge(a, a, exit(model.SideTop), label("retry"))
	res := b.route(t)
	checkAll(t, res)
	// The pinned loop decides the node's loop side; the other joins it.
	for i, side := range []model.Side{model.SideTop, model.SideTop} {
		e := res.Edges[i]
		if len(e.Points) != 4 || e.ExitSide != side || e.EnterSide != side {
			t.Errorf("loop %d: sides %v/%v points %v, want a 4-point loop on %v", i, e.ExitSide, e.EnterSide, e.Points, side)
		}
		checkPort(t, "loop exit", e.From.Shape, side, e.Points[0], e.Points[1], false)
		checkPort(t, "loop entry", e.From.Shape, side, e.Points[3], e.Points[2], false)
	}
}

func TestGroupEndpointAttachesToBorder(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 50)
	g := b.group(nil, "Group", 250, 0)
	inner := b.node(g, "Inner", 0, 0)
	b.edge(a, g)
	b.edge(g, inner)
	res := b.route(t)
	checkAll(t, res)
	gr := res.ByEl[g].Rect
	e := res.Edges[0]
	end := e.Points[len(e.Points)-1]
	if e.EnterSide != model.SideLeft || math.Abs(end.X-gr.X) > eps {
		t.Errorf("edge to group ends at %v on %v, want the left border of %v", end, e.EnterSide, gr)
	}
}

func TestDeterministic(t *testing.T) {
	build := func() *layout.Result {
		b := newBuilder()
		var ns []*model.Element
		for i := range 9 {
			ns = append(ns, b.node(nil, fmt.Sprint("N", i), float64(i%3)*180, float64(i/3)*110))
		}
		for i := range ns {
			for k := i + 2; k < len(ns); k += 3 {
				b.edge(ns[i], ns[k], label(fmt.Sprint(i, "-", k)))
			}
		}
		return b.route(t)
	}
	r1, r2 := build(), build()
	checkAll(t, r1)
	for i := range r1.Edges {
		a, c := r1.Edges[i], r2.Edges[i]
		if !reflect.DeepEqual(a.Points, c.Points) || a.LabelRect != c.LabelRect {
			t.Fatalf("edge %d differs between runs:\n%v\n%v", i, a.Points, c.Points)
		}
	}
}

func TestImpossiblePinIsAnError(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 100)
	// A wide box sitting right on top of A boxes in its top side.
	b.add(nil, &model.Element{Kind: model.KindNode, Label: "Lid", X: -50, Y: 60,
		Width: model.Some(200.0), Height: model.Some(40.0)})
	c := b.node(nil, "C", 300, 100)
	b.edge(a, c)
	b.edge(a, c, exit(model.SideTop))
	res, _ := layout.Layout(b.d, themes.Light)
	err := route.Route(res)
	if err == nil {
		t.Fatal("no error for a boxed-in exit")
	}
	want := "x.go:2: edge A → C: no route from exit top"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q, want it to contain %q", err, want)
	}
	if strings.Contains(err.Error(), "x.go:1:") {
		t.Errorf("error mentions the routable edge: %q", err)
	}
}

func TestWaypointInsideNodeIsAnError(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	b.node(nil, "Block", 200, 0)
	c := b.node(nil, "C", 400, 0)
	b.edge(a, c, via(model.Point{X: 250, Y: 20}))
	res, _ := layout.Layout(b.d, themes.Light)
	err := route.Route(res)
	if err == nil || !strings.Contains(err.Error(), `x.go:1: edge A → C: waypoint (250, 20) is inside node "Block"`) {
		t.Errorf("got %v", err)
	}
}

func TestCongestionDegradesInsteadOfFailing(t *testing.T) {
	// B is walled in with gaps smaller than the clearance; the route has
	// to squeeze through one.
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	c := b.node(nil, "B", 300, 100)
	for i, p := range []model.Point{{X: 300, Y: 50}, {X: 300, Y: 150}, {X: 195, Y: 100}, {X: 405, Y: 100}} {
		b.node(nil, fmt.Sprint("W", i), p.X, p.Y)
	}
	b.edge(a, c)
	checkAll(t, b.route(t))
}

func crossGroups(e *model.Edge) { e.CrossGroups = true }

func TestCrossGroupsPassesThroughForeignGroups(t *testing.T) {
	for _, cross := range []bool{false, true} {
		b := newBuilder()
		g := b.group(nil, "Wall", 200, 0)
		b.node(g, "X", 0, 0)
		b.node(g, "Y", 0, 300)
		a := b.node(nil, "A", 0, 210)
		c := b.node(nil, "B", 600, 210)
		if cross {
			b.edge(a, c, crossGroups)
		} else {
			b.edge(a, c)
		}
		res := b.route(t)
		checkAll(t, res)
		wall := res.ByEl[g].Rect
		pts := res.Edges[0].Points
		through := false
		for i := 0; i+1 < len(pts); i++ {
			through = through || crosses(wall, pts[i], pts[i+1])
		}
		switch {
		case cross && (!through || len(pts) != 2):
			t.Errorf("CrossGroups: want a straight route through the group, got %v", pts)
		case !cross && through:
			t.Errorf("without CrossGroups: route %v enters the group %v", pts, wall)
		}
	}
}

func TestSelfLoopsStayInTheirBand(t *testing.T) {
	b := newBuilder()
	a := b.node(nil, "A", 0, 0)
	o := b.node(nil, "B", 300, 0)
	b.edge(a, a)
	b.edge(a, a, label("retry"))
	b.edge(a, o)
	res := b.route(t)
	checkAll(t, res)
	box := res.ByEl[a]
	if box.LoopSide == model.SideAuto {
		t.Fatal("layout reserved no side for the loops")
	}
	var far []float64
	for _, e := range res.Edges[:2] {
		if e.ExitSide != box.LoopSide || e.EnterSide != box.LoopSide {
			t.Errorf("loop on %v/%v, want %v", e.ExitSide, e.EnterSide, box.LoopSide)
		}
		for _, p := range e.Points {
			if !contains(box.Rect, p) {
				t.Errorf("loop point %v outside the node's extent %v", p, box.Rect)
			}
		}
		far = append(far, e.Points[1].X)
		if e.LabelRect.W > 0 && !box.Rect.Contains(e.LabelRect) {
			t.Errorf("loop label %v outside the node's extent %v", e.LabelRect, box.Rect)
		}
	}
	if box.LoopSide == model.SideRight && !(far[1] > far[0]) {
		t.Errorf("second loop at %v doesn't nest outside the first at %v", far[1], far[0])
	}
	// The other edge keeps out of the band.
	band := layout.Rect{X: box.Shape.Right(), Y: box.Rect.Y, W: box.Rect.Right() - box.Shape.Right(), H: box.Rect.H}
	pts := res.Edges[2].Points
	for i := 0; i+1 < len(pts); i++ {
		if box.LoopSide == model.SideRight && crosses(band, pts[i], pts[i+1]) {
			t.Errorf("edge segment %v→%v runs through the loop band %v", pts[i], pts[i+1], band)
		}
	}
}

func contains(r layout.Rect, p model.Point) bool {
	return p.X >= r.X-eps && p.X <= r.Right()+eps && p.Y >= r.Y-eps && p.Y <= r.Bottom()+eps
}

func TestBottomPortsSitBelowFigureText(t *testing.T) {
	b := newBuilder()
	p := b.add(nil, &model.Element{Kind: model.KindNode, Label: "Customer", Desc: "Buys things", Shape: model.ShapePerson, X: 0, Y: 0})
	c := b.node(nil, "Shop", 0, 200)
	b.edge(c, p, exit(model.SideTop), enter(model.SideBottom))
	res := b.route(t)
	box := res.ByEl[p]
	textBottom := 0.0
	for _, ln := range box.Lines {
		textBottom = math.Max(textBottom, ln.Y)
	}
	pts := res.Edges[0].Points
	end := pts[len(pts)-1]
	if end.Y <= textBottom {
		t.Errorf("edge ends at %v, above the bottom of the person's text (baseline %v)", end, textBottom)
	}
	if res.Edges[0].EnterSide != model.SideBottom {
		t.Errorf("enters on %v, want bottom", res.Edges[0].EnterSide)
	}
}
