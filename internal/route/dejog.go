package route

import (
	"math"

	"github.com/grindlemire/diagram/internal/model"
)

// jogCost is what a jog counts against a route when dejogging: a route
// may cross another edge or two to get rid of one.
const jogCost = 300

// jogCount counts a route's jogs and hooks: segments shorter than short
// between two that run the same way (a step sideways) or opposite ways
// (an overshoot that doubles back).
func jogCount(pts []model.Point, short float64) int {
	n := 0
	for k := 1; k+2 < len(pts); k++ {
		a, b, c, d := pts[k-1], pts[k], pts[k+1], pts[k+2]
		if seglen(b, c) >= short {
			continue
		}
		if (b.X-a.X)*(d.X-c.X)+(b.Y-a.Y)*(d.Y-c.Y) != 0 {
			n++
		}
	}
	return n
}

// dejog reroutes each edge whose route still has a jog that straightening
// couldn't remove, with its unpinned sides open again, and keeps the new
// route only if it has no jog, no more bends, and scores better. New
// ports must keep PortSpacing from the other ends on their side. Labels
// are ignored: they are all placed again afterwards.
func (r *router) dejog() {
	short := 2 * r.c
	// Labels are placed again once routes are final, so a jog that only
	// steps around one isn't needed: route as if they weren't there.
	labels, owners := r.labels, r.labelEdge
	r.labels, r.labelEdge = nil, nil
	defer func() { r.labels, r.labelEdge = labels, owners }()
	for _, j := range r.jobs {
		if j.kind != kindRouted || j.err != nil || j.src == nil || len(j.e.E.Via) > 0 {
			continue
		}
		old := j.e.Points
		if jogCount(old, short) == 0 {
			continue
		}
		r.removeEdge(j.i)
		cur := r.measurePath(j, old)
		src, dst := *j.src, *j.dst
		free := func(en *end, c cand) bool {
			for _, o := range r.jobs {
				for _, oe := range []*end{o.src, o.dst, o.selfEnd[0], o.selfEnd[1]} {
					if oe != nil && oe != en && oe.box == en.box && oe.side == c.side &&
						math.Abs(oe.pos-c.pos) < r.t.PortSpacing-eps {
						return false
					}
				}
			}
			return true
		}
		keep := func(en *end, src bool) cand {
			c := cand{side: en.side, pos: en.pos, p: en.point, travel: en.travel}
			c.stub = r.stubLen(j, c.p, c.travel, en.box, en.side, r.wantStub(j, src))
			return c
		}
		// pick lists the current port, then one on each other side at the
		// free position nearest the side's preferred one.
		pick := func(en *end, src bool) []cand {
			out := []cand{keep(en, src)}
			for _, c := range r.candidates(j, src) {
				if c.side == en.side {
					continue
				}
				lo, hi := r.sideSpan(en.box, c.side)
				want := c.pos
				for k := 0; k < 12; k++ {
					off := float64((k+1)/2) * r.t.PortSpacing
					if k%2 == 1 {
						off = -off
					}
					c.pos = want + off
					if c.pos < lo-eps || c.pos > hi+eps || !free(en, c) {
						continue
					}
					c.p = r.portAt(en.box, c.side, c.pos)
					c.stub = r.stubLen(j, c.p, c.travel, en.box, c.side, r.wantStub(j, src))
					out = append(out, c)
					break
				}
			}
			return out
		}
		srcs, dsts := pick(j.src, true), pick(j.dst, false)
		// The cheapest route may well be the jogged one, so try too with
		// each end's current side ruled out.
		tries := [][2][]cand{{srcs, dsts}, {srcs[1:], dsts}, {srcs, dsts[1:]}}
		var best []model.Point
		var bestS, bestD cand
		bestCost := cur.cost + jogCost*float64(jogCount(old, short))
		if !cur.ok {
			bestCost = inf // the old route hits something anyway
		}
		for _, t := range tries {
			if len(t[0]) == 0 || len(t[1]) == 0 {
				continue
			}
			pts, si, di, err := r.search(j, t[0], t[1])
			if err != nil {
				continue
			}
			s, d := t[0][si], t[1][di]
			apply := func(en *end, c cand) {
				en.side, en.pos, en.point, en.travel = c.side, c.pos, c.p, c.travel
			}
			apply(j.src, s)
			apply(j.dst, d)
			pts = r.straighten(j, r.separate(j, pts))
			m := r.measurePath(j, pts)
			if m.ok && jogCount(pts, short) == 0 && len(pts) <= len(old) && m.cost < bestCost {
				best, bestS, bestD, bestCost = pts, cand{side: j.src.side, pos: j.src.pos, p: j.src.point, travel: j.src.travel},
					cand{side: j.dst.side, pos: j.dst.pos, p: j.dst.point, travel: j.dst.travel}, m.cost
			}
			*j.src, *j.dst = src, dst
		}
		accepted := best != nil
		if accepted {
			j.src.side, j.src.pos, j.src.point, j.src.travel = bestS.side, bestS.pos, bestS.p, bestS.travel
			j.dst.side, j.dst.pos, j.dst.point, j.dst.travel = bestD.side, bestD.pos, bestD.p, bestD.travel
			j.e.Points = best
			j.e.ExitSide, j.e.EnterSide = j.src.side, j.dst.side
		}
		if !accepted {
			*j.src, *j.dst = src, dst
			j.e.Points = old
		}
		r.addPath(j.i, j.e.Points)
	}
}
