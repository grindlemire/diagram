package layout

import (
	"fmt"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/themes"
)

// SampleKind is what a legend entry's sample shows.
type SampleKind uint8

const (
	SampleEdge  SampleKind = iota + 1 // a short line with an arrowhead
	SampleNode                        // a small box in a node's look
	SampleGroup                       // a small box in a group's look
)

// Sample is the drawing in a legend entry: a named style shown as an edge
// or as a box. A box sample's look is its Box's Look.
type Sample struct {
	Kind SampleKind
	Name string

	// Edge samples.
	Color themes.Color
	Width float64
	Dash  model.Dash
}

// Legend entry metrics.
const (
	legendGap      = 6  // between entries
	legendPadding  = 14 // inside the legend's border
	legendEntryGap = 8  // between a sample and its name
	sampleW        = 32 // every sample, so the names line up
	sampleEdgeH    = 14
	sampleBoxH     = 16
	sampleMaxR     = 4 // largest corner radius on a box sample
	sampleMaxW     = 3 // widest border on a box sample
)

// legendFontSize is the font size of a legend entry's name.
func legendFontSize(t *themes.Theme) float64 { return t.DescFontSize }

// legends builds the entries of every legend in d: for each named style
// used anywhere, in the order styles were first applied, a row holding a
// sample and the style's name. The rows are generated elements, kept apart
// from the model's tree; it returns them by legend.
func (l *layouter) legends(d *model.Diagram) map[*model.Element][]*model.Element {
	var legends []*model.Element
	model.Walk(d.Root, func(e *model.Element) {
		if e.Legend {
			legends = append(legends, e)
		}
	})
	if len(legends) == 0 {
		return nil
	}
	kinds := usedStyles(d)
	ids := map[string]bool{}
	model.Walk(d.Root, func(e *model.Element) { ids[e.ID] = true })
	for _, e := range d.Edges {
		ids[e.ID] = true
	}
	out := map[*model.Element][]*model.Element{}
	for _, lg := range legends {
		for i, ns := range d.Named {
			kind := kinds[ns.Name]
			if kind == 0 {
				continue
			}
			seg := model.Slug(ns.Name)
			if seg == "" {
				seg = fmt.Sprintf("%d", i+1)
			}
			id := lg.ID + "/style-" + seg
			for n := 2; ids[id]; n++ {
				id = fmt.Sprintf("%s/style-%s-%d", lg.ID, seg, n)
			}
			ids[id] = true
			out[lg] = append(out[lg], l.entry(lg, ns, kind, id))
		}
	}
	return out
}

// usedStyles returns, for each named style used on the diagram's elements
// and edges, the kind of sample that shows it best: an edge if it is used
// on an edge, else a node if used on a node, else a group.
func usedStyles(d *model.Diagram) map[string]SampleKind {
	kinds := map[string]SampleKind{}
	mark := func(names []string, k SampleKind) {
		for _, n := range names {
			if cur := kinds[n]; cur == 0 || k < cur {
				kinds[n] = k
			}
		}
	}
	model.Walk(d.Root, func(e *model.Element) {
		switch {
		case e == d.Root:
		case e.Kind == model.KindNode:
			mark(e.Style.Names, SampleNode)
		default:
			mark(e.Style.Names, SampleGroup)
		}
	})
	for _, e := range d.Edges {
		mark(e.Style.Names, SampleEdge)
	}
	return kinds
}

// entry makes one legend entry: a row with a sample and a text node.
func (l *layouter) entry(lg *model.Element, ns *model.NamedStyle, kind SampleKind, id string) *model.Element {
	t := l.t
	depth := lg.Depth + 1
	row := &model.Element{
		Kind: model.KindRow, Parent: lg, Diagram: lg.Diagram, Src: ns.Src, Depth: depth, ID: id,
		Gap: model.Some(float64(legendEntryGap)), Generated: true,
	}
	st := ns.Style
	st.Names = []string{ns.Name}
	sample := &model.Element{
		Kind: model.KindNode, Parent: row, Diagram: lg.Diagram, Src: ns.Src, Depth: depth, ID: id + "#sample",
		Style: st, Generated: true,
	}
	sm := &Sample{Kind: kind, Name: ns.Name}
	if kind == SampleEdge {
		s := ns.Style.Over(model.Style{Stroke: t.EdgeColor, StrokeWidth: model.Some(t.EdgeWidth)})
		sm.Color, sm.Width, sm.Dash = s.Stroke, s.StrokeWidth.V, s.Dash
		sample.Width, sample.Height = model.Some(float64(sampleW)), model.Some(float64(sampleEdgeH))
		sample.Shape = model.ShapeText
	} else {
		sample.Width, sample.Height = model.Some(float64(sampleW)), model.Some(float64(sampleBoxH))
	}
	l.samples[sample] = sm
	name := &model.Element{
		Kind: model.KindNode, Parent: row, Diagram: lg.Diagram, Src: ns.Src, Depth: depth, ID: id + "#name",
		Label: ns.Name, Shape: model.ShapeText, Generated: true,
		Style: model.Style{FontSize: model.Some(legendFontSize(t)), Bold: model.Some(false), Names: []string{ns.Name}},
	}
	row.Children = []*model.Element{sample, name}
	return row
}

// sampleLook adjusts a box sample's look: a group style shows a group's
// look, and corners and borders are scaled down to the sample's size.
func (l *layouter) sampleLook(b *Box) {
	if b.Sample == nil {
		return
	}
	if b.Sample.Kind == SampleGroup {
		e := *b.El
		e.Depth = 0
		b.Look = l.groupLook(&e)
	}
	b.Look.Radius = min(b.Look.Radius, sampleMaxR)
	b.Look.StrokeWidth = min(b.Look.StrokeWidth, sampleMaxW)
}

// emptyLegend reports whether e is a legend with nothing to show, which
// layout leaves out entirely.
func (l *layouter) emptyLegend(e *model.Element) bool {
	return e.Legend && len(e.Children) == 0 && len(l.legendKids[e]) == 0
}
