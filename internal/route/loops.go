package route

import (
	"math"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

// Self-loops on a node's LoopSide live in the band layout reserved
// between the node's shape and its extent on that side. The loops nest in
// declaration order, the first innermost, each standing loopStep further
// out than the one inside it; their labels stack along the side just
// outside the outermost loop.

// loopStep is how far each nested self-loop stands out from the one inside
// it. Layout sizes the band with the same step.
func (r *router) loopStep() float64 { return r.c + 8 }

// hasBand reports whether b has self-loops routed in a reserved band.
func (r *router) hasBand(b *layout.Box) bool {
	for _, j := range r.jobs {
		if j.banded && j.e.From == b {
			return true
		}
	}
	return false
}

// band returns the area reserved for b's self-loops: between the side its
// ports attach to and the edge of its extent, across the extent.
func (r *router) band(b *layout.Box) layout.Rect {
	s := b.LoopSide
	in := sideCoord(r.portRect(b, s), s)
	rc := b.Rect
	switch s {
	case model.SideRight:
		return layout.Rect{X: in, Y: rc.Y, W: math.Max(rc.Right()-in, 0), H: rc.H}
	case model.SideLeft:
		return layout.Rect{X: rc.X, Y: rc.Y, W: math.Max(in-rc.X, 0), H: rc.H}
	case model.SideBottom:
		return layout.Rect{X: rc.X, Y: in, W: rc.W, H: math.Max(rc.Bottom()-in, 0)}
	}
	return layout.Rect{X: rc.X, Y: rc.Y, W: rc.W, H: math.Max(in-rc.Y, 0)}
}

// bandLoops routes every banded self-loop and places its label, recording
// both as obstacles for the edges routed after.
func (r *router) bandLoops() {
	done := map[*layout.Box]bool{}
	for _, j := range r.jobs {
		if !j.banded || done[j.e.From] {
			continue
		}
		b := j.e.From
		done[b] = true
		var loops []*job
		for _, o := range r.jobs {
			if o.banded && o.e.From == b {
				loops = append(loops, o)
			}
		}
		r.bandNode(b, loops)
	}
}

func (r *router) bandNode(b *layout.Box, loops []*job) {
	s := b.LoopSide
	n := dirOf(s)
	lo, hi := r.sideSpan(b, s)
	mid := (lo + hi) / 2
	// Half-widths: the innermost loop spans two port spacings, and each
	// loop around it is ParallelOffset wider on both ends.
	half := make([]float64, len(loops))
	for i := range loops {
		half[i] = r.t.PortSpacing + float64(i)*r.t.ParallelOffset
	}
	if room := (hi - lo) / 2; half[len(half)-1] > room {
		for i := range half {
			half[i] = room * float64(i+1) / float64(len(half))
		}
	}
	base := sideCoord(r.portRect(b, s), s)
	far := func(i int) float64 {
		d := float64(i+1) * r.loopStep()
		if s == model.SideLeft || s == model.SideTop {
			return base - d
		}
		return base + d
	}
	onFar := func(p model.Point, v float64) model.Point {
		if horiz(s) {
			return model.Point{X: p.X, Y: v}
		}
		return model.Point{X: v, Y: p.Y}
	}
	for i, j := range loops {
		a := &end{j: j, order: 0, box: b, side: s, travel: n, pos: mid - half[i]}
		z := &end{j: j, order: 1, box: b, side: s, travel: n, pos: mid + half[i], loop: true}
		a.ideal, z.ideal = a.pos, z.pos
		a.point, z.point = r.portAt(b, s, a.pos), r.portAt(b, s, z.pos)
		j.selfEnd = [2]*end{a, z}
		j.loopSide = s
		f := far(i)
		e := j.e
		e.Points = []model.Point{a.point, onFar(a.point, f), onFar(z.point, f), z.point}
		e.ExitSide, e.EnterSide = s, s
		e.Rounded = true
		r.addPath(j.i, e.Points)
	}

	// Labels stack along the side, just outside the outermost loop.
	type lbl struct {
		j             *job
		across, along float64
	}
	var ls []lbl
	total := 0.0
	for _, j := range loops {
		if j.e.Label == nil {
			continue
		}
		w, h := layout.LabelSize(r.t, j.e.Label)
		l := lbl{j, w, h}
		if horiz(s) {
			l.across, l.along = h, w
		}
		if len(ls) > 0 {
			total += labelGap
		}
		total += l.along
		ls = append(ls, l)
	}
	out := far(len(loops) - 1)
	pos := mid - total/2
	for _, l := range ls {
		c := pos + l.along/2
		pos += l.along + labelGap
		d := labelGap + l.across/2
		if s == model.SideLeft || s == model.SideTop {
			d = -d
		}
		at := model.Point{X: out + d, Y: c}
		if horiz(s) {
			at = model.Point{X: c, Y: out + d}
		}
		layout.PlaceLabel(r.t, l.j.e, at)
		r.labels = append(r.labels, l.j.e.LabelRect)
		r.labelEdge = append(r.labelEdge, l.j.i)
	}
}

// labelGap separates a banded loop's label from the loop and from the
// next label.
const labelGap = 4
