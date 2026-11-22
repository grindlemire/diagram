// Package route computes edge geometry for a laid-out diagram.
//
// Edges are routed orthogonally on a sparse grid made of the lines that
// matter: obstacle borders plus clearance, channel midlines between
// obstacles, and port and waypoint coordinates. A* searches that grid
// (state: point and the axis it was reached on, or the direction and
// waypoint index for edges with waypoints) with a cost of length, a large
// penalty per bend, penalties for crossing other edges or sharing their
// line (rising with each edge already there), a cost for squeezing
// through a node's clearance, and a heavy one for leaving the diagram.
//
// Routing runs in passes:
//
//  1. Self-loops are drawn in the band layout reserved for them on their
//     node's LoopSide, and become obstacles for everything else.
//  2. Each edge is routed with every unpinned side of both ends open, so
//     its sides are the ones its cheapest route uses. Edges choose in
//     declaration order, then those whose routes collide with others
//     choose again with every other choice known.
//  3. Ends sharing a side are ordered by how their routes turn and spread
//     along it, keeping ends that line up with the other end in place.
//  4. Every edge is routed between its final ports in declaration order,
//     and its label placed, so later edges avoid earlier labels. Short jogs
//     are straightened and segments pressed against other edges moved
//     apart. Then each colliding edge is rerouted once around all the
//     others, kept only if that doesn't add crossings or bends.
//  5. Each route is replaced by one with fewer bends between the same
//     ports where that hits nothing and adds no conflicts.
//  6. Segments of different edges sharing a line are nudged into separate
//     lanes, and every label placed again.
//
// When a route can't be found with hard obstacles, the router retries with
// soft ones (only node shapes stay solid). It reports an error only when
// that fails too, which happens when a pinned side or a waypoint is boxed
// in.
package route

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/text"
	"github.com/grindlemire/diagram/themes"
)

// Cost weights, in pixels of path length.
const (
	bendCost      = 40  // per bend
	crossCost     = 80  // per crossing of an already-routed edge
	groupCross    = 12  // per crossing of a group border the edge must cross
	foreignCross  = 30  // per crossing of the border of a group holding neither end
	softZone      = 6   // per pixel inside an obstacle's clearance (degraded)
	clearCost     = 4   // per pixel inside a node's clearance
	minClear      = 4   // solid margin around nodes
	softCross     = 400 // per crossing into a forbidden group or label (degraded)
	softInside    = 3   // per pixel inside a forbidden group (degraded)
	outsideCost   = 2   // per pixel outside the diagram's bounds
	outsideCross  = 40  // per crossing of the diagram's bounds
	overlapCost   = 1   // per pixel on top of another edge, times the edges there
	nearEdgeCost  = 1   // per pixel closer than ParallelOffset to another edge
	besideCost    = 0.1 // per pixel in the lane next to another edge
	proximityCost = 0.6 // per pixel running right next to a node, tapering off
	borderCost    = 0.6 // per pixel running right next to a group border
	sideTie       = 1   // per rank of a side, to break ties between sides
	crowdCost     = 4   // per end already on a side, when choosing sides
	loopSideCost  = 160 // for an edge end on the side reserved for self-loops

	// maxReselect is the most edges for which sides are chosen twice, and
	// maxImprove the most for which every route is tried again once all
	// are known; beyond them those passes cost more than they return.
	maxReselect     = 60
	maxImprove      = 120
	maxImproveEdges = 20 // the most edges improve reroutes
)

// Route fills in every edge's points, sides and label position.
func Route(res *layout.Result) error {
	return Start(res).Finish()
}

// Session is a routing in progress: sides chosen and every edge sketched
// (routed once, in declaration order), but not yet routed between its
// final ports. The sketches show where edges will run, which is enough to
// judge a layout's spacing before paying for the rest.
type Session struct{ r *router }

// Start sketches every edge of res.
func Start(res *layout.Result) *Session {
	r := newRouter(res)
	r.start()
	return &Session{r}
}

// Sketches returns each edge's sketched route, indexed like res.Edges;
// nil for edges that have none (straight edges, unbanded self-loops, and
// edges that couldn't be routed).
func (s *Session) Sketches() [][]model.Point {
	out := make([][]model.Point, len(s.r.jobs))
	for i, j := range s.r.jobs {
		switch {
		case j.banded:
			out[i] = j.e.Points
		case j.kind == kindRouted && j.err == nil:
			out[i] = j.sketch
		}
	}
	return out
}

// Score rates the sketches, lower being better: bends, crossings, length
// run on top of other edges and length outside the diagram, weighted as
// quality.Metrics.Score weighs them.
func (s *Session) Score() float64 {
	paths := s.Sketches()
	b := s.r.bounds
	var v float64
	for i, p := range paths {
		if len(p) < 2 {
			continue
		}
		v += 40 * float64(len(p)-2)
		for k := 0; k+1 < len(p); k++ {
			v += seglen(p[k], p[k+1]) - clipLen(b, p[k], p[k+1])
			for _, q := range paths[i+1:] {
				for m := 0; m+1 < len(q); m++ {
					if crossesAny(p[k], p[k+1], q[m], q[m+1]) {
						v += 80
					}
					v += 2 * nearLen(p[k], p[k+1], q[m], q[m+1], 1)
				}
			}
		}
	}
	return v
}

// Finish routes every edge between its final ports and places the labels.
func (s *Session) Finish() error { return s.r.finish() }

type kind uint8

const (
	kindRouted kind = iota
	kindStraight
	kindSelf
)

// job is one edge being routed.
type job struct {
	i    int
	e    *layout.Edge
	kind kind
	err  error

	// Route constraints shared by both passes.
	region   *layout.Box   // innermost visible group holding both ends, or nil
	allowed  []*layout.Box // visible groups the edge may cross into
	srcInner bool          // From is a group that contains To
	dstInner bool          // To is a group that contains From

	// Ends chosen by the first pass.
	src     *end
	dst     *end
	selfEnd [2]*end

	// Self-loops.
	loopSide model.Side
	loopSpan float64
	banded   bool          // routed in the band layout reserved on the node's LoopSide
	sketch   []model.Point // first-pass route
}

type router struct {
	res *layout.Result
	t   *themes.Theme
	c   float64 // clearance

	boxIdx  map[*layout.Box]int
	nodes   []*layout.Box
	groups  []*layout.Box // visible groups
	headers []obstacle    // group header text, and nodes' badges and status dots
	texts   []obstacle    // text outside a node's shape (Person, IconOnly)
	bounds  layout.Rect

	jobs      []*job
	segs      []seg         // routed segments, for crossing and overlap costs
	labels    []layout.Rect // placed labels
	labelEdge []int         // the edge each label belongs to

	bands []layout.Rect // areas reserved for self-loops
	// labelsFinal is set once routes are final. Until then a label that
	// can't sit by its line isn't an obstacle: where it floats depends on
	// routes not yet drawn, and it is placed again at the end.
	labelsFinal bool

	segVer, segIdxVer int
	segIdx, nodeIdx   *spatial
	jobIdx            *spatial // routes, while nudging
	jobSegs           [][2]int
	midX, midY        []float64 // channel midlines, computed once
	load              [][5]int  // ends on each side of each box, by boxIdx, after pass 1

	scratch scratch
}

type obstacle struct {
	r     layout.Rect
	owner *layout.Box
}

// seg is a piece of a routed edge.
type seg struct {
	a, b model.Point
	edge int
}

func (s seg) horizontal() bool { return s.a.Y == s.b.Y }
func (s seg) vertical() bool   { return s.a.X == s.b.X }

func newRouter(res *layout.Result) *router {
	t := res.Theme
	r := &router{res: res, t: t, c: t.EdgeClearance, boxIdx: map[*layout.Box]int{}}
	r.bounds = layout.Rect{W: res.Width, H: res.Height}
	r.load = make([][5]int, len(res.Boxes))
	for i, b := range res.Boxes {
		r.boxIdx[b] = i
		if b.El.Kind == model.KindNode {
			r.nodes = append(r.nodes, b)
			if tr, ok := outsideText(b); ok {
				r.texts = append(r.texts, obstacle{tr, b})
			}
			// Badges and status dots stick out of the shape. Routes and
			// labels keep off them as they do off group headers: small
			// and solid, without a node's wide clearance.
			for _, m := range b.Marks() {
				r.headers = append(r.headers, obstacle{m, b})
			}
			continue
		}
		if b.Shown {
			r.groups = append(r.groups, b)
			if hr, ok := headerRect(b); ok {
				r.headers = append(r.headers, obstacle{hr, b})
			}
		}
	}
	return r
}

// outsideText returns the box around a node's text when it sits outside the
// node's shape, as it does for Person and IconOnly nodes.
func outsideText(b *layout.Box) (layout.Rect, bool) {
	if !figure(b) || len(b.Lines) == 0 {
		return layout.Rect{}, false
	}
	return textBounds(b.Lines), true
}

// figure reports whether b is a node drawn as a figure with its text
// underneath: a Person, or an IconOnly node with an icon.
func figure(b *layout.Box) bool {
	e := b.El
	return e.Kind == model.KindNode && (e.Shape == model.ShapePerson || e.IconPlace == model.IconOnly && e.Icon != nil)
}

// portRect is the box whose side s ports attach to: the shape, except
// below a figure with text underneath, where ports attach under the text
// so edges never run through it.
func (r *router) portRect(b *layout.Box, s model.Side) layout.Rect {
	if s != model.SideBottom || !figure(b) || len(b.Lines) == 0 {
		return b.Shape
	}
	tb := textBounds(b.Lines)
	ln := b.Lines[len(b.Lines)-1]
	f := text.FaceFor(ln.Weight)
	asc, desc := f.Ascent(ln.Size), f.Descent(ln.Size)
	lineH := ln.Size * r.t.LineHeight
	bottom := ln.Y + desc + (lineH-(asc+desc))/2
	x0, x1 := math.Min(b.Shape.X, tb.X), math.Max(b.Shape.Right(), tb.Right())
	return layout.Rect{X: x0, Y: b.Shape.Y, W: x1 - x0, H: math.Max(bottom, b.Shape.Bottom()) - b.Shape.Y}
}

// headerRect returns the box around a group's header icon and text.
func headerRect(b *layout.Box) (layout.Rect, bool) {
	if len(b.Lines) == 0 && b.Icon.W == 0 {
		return layout.Rect{}, false
	}
	switch {
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

func union(a, b layout.Rect) layout.Rect {
	x, y := math.Min(a.X, b.X), math.Min(a.Y, b.Y)
	return layout.Rect{X: x, Y: y, W: math.Max(a.Right(), b.Right()) - x, H: math.Max(a.Bottom(), b.Bottom()) - y}
}

func inflate(r layout.Rect, d float64) layout.Rect {
	return layout.Rect{X: r.X - d, Y: r.Y - d, W: r.W + 2*d, H: r.H + 2*d}
}

// within reports whether element e is g or inside g.
func within(e, g *model.Element) bool {
	for x := e; x != nil; x = x.Parent {
		if x == g {
			return true
		}
	}
	return false
}

// start creates the jobs, draws the banded self-loops and sketches every
// edge (pass 1, first round).
func (r *router) start() {
	for i, e := range r.res.Edges {
		r.jobs = append(r.jobs, r.newJob(i, e))
	}

	// Self-loops in their reserved bands come first: their geometry is
	// fixed, and every other edge routes around them.
	for _, b := range r.nodes {
		if r.hasBand(b) {
			r.bands = append(r.bands, r.band(b))
		}
	}
	r.bandLoops()

	// Pass 1: choose sides by routing with every unpinned side open. Each
	// edge chooses once in declaration order here, then again in finish,
	// if its route collides with others, with every other edge's choice
	// known.
	for _, j := range r.jobs {
		if j.kind != kindRouted || j.err != nil {
			continue
		}
		r.firstPass(j)
	}
}

// finish chooses sides again where sketches collide, then routes every
// edge between its final ports, improves, simplifies and nudges the
// routes, and places the labels.
func (r *router) finish() error {
	routed := 0
	for _, j := range r.jobs {
		if j.kind == kindRouted && j.src != nil {
			routed++
		}
	}
	for _, j := range r.jobs {
		if routed > maxReselect {
			break
		}
		if j.kind != kindRouted || j.err != nil || j.src == nil || !r.conflicted(j) {
			continue
		}
		r.reselect(j)
	}

	// Spread ends that share a side and route every edge between its
	// final ports.
	r.spread()
	err := r.final()
	if err == nil {
		// Reroute each edge once more, now that every other edge is known.
		if routed <= maxImprove {
			before := r.snapshot()
			r.improve()
			if r.score() > before.score {
				r.restore(before)
			}
		}
		r.simplifyRoutes()
		r.dejog()
		r.nudge()
		if routed <= maxImprove {
			r.uncross()
		}
		r.relabel()
	}
	return err
}

// relabel records every route's segments afresh and places all labels
// again, in declaration order.
func (r *router) relabel() {
	r.labelsFinal = true
	r.segs, r.labels, r.labelEdge = nil, nil, nil
	r.segsChanged()
	r.bandLoops()
	for _, j := range r.jobs {
		if j.banded || j.err != nil && j.kind == kindRouted {
			continue
		}
		r.addPath(j.i, j.e.Points)
	}
	for _, j := range r.jobs {
		if !j.banded {
			r.placeLabel(j)
		}
	}
}

// collisions rates how badly j's route collides with the others:
// crossings and length run close beside them.
func (r *router) collisions(j *job) float64 {
	v := 0.0
	w := r.t.ParallelOffset - 0.5
	pts := j.e.Points
	for k := 0; k+1 < len(pts); k++ {
		a, b := pts[k], pts[k+1]
		r.segsNear(inflate(segRect(a, b), w), func(s seg) {
			if s.edge == j.i {
				return
			}
			if crossesAny(a, b, s.a, s.b) {
				v += crossCost
			}
			v += nearLen(a, b, s.a, s.b, w)
		})
	}
	return v
}

// conflicted reports whether j's current route crosses or runs close
// beside another edge, or bends more than twice: the cases where routing
// it again may find something better.
func (r *router) conflicted(j *job) bool {
	var mine []seg
	for _, s := range r.segs {
		if s.edge == j.i {
			mine = append(mine, s)
		}
	}
	if len(mine) > 3 {
		return true
	}
	w := r.t.ParallelOffset - 0.5
	for _, s := range r.segs {
		if s.edge == j.i {
			continue
		}
		for _, m := range mine {
			if crossesAny(m.a, m.b, s.a, s.b) || nearLen(m.a, m.b, s.a, s.b, w) > 0 {
				return true
			}
		}
	}
	return false
}

// improve rips up each routed edge in turn and routes it again around all
// the others, including those declared after it.
func (r *router) improve() {
	// In a big diagram only the edges that collide most get another go.
	type scored struct {
		j *job
		v float64
	}
	var todo []scored
	for _, j := range r.jobs {
		if j.kind != kindRouted || j.err != nil || j.src == nil || !r.conflicted(j) {
			continue
		}
		todo = append(todo, scored{j, r.collisions(j)})
	}
	if len(todo) > maxImproveEdges {
		slices.SortStableFunc(todo, func(a, b scored) int {
			switch {
			case a.v > b.v:
				return -1
			case a.v < b.v:
				return 1
			}
			return 0
		})
		todo = todo[:maxImproveEdges]
		slices.SortStableFunc(todo, func(a, b scored) int { return a.j.i - b.j.i })
	}
	for _, t := range todo {
		j := t.j
		old := j.e.Points
		r.removeEdge(j.i)
		r.secondPass(j)
		if j.err != nil {
			j.err = nil
			j.e.Points = old
			r.addPath(j.i, old)
		}
		r.placeLabel(j)
	}
}

// state is a saved routing result.
type state struct {
	score    float64
	edges    []layout.Edge
	labels   []layout.Line
	ends     []end
	segs     []seg
	rects    []layout.Rect
	rectEdge []int
}

func (r *router) snapshot() state {
	st := state{score: r.score(), segs: r.segs, rects: r.labels, rectEdge: r.labelEdge}
	for _, j := range r.jobs {
		// Copy the points: nudging moves them in place.
		e := *j.e
		e.Points = slices.Clone(e.Points)
		st.edges = append(st.edges, e)
		var ln layout.Line
		if j.e.Label != nil {
			ln = *j.e.Label
		}
		st.labels = append(st.labels, ln)
		for _, en := range []*end{j.src, j.dst, j.selfEnd[0], j.selfEnd[1]} {
			if en != nil {
				st.ends = append(st.ends, *en)
			}
		}
	}
	return st
}

func (r *router) restore(st state) {
	r.segs, r.labels, r.labelEdge = st.segs, st.rects, st.rectEdge
	r.segsChanged()
	k := 0
	for i, j := range r.jobs {
		lbl := j.e.Label
		*j.e = st.edges[i]
		// A state may be restored more than once.
		j.e.Points = slices.Clone(j.e.Points)
		j.e.Label = lbl
		if lbl != nil {
			*lbl = st.labels[i]
		}
		for _, en := range []*end{j.src, j.dst, j.selfEnd[0], j.selfEnd[1]} {
			if en != nil {
				*en = st.ends[k]
				k++
			}
		}
	}
}

// score rates a routing: crossings, then bends.
func (r *router) score() float64 {
	var s float64
	for a, ja := range r.jobs {
		pa := ja.e.Points
		s += bendCost * float64(max(len(pa)-2, 0))
		for _, jb := range r.jobs[a+1:] {
			pb := jb.e.Points
			for i := 0; i+1 < len(pa); i++ {
				for k := 0; k+1 < len(pb); k++ {
					if properCross(pa[i], pa[i+1], pb[k], pb[k+1]) {
						s += crossCost
					}
				}
			}
		}
	}
	return s
}

// final routes every edge between its spread ports and places labels, in
// declaration order.
func (r *router) final() error {
	r.segs, r.labels, r.labelEdge = nil, nil, nil
	r.segsChanged()
	r.bandLoops()
	var errs []error
	for _, j := range r.jobs {
		if j.banded {
			continue
		}
		if j.kind == kindRouted && j.src != nil {
			j.err = nil
		}
		switch {
		case j.err != nil:
			straight(r, j)
		case j.kind == kindStraight:
			straight(r, j)
		case j.kind == kindSelf:
			r.selfLoop(j)
		default:
			r.secondPass(j)
		}
		if j.err != nil {
			errs = append(errs, j.err)
			continue
		}
		r.placeLabel(j)
	}
	return errors.Join(errs...)
}

func (r *router) newJob(i int, e *layout.Edge) *job {
	j := &job{i: i, e: e}
	switch {
	case e.E.Straight:
		j.kind = kindStraight
		return j
	case e.From == e.To:
		j.kind = kindSelf
		b := e.From
		pin := e.E.Exit
		if pin == model.SideAuto {
			pin = e.E.Enter
		}
		j.banded = b.LoopSide != model.SideAuto && (pin == model.SideAuto || pin == b.LoopSide)
		return j
	}
	from, to := e.From.El, e.To.El
	j.srcInner = from.Kind.IsGroup() && within(to, from)
	j.dstInner = to.Kind.IsGroup() && within(from, to)

	// The region is the innermost visible group that holds both ends.
	for x := from; x != nil; x = x.Parent {
		b := r.res.ByEl[x]
		if b == nil || !b.Shown || !within(to, x) {
			continue
		}
		j.region = b
		break
	}
	for _, g := range r.groups {
		if within(from, g.El) || within(to, g.El) || e.E.CrossGroups {
			j.allowed = append(j.allowed, g)
			continue
		}
		for _, v := range e.E.Via {
			if contains(g.Rect, v) {
				j.allowed = append(j.allowed, g)
				break
			}
		}
	}
	for _, v := range e.E.Via {
		for _, n := range r.nodes {
			if strictlyInside(n.Shape, v) {
				j.err = r.edgeErr(j, "waypoint (%s, %s) is inside %s", fmtNum(v.X), fmtNum(v.Y), n.El.Describe())
				return j
			}
		}
	}
	return j
}

func (r *router) edgeErr(j *job, format string, args ...any) error {
	return fmt.Errorf("%s: %s: %s", j.e.E.Src, j.e.E.Describe(), fmt.Sprintf(format, args...))
}

func fmtNum(v float64) string {
	return fmt.Sprint(math.Round(v*100) / 100)
}

func contains(r layout.Rect, p model.Point) bool {
	return p.X >= r.X && p.X <= r.Right() && p.Y >= r.Y && p.Y <= r.Bottom()
}

func strictlyInside(r layout.Rect, p model.Point) bool {
	const eps = 1e-6
	return p.X > r.X+eps && p.X < r.Right()-eps && p.Y > r.Y+eps && p.Y < r.Bottom()-eps
}

// addPath records a routed path's segments for later edges' costs.
func (r *router) addPath(i int, pts []model.Point) {
	for k := 0; k+1 < len(pts); k++ {
		s := seg{pts[k], pts[k+1], i}
		if s.a.X > s.b.X || s.a.Y > s.b.Y {
			s.a, s.b = s.b, s.a
		}
		r.segs = append(r.segs, s)
		r.segAdded(len(r.segs) - 1)
	}
}

// simplify drops repeated and collinear interior points.
func simplify(pts []model.Point) []model.Point {
	var out []model.Point
	for _, p := range pts {
		if n := len(out); n > 0 && near(out[n-1], p) {
			continue
		}
		if n := len(out); n >= 2 && collinear(out[n-2], out[n-1], p) {
			out[n-1] = p
			continue
		}
		out = append(out, p)
	}
	return out
}

func near(a, b model.Point) bool { return math.Abs(a.X-b.X) < 1e-6 && math.Abs(a.Y-b.Y) < 1e-6 }

func collinear(a, b, c model.Point) bool {
	const eps = 1e-6
	return math.Abs(a.X-b.X) < eps && math.Abs(b.X-c.X) < eps || math.Abs(a.Y-b.Y) < eps && math.Abs(b.Y-c.Y) < eps
}
