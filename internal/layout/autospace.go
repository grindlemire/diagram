package layout

import (
	"math"
	"slices"

	"github.com/grindlemire/diagram/internal/model"
)

// Crowded looks at the routed edges and returns, for every container whose
// gaps between children carry more parallel edges than fit side by side,
// the spacing to add on top of extra, the spacing this layout used. It
// reports whether anything grew. A gap with n edges running along it
// needs EdgeClearance on both sides, 1.5×ParallelOffset per edge, and the
// width across it of the widest label placed in it.
type span struct{ lo, hi float64 }

// paths are the edges' routes, indexed like r.Edges; when nil, the routed
// edges' points and labels are used.
func (r *Result) Crowded(extra map[*model.Element]Spacing, paths [][]model.Point) (map[*model.Element]Spacing, bool) {
	if paths == nil {
		for _, e := range r.Edges {
			paths = append(paths, e.Points)
		}
	}
	t := r.Theme
	need := func(n int, label float64) float64 {
		if n == 0 && label == 0 {
			return 0
		}
		return 2*t.EdgeClearance + float64(n)*1.5*t.ParallelOffset + label
	}
	// labelAcross is the widest (vertical gaps) or tallest (horizontal
	// gaps) label centred between lo and hi across the gap, within along.
	labelAcross := func(vertical bool, lo, hi float64, along span) float64 {
		best := 0.0
		for _, e := range r.Edges {
			l := e.LabelRect
			if l.W == 0 {
				continue
			}
			c, a0, a1, ext := l.Center().X, l.Y, l.Bottom(), l.W
			if !vertical {
				c, a0, a1, ext = l.Center().Y, l.X, l.Right(), l.H
			}
			if c > lo && c < hi && a1 > along.lo && a0 < along.hi {
				best = math.Max(best, ext)
			}
		}
		return best
	}
	// Axis-aligned segments of every edge: vertical ones by x, horizontal
	// ones by y.
	type aseg struct {
		at     float64
		lo, hi float64
	}
	var vs, hs []aseg
	for _, pts := range paths {
		for k := 0; k+1 < len(pts); k++ {
			a, b := pts[k], pts[k+1]
			switch {
			case a.X == b.X && a.Y != b.Y:
				vs = append(vs, aseg{a.X, math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)})
			case a.Y == b.Y && a.X != b.X:
				hs = append(hs, aseg{a.Y, math.Min(a.X, b.X), math.Max(a.X, b.X)})
			}
		}
	}
	// depth is the most segments that overlap along a channel: those of
	// segs lying strictly between lo and hi across it, within along.
	depth := func(segs []aseg, lo, hi float64, along span) int {
		type ev struct {
			at float64
			d  int
		}
		var evs []ev
		for _, s := range segs {
			if s.at <= lo+0.5 || s.at >= hi-0.5 {
				continue
			}
			a, b := math.Max(s.lo, along.lo), math.Min(s.hi, along.hi)
			if b-a < 1 {
				continue
			}
			evs = append(evs, ev{a, 1}, ev{b, -1})
		}
		slices.SortFunc(evs, func(x, y ev) int {
			switch {
			case x.at < y.at:
				return -1
			case x.at > y.at:
				return 1
			}
			return x.d - y.d // ends before starts at the same point
		})
		n, best := 0, 0
		for _, e := range evs {
			n += e.d
			best = max(best, n)
		}
		return best
	}

	out := map[*model.Element]Spacing{}
	for k, v := range extra {
		out[k] = v
	}
	changed := false
	grow := func(el *model.Element, x bool, by float64) {
		if by <= 0.5 {
			return
		}
		by = math.Ceil(by)
		s := out[el]
		if x {
			s.X += by
		} else {
			s.Y += by
		}
		out[el] = s
		changed = true
	}
	all := append([]*Box{r.Root}, r.Boxes...)
	for _, b := range all {
		e := b.El
		kids := b.kids
		if e.Kind == model.KindNode || e.Kind == model.KindCanvas || len(kids) < 2 {
			continue
		}
		c := b.Content
		// Each axis grows by the largest shortfall of any of its gaps.
		var wantX, wantY float64
		switch e.Kind {
		case model.KindRow:
			for i := 0; i+1 < len(kids); i++ {
				lo, hi := kids[i].Rect.Right(), kids[i+1].Rect.X
				wantX = math.Max(wantX, need(depth(vs, lo, hi, span{c.Y, c.Bottom()}), labelAcross(true, lo, hi, span{c.Y, c.Bottom()}))-(hi-lo))
			}
		case model.KindColumn:
			for i := 0; i+1 < len(kids); i++ {
				lo, hi := kids[i].Rect.Bottom(), kids[i+1].Rect.Y
				wantY = math.Max(wantY, need(depth(hs, lo, hi, span{c.X, c.Right()}), labelAcross(false, lo, hi, span{c.X, c.Right()}))-(hi-lo))
			}
		case model.KindGrid:
			for col := 0; col+1 < e.Cols; col++ {
				lo, hi := math.Inf(-1), math.Inf(1)
				for _, k := range kids {
					if !inGrid(e, k.El) {
						continue
					}
					if k.El.Col+spanOf(k.El.ColSpan)-1 == col {
						lo = math.Max(lo, k.Rect.Right())
					}
					if k.El.Col == col+1 {
						hi = math.Min(hi, k.Rect.X)
					}
				}
				if !math.IsInf(lo, 0) && !math.IsInf(hi, 0) && hi > lo {
					wantX = math.Max(wantX, need(depth(vs, lo, hi, span{c.Y, c.Bottom()}), labelAcross(true, lo, hi, span{c.Y, c.Bottom()}))-(hi-lo))
				}
			}
			rows := 0
			for _, k := range kids {
				if inGrid(e, k.El) {
					rows = max(rows, k.El.Row+spanOf(k.El.RowSpan))
				}
			}
			for row := 0; row+1 < rows; row++ {
				lo, hi := math.Inf(-1), math.Inf(1)
				for _, k := range kids {
					if !inGrid(e, k.El) {
						continue
					}
					if k.El.Row+spanOf(k.El.RowSpan)-1 == row {
						lo = math.Max(lo, k.Rect.Bottom())
					}
					if k.El.Row == row+1 {
						hi = math.Min(hi, k.Rect.Y)
					}
				}
				if !math.IsInf(lo, 0) && !math.IsInf(hi, 0) && hi > lo {
					wantY = math.Max(wantY, need(depth(hs, lo, hi, span{c.X, c.Right()}), labelAcross(false, lo, hi, span{c.X, c.Right()}))-(hi-lo))
				}
			}
		}
		grow(e, true, wantX)
		grow(e, false, wantY)
	}
	return out, changed
}
