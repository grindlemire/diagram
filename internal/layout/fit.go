package layout

import (
	"math"

	"github.com/grindlemire/diagram/internal/model"
)

// Fit grows the canvas to contain every box, edge, arrowhead and label,
// shifting the whole drawing when something extends above or left of the
// origin. Routing can send edges around the outside of the layout; without
// this they would be clipped at the image border.
func (r *Result) Fit() {
	minX, minY := 0.0, 0.0
	maxX, maxY := r.Width, r.Height
	grow := func(x0, y0, x1, y1 float64) {
		minX, minY = math.Min(minX, x0), math.Min(minY, y0)
		maxX, maxY = math.Max(maxX, x1), math.Max(maxY, y1)
	}
	for _, b := range r.Boxes {
		grow(b.Rect.X, b.Rect.Y, b.Rect.Right(), b.Rect.Bottom())
		for _, m := range b.Marks() {
			grow(m.X, m.Y, m.Right(), m.Bottom())
		}
	}
	for _, e := range r.Edges {
		// Arrowheads and stroke extend past the polyline.
		pad := e.Width/2 + r.Theme.ArrowSize*math.Max(1, e.Width/1.5)
		for _, p := range e.Points {
			grow(p.X-pad, p.Y-pad, p.X+pad, p.Y+pad)
		}
		if e.LabelRect.W > 0 {
			l := e.LabelRect
			grow(l.X, l.Y, l.Right(), l.Bottom())
		}
	}
	r.Translate(-minX, -minY)
	r.Width, r.Height = maxX-minX, maxY-minY
}

// Translate moves the whole drawing by (dx, dy).
func (r *Result) Translate(dx, dy float64) {
	if dx == 0 && dy == 0 {
		return
	}
	mv := func(rc *Rect) { rc.X += dx; rc.Y += dy }
	seen := map[*Box]bool{}
	all := append([]*Box{r.Root}, r.Boxes...)
	for _, b := range all {
		if b == nil || seen[b] {
			continue
		}
		seen[b] = true
		mv(&b.Rect)
		mv(&b.Shape)
		mv(&b.Content)
		if b.Icon.W > 0 || b.Icon.H > 0 {
			mv(&b.Icon)
		}
		for i := range b.Lines {
			b.Lines[i].X += dx
			b.Lines[i].Y += dy
		}
		for i := range b.Badges {
			mv(&b.Badges[i].Rect)
			b.Badges[i].Line.X += dx
			b.Badges[i].Line.Y += dy
		}
		if b.Status != nil {
			b.Status.CX += dx
			b.Status.CY += dy
		}
	}
	for _, e := range r.Edges {
		for i := range e.Points {
			e.Points[i] = model.Point{X: e.Points[i].X + dx, Y: e.Points[i].Y + dy}
		}
		if e.LabelRect.W > 0 {
			mv(&e.LabelRect)
		}
		if e.Label != nil {
			e.Label.X += dx
			e.Label.Y += dy
		}
	}
}
