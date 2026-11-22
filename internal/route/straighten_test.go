package route

import (
	"testing"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/themes"
)

// routed lays out and routes two nodes a and b with one edge a→b leaving
// and entering by the given sides, and returns the router and the edge's
// job with its own segments taken out of the routing state, ready for a
// pass to be run on a hand-made route.
func routed(t *testing.T, a, b *model.Element, exit, enter model.Side) (*router, *job) {
	t.Helper()
	d := model.New("test")
	c := &model.Element{Kind: model.KindCanvas, Parent: d.Root, Diagram: d}
	d.Root.Children = append(d.Root.Children, c)
	for _, n := range []*model.Element{a, b} {
		n.Kind, n.Parent, n.Diagram = model.KindNode, c, d
		c.Children = append(c.Children, n)
	}
	d.Edges = append(d.Edges, &model.Edge{From: a, To: b, Exit: exit, Enter: enter, Src: model.Src{File: "x.go", Line: 1}})
	res, _ := layout.Layout(d, themes.Light)
	s := Start(res)
	if err := s.Finish(); err != nil {
		t.Fatalf("Route: %v", err)
	}
	j := s.r.jobs[0]
	s.r.removeEdge(j.i)
	return s.r, j
}

// centre moves en's port to the middle of its side.
func (r *router) centre(en *end) {
	lo, hi := r.sideSpan(en.box, en.side)
	en.pos = (lo + hi) / 2
	en.point = r.portAt(en.box, en.side, en.pos)
}

func node(label string, shape model.Shape, x, y float64) *model.Element {
	return &model.Element{Label: label, Shape: shape, X: x, Y: y, Width: model.Some(100.0), Height: model.Some(60.0)}
}

// Flattening a step must not cut the arrow's straight run short: with the
// source's port already at the end of its side, the only move left would
// leave the arrowhead no room, so the step stays.
func TestStraightenKeepsArrowStub(t *testing.T) {
	r, j := routed(t, node("A", model.ShapeBox, 0, 0), node("B", model.ShapeBox, 200, 18), model.SideRight, model.SideTop)
	lo, _ := r.sideSpan(j.src.box, j.src.side)
	j.src.pos, j.src.point = lo, r.portAt(j.src.box, j.src.side, lo)
	r.centre(j.dst)
	p, q := j.src.point, j.dst.point
	pts := []model.Point{p, {X: p.X + 50, Y: p.Y}, {X: p.X + 50, Y: p.Y - 14}, {X: q.X, Y: p.Y - 14}, q}
	out := r.straighten(j, pts)
	n := len(out)
	want := r.stubLen(j, q, j.dst.travel, j.dst.box, j.dst.side, r.wantStub(j, false))
	if got := seglen(out[n-2], out[n-1]); got < want-0.5 {
		t.Errorf("straighten(%v) = %v: last leg %.1f, want at least %.1f", pts, out, got, want)
	}
}

// A small step between two nodes slightly out of line is removed by
// sliding a port, also along a cylinder's curved top or bottom, where the
// port moves along its stub as it slides.
func TestStraightenSlidesPortsOnCurvedSides(t *testing.T) {
	r, j := routed(t, node("A", model.ShapeCylinder, 0, 0), node("B", model.ShapeCylinder, 4, 200), model.SideBottom, model.SideTop)
	r.centre(j.src)
	r.centre(j.dst)
	p, q := j.src.point, j.dst.point
	mid := (p.Y + q.Y) / 2
	pts := []model.Point{p, {X: p.X, Y: mid}, {X: q.X, Y: mid}, q}
	out := r.straighten(j, pts)
	if len(out) != 2 {
		t.Fatalf("straighten(%v) = %v, want a straight line", pts, out)
	}
	if out[0].X != out[1].X {
		t.Errorf("straighten(%v) = %v, not vertical", pts, out)
	}
	// The moved port sits on the outline.
	for _, en := range []*end{j.src, j.dst} {
		if got := r.portAt(en.box, en.side, en.pos); !near(got, en.point) {
			t.Errorf("port at %v, outline at %v", en.point, got)
		}
	}
}

// A port under a figure's text starts inside the text's margin: the
// segment leaving it doesn't count as running into the text.
func TestMeasurePathLetsPortsLeaveTheirOwnText(t *testing.T) {
	person := &model.Element{Label: "Customer", Desc: "Buys things", Shape: model.ShapePerson}
	r, j := routed(t, person, node("B", model.ShapeBox, 0, 200), model.SideBottom, model.SideTop)
	if m := r.measurePath(j, j.e.Points); !m.ok {
		t.Errorf("measurePath(%v) is not ok", j.e.Points)
	}
}
