package layout

import (
	"image/color"
	"math"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/text"
	"github.com/grindlemire/diagram/themes"
)

// Badge is a pill with text on a node's top-right corner.
type Badge struct {
	Rect Rect // the pill
	Line Line
	Fill themes.Color
}

// Dot is a status dot on a node's top-left corner.
type Dot struct {
	CX, CY, R float64
	Color     themes.Color
}

// Rect returns the dot's bounding box.
func (d *Dot) Rect() Rect { return Rect{d.CX - d.R, d.CY - d.R, 2 * d.R, 2 * d.R} }

// Badge and status dot metrics.
const (
	badgePadX = 6   // text to pill end
	badgePadY = 1   // text to pill top and bottom
	badgeGap  = 4   // between stacked badges
	dotR      = 4.5 // status dot radius
	// MarkRing is the width of the ring, in the colour behind the node,
	// drawn around badges and dots to set them off from the node's border.
	MarkRing = 2
)

// Marks returns the boxes a node's badges and status dot cover, rings
// included.
func (b *Box) Marks() []Rect {
	var out []Rect
	for _, bg := range b.Badges {
		out = append(out, inflateRect(bg.Rect, MarkRing))
	}
	if b.Status != nil {
		out = append(out, inflateRect(b.Status.Rect(), MarkRing))
	}
	return out
}

func inflateRect(r Rect, d float64) Rect { return Rect{r.X - d, r.Y - d, r.W + 2*d, r.H + 2*d} }

// layoutMarks places a node's badges and status dot on the corners of its
// outline, and picks the colour of the ring around them.
func (l *layouter) layoutMarks(b *Box) {
	e, t := b.El, l.t
	if len(e.Badges) == 0 && e.Status.IsZero() {
		return
	}
	b.Ring = l.behind(e)
	if !e.Status.IsZero() {
		p := l.corner(b, false)
		b.Status = &Dot{CX: p.X, CY: p.Y, R: dotR, Color: e.Status}
	}
	if len(e.Badges) == 0 {
		return
	}
	size := t.LabelFontSize
	f := text.FaceFor(text.SemiBold)
	asc, desc := f.Ascent(size), f.Descent(size)
	h := size*t.LineHeight + 2*badgePadY
	fill, col := themes.Of(themes.Accent), l.badgeText()
	p := l.corner(b, true)
	right := p.X + h/2
	for _, s := range e.Badges {
		tw := f.Width(s, size)
		w := math.Max(tw+2*badgePadX, h)
		r := Rect{right - w, p.Y - h/2, w, h}
		b.Badges = append(b.Badges, Badge{
			Rect: r, Fill: fill,
			Line: Line{X: r.X + w/2, Y: p.Y - (asc+desc)/2 + asc, Text: s, Size: size, Weight: text.SemiBold, Anchor: AnchorMiddle, Width: tw, Color: col},
		})
		right = r.X - badgeGap
	}
}

// corner returns the point on a node's outline nearest the top-right
// (right) or top-left corner of its box: on rounded corners and curved
// outlines, the middle of the curve.
func (l *layouter) corner(b *Box, right bool) model.Point {
	r := b.Shape
	x0, x1, y := r.X, r.Right(), r.Y
	in := 0.0 // how far the point sits in from both edges
	const k = 1 - math.Sqrt2/2
	e := b.El
	switch {
	case e.Shape == model.ShapePerson || e.IconPlace == model.IconOnly && e.Icon != nil || e.Shape == model.ShapeText:
	case e.Shape == model.ShapeBox, e.Shape == model.ShapeRect:
		in = math.Min(b.Look.Radius, r.H/2) * k
	case e.Shape == model.ShapePill:
		in = r.H / 2 * k
	case e.Shape == model.ShapeCircle:
		in = r.W / 2 * k
	case e.Shape == model.ShapeCylinder:
		// The widest point of the lid.
		y += b.Cap
	case e.Shape == model.ShapeQueue:
		// The middle of the curve of the end on that side.
		if right {
			x1 -= b.Cap * k
		} else {
			x0 += b.Cap * k
		}
		y += r.H / 2 * k
	case e.Shape == model.ShapeNote:
		if right {
			f := math.Min(12, math.Min(r.W, r.H)/3)
			x1 -= f / 2
			y += f / 2
		}
	}
	if right {
		return model.Point{X: x1 - in, Y: y + in}
	}
	return model.Point{X: x0 + in, Y: y + in}
}

// behind returns the colour behind a node: the fill of the nearest drawn
// group around it, or the background.
func (l *layouter) behind(e *model.Element) themes.Color {
	for p := e.Parent; p != nil; p = p.Parent {
		if pb := l.res.ByEl[p]; pb != nil && pb.Shown && !pb.Look.Fill.IsZero() {
			return pb.Look.Fill
		}
	}
	return themes.Of(themes.Background)
}

// badgeText returns the colour of badge text on the accent fill: the
// theme's background where that reads well on the accent in every palette
// the theme can show, as it does in the bundled themes, otherwise dark or
// white, whichever reads better.
func (l *layouter) badgeText() themes.Color {
	pals := l.palettes()
	fills := make([]color.RGBA, len(pals))
	for i, p := range pals {
		fills[i] = p[themes.Accent]
	}
	bg := themes.Of(themes.Background)
	if worst(bg, pals, fills) >= minTextContrast {
		return bg
	}
	if worstOf(lightText, fills) > worstOf(darkText, fills) {
		return rgb(lightText)
	}
	return rgb(darkText)
}
