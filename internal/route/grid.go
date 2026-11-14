package route

import (
	"math"
	"slices"
	"sort"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

const eps = 1e-6

// grid is a sparse orthogonal routing grid with a cost for every unit
// segment between neighbouring points.
type grid struct {
	xs, ys     []float64
	nx, ny     int
	mulH, addH []float64 // horizontal segment (i,j)→(i+1,j) at j*(nx-1)+i
	mulV, addV []float64 // vertical segment (i,j)→(i,j+1) at j*nx+i
	restricted []bool    // points where a route may not turn
	cntH, cntV []uint8   // edges lying on each segment
}

var inf = math.Inf(1)

func (g *grid) hIdx(i, j int) int { return j*(g.nx-1) + i }
func (g *grid) vIdx(i, j int) int { return j*g.nx + i }

// coords collects grid coordinates. Fixed ones (obstacle borders, ports,
// waypoints) are kept exactly; loose ones (channel midlines, lanes beside
// other edges) are dropped when a coordinate already lies within looseGap,
// which keeps the grid small without losing any useful line.
type coords struct{ fixed, loose []float64 }

const looseGap = 2

func (c *coords) add(v ...float64)   { c.fixed = append(c.fixed, v...) }
func (c *coords) maybe(v ...float64) { c.loose = append(c.loose, v...) }

// finish sorts and dedupes the coordinates, dropping any outside [lo, hi].
func (c coords) finish(lo, hi float64) []float64 {
	dedupe := func(vs []float64, gap float64) []float64 {
		slices.Sort(vs)
		var out []float64
		for _, v := range vs {
			if v < lo-eps || v > hi+eps {
				continue
			}
			if n := len(out); n > 0 && v-out[n-1] < gap {
				continue
			}
			out = append(out, v)
		}
		return out
	}
	out := dedupe(c.fixed, 1e-3)
	for _, v := range dedupe(c.loose, looseGap) {
		i := sort.SearchFloat64s(out, v)
		if i < len(out) && out[i]-v < looseGap || i > 0 && v-out[i-1] < looseGap {
			continue
		}
		out = slices.Insert(out, i, v)
	}
	return out
}

func index(vs []float64, v float64) int {
	i := sort.SearchFloat64s(vs, v-1e-3)
	if i < len(vs) && math.Abs(vs[i]-v) <= 1e-3 {
		return i
	}
	return -1
}

// eachH calls fn for every horizontal segment inside the open rect.
func (g *grid) eachH(rc layout.Rect, fn func(k int, y float64)) {
	j0 := sort.Search(g.ny, func(j int) bool { return g.ys[j] > rc.Y+eps })
	i0 := sort.Search(g.nx-1, func(i int) bool { return g.xs[i+1] > rc.X+eps })
	for j := j0; j < g.ny && g.ys[j] < rc.Bottom()-eps; j++ {
		for i := i0; i < g.nx-1 && g.xs[i] < rc.Right()-eps; i++ {
			fn(g.hIdx(i, j), g.ys[j])
		}
	}
}

// eachV calls fn for every vertical segment inside the open rect.
func (g *grid) eachV(rc layout.Rect, fn func(k int, x float64)) {
	i0 := sort.Search(g.nx, func(i int) bool { return g.xs[i] > rc.X+eps })
	j0 := sort.Search(g.ny-1, func(j int) bool { return g.ys[j+1] > rc.Y+eps })
	for j := j0; j < g.ny-1 && g.ys[j] < rc.Bottom()-eps; j++ {
		for i := i0; i < g.nx && g.xs[i] < rc.Right()-eps; i++ {
			fn(g.vIdx(i, j), g.xs[i])
		}
	}
}

// crossX calls fn for every horizontal segment that crosses the vertical
// line at x between y0 and y1 (exclusive). Each crossing counts once.
func (g *grid) crossX(x, y0, y1 float64, fn func(k int)) {
	i := sort.Search(g.nx-1, func(i int) bool { return g.xs[i+1] >= x-eps })
	if i >= g.nx-1 || g.xs[i] >= x-eps {
		return
	}
	for j := sort.Search(g.ny, func(j int) bool { return g.ys[j] > y0+eps }); j < g.ny && g.ys[j] < y1-eps; j++ {
		fn(g.hIdx(i, j))
	}
}

// crossY is crossX for horizontal lines.
func (g *grid) crossY(y, x0, x1 float64, fn func(k int)) {
	j := sort.Search(g.ny-1, func(j int) bool { return g.ys[j+1] >= y-eps })
	if j >= g.ny-1 || g.ys[j] >= y-eps {
		return
	}
	for i := sort.Search(g.nx, func(i int) bool { return g.xs[i] > x0+eps }); i < g.nx && g.xs[i] < x1-eps; i++ {
		fn(g.vIdx(i, j))
	}
}

// region marks for a rect: block it, or add a per-pixel cost.
func (g *grid) block(rc layout.Rect) {
	g.eachH(rc, func(k int, _ float64) { g.mulH[k] = inf })
	g.eachV(rc, func(k int, _ float64) { g.mulV[k] = inf })
}

func (g *grid) penalize(rc layout.Rect, m float64) {
	g.eachH(rc, func(k int, _ float64) { g.mulH[k] += m })
	g.eachV(rc, func(k int, _ float64) { g.mulV[k] += m })
}

// crossings adds a fixed cost to every segment crossing rc's border.
func (g *grid) crossings(rc layout.Rect, cost float64) {
	h := func(k int) { g.addH[k] += cost }
	v := func(k int) { g.addV[k] += cost }
	g.crossX(rc.X, rc.Y, rc.Bottom(), h)
	g.crossX(rc.Right(), rc.Y, rc.Bottom(), h)
	g.crossY(rc.Y, rc.X, rc.Right(), v)
	g.crossY(rc.Bottom(), rc.X, rc.Right(), v)
}

// proximity adds a cost to segments running alongside rc, tapering from k
// at its border to nothing at distance w.
func (g *grid) proximity(rc layout.Rect, w, k float64) {
	g.eachH(layout.Rect{X: rc.X, Y: rc.Y - w, W: rc.W, H: rc.H + 2*w}, func(i int, y float64) {
		if d := math.Max(rc.Y-y, y-rc.Bottom()); d > eps {
			g.mulH[i] += k * (w - d) / w
		}
	})
	g.eachV(layout.Rect{X: rc.X - w, Y: rc.Y, W: rc.W + 2*w, H: rc.H}, func(i int, x float64) {
		if d := math.Max(rc.X-x, x-rc.Right()); d > eps {
			g.mulV[i] += k * (w - d) / w
		}
	})
}

// borderProximity adds a cost to segments running along rc's border on
// either side.
func (g *grid) borderProximity(rc layout.Rect, w, k float64) {
	for _, y := range []float64{rc.Y, rc.Bottom()} {
		g.eachH(layout.Rect{X: rc.X, Y: y - w, W: rc.W, H: 2 * w}, func(i int, yy float64) {
			g.mulH[i] += k * (w - math.Abs(yy-y)) / w
		})
	}
	for _, x := range []float64{rc.X, rc.Right()} {
		g.eachV(layout.Rect{X: x - w, Y: rc.Y, W: 2 * w, H: rc.H}, func(i int, xx float64) {
			g.mulV[i] += k * (w - math.Abs(xx-x)) / w
		})
	}
}

// outside applies f to every segment not within rc.
func (g *grid) outside(rc layout.Rect, f func(m *float64)) {
	for j := 0; j < g.ny; j++ {
		for i := 0; i < g.nx; i++ {
			x, y := g.xs[i], g.ys[j]
			if i+1 < g.nx && (y < rc.Y-eps || y > rc.Bottom()+eps || x < rc.X-eps || g.xs[i+1] > rc.Right()+eps) {
				f(&g.mulH[g.hIdx(i, j)])
			}
			if j+1 < g.ny && (x < rc.X-eps || x > rc.Right()+eps || y < rc.Y-eps || g.ys[j+1] > rc.Bottom()+eps) {
				f(&g.mulV[g.vIdx(i, j)])
			}
		}
	}
}

// edgeSeg adds the costs of crossing or running along a routed segment.
func (g *grid) edgeSeg(s seg, off float64) {
	// Each further edge on the same line costs more than the last, so a
	// channel takes a few edges (which nudging spreads apart) but not a
	// crowd.
	cost := func(d float64, cnt *uint8) float64 {
		switch {
		case d < 0.5:
			*cnt++
			return overlapCost * float64(*cnt)
		case d < off-0.5:
			return nearEdgeCost * 2 * (1 - d/off)
		}
		return besideCost
	}
	switch {
	case s.horizontal() && s.a.X != s.b.X:
		y := s.a.Y
		g.crossY(y, s.a.X, s.b.X, func(k int) { g.addV[k] += crossCost })
		g.eachH(layout.Rect{X: s.a.X, Y: y - 1.5*off, W: s.b.X - s.a.X, H: 3 * off}, func(k int, yy float64) {
			g.mulH[k] += cost(math.Abs(yy-y), &g.cntH[k])
		})
	case s.vertical() && s.a.Y != s.b.Y:
		x := s.a.X
		g.crossX(x, s.a.Y, s.b.Y, func(k int) { g.addH[k] += crossCost })
		g.eachV(layout.Rect{X: x - 1.5*off, Y: s.a.Y, W: 3 * off, H: s.b.Y - s.a.Y}, func(k int, xx float64) {
			g.mulV[k] += cost(math.Abs(xx-x), &g.cntV[k])
		})
	}
}

// corridor clears the straight run out of a port.
func (g *grid) corridor(c cand) {
	i, j := index(g.xs, c.p.X), index(g.ys, c.p.Y)
	if i < 0 || j < 0 {
		return
	}
	for {
		ni, nj := i+dx[c.travel], j+dy[c.travel]
		if ni < 0 || nj < 0 || ni >= g.nx || nj >= g.ny {
			return
		}
		if math.Abs(g.xs[i]-c.p.X)+math.Abs(g.ys[j]-c.p.Y) >= c.stub-eps {
			return
		}
		g.restricted[j*g.nx+i] = true
		switch c.travel {
		case dirRight:
			g.mulH[g.hIdx(i, j)] = 0
		case dirLeft:
			g.mulH[g.hIdx(ni, j)] = 0
		case dirDown:
			g.mulV[g.vIdx(i, j)] = 0
		case dirUp:
			g.mulV[g.vIdx(i, nj)] = 0
		}
		i, j = ni, nj
	}
}

// restrict marks the port and the points within its stub as no-turn
// points.
func (g *grid) restrict(c cand) {
	i, j := index(g.xs, c.p.X), index(g.ys, c.p.Y)
	for first := true; i >= 0 && j >= 0 && i < g.nx && j < g.ny; first = false {
		if !first && math.Abs(g.xs[i]-c.p.X)+math.Abs(g.ys[j]-c.p.Y) >= c.stub-eps {
			return
		}
		g.restricted[j*g.nx+i] = true
		i, j = i+dx[c.travel], j+dy[c.travel]
	}
}

// midlines returns the centre lines of the channels between pairs of
// obstacles that face each other.
func (r *router) midlines() (xs, ys []float64) {
	var rs []layout.Rect
	for _, n := range r.nodes {
		rs = append(rs, n.Shape)
	}
	for _, g := range r.groups {
		rs = append(rs, g.Rect)
	}
	reach := r.t.Gap
	// A channel between twice the clearance and one lane more already has
	// its two clearance lines a lane apart; a midline would only crowd
	// the grid.
	needMid := func(gap float64) bool {
		return gap < 2*r.c-eps || gap > 2*r.c+r.t.ParallelOffset+eps
	}
	for a, ra := range rs {
		for b, rb := range rs {
			if a == b || ra.Contains(rb) || rb.Contains(ra) {
				continue
			}
			if rb.X > ra.Right()+eps && rb.Y < ra.Bottom()+reach && ra.Y < rb.Bottom()+reach && needMid(rb.X-ra.Right()) {
				xs = append(xs, (ra.Right()+rb.X)/2)
			}
			if rb.Y > ra.Bottom()+eps && rb.X < ra.Right()+reach && ra.X < rb.Right()+reach && needMid(rb.Y-ra.Bottom()) {
				ys = append(ys, (ra.Bottom()+rb.Y)/2)
			}
		}
	}
	return xs, ys
}

// build makes the grid for one search. Level 0 treats every obstacle as
// solid; level 1 keeps only node shapes solid and makes the rest costly.
//
// The grid covers the diagram, or window when it is set.
func (r *router) build(j *job, srcs, dsts []cand, level int, window *layout.Rect) *grid {
	c := r.c
	off := r.t.ParallelOffset
	e := j.e
	var xs, ys coords
	addRect := func(rc layout.Rect) {
		xs.add(rc.X, rc.Right())
		ys.add(rc.Y, rc.Bottom())
	}
	outer := r.bounds
	for _, v := range e.E.Via {
		outer = union(outer, layout.Rect{X: v.X, Y: v.Y})
	}
	outer = inflate(outer, 2*c)
	if window != nil {
		outer = intersect(outer, *window)
	}
	addRect(outer)
	for _, n := range r.nodes {
		addRect(inflate(n.Shape, c))
	}
	for _, t := range r.texts {
		addRect(inflate(t.r, c))
	}
	for _, g := range r.groups {
		addRect(inflate(g.Rect, c))
		addRect(inflate(g.Rect, -c/2))
	}
	for _, h := range r.headers {
		addRect(inflate(h.r, c/2))
	}
	for _, l := range r.labels {
		addRect(inflate(l, c/2))
	}
	if r.midX == nil {
		r.midX, r.midY = r.midlines()
	}
	mx, my := r.midX, r.midY
	xs.maybe(mx...)
	ys.maybe(my...)
	for _, cs := range [][]cand{srcs, dsts} {
		for _, cd := range cs {
			xs.add(cd.p.X)
			ys.add(cd.p.Y)
			q := step(cd.p, cd.travel, cd.stub)
			xs.add(q.X)
			ys.add(q.Y)
		}
	}
	for _, v := range e.E.Via {
		xs.add(v.X)
		ys.add(v.Y)
	}
	g := &grid{xs: xs.finish(outer.X, outer.Right()), ys: ys.finish(outer.Y, outer.Bottom())}
	g.nx, g.ny = len(g.xs), len(g.ys)
	nh, nv := (g.nx-1)*g.ny, g.nx*(g.ny-1)
	sc := &r.scratch
	if cap(sc.mul) < nh+nv {
		sc.mul = make([]float64, nh+nv)
		sc.add = make([]float64, nh+nv)
	}
	if cap(sc.cnt) < nh+nv {
		sc.cnt = make([]uint8, nh+nv)
	}
	cnt := sc.cnt[:nh+nv]
	clear(cnt)
	g.cntH, g.cntV = cnt[:nh:nh], cnt[nh:]
	if cap(sc.restr) < g.nx*g.ny {
		sc.restr = make([]bool, g.nx*g.ny)
	}
	mul, addc := sc.mul[:nh+nv], sc.add[:nh+nv]
	clear(mul)
	clear(addc)
	g.mulH, g.mulV = mul[:nh:nh], mul[nh:]
	g.addH, g.addV = addc[:nh:nh], addc[nh:]
	g.restricted = sc.restr[:g.nx*g.ny]
	clear(g.restricted)

	soft := func(rc layout.Rect) {
		if level == 0 {
			g.block(rc)
		} else {
			g.penalize(rc, softZone)
		}
	}

	// A node's clearance is costly rather than solid, so channels
	// narrower than twice the clearance stay usable; only a thin margin
	// around the shape is solid.
	clearance := func(rc layout.Rect) {
		if level == 0 {
			g.penalize(inflate(rc, c), clearCost)
			g.block(inflate(rc, minClear))
		} else {
			g.penalize(inflate(rc, c), softZone)
			g.block(rc)
		}
	}

	g.outside(r.bounds, func(m *float64) { *m += outsideCost })
	g.crossings(r.bounds, outsideCross)
	if j.region != nil {
		in := inflate(j.region.Rect, -c/2)
		if level == 0 {
			g.outside(in, func(m *float64) { *m = inf })
		} else {
			g.outside(in, func(m *float64) { *m += softZone })
		}
	}
	isEnd := func(b *layout.Box) bool { return b == e.From || b == e.To }
	innerGroup := func(b *layout.Box) bool {
		return b == e.From && j.srcInner || b == e.To && j.dstInner
	}
	for _, b := range []*layout.Box{e.From, e.To} {
		if !innerGroup(b) {
			clearance(b.Shape)
		}
	}
	for _, n := range r.nodes {
		if !isEnd(n) {
			clearance(n.Shape)
		}
	}
	// Text under a figure keeps the same clearance as a node's shape.
	for _, t := range r.texts {
		clearance(t.r)
	}
	for _, bd := range r.bands {
		soft(bd)
	}
	for _, gb := range r.groups {
		if slices.Contains(j.allowed, gb) {
			inFrom, inTo := within(e.From.El, gb.El), within(e.To.El, gb.El)
			switch {
			case !inFrom && !inTo:
				// CrossGroups: a group holding neither end.
				g.crossings(gb.Rect, foreignCross)
			case !inFrom || !inTo:
				g.crossings(gb.Rect, groupCross)
			}
			continue
		}
		if level == 0 {
			g.block(inflate(gb.Rect, c))
		} else {
			g.penalize(gb.Rect, softInside)
			g.crossings(gb.Rect, softCross)
		}
	}
	for _, h := range r.headers {
		soft(inflate(h.r, 4))
	}
	for _, l := range r.labels {
		rc := inflate(l, 4)
		// A label that sits on one of this edge's ports can't be avoided.
		onPort := false
		for _, cs := range [][]cand{srcs, dsts} {
			for _, cd := range cs {
				onPort = onPort || overlap(rc, stubRect(cd.p, cd.travel, cd.stub)) > 0
			}
		}
		if level == 0 && !onPort {
			g.block(rc)
			continue
		}
		g.penalize(rc, softZone)
		g.crossings(rc, softCross)
	}
	// The straight run out of each port stays open whatever it starts in:
	// ports on curved outlines sit inside the shape's box, and ports under
	// a figure's text start inside the text's margin.
	for _, cs := range [][]cand{srcs, dsts} {
		for _, cd := range cs {
			g.corridor(cd)
		}
	}
	for _, n := range r.nodes {
		g.proximity(n.Shape, 2*c, proximityCost)
	}
	for _, t := range r.texts {
		g.proximity(t.r, 2*c, proximityCost)
	}
	for _, gb := range r.groups {
		g.borderProximity(gb.Rect, c, borderCost)
	}
	if win := (layout.Rect{X: g.xs[0], Y: g.ys[0], W: g.xs[g.nx-1] - g.xs[0], H: g.ys[g.ny-1] - g.ys[0]}); win.W*win.H < r.bounds.W*r.bounds.H/3 {
		r.segsNear(win, func(s seg) { g.edgeSeg(s, off) })
	} else {
		for _, s := range r.segs {
			g.edgeSeg(s, off)
		}
	}
	for _, cs := range [][]cand{srcs, dsts} {
		for _, cd := range cs {
			g.restrict(cd)
		}
	}
	return g
}

// ---- search ----

type gport struct {
	pt     int
	travel dir
	cost   float64
}

type item struct {
	f, g float64
	s    int32
	seq  uint32
}

// pq is a 4-ary min-heap of items ordered by f, then insertion order.
type pq []item

func less(a, b *item) bool {
	if a.f != b.f {
		return a.f < b.f
	}
	// Among equal estimates, the state furthest along goes first, which
	// keeps the search from flooding plateaus of equally good routes.
	if a.g != b.g {
		return a.g > b.g
	}
	return a.seq < b.seq
}

func (q *pq) push(it item) {
	h := append(*q, it)
	i := len(h) - 1
	for i > 0 {
		p := (i - 1) / 4
		if !less(&it, &h[p]) {
			break
		}
		h[i] = h[p]
		i = p
	}
	h[i] = it
	*q = h
}

func (q *pq) pop() item {
	h := *q
	top := h[0]
	n := len(h) - 1
	last := h[n]
	h = h[:n]
	if n > 0 {
		i := 0
		for {
			c := 4*i + 1
			if c >= n {
				break
			}
			m := c
			for k := c + 1; k < c+4 && k < n; k++ {
				if less(&h[k], &h[m]) {
					m = k
				}
			}
			if !less(&h[m], &last) {
				break
			}
			h[i] = h[m]
			i = m
		}
		h[i] = last
	}
	*q = h
	return top
}

// cell is a search state's best known cost and predecessor, valid only
// when its stamp matches the current generation.
type cell struct {
	dist  float64
	prev  int32
	stamp uint32
}

// scratch holds search buffers reused across searches.
type scratch struct {
	cells []cell
	gen   uint32
	heap  pq

	// Grid buffers, reused by build.
	mul, add []float64
	restr    []bool
	cnt      []uint8
}

func (sc *scratch) reset(n int) {
	if len(sc.cells) < n {
		sc.cells = make([]cell, n+n/4)
		sc.gen = 0
	}
	sc.gen++
	if sc.gen == 0 { // wrapped
		clear(sc.cells)
		sc.gen = 1
	}
	sc.heap = sc.heap[:0]
}

// goal is a target port prepared for the heuristic.
type goal struct {
	pt     int
	x, y   float64
	arrive dir
	vert   bool
	cost   float64
}

// astar finds the cheapest path from one of srcs to one of dsts through
// the vias in order. Routes start moving in a source's travel direction
// and arrive at a target moving against its travel direction. It returns
// grid point indices and the indices of the chosen source and target.
func (g *grid) astar(sc *scratch, srcs, dsts []gport, vias []int) (path []int, si, di int, cost float64, ok bool) {
	nx := g.nx
	npts := nx * g.ny
	nv := len(vias)
	// A state is a point and how the route arrived there. Without
	// waypoints only the axis matters: turning costs a bend, and reversing
	// never pays. With waypoints a route may need to double back past one,
	// so the direction is kept and reversing is forbidden.
	nd4 := nv > 0
	nstates := 2
	if nd4 {
		nstates = 4
	}
	base := (nv + 1) * npts * nstates
	sc.reset(base + len(dsts))
	cells := sc.cells
	gen := sc.gen
	xs, ys := g.xs, g.ys

	goals := make([]goal, len(dsts))
	for k, dp := range dsts {
		a := dp.travel.opposite()
		goals[k] = goal{pt: dp.pt, x: xs[dp.pt%nx], y: ys[dp.pt/nx], arrive: a, vert: a == dirUp || a == dirDown, cost: dp.cost}
	}
	// toGoal is a lower bound on the cost from (x, y) moving in d to the
	// nearest target: distance plus the bends needed to arrive the right
	// way.
	toGoal := func(x, y float64, d dir) float64 {
		best := inf
		vert := d == dirUp || d == dirDown
		for k := range goals {
			gl := &goals[k]
			c := math.Abs(x-gl.x) + math.Abs(y-gl.y)
			if gl.vert && x == gl.x || !gl.vert && y == gl.y {
				if vert != gl.vert {
					c += bendCost
				}
			} else if vert == gl.vert {
				c += 2 * bendCost
			} else {
				c += bendCost
			}
			if c < best {
				best = c
			}
		}
		return best
	}
	man := func(a, b int) float64 {
		return math.Abs(xs[a%nx]-xs[b%nx]) + math.Abs(ys[a/nx]-ys[b/nx])
	}
	chain := make([]float64, nv+1)
	for k := nv - 1; k >= 0; k-- {
		if k == nv-1 {
			best := inf
			for _, gl := range goals {
				best = math.Min(best, man(vias[k], gl.pt))
			}
			chain[k] = best
		} else {
			chain[k] = man(vias[k], vias[k+1]) + chain[k+1]
		}
	}
	h := func(p, leg int, d dir) float64 {
		x, y := xs[p%nx], ys[p/nx]
		if leg < nv {
			v := vias[leg]
			vx, vy := xs[v%nx], ys[v/nx]
			b := 0.0
			if x != vx && y != vy {
				b = bendCost
			}
			return math.Abs(x-vx) + math.Abs(y-vy) + b + chain[leg]
		}
		return toGoal(x, y, d)
	}
	advance := func(p, leg int) int {
		for leg < nv && vias[leg] == p {
			leg++
		}
		return leg
	}
	state := func(p, leg int, d dir) int {
		if nd4 {
			return (leg*npts+p)*4 + int(d)
		}
		return (leg*npts+p)*2 + int(d&1)
	}

	q := &sc.heap
	var seq uint32
	push := func(s int, gc, f float64, from int) {
		c := &cells[s]
		if c.stamp == gen && gc >= c.dist {
			return
		}
		c.dist, c.stamp, c.prev = gc, gen, int32(from)
		q.push(item{f: f, g: gc, s: int32(s), seq: seq})
		seq++
	}
	for _, sp := range srcs {
		leg := advance(sp.pt, 0)
		push(state(sp.pt, leg, sp.travel), sp.cost, sp.cost+h(sp.pt, leg, sp.travel), -1)
	}
	for len(*q) > 0 {
		it := q.pop()
		st := int(it.s)
		if it.g > cells[st].dist {
			continue
		}
		if st >= base {
			di = st - base
			var states []int
			for s := int(cells[st].prev); s >= 0; s = int(cells[s].prev) {
				states = append(states, s)
			}
			slices.Reverse(states)
			for _, s := range states {
				p := (s / nstates) % npts
				if n := len(path); n == 0 || path[n-1] != p {
					path = append(path, p)
				}
			}
			first := states[0]
			for k, sp := range srcs {
				if state(sp.pt, advance(sp.pt, 0), sp.travel) == first {
					si = k
					break
				}
			}
			return path, si, di, it.g, true
		}
		sd := dir(st % nstates) // the arrival direction, or just its axis
		p := (st / nstates) % npts
		leg := (st / nstates) / npts
		axis := int(sd & 1) // 0 horizontal, 1 vertical
		i, jj := p%nx, p/nx
		x, y := xs[i], ys[jj]
		restr := g.restricted[p]
		for nd := dir(0); nd < 4; nd++ {
			if nd4 && (nd == sd.opposite() || restr && nd != sd) || !nd4 && restr && int(nd&1) != axis {
				continue
			}
			var k, np int
			var mul, add, l float64
			switch nd {
			case dirRight:
				if i+1 >= nx {
					continue
				}
				k = jj*(nx-1) + i
				mul, add = g.mulH[k], g.addH[k]
				np, l = p+1, xs[i+1]-x
			case dirLeft:
				if i == 0 {
					continue
				}
				k = jj*(nx-1) + i - 1
				mul, add = g.mulH[k], g.addH[k]
				np, l = p-1, x-xs[i-1]
			case dirDown:
				if jj+1 >= g.ny {
					continue
				}
				k = p
				mul, add = g.mulV[k], g.addV[k]
				np, l = p+nx, ys[jj+1]-y
			default:
				if jj == 0 {
					continue
				}
				k = p - nx
				mul, add = g.mulV[k], g.addV[k]
				np, l = p-nx, y-ys[jj-1]
			}
			if mul == inf {
				continue
			}
			c := it.g + l*(1+mul) + add
			if int(nd&1) != axis {
				c += bendCost
			}
			nl := leg
			if nv > 0 {
				nl = advance(np, leg)
			}
			s := state(np, nl, nd)
			if cs := &cells[s]; cs.stamp == gen && c >= cs.dist {
				continue
			}
			push(s, c, c+h(np, nl, nd), st)
			if nl == nv {
				for k := range goals {
					if gl := &goals[k]; gl.pt == np && nd == gl.arrive {
						push(base+k, c+gl.cost, c+gl.cost, s)
					}
				}
			}
		}
	}
	return nil, 0, 0, 0, false
}

// search routes j between the candidate ports. It tries a window around
// the ports and waypoints first, then the whole diagram, then the whole
// diagram with soft obstacles.
func (r *router) search(j *job, srcs, dsts []cand) (pts []model.Point, si, di int, err error) {
	var box layout.Rect
	for i, c := range append(append([]cand{}, srcs...), dsts...) {
		pr := layout.Rect{X: c.p.X, Y: c.p.Y}
		if i == 0 {
			box = pr
		} else {
			box = union(box, pr)
		}
	}
	for _, v := range j.e.E.Via {
		box = union(box, layout.Rect{X: v.X, Y: v.Y})
	}
	win := inflate(box, 2*r.t.Gap+math.Max(box.W, box.H)/4)
	attempts := []attempt{{0, &win}, {0, nil}, {1, nil}}
	if win.Contains(inflate(r.bounds, 2*r.c)) {
		attempts = attempts[1:]
	}
	return r.searchIn(j, srcs, dsts, attempts)
}

type attempt struct {
	level  int
	window *layout.Rect
}

// searchIn routes j trying each attempt's grid in turn.
func (r *router) searchIn(j *job, srcs, dsts []cand, attempts []attempt) (pts []model.Point, si, di int, err error) {
	for n, at := range attempts {
		g := r.build(j, srcs, dsts, at.level, at.window)
		gp := func(cs []cand) []gport {
			var out []gport
			for _, c := range cs {
				i, k := index(g.xs, c.p.X), index(g.ys, c.p.Y)
				if i < 0 || k < 0 {
					continue
				}
				out = append(out, gport{pt: k*g.nx + i, travel: c.travel, cost: c.cost})
			}
			return out
		}
		var vias []int
		for _, v := range j.e.E.Via {
			i, k := index(g.xs, v.X), index(g.ys, v.Y)
			if i >= 0 && k >= 0 {
				vias = append(vias, k*g.nx+i)
			}
		}
		path, si, di, _, ok := g.astar(&r.scratch, gp(srcs), gp(dsts), vias)
		if !ok {
			if n == len(attempts)-1 {
				return nil, 0, 0, r.noRoute(j, g, srcs, dsts)
			}
			continue
		}
		for _, p := range path {
			pts = append(pts, model.Point{X: g.xs[p%g.nx], Y: g.ys[p/g.nx]})
		}
		pts = snapEnds(pts, srcs[si].p, dsts[di].p)
		return simplify(pts), si, di, nil
	}
	panic("unreachable")
}

func intersect(a, b layout.Rect) layout.Rect {
	x, y := math.Max(a.X, b.X), math.Max(a.Y, b.Y)
	return layout.Rect{X: x, Y: y, W: math.Max(math.Min(a.Right(), b.Right())-x, 0), H: math.Max(math.Min(a.Bottom(), b.Bottom())-y, 0)}
}

// snapEnds moves the ends of a path exactly onto its ports, carrying the
// perpendicular stubs along.
func snapEnds(pts []model.Point, a, b model.Point) []model.Point {
	fix := func(i, nb int, p model.Point) {
		old := pts[i]
		pts[i] = p
		if nb < 0 || nb >= len(pts) {
			return
		}
		if math.Abs(pts[nb].X-old.X) < 1e-6 {
			pts[nb].X = p.X
		} else if math.Abs(pts[nb].Y-old.Y) < 1e-6 {
			pts[nb].Y = p.Y
		}
	}
	fix(0, 1, a)
	fix(len(pts)-1, len(pts)-2, b)
	return pts
}

// noRoute explains why no route exists.
func (r *router) noRoute(j *job, g *grid, srcs, dsts []cand) error {
	e := j.e.E
	stuck := func(cs []cand, b *layout.Box) bool {
		for _, c := range cs {
			if g.escapes(c, inflate(b.Shape, 2*r.c)) {
				return false
			}
		}
		return true
	}
	switch {
	case e.Exit != model.SideAuto && stuck(srcs, j.e.From):
		return r.edgeErr(j, "no route from exit %s", e.Exit)
	case e.Enter != model.SideAuto && stuck(dsts, j.e.To):
		return r.edgeErr(j, "no route to entry %s", e.Enter)
	case stuck(srcs, j.e.From):
		return r.edgeErr(j, "no route out of %s", j.e.From.El.Describe())
	case stuck(dsts, j.e.To):
		return r.edgeErr(j, "no route into %s", j.e.To.El.Describe())
	case len(e.Via) > 0:
		return r.edgeErr(j, "no route through its waypoints")
	}
	return r.edgeErr(j, "no route")
}

// escapes reports whether open segments lead from port c, leaving in its
// travel direction, to some point outside rc.
func (g *grid) escapes(c cand, rc layout.Rect) bool {
	i, k := index(g.xs, c.p.X), index(g.ys, c.p.Y)
	if i < 0 || k < 0 {
		return false
	}
	open := func(i, k int, d dir) (int, int, bool) {
		ni, nk := i+dx[d], k+dy[d]
		if ni < 0 || nk < 0 || ni >= g.nx || nk >= g.ny {
			return 0, 0, false
		}
		var m float64
		switch d {
		case dirRight:
			m = g.mulH[g.hIdx(i, k)]
		case dirLeft:
			m = g.mulH[g.hIdx(ni, k)]
		case dirDown:
			m = g.mulV[g.vIdx(i, k)]
		default:
			m = g.mulV[g.vIdx(i, nk)]
		}
		return ni, nk, !math.IsInf(m, 1)
	}
	ni, nk, ok := open(i, k, c.travel)
	if !ok {
		return false
	}
	seen := make([]bool, g.nx*g.ny)
	seen[k*g.nx+i] = true
	stack := []int{nk*g.nx + ni}
	seen[nk*g.nx+ni] = true
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		pi, pk := p%g.nx, p/g.nx
		if !contains(rc, model.Point{X: g.xs[pi], Y: g.ys[pk]}) {
			return true
		}
		for d := dir(0); d < 4; d++ {
			if qi, qk, ok := open(pi, pk, d); ok && !seen[qk*g.nx+qi] {
				seen[qk*g.nx+qi] = true
				stack = append(stack, qk*g.nx+qi)
			}
		}
	}
	return false
}

// stubRect is a thin box around the straight run out of a port.
func stubRect(p model.Point, d dir, l float64) layout.Rect {
	q := step(p, d, l)
	rc := layout.Rect{X: math.Min(p.X, q.X), Y: math.Min(p.Y, q.Y), W: math.Abs(q.X - p.X), H: math.Abs(q.Y - p.Y)}
	return inflate(rc, 2)
}
