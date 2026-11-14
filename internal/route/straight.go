package route

import (
	"math"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

// straight draws a straight edge from border to border through any
// waypoints. Straight edges between the same pair without waypoints are
// offset from each other by ParallelOffset.
func straight(r *router, j *job) {
	e := j.e
	from, to := e.From.Shape, e.To.Shape
	off := model.Point{}
	if len(e.E.Via) == 0 {
		off = r.straightOffset(j)
	}
	fc := add(from.Center(), off)
	tc := add(to.Center(), off)
	targets := append(append([]model.Point{}, e.E.Via...), tc)
	sources := append([]model.Point{fc}, e.E.Via...)
	start := clip(from, fc, e.E.Exit, targets[0])
	end := clip(to, tc, e.E.Enter, sources[len(sources)-1])
	pts := []model.Point{start}
	pts = append(pts, e.E.Via...)
	pts = append(pts, end)
	e.Points = pts
	e.ExitSide, e.EnterSide = sideOf(from, start), sideOf(to, end)
	e.Rounded = false
	r.addPath(j.i, pts)
}

// straightOffset returns how far to shift this straight edge so it doesn't
// cover other straight edges between the same two boxes.
func (r *router) straightOffset(j *job) model.Point {
	same := func(o *job) bool {
		return o.kind == kindStraight && len(o.e.E.Via) == 0 &&
			(o.e.From == j.e.From && o.e.To == j.e.To || o.e.From == j.e.To && o.e.To == j.e.From)
	}
	n, k := 0, 0
	for _, o := range r.jobs {
		if !same(o) {
			continue
		}
		if o == j {
			k = n
		}
		n++
	}
	if n < 2 {
		return model.Point{}
	}
	// Normal of the line between the canonical (first, second) box order,
	// so edges in either direction share one frame.
	a, b := j.e.From, j.e.To
	if r.boxIdx[a] > r.boxIdx[b] {
		a, b = b, a
	}
	ac, bc := a.Shape.Center(), b.Shape.Center()
	dx, dy := bc.X-ac.X, bc.Y-ac.Y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return model.Point{}
	}
	d := (float64(k) - float64(n-1)/2) * r.t.ParallelOffset
	return model.Point{X: -dy / l * d, Y: dx / l * d}
}

func add(a, b model.Point) model.Point { return model.Point{X: a.X + b.X, Y: a.Y + b.Y} }

// clip returns the midpoint of a pinned side, or the point where the line
// from c (inside r) towards p leaves r.
func clip(r layout.Rect, c model.Point, s model.Side, p model.Point) model.Point {
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
	if dx == 0 && dy == 0 {
		return c
	}
	k := math.Inf(1)
	if dx > 0 {
		k = math.Min(k, (r.Right()-c.X)/dx)
	} else if dx < 0 {
		k = math.Min(k, (r.X-c.X)/dx)
	}
	if dy > 0 {
		k = math.Min(k, (r.Bottom()-c.Y)/dy)
	} else if dy < 0 {
		k = math.Min(k, (r.Y-c.Y)/dy)
	}
	k = math.Max(k, 0)
	return model.Point{X: c.X + dx*k, Y: c.Y + dy*k}
}

func sideOf(r layout.Rect, p model.Point) model.Side {
	const eps = 0.5
	switch {
	case math.Abs(p.Y-r.Y) < eps:
		return model.SideTop
	case math.Abs(p.Y-r.Bottom()) < eps:
		return model.SideBottom
	case math.Abs(p.X-r.X) < eps:
		return model.SideLeft
	}
	return model.SideRight
}

// selfLoop draws a small rectangular loop off one side of the box, between
// the two ports that spreading gave it.
func (r *router) selfLoop(j *job) {
	e := j.e
	a, b := j.selfEnd[0], j.selfEnd[1]
	n := dirOf(a.side)
	out := r.loopOut()
	// Loops on the same side nest outward.
	for _, o := range r.jobs {
		if o == j {
			break
		}
		if o.kind == kindSelf && o.e.From == e.From && o.selfEnd[0] != nil && o.selfEnd[0].side == a.side {
			out += r.t.ParallelOffset
		}
	}
	p0, p1 := a.point, b.point
	q0 := step(p0, n, out)
	q1 := step(p1, n, out)
	// Square the loop off at the outermost of the two (curved outlines).
	switch n {
	case dirRight:
		q0.X = math.Max(q0.X, q1.X)
		q1.X = q0.X
	case dirLeft:
		q0.X = math.Min(q0.X, q1.X)
		q1.X = q0.X
	case dirDown:
		q0.Y = math.Max(q0.Y, q1.Y)
		q1.Y = q0.Y
	case dirUp:
		q0.Y = math.Min(q0.Y, q1.Y)
		q1.Y = q0.Y
	}
	e.Points = []model.Point{p0, q0, q1, p1}
	e.ExitSide, e.EnterSide = a.side, a.side
	e.Rounded = true
	r.addPath(j.i, e.Points)
}

func step(p model.Point, d dir, l float64) model.Point {
	return model.Point{X: p.X + float64(dx[d])*l, Y: p.Y + float64(dy[d])*l}
}
