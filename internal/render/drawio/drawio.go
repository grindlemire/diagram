// Package drawio writes laid-out diagrams as editable draw.io files.
//
// The output is built from draw.io's own concepts so it stays editable:
// visible groups become containers that hold their children, nodes become
// vertices with native shapes, icons become image cells inside their node,
// and edges are connected to their endpoint cells with fixed ports and our
// waypoints. The XML is uncompressed so it diffs well.
package drawio

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/num"
	"github.com/grindlemire/diagram/internal/text"
	"github.com/grindlemire/diagram/themes"
)

// Page is one diagram, written as one draw.io page.
type Page struct {
	Name   string         // page title
	Result *layout.Result // laid out and routed
	Margin float64        // empty space around the diagram
}

// Render writes pages as one uncompressed .drawio file.
func Render(w io.Writer, pages []Page) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "<mxfile host=\"diagram\" pages=\"%d\">\n", len(pages))
	for i, p := range pages {
		if p.Result == nil || p.Result.Theme == nil {
			return fmt.Errorf("drawio: page %d: no layout", i+1)
		}
		pw := &pageWriter{
			w: bw, res: p.Result, t: p.Result.Theme, m: p.Margin,
			ids:  map[*layout.Box]string{},
			used: map[string]bool{"0": true, "1": true},
		}
		pw.page(i, p.Name)
	}
	bw.WriteString("</mxfile>\n")
	return bw.Flush()
}

// fontSource loads Inter's regular and semibold weights from Google Fonts.
// draw.io expects the URL encoded as by encodeURIComponent.
const fontSource = "https%3A%2F%2Ffonts.googleapis.com%2Fcss%3Ffamily%3DInter%3A400%2C600"

type pageWriter struct {
	w    *bufio.Writer
	res  *layout.Result
	t    *themes.Theme
	m    float64
	ids  map[*layout.Box]string // emitted cells; lookup only, never ranged over
	used map[string]bool
}

func (pw *pageWriter) page(i int, name string) {
	if name == "" {
		name = fmt.Sprintf("Page-%d", i+1)
	}
	res := pw.res
	bg, _ := pw.color(themes.Of(themes.Background))
	fmt.Fprintf(pw.w, "  <diagram id=\"page-%d\" name=\"%s\">\n", i+1, esc(name))
	fmt.Fprintf(pw.w, "    <mxGraphModel grid=\"1\" gridSize=\"10\" guides=\"1\" tooltips=\"1\" connect=\"1\" arrows=\"1\" fold=\"1\" page=\"1\" pageScale=\"1\" pageWidth=\"%s\" pageHeight=\"%s\" background=\"%s\" math=\"0\" shadow=\"0\">\n",
		num.Fmt(math.Ceil(res.Width+2*pw.m)), num.Fmt(math.Ceil(res.Height+2*pw.m)), bg)
	pw.w.WriteString("      <root>\n        <mxCell id=\"0\"/>\n        <mxCell id=\"1\" parent=\"0\"/>\n")
	for _, b := range res.Boxes {
		switch {
		case b.Sample != nil && b.Sample.Kind == layout.SampleEdge:
			pw.sampleEdge(b)
		case b.El.Kind == model.KindNode:
			pw.node(b)
		case b.Shown:
			pw.group(b)
		}
	}
	for _, e := range res.Edges {
		pw.edge(e)
	}
	pw.w.WriteString("      </root>\n    </mxGraphModel>\n  </diagram>\n")
}

// id returns a page-unique cell ID derived from want.
func (pw *pageWriter) id(want string) string {
	if want == "" {
		want = "cell"
	}
	id := want
	for n := 2; pw.used[id]; n++ {
		id = fmt.Sprintf("%s~%d", want, n)
	}
	pw.used[id] = true
	return id
}

// parent returns the cell a box is drawn inside and that cell's origin.
// Invisible containers are not emitted, so this is the nearest visible
// group, or the layer.
func (pw *pageWriter) parent(b *layout.Box) (string, model.Point) {
	for p := b.El.Parent; p != nil; p = p.Parent {
		pb := pw.res.ByEl[p]
		if pb == nil {
			break
		}
		if id, ok := pw.ids[pb]; ok {
			return id, model.Point{X: pb.Shape.X, Y: pb.Shape.Y}
		}
	}
	return "1", model.Point{X: -pw.m, Y: -pw.m}
}

// ---- cells ----

func (pw *pageWriter) vertex(id, value string, st *style, parent string, r layout.Rect, origin model.Point, el *model.Element) {
	geom := fmt.Sprintf("          <mxGeometry x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" as=\"geometry\"/>\n",
		num.Fmt(r.X-origin.X), num.Fmt(r.Y-origin.Y), num.Fmt(r.W), num.Fmt(r.H))
	tooltip, link := "", ""
	if el != nil {
		tooltip, link = el.Tooltip, el.Link
	}
	pw.cell(id, value, tooltip, link, fmt.Sprintf(` style="%s" vertex="1" parent="%s"`, esc(st.String()), esc(parent)), geom)
}

// cell writes an mxCell with the given attributes after its id and value,
// and inner, its content lines. A cell with a tooltip or link is wrapped
// in a UserObject, as draw.io writes cells with properties: the
// UserObject carries the id, the value as its label, and the tooltip and
// link.
func (pw *pageWriter) cell(id, value, tooltip, link, attrs, inner string) {
	if tooltip == "" && link == "" {
		fmt.Fprintf(pw.w, "        <mxCell id=\"%s\" value=\"%s\"%s>\n%s        </mxCell>\n", esc(id), esc(value), attrs, inner)
		return
	}
	fmt.Fprintf(pw.w, "        <UserObject label=\"%s\"", esc(value))
	if tooltip != "" {
		fmt.Fprintf(pw.w, " tooltip=\"%s\"", esc(tooltip))
	}
	if link != "" {
		fmt.Fprintf(pw.w, " link=\"%s\"", esc(link))
	}
	fmt.Fprintf(pw.w, " id=\"%s\">\n          <mxCell%s>\n", esc(id), attrs)
	for ln := range strings.Lines(inner) {
		pw.w.WriteString("  " + ln)
	}
	pw.w.WriteString("          </mxCell>\n        </UserObject>\n")
}

func (pw *pageWriter) group(b *layout.Box) {
	id := pw.id(b.El.ID)
	parent, origin := pw.parent(b)
	pw.ids[b] = id
	r := b.Shape
	st := &style{}
	rounded(st, b.Look.Radius, r)
	pw.outline(st, b.Look)
	st.add("container", "1")
	st.add("collapsible", "0")
	st.add("recursiveResize", "0")
	st.add("html", "1")
	st.add("whiteSpace", "wrap")
	st.add("spacing", "0")
	value := ""
	if len(b.Lines) > 0 {
		value = pw.richText(st, b.Lines)
		tb := pw.textBounds(b.Lines)
		st.add("verticalAlign", "top")
		st.add("spacingTop", num.Fmt(tb.Y-r.Y))
		if b.El.HeaderTop {
			// Centre the text where the layout put it, which is off the
			// group's centre when there's an icon beside it.
			d := tb.X + tb.W/2 - (r.X + r.W/2)
			st.add("align", "center")
			st.add("spacingLeft", num.Fmt(math.Max(0, 2*d)))
			st.add("spacingRight", num.Fmt(math.Max(0, -2*d)))
		} else {
			st.add("align", "left")
			st.add("spacingLeft", num.Fmt(tb.X-r.X))
		}
	}
	pw.vertex(id, value, st, parent, r, origin, b.El)
	pw.icon(b, id, model.Point{X: r.X, Y: r.Y})
}

func (pw *pageWriter) node(b *layout.Box) {
	e := b.El
	id := pw.id(e.ID)
	parent, origin := pw.parent(b)
	pw.ids[b] = id
	r := b.Shape
	st := &style{}
	iconOnly := e.IconPlace == model.IconOnly && e.Icon != nil && e.Shape != model.ShapePerson
	free := iconOnly || e.Shape == model.ShapePerson

	switch {
	case iconOnly:
		pw.imageStyle(st, e.Icon, b.Look.IconColor)
	case e.Shape == model.ShapeText:
		st.add("text", "")
		pw.outline(st, b.Look)
	default:
		pw.shapeStyle(st, b)
		pw.outline(st, b.Look)
	}
	st.add("html", "1")
	value := ""
	if len(b.Lines) > 0 {
		value = pw.richText(st, b.Lines)
		tb := pw.textBounds(b.Lines)
		st.add("spacing", "0")
		if free {
			// The label hangs below the figure, unwrapped, with our breaks.
			st.add("whiteSpace", "nowrap")
			st.add("verticalLabelPosition", "bottom")
			st.add("verticalAlign", "top")
			st.add("spacingTop", num.Fmt(tb.Y-r.Bottom()))
		} else {
			pw.textBox(st, r, tb, b.Lines[0].Anchor)
		}
	} else if free {
		st.add("verticalLabelPosition", "bottom")
		st.add("verticalAlign", "top")
	}
	pw.vertex(id, value, st, parent, r, origin, e)
	if !iconOnly {
		pw.icon(b, id, model.Point{X: r.X, Y: r.Y})
	}
	pw.marks(b, id, model.Point{X: r.X, Y: r.Y})
}

// marks emits a node's status dot and badges as small cells inside the
// node's cell, so they move with it and can be edited or deleted on
// their own.
func (pw *pageWriter) marks(b *layout.Box, parent string, origin model.Point) {
	ring := func(st *style) {
		pw.colorKey(st, "strokeColor", "strokeOpacity", b.Ring)
		st.add("strokeWidth", num.Fmt(layout.MarkRing))
		st.add("connectable", "0")
	}
	if d := b.Status; d != nil {
		st := &style{}
		st.add("ellipse", "")
		pw.colorKey(st, "fillColor", "fillOpacity", d.Color)
		ring(st)
		pw.vertex(pw.id(b.El.ID+"#status"), "", st, parent, d.Rect(), origin, nil)
	}
	for i, bg := range b.Badges {
		st := &style{}
		st.add("rounded", "1")
		st.add("arcSize", "50")
		pw.colorKey(st, "fillColor", "fillOpacity", bg.Fill)
		ring(st)
		st.add("html", "1")
		st.add("whiteSpace", "nowrap")
		st.add("spacing", "0")
		value := pw.richText(st, []layout.Line{bg.Line})
		st.add("align", "center")
		st.add("verticalAlign", "middle")
		pw.vertex(pw.id(fmt.Sprintf("%s#badge-%d", b.El.ID, i+1)), value, st, parent, bg.Rect, origin, nil)
	}
}

// sampleEdge emits a legend's edge sample as a free-standing edge inside
// the legend's container.
func (pw *pageWriter) sampleEdge(b *layout.Box) {
	s, t := b.Sample, pw.t
	parent, origin := pw.parent(b)
	st := &style{}
	st.add("edgeStyle", "none")
	st.add("rounded", "0")
	st.add("html", "1")
	pw.colorKey(st, "strokeColor", "strokeOpacity", s.Color)
	st.add("strokeWidth", num.Fmt(s.Width))
	dash(st, s.Dash)
	size := t.ArrowSize * math.Max(1, s.Width/1.5)
	arrow(st, "end", model.ArrowTriangle, size)
	arrow(st, "start", model.ArrowNone, size)
	r := b.Shape
	y := r.Y + r.H/2 - origin.Y
	var geom strings.Builder
	geom.WriteString("          <mxGeometry relative=\"1\" as=\"geometry\">\n")
	fmt.Fprintf(&geom, "            <mxPoint x=\"%s\" y=\"%s\" as=\"sourcePoint\"/>\n", num.Fmt(r.X-origin.X), num.Fmt(y))
	fmt.Fprintf(&geom, "            <mxPoint x=\"%s\" y=\"%s\" as=\"targetPoint\"/>\n", num.Fmt(r.Right()-origin.X), num.Fmt(y))
	geom.WriteString("          </mxGeometry>\n")
	pw.cell(pw.id(b.El.ID), "", "", "", fmt.Sprintf(` style="%s" edge="1" parent="%s"`, esc(st.String()), esc(parent)), geom.String())
}

// textBox insets a node's label area so draw.io centres the text where
// the layout put it. Our lines are kept as explicit breaks; the area keeps
// some slack so differences in font metrics don't rewrap them.
func (pw *pageWriter) textBox(st *style, r, tb layout.Rect, a layout.Anchor) {
	const slack = 4
	st.add("whiteSpace", "wrap")
	st.add("verticalAlign", "middle")
	st.add("spacingTop", num.Fmt(tb.Y-r.Y))
	st.add("spacingBottom", num.Fmt(r.Bottom()-tb.Bottom()))
	if a == layout.AnchorStart {
		st.add("align", "left")
		st.add("spacingLeft", num.Fmt(tb.X-r.X))
		st.add("spacingRight", num.Fmt(math.Max(0, r.Right()-tb.Right()-2*slack)))
		return
	}
	cx := tb.X + tb.W/2
	half := math.Max(math.Min(cx-r.X, r.Right()-cx)-2*slack, tb.W/2+slack)
	st.add("align", "center")
	st.add("spacingLeft", num.Fmt(cx-half-r.X))
	st.add("spacingRight", num.Fmt(r.Right()-(cx+half)))
}

// shapeStyle sets the draw.io shape for a node's outline.
func (pw *pageWriter) shapeStyle(st *style, b *layout.Box) {
	r := b.Shape
	switch b.El.Shape {
	case model.ShapeBox, model.ShapeRect:
		rounded(st, b.Look.Radius, r)
	case model.ShapePill:
		st.add("rounded", "1")
		st.add("arcSize", "50")
	case model.ShapeCircle:
		st.add("ellipse", "")
		st.add("perimeter", "ellipsePerimeter")
	case model.ShapeCylinder:
		st.add("shape", "cylinder3")
		st.add("size", num.Fmt(b.Cap))
		st.add("backgroundOutline", "1")
	case model.ShapeQueue:
		// A cylinder turned on its side, lid on the right. Ports stay in
		// the unrotated bounds.
		st.add("shape", "cylinder3")
		st.add("direction", "south")
		st.add("anchorPointDirection", "0")
		st.add("size", num.Fmt(b.Cap))
		st.add("backgroundOutline", "1")
	case model.ShapeDocument:
		st.add("shape", "document")
		if r.H > 0 {
			st.add("size", num.Fmt(b.Wave/r.H))
		}
		st.add("backgroundOutline", "1")
	case model.ShapeNote:
		st.add("shape", "note")
		st.add("size", num.Fmt(math.Min(12, math.Min(r.W, r.H)/3)))
		st.add("backgroundOutline", "1")
		st.add("darkOpacity", "0")
	case model.ShapePerson:
		st.add("shape", "actor")
	}
}

// rounded sets a fixed corner radius. With absoluteArcSize, draw.io's
// radius is arcSize/2, capped at half the shorter side as ours is.
func rounded(st *style, rad float64, r layout.Rect) {
	rad = math.Min(rad, math.Min(r.W, r.H)/2)
	if rad <= 0 {
		st.add("rounded", "0")
		return
	}
	st.add("rounded", "1")
	st.add("absoluteArcSize", "1")
	st.add("arcSize", num.Fmt(2*rad))
}

// outline sets fill, stroke and dash from a look.
func (pw *pageWriter) outline(st *style, l layout.Look) {
	pw.colorKey(st, "fillColor", "fillOpacity", l.Fill)
	if l.StrokeWidth > 0 && !l.Stroke.IsZero() {
		pw.colorKey(st, "strokeColor", "strokeOpacity", l.Stroke)
		st.add("strokeWidth", num.Fmt(l.StrokeWidth))
		dash(st, l.Dash)
	} else {
		st.add("strokeColor", "none")
	}
}

func dash(st *style, d model.Dash) {
	// draw.io multiplies dash patterns by the stroke width, as we do.
	switch d {
	case model.DashDashed:
		st.add("dashed", "1")
		st.add("dashPattern", "5 3")
	case model.DashDotted:
		st.add("dashed", "1")
		st.add("dashPattern", "1 1.5")
	}
}

// icon emits a box's icon as an image cell inside the box's cell, so it
// moves with it and can be replaced on its own.
func (pw *pageWriter) icon(b *layout.Box, parent string, origin model.Point) {
	if b.El.Icon == nil || b.Icon.W == 0 {
		return
	}
	st := &style{}
	pw.imageStyle(st, b.El.Icon, b.Look.IconColor)
	st.add("connectable", "0")
	pw.vertex(pw.id(b.El.ID+"#icon"), "", st, parent, b.Icon, origin, nil)
}

func (pw *pageWriter) imageStyle(st *style, ic *icons.Icon, col themes.Color) {
	st.add("shape", "image")
	st.add("imageAspect", "0")
	st.add("aspect", "fixed")
	st.add("image", pw.dataURI(ic, col))
}

// dataURI encodes an icon in the form draw.io expects inside a style,
// which has no ";base64" because ";" separates style keys. Monochrome SVG
// icons are recoloured first.
func (pw *pageWriter) dataURI(ic *icons.Icon, col themes.Color) string {
	if ic.Kind() == icons.PNG {
		return "data:image/png," + base64.StdEncoding.EncodeToString(ic.Data())
	}
	data := ic.Data()
	if ic.Monochrome() {
		hex := themes.HexString(col.Resolve(pw.t))
		data = []byte(strings.ReplaceAll(string(data), "currentColor", hex))
	}
	return "data:image/svg+xml," + base64.StdEncoding.EncodeToString(data)
}

// ---- text ----

// richText returns lines as an HTML label and sets the style's font from
// the first line. Later lines that differ, such as a description, get a
// span. Our wrapping is kept as explicit breaks.
func (pw *pageWriter) richText(st *style, lines []layout.Line) string {
	base := lines[0]
	pw.font(st, base.Size, base.Weight, base.Color)
	baseCol := pw.hex(base.Color)
	var out strings.Builder
	for i := 0; i < len(lines); {
		j := i + 1
		for j < len(lines) && sameFont(lines[i], lines[j]) {
			j++
		}
		var css []string
		ln := lines[i]
		if ln.Size != base.Size {
			css = append(css, "font-size:"+num.Fmt(ln.Size)+"px")
		}
		if ln.Weight != base.Weight {
			css = append(css, "font-weight:"+weight(ln.Weight))
		}
		if c := pw.hex(ln.Color); c != baseCol {
			css = append(css, "color:"+c)
		}
		if i > 0 {
			out.WriteString("<br>")
		}
		if len(css) > 0 {
			fmt.Fprintf(&out, "<span style=\"%s\">", strings.Join(css, ";"))
		}
		for k := i; k < j; k++ {
			if k > i {
				out.WriteString("<br>")
			}
			out.WriteString(escHTML(lines[k].Text))
		}
		if len(css) > 0 {
			out.WriteString("</span>")
		}
		i = j
	}
	return out.String()
}

func sameFont(a, b layout.Line) bool {
	return a.Size == b.Size && a.Weight == b.Weight && a.Color == b.Color
}

func weight(w text.Weight) string {
	if w == text.SemiBold {
		return "600"
	}
	return "400"
}

func (pw *pageWriter) font(st *style, size float64, w text.Weight, col themes.Color) {
	st.add("fontFamily", pw.t.FontFamily)
	if pw.t.FontFamily == "Inter" {
		st.add("fontSource", fontSource)
	}
	st.add("fontSize", num.Fmt(size))
	st.add("fontColor", pw.hex(col))
	// draw.io's bold asks for weight 700; with only 400 and 600 loaded the
	// browser picks 600.
	if w == text.SemiBold {
		st.add("fontStyle", "1")
	} else {
		st.add("fontStyle", "0")
	}
}

// textBounds returns the box the lines occupy, using the line height the
// layout used.
func (pw *pageWriter) textBounds(lines []layout.Line) layout.Rect {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, ln := range lines {
		f := text.FaceFor(ln.Weight)
		asc, desc := f.Ascent(ln.Size), f.Descent(ln.Size)
		lh := ln.Size * pw.t.LineHeight
		top := ln.Y - asc - (lh-(asc+desc))/2
		x := ln.X
		if ln.Anchor == layout.AnchorMiddle {
			x -= ln.Width / 2
		}
		minX, maxX = math.Min(minX, x), math.Max(maxX, x+ln.Width)
		minY, maxY = math.Min(minY, top), math.Max(maxY, top+lh)
	}
	return layout.Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
}

// ---- edges ----

func (pw *pageWriter) edge(e *layout.Edge) {
	t := pw.t
	st := &style{}
	if e.Rounded {
		st.add("edgeStyle", "orthogonalEdgeStyle")
		st.add("rounded", "1")
		st.add("arcSize", num.Fmt(2*t.EdgeCornerRad))
		st.add("orthogonalLoop", "1")
		st.add("jettySize", "auto")
	} else {
		st.add("edgeStyle", "none")
		st.add("rounded", "0")
	}
	st.add("html", "1")
	pw.colorKey(st, "strokeColor", "strokeOpacity", e.Color)
	st.add("strokeWidth", num.Fmt(e.Width))
	dash(st, e.Dash)
	size := t.ArrowSize * math.Max(1, e.Width/1.5)
	arrow(st, "end", e.Head, size)
	arrow(st, "start", e.Tail, size)

	pts := simplify(e.Points)
	src, srcOK := pw.ids[e.From]
	dst, dstOK := pw.ids[e.To]
	if len(pts) >= 2 {
		if srcOK {
			port(st, "exit", e.From.Shape, pts[0])
		}
		if dstOK {
			port(st, "entry", e.To.Shape, pts[len(pts)-1])
		}
	}

	value := ""
	var lx float64
	var off model.Point
	labelled := false
	if e.Label != nil {
		value = pw.richText(st, []layout.Line{*e.Label})
		pw.colorKey(st, "labelBackgroundColor", "", t.EdgeLabelFill)
		// draw.io pads its label background equally on every side; split
		// the difference between our horizontal and vertical padding.
		st.add("labelPadding", num.Fmt((t.LabelPaddingX+t.LabelPaddingY)/2))
		if e.LabelRect.W > 0 && len(pts) >= 2 {
			lx, off = along(pts, e.LabelRect.Center())
			labelled = true
		}
	}

	id := pw.id(e.E.ID)
	attrs := fmt.Sprintf(` style="%s" edge="1" parent="1"`, esc(st.String()))
	if srcOK {
		attrs += fmt.Sprintf(` source="%s"`, esc(src))
	}
	if dstOK {
		attrs += fmt.Sprintf(` target="%s"`, esc(dst))
	}
	var kids strings.Builder
	if len(pts) >= 2 {
		if !srcOK {
			pw.point(&kids, pts[0], "sourcePoint")
		}
		if !dstOK {
			pw.point(&kids, pts[len(pts)-1], "targetPoint")
		}
		if len(pts) > 2 {
			kids.WriteString("            <Array as=\"points\">\n")
			for _, p := range pts[1 : len(pts)-1] {
				fmt.Fprintf(&kids, "              <mxPoint x=\"%s\" y=\"%s\"/>\n", num.Fmt(p.X+pw.m), num.Fmt(p.Y+pw.m))
			}
			kids.WriteString("            </Array>\n")
		}
	}
	if labelled && (num.Round(off.X) != 0 || num.Round(off.Y) != 0) {
		fmt.Fprintf(&kids, "            <mxPoint x=\"%s\" y=\"%s\" as=\"offset\"/>\n", num.Fmt(off.X), num.Fmt(off.Y))
	}
	var geom strings.Builder
	geom.WriteString("          <mxGeometry")
	if labelled {
		fmt.Fprintf(&geom, " x=\"%s\"", num.Fmt(lx))
	}
	geom.WriteString(" relative=\"1\" as=\"geometry\"")
	if kids.Len() == 0 {
		geom.WriteString("/>\n")
	} else {
		geom.WriteString(">\n")
		geom.WriteString(kids.String())
		geom.WriteString("          </mxGeometry>\n")
	}
	pw.cell(id, value, e.E.Tooltip, "", attrs, geom.String())
}

func (pw *pageWriter) point(w io.Writer, p model.Point, as string) {
	fmt.Fprintf(w, "            <mxPoint x=\"%s\" y=\"%s\" as=\"%s\"/>\n", num.Fmt(p.X+pw.m), num.Fmt(p.Y+pw.m), as)
}

// port pins an edge end to a point on its cell, as fractions of the cell.
func port(st *style, prefix string, r layout.Rect, p model.Point) {
	fx, fy := 0.5, 0.5
	if r.W > 0 {
		fx = (p.X - r.X) / r.W
	}
	if r.H > 0 {
		fy = (p.Y - r.Y) / r.H
	}
	st.add(prefix+"X", fmtFrac(fx))
	st.add(prefix+"Y", fmtFrac(fy))
	st.add(prefix+"Dx", "0")
	st.add(prefix+"Dy", "0")
	st.add(prefix+"Perimeter", "0")
}

// fmtFrac formats a port fraction with enough precision to land within a
// fraction of a pixel on large shapes.
func fmtFrac(v float64) string {
	r := math.Round(v*10000) / 10000
	if r == 0 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", r), "0"), ".")
}

func arrow(st *style, end string, k model.Arrow, size float64) {
	name, fill, sz := "none", "", size
	switch k {
	case model.ArrowTriangle:
		name, fill = "block", "1"
	case model.ArrowOpen:
		name, fill = "open", "0"
	case model.ArrowDiamond:
		// diamondThin's width is about 0.6 of its length, like ours.
		name, fill, sz = "diamondThin", "1", size*1.5
	case model.ArrowDot:
		name, fill, sz = "oval", "1", size*0.8
	}
	st.add(end+"Arrow", name)
	if name == "none" {
		return
	}
	st.add(end+"Fill", fill)
	st.add(end+"Size", num.Fmt(sz))
}

// simplify drops repeated and collinear interior points, which draw.io's
// orthogonal style would otherwise treat as extra segments.
func simplify(in []model.Point) []model.Point {
	var out []model.Point
	for _, p := range in {
		if n := len(out); n > 0 && near(out[n-1], p) {
			continue
		}
		if n := len(out); n >= 2 && collinear(out[n-2], out[n-1], p) {
			out[n-1] = p
			continue
		}
		out = append(out, p)
	}
	if len(out) == 1 && len(in) >= 2 {
		out = append(out, in[len(in)-1])
	}
	return out
}

func near(a, b model.Point) bool { return math.Abs(a.X-b.X) < 0.01 && math.Abs(a.Y-b.Y) < 0.01 }

func collinear(a, b, c model.Point) bool {
	cross := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	dot := (b.X-a.X)*(c.X-b.X) + (b.Y-a.Y)*(c.Y-b.Y)
	return math.Abs(cross) < 1e-6 && dot > 0
}

// along finds where p projects onto the polyline and returns draw.io's
// relative label position (-1 at the source, 1 at the target) and the
// offset from that point to p.
func along(pts []model.Point, p model.Point) (float64, model.Point) {
	var total float64
	for i := 1; i < len(pts); i++ {
		total += dist(pts[i-1], pts[i])
	}
	if total == 0 {
		return 0, model.Point{X: p.X - pts[0].X, Y: p.Y - pts[0].Y}
	}
	best := math.Inf(1)
	var at float64
	var q model.Point
	var run float64
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		l := dist(a, b)
		f := 0.0
		if l > 0 {
			f = ((p.X-a.X)*(b.X-a.X) + (p.Y-a.Y)*(b.Y-a.Y)) / (l * l)
			f = math.Max(0, math.Min(1, f))
		}
		c := model.Point{X: a.X + (b.X-a.X)*f, Y: a.Y + (b.Y-a.Y)*f}
		if d := dist(c, p); d < best-1e-9 {
			best, at, q = d, run+l*f, c
		}
		run += l
	}
	return num.Round(2*at/total - 1), model.Point{X: p.X - q.X, Y: p.Y - q.Y}
}

func dist(a, b model.Point) float64 { return math.Hypot(b.X-a.X, b.Y-a.Y) }

// ---- colours and styles ----

// color resolves c to a draw.io colour and an opacity percentage (100 when
// opaque). Unset colours are "none".
func (pw *pageWriter) color(c themes.Color) (string, int) {
	if c.IsZero() {
		return "none", 100
	}
	v := c.Resolve(pw.t)
	op := int(math.Round(float64(v.A) * 100 / 255))
	v.A = 0xff
	return themes.HexString(v), op
}

func (pw *pageWriter) hex(c themes.Color) string {
	h, _ := pw.color(c)
	return h
}

func (pw *pageWriter) colorKey(st *style, key, opKey string, c themes.Color) {
	h, op := pw.color(c)
	st.add(key, h)
	if op < 100 && opKey != "" && h != "none" {
		st.add(opKey, fmt.Sprint(op))
	}
}

// style is a draw.io style string in insertion order.
type style struct{ parts []string }

// add appends key=value, or a bare key (a named style such as "text")
// when value is empty.
func (s *style) add(k, v string) {
	if v == "" {
		s.parts = append(s.parts, k)
		return
	}
	s.parts = append(s.parts, k+"="+v)
}

func (s *style) String() string { return strings.Join(s.parts, ";") + ";" }

var (
	attrEsc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "\n", "&#10;", "\r", "&#13;", "\t", "&#9;")
	htmlEsc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
)

// esc escapes an XML attribute value.
func esc(s string) string { return attrEsc.Replace(s) }

// escHTML escapes text inside an HTML label.
func escHTML(s string) string { return htmlEsc.Replace(s) }
