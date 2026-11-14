// Package layout sizes and places every element of a model.
//
// Layout runs in two passes. measure walks the tree bottom-up and computes
// each element's natural size from its content. place walks top-down and
// assigns absolute positions. Containers in a flow or grid stretch across
// the cross axis to fill the space their parent gives them; nodes keep their
// natural size and are aligned inside it.
//
// A node has three nested sizes. Its body is what it draws: the box, or for
// Person and IconOnly nodes the figure with its label. Its slot is the body
// grown to the size Uniform gives its siblings (shapes with intrinsic
// proportions keep their body and are centred in the slot). Its extent,
// Box.Rect, is the slot plus a band on Box.LoopSide reserved for its
// self-loops. Containers place and align nodes by their extent.
package layout

import (
	"errors"
	"fmt"
	"math"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/text"
	"github.com/grindlemire/diagram/themes"
)

// Rect is an axis-aligned box.
type Rect struct{ X, Y, W, H float64 }

// Center returns the centre of r.
func (r Rect) Center() model.Point { return model.Point{X: r.X + r.W/2, Y: r.Y + r.H/2} }

// Right and Bottom return the far edges.
func (r Rect) Right() float64  { return r.X + r.W }
func (r Rect) Bottom() float64 { return r.Y + r.H }

// Contains reports whether r fully contains o.
func (r Rect) Contains(o Rect) bool {
	const eps = 1e-6
	return o.X >= r.X-eps && o.Y >= r.Y-eps && o.Right() <= r.Right()+eps && o.Bottom() <= r.Bottom()+eps
}

// Overlaps reports whether r and o share any area.
func (r Rect) Overlaps(o Rect) bool {
	const eps = 1e-6
	return r.X < o.Right()-eps && o.X < r.Right()-eps && r.Y < o.Bottom()-eps && o.Y < r.Bottom()-eps
}

// Line is one line of text. X is its anchor point and Y its baseline.
type Line struct {
	X, Y   float64
	Text   string
	Size   float64
	Weight text.Weight
	Anchor Anchor
	Width  float64
	Color  themes.Color
}

// Anchor is horizontal text alignment.
type Anchor uint8

const (
	AnchorStart Anchor = iota
	AnchorMiddle
)

// Look is an element's resolved appearance.
type Look struct {
	Fill        themes.Color // zero for none
	Stroke      themes.Color // zero for none
	StrokeWidth float64
	Dash        model.Dash
	Radius      float64
	IconColor   themes.Color
}

// Box is a laid-out element.
type Box struct {
	El   *model.Element
	Rect Rect // the element's full extent

	// Shape is the outline that is drawn and that edges attach to. It
	// equals Rect except for IconOnly and Person nodes, whose outline is
	// the icon or figure only.
	Shape Rect

	Look  Look
	Lines []Line // label, description and header text
	Icon  Rect   // zero W when there is no icon
	Shown bool   // false for invisible containers

	// Content is where a container's children sit.
	Content Rect

	// Cap is a Cylinder's cap height or a Queue's cap width; Wave is a
	// Document's wave height.
	Cap, Wave float64

	// LoopSide is the side of a node reserved for its self-loops, or
	// SideAuto when it has none. Layout widens Rect on that side to make
	// room for the loops and their labels; Shape stays the drawn outline.
	LoopSide model.Side

	// Badges and Status are a node's badges, rightmost first, and its
	// status dot; Ring is the colour of the ring drawn around them.
	Badges []Badge
	Status *Dot
	Ring   themes.Color

	// Sample is set on a legend entry's sample, which draws the named
	// style it stands for instead of a node.
	Sample *Sample

	kids       []*Box  // children in layout order, generated ones included
	natW, natH float64 // natural size from measure: the extent for nodes
	hdr        header  // measured group header
	node       nodeMeasure

	// Node sizes; see the package comment.
	bodyW, bodyH float64
	slotW, slotH float64
	// loopDepth is how far the self-loop band reaches out from the slot on
	// LoopSide; loopAlong is how much room the loops' labels need along
	// that side.
	loopDepth, loopAlong float64
}

// Edge is an edge with resolved geometry. Routing fills in Points, the
// sides and the label position.
type Edge struct {
	E        *model.Edge
	From, To *Box

	Points    []model.Point // polyline from source border to target border
	ExitSide  model.Side
	EnterSide model.Side
	Rounded   bool // draw bends with Theme.EdgeCornerRad

	Color     themes.Color
	Width     float64
	Dash      model.Dash
	Head      model.Arrow
	Tail      model.Arrow
	Label     *Line
	LabelRect Rect // background pill; zero W when there's no label
}

// Result is a laid-out diagram.
type Result struct {
	Diagram *model.Diagram
	Theme   *themes.Theme
	Width   float64
	Height  float64
	Boxes   []*Box // pre-order, declaration order
	ByEl    map[*model.Element]*Box
	Root    *Box
	Edges   []*Edge
}

// Layout sizes and places every element of d using theme t. Edges get
// their resolved styles but no geometry; routing adds that. It returns the
// problems it finds, such as overlapping canvas children, joined with
// errors.Join; the result is complete either way.
func Layout(d *model.Diagram, t *themes.Theme) (*Result, error) {
	return LayoutSpaced(d, t, nil)
}

// Spacing is extra room added to a container's gaps between children:
// X between children side by side (a Row's gaps, a Grid's column gaps),
// Y between children stacked (a Column's gaps, a Grid's row gaps).
type Spacing struct{ X, Y float64 }

// LayoutSpaced is Layout with extra gap space for some containers. The
// model is left as it is.
func LayoutSpaced(d *model.Diagram, t *themes.Theme, extra map[*model.Element]Spacing) (*Result, error) {
	d.Finalize()
	l := &layouter{t: t, extra: extra, res: &Result{Diagram: d, Theme: t, ByEl: map[*model.Element]*Box{}},
		samples: map[*model.Element]*Sample{}}
	l.legendKids = l.legends(d)
	root := l.box(d.Root)
	for _, e := range d.Edges {
		l.res.Edges = append(l.res.Edges, l.edge(e))
	}
	l.reserveLoops()
	l.floor = l.spacingFloor()
	l.measure(root)
	l.place(root, Rect{0, 0, root.natW, root.natH})
	l.res.Root = root
	l.res.Width, l.res.Height = root.natW, root.natH
	return l.res, errors.Join(l.errs...)
}

type layouter struct {
	t          *themes.Theme
	legendKids map[*model.Element][]*model.Element // generated legend entries
	samples    map[*model.Element]*Sample
	extra      map[*model.Element]Spacing
	floor      map[*model.Element]Spacing // least gaps, see spacingFloor
	res        *Result
	errs       []error
}

// loopStep is how far each self-loop on a side reaches out from the node.
// It matches the router's stand-off for a first loop, which later loops on
// the same side nest outside of.
func loopStep(t *themes.Theme) float64 { return t.EdgeClearance + 8 }

// reserveLoops picks each node's LoopSide and sizes the band its self-loops
// and their labels need there. The side is the pinned side of the node's
// first pinned self-loop, or else the side facing the least traffic (see
// quietSide). Loops pinned to another side get no band.
func (l *layouter) reserveLoops() {
	count := map[*Box]int{}
	for _, e := range l.res.Edges {
		b := e.From
		if b == nil || b != e.To || b.El.Kind != model.KindNode || b.LoopSide != model.SideAuto {
			continue
		}
		b.LoopSide = l.quietSide(b)
		for _, o := range l.res.Edges {
			if o.From == b && o.To == b {
				if side := loopPin(o.E); side != model.SideAuto {
					b.LoopSide = side
					break
				}
			}
		}
	}
	for _, e := range l.res.Edges {
		b := e.From
		if b == nil || b != e.To || b.El.Kind != model.KindNode {
			continue
		}
		side := loopPin(e.E)
		if side != model.SideAuto && side != b.LoopSide {
			continue
		}
		count[b]++
		if e.Label == nil {
			continue
		}
		lw, lh := LabelSize(l.t, e.Label)
		across, along := lw, lh
		if b.LoopSide == model.SideTop || b.LoopSide == model.SideBottom {
			across, along = lh, lw
		}
		b.loopDepth = math.Max(b.loopDepth, across)
		b.loopAlong = math.Max(b.loopAlong, along)
	}
	for _, b := range l.res.Boxes {
		if n := count[b]; n > 0 {
			b.loopDepth += float64(n)*loopStep(l.t) + l.t.EdgeClearance
		}
	}
}

// quietSide is the side of node b least in the way of other things, for
// its self-loops: in a Row, Top or Bottom; in a Column, Right or Left; in
// a Grid, any side, counting occupied neighbouring cells; elsewhere Right.
// Each candidate side scores the edges that leave b towards it (and, in a
// grid, a neighbour on it); ties go to the first candidate listed.
func (l *layouter) quietSide(b *Box) model.Side {
	e := b.El
	var cands []model.Side
	parent := e.Parent
	switch {
	case parent == nil:
	case parent.Kind == model.KindRow:
		cands = []model.Side{model.SideTop, model.SideBottom}
	case parent.Kind == model.KindColumn:
		cands = []model.Side{model.SideRight, model.SideLeft}
	case parent.Kind == model.KindGrid:
		cands = []model.Side{model.SideRight, model.SideTop, model.SideBottom, model.SideLeft}
	}
	if len(cands) == 0 {
		return model.SideRight
	}
	score := map[model.Side]int{}
	for _, o := range l.res.Edges {
		var other *model.Element
		switch {
		case o.From == o.To:
			continue
		case o.From == b:
			other = o.To.El
		case o.To == b:
			other = o.From.El
		default:
			continue
		}
		if s := direction(e, other); s != model.SideAuto {
			score[s] += 2
		}
	}
	if parent.Kind == model.KindGrid {
		for _, k := range parent.Children {
			if k == e {
				continue
			}
			switch {
			case k.Row == e.Row && k.Col == e.Col+spanOf(e.ColSpan):
				score[model.SideRight]++
			case k.Row == e.Row && k.Col+spanOf(k.ColSpan) == e.Col:
				score[model.SideLeft]++
			case k.Col == e.Col && k.Row == e.Row+spanOf(e.RowSpan):
				score[model.SideBottom]++
			case k.Col == e.Col && k.Row+spanOf(k.RowSpan) == e.Row:
				score[model.SideTop]++
			}
		}
	}
	best := cands[0]
	for _, s := range cands[1:] {
		if score[s] < score[best] {
			best = s
		}
	}
	return best
}

// direction is the side of element a that element b lies towards, judged
// from the tree alone (layout hasn't placed anything yet): their order in
// the flow, grid cells or canvas positions of the innermost container
// holding both. It's SideAuto when that can't be told.
func direction(a, b *model.Element) model.Side {
	path := func(e *model.Element) []*model.Element {
		var out []*model.Element
		for x := e; x != nil; x = x.Parent {
			out = append(out, x)
		}
		return out
	}
	pa, pb := path(a), path(b)
	// Walk down from the root while the paths agree.
	i, j := len(pa)-1, len(pb)-1
	for i > 0 && j > 0 && pa[i-1] == pb[j-1] {
		i--
		j--
	}
	if pa[i] != pb[j] || i == 0 || j == 0 {
		return model.SideAuto // one holds the other
	}
	lca, ca, cb := pa[i], pa[i-1], pb[j-1]
	index := func(c *model.Element) int {
		for k, x := range lca.Children {
			if x == c {
				return k
			}
		}
		return -1
	}
	horiz := func(d float64) model.Side {
		if d < 0 {
			return model.SideLeft
		}
		return model.SideRight
	}
	vert := func(d float64) model.Side {
		if d < 0 {
			return model.SideTop
		}
		return model.SideBottom
	}
	switch lca.Kind {
	case model.KindRow:
		return horiz(float64(index(cb) - index(ca)))
	case model.KindColumn:
		return vert(float64(index(cb) - index(ca)))
	case model.KindGrid:
		dr, dc := cb.Row-ca.Row, cb.Col-ca.Col
		if abs(dr) >= abs(dc) && dr != 0 {
			return vert(float64(dr))
		}
		if dc != 0 {
			return horiz(float64(dc))
		}
	case model.KindCanvas:
		dx, dy := cb.X-ca.X, cb.Y-ca.Y
		if math.Abs(dy) > math.Abs(dx) {
			return vert(dy)
		}
		if dx != 0 {
			return horiz(dx)
		}
	}
	return model.SideAuto
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// loopPin is the side a self-loop is pinned to, or SideAuto.
func loopPin(e *model.Edge) model.Side {
	if e.Exit != model.SideAuto {
		return e.Exit
	}
	return e.Enter
}

// intrinsic reports whether a node's shape keeps its own proportions under
// Uniform: circles, and Person and IconOnly figures.
func intrinsic(e *model.Element) bool {
	return e.Shape == model.ShapeCircle || e.Shape == model.ShapePerson || e.IconPlace == model.IconOnly && e.Icon != nil
}

// extent sets a node's natural size (its extent) from its slot and loop
// band.
func (b *Box) extent() {
	w, h := b.slotW, b.slotH
	if b.LoopSide != model.SideAuto {
		if b.LoopSide == model.SideLeft || b.LoopSide == model.SideRight {
			w += b.loopDepth
			h = math.Max(h, b.loopAlong)
		} else {
			h += b.loopDepth
			w = math.Max(w, b.loopAlong)
		}
	}
	b.natW, b.natH = w, h
}

// bodyOffset is where a node's body sits inside its extent.
func (b *Box) bodyOffset() (dx, dy float64) {
	slotX, slotY := (b.natW-b.slotW)/2, (b.natH-b.slotH)/2
	switch b.LoopSide {
	case model.SideRight:
		slotX = 0
	case model.SideLeft:
		slotX = b.natW - b.slotW
	case model.SideBottom:
		slotY = 0
	case model.SideTop:
		slotY = b.natH - b.slotH
	}
	return slotX + (b.slotW-b.bodyW)/2, slotY + (b.slotH-b.bodyH)/2
}

// box creates the boxes for e and its descendants: its children in the
// model, less empty legends, then any generated legend entries.
func (l *layouter) box(e *model.Element) *Box {
	b := &Box{El: e, Sample: l.samples[e]}
	l.res.ByEl[e] = b
	if e != e.Diagram.Root {
		l.res.Boxes = append(l.res.Boxes, b)
	}
	for _, c := range e.Children {
		if !l.emptyLegend(c) {
			b.kids = append(b.kids, l.box(c))
		}
	}
	for _, c := range l.legendKids[e] {
		b.kids = append(b.kids, l.box(c))
	}
	return b
}

func (l *layouter) children(b *Box) []*Box { return b.kids }

// Kids returns a container's children in layout order. It differs from
// the model's children where layout generated legend entries or left out
// an empty legend.
func (b *Box) Kids() []*Box { return b.kids }

// ---- measure ----

func (l *layouter) measure(b *Box) {
	if b.El.Kind == model.KindNode {
		l.measureNode(b)
		return
	}
	kids := l.children(b)
	for _, k := range kids {
		l.measure(k)
	}
	if b.El.Uniform {
		// Every node gets the same slot; boxes grow to fill it, while
		// shapes with intrinsic proportions keep their size and sit in
		// its centre. Loop bands stay outside the slot.
		var w, h float64
		for _, k := range kids {
			if k.El.Kind == model.KindNode && !k.El.Width.Set {
				w, h = math.Max(w, k.slotW), math.Max(h, k.slotH)
			}
		}
		for _, k := range kids {
			if k.El.Kind != model.KindNode || k.El.Width.Set {
				continue
			}
			k.slotW, k.slotH = w, h
			switch {
			case k.El.Shape == model.ShapeCircle && !(k.El.IconPlace == model.IconOnly && k.El.Icon != nil):
				d := math.Min(w, h)
				k.bodyW, k.bodyH = d, d
			case !intrinsic(k.El):
				k.bodyW, k.bodyH = w, h
			}
			k.extent()
		}
	}
	l.measureGroup(b, kids)
}

// gapX and gapY are a container's gaps between children side by side and
// stacked: its own gap or the theme's, raised to the spacing floor, plus
// any extra spacing.
func (l *layouter) gapX(e *model.Element) float64 {
	return math.Max(l.gap(e), l.floor[e].X) + l.extra[e].X
}
func (l *layouter) gapY(e *model.Element) float64 {
	return math.Max(l.gap(e), l.floor[e].Y) + l.extra[e].Y
}

// gap is a container's own gap, or the default for its kind.
func (l *layouter) gap(e *model.Element) float64 {
	if e.Legend {
		return e.Gap.Or(legendGap)
	}
	return e.Gap.Or(l.t.Gap)
}

// flowGap is a Row's or Column's gap along its main axis.
func (l *layouter) flowGap(e *model.Element) float64 {
	if e.Kind == model.KindColumn {
		return l.gapY(e)
	}
	return l.gapX(e)
}

func (l *layouter) padding(b *Box) float64 {
	if b.El == b.El.Diagram.Root {
		return b.El.Padding.Or(0)
	}
	if !b.El.Visible() {
		return b.El.Padding.Or(0)
	}
	if b.El.Legend {
		return b.El.Padding.Or(legendPadding)
	}
	return b.El.Padding.Or(l.t.GroupPadding)
}

const emptyGroupW, emptyGroupH = 120, 48

func (l *layouter) measureGroup(b *Box, kids []*Box) {
	e := b.El
	b.Shown = e != e.Diagram.Root && e.Visible()
	if b.Shown {
		b.hdr = l.measureHeader(e)
	}
	cw, ch := l.contentSize(b, kids)
	if len(kids) == 0 && b.Shown {
		cw, ch = emptyGroupW, emptyGroupH-b.hdr.h
		if ch < 0 {
			ch = 0
		}
	}
	p := l.padding(b)
	hh := b.hdr.h
	if hh > 0 && len(kids) > 0 {
		hh += l.t.HeaderGap
	}
	b.natW = math.Max(cw, b.hdr.w) + 2*p
	b.natH = ch + hh + 2*p
}

func (l *layouter) contentSize(b *Box, kids []*Box) (w, h float64) {
	e := b.El
	g := l.flowGap(e)
	switch e.Kind {
	case model.KindRow, model.KindColumn:
		col := e.Kind == model.KindColumn
		for i, k := range kids {
			main, cross := k.natW, k.natH
			if col {
				main, cross = k.natH, k.natW
			}
			if i > 0 {
				w += g
			}
			w += main
			if k.El.Kind != model.KindNode {
				h = math.Max(h, cross)
			}
		}
		before, after := crossReach(kids, col, alignFrac(e.Align))
		h = math.Max(h, before+after)
		if col {
			w, h = h, w
		}
	case model.KindGrid:
		cols, rows := l.gridTracks(e, kids)
		w, h = sum(cols)+l.gapX(e)*float64(max(len(cols)-1, 0)), sum(rows)+l.gapY(e)*float64(max(len(rows)-1, 0))
	case model.KindCanvas:
		minX, minY, maxX, maxY := canvasBounds(kids)
		w, h = maxX-minX, maxY-minY
	}
	return w, h
}

// gridTracks returns column widths and row heights. Spanning children that
// need more room than their tracks provide grow those tracks evenly.
func (l *layouter) gridTracks(e *model.Element, kids []*Box) (cols, rows []float64) {
	nrows := 0
	for _, k := range kids {
		if inGrid(e, k.El) {
			nrows = max(nrows, k.El.Row+spanOf(k.El.RowSpan))
		}
	}
	cols, rows = make([]float64, e.Cols), make([]float64, nrows)
	gx, gy := l.gapX(e), l.gapY(e)
	grow := func(tracks []float64, g float64, start, n int, need float64) {
		have := sum(tracks[start:start+n]) + g*float64(n-1)
		if need > have {
			extra := (need - have) / float64(n)
			for i := start; i < start+n; i++ {
				tracks[i] += extra
			}
		}
	}
	// Nodes alone in a track line their bodies up, so the track needs the
	// most any of them reaches before and after that line.
	ta := l.gridAnchors(e, kids, len(cols), len(rows))
	for i := range cols {
		grow(cols, gx, i, 1, ta.colBefore[i]+ta.colAfter[i])
	}
	for i := range rows {
		grow(rows, gy, i, 1, ta.rowBefore[i]+ta.rowAfter[i])
	}
	// Single-span children first, so spans only add what's missing.
	for pass := 0; pass < 2; pass++ {
		for _, k := range kids {
			cs := spanOf(k.El.ColSpan)
			if (pass == 0) != (cs == 1) || !inGrid(e, k.El) || cs == 1 && k.El.Kind == model.KindNode {
				continue
			}
			grow(cols, gx, k.El.Col, cs, k.natW)
		}
		for _, k := range kids {
			rs := spanOf(k.El.RowSpan)
			if (pass == 0) != (rs == 1) || !inGrid(e, k.El) || rs == 1 && k.El.Kind == model.KindNode {
				continue
			}
			grow(rows, gy, k.El.Row, rs, k.natH)
		}
	}
	return cols, rows
}

// trackAnchors holds, per grid column and row, how far the single-span
// nodes in it reach before and after the line their bodies share.
type trackAnchors struct {
	colBefore, colAfter, rowBefore, rowAfter []float64
}

func (l *layouter) gridAnchors(e *model.Element, kids []*Box, ncols, nrows int) trackAnchors {
	ta := trackAnchors{make([]float64, ncols), make([]float64, ncols), make([]float64, nrows), make([]float64, nrows)}
	fx := alignFrac(e.Align)
	for _, k := range kids {
		if k.El.Kind != model.KindNode || !inGrid(e, k.El) {
			continue
		}
		if spanOf(k.El.ColSpan) == 1 {
			b, a := k.reach(true, fx)
			c := k.El.Col
			ta.colBefore[c], ta.colAfter[c] = math.Max(ta.colBefore[c], b), math.Max(ta.colAfter[c], a)
		}
		if spanOf(k.El.RowSpan) == 1 {
			b, a := k.reach(false, 0.5)
			r := k.El.Row
			ta.rowBefore[r], ta.rowAfter[r] = math.Max(ta.rowBefore[r], b), math.Max(ta.rowAfter[r], a)
		}
	}
	return ta
}

// reach returns how far a node's extent reaches before and after an anchor
// point on its body, along the horizontal or vertical axis. frac picks the
// anchor: 0 for the body's start, 0.5 for its centre, 1 for its end. A
// loop band adds reach on its own side only.
func (b *Box) reach(horiz bool, frac float64) (before, after float64) {
	dx, dy := b.bodyOffset()
	ext, off, body := b.natH, dy, b.bodyH
	if horiz {
		ext, off, body = b.natW, dx, b.bodyW
	}
	p := off + frac*body
	return p, ext - p
}

// crossReach is the most any node in kids reaches before and after the
// shared line its body is aligned on, across the flow's main axis.
func crossReach(kids []*Box, col bool, frac float64) (before, after float64) {
	for _, k := range kids {
		if k.El.Kind == model.KindNode {
			b, a := k.reach(col, frac)
			before, after = math.Max(before, b), math.Max(after, a)
		}
	}
	return before, after
}

func alignFrac(a model.Align) float64 {
	switch a {
	case model.AlignStart:
		return 0
	case model.AlignEnd:
		return 1
	}
	return 0.5
}

// anchorLine places the line that bodies align on within [start,
// start+size): at frac of the way along, moved in just enough to leave
// before and after free on either side.
func anchorLine(start, size, before, after, frac float64) float64 {
	return math.Max(start+before, math.Min(start+frac*size, start+size-after))
}

// inGrid reports whether a grid child's cell lies inside the grid. The
// builder reports children that don't as errors; layout still runs so it
// can report its own errors with them, and leaves such children at the
// grid's origin.
func inGrid(g, k *model.Element) bool {
	return k.Row >= 0 && k.Col >= 0 && k.Col+spanOf(k.ColSpan) <= g.Cols
}

func canvasBounds(kids []*Box) (minX, minY, maxX, maxY float64) {
	for _, k := range kids {
		x, y := canvasOrigin(k)
		minX, minY = math.Min(minX, x), math.Min(minY, y)
		maxX, maxY = math.Max(maxX, x+k.natW), math.Max(maxY, y+k.natH)
	}
	return
}

// canvasOrigin is where a canvas child's extent starts in canvas
// coordinates. A node's coordinates place its body, so a loop band on its
// left or top reaches past them.
func canvasOrigin(k *Box) (x, y float64) {
	x, y = k.El.X, k.El.Y
	if k.El.Kind == model.KindNode {
		dx, dy := k.bodyOffset()
		x, y = x-dx, y-dy
	}
	return x, y
}

func spanOf(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

func sum(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x
	}
	return s
}

// ---- place ----

// place positions b inside the area its parent allocated.
func (l *layouter) place(b *Box, area Rect) {
	e := b.El
	if e.Kind == model.KindNode {
		w, h := b.natW, b.natH
		b.Rect = Rect{alignIn(area.X, area.W, w, l.alignOf(e.Parent)), area.Y + (area.H-h)/2, w, h}
		dx, dy := b.bodyOffset()
		l.layoutNode(b, Rect{b.Rect.X + dx, b.Rect.Y + dy, b.bodyW, b.bodyH})
		return
	}
	if e.Legend {
		// A legend keeps its natural size, aligned like a node.
		a := l.alignOf(e.Parent)
		area = Rect{alignIn(area.X, area.W, b.natW, a), alignIn(area.Y, area.H, b.natH, a), b.natW, b.natH}
	}
	b.Rect = area
	b.Shape = area
	p := l.padding(b)
	top := area.Y + p
	if b.Shown {
		b.Look = l.groupLook(e)
		l.layoutHeader(b, Rect{area.X + p, top, area.W - 2*p, b.hdr.h})
		if b.hdr.h > 0 && len(b.kids) > 0 {
			top += b.hdr.h + l.t.HeaderGap
		}
	}
	b.Content = Rect{area.X + p, top, area.W - 2*p, area.Bottom() - p - top}
	kids := l.children(b)
	switch e.Kind {
	case model.KindRow, model.KindColumn:
		l.placeFlow(b, kids)
	case model.KindGrid:
		l.placeGrid(b, kids)
	case model.KindCanvas:
		minX, minY, _, _ := canvasBounds(kids)
		for _, k := range kids {
			x, y := canvasOrigin(k)
			l.place(k, Rect{b.Content.X + x - minX, b.Content.Y + y - minY, k.natW, k.natH})
		}
		if !e.AllowOverlap {
			l.checkOverlap(b, kids)
		}
	}
}

// checkOverlap records an error for each pair of a canvas's children whose
// extents overlap, naming the later one first.
func (l *layouter) checkOverlap(canvas *Box, kids []*Box) {
	for j, b := range kids {
		for _, a := range kids[:j] {
			if b.Rect.Overlaps(a.Rect) {
				l.errs = append(l.errs, fmt.Errorf("%s: %s overlaps %s (%s) in %s",
					b.El.Src, b.El.Describe(), a.El.Describe(), a.El.Src, canvas.El.Describe()))
			}
		}
	}
}

func (l *layouter) alignOf(parent *model.Element) model.Align {
	if parent == nil {
		return model.AlignCenter
	}
	return parent.Align
}

func alignIn(start, avail, size float64, a model.Align) float64 {
	switch a {
	case model.AlignStart:
		return start
	case model.AlignEnd:
		return start + avail - size
	}
	return start + (avail-size)/2
}

func (l *layouter) placeFlow(b *Box, kids []*Box) {
	e := b.El
	row := e.Kind == model.KindRow
	g := l.flowGap(e)
	c := b.Content
	var total float64
	for i, k := range kids {
		if i > 0 {
			total += g
		}
		if row {
			total += k.natW
		} else {
			total += k.natH
		}
	}
	avail := c.H
	if row {
		avail = c.W
	}
	lead, g := justify(e.Justify, math.Max(avail-total, 0), g, len(kids))
	// Nodes line up by their bodies, not their extents, so a loop band
	// doesn't push a node out of line with its siblings.
	frac := alignFrac(e.Align)
	before, after := crossReach(kids, !row, frac)
	if row {
		line := anchorLine(c.Y, c.H, before, after, frac)
		pos := c.X + lead
		for _, k := range kids {
			area := Rect{pos, c.Y, k.natW, c.H}
			if k.El.Kind == model.KindNode {
				b, _ := k.reach(false, frac)
				area = Rect{pos, line - b, k.natW, k.natH}
			}
			l.place(k, area)
			pos += k.natW + g
		}
		return
	}
	line := anchorLine(c.X, c.W, before, after, frac)
	pos := c.Y + lead
	for _, k := range kids {
		area := Rect{c.X, pos, c.W, k.natH}
		if k.El.Kind == model.KindNode {
			b, _ := k.reach(true, frac)
			area = Rect{line - b, pos, k.natW, k.natH}
		}
		l.place(k, area)
		pos += k.natH + g
	}
}

// justify distributes free main-axis space: it returns the space before
// the first child and the gap between children.
func justify(j model.Justify, free, gap float64, n int) (lead, g float64) {
	switch j {
	case model.JustifyCenter:
		return free / 2, gap
	case model.JustifyEnd:
		return free, gap
	case model.JustifySpaceBetween:
		if n > 1 {
			return 0, gap + free/float64(n-1)
		}
	}
	return 0, gap
}

func (l *layouter) placeGrid(b *Box, kids []*Box) {
	e := b.El
	gx, gy := l.gapX(e), l.gapY(e)
	cols, rows := l.gridTracks(e, kids)
	c := b.Content
	// Spread any extra width or height evenly across tracks.
	if n := len(cols); n > 0 {
		if extra := c.W - (sum(cols) + gx*float64(n-1)); extra > 0 {
			for i := range cols {
				cols[i] += extra / float64(n)
			}
		}
	}
	if n := len(rows); n > 0 {
		if extra := c.H - (sum(rows) + gy*float64(n-1)); extra > 0 {
			for i := range rows {
				rows[i] += extra / float64(n)
			}
		}
	}
	colX := offsets(c.X, cols, gx)
	rowY := offsets(c.Y, rows, gy)
	ta := l.gridAnchors(e, kids, len(cols), len(rows))
	fx := alignFrac(e.Align)
	for _, k := range kids {
		if !inGrid(e, k.El) {
			l.place(k, Rect{c.X, c.Y, k.natW, k.natH})
			continue
		}
		rs, cs := spanOf(k.El.RowSpan), spanOf(k.El.ColSpan)
		x, y := colX[k.El.Col], rowY[k.El.Row]
		w := colX[k.El.Col+cs-1] + cols[k.El.Col+cs-1] - x
		h := rowY[k.El.Row+rs-1] + rows[k.El.Row+rs-1] - y
		area := Rect{x, y, w, h}
		if k.El.Kind == model.KindNode {
			// Nodes alone in their track line up with the others in it; a
			// spanning node is placed on its own.
			ox, oxa := k.reach(true, fx)
			oy, oya := k.reach(false, 0.5)
			bx, ax, by, ay := ox, oxa, oy, oya
			if cs == 1 {
				bx, ax = ta.colBefore[k.El.Col], ta.colAfter[k.El.Col]
			}
			if rs == 1 {
				by, ay = ta.rowBefore[k.El.Row], ta.rowAfter[k.El.Row]
			}
			area = Rect{anchorLine(x, w, bx, ax, fx) - ox, anchorLine(y, h, by, ay, 0.5) - oy, k.natW, k.natH}
		}
		l.place(k, area)
	}
}

func offsets(start float64, tracks []float64, gap float64) []float64 {
	out := make([]float64, len(tracks))
	pos := start
	for i, t := range tracks {
		out[i] = pos
		pos += t + gap
	}
	return out
}
