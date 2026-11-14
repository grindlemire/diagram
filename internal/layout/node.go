package layout

import (
	"math"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/text"
	"github.com/grindlemire/diagram/themes"
)

// textBlock is measured, wrapped text.
type textBlock struct {
	lines  []string
	widths []float64
	size   float64
	weight text.Weight
	lineH  float64
	w, h   float64
}

func (l *layouter) block(s string, size float64, w text.Weight, maxW float64) textBlock {
	if s == "" {
		return textBlock{}
	}
	f := text.FaceFor(w)
	tb := textBlock{size: size, weight: w, lineH: size * l.t.LineHeight}
	tb.lines = f.Wrap(s, size, maxW)
	for _, ln := range tb.lines {
		lw := f.Width(ln, size)
		tb.widths = append(tb.widths, lw)
		tb.w = math.Max(tb.w, lw)
	}
	tb.h = tb.lineH * float64(len(tb.lines))
	return tb
}

// emit appends the block's lines with their top at y. For AnchorMiddle, x
// is the centre; for AnchorStart, the left edge.
func (l *layouter) emit(b *Box, tb textBlock, x, y float64, a Anchor, col themes.Color) {
	f := text.FaceFor(tb.weight)
	asc, desc := f.Ascent(tb.size), f.Descent(tb.size)
	for i, s := range tb.lines {
		base := y + float64(i)*tb.lineH + (tb.lineH-(asc+desc))/2 + asc
		b.Lines = append(b.Lines, Line{X: x, Y: base, Text: s, Size: tb.size, Weight: tb.weight, Anchor: a, Width: tb.widths[i], Color: col})
	}
}

type nodeMeasure struct {
	label, desc  textBlock
	iconW, iconH float64
	textW, textH float64
	contentW     float64
	contentH     float64
	figureH      float64 // Person figure height
	capY         float64 // Cylinder cap height (or Queue cap width)
	wave         float64 // Document wave height
}

func (l *layouter) labelWeight(e *model.Element) text.Weight {
	if e.Style.Bold.Set && !e.Style.Bold.V {
		return text.Regular
	}
	return text.SemiBold
}

func (l *layouter) iconSize(e *model.Element, def float64) (w, h float64) {
	if e.Icon == nil {
		return 0, 0
	}
	w = e.IconSize.Or(def)
	iw, ih := e.Icon.Size()
	h = w
	if iw > 0 {
		h = w * ih / iw
	}
	return w, h
}

const (
	personW, personH = 36, 44
	minCapY          = 6
)

func (l *layouter) measureNode(b *Box) {
	e, t := b.El, l.t
	m := &b.node
	fs := e.Style.FontSize.Or(t.FontSize)
	m.iconW, m.iconH = l.iconSize(e, t.IconSize)

	free := e.Shape == model.ShapePerson || e.IconPlace == model.IconOnly && e.Icon != nil
	maxText := e.MaxWidth.Or(t.NodeMaxWidth - 2*t.NodePaddingX)
	if e.IconPlace == model.IconLeft && e.Icon != nil {
		maxText -= m.iconW + t.IconGap
	}
	if free {
		maxText = e.MaxWidth.Or(t.NodeMaxWidth)
	}
	m.label = l.block(e.Label, fs, l.labelWeight(e), maxText)
	m.desc = l.block(e.Desc, t.DescFontSize, text.Regular, maxText)
	m.textW = math.Max(m.label.w, m.desc.w)
	m.textH = m.label.h
	if m.desc.h > 0 {
		if m.textH > 0 {
			m.textH += t.DescGap
		}
		m.textH += m.desc.h
	}

	if free {
		figW, figH := m.iconW, m.iconH
		if e.Shape == model.ShapePerson {
			figW, figH = personW, personH
		}
		m.figureH = figH
		b.bodyW = math.Max(figW, m.textW)
		b.bodyH = figH
		if m.textH > 0 {
			b.bodyH += t.IconGap + m.textH
		}
		b.slotW, b.slotH = b.bodyW, b.bodyH
		b.extent()
		return
	}

	switch {
	case e.Icon != nil && e.IconPlace == model.IconLeft:
		m.contentW = m.iconW
		if m.textW > 0 {
			m.contentW += t.IconGap + m.textW
		}
		m.contentH = math.Max(m.iconH, m.textH)
	case e.Icon != nil:
		m.contentW = math.Max(m.iconW, m.textW)
		m.contentH = m.iconH
		if m.textH > 0 {
			m.contentH += t.IconGap + m.textH
		}
	default:
		m.contentW, m.contentH = m.textW, m.textH
	}

	px, py := t.NodePaddingX, t.NodePaddingY
	if e.Shape == model.ShapeText {
		px, py = 4, 2
	}
	w := m.contentW + 2*px
	h := m.contentH + 2*py
	if e.Shape != model.ShapeText {
		w = math.Max(w, t.NodeMinWidth)
	}
	switch e.Shape {
	case model.ShapePill:
		w += h / 2
	case model.ShapeCircle:
		d := math.Hypot(m.contentW, m.contentH) + 2*py
		w, h = d, d
	case model.ShapeCylinder:
		m.capY = math.Max(minCapY, math.Min(w*0.08, 14))
		h += 2 * m.capY
	case model.ShapeQueue:
		m.capY = math.Max(minCapY, math.Min(h*0.2, 14))
		w += 2 * m.capY
	case model.ShapeDocument:
		m.wave = math.Max(6, h*0.12)
		h += m.wave
	case model.ShapeNote:
		w += 6
	}
	if e.Width.Set {
		w, h = e.Width.V, e.Height.V
	}
	b.bodyW, b.bodyH = w, h
	b.slotW, b.slotH = w, h
	b.extent()
}

// layoutNode positions a node's icon and text inside its body r, which
// lies within its extent b.Rect.
func (l *layouter) layoutNode(b *Box, r Rect) {
	e, m := b.El, &b.node
	t := l.t
	b.Shape = r
	b.Look = l.nodeLook(e)
	l.sampleLook(b)
	b.Cap, b.Wave = m.capY, m.wave
	defer l.layoutMarks(b)
	textCol, descCol := l.textColors(e, b.Look.Fill, t.NodeText, &b.Look)
	if e.Shape == model.ShapePerson || e.IconPlace == model.IconOnly && e.Icon != nil {
		figW := m.iconW
		if e.Shape == model.ShapePerson {
			figW = personW
		}
		fig := Rect{r.X + (r.W-figW)/2, r.Y, figW, m.figureH}
		b.Shape = fig
		if e.Shape != model.ShapePerson {
			b.Icon = fig
		}
		l.emitText(b, r.X+r.W/2, fig.Bottom()+t.IconGap, AnchorMiddle, textCol, descCol)
		return
	}

	// Content area, excluding shape-specific decoration.
	inner := r
	switch e.Shape {
	case model.ShapeCylinder:
		inner = Rect{r.X, r.Y + 2*m.capY, r.W, r.H - 2*m.capY}
	case model.ShapeQueue:
		inner = Rect{r.X, r.Y, r.W - 2*m.capY, r.H}
	case model.ShapeDocument:
		inner = Rect{r.X, r.Y, r.W, r.H - m.wave}
	}
	cx := inner.X + (inner.W-m.contentW)/2
	cy := inner.Y + (inner.H-m.contentH)/2

	switch {
	case e.Icon != nil && e.IconPlace == model.IconLeft:
		b.Icon = Rect{cx, cy + (m.contentH-m.iconH)/2, m.iconW, m.iconH}
		tx := cx + m.iconW + t.IconGap
		l.emitText(b, tx, cy+(m.contentH-m.textH)/2, AnchorStart, textCol, descCol)
	case e.Icon != nil:
		b.Icon = Rect{inner.X + (inner.W-m.iconW)/2, cy, m.iconW, m.iconH}
		l.emitText(b, inner.X+inner.W/2, cy+m.iconH+t.IconGap, AnchorMiddle, textCol, descCol)
	default:
		l.emitText(b, inner.X+inner.W/2, cy, AnchorMiddle, textCol, descCol)
	}
}

func (l *layouter) emitText(b *Box, x, y float64, a Anchor, col, descCol themes.Color) {
	m := &b.node
	l.emit(b, m.label, x, y, a, col)
	if m.desc.h > 0 {
		dy := y + m.label.h
		if m.label.h > 0 {
			dy += l.t.DescGap
		}
		l.emit(b, m.desc, x, dy, a, descCol)
	}
}

func (l *layouter) nodeLook(e *model.Element) Look {
	t := l.t
	s := e.Style.Over(model.Style{
		Fill:        t.NodeFill,
		Stroke:      t.NodeBorder,
		StrokeWidth: model.Some(t.NodeStroke),
		Radius:      model.Some(t.NodeRadius),
	})
	switch e.Shape {
	case model.ShapeRect:
		if !e.Style.Radius.Set {
			s.Radius = model.Some(0.0)
		}
	case model.ShapeNote:
		if e.Style.Fill.IsZero() {
			s.Fill = themes.Of(themes.Level2)
		}
		s.Radius = model.Some(0.0)
	case model.ShapeText:
		if e.Style.Fill.IsZero() {
			s.Fill = themes.Color{}
		}
		if e.Style.Stroke.IsZero() {
			s.Stroke = themes.Color{}
		}
	case model.ShapePerson:
		if e.Style.Fill.IsZero() {
			s.Fill = themes.Of(themes.Level2)
		}
	}
	return Look{
		Fill: s.Fill, Stroke: s.Stroke, StrokeWidth: s.StrokeWidth.V,
		Dash: s.Dash, Radius: s.Radius.V, IconColor: l.iconColor(e),
	}
}

func (l *layouter) iconColor(e *model.Element) themes.Color {
	if !e.IconColor.IsZero() {
		return e.IconColor
	}
	return l.t.IconColor
}

// ---- group headers ----

type header struct {
	label, desc  textBlock
	iconW, iconH float64
	textW, textH float64
	w, h         float64
}

const headerIconSize = 20

func (l *layouter) measureHeader(e *model.Element) header {
	t := l.t
	var h header
	h.label = l.block(e.Label, e.Style.FontSize.Or(t.HeaderFontSize), l.labelWeight(e), 0)
	h.desc = l.block(e.Desc, t.DescFontSize, text.Regular, 0)
	h.iconW, h.iconH = l.iconSize(e, headerIconSize)
	h.textW = math.Max(h.label.w, h.desc.w)
	h.textH = h.label.h
	if h.desc.h > 0 {
		if h.textH > 0 {
			h.textH += t.DescGap
		}
		h.textH += h.desc.h
	}
	h.w = h.textW
	if h.iconW > 0 {
		h.w += h.iconW
		if h.textW > 0 {
			h.w += t.IconGap
		}
	}
	h.h = math.Max(h.iconH, h.textH)
	return h
}

func (l *layouter) layoutHeader(b *Box, area Rect) {
	h, t := b.hdr, l.t
	if h.h == 0 {
		return
	}
	x := area.X
	if b.El.HeaderTop {
		x = area.X + (area.W-h.w)/2
	}
	if h.iconW > 0 {
		b.Icon = Rect{x, area.Y + (h.h-h.iconH)/2, h.iconW, h.iconH}
		x += h.iconW + t.IconGap
	}
	col, descCol := l.textColors(b.El, b.Look.Fill, t.GroupText, &b.Look)
	y := area.Y + (h.h-h.textH)/2
	l.emit(b, h.label, x, y, AnchorStart, col)
	if h.desc.h > 0 {
		dy := y + h.label.h
		if h.label.h > 0 {
			dy += t.DescGap
		}
		l.emit(b, h.desc, x, dy, AnchorStart, descCol)
	}
}

func (l *layouter) groupLook(e *model.Element) Look {
	t := l.t
	s := e.Style.Over(model.Style{
		Fill:        t.GroupFill(e.Depth),
		Stroke:      t.GroupBorder,
		StrokeWidth: model.Some(t.GroupStroke),
		Radius:      model.Some(t.GroupRadius),
	})
	return Look{Fill: s.Fill, Stroke: s.Stroke, StrokeWidth: s.StrokeWidth.V, Dash: s.Dash, Radius: s.Radius.V, IconColor: l.iconColor(e)}
}

// ---- edges ----

func (l *layouter) edge(e *model.Edge) *Edge {
	t := l.t
	s := e.Style.Over(model.Style{Stroke: t.EdgeColor, StrokeWidth: model.Some(t.EdgeWidth)})
	out := &Edge{
		E: e, From: l.res.ByEl[e.From], To: l.res.ByEl[e.To],
		Color: s.Stroke, Width: s.StrokeWidth.V, Dash: s.Dash,
		Head: e.Head, Tail: e.Tail, Rounded: !e.Straight,
	}
	if out.Head == model.ArrowUnset {
		out.Head = model.ArrowTriangle
	}
	if out.Tail == model.ArrowUnset {
		out.Tail = model.ArrowNone
	}
	if e.Label != "" {
		w := text.Regular
		if s.Bold.Set && s.Bold.V {
			w = text.SemiBold
		}
		size := s.FontSize.Or(t.LabelFontSize)
		col := e.Style.TextColor
		if col.IsZero() {
			col = t.EdgeLabelText
		}
		out.Label = &Line{Text: e.Label, Size: size, Weight: w, Anchor: AnchorMiddle, Width: text.FaceFor(w).Width(e.Label, size), Color: col}
	}
	return out
}

// LabelSize returns the size of an edge label's background pill.
func LabelSize(t *themes.Theme, ln *Line) (w, h float64) {
	return ln.Width + 2*t.LabelPaddingX, ln.Size*t.LineHeight + 2*t.LabelPaddingY
}

// PlaceLabel centres an edge label (and its pill) on a point.
func PlaceLabel(t *themes.Theme, e *Edge, at model.Point) {
	if e.Label == nil {
		return
	}
	w, h := LabelSize(t, e.Label)
	e.LabelRect = Rect{at.X - w/2, at.Y - h/2, w, h}
	f := text.FaceFor(e.Label.Weight)
	asc, desc := f.Ascent(e.Label.Size), f.Descent(e.Label.Size)
	e.Label.X = at.X
	e.Label.Y = at.Y - (asc+desc)/2 + asc
}
