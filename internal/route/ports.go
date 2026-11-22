package route

import (
	"cmp"
	"math"
	"slices"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

// dir is a direction of travel on the grid.
type dir uint8

const (
	dirRight dir = iota
	dirDown
	dirLeft
	dirUp
)

var (
	dx = [4]int{1, 0, -1, 0}
	dy = [4]int{0, 1, 0, -1}
)

func (d dir) opposite() dir { return (d + 2) % 4 }

// dirOf returns a side's outward normal.
func dirOf(s model.Side) dir {
	switch s {
	case model.SideTop:
		return dirUp
	case model.SideLeft:
		return dirLeft
	case model.SideBottom:
		return dirDown
	}
	return dirRight
}

// horiz reports whether a side runs horizontally, so positions along it
// are x coordinates.
func horiz(s model.Side) bool { return s == model.SideTop || s == model.SideBottom }

// sides in tie-break order: vertical connections win ties.
var sides = [4]model.Side{model.SideBottom, model.SideTop, model.SideRight, model.SideLeft}

// cand is a candidate port.
type cand struct {
	side    model.Side
	pos     float64 // coordinate along the side
	p       model.Point
	travel  dir // direction the route takes away from the box
	cost    float64
	aligned bool // pos lines up with the other end
	stub    float64
}

// end is one end of a routed edge after the first pass.
type end struct {
	j      *job
	order  int // 0 source, 1 target
	box    *layout.Box
	side   model.Side
	travel dir
	key    float64
	ideal  float64
	anchor bool
	loop   bool // second end of a self-loop
	pos    float64
	point  model.Point
}

// sideSpan returns the part of a side that ports may use, keeping clear of
// rounded corners and curved outlines.
func (r *router) sideSpan(b *layout.Box, s model.Side) (lo, hi float64) {
	rc := r.portRect(b, s)
	a, z := rc.Y, rc.Bottom()
	if horiz(s) {
		a, z = rc.X, rc.Right()
	}
	m := math.Max(b.Look.Radius, 3)
	if b.El.Kind == model.KindNode {
		switch b.El.Shape {
		case model.ShapeCircle:
			m = (z - a) * 0.3
		case model.ShapePill:
			if horiz(s) {
				m = rc.H / 2
			} else {
				m = rc.H * 0.3
			}
		case model.ShapeCylinder:
			if horiz(s) {
				m = (z - a) * 0.15
			}
		case model.ShapeQueue:
			if !horiz(s) {
				m = (z - a) * 0.3
			}
		case model.ShapeText:
			m = 2
		case model.ShapePerson:
			switch s {
			case model.SideTop:
				m = (z - a) / 2
			case model.SideBottom:
				m = 4
			default:
				// Below the shoulders.
				a = rc.Y + 2*rc.W*0.24 + 4 + rc.W*0.3
				z -= 2
				m = 0
			}
		}
	}
	m = math.Min(m, (z-a)/2)
	return r.clearOfMarks(b, s, a+m, z-m)
}

// clearOfMarks narrows a side's port span [lo, hi] to the longest stretch
// not under one of the node's badges or its status dot, keeping
// PortSpacing/2 from them, when that stretch is still at least
// minMarkSpan of the side; otherwise it leaves the span alone.
func (r *router) clearOfMarks(b *layout.Box, s model.Side, lo, hi float64) (float64, float64) {
	marks := b.Marks()
	if len(marks) == 0 || hi <= lo {
		return lo, hi
	}
	pad := r.t.PortSpacing / 2
	sh := b.Shape
	type iv struct{ a, z float64 }
	var cover []iv
	for _, m := range marks {
		// A mark covers the side where it reaches across the side's line.
		switch s {
		case model.SideTop:
			if m.Y <= sh.Y && m.Bottom() >= sh.Y {
				cover = append(cover, iv{m.X - pad, m.Right() + pad})
			}
		case model.SideBottom:
			if m.Y <= sh.Bottom() && m.Bottom() >= sh.Bottom() {
				cover = append(cover, iv{m.X - pad, m.Right() + pad})
			}
		case model.SideLeft:
			if m.X <= sh.X && m.Right() >= sh.X {
				cover = append(cover, iv{m.Y - pad, m.Bottom() + pad})
			}
		default:
			if m.X <= sh.Right() && m.Right() >= sh.Right() {
				cover = append(cover, iv{m.Y - pad, m.Bottom() + pad})
			}
		}
	}
	if len(cover) == 0 {
		return lo, hi
	}
	slices.SortFunc(cover, func(x, y iv) int { return cmp.Compare(x.a, y.a) })
	bestLo, bestHi := lo, lo
	at := lo
	for _, c := range append(cover, iv{hi, hi}) {
		if end := math.Min(c.a, hi); end-at > bestHi-bestLo {
			bestLo, bestHi = at, end
		}
		at = math.Max(at, c.z)
	}
	if bestHi-bestLo < minMarkSpan*(hi-lo) {
		return lo, hi
	}
	return bestLo, bestHi
}

// minMarkSpan is the least share of a side that clearOfMarks leaves for
// ports.
const minMarkSpan = 0.4

// portAt returns the point on a box's outline at position pos along side s.
func (rt *router) portAt(b *layout.Box, s model.Side, pos float64) model.Point {
	r := rt.portRect(b, s)
	var p model.Point
	switch s {
	case model.SideTop:
		p = model.Point{X: pos, Y: r.Y}
	case model.SideBottom:
		p = model.Point{X: pos, Y: r.Bottom()}
	case model.SideLeft:
		p = model.Point{X: r.X, Y: pos}
	default:
		p = model.Point{X: r.Right(), Y: pos}
	}
	if b.El.Kind != model.KindNode {
		return p
	}
	ell := func(off, rad float64) float64 { // depth of an ellipse below its tangent
		if rad <= 0 {
			return 0
		}
		u := off / rad
		return rad * (1 - math.Sqrt(math.Max(0, 1-u*u)))
	}
	c := r.Center()
	switch b.El.Shape {
	case model.ShapeCircle:
		rad := r.W / 2
		switch s {
		case model.SideTop:
			p.Y += ell(pos-c.X, rad)
		case model.SideBottom:
			p.Y -= ell(pos-c.X, rad)
		case model.SideLeft:
			p.X += ell(pos-c.Y, rad)
		default:
			p.X -= ell(pos-c.Y, rad)
		}
	case model.ShapePill:
		rad := r.H / 2
		switch s {
		case model.SideLeft:
			p.X += ell(pos-c.Y, rad)
		case model.SideRight:
			p.X -= ell(pos-c.Y, rad)
		}
	case model.ShapeCylinder:
		if horiz(s) {
			d := ell(pos-c.X, r.W/2) * b.Cap / (r.W / 2)
			if s == model.SideTop {
				p.Y += d
			} else {
				p.Y -= d
			}
		}
	case model.ShapeQueue:
		if !horiz(s) {
			d := ell(pos-c.Y, r.H/2) * b.Cap / (r.H / 2)
			if s == model.SideLeft {
				p.X += d
			} else {
				p.X -= d
			}
		}
	case model.ShapePerson:
		if s == model.SideTop {
			p.Y++
		}
	case model.ShapeDocument:
		if s == model.SideBottom {
			u := (pos - r.X) / r.W
			p.Y = r.Bottom() - b.Wave/2 - 0.825*b.Wave*math.Sin(2*math.Pi*u)
		}
	}
	return p
}

// sideGap is how far other lies beyond side s of r (negative when it
// doesn't).
func sideGap(r, other layout.Rect, s model.Side) float64 {
	switch s {
	case model.SideTop:
		return r.Y - other.Bottom()
	case model.SideBottom:
		return other.Y - r.Bottom()
	case model.SideLeft:
		return r.X - other.Right()
	}
	return other.X - r.Right()
}

// sideCoord returns the coordinate of side s of r.
func sideCoord(r layout.Rect, s model.Side) float64 {
	switch s {
	case model.SideTop:
		return r.Y
	case model.SideBottom:
		return r.Bottom()
	case model.SideLeft:
		return r.X
	}
	return r.Right()
}

func oppositeSide(s model.Side) model.Side {
	return [...]model.Side{model.SideAuto, model.SideBottom, model.SideLeft, model.SideTop, model.SideRight}[s]
}

// candidates returns the ports one end of j may use in the first pass.
func (r *router) candidates(j *job, src bool) []cand {
	e := j.e
	box, other := e.From, e.To
	pin := e.E.Exit
	groupInner, childInner := j.srcInner, j.dstInner
	var via *model.Point
	if len(e.E.Via) > 0 {
		v := e.E.Via[0]
		via = &v
	}
	if !src {
		box, other = e.To, e.From
		pin = e.E.Enter
		groupInner, childInner = j.dstInner, j.srcInner
		if len(e.E.Via) > 0 {
			v := e.E.Via[len(e.E.Via)-1]
			via = &v
		}
	}
	otherRect := other.Shape
	inner := groupInner || childInner
	if via != nil {
		otherRect = layout.Rect{X: via.X, Y: via.Y}
		inner = groupInner
	}

	type scored struct {
		s     model.Side
		score float64
	}
	var ss []scored
	for _, s := range sides {
		if pin != model.SideAuto && s != pin {
			continue
		}
		or := otherRect
		if via == nil {
			or = r.portRect(other, oppositeSide(s))
		}
		sc := sideGap(r.portRect(box, s), or, s)
		if inner {
			sc = -math.Abs(sideCoord(r.portRect(box, s), s) - sideCoord(otherRect, s))
		}
		ss = append(ss, scored{s, sc})
	}
	slices.SortStableFunc(ss, func(a, b scored) int {
		switch {
		case a.score > b.score+1e-6:
			return -1
		case b.score > a.score+1e-6:
			return 1
		}
		return 0
	})

	var out []cand
	for rank, sc := range ss {
		s := sc.s
		c := cand{side: s, travel: dirOf(s)}
		if groupInner {
			c.travel = c.travel.opposite()
		}
		// Sides are chosen by what their routes actually cost; the rank
		// only breaks ties, in favour of sides facing the other end.
		c.cost = float64(rank) * sideTie
		if inner {
			c.cost = float64(rank) * bendCost * 0.5
		}
		c.cost += r.crowding(box, s)
		if s == box.LoopSide && r.hasBand(box) {
			c.cost += loopSideCost
		}
		lo, hi := r.sideSpan(box, s)
		c.pos = (lo + hi) / 2
		// Line up with the other end when the two face each other.
		olo, ohi := 0.0, 0.0
		faces := false
		switch {
		case via != nil:
			olo, ohi = via.X, via.X
			if !horiz(s) {
				olo, ohi = via.Y, via.Y
			}
			faces = inner || sc.score > 0
		case inner:
			olo, ohi = r.sideSpan(other, s)
			faces = true
		default:
			olo, ohi = r.sideSpan(other, oppositeSide(s))
			faces = sc.score > 0
		}
		if faces {
			a, b := math.Max(lo, olo), math.Min(hi, ohi)
			if a <= b+1e-9 {
				c.pos = (a + b) / 2
				c.aligned = true
			}
		}
		c.p = r.portAt(box, s, c.pos)
		c.stub = r.stubLen(j, c.p, c.travel, box, s, r.wantStub(j, src))
		out = append(out, c)
	}
	return out
}

// wantStub is how long the straight run at one end of j should be: the
// clearance, or enough for the arrowhead and a rounded bend.
func (r *router) wantStub(j *job, src bool) float64 {
	k := j.e.Head
	if src {
		k = j.e.Tail
	}
	size := r.t.ArrowSize * math.Max(1, j.e.Width/1.5)
	var l float64
	switch k {
	case model.ArrowNone:
		return r.c
	case model.ArrowDiamond:
		l = 1.5 * size
	case model.ArrowOpen:
		l = 0.5 * size
	default:
		l = 0.8 * size
	}
	return math.Max(r.c, l+r.t.EdgeCornerRad+4)
}

// stubLen is how far a route must run straight out of a port before it may
// turn: want, or half the free distance to the nearest obstacle ahead when
// that is less.
//
// The length is measured from the side of the shape's box, so ports set
// into a curved outline get a longer stub.
func (r *router) stubLen(j *job, p model.Point, d dir, self *layout.Box, s model.Side, want float64) float64 {
	pr := r.portRect(self, s)
	inset := math.Abs(sideCoord(pr, s) - p.X)
	if horiz(s) {
		inset = math.Abs(sideCoord(pr, s) - p.Y)
	}
	free := math.Inf(1)
	hit := func(rc layout.Rect) {
		var dist float64
		switch d {
		case dirRight, dirLeft:
			if p.Y <= rc.Y || p.Y >= rc.Bottom() {
				return
			}
			if d == dirRight {
				dist = rc.X - p.X
			} else {
				dist = p.X - rc.Right()
			}
		default:
			if p.X <= rc.X || p.X >= rc.Right() {
				return
			}
			if d == dirDown {
				dist = rc.Y - p.Y
			} else {
				dist = p.Y - rc.Bottom()
			}
		}
		if dist >= -1e-6 {
			free = math.Min(free, math.Max(dist, 0))
		}
	}
	for _, n := range r.nodes {
		if n != self {
			hit(n.Shape)
		}
	}
	for _, l := range r.labels {
		hit(l)
	}
	stub := inset + math.Min(want, math.Max(free-inset, 0)/2)
	// A route confined to a group can't run its stub past the group's
	// inner margin.
	if j != nil && j.region != nil {
		in := inflate(j.region.Rect, -r.c/2)
		var wall float64
		switch d {
		case dirLeft:
			wall = p.X - in.X
		case dirRight:
			wall = in.Right() - p.X
		case dirUp:
			wall = p.Y - in.Y
		default:
			wall = in.Bottom() - p.Y
		}
		if wall > 0 {
			stub = math.Min(stub, wall)
		}
	}
	return stub
}

// endStub is the stub en, one of j's ends, is granted now.
func (r *router) endStub(j *job, en *end, src bool) float64 {
	return r.stubLen(j, en.point, en.travel, en.box, en.side, r.wantStub(j, src))
}

// spread assigns final port positions to every end, spacing ends that share
// a side and ordering them so their routes don't cross.
func (r *router) spread() {
	var ends []*end
	for _, j := range r.jobs {
		switch {
		case j.err != nil:
		case j.kind == kindRouted && j.src != nil:
			ends = append(ends, j.src, j.dst)
		case j.banded:
		case j.kind == kindSelf:
			if j.loopSide == model.SideAuto {
				r.chooseLoopSide(j)
			}
			s := j.loopSide
			lo, hi := r.sideSpan(j.e.From, s)
			for k := range 2 {
				en := &end{j: j, order: k, box: j.e.From, side: s, travel: dirOf(s), ideal: (lo + hi) / 2, loop: k == 1}
				j.selfEnd[k] = en
				ends = append(ends, en)
			}
		}
	}
	slices.SortStableFunc(ends, func(a, b *end) int {
		if c := r.boxIdx[a.box] - r.boxIdx[b.box]; c != 0 {
			return c
		}
		if c := int(a.side) - int(b.side); c != 0 {
			return c
		}
		if a.key < b.key-1e-9 {
			return -1
		}
		if a.key > b.key+1e-9 {
			return 1
		}
		if c := a.j.i - b.j.i; c != 0 {
			return c
		}
		return a.order - b.order
	})
	for i := 0; i < len(ends); {
		k := i + 1
		for k < len(ends) && ends[k].box == ends[i].box && ends[k].side == ends[i].side {
			k++
		}
		r.spreadSide(ends[i:k])
		i = k
	}
	r.unstack(ends)
}

// unstack moves apart ports on facing sides of two boxes whose stubs would
// lie on one line, heading towards each other: two different edges
// leaving the right side of one box and the left side of the box beyond
// it at the same height share their first stretch of line. Each moves half
// a lane along its side (or one moves a whole lane), towards the way its
// route turns, keeping PortSpacing from the other ports on its side.
func (r *router) unstack(ends []*end) {
	off := r.t.ParallelOffset
	free := func(en *end, pos float64) bool {
		lo, hi := r.sideSpan(en.box, en.side)
		if pos < lo-eps || pos > hi+eps || !straightSide(en.box, en.side) {
			return false
		}
		for _, o := range ends {
			if o != en && o.box == en.box && o.side == en.side && math.Abs(o.pos-pos) < r.t.PortSpacing-eps {
				return false
			}
		}
		return true
	}
	move := func(en *end, pos float64) {
		en.pos = pos
		en.point = r.portAt(en.box, en.side, pos)
	}
	for x, a := range ends {
		for _, b := range ends[x+1:] {
			if a.j == b.j || a.box == b.box || b.side != oppositeSide(a.side) || a.anchor || b.anchor || a.loop || b.loop {
				continue
			}
			if math.Abs(a.pos-b.pos) >= off-eps || sideGap(a.box.Shape, b.box.Shape, a.side) <= 0 {
				continue
			}
			// a turns towards larger coordinates along the side when its
			// key is larger; it goes on that side of b.
			dir := 1.0
			if a.key < b.key || a.key == b.key && a.j.i > b.j.i {
				dir = -1
			}
			mid := (a.pos + b.pos) / 2
			switch {
			case free(a, mid+dir*off/2) && free(b, mid-dir*off/2):
				move(a, mid+dir*off/2)
				move(b, mid-dir*off/2)
			case free(a, b.pos+dir*off):
				move(a, b.pos+dir*off)
			case free(b, a.pos-dir*off):
				move(b, a.pos-dir*off)
			}
		}
	}
}

func (r *router) spreadSide(es []*end) {
	b, s := es[0].box, es[0].side
	lo, hi := r.sideSpan(b, s)
	n := len(es)
	sp := math.Max(r.t.PortSpacing, 1)
	off := make([]float64, n)
	for i := 1; i < n; i++ {
		g := sp
		if es[i].loop {
			g = es[i].j.loopSpan
		}
		off[i] = off[i-1] + g
	}
	if total := off[n-1]; total > hi-lo && total > 0 {
		k := math.Max(hi-lo, 0) / total
		for i := range off {
			off[i] *= k
		}
	}
	z := make([]float64, n)
	w := make([]float64, n)
	for i, e := range es {
		z[i] = e.ideal - off[i]
		w[i] = 1
		if e.anchor {
			w[i] = 1e6
		}
	}
	z = isotonic(z, w)
	for i, e := range es {
		v := math.Min(math.Max(z[i], lo), math.Max(hi-off[n-1], lo))
		e.pos = v + off[i]
		if e.anchor && math.Abs(e.pos-e.ideal) < 0.05 {
			e.pos = e.ideal
		}
		e.point = r.portAt(b, s, e.pos)
	}
}

// isotonic returns the non-decreasing sequence closest to y in weighted
// least squares (pool adjacent violators).
func isotonic(y, w []float64) []float64 {
	type block struct {
		v, w float64
		n    int
	}
	var bs []block
	for i := range y {
		bs = append(bs, block{y[i], w[i], 1})
		for len(bs) > 1 && bs[len(bs)-2].v > bs[len(bs)-1].v {
			a, b := bs[len(bs)-2], bs[len(bs)-1]
			bs = append(bs[:len(bs)-2], block{(a.v*a.w + b.v*b.w) / (a.w + b.w), a.w + b.w, a.n + b.n})
		}
	}
	out := make([]float64, 0, len(y))
	for _, b := range bs {
		for range b.n {
			out = append(out, b.v)
		}
	}
	return out
}

// endFromPath builds an end from a first-pass route. pts runs away from
// the box.
func (r *router) endFromPath(j *job, order int, box *layout.Box, c cand, pts []model.Point, vias []model.Point) *end {
	e := &end{j: j, order: order, box: box, side: c.side, travel: c.travel}
	e.key = endKey(e, pts)
	lo, hi := r.sideSpan(box, c.side)
	e.ideal = (lo + hi) / 2
	direct := len(pts) == 2 || len(vias) > 0 && near(pts[1], vias[0])
	if c.aligned && direct {
		e.ideal, e.anchor = c.pos, true
	}
	return e
}

// loopOut is how far a self-loop stands off its side.
func (r *router) loopOut() float64 { return r.c + 8 }

// chooseLoopSide picks the side for a self-loop: the pinned one, or the
// first of right, bottom, top and left whose loop and label would overlap
// the least.
func (r *router) chooseLoopSide(j *job) {
	b := j.e.From
	lw, lh := 0.0, 0.0
	if j.e.Label != nil {
		lw, lh = layout.LabelSize(r.t, j.e.Label)
	}
	span := func(s model.Side) float64 {
		sp := 2 * r.t.PortSpacing
		if j.e.Label == nil {
			return sp
		}
		if horiz(s) {
			return math.Max(sp, lw+8)
		}
		return math.Max(sp, lh+8)
	}
	pin := j.e.E.Exit
	if pin == model.SideAuto {
		pin = j.e.E.Enter
	}
	if pin != model.SideAuto {
		j.loopSide, j.loopSpan = pin, span(pin)
		return
	}
	best, bestScore := model.SideRight, inf
	for rank, s := range []model.Side{model.SideRight, model.SideBottom, model.SideTop, model.SideLeft} {
		lo, hi := r.sideSpan(b, s)
		c := (lo + hi) / 2
		sp := span(s) + 8
		depth := r.loopOut() + 4
		if j.e.Label != nil {
			if horiz(s) {
				depth += lh / 2
			} else {
				depth += lw / 2
			}
		}
		var rc layout.Rect
		switch s {
		case model.SideRight:
			rc = layout.Rect{X: b.Shape.Right(), Y: c - sp/2, W: depth, H: sp}
		case model.SideLeft:
			rc = layout.Rect{X: b.Shape.X - depth, Y: c - sp/2, W: depth, H: sp}
		case model.SideBottom:
			rc = layout.Rect{X: c - sp/2, Y: b.Shape.Bottom(), W: sp, H: depth}
		default:
			rc = layout.Rect{X: c - sp/2, Y: b.Shape.Y - depth, W: sp, H: depth}
		}
		score := float64(rank)
		for _, n := range r.nodes {
			if n != b {
				score += 10 * overlap(rc, inflate(n.Shape, 4))
			}
		}
		for _, t := range r.texts {
			score += 10 * overlap(rc, t.r)
		}
		for _, sg := range r.segs {
			score += 20 * clipLen(rc, sg.a, sg.b)
		}
		score += 10 * (rc.W*rc.H - overlap(rc, r.bounds))
		if score < bestScore {
			best, bestScore = s, score
		}
	}
	j.loopSide, j.loopSpan = best, span(best)
}

// endKey orders an end along its side by how its route turns: routes that
// turn towards the side's far end sort after those going straight, which
// sort after those turning the other way, and among routes turning the
// same way, the one that turns first sits outermost so the two don't
// cross. pts runs away from the box.
func endKey(e *end, pts []model.Point) float64 {
	if len(pts) < 3 {
		return 0
	}
	tx, ty := 1.0, 0.0
	if !horiz(e.side) {
		tx, ty = 0, 1
	}
	depth := math.Abs(pts[1].X-pts[0].X) + math.Abs(pts[1].Y-pts[0].Y)
	turn := (pts[2].X-pts[1].X)*tx + (pts[2].Y-pts[1].Y)*ty
	switch {
	case turn > 1e-6:
		return 1e5 - depth
	case turn < -1e-6:
		return depth - 1e5
	}
	return 0
}

// crowding is the first-pass cost of putting another end on side s of b:
// ends already there push it off-centre and towards the corners.
func (r *router) crowding(b *layout.Box, s model.Side) float64 {
	i, ok := r.boxIdx[b]
	if !ok {
		return 0
	}
	return crowdCost * float64(r.load[i][s])
}
