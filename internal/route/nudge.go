package route

import (
	"math"
	"slices"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

// nudge spreads apart parallel segments of different edges that run on top
// of each other or closer than ParallelOffset. Routes are searched on a
// grid that has one line per channel, so edges sharing a channel share a
// line; nudging gives each its own lane.
//
// Segments that overlap (directly or through others) form a cluster. Each
// cluster is ordered so that segments turning off early sit on the side
// they turn towards, which keeps the lanes from crossing, then spaced
// evenly around where they were, as far apart as the free space beside
// them allows. Segments attached to ports, and the segments of straight
// edges and self-loops, stay put; the others move around them.
func (r *router) nudge() {
	r.indexJobs()
	for _, vertical := range []bool{true, false} {
		r.nudgeAxis(vertical)
	}
}

// member is a segment taking part in nudging.
type member struct {
	j     *job
	k     int     // segment index in j.e.Points
	port  *end    // the end whose port slides when a port's segment moves
	c     float64 // coordinate across the segment
	lo    float64 // extent along it
	hi    float64
	fixed bool
	// Turns at the low and high ends: -1 towards smaller coordinates, +1
	// towards larger, 0 for an end at a port.
	turnLo, turnHi int
}

func (r *router) nudgeAxis(vertical bool) {
	var ms []*member
	for _, j := range r.jobs {
		pts := j.e.Points
		if len(pts) < 2 || j.err != nil {
			continue
		}
		movable := j.kind == kindRouted && len(j.e.E.Via) == 0
		for k := 0; k+1 < len(pts); k++ {
			a, b := pts[k], pts[k+1]
			if vertical && (a.X != b.X || a.Y == b.Y) || !vertical && (a.Y != b.Y || a.X == b.X) {
				continue
			}
			port := r.portEnd(j, k)
			m := &member{j: j, k: k, port: port, fixed: !movable || (k == 0 || k+2 == len(pts)) && port == nil}
			if vertical {
				m.c, m.lo, m.hi = a.X, math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)
			} else {
				m.c, m.lo, m.hi = a.Y, math.Min(a.X, b.X), math.Max(a.X, b.X)
			}
			m.turnLo, m.turnHi = turns(pts, k, vertical)
			ms = append(ms, m)
		}
	}
	// Cluster overlapping segments (union-find).
	parent := make([]int, len(ms))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	order := make([]int, len(ms))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		switch {
		case ms[a].c < ms[b].c:
			return -1
		case ms[a].c > ms[b].c:
			return 1
		}
		return 0
	})
	for x := range order {
		a := ms[order[x]]
		for y := x + 1; y < len(order); y++ {
			b := ms[order[y]]
			if b.c-a.c >= r.t.ParallelOffset-0.5 {
				break
			}
			if a.j == b.j || math.Min(a.hi, b.hi)-math.Max(a.lo, b.lo) <= 1 {
				continue
			}
			if a.fixed && b.fixed {
				continue
			}
			parent[find(order[x])] = find(order[y])
		}
	}
	groups := map[int][]*member{}
	var roots []int
	for _, i := range order {
		root := find(i)
		if _, ok := groups[root]; !ok {
			roots = append(roots, root)
		}
		groups[root] = append(groups[root], ms[i])
	}
	slices.Sort(roots)
	for _, root := range roots {
		g := groups[root]
		if len(g) < 2 {
			continue
		}
		r.nudgeCluster(g, vertical)
	}
}

// turns returns which way the route turns at each end of segment k:
// -1 towards smaller coordinates across the segment, +1 towards larger,
// and 0 where the segment ends at a port.
func turns(pts []model.Point, k int, vertical bool) (lo, hi int) {
	a, b := pts[k], pts[k+1]
	across := func(p, q model.Point) int {
		d := q.Y - p.Y
		if vertical {
			d = q.X - p.X
		}
		switch {
		case d > eps:
			return 1
		case d < -eps:
			return -1
		}
		return 0
	}
	ta, tb := 0, 0
	if k > 0 {
		ta = across(a, pts[k-1])
	}
	if k+2 < len(pts) {
		tb = across(b, pts[k+2])
	}
	along := b.Y - a.Y
	if !vertical {
		along = b.X - a.X
	}
	if along >= 0 {
		return ta, tb
	}
	return tb, ta
}

// before reports whether a should sit on the smaller-coordinate side of b
// (+1), the larger (-1), or either (0).
func before(a, b *member) int {
	v := 0
	// The segment that ends first at the low end turns off there while
	// the other carries on; it should be on the side it turns towards.
	switch {
	case a.lo > b.lo+0.5 && a.turnLo != 0:
		v -= a.turnLo
	case b.lo > a.lo+0.5 && b.turnLo != 0:
		v += b.turnLo
	case math.Abs(a.lo-b.lo) <= 0.5 && a.turnLo != b.turnLo:
		v += b.turnLo - a.turnLo
	}
	switch {
	case a.hi < b.hi-0.5 && a.turnHi != 0:
		v -= a.turnHi
	case b.hi < a.hi-0.5 && b.turnHi != 0:
		v += b.turnHi
	case math.Abs(a.hi-b.hi) <= 0.5 && a.turnHi != b.turnHi:
		v += b.turnHi - a.turnHi
	}
	return v
}

func (r *router) nudgeCluster(g []*member, vertical bool) {
	// Order: by pairwise preference, falling back to the current
	// coordinate and then declaration order (insertion sort, as the
	// preference isn't always transitive).
	base := func(a, b *member) bool {
		if a.c != b.c {
			return a.c < b.c
		}
		if a.j.i != b.j.i {
			return a.j.i < b.j.i
		}
		return a.k < b.k
	}
	sorted := []*member{}
	for _, m := range g {
		at := len(sorted)
		for i, o := range sorted {
			p := before(m, o)
			if p > 0 || p == 0 && base(m, o) {
				at = i
				break
			}
		}
		sorted = slices.Insert(sorted, at, m)
	}
	// Fixed segments can't move, so they keep their place in the order.
	var fixedCs []float64
	for _, m := range sorted {
		if m.fixed {
			fixedCs = append(fixedCs, m.c)
		}
	}
	if !slices.IsSorted(fixedCs) {
		slices.SortStableFunc(sorted, func(a, b *member) int {
			if a.fixed && b.fixed {
				switch {
				case a.c < b.c:
					return -1
				case a.c > b.c:
					return 1
				}
			}
			return 0
		})
	}

	n := len(sorted)
	lo, hi := make([]float64, n), make([]float64, n)
	for i, m := range sorted {
		if m.fixed {
			lo[i], hi[i] = m.c, m.c
			continue
		}
		lo[i], hi[i] = r.freeRange(m, vertical)
	}
	targets := make([]float64, n)
	for i, m := range sorted {
		targets[i] = m.c
	}
	fixed := make([]bool, n)
	for i, m := range sorted {
		fixed[i] = m.fixed
	}
	// Lanes spread evenly across the room the movable segments share,
	// at most maxLane apart, centred in it.
	room0, room1 := math.Inf(-1), math.Inf(1)
	movers := 0
	for i := range sorted {
		if !fixed[i] {
			room0, room1 = math.Max(room0, lo[i]), math.Min(room1, hi[i])
			movers++
		}
	}
	sp0 := r.t.ParallelOffset
	centre := math.NaN()
	if movers > 0 && room1 > room0 {
		sp0 = math.Max(sp0, math.Min(maxLane, (room1-room0)/float64(n+1)))
		centre = (room0 + room1) / 2
	}
	var pos []float64
	for pos == nil {
		for sp := sp0; sp >= 2.5 && pos == nil; sp *= 0.75 {
			pos = placeTracks(sorted, targets, lo, hi, fixed, sp, centre)
		}
		if pos != nil {
			break
		}
		// No room for everyone: the movable segment with the least room
		// stays where it is, and the rest try again.
		k := -1
		for i := range sorted {
			if !fixed[i] && (k < 0 || hi[i]-lo[i] < hi[k]-lo[k]) {
				k = i
			}
		}
		if k < 0 {
			break
		}
		fixed[k] = true
		lo[k], hi[k] = targets[k], targets[k]
	}
	if pos == nil {
		return
	}
	// Apply, keeping the moves only if they add no crossings.
	type saved struct {
		j   *job
		pts []model.Point
	}
	var olds []saved
	seen := map[*job]bool{}
	for _, m := range sorted {
		if !seen[m.j] {
			seen[m.j] = true
			olds = append(olds, saved{m.j, slices.Clone(m.j.e.Points)})
		}
	}
	before := 0.0
	for _, o := range olds {
		before += r.conflictOf(o.j)
	}
	for i, m := range sorted {
		if fixed[i] || pos[i] == m.c {
			continue
		}
		pts := m.j.e.Points
		if vertical {
			pts[m.k].X, pts[m.k+1].X = pos[i], pos[i]
		} else {
			pts[m.k].Y, pts[m.k+1].Y = pos[i], pos[i]
		}
		if en := m.port; en != nil {
			en.pos += pos[i] - m.c
			en.point = r.portAt(en.box, en.side, en.pos)
		}
	}
	after := 0.0
	for _, o := range olds {
		after += r.conflictOf(o.j)
	}
	if after > before+1e-6 {
		for _, o := range olds {
			o.j.e.Points = o.pts
		}
		for i, m := range sorted {
			if en := m.port; en != nil && !fixed[i] {
				en.pos -= pos[i] - m.c
				en.point = r.portAt(en.box, en.side, en.pos)
			}
		}
		return
	}
	for i, m := range sorted {
		m.c = pos[i]
	}
}

// placeTracks gives each segment, in order, the lowest position at least
// sp beyond every earlier segment it overlaps along its length, starting
// from a block centred where the overlapping segments were. Segments that
// don't overlap may share a position. It returns nil when a segment can't
// stay within its bounds.
func placeTracks(ms []*member, targets, lo, hi []float64, fixed []bool, sp, centre float64) []float64 {
	n := len(ms)
	overlaps := func(a, b *member) bool { return math.Min(a.hi, b.hi)-math.Max(a.lo, b.lo) > 1 }
	pos := make([]float64, n)
	for i, m := range ms {
		if fixed[i] {
			pos[i] = targets[i]
			continue
		}
		// Centre the block of segments overlapping this one.
		cnt, sum, rank := 1, targets[i], 0
		for k, o := range ms {
			if k == i || !overlaps(m, o) {
				continue
			}
			cnt++
			sum += targets[k]
			if k < i {
				rank++
			}
		}
		mid := sum / float64(cnt)
		if !math.IsNaN(centre) {
			mid = centre
		}
		p := mid + (float64(rank)-float64(cnt-1)/2)*sp
		p = math.Max(p, lo[i])
		for k := 0; k < i; k++ {
			if overlaps(m, ms[k]) {
				p = math.Max(p, pos[k]+sp)
			}
		}
		if p > hi[i]+eps {
			return nil
		}
		pos[i] = math.Round(p*100) / 100
	}
	// Fixed segments must not end up behind movable ones they overlap.
	for i := range ms {
		if !fixed[i] {
			continue
		}
		for k := range ms {
			if k != i && !fixed[k] && overlaps(ms[i], ms[k]) && math.Abs(pos[k]-pos[i]) < sp-eps {
				return nil
			}
		}
	}
	return pos
}

// freeRange is how far a movable segment can shift across its direction
// without running into an obstacle, crossing a group border or turning
// the segments on either side of it around.
func (r *router) freeRange(m *member, vertical bool) (lo, hi float64) {
	lo, hi = math.Inf(-1), math.Inf(1)
	pts := m.j.e.Points
	gap := float64(minClear)
	// The segment's extent, closed: the neighbouring segments grow or
	// shrink at its ends as it moves.
	ext := func(rc layout.Rect) (a, b, c0, c1 float64) {
		if vertical {
			return rc.Y, rc.Bottom(), rc.X, rc.Right()
		}
		return rc.X, rc.Right(), rc.Y, rc.Bottom()
	}
	limit := func(rc layout.Rect, g float64) {
		a, b, c0, c1 := ext(rc)
		if b < m.lo-g || a > m.hi+g {
			return
		}
		switch {
		case c1 <= m.c:
			lo = math.Max(lo, c1+g)
		case c0 >= m.c:
			hi = math.Min(hi, c0-g)
		}
	}
	for _, n := range r.nodes {
		limit(n.Shape, gap)
	}
	for _, t := range r.texts {
		limit(t.r, gap)
	}
	for _, h := range r.headers {
		limit(h.r, 2)
	}
	for _, bd := range r.bands {
		limit(bd, 2)
	}
	// Labels don't limit it: they are all placed again afterwards.
	// Group borders stay on the same side.
	for _, g := range r.groups {
		for _, ln := range borders(g.Rect) {
			lr := layout.Rect{X: ln[0].X, Y: ln[0].Y, W: ln[1].X - ln[0].X, H: ln[1].Y - ln[0].Y}
			limit(lr, r.c/2)
		}
	}
	if b := r.bounds; true {
		if vertical {
			lo, hi = math.Max(lo, b.X-2*r.c), math.Min(hi, b.Right()+2*r.c)
		} else {
			lo, hi = math.Max(lo, b.Y-2*r.c), math.Min(hi, b.Bottom()+2*r.c)
		}
	}
	lo, hi = math.Min(lo, m.c), math.Max(hi, m.c)
	// Neighbouring segments keep their direction, and none gets shorter
	// than twice the clearance (unless it already is), which would make a
	// jog. Next to a port, the stub keeps its length.
	for _, q := range []int{m.k - 1, m.k + 2} {
		if q < 0 || q >= len(pts) {
			continue
		}
		v := pts[q].Y
		if vertical {
			v = pts[q].X
		}
		d := 2*r.c + 1
		if q == 0 {
			d = r.wantStub(m.j, true)
		} else if q == len(pts)-1 {
			d = r.wantStub(m.j, false)
		}
		if v < m.c {
			lo = math.Max(lo, v+math.Min(d, m.c-v))
		} else {
			hi = math.Min(hi, v-math.Min(d, v-m.c))
		}
	}
	// A port's segment slides the port along its side, which keeps clear
	// of its corners and of the other ports there.
	if en := m.port; en != nil {
		a, z := r.sideSpan(en.box, en.side)
		a, z = a-en.pos+m.c, z-en.pos+m.c // in the segment's coordinate
		for _, o := range r.jobs {
			for _, oe := range []*end{o.src, o.dst, o.selfEnd[0], o.selfEnd[1]} {
				if oe == nil || oe == en || oe.box != en.box || oe.side != en.side {
					continue
				}
				if c := oe.pos - en.pos + m.c; c < m.c {
					a = math.Max(a, c+r.t.PortSpacing)
				} else {
					z = math.Min(z, c-r.t.PortSpacing)
				}
			}
		}
		lo, hi = math.Max(lo, math.Min(a, m.c)), math.Min(hi, math.Max(z, m.c))
	}
	return lo, hi
}

// maxLane is the widest spacing nudging gives parallel edges, so two of
// them in a wide channel don't drift to its opposite walls.
const maxLane = 32

// portEnd returns the end of j whose port segment k is, when that port can
// slide along a straight side, or nil.
func (r *router) portEnd(j *job, k int) *end {
	if j.kind != kindRouted || j.src == nil || len(j.e.E.Via) > 0 {
		return nil
	}
	var en *end
	switch {
	case k == 0:
		en = j.src
	case k+2 == len(j.e.Points):
		en = j.dst
	default:
		return nil
	}
	if !straightSide(en.box, en.side) {
		return nil
	}
	return en
}

// straightSide reports whether side s of b is a straight line, so a port
// sliding along it stays on the outline.
func straightSide(b *layout.Box, s model.Side) bool {
	if b.El.Kind != model.KindNode {
		return true
	}
	switch b.El.Shape {
	case model.ShapeCircle, model.ShapePerson:
		return false
	case model.ShapePill:
		return horiz(s)
	case model.ShapeCylinder:
		return !horiz(s)
	case model.ShapeQueue:
		return horiz(s)
	case model.ShapeDocument:
		return s != model.SideBottom
	}
	return !figure(b)
}

// conflictOf rates how j's route collides with the others: crossings,
// plus length run on top of or right beside another edge.
func (r *router) conflictOf(j *job) float64 {
	v := 0.0
	pa := j.e.Points
	w := r.t.ParallelOffset / 2
	for i := 0; i+1 < len(pa); i++ {
		a, b := pa[i], pa[i+1]
		r.jobIdx.query(inflate(segRect(a, b), nudgeReach), func(id int) {
			ref := r.jobSegs[id]
			if ref[0] == j.i {
				return
			}
			pb := r.jobs[ref[0]].e.Points
			c, d := pb[ref[1]], pb[ref[1]+1]
			if crossesAny(a, b, c, d) {
				v += crossCost
			}
			v += sharedCost * nearLen(a, b, c, d, w)
		})
	}
	return v
}

// sharedCost is what nudging counts per pixel of two edges on one line:
// lines drawn on top of each other are harder to follow than a crossing,
// so a nudge that trades 20px of shared line for a crossing is kept.
const sharedCost = 4

// nudgeReach bounds how far nudging moves a segment, for finding the
// segments a move can affect.
const nudgeReach = 120

// indexJobs indexes every route's segments, as (edge, segment) pairs.
func (r *router) indexJobs() {
	r.jobSegs = r.jobSegs[:0]
	r.jobIdx = newSpatial(r.indexArea(), 0)
	for i, j := range r.jobs {
		pts := j.e.Points
		for k := 0; k+1 < len(pts); k++ {
			r.jobIdx.insert(len(r.jobSegs), segRect(pts[k], pts[k+1]))
			r.jobSegs = append(r.jobSegs, [2]int{i, k})
		}
	}
}
