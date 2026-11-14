// Package paint turns a laid-out, routed diagram into scene primitives.
package paint

import (
	"fmt"
	"math"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/scene"
	"github.com/grindlemire/diagram/internal/text"
	"github.com/grindlemire/diagram/themes"
)

// Options control painting.
type Options struct {
	Margin      float64
	Transparent bool
	// Interactive also emits an empty group for each invisible container,
	// so that every edge endpoint and parent in Meta names a drawn group.
	Interactive bool
}

// Paint builds the scene. Draw order: groups (outermost first), edges,
// nodes, then edge labels, so labels are never hidden.
func Paint(res *layout.Result, opt Options) *scene.Scene {
	t := res.Theme
	sc := &scene.Scene{
		Width:  res.Width + 2*opt.Margin,
		Height: res.Height + 2*opt.Margin,
		Theme:  t,
	}
	if !opt.Transparent {
		sc.Background = themes.Of(themes.Background)
	}
	var groups, nodes, edges, labels []scene.Item
	for _, b := range res.Boxes {
		switch {
		case b.El.Generated:
			// Legend entries: samples and names, drawn as pictures rather
			// than as diagram elements.
			if b.El.Kind == model.KindNode {
				nodes = append(nodes, entry(t, b))
			}
		case b.El.Kind == model.KindNode:
			nodes = append(nodes, node(t, b))
		case b.Shown:
			groups = append(groups, group(b))
		case opt.Interactive && b.El.Parent != nil:
			m := meta(b.El)
			m.Hidden = true
			groups = append(groups, &scene.Group{ID: b.El.ID, Class: "group", Meta: m})
		}
	}
	for _, e := range res.Edges {
		edges = append(edges, edge(t, e))
		if e.Label != nil && e.LabelRect.W > 0 {
			labels = append(labels, label(t, e))
		}
	}
	items := append(append(append(groups, edges...), nodes...), labels...)
	scene.Translate(items, opt.Margin, opt.Margin)
	sc.Items = items
	return sc
}

func dash(d model.Dash, w float64) ([]float64, scene.LineCap) {
	w = math.Max(w, 1)
	switch d {
	case model.DashDashed:
		return []float64{5 * w, 3 * w}, scene.CapButt
	case model.DashDotted:
		return []float64{0.01, 2.5 * w}, scene.CapRound
	}
	return nil, scene.CapButt
}

func boxPaint(l layout.Look) scene.Paint {
	d, c := dash(l.Dash, l.StrokeWidth)
	p := scene.Paint{Fill: l.Fill, StrokeWidth: l.StrokeWidth, Dash: d, Cap: c, Join: scene.JoinRound}
	if l.StrokeWidth > 0 {
		p.Stroke = l.Stroke
	}
	return p
}

func lines(b *layout.Box) []scene.Item {
	var out []scene.Item
	for _, ln := range b.Lines {
		out = append(out, textItem(ln))
	}
	return out
}

func textItem(ln layout.Line) *scene.Text {
	a := scene.AnchorStart
	if ln.Anchor == layout.AnchorMiddle {
		a = scene.AnchorMiddle
	}
	w := scene.Regular
	if ln.Weight == text.SemiBold {
		w = scene.SemiBold
	}
	return &scene.Text{X: ln.X, Y: ln.Y, Text: ln.Text, Size: ln.Size, Weight: w, Anchor: a, Color: ln.Color, Width: ln.Width}
}

func icon(b *layout.Box) []scene.Item {
	if b.El.Icon == nil || b.Icon.W == 0 {
		return nil
	}
	r := b.Icon
	return []scene.Item{&scene.Image{X: r.X, Y: r.Y, W: r.W, H: r.H, Icon: b.El.Icon, Color: b.Look.IconColor}}
}

// meta describes a node or container.
func meta(e *model.Element) *scene.Meta {
	m := &scene.Meta{Kind: scene.KindNode, Label: e.Label, Desc: e.Desc, Tooltip: e.Tooltip, Link: e.Link}
	if e.Kind.IsGroup() {
		m.Kind = scene.KindGroup
	}
	if p := e.Parent; p != nil && p.Parent != nil {
		m.Parent = p.ID
	}
	return m
}

// classes returns base followed by a class for each named style.
func classes(base string, names []string) string {
	for _, n := range names {
		base += " " + StyleClass(n)
	}
	return base
}

// StyleClass is the CSS class SVG and HTML output give elements that use
// the named style name.
func StyleClass(name string) string {
	if s := model.Slug(name); s != "" {
		return "style-" + s
	}
	return fmt.Sprintf("style-%x", name)
}

func group(b *layout.Box) scene.Item {
	r := b.Rect
	base := "group"
	if b.El.Legend {
		base = "group legend"
	}
	g := &scene.Group{ID: b.El.ID, Class: classes(base, b.El.Style.Names), Meta: meta(b.El)}
	g.Items = append(g.Items, &scene.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H, R: b.Look.Radius, Paint: boxPaint(b.Look)})
	g.Items = append(g.Items, icon(b)...)
	g.Items = append(g.Items, lines(b)...)
	return g
}

func node(t *themes.Theme, b *layout.Box) scene.Item {
	g := &scene.Group{ID: b.El.ID, Class: classes("node", b.El.Style.Names), Meta: meta(b.El)}
	g.Items = append(g.Items, shape(b)...)
	g.Items = append(g.Items, icon(b)...)
	g.Items = append(g.Items, lines(b)...)
	g.Items = append(g.Items, marks(b)...)
	return g
}

// marks draws a node's status dot and badges, each on a ring in the colour
// behind the node.
func marks(b *layout.Box) []scene.Item {
	var out []scene.Item
	ring := scene.Paint{Fill: b.Ring}
	if d := b.Status; d != nil {
		out = append(out,
			&scene.Ellipse{CX: d.CX, CY: d.CY, RX: d.R + layout.MarkRing, RY: d.R + layout.MarkRing, Paint: ring},
			&scene.Ellipse{CX: d.CX, CY: d.CY, RX: d.R, RY: d.R, Paint: scene.Paint{Fill: d.Color}})
	}
	// Leftmost first, so a badge's ring never cuts into the badge on its
	// right.
	for i := len(b.Badges) - 1; i >= 0; i-- {
		bg := b.Badges[i]
		r := bg.Rect
		o := r.H/2 + layout.MarkRing
		out = append(out,
			&scene.Rect{X: r.X - layout.MarkRing, Y: r.Y - layout.MarkRing, W: r.W + 2*layout.MarkRing, H: r.H + 2*layout.MarkRing, R: o, Paint: ring},
			&scene.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H, R: r.H / 2, Paint: scene.Paint{Fill: bg.Fill}},
			textItem(bg.Line))
	}
	if len(out) == 0 {
		return nil
	}
	return []scene.Item{&scene.Group{Class: "marks", Items: out}}
}

// entry draws a generated legend entry: a named style's sample, or its
// name. It carries no Meta, so interactive output treats it as part of
// the picture rather than as an element.
func entry(t *themes.Theme, b *layout.Box) scene.Item {
	g := &scene.Group{ID: b.El.ID, Class: classes("legend-entry", b.El.Style.Names)}
	switch s := b.Sample; {
	case s == nil:
		g.Items = append(g.Items, lines(b)...)
	case s.Kind == layout.SampleEdge:
		r := b.Shape
		y := r.Y + r.H/2
		e := &layout.Edge{
			E:      &model.Edge{},
			Points: []model.Point{{X: r.X, Y: y}, {X: r.Right(), Y: y}},
			Color:  s.Color, Width: s.Width, Dash: s.Dash, Head: model.ArrowTriangle, Tail: model.ArrowNone,
		}
		g.Items = append(g.Items, edge(t, e).(*scene.Group).Items...)
	default:
		g.Items = append(g.Items, shape(b)...)
	}
	return g
}

const kappa = 0.5522847498 // cubic approximation of a quarter circle

func shape(b *layout.Box) []scene.Item {
	e := b.El
	p := boxPaint(b.Look)
	r := b.Shape
	x, y, w, h := r.X, r.Y, r.W, r.H
	if e.IconPlace == model.IconOnly && e.Icon != nil && e.Shape != model.ShapePerson {
		return nil
	}
	switch e.Shape {
	case model.ShapeText:
		return nil
	case model.ShapeBox, model.ShapeRect:
		return []scene.Item{&scene.Rect{X: x, Y: y, W: w, H: h, R: math.Min(b.Look.Radius, h/2), Paint: p}}
	case model.ShapePill:
		return []scene.Item{&scene.Rect{X: x, Y: y, W: w, H: h, R: h / 2, Paint: p}}
	case model.ShapeCircle:
		return []scene.Item{&scene.Ellipse{CX: x + w/2, CY: y + h/2, RX: w / 2, RY: h / 2, Paint: p}}
	case model.ShapeCylinder:
		ry := b.Cap
		rx := w / 2
		var body scene.Path
		body.MoveTo(x, y+ry)
		body.LineTo(x, y+h-ry)
		halfEllipse(&body, x+rx, y+h-ry, rx, ry, true)
		body.LineTo(x+w, y+ry)
		halfEllipse(&body, x+rx, y+ry, rx, -ry, false)
		body.Close()
		var rim scene.Path
		rim.MoveTo(x, y+ry)
		halfEllipse(&rim, x+rx, y+ry, rx, ry, true)
		return []scene.Item{&scene.Shape{Path: body, Paint: p}, &scene.Shape{Path: rim, Paint: strokeOnly(p)}}
	case model.ShapeQueue:
		rx := b.Cap
		ry := h / 2
		var body scene.Path
		body.MoveTo(x+rx, y)
		body.LineTo(x+w-rx, y)
		halfEllipseV(&body, x+w-rx, y+ry, rx, ry, true)
		body.LineTo(x+rx, y+h)
		halfEllipseV(&body, x+rx, y+ry, -rx, ry, false)
		body.Close()
		var rim scene.Path
		rim.MoveTo(x+w-rx, y)
		halfEllipseV(&rim, x+w-rx, y+ry, -rx, ry, true)
		return []scene.Item{&scene.Shape{Path: body, Paint: p}, &scene.Shape{Path: rim, Paint: strokeOnly(p)}}
	case model.ShapeDocument:
		wave := b.Wave
		yb := y + h - wave/2
		var path scene.Path
		path.MoveTo(x, y)
		path.LineTo(x+w, y)
		path.LineTo(x+w, yb)
		path.CubicTo(x+0.85*w, yb+wave*1.1, x+0.65*w, yb+wave*1.1, x+0.5*w, yb)
		path.CubicTo(x+0.35*w, yb-wave*1.1, x+0.15*w, yb-wave*1.1, x, yb)
		path.Close()
		return []scene.Item{&scene.Shape{Path: path, Paint: p}}
	case model.ShapeNote:
		f := math.Min(12, math.Min(w, h)/3)
		var path scene.Path
		path.MoveTo(x, y)
		path.LineTo(x+w-f, y)
		path.LineTo(x+w, y+f)
		path.LineTo(x+w, y+h)
		path.LineTo(x, y+h)
		path.Close()
		var fold scene.Path
		fold.MoveTo(x+w-f, y)
		fold.LineTo(x+w-f, y+f)
		fold.LineTo(x+w, y+f)
		return []scene.Item{&scene.Shape{Path: path, Paint: p}, &scene.Shape{Path: fold, Paint: strokeOnly(p)}}
	case model.ShapePerson:
		cx := x + w/2
		hr := w * 0.24
		head := &scene.Ellipse{CX: cx, CY: y + hr + 1, RX: hr, RY: hr, Paint: p}
		top := y + 2*hr + 4
		var body scene.Path
		body.MoveTo(x, y+h)
		body.LineTo(x, top+w*0.3)
		body.CubicTo(x, top+w*0.3-w*0.3*kappa, cx-w*0.5*kappa, top, cx, top)
		body.CubicTo(cx+w*0.5*kappa, top, x+w, top+w*0.3-w*0.3*kappa, x+w, top+w*0.3)
		body.LineTo(x+w, y+h)
		body.Close()
		return []scene.Item{head, &scene.Shape{Path: body, Paint: p}}
	}
	return nil
}

func strokeOnly(p scene.Paint) scene.Paint {
	p.Fill = themes.Color{}
	return p
}

// halfEllipse appends the lower (down=true) or upper half of an ellipse
// centred at (cx, cy), from its left point to its right point when down,
// or right to left otherwise. A negative ry flips it.
func halfEllipse(p *scene.Path, cx, cy, rx, ry float64, down bool) {
	if down {
		p.CubicTo(cx-rx, cy+ry*kappa, cx-rx*kappa, cy+ry, cx, cy+ry)
		p.CubicTo(cx+rx*kappa, cy+ry, cx+rx, cy+ry*kappa, cx+rx, cy)
		return
	}
	p.CubicTo(cx+rx, cy+ry*kappa, cx+rx*kappa, cy+ry, cx, cy+ry)
	p.CubicTo(cx-rx*kappa, cy+ry, cx-rx, cy+ry*kappa, cx-rx, cy)
}

// halfEllipseV appends the right (right=true, top to bottom) or left
// (bottom to top) half of an ellipse. A negative rx flips it.
func halfEllipseV(p *scene.Path, cx, cy, rx, ry float64, right bool) {
	if right {
		p.CubicTo(cx+rx*kappa, cy-ry, cx+rx, cy-ry*kappa, cx+rx, cy)
		p.CubicTo(cx+rx, cy+ry*kappa, cx+rx*kappa, cy+ry, cx, cy+ry)
		return
	}
	p.CubicTo(cx+rx*kappa, cy+ry, cx+rx, cy+ry*kappa, cx+rx, cy)
	p.CubicTo(cx+rx, cy-ry*kappa, cx+rx*kappa, cy-ry, cx, cy-ry)
}

// ---- edges ----

func edge(t *themes.Theme, e *layout.Edge) scene.Item {
	g := &scene.Group{ID: e.E.ID, Class: classes("edge", e.E.Style.Names), Meta: &scene.Meta{
		Kind: scene.KindEdge, Label: e.E.Label, Tooltip: e.E.Tooltip, From: idOf(e.E.From), To: idOf(e.E.To),
	}}
	pts := append([]model.Point(nil), e.Points...)
	if len(pts) < 2 {
		return g
	}
	d, c := dash(e.Dash, e.Width)
	line := scene.Paint{Stroke: e.Color, StrokeWidth: e.Width, Dash: d, Cap: c, Join: scene.JoinRound}
	if c == scene.CapButt {
		line.Cap = scene.CapRound
		if d != nil {
			line.Cap = scene.CapButt
		}
	}

	size := t.ArrowSize * math.Max(1, e.Width/1.5)
	head, tail := e.Head != model.ArrowNone, e.Tail != model.ArrowNone
	var heads []scene.Item
	if head {
		var h scene.Item
		pts, h = arrowEnd(pts, false, tail, e.Head, size, e.Color, e.Width)
		heads = append(heads, h)
	}
	if tail {
		var h scene.Item
		pts, h = arrowEnd(pts, true, head, e.Tail, size, e.Color, e.Width)
		heads = append(heads, h)
	}
	r := 0.0
	if e.Rounded {
		r = t.EdgeCornerRad
	}
	g.Items = append(g.Items, &scene.Shape{Path: polyline(pts, r), Paint: line})
	g.Items = append(g.Items, heads...)
	return g
}

func idOf(e *model.Element) string {
	if e == nil {
		return ""
	}
	return e.ID
}

// minStraight is the shortest straight run worth drawing between a bend
// and an arrowhead; below it a bend is left sharp.
const minStraight = 2

// arrowEnd draws the arrowhead at the target end of pts, or at the source
// end when atStart, and returns pts with that end pulled back to the
// arrowhead's base. other says whether the opposite end has an arrowhead
// too.
//
// The arrowhead shrinks (to half its size at most) so that it fits on the
// final segment with a little straight line left before the bend behind
// it. If even that is too long, the line stops at the bend instead of
// doubling back along the segment.
func arrowEnd(pts []model.Point, atStart, other bool, k model.Arrow, size float64, col themes.Color, lw float64) ([]model.Point, scene.Item) {
	pts = append([]model.Point(nil), pts...)
	n := len(pts)
	tipI, prevI := n-1, n-2
	if atStart {
		tipI, prevI = 0, 1
	}
	tip, prev := pts[tipI], pts[prevI]
	seg := dist(prev, tip)
	// The furthest the line may be pulled back: the bend, or the middle of
	// a lone segment shared with another arrowhead.
	limit := prev
	if n == 2 && other {
		limit = model.Point{X: (prev.X + tip.X) / 2, Y: (prev.Y + tip.Y) / 2}
	}
	avail := dist(limit, tip)
	if fit := (avail - minStraight) / headLen(k); fit < size {
		size = math.Max(fit, size/2)
	}
	end, item := arrow(k, prev, tip, size, col, lw)
	if dist(tip, end) > avail || seg > 0 && dot(end, prev, tip) <= 0 {
		end = limit
	}
	pts[tipI] = end
	if n > 2 && dist(end, prev) < 1e-9 {
		pts = append(pts[:tipI], pts[tipI+1:]...)
	}
	return pts, item
}

// headLen is how far an arrowhead of size 1 reaches back from its tip.
func headLen(k model.Arrow) float64 {
	switch k {
	case model.ArrowDiamond:
		return 1.5
	case model.ArrowDot:
		return 0.8
	}
	return 1
}

// dot is the dot product of p-from and to-from.
func dot(p, from, to model.Point) float64 {
	return (p.X-from.X)*(to.X-from.X) + (p.Y-from.Y)*(to.Y-from.Y)
}

// polyline builds a path through pts with bends rounded to radius r. A
// bend may use up half of a segment it shares with another bend, or all of
// the first and last segments (which end at a port or an arrowhead); bends
// with less than minStraight of room stay sharp.
func polyline(pts []model.Point, r float64) scene.Path {
	var p scene.Path
	p.MoveTo(pts[0].X, pts[0].Y)
	last := len(pts) - 1
	for i := 1; i < last; i++ {
		a, b, c := pts[i-1], pts[i], pts[i+1]
		in, out := dist(a, b), dist(b, c)
		if i-1 > 0 {
			in /= 2
		}
		if i+1 < last {
			out /= 2
		}
		rr := math.Min(r, math.Min(in, out))
		if rr < minStraight || collinear(a, b, c) {
			p.LineTo(b.X, b.Y)
			continue
		}
		from := towards(b, a, rr)
		to := towards(b, c, rr)
		p.LineTo(from.X, from.Y)
		p.QuadTo(b.X, b.Y, to.X, to.Y)
	}
	p.LineTo(pts[last].X, pts[last].Y)
	return p
}

func dist(a, b model.Point) float64 { return math.Hypot(b.X-a.X, b.Y-a.Y) }

func towards(from, to model.Point, d float64) model.Point {
	l := dist(from, to)
	if l == 0 {
		return from
	}
	return model.Point{X: from.X + (to.X-from.X)*d/l, Y: from.Y + (to.Y-from.Y)*d/l}
}

func collinear(a, b, c model.Point) bool {
	return math.Abs((b.X-a.X)*(c.Y-a.Y)-(b.Y-a.Y)*(c.X-a.X)) < 1e-6
}

// arrow draws an arrowhead with its tip at tip, pointing away from prev,
// and returns where the line should now end.
func arrow(k model.Arrow, prev, tip model.Point, size float64, col themes.Color, lw float64) (model.Point, scene.Item) {
	l := dist(prev, tip)
	if l == 0 {
		return tip, &scene.Group{}
	}
	ux, uy := (tip.X-prev.X)/l, (tip.Y-prev.Y)/l // direction
	nx, ny := -uy, ux                            // normal
	at := func(along, across float64) (float64, float64) {
		return tip.X - ux*along + nx*across, tip.Y - uy*along + ny*across
	}
	fill := scene.Paint{Fill: col}
	half := size * 0.45
	var p scene.Path
	switch k {
	case model.ArrowOpen:
		x1, y1 := at(size, half)
		x2, y2 := at(size, -half)
		p.MoveTo(x1, y1)
		p.LineTo(tip.X, tip.Y)
		p.LineTo(x2, y2)
		return model.Point{X: tip.X - ux*lw/2, Y: tip.Y - uy*lw/2},
			&scene.Shape{Path: p, Paint: scene.Paint{Stroke: col, StrokeWidth: lw, Cap: scene.CapRound, Join: scene.JoinRound}}
	case model.ArrowDiamond:
		x1, y1 := at(size*0.75, half)
		x2, y2 := at(size*1.5, 0)
		x3, y3 := at(size*0.75, -half)
		p.MoveTo(tip.X, tip.Y)
		p.LineTo(x1, y1)
		p.LineTo(x2, y2)
		p.LineTo(x3, y3)
		p.Close()
		bx, by := at(size*1.5, 0)
		return model.Point{X: bx, Y: by}, &scene.Shape{Path: p, Paint: fill}
	case model.ArrowDot:
		rad := size * 0.4
		cx, cy := at(rad, 0)
		bx, by := at(rad*2, 0)
		return model.Point{X: bx, Y: by}, &scene.Ellipse{CX: cx, CY: cy, RX: rad, RY: rad, Paint: fill}
	}
	x1, y1 := at(size, half)
	x2, y2 := at(size, -half)
	p.MoveTo(tip.X, tip.Y)
	p.LineTo(x1, y1)
	p.LineTo(x2, y2)
	p.Close()
	bx, by := at(size*0.8, 0)
	return model.Point{X: bx, Y: by}, &scene.Shape{Path: p, Paint: scene.Paint{Fill: col, Stroke: col, StrokeWidth: 1, Join: scene.JoinRound}}
}

func label(t *themes.Theme, e *layout.Edge) scene.Item {
	r := e.LabelRect
	return &scene.Group{Class: classes("edge-label", e.E.Style.Names), Meta: &scene.Meta{Kind: scene.KindLabel, For: e.E.ID}, Items: []scene.Item{
		&scene.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H, R: r.H / 2, Paint: scene.Paint{Fill: t.EdgeLabelFill}},
		textItem(*e.Label),
	}}
}
