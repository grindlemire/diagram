package route

import (
	"math"
	"slices"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

// Label placement.
//
// A label goes on its edge's line where it fits, else beside the line,
// touching it and alongside the segment it annotates, else further out
// into free space. Every candidate is scored, in strict order of
// importance: overlapping a node or any text, then overlapping another
// label, then crossing edges, ports' stubs or group
// borders or reaching outside the diagram, and last its distance from the
// line. One exception keeps labels on short edges by their line: a label
// on or touching its line may spill over the borders of its own two end
// nodes (never their text or icon), at a small cost per px². Candidates come in rings
// of increasing distance, and the search stops after the first ring that
// holds a spot with no conflicts.

// Weights for the tiers of a label's score.
const (
	labelHard  = 1e6 // per px² over a node or text
	labelSoft  = 1e4 // per px² over another label
	labelCross = 100 // per px of edge, stub or group border under it, or outside the diagram
	// labelAway is per px of distance from the line: a label leaves its
	// line only to clear more than half its distance in crossings, as one
	// that floats free reads as belonging to nothing.
	labelAway = labelCross / 2
	// labelSpill is per px² of a label over its own end nodes' borders:
	// cheap enough that spilling a little beats leaving the line.
	labelSpill = 0.5
)

// lspot is a candidate label position: the pill and the text's centre.
type lspot struct {
	rect   layout.Rect
	at     model.Point
	dist   float64 // from the line, the last tie-breaker
	online bool    // sits on the edge's own line
}

// lineGap is the space between a label beside its edge and the line.
const lineGap = 1.5

// placeLabel places j's label at its best spot and records it as an
// obstacle for later edges and labels.
func (r *router) placeLabel(j *job) {
	e := j.e
	if e.Label == nil || len(e.Points) < 2 {
		return
	}
	w, h := layout.LabelSize(r.t, e.Label)
	rings := r.labelRings(j, w, h)
	var area layout.Rect
	first := true
	for _, ring := range rings {
		for _, sp := range ring {
			if first {
				area, first = sp.rect, false
			} else {
				area = union(area, sp.rect)
			}
		}
	}
	stubs := r.stubsNear(j, area)
	var best lspot
	bestScore := inf
	for _, ring := range rings {
		for _, sp := range ring {
			c := r.labelConflict(j, sp, stubs)
			if sc := c + sp.dist*labelAway; sc < bestScore-1e-9 {
				best, bestScore = sp, sc
			}
		}
		if bestScore < labelCross*0.5+maxDist(ring)*labelAway { // a spot with no conflicts
			break
		}
	}
	layout.PlaceLabel(r.t, e, best.at)
	e.LabelRect = best.rect
	if best.dist > lineGap+1 && !r.labelsFinal {
		return
	}
	r.labels = append(r.labels, e.LabelRect)
	r.labelEdge = append(r.labelEdge, j.i)
}

// labelRings lists j's candidate label spots in rings of increasing
// distance from its line, most preferred first within each ring.
func (r *router) labelRings(j *job, w, h float64) [][]lspot {
	e := j.e
	tw := e.Label.Width // the text alone, without the pill's padding
	centred := func(p model.Point, dist float64, online bool) lspot {
		return lspot{rect: layout.Rect{X: p.X - w/2, Y: p.Y - h/2, W: w, H: h}, at: p, dist: dist, online: online}
	}
	ps := pieces(e.Points)
	hz := func(p piece) bool { return p.a.Y == p.b.Y && p.a.X != p.b.X }
	vt := func(p piece) bool { return p.a.X == p.b.X && p.a.Y != p.b.Y }

	if j.kind == kindSelf {
		mid := lerp(ps[1].a, ps[1].b, 0.5)
		d := dirOf(j.selfEnd[0].side)
		var ring []lspot
		for s := 0.0; s <= w/2+8; s += 4 {
			ring = append(ring, centred(step(mid, d, s), s, s == 0))
		}
		return [][]lspot{ring}
	}

	// The pieces in order of preference: longest first, or nearest the
	// end a LabelAt names.
	order := make([]int, len(ps))
	for i := range order {
		order[i] = i
	}
	switch e.E.LabelAt {
	case model.LabelEnd:
		slices.Reverse(order)
	case model.LabelMiddle:
		slices.SortStableFunc(order, func(a, b int) int {
			switch {
			case ps[a].l > ps[b].l+1e-9:
				return -1
			case ps[b].l > ps[a].l+1e-9:
				return 1
			}
			return 0
		})
	}
	margin := r.t.EdgeCornerRad + 2
	extent := func(p piece) float64 {
		if hz(p) {
			return w
		}
		return h
	}
	// Positions along a piece for a label's centre, measured from its
	// start: centred first, then a sweep. Too short a piece gets its
	// middle only.
	along := func(p piece, ext float64) []float64 {
		free := p.l - ext - 2*margin
		if free < 0 {
			return []float64{p.l / 2}
		}
		var out []float64
		if e.E.LabelAt == model.LabelMiddle {
			out = append(out, margin+ext/2+free/2)
		}
		start := margin + ext/2
		if e.E.LabelAt == model.LabelStart && p == ps[0] || e.E.LabelAt == model.LabelEnd && p == ps[len(ps)-1] {
			// Near the named end first.
			for d := 0.0; d <= free; d += 6 {
				if e.E.LabelAt == model.LabelEnd {
					out = append(out, p.l-start-d)
				} else {
					out = append(out, start+d)
				}
			}
		}
		for _, f := range []float64{0.5, 0.3, 0.7, 0.1, 0.9} {
			out = append(out, start+f*free)
		}
		for d := 0.0; d <= free; d += 8 {
			out = append(out, start+d)
		}
		return out
	}
	at := func(p piece, d float64) model.Point {
		if p.l == 0 {
			return p.a
		}
		return lerp(p.a, p.b, d/p.l)
	}

	// Ring 0: on the line, where the label fits along the piece.
	var on []lspot
	for _, i := range order {
		p := ps[i]
		if !hz(p) && !vt(p) {
			continue
		}
		short := p.l < extent(p)+2*margin
		for _, d := range along(p, extent(p)) {
			sp := centred(at(p, d), 0, true)
			if short {
				sp.dist = 2 // spills past the piece's ends
			}
			on = append(on, sp)
		}
	}
	// Beside the line, at distance off from it: alongside the piece,
	// centred on it or slid along it as far as still overlapping it.
	beside := func(off float64) []lspot {
		var out []lspot
		for _, i := range order {
			p := ps[i]
			if !hz(p) && !vt(p) {
				continue
			}
			ext := w
			if vt(p) {
				ext = h
			}
			ds := along(p, ext)
			if p.l < ext {
				// A label longer than the piece slides along it.
				for _, f := range []float64{-0.25, 0.25, -0.45, 0.45} {
					ds = append(ds, p.l/2+f*ext)
				}
			}
			for _, d := range ds {
				c := at(p, d)
				for _, side := range []float64{1, -1} {
					var sp lspot
					if hz(p) {
						// Below or above: the pill's vertical padding is
						// small, so the text sits close to the line.
						y := c.Y + side*(h/2+lineGap+off)
						sp = centred(model.Point{X: c.X, Y: y}, lineGap+off, false)
					} else {
						// Right or left: the text keeps lineGap from the
						// line, and the pill is trimmed on the line's side
						// so it doesn't cover it.
						x := c.X + side*(tw/2+2+lineGap+off)
						rect := layout.Rect{X: x - w/2, Y: c.Y - h/2, W: w, H: h}
						if inner := c.X + side*(lineGap+off); side > 0 && rect.X < inner {
							rect.W -= inner - rect.X
							rect.X = inner
						} else if side < 0 && rect.Right() > inner {
							rect.W -= rect.Right() - inner
						}
						sp = lspot{rect: rect, at: model.Point{X: x, Y: c.Y}, dist: lineGap + off}
					}
					sp.dist += math.Abs(d-p.l/2) * 0.01
					out = append(out, sp)
				}
			}
		}
		return out
	}
	rings := [][]lspot{on, beside(0)}
	for _, off := range []float64{4, 8, 16, 24, 36, 52, 72} {
		rings = append(rings, beside(off))
	}
	return rings
}

// labelConflict scores what a label at sp collides with (see the tiers
// above); zero means nothing.
func (r *router) labelConflict(j *job, sp lspot, stubs []layout.Rect) float64 {
	rc := sp.rect
	var hard, soft, cross, spill float64
	touching := sp.dist <= lineGap+0.5
	r.nodesNear(inflate(rc, 2), func(n *layout.Box) {
		if touching && (n == j.e.From || n == j.e.To) {
			// A label on or against its line may spill over the border
			// and padding of its own ends, though never their text or
			// icon: a label by its line beats one floating free.
			spill += overlap(rc, n.Shape)
			if len(n.Lines) > 0 {
				hard += overlap(rc, inflate(textBounds(n.Lines), 2))
			}
			if n.Icon.W > 0 {
				hard += overlap(rc, inflate(n.Icon, 2))
			}
			return
		}
		hard += overlap(rc, inflate(n.Shape, 2))
	})
	for _, t := range r.texts {
		hard += overlap(rc, inflate(t.r, 2))
	}
	for _, h := range r.headers {
		hard += overlap(rc, inflate(h.r, 2))
	}
	for _, bd := range r.bands {
		hard += overlap(rc, bd)
	}
	for _, l := range r.labels {
		soft += overlap(rc, inflate(l, 2))
	}
	// Reaching outside the diagram grows the canvas: worth avoiding, but
	// less than covering anything.
	b := r.bounds
	cross += math.Max(0, math.Max(math.Max(b.X-rc.X, rc.Right()-b.Right()), math.Max(b.Y-rc.Y, rc.Bottom()-b.Bottom())))
	for _, st := range stubs {
		cross += math.Sqrt(overlap(rc, st))
	}
	in := inflate(rc, 1)
	r.segsNear(in, func(sg seg) {
		if sg.edge != j.i || !sp.online {
			cross += clipLen(in, sg.a, sg.b)
		}
	})
	// A label away from its line mustn't sit nearer another edge, which
	// it would seem to belong to.
	if sp.dist > lineGap+1 {
		near := inflate(rc, sp.dist)
		r.segsNear(near, func(sg seg) {
			if sg.edge == j.i || clipLen(near, sg.a, sg.b) == 0 {
				return
			}
			if d := rectDist(rc, sg.a, sg.b); d < sp.dist {
				cross += 10 + sp.dist - d
			}
		})
	}
	// A group border under a label matters less than an edge.
	for _, g := range r.groups {
		for _, ln := range borders(g.Rect) {
			cross += 0.2 * clipLen(rc, ln[0], ln[1])
		}
	}
	return labelHard*hard + labelSoft*soft + labelCross*cross + labelSpill*spill
}

func maxDist(ring []lspot) float64 {
	d := 0.0
	for _, sp := range ring {
		d = math.Max(d, sp.dist)
	}
	return d
}

// rectDist is the distance from rectangle rc to segment ab (0 when they
// touch); ab is axis-aligned.
func rectDist(rc layout.Rect, a, b model.Point) float64 {
	x0, x1 := math.Min(a.X, b.X), math.Max(a.X, b.X)
	y0, y1 := math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)
	dx := math.Max(0, math.Max(rc.X-x1, x0-rc.Right()))
	dy := math.Max(0, math.Max(rc.Y-y1, y0-rc.Bottom()))
	return math.Hypot(dx, dy)
}
