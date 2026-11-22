package route

import (
	"slices"

	"github.com/grindlemire/diagram/internal/model"
)

// maxUncross is the most pairs of edges uncross tries to untangle.
const maxUncross = 12

// uncross untangles pairs of edges with ends on the same box whose final
// routes cross or run on one line. For each pair it tries the two ends'
// ports swapped and as they are, routing either edge first (the first
// one routed takes its lanes and the second fits around it), nudges the
// routes again and keeps the least tangled result when the diagram as a
// whole is no worse (tally.noWorse): in particular no more length on one
// line with another edge, as two edges on one line read as one.
//
// Ports are ordered from the first-pass sketches, which only see how each
// route turns next to its port (endKey). Two routes that come from
// opposite directions further away can get each other's port and cross
// on their way in. uncross runs on nudged routes because nudging decides
// which of two edges sharing a lane ends up on which side.
func (r *router) uncross() {
	var ends []*end
	for _, j := range r.jobs {
		if j.kind != kindRouted || j.err != nil || j.src == nil || len(j.e.E.Via) > 0 {
			continue
		}
		ends = append(ends, j.src, j.dst)
	}
	type pair struct{ a, b *end }
	var pairs []pair
	for i, a := range ends {
		for _, b := range ends[i+1:] {
			if a.box == b.box && a.j != b.j && !a.anchor && !b.anchor && r.swappable(a, b) && r.tangle(a.j, b.j) > 0 {
				pairs = append(pairs, pair{a, b})
			}
		}
	}
	for n, p := range pairs {
		if n >= maxUncross {
			break
		}
		old := r.tangle(p.a.j, p.b.j)
		if old == 0 {
			continue // an earlier change fixed it
		}
		before := r.snapshot()
		base := r.diagramTally()
		clean := r.clean(p.a.j) && r.clean(p.b.j)
		var best *state
		bestTangle := old
		for _, swap := range []bool{true, false} {
			for _, rev := range []bool{false, true} {
				if !swap && !rev {
					continue // that is the current routing
				}
				if swap {
					swapEnds(p.a, p.b)
				}
				js := []*job{p.a.j, p.b.j}
				slices.SortFunc(js, func(x, y *job) int { return x.i - y.i })
				if rev {
					js[0], js[1] = js[1], js[0]
				}
				if r.reroute(js) {
					r.nudge()
					t := r.tangle(p.a.j, p.b.j)
					if t < bestTangle-eps && r.diagramTally().noWorse(base) &&
						(!clean || r.clean(p.a.j) && r.clean(p.b.j)) {
						st := r.snapshot()
						best, bestTangle = &st, t
					}
				}
				r.restore(before)
			}
		}
		if best != nil {
			r.restore(*best)
		}
	}
}

// reroute routes js again between their current ports, in order. It
// reports whether every one found a route; if not, the caller restores
// the earlier state.
func (r *router) reroute(js []*job) bool {
	for _, j := range js {
		r.removeEdge(j.i)
	}
	for _, j := range js {
		r.secondPass(j)
		if j.err != nil {
			j.err = nil
			return false
		}
		r.placeLabel(j)
	}
	return true
}

// tally counts what uncross weighs across the whole diagram.
type tally struct {
	bends, jogs, crossings int
	onTop                  float64 // length edges run on one line with another
}

// noWorse reports whether t is no worse than u: no more crossings, jogs
// or length on one line, and any extra bends paid for by fewer crossings.
func (t tally) noWorse(u tally) bool {
	return t.crossings <= u.crossings && t.jogs <= u.jogs && t.onTop <= u.onTop+0.5 &&
		bendCost*t.bends+crossCost*t.crossings <= bendCost*u.bends+crossCost*u.crossings
}

// diagramTally counts every route's bends, jogs and crossings and the
// length edges run on one line.
func (r *router) diagramTally() tally {
	var t tally
	short := 2 * r.c
	for a, ja := range r.jobs {
		pa := ja.e.Points
		t.bends += max(len(pa)-2, 0)
		t.jogs += jogCount(pa, short)
		for _, jb := range r.jobs[a+1:] {
			pb := jb.e.Points
			for i := 0; i+1 < len(pa); i++ {
				for k := 0; k+1 < len(pb); k++ {
					if properCross(pa[i], pa[i+1], pb[k], pb[k+1]) {
						t.crossings++
					}
					t.onTop += nearLen(pa[i], pa[i+1], pb[k], pb[k+1], onTopDist)
				}
			}
		}
	}
	return t
}

// clean reports whether j's route touches no obstacle.
func (r *router) clean(j *job) bool {
	r.removeEdge(j.i)
	ok := r.measurePath(j, j.e.Points).ok
	r.addPath(j.i, j.e.Points)
	return ok
}

// swappable reports whether ends a and b on one box may take each other's
// port: on the same side always, across sides only when neither end is
// pinned to its side.
func (r *router) swappable(a, b *end) bool {
	if a.side == b.side {
		return true
	}
	return !pinned(a) && !pinned(b)
}

// pinned reports whether en's side was set by the edge's Exit or Enter.
func pinned(en *end) bool {
	if en == en.j.src {
		return en.j.e.E.Exit != model.SideAuto
	}
	return en.j.e.E.Enter != model.SideAuto
}

// swapEnds gives a and b each other's port.
func swapEnds(a, b *end) {
	a.side, b.side = b.side, a.side
	a.pos, b.pos = b.pos, a.pos
	a.point, b.point = b.point, a.point
	a.travel, b.travel = b.travel, a.travel
}

// tangle rates how badly the routes of a and b get in each other's way:
// their crossings, and the length they run on one line, where they can
// swap sides unseen.
func (r *router) tangle(a, b *job) float64 {
	pa, pb := a.e.Points, b.e.Points
	v := 0.0
	for i := 0; i+1 < len(pa); i++ {
		for k := 0; k+1 < len(pb); k++ {
			if crossesAny(pa[i], pa[i+1], pb[k], pb[k+1]) {
				v += crossCost
			}
			v += nearLen(pa[i], pa[i+1], pb[k], pb[k+1], onTopDist)
		}
	}
	return v
}
