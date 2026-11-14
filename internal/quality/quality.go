// Package quality measures how good a diagram's edge routes are: bends,
// crossings, detours, routes through things they should avoid, label
// collisions and excursions outside the layout. It is a review and
// regression tool; nothing in the rendering pipeline uses it.
package quality

import (
	"math"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

// Edge holds the metrics of one routed edge.
type Edge struct {
	ID        string
	Bends     int
	Crossings int // proper crossings with other edges
	// Length is the route's length and Manhattan the Manhattan distance
	// between its two ports. Ratio is Length / Manhattan, or 0 for
	// self-loops and edges whose ports coincide.
	Length, Manhattan, Ratio float64
	// Through counts segments passing through a node that isn't one of
	// the edge's ends, through text outside a node's shape (a Person's or
	// IconOnly node's label, including its own ends'), or through a group
	// header.
	Through int
	// Label collisions: LabelOverNode counts the node shapes the label's
	// pill overlaps other than the edge's own ends, LabelOverOwnEnd the
	// ends it spills over, LabelOverText the blocks of text or icons (a
	// node's label and icon, text under a figure, a group header), and
	// LabelOverlaps the other labels and other edges' routes.
	LabelOverNode, LabelOverOwnEnd, LabelOverText, LabelOverlaps int
	// Detached is 1 when the label sits more than 4px from its edge.
	Detached int
	// Jogs counts S-bends: two consecutive bends turning opposite ways
	// around a segment shorter than twice the edge clearance. Edges with
	// waypoints aren't counted.
	Jogs int
	// Shared is the length this edge runs on top of other edges.
	Shared float64
	// OutsideLen is the length of the route (and label) outside the
	// layout's bounds; OutsideDist how far beyond them it reaches.
	OutsideLen, OutsideDist float64
}

// Metrics are a diagram's totals with the per-edge breakdown.
type Metrics struct {
	Edges           []Edge `json:"-"`
	Bends           int
	Crossings       int // each crossing pair once
	Ratio           float64
	Through         int
	LabelOverNode   int
	LabelOverOwnEnd int
	LabelOverText   int
	LabelOverlaps   int
	Detached        int
	Jogs            int
	Shared          float64
	OutsideLen      float64
	OutsideDist     float64
}

// Measure computes the metrics of a routed layout. Call it before
// Result.Fit: the layout's bounds are taken from Width and Height.
func Measure(res *layout.Result) Metrics {
	bounds := layout.Rect{W: res.Width, H: res.Height}
	var nodes, texts, headers, nodeText []obst
	for _, b := range res.Boxes {
		if b.El.Kind == model.KindNode {
			nodes = append(nodes, obst{b.Shape, b})
			if len(b.Lines) > 0 {
				nodeText = append(nodeText, obst{textBounds(b.Lines), b})
			}
			if b.Icon.W > 0 {
				nodeText = append(nodeText, obst{b.Icon, b})
			}
			figure := b.El.Shape == model.ShapePerson || b.El.IconPlace == model.IconOnly && b.El.Icon != nil
			if figure && len(b.Lines) > 0 {
				texts = append(texts, obst{textBounds(b.Lines), b})
			}
			continue
		}
		if b.Shown {
			if hr, ok := headerRect(b); ok {
				headers = append(headers, obst{hr, b})
			}
		}
	}

	var m Metrics
	segs := make([][]seg, len(res.Edges))
	for i, e := range res.Edges {
		for k := 0; k+1 < len(e.Points); k++ {
			segs[i] = append(segs[i], seg{e.Points[k], e.Points[k+1]})
		}
	}
	ratios, nratio := 0.0, 0
	for i, e := range res.Edges {
		me := Edge{ID: e.E.ID}
		pts := e.Points
		if len(pts) >= 2 {
			me.Bends = len(pts) - 2
			if len(e.E.Via) == 0 {
				me.Jogs = jogs(pts, 2*res.Theme.EdgeClearance)
			}
			for _, s := range segs[i] {
				me.Length += s.len()
			}
			a, z := pts[0], pts[len(pts)-1]
			me.Manhattan = math.Abs(a.X-z.X) + math.Abs(a.Y-z.Y)
			if e.From != e.To && me.Manhattan > 1 {
				me.Ratio = me.Length / me.Manhattan
				ratios += me.Ratio
				nratio++
			}
		}
		for k, other := range segs {
			if k == i {
				continue
			}
			for _, s := range segs[i] {
				for _, o := range other {
					if s.crosses(o) {
						me.Crossings++
					}
					me.Shared += s.shared(o)
				}
			}
		}
		isEnd := func(b *layout.Box) bool { return b == e.From || b == e.To }
		for _, s := range segs[i] {
			for _, n := range nodes {
				if !isEnd(n.box) && s.clip(inflate(n.r, -1)) > 0.5 {
					me.Through++
				}
			}
			for _, t := range texts {
				if s.clip(inflate(t.r, -1)) > 0.5 {
					me.Through++
				}
			}
			for _, h := range headers {
				if s.clip(inflate(h.r, -1)) > 0.5 {
					me.Through++
				}
			}
			me.OutsideLen += s.len() - s.clip(bounds)
			for _, p := range []model.Point{s.a, s.b} {
				me.OutsideDist = math.Max(me.OutsideDist, beyond(bounds, layout.Rect{X: p.X, Y: p.Y}))
			}
		}
		if e.LabelRect.W > 0 {
			lr := inflate(e.LabelRect, -1)
			count := func(n *int, r layout.Rect) {
				if overlapArea(lr, r) > 0 {
					*n++
				}
			}
			for _, n := range nodes {
				if n.box == e.From || n.box == e.To {
					count(&me.LabelOverOwnEnd, n.r)
				} else {
					count(&me.LabelOverNode, n.r)
				}
			}
			for _, t := range nodeText {
				count(&me.LabelOverText, t.r)
			}
			for _, h := range headers {
				count(&me.LabelOverText, h.r)
			}
			for k, o := range res.Edges {
				if k == i {
					continue
				}
				if o.LabelRect.W > 0 {
					count(&me.LabelOverlaps, o.LabelRect)
				}
				for _, s := range segs[k] {
					if s.clip(lr) > 0.5 {
						me.LabelOverlaps++
						break
					}
				}
			}
			gap := math.Inf(1)
			for _, s := range segs[i] {
				gap = math.Min(gap, rectSegDist(e.LabelRect, s))
			}
			if gap > 4 {
				me.Detached = 1
			}
			me.OutsideDist = math.Max(me.OutsideDist, beyond(bounds, e.LabelRect))
		}
		m.Edges = append(m.Edges, me)
		m.Bends += me.Bends
		m.Crossings += me.Crossings
		m.Through += me.Through
		m.LabelOverNode += me.LabelOverNode
		m.LabelOverOwnEnd += me.LabelOverOwnEnd
		m.LabelOverText += me.LabelOverText
		m.LabelOverlaps += me.LabelOverlaps
		m.Detached += me.Detached
		m.Jogs += me.Jogs
		m.Shared += me.Shared
		m.OutsideLen += me.OutsideLen
		m.OutsideDist = math.Max(m.OutsideDist, me.OutsideDist)
	}
	m.Crossings /= 2
	m.Shared /= 2
	if nratio > 0 {
		m.Ratio = ratios / float64(nratio)
	}
	m.Ratio = round(m.Ratio, 1000)
	m.Shared = round(m.Shared, 10)
	m.OutsideLen = round(m.OutsideLen, 10)
	m.OutsideDist = round(m.OutsideDist, 10)
	return m
}

func round(v, k float64) float64 { return math.Round(v*k) / k }

type obst struct {
	r   layout.Rect
	box *layout.Box
}

type seg struct{ a, b model.Point }

func (s seg) len() float64 { return math.Hypot(s.b.X-s.a.X, s.b.Y-s.a.Y) }

// crosses reports whether s and o cross at a point strictly inside both.
func (s seg) crosses(o seg) bool {
	const e = 1e-6
	d := func(p, q, r model.Point) float64 { return (q.X-p.X)*(r.Y-p.Y) - (q.Y-p.Y)*(r.X-p.X) }
	d1, d2 := d(o.a, o.b, s.a), d(o.a, o.b, s.b)
	d3, d4 := d(s.a, s.b, o.a), d(s.a, s.b, o.b)
	return (d1 > e && d2 < -e || d1 < -e && d2 > e) && (d3 > e && d4 < -e || d3 < -e && d4 > e)
}

// shared returns how long s and o run on top of each other (axis-aligned
// segments within 1px).
func (s seg) shared(o seg) float64 {
	const w = 1
	switch {
	case s.a.Y == s.b.Y && o.a.Y == o.b.Y && s.a.X != s.b.X && o.a.X != o.b.X:
		if math.Abs(s.a.Y-o.a.Y) > w {
			return 0
		}
		return math.Max(0, math.Min(math.Max(s.a.X, s.b.X), math.Max(o.a.X, o.b.X))-math.Max(math.Min(s.a.X, s.b.X), math.Min(o.a.X, o.b.X)))
	case s.a.X == s.b.X && o.a.X == o.b.X && s.a.Y != s.b.Y && o.a.Y != o.b.Y:
		if math.Abs(s.a.X-o.a.X) > w {
			return 0
		}
		return math.Max(0, math.Min(math.Max(s.a.Y, s.b.Y), math.Max(o.a.Y, o.b.Y))-math.Max(math.Min(s.a.Y, s.b.Y), math.Min(o.a.Y, o.b.Y)))
	}
	return 0
}

// clip returns the length of s inside rc (Liang–Barsky).
func (s seg) clip(rc layout.Rect) float64 {
	if rc.W <= 0 || rc.H <= 0 {
		return 0
	}
	a, b := s.a, s.b
	t0, t1 := 0.0, 1.0
	ddx, ddy := b.X-a.X, b.Y-a.Y
	for _, c := range [4][2]float64{
		{-ddx, a.X - rc.X}, {ddx, rc.Right() - a.X},
		{-ddy, a.Y - rc.Y}, {ddy, rc.Bottom() - a.Y},
	} {
		p, q := c[0], c[1]
		if p == 0 {
			if q < 0 {
				return 0
			}
			continue
		}
		t := q / p
		if p < 0 {
			t0 = math.Max(t0, t)
		} else {
			t1 = math.Min(t1, t)
		}
	}
	if t1 <= t0 {
		return 0
	}
	return (t1 - t0) * math.Hypot(ddx, ddy)
}

// beyond returns how far r reaches outside bounds.
func beyond(bounds, r layout.Rect) float64 {
	return math.Max(0, math.Max(math.Max(bounds.X-r.X, r.Right()-bounds.Right()), math.Max(bounds.Y-r.Y, r.Bottom()-bounds.Bottom())))
}

func overlapArea(a, b layout.Rect) float64 {
	w := math.Min(a.Right(), b.Right()) - math.Max(a.X, b.X)
	h := math.Min(a.Bottom(), b.Bottom()) - math.Max(a.Y, b.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

func inflate(r layout.Rect, d float64) layout.Rect {
	return layout.Rect{X: r.X - d, Y: r.Y - d, W: r.W + 2*d, H: r.H + 2*d}
}

func union(a, b layout.Rect) layout.Rect {
	x, y := math.Min(a.X, b.X), math.Min(a.Y, b.Y)
	return layout.Rect{X: x, Y: y, W: math.Max(a.Right(), b.Right()) - x, H: math.Max(a.Bottom(), b.Bottom()) - y}
}

func headerRect(b *layout.Box) (layout.Rect, bool) {
	switch {
	case len(b.Lines) == 0 && b.Icon.W == 0:
		return layout.Rect{}, false
	case len(b.Lines) == 0:
		return b.Icon, true
	case b.Icon.W > 0:
		return union(textBounds(b.Lines), b.Icon), true
	}
	return textBounds(b.Lines), true
}

func textBounds(lines []layout.Line) layout.Rect {
	var out layout.Rect
	for i, ln := range lines {
		x := ln.X
		if ln.Anchor == layout.AnchorMiddle {
			x -= ln.Width / 2
		}
		lr := layout.Rect{X: x, Y: ln.Y - ln.Size*0.95, W: ln.Width, H: ln.Size * 1.25}
		if i == 0 {
			out = lr
		} else {
			out = union(out, lr)
		}
	}
	return out
}

// Score rates a diagram's routes as one number, lower being better: bends,
// crossings, routes through things, label collisions, length shared with
// other edges and length outside the layout, weighted roughly as the
// router weighs them.
func (m Metrics) Score() float64 {
	return 40*float64(m.Bends) + 80*float64(m.Crossings) + 200*float64(m.Through) +
		400*float64(m.LabelOverNode+m.LabelOverText) + 100*float64(m.LabelOverlaps+m.Detached) +
		20*float64(m.LabelOverOwnEnd) + 300*float64(m.Jogs) +
		2*m.Shared + m.OutsideLen
}

// rectSegDist is the distance between a rectangle and an axis-aligned
// segment (0 when they touch).
func rectSegDist(r layout.Rect, s seg) float64 {
	x0, x1 := math.Min(s.a.X, s.b.X), math.Max(s.a.X, s.b.X)
	y0, y1 := math.Min(s.a.Y, s.b.Y), math.Max(s.a.Y, s.b.Y)
	dx := math.Max(0, math.Max(r.X-x1, x0-r.Right()))
	dy := math.Max(0, math.Max(r.Y-y1, y0-r.Bottom()))
	return math.Hypot(dx, dy)
}

// jogs counts S-bends in a polyline: a segment shorter than short whose
// neighbours run the same way, so the route steps sideways and carries on.
func jogs(pts []model.Point, short float64) int {
	n := 0
	for k := 1; k+2 < len(pts); k++ {
		a, b, c, d := pts[k-1], pts[k], pts[k+1], pts[k+2]
		mid := math.Abs(c.X-b.X) + math.Abs(c.Y-b.Y)
		if mid >= short {
			continue
		}
		d1 := model.Point{X: b.X - a.X, Y: b.Y - a.Y}
		d2 := model.Point{X: d.X - c.X, Y: d.Y - c.Y}
		if d1.X*d2.X+d1.Y*d2.Y > 0 {
			n++
		}
	}
	return n
}
