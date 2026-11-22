package route

import (
	"math"
	"slices"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

// firstPass routes j with free choice of unpinned sides and records the
// ends it chose.
func (r *router) firstPass(j *job) {
	srcs, dsts := r.candidates(j, true), r.candidates(j, false)
	pts, si, di, err := r.search(j, srcs, dsts)
	if err != nil {
		j.err = err
		return
	}
	vias := j.e.E.Via
	j.src = r.endFromPath(j, 0, j.e.From, srcs[si], pts, vias)
	j.dst = r.endFromPath(j, 1, j.e.To, dsts[di], reversed(pts), reversed(vias))
	r.load[r.boxIdx[j.e.From]][j.src.side]++
	r.load[r.boxIdx[j.e.To]][j.dst.side]++
	r.addPath(j.i, pts)
	j.sketch = pts
}

// reselect rips up j's first-pass route and chooses its sides again, now
// that the first-pass routes of the edges declared after it are known.
func (r *router) reselect(j *job) {
	r.removeEdge(j.i)
	r.load[r.boxIdx[j.e.From]][j.src.side]--
	r.load[r.boxIdx[j.e.To]][j.dst.side]--
	src, dst := j.src, j.dst
	r.firstPass(j)
	if j.err != nil {
		// Keep the earlier choice; its route was found before.
		j.err, j.src, j.dst = nil, src, dst
		r.load[r.boxIdx[j.e.From]][j.src.side]++
		r.load[r.boxIdx[j.e.To]][j.dst.side]++
	}
}

// removeEdge drops edge i's segments and label from the routing state.
func (r *router) removeEdge(i int) {
	segs := r.segs[:0:0]
	for _, s := range r.segs {
		if s.edge != i {
			segs = append(segs, s)
		}
	}
	r.segs = segs
	r.segsChanged()
	var labels []layout.Rect
	var owners []int
	for k, l := range r.labels {
		if r.labelEdge[k] != i {
			labels, owners = append(labels, l), append(owners, r.labelEdge[k])
		}
	}
	r.labels, r.labelEdge = labels, owners
}

// secondPass routes j between its final ports.
func (r *router) secondPass(j *job) {
	mk := func(en *end) cand {
		c := cand{side: en.side, pos: en.pos, p: en.point, travel: en.travel}
		c.stub = r.stubLen(j, c.p, c.travel, en.box, en.side, r.wantStub(j, en.order == 0))
		return c
	}
	pts, _, _, err := r.search(j, []cand{mk(j.src)}, []cand{mk(j.dst)})
	if err != nil {
		j.err = err
		return
	}
	e := j.e
	e.Points = r.straighten(j, r.separate(j, pts))
	e.ExitSide, e.EnterSide = j.src.side, j.dst.side
	e.Rounded = true
	r.addPath(j.i, e.Points)
}

// straighten removes jogs and hooks: a short segment (under twice the
// clearance) between two that run the same way, which makes the route
// step sideways and carry on, or opposite ways, which makes it overshoot
// and double back. One of the two outer segments moves in line with the
// other, sliding a port along its side if it is attached to one. A move
// is kept when it hits nothing, keeps both ends' stubs, runs on top of
// no more of another edge than before (two edges on one line read as one;
// one merely close by, nudging separates later), and costs less than the
// jog (jogCost): a jog is worse than a crossing.
func (r *router) straighten(j *job, pts []model.Point) []model.Point {
	// slide returns where en's port lands when its segment moves by d, if
	// it can move there.
	slide := func(en *end, d model.Point) (model.Point, bool) {
		pos := en.pos + d.X
		if !horiz(en.side) {
			pos = en.pos + d.Y
		}
		lo, hi := r.sideSpan(en.box, en.side)
		if pos < lo-eps || pos > hi+eps {
			return model.Point{}, false
		}
		for _, o := range r.jobs {
			for _, oe := range []*end{o.src, o.dst, o.selfEnd[0], o.selfEnd[1]} {
				if oe != nil && oe != en && oe.box == en.box && oe.side == en.side &&
					math.Abs(oe.pos-pos) < r.t.PortSpacing-eps {
					return model.Point{}, false
				}
			}
		}
		// On a curved outline the port also moves along its stub, which
		// then starts there.
		np, want := r.portAt(en.box, en.side, pos), add(en.point, d)
		if horiz(en.side) && math.Abs(np.X-want.X) > 1e-6 || !horiz(en.side) && math.Abs(np.Y-want.Y) > 1e-6 {
			return model.Point{}, false
		}
		return np, true
	}
	// No move may shorten an end's straight run below the stub its port
	// is granted, or below what it already has: the arrowhead and the
	// bend need it.
	n := len(pts)
	stub0 := math.Max(r.c/2, math.Min(r.endStub(j, j.src, true), seglen(pts[0], pts[1]))-0.5)
	stub1 := math.Max(r.c/2, math.Min(r.endStub(j, j.dst, false), seglen(pts[n-2], pts[n-1]))-0.5)
	short := 2 * r.c
	for changed := true; changed; {
		changed = false
		cur := r.measurePath(j, pts)
		for k := 1; k+2 < len(pts); k++ {
			d := model.Point{X: pts[k+1].X - pts[k].X, Y: pts[k+1].Y - pts[k].Y}
			before, after := dirBetween(pts[k-1], pts[k]), dirBetween(pts[k+1], pts[k+2])
			if seglen(pts[k], pts[k+1]) >= short || before != after && before != after.opposite() {
				continue
			}
			// Move segment i by dd, keeping end stubs of at least s0 and
			// s1; returns the new points, the end whose port slides, if
			// any, and where that port lands.
			try := func(i int, dd model.Point, s0, s1 float64) ([]model.Point, *end, model.Point, bool) {
				var en *end
				switch {
				case i == 0:
					en = j.src
				case i+1 == len(pts)-1:
					en = j.dst
				}
				c := slices.Clone(pts)
				c[i], c[i+1] = add(c[i], dd), add(c[i+1], dd)
				var np model.Point
				if en != nil {
					var ok bool
					if np, ok = slide(en, dd); !ok {
						return nil, nil, np, false
					}
					if en == j.src {
						c[0] = np
					} else {
						c[len(c)-1] = np
					}
				}
				c = simplify(c)
				// The route must still leave and arrive along its ports'
				// stubs, and never turn back on itself.
				n := len(c)
				if n < 2 || n >= len(pts) || dirBetween(c[0], c[1]) != j.src.travel || dirBetween(c[n-2], c[n-1]) != j.dst.travel.opposite() ||
					seglen(c[0], c[1]) < s0 || seglen(c[n-2], c[n-1]) < s1 || reverses(c) {
					return nil, nil, np, false
				}
				// A jog costs more than a crossing, but not more than
				// merging into another edge's line.
				m := r.measurePath(j, c)
				if !m.ok || m.onTop > cur.onTop+0.5 || cur.ok && m.cost >= cur.cost+jogCost {
					return nil, nil, np, false
				}
				return c, en, np, true
			}
			var best []model.Point
			var bestEnd *end
			var bestMove, bestPort model.Point
			bestCost := inf
			moves := func(s0, s1 float64) {
				for _, o := range []struct {
					i int
					d model.Point
				}{{k - 1, d}, {k + 1, model.Point{X: -d.X, Y: -d.Y}}} {
					if c, en, np, ok := try(o.i, o.d, s0, s1); ok {
						if cost := r.measurePath(j, c).cost; cost < bestCost {
							best, bestEnd, bestMove, bestPort, bestCost = c, en, o.d, np, cost
						}
					}
				}
			}
			moves(stub0, stub1)
			// Where there is no room for both stubs, a jog too short to
			// round reads as a notch, which is worse than a short stub.
			if best == nil && seglen(pts[k], pts[k+1]) < 2*r.t.EdgeCornerRad {
				moves(r.c/2, r.c/2)
			}
			if best != nil {
				if bestEnd != nil {
					bestEnd.pos += bestMove.X + bestMove.Y
					bestEnd.point = bestPort
				}
				pts = best
				changed = true
				break
			}
		}
	}
	return pts
}

// reverses reports whether a route turns straight back on itself anywhere.
func reverses(pts []model.Point) bool {
	for k := 0; k+2 < len(pts); k++ {
		if dirBetween(pts[k], pts[k+1]) == dirBetween(pts[k+1], pts[k+2]).opposite() {
			return true
		}
	}
	return false
}

// separate moves the interior segments of a new route away from earlier
// routes' parallel segments that run closer than ParallelOffset, where
// there's room. A* prefers distinct lanes already; this fixes up the
// places where the grid offered none at the right distance.
func (r *router) separate(j *job, pts []model.Point) []model.Point {
	off := r.t.ParallelOffset
	clear := func(a, b model.Point) bool {
		for _, n := range r.nodes {
			if clipLen(inflate(n.Shape, 4), a, b) > 0.01 {
				return false
			}
		}
		for _, l := range r.labels {
			if clipLen(l, a, b) > 0.01 {
				return false
			}
		}
		// Don't hug group borders.
		for _, g := range r.groups {
			for _, ln := range borders(g.Rect) {
				if parallelWithin(a, b, ln[0], ln[1], r.c/2) {
					return false
				}
			}
		}
		return true
	}
	borderHits := func(pts []model.Point) int {
		n := 0
		for i := 0; i+1 < len(pts); i++ {
			for _, g := range r.groups {
				for _, ln := range borders(g.Rect) {
					if properCross(pts[i], pts[i+1], ln[0], ln[1]) {
						n++
					}
				}
			}
		}
		return n
	}
	// closest returns the signed offset to the nearest parallel, overlapping
	// segment of another edge within off of a→b, and whether there is one.
	closest := func(a, b model.Point, skip float64) (float64, bool) {
		hz := a.Y == b.Y
		best, found := 0.0, false
		for _, s := range r.segs {
			var d, lo, hi, slo, shi float64
			switch {
			case hz && s.horizontal() && s.a.X != s.b.X:
				d = s.a.Y - a.Y
				lo, hi, slo, shi = math.Min(a.X, b.X), math.Max(a.X, b.X), s.a.X, s.b.X
			case !hz && s.vertical() && s.a.Y != s.b.Y:
				d = s.a.X - a.X
				lo, hi, slo, shi = math.Min(a.Y, b.Y), math.Max(a.Y, b.Y), s.a.Y, s.b.Y
			default:
				continue
			}
			if math.Abs(d) >= skip || math.Min(hi, shi)-math.Max(lo, slo) < 2 {
				continue
			}
			if !found || math.Abs(d) < math.Abs(best) {
				best, found = d, true
			}
		}
		return best, found
	}
	// The end segments keep the stubs their ports are granted.
	n := len(pts)
	stub0 := math.Min(r.endStub(j, j.src, true), seglen(pts[0], pts[1])) - 0.5
	stub1 := math.Min(r.endStub(j, j.dst, false), seglen(pts[n-2], pts[n-1])) - 0.5
	for k := 1; k+2 < len(pts); k++ {
		a, b := pts[k], pts[k+1]
		d, ok := closest(a, b, off-0.5)
		if !ok {
			continue
		}
		hz := a.Y == b.Y
		// Shift to off away from the neighbour, on whichever side is the
		// smaller move first.
		shifts := []float64{d - math.Copysign(off, d), d + math.Copysign(off, d)}
		if math.Abs(shifts[1]) < math.Abs(shifts[0]) {
			shifts[0], shifts[1] = shifts[1], shifts[0]
		}
		for _, sh := range shifts {
			mv := model.Point{Y: sh}
			if !hz {
				mv = model.Point{X: sh}
			}
			na, nb := add(a, mv), add(b, mv)
			// The neighbouring segments must keep their direction.
			if !sameDir(pts[k-1], a, na) || !sameDir(pts[k+2], b, nb) {
				continue
			}
			if k == 1 && seglen(pts[0], na) < stub0 || k+2 == n-1 && seglen(nb, pts[n-1]) < stub1 {
				continue
			}
			if !clear(pts[k-1], na) || !clear(na, nb) || !clear(nb, pts[k+2]) {
				continue
			}
			if _, bad := closest(na, nb, off-0.5); bad {
				continue
			}
			before := borderHits(pts[k-1 : k+3])
			oa, ob := pts[k], pts[k+1]
			pts[k], pts[k+1] = na, nb
			if borderHits(pts[k-1:k+3]) > before {
				pts[k], pts[k+1] = oa, ob
				continue
			}
			break
		}
	}
	return simplify(pts)
}

func borders(rc layout.Rect) [4][2]model.Point {
	tl, tr := model.Point{X: rc.X, Y: rc.Y}, model.Point{X: rc.Right(), Y: rc.Y}
	bl, br := model.Point{X: rc.X, Y: rc.Bottom()}, model.Point{X: rc.Right(), Y: rc.Bottom()}
	return [4][2]model.Point{{tl, tr}, {bl, br}, {tl, bl}, {tr, br}}
}

// parallelWithin reports whether axis-aligned segments ab and cd run
// parallel, overlapping, closer than d.
func parallelWithin(a, b, c, e model.Point, d float64) bool {
	switch {
	case a.Y == b.Y && c.Y == e.Y:
		return math.Abs(a.Y-c.Y) < d && math.Min(math.Max(a.X, b.X), math.Max(c.X, e.X))-math.Max(math.Min(a.X, b.X), math.Min(c.X, e.X)) > 0
	case a.X == b.X && c.X == e.X:
		return math.Abs(a.X-c.X) < d && math.Min(math.Max(a.Y, b.Y), math.Max(c.Y, e.Y))-math.Max(math.Min(a.Y, b.Y), math.Min(c.Y, e.Y)) > 0
	}
	return false
}

// properCross reports whether axis-aligned segments ab and cd cross at a
// point inside both.
func properCross(a, b, c, e model.Point) bool {
	if a.Y == b.Y {
		a, b, c, e = c, e, a, b
	}
	if a.X != b.X || c.Y != e.Y {
		return false
	}
	return a.X > math.Min(c.X, e.X) && a.X < math.Max(c.X, e.X) && c.Y > math.Min(a.Y, b.Y) && c.Y < math.Max(a.Y, b.Y)
}

// sameDir reports whether moving the end of segment from→to to nto keeps
// the segment pointing the same way with some length left.
func sameDir(from, to, nto model.Point) bool {
	d0 := model.Point{X: to.X - from.X, Y: to.Y - from.Y}
	d1 := model.Point{X: nto.X - from.X, Y: nto.Y - from.Y}
	return d0.X*d1.X+d0.Y*d1.Y > 0 && math.Abs(d1.X)+math.Abs(d1.Y) >= 4
}

func reversed[T any](s []T) []T {
	out := slices.Clone(s)
	slices.Reverse(out)
	return out
}

// ---- labels ----

type piece struct {
	a, b model.Point
	l    float64
}

func pieces(pts []model.Point) []piece {
	var out []piece
	for i := 0; i+1 < len(pts); i++ {
		out = append(out, piece{pts[i], pts[i+1], math.Hypot(pts[i+1].X-pts[i].X, pts[i+1].Y-pts[i].Y)})
	}
	return out
}

func lerp(a, b model.Point, t float64) model.Point {
	return model.Point{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t}
}

// stubsNear returns the stubs of other edges' ports that touch area.
func (r *router) stubsNear(j *job, area layout.Rect) []layout.Rect {
	var out []layout.Rect
	for _, o := range r.jobs {
		if o == j || o.kind != kindRouted || o.src == nil {
			continue
		}
		for _, en := range []*end{o.src, o.dst} {
			if st := stubRect(en.point, en.travel, r.c+6); overlap(inflate(area, 1), st) > 0 {
				out = append(out, st)
			}
		}
	}
	return out
}

func overlap(a, b layout.Rect) float64 {
	w := math.Min(a.Right(), b.Right()) - math.Max(a.X, b.X)
	h := math.Min(a.Bottom(), b.Bottom()) - math.Max(a.Y, b.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// clipLen returns the length of segment ab inside rc (Liang–Barsky).
func clipLen(rc layout.Rect, a, b model.Point) float64 {
	t0, t1 := 0.0, 1.0
	ddx, ddy := b.X-a.X, b.Y-a.Y
	for _, c := range [4][2]float64{
		{-ddx, a.X - rc.X}, {ddx, rc.Right() - a.X},
		{-ddy, a.Y - rc.Y}, {ddy, rc.Bottom() - a.Y},
	} {
		p, q := c[0], c[1]
		if p == 0 {
			if q <= 0 {
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
