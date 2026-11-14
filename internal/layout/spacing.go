package layout

import (
	"math"

	"github.com/grindlemire/diagram/internal/model"
)

// spacingFloor returns, for each Row, Column and Grid, the least gap its
// children need for the edges that run between them, unless the diagram
// asks for ExactSpacing:
//
//   - Every gap of a container holding an end of some edge gets room for
//     one edge and its clearance on both sides: 2×EdgeClearance +
//     ParallelOffset.
//   - A gap that a labelled edge crosses between two neighbouring
//     children gets room for the label on the edge's line, between the
//     bends and the arrowhead.
//
// A container's gap is raised to its floor; a larger gap is kept.
func (l *layouter) spacingFloor() map[*model.Element]Spacing {
	d := l.res.Diagram
	if d.ExactSpacing {
		return nil
	}
	t := l.t
	base := 2*t.EdgeClearance + t.ParallelOffset
	out := map[*model.Element]Spacing{}
	raise := func(e *model.Element, x, y float64) {
		s := out[e]
		s.X, s.Y = math.Max(s.X, x), math.Max(s.Y, y)
		out[e] = s
	}
	for _, ed := range l.res.Edges {
		for _, end := range []*Box{ed.From, ed.To} {
			if end == nil {
				continue
			}
			for x := end.El.Parent; x != nil; x = x.Parent {
				if x.Kind == model.KindRow || x.Kind == model.KindColumn || x.Kind == model.KindGrid {
					raise(x, base, base)
				}
			}
		}
	}
	margin := t.EdgeCornerRad + 2
	for _, ed := range l.res.Edges {
		if ed.Label == nil || ed.From == nil || ed.To == nil || ed.From == ed.To {
			continue
		}
		lca, ca, cb := siblingsHolding(ed.From.El, ed.To.El)
		if lca == nil {
			continue
		}
		w, h := LabelSize(t, ed.Label)
		alongX := w + 2*margin + t.ArrowSize
		alongY := h + 2*margin + t.ArrowSize
		switch lca.Kind {
		case model.KindRow:
			if l.adjacent(lca, ca, cb) {
				raise(lca, alongX, 0)
			}
		case model.KindColumn:
			if l.adjacent(lca, ca, cb) {
				raise(lca, 0, alongY)
			}
		case model.KindGrid:
			switch {
			case ca.Row == cb.Row && (ca.Col+spanOf(ca.ColSpan) == cb.Col || cb.Col+spanOf(cb.ColSpan) == ca.Col):
				raise(lca, alongX, 0)
			case ca.Col == cb.Col && (ca.Row+spanOf(ca.RowSpan) == cb.Row || cb.Row+spanOf(cb.RowSpan) == ca.Row):
				raise(lca, 0, alongY)
			}
		}
	}
	return out
}

// siblingsHolding returns the innermost container holding both a and b,
// and its children that hold each, or nil when one holds the other.
func siblingsHolding(a, b *model.Element) (lca, ca, cb *model.Element) {
	path := func(e *model.Element) []*model.Element {
		var out []*model.Element
		for x := e; x != nil; x = x.Parent {
			out = append(out, x)
		}
		return out
	}
	pa, pb := path(a), path(b)
	i, j := len(pa)-1, len(pb)-1
	for i > 0 && j > 0 && pa[i-1] == pb[j-1] {
		i--
		j--
	}
	if pa[i] != pb[j] || i == 0 || j == 0 {
		return nil, nil, nil
	}
	return pa[i], pa[i-1], pb[j-1]
}

// adjacent reports whether a and b are next to each other among flow
// container f's children as laid out.
func (l *layouter) adjacent(f, a, b *model.Element) bool {
	kids := l.res.ByEl[f].kids
	for i := 0; i+1 < len(kids); i++ {
		x, y := kids[i].El, kids[i+1].El
		if x == a && y == b || x == b && y == a {
			return true
		}
	}
	return false
}
