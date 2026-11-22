package route

import (
	"math"
	"slices"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

// simplifyRoutes tries to replace each routed edge with a route that has
// fewer bends between the same ports: a straight line, an L, a Z or U, or
// a three-bend route. A replacement is kept only if it hits no obstacle,
// stays where the edge may go and adds no crossings, no overlaps with
// other edges and no length outside the diagram. Changed edges get their
// labels placed again.
func (r *router) simplifyRoutes() {
	for _, j := range r.jobs {
		if j.kind != kindRouted || j.err != nil || j.src == nil || len(j.e.E.Via) > 0 {
			continue
		}
		pts := j.e.Points
		if len(pts) < 3 {
			continue
		}
		r.removeEdge(j.i)
		cur := r.measurePath(j, pts)
		best, bestM := pts, cur
		for _, cand := range r.simpler(j, pts) {
			m := r.measurePath(j, cand)
			if !m.ok || len(cand) >= len(best) && !(len(cand) == len(best) && m.cost < bestM.cost-1e-6) {
				continue
			}
			if m.crossings > cur.crossings || m.near > cur.near+0.5 || m.outside > cur.outside+0.5 ||
				m.hug > cur.hug+0.5 || m.cost > cur.cost+2*bendCost*float64(len(pts)-len(cand)) {
				continue
			}
			best, bestM = cand, m
		}
		j.e.Points = best
		r.addPath(j.i, best)
		r.placeLabel(j)
	}
}

// simpler lists routes with fewer bends than pts between its two ports,
// which leave and arrive in the same directions.
func (r *router) simpler(j *job, pts []model.Point) [][]model.Point {
	p0, p1 := pts[0], pts[len(pts)-1]
	d0 := j.src.travel
	d1 := j.dst.travel.opposite() // direction of arrival
	s0 := r.stubLen(j, p0, d0, j.src.box, j.src.side, r.wantStub(j, true))
	s1 := r.stubLen(j, p1, j.dst.travel, j.dst.box, j.dst.side, r.wantStub(j, false))
	hz := func(d dir) bool { return d == dirLeft || d == dirRight }
	// Coordinates worth trying for free segments: those of the current
	// route, the midpoint, just past the stubs, and a sweep in between.
	var xs, ys []float64
	for _, p := range pts {
		xs, ys = append(xs, p.X), append(ys, p.Y)
	}
	xs = append(xs, (p0.X+p1.X)/2)
	ys = append(ys, (p0.Y+p1.Y)/2)
	q0, q1 := step(p0, d0, s0), step(p1, d1.opposite(), s1)
	xs, ys = append(xs, q0.X, q1.X), append(ys, q0.Y, q1.Y)
	off := math.Max(r.t.ParallelOffset, 4)
	sweep := func(vs []float64, a, b float64) []float64 {
		lo, hi := math.Min(a, b), math.Max(a, b)
		if n := (hi - lo) / off; n > 40 {
			return vs
		}
		for v := lo + off; v < hi; v += off {
			vs = append(vs, v)
		}
		return vs
	}
	xs = sweep(xs, p0.X, p1.X)
	ys = sweep(ys, p0.Y, p1.Y)
	xs, ys = uniq(xs), uniq(ys)

	var out [][]model.Point
	add := func(c ...model.Point) {
		c = simplify(c)
		if len(c) < len(pts) {
			out = append(out, c)
		}
	}
	switch {
	case d0 == d1:
		// Straight, or a Z through any crossing coordinate.
		add(p0, p1)
		if hz(d0) {
			for _, x := range xs {
				add(p0, model.Point{X: x, Y: p0.Y}, model.Point{X: x, Y: p1.Y}, p1)
			}
		} else {
			for _, y := range ys {
				add(p0, model.Point{X: p0.X, Y: y}, model.Point{X: p1.X, Y: y}, p1)
			}
		}
	case d0 == d1.opposite():
		// A U.
		if hz(d0) {
			for _, x := range xs {
				add(p0, model.Point{X: x, Y: p0.Y}, model.Point{X: x, Y: p1.Y}, p1)
			}
		} else {
			for _, y := range ys {
				add(p0, model.Point{X: p0.X, Y: y}, model.Point{X: p1.X, Y: y}, p1)
			}
		}
	default:
		// An L, or three bends: out, across, along, in.
		if hz(d0) {
			add(p0, model.Point{X: p1.X, Y: p0.Y}, p1)
			for _, x := range xs {
				for _, y := range ys {
					add(p0, model.Point{X: x, Y: p0.Y}, model.Point{X: x, Y: y}, model.Point{X: p1.X, Y: y}, p1)
				}
			}
		} else {
			add(p0, model.Point{X: p0.X, Y: p1.Y}, p1)
			for _, y := range ys {
				for _, x := range xs {
					add(p0, model.Point{X: p0.X, Y: y}, model.Point{X: x, Y: y}, model.Point{X: x, Y: p1.Y}, p1)
				}
			}
		}
	}
	// Keep only routes that leave and arrive the right way with their
	// stubs, and never reverse.
	valid := out[:0]
	for _, c := range out {
		if len(c) < 2 || dirBetween(c[0], c[1]) != d0 || dirBetween(c[len(c)-2], c[len(c)-1]) != d1 {
			continue
		}
		if seglen(c[0], c[1]) < s0-eps || seglen(c[len(c)-2], c[len(c)-1]) < s1-eps {
			continue
		}
		bad := false
		for k := 0; k+2 < len(c); k++ {
			if dirBetween(c[k], c[k+1]) == dirBetween(c[k+1], c[k+2]).opposite() {
				bad = true
				break
			}
		}
		if !bad {
			valid = append(valid, c)
		}
	}
	return valid
}

func uniq(vs []float64) []float64 {
	slices.Sort(vs)
	out := vs[:0]
	for _, v := range vs {
		if n := len(out); n == 0 || v-out[n-1] > 0.01 {
			out = append(out, v)
		}
	}
	return out
}

func seglen(a, b model.Point) float64 { return math.Abs(b.X-a.X) + math.Abs(b.Y-a.Y) }

// dirBetween is the direction from a to b along one axis.
func dirBetween(a, b model.Point) dir {
	switch {
	case b.X > a.X+eps:
		return dirRight
	case b.X < a.X-eps:
		return dirLeft
	case b.Y > a.Y+eps:
		return dirDown
	}
	return dirUp
}

// pathMetrics rates a candidate route for an edge against everything else
// already routed.
type pathMetrics struct {
	ok        bool    // touches no obstacle
	crossings int     // proper crossings with other edges
	near      float64 // length closer than ParallelOffset to other edges
	onTop     float64 // length on the same line as another edge, which reads as one line
	hug       float64 // length inside the clearance of nodes that aren't ends
	outside   float64 // length outside the diagram's bounds
	cost      float64 // length plus the above, weighted as in the search
}

// measurePath checks a route for edge j. The edge's own segments must not
// be in r.segs.
func (r *router) measurePath(j *job, pts []model.Point) pathMetrics {
	m := pathMetrics{ok: true}
	e := j.e
	c := r.c
	isEnd := func(b *layout.Box) bool { return b == e.From || b == e.To }
	last := len(pts) - 2
	inb := r.jobBounds(j)
	for k := 0; k+1 < len(pts); k++ {
		a, b := pts[k], pts[k+1]
		l := seglen(a, b)
		m.cost += l
		bad := false
		r.nodesNear(inflate(segRect(a, b), c), func(n *layout.Box) {
			if bad {
				return
			}
			if isEnd(n) {
				// Only the first and last segments may touch an end, and
				// only where they leave it.
				in := inflate(n.Shape, -0.5)
				if k != 0 && k != last {
					in = inflate(n.Shape, minClear)
				}
				if clipLen(in, a, b) > 0.01 {
					bad = true
				}
				return
			}
			if clipLen(inflate(n.Shape, minClear), a, b) > 0.01 {
				bad = true
				return
			}
			m.hug += clipLen(inflate(n.Shape, c-0.5), a, b)
		})
		if bad {
			m.ok = false
			return m
		}
		for _, t := range r.texts {
			// A port under a figure's text starts inside the text's
			// margin, so the segment leaving it may cross the margin.
			if t.owner == e.From && k == 0 || t.owner == e.To && k == last {
				if clipLen(inflate(t.r, -0.5), a, b) > 0.01 {
					m.ok = false
					return m
				}
				continue
			}
			if clipLen(inflate(t.r, minClear), a, b) > 0.01 {
				m.ok = false
				return m
			}
			m.hug += clipLen(inflate(t.r, c-0.5), a, b)
		}
		for _, h := range r.headers {
			if clipLen(inflate(h.r, 2), a, b) > 0.01 {
				m.ok = false
				return m
			}
			// The search keeps a little further off (its soft zone).
			m.hug += clipLen(inflate(h.r, 4), a, b)
		}
		for _, lr := range r.labels {
			if clipLen(inflate(lr, 2), a, b) > 0.01 {
				m.ok = false
				return m
			}
		}
		for _, g := range r.groups {
			if slices.Contains(j.allowed, g) {
				continue
			}
			if clipLen(inflate(g.Rect, c/2), a, b) > 0.01 {
				m.ok = false
				return m
			}
		}
		if j.region != nil {
			in := inflate(j.region.Rect, -c/2+0.01)
			if !contains(in, a) || !contains(in, b) {
				m.ok = false
				return m
			}
		}
		m.outside += l - clipLen(inb, a, b)
		r.segsNear(inflate(segRect(a, b), r.t.ParallelOffset), func(s seg) {
			if crossesAny(a, b, s.a, s.b) {
				m.crossings++
			}
			m.near += nearLen(a, b, s.a, s.b, r.t.ParallelOffset-0.5)
			m.onTop += nearLen(a, b, s.a, s.b, onTopDist)
		})
	}
	m.cost += bendCost*float64(len(pts)-2) + crossCost*float64(m.crossings) +
		clearCost*m.hug + overlapCost*m.near + outsideCost*m.outside
	return m
}

// jobBounds is the diagram's bounds grown to where j's stubs end when
// they were lengthened across a group border, as the search counts them:
// a route whose stub ends outside has to turn there.
func (r *router) jobBounds(j *job) layout.Rect {
	b := r.bounds
	for _, en := range []*end{j.src, j.dst} {
		if en == nil {
			continue
		}
		// Only a port near the edge can have its stub end outside.
		if in := inflate(b, -4*r.c); strictlyInside(in, en.point) {
			continue
		}
		src := en == j.src
		if st := r.endStub(j, en, src); st > r.wantStub(j, src)+eps {
			q := step(en.point, en.travel, st)
			b = union(b, layout.Rect{X: q.X, Y: q.Y})
		}
	}
	return b
}

// onTopDist is how close two parallel lines are taken to be one line.
const onTopDist = 2

// crossesAny reports whether segments ab and cd cross at a point inside
// both (any orientation).
func crossesAny(a, b, c, d model.Point) bool {
	const e = 1e-6
	o := func(p, q, s model.Point) float64 { return (q.X-p.X)*(s.Y-p.Y) - (q.Y-p.Y)*(s.X-p.X) }
	d1, d2 := o(c, d, a), o(c, d, b)
	d3, d4 := o(a, b, c), o(a, b, d)
	return (d1 > e && d2 < -e || d1 < -e && d2 > e) && (d3 > e && d4 < -e || d3 < -e && d4 > e)
}

// nearLen is how long axis-aligned segments ab and cd run parallel closer
// than w.
func nearLen(a, b, c, d model.Point, w float64) float64 {
	switch {
	case a.Y == b.Y && c.Y == d.Y && a.X != b.X && c.X != d.X:
		if math.Abs(a.Y-c.Y) >= w {
			return 0
		}
		return math.Max(0, math.Min(math.Max(a.X, b.X), math.Max(c.X, d.X))-math.Max(math.Min(a.X, b.X), math.Min(c.X, d.X)))
	case a.X == b.X && c.X == d.X && a.Y != b.Y && c.Y != d.Y:
		if math.Abs(a.X-c.X) >= w {
			return 0
		}
		return math.Max(0, math.Min(math.Max(a.Y, b.Y), math.Max(c.Y, d.Y))-math.Max(math.Min(a.Y, b.Y), math.Min(c.Y, d.Y)))
	}
	return 0
}
