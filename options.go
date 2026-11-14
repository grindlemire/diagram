package diagram

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/themes"
)

// A NodeOption configures a node.
type NodeOption interface {
	applyNode(*model.Element, *ctx)
}

// A GroupOption configures a container (Row, Column, Grid or Canvas).
type GroupOption interface {
	applyGroup(*model.Element, *ctx)
}

// An EdgeOption configures an edge.
type EdgeOption interface {
	applyEdge(*model.Edge, *ctx)
}

// An ElementOption configures a node or a container.
type ElementOption interface {
	NodeOption
	GroupOption
}

// An AnyOption configures a node, container or edge.
type AnyOption interface {
	NodeOption
	GroupOption
	EdgeOption
}

// A DiagramOption configures a diagram.
type DiagramOption interface {
	applyDiagram(*model.Diagram, *ctx)
}

// ctx carries what an option needs to record an error against the call
// that used it.
type ctx struct {
	d    *model.Diagram
	src  model.Src
	what string
}

func (c *ctx) errorf(format string, args ...any) {
	c.d.Errorf(c.src, c.what+": "+format, args...)
}

func (c *ctx) checkLen(name string, v float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		c.errorf("%s(%v): want a finite, non-negative number", name, v)
		return false
	}
	return true
}

func (c *ctx) checkColor(name string, col Color) bool {
	if err := col.Err(); err != nil {
		c.errorf("%s: %v", name, err)
		return false
	}
	return true
}

// nodeOpt applies only to nodes.
type nodeOpt func(*model.Element, *ctx)

func (f nodeOpt) applyNode(e *model.Element, c *ctx) { f(e, c) }

// groupOpt applies only to containers.
type groupOpt func(*model.Element, *ctx)

func (f groupOpt) applyGroup(e *model.Element, c *ctx) { f(e, c) }

// elemOpt applies to nodes and containers.
type elemOpt func(*model.Element, *ctx)

func (f elemOpt) applyNode(e *model.Element, c *ctx)  { f(e, c) }
func (f elemOpt) applyGroup(e *model.Element, c *ctx) { f(e, c) }

// edgeOpt applies only to edges.
type edgeOpt func(*model.Edge, *ctx)

func (f edgeOpt) applyEdge(e *model.Edge, c *ctx) { f(e, c) }

type diagramOpt func(*model.Diagram, *ctx)

func (f diagramOpt) applyDiagram(d *model.Diagram, c *ctx) { f(d, c) }

// ---- Diagram options ----

// Theme sets the diagram's theme. The default is themes.Light.
func Theme(t *themes.Theme) DiagramOption {
	return diagramOpt(func(d *model.Diagram, c *ctx) {
		if t == nil {
			c.errorf("Theme(nil)")
			return
		}
		d.Theme = t
	})
}

// Vertical makes the diagram's top level a Column instead of a Row.
var Vertical DiagramOption = diagramOpt(func(d *model.Diagram, _ *ctx) {
	d.Vertical = true
	d.Root.Kind = model.KindColumn
})

// ExactSpacing lays out every gap exactly as written. By default, gaps
// between children are raised to a floor that leaves room for an edge and
// its label wherever edges connect, and after routing, gaps whose edges
// don't fit side by side are widened and the diagram laid out again.
// ExactSpacing turns both off: use it when positions must match a
// design, such as a Canvas traced from a mock-up or gaps matching another
// diagram, and edges may squeeze or detour to fit.
var ExactSpacing DiagramOption = diagramOpt(func(d *model.Diagram, _ *ctx) {
	d.ExactSpacing = true
})

// ---- Node and group content ----

// Desc sets secondary text, drawn smaller and muted under the label.
func Desc(s string) ElementOption {
	return elemOpt(func(e *model.Element, _ *ctx) { e.Desc = s })
}

// Icon sets a node's icon, or the icon in a group's header.
func Icon(ic *icons.Icon) ElementOption {
	return elemOpt(func(e *model.Element, c *ctx) {
		if ic == nil {
			c.errorf("Icon(nil)")
			return
		}
		e.Icon = ic
	})
}

// IconColor sets the colour of a node's icon, or of a group's header icon.
// It only affects monochrome icons, which are drawn in the theme's icon
// colour by default; multicoloured icons keep their own colours.
func IconColor(col Color) ElementOption {
	return elemOpt(func(e *model.Element, c *ctx) {
		if c.checkColor("IconColor", col) {
			e.IconColor = col
		}
	})
}

// ID overrides the element's or edge's generated ID, which is used for
// draw.io cell IDs and SVG id attributes. IDs must be unique in a diagram.
func ID(id string) AnyOption {
	return idOpt(id)
}

type idOpt string

func (o idOpt) applyNode(e *model.Element, c *ctx)  { o.applyElem(e, c) }
func (o idOpt) applyGroup(e *model.Element, c *ctx) { o.applyElem(e, c) }
func (o idOpt) applyElem(e *model.Element, c *ctx) {
	if o == "" {
		c.errorf(`ID(""): want a non-empty ID`)
		return
	}
	c.d.ClaimID(string(o), c.src, c.what)
	e.ExplicitID = string(o)
}
func (o idOpt) applyEdge(e *model.Edge, c *ctx) {
	if o == "" {
		c.errorf(`ID(""): want a non-empty ID`)
		return
	}
	c.d.ClaimID(string(o), c.src, c.what)
	e.ExplicitID = string(o)
}

// Tooltip adds text shown when the pointer rests on a node, container or
// edge. In SVG output it becomes a <title> the viewer shows on hover; in
// HTML output it appears in the tooltip under the label and description;
// in draw.io output it is the cell's tooltip. PNG output ignores it.
func Tooltip(text string) AnyOption {
	return tooltipOpt(text)
}

type tooltipOpt string

func (o tooltipOpt) applyNode(e *model.Element, _ *ctx)  { e.Tooltip = string(o) }
func (o tooltipOpt) applyGroup(e *model.Element, _ *ctx) { e.Tooltip = string(o) }
func (o tooltipOpt) applyEdge(e *model.Edge, _ *ctx)     { e.Tooltip = string(o) }

// Link makes a node or container link to url, which must be an http,
// https or mailto URL, or a relative reference. In SVG output the element
// is wrapped in an <a>; in HTML output its tooltip has an Open link and
// Cmd-click or Ctrl-click opens it; in draw.io output it is the cell's
// link. PNG output ignores it.
func Link(url string) ElementOption {
	return elemOpt(func(e *model.Element, c *ctx) {
		if err := checkURL(url); err != nil {
			c.errorf("Link(%q): %v", url, err)
			return
		}
		e.Link = url
	})
}

func checkURL(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("want a URL")
	}
	u, err := url.Parse(s)
	if err != nil {
		return errors.New("not a valid URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "":
		return nil
	case "http", "https":
		if u.Host == "" {
			return errors.New("want a host")
		}
		return nil
	case "mailto":
		if u.Opaque == "" {
			return errors.New("want an address")
		}
		return nil
	}
	return fmt.Errorf("scheme %q not allowed (want http, https, mailto or a relative URL)", u.Scheme)
}

// Span makes a grid child cover rows×cols cells, starting at its cell.
func Span(rows, cols int) ElementOption {
	return elemOpt(func(e *model.Element, c *ctx) {
		if e.Parent == nil || e.Parent.Kind != model.KindGrid {
			c.errorf("Span used outside a Grid")
			return
		}
		if rows < 1 || cols < 1 {
			c.errorf("Span(%d, %d): want at least 1×1", rows, cols)
			return
		}
		e.RowSpan, e.ColSpan = rows, cols
	})
}

// ---- Node options ----

// Shape is a node's outline. Every shape has a native draw.io equivalent.
type Shape uint8

const (
	Box      Shape = Shape(model.ShapeBox) // rounded rectangle, the default
	Rect     Shape = Shape(model.ShapeRect)
	Pill     Shape = Shape(model.ShapePill)
	Circle   Shape = Shape(model.ShapeCircle)
	Cylinder Shape = Shape(model.ShapeCylinder) // databases
	Queue    Shape = Shape(model.ShapeQueue)    // horizontal cylinder
	Document Shape = Shape(model.ShapeDocument)
	Person   Shape = Shape(model.ShapePerson)
	Note     Shape = Shape(model.ShapeNote) // folded corner, for annotations
	Text     Shape = Shape(model.ShapeText) // no border or fill
)

func (s Shape) applyNode(e *model.Element, _ *ctx) { e.Shape = model.Shape(s) }

// IconPlacement is where a node's icon sits.
type IconPlacement uint8

const (
	IconTop  IconPlacement = IconPlacement(model.IconTop)  // above the label (default)
	IconLeft IconPlacement = IconPlacement(model.IconLeft) // left of the text
	IconOnly IconPlacement = IconPlacement(model.IconOnly) // no box; label under the icon
)

func (p IconPlacement) applyNode(e *model.Element, _ *ctx) { e.IconPlace = model.IconPlace(p) }

// IconSize sets the icon's width in pixels; height follows its aspect ratio.
func IconSize(px float64) NodeOption {
	return nodeOpt(func(e *model.Element, c *ctx) {
		if c.checkLen("IconSize", px) {
			e.IconSize = model.Some(px)
		}
	})
}

// Size fixes a node's size instead of fitting it to its content.
func Size(w, h float64) NodeOption {
	return nodeOpt(func(e *model.Element, c *ctx) {
		if c.checkLen("Size", w) && c.checkLen("Size", h) {
			e.Width, e.Height = model.Some(w), model.Some(h)
		}
	})
}

// MaxWidth sets the width at which a node's text wraps.
func MaxWidth(px float64) NodeOption {
	return nodeOpt(func(e *model.Element, c *ctx) {
		if c.checkLen("MaxWidth", px) {
			e.MaxWidth = model.Some(px)
		}
	})
}

// Badge adds a small pill with text, such as "×3" or "beta", straddling
// the node's top-right corner. It is drawn in the theme's accent colour
// with text that stays readable on it. Several badges stack leftwards from
// the corner in the order given. Edge labels keep clear of badges, and
// edges avoid attaching under them where the side has room elsewhere.
func Badge(text string) NodeOption {
	return nodeOpt(func(e *model.Element, c *ctx) {
		if strings.TrimSpace(text) == "" {
			c.errorf(`Badge(%q): want some text`, text)
			return
		}
		e.Badges = append(e.Badges, text)
	})
}

// Status adds a small dot in colour col on the node's top-left corner,
// such as diagram.Green for healthy, diagram.Yellow or diagram.Orange for
// degraded and diagram.Red for down. A ring in the colour behind the node
// sets it off from the border. A later Status replaces an earlier one.
func Status(col Color) NodeOption {
	return nodeOpt(func(e *model.Element, c *ctx) {
		if col.IsZero() {
			c.errorf("Status: want a colour")
			return
		}
		if c.checkColor("Status", col) {
			e.Status = col
		}
	})
}

// ---- Group options ----

// Gap sets the space between a container's children.
func Gap(px float64) GroupOption {
	return groupOpt(func(e *model.Element, c *ctx) {
		if c.checkLen("Gap", px) {
			e.Gap = model.Some(px)
		}
	})
}

// Padding sets the space between a container's border and its children.
func Padding(px float64) GroupOption {
	return groupOpt(func(e *model.Element, c *ctx) {
		if c.checkLen("Padding", px) {
			e.Padding = model.Some(px)
		}
	})
}

// Alignment is how children line up across a container's main axis, on
// the cross axis: vertically in a Row, horizontally in a Column.
type Alignment uint8

const (
	AlignCenter Alignment = Alignment(model.AlignCenter) // the default
	AlignStart  Alignment = Alignment(model.AlignStart)
	AlignEnd    Alignment = Alignment(model.AlignEnd)
)

func (a Alignment) applyGroup(e *model.Element, _ *ctx) { e.Align = model.Align(a) }

// Justify is where a Row's or Column's children sit along its main axis
// when the container is larger than they need, for example a Column
// stretched to the height of a taller sibling.
type Justify uint8

const (
	JustifyStart  Justify = Justify(model.JustifyStart)  // at the start: top or left (the default)
	JustifyCenter Justify = Justify(model.JustifyCenter) // in the middle
	JustifyEnd    Justify = Justify(model.JustifyEnd)    // at the end: bottom or right
	// JustifySpaceBetween puts the first child at the start, the last at
	// the end, and spreads the extra space evenly between them.
	JustifySpaceBetween Justify = Justify(model.JustifySpaceBetween)
)

func (j Justify) applyGroup(e *model.Element, c *ctx) {
	if e.Kind != model.KindRow && e.Kind != model.KindColumn {
		c.errorf("Justify applies only to a Row or Column")
		return
	}
	e.Justify = model.Justify(j)
}

type flagOpt func(*model.Element)

func (f flagOpt) applyGroup(e *model.Element, _ *ctx) { f(e) }

var (
	// Uniform gives every direct child node of the container the same
	// size: the largest any of them needs. Circles stay round and Person
	// and IconOnly figures keep their own size; each is centred in the
	// space the others fill. Nodes with a fixed Size are left alone.
	Uniform GroupOption = flagOpt(func(e *model.Element) { e.Uniform = true })

	// Invisible hides a labelled container's box and header; it still
	// lays out its children.
	Invisible GroupOption = flagOpt(func(e *model.Element) { e.Invisible = model.Some(true) })

	// Visible draws a box around an unlabelled container.
	Visible GroupOption = flagOpt(func(e *model.Element) { e.Invisible = model.Some(false) })

	// HeaderTop centres the group's header at the top instead of placing it
	// at the top left.
	HeaderTop GroupOption = flagOpt(func(e *model.Element) { e.HeaderTop = true })

	// AllowOverlap lets a Canvas's children overlap. Without it,
	// overlapping children are an error.
	AllowOverlap GroupOption = groupOpt(func(e *model.Element, c *ctx) {
		if e.Kind != model.KindCanvas {
			c.errorf("AllowOverlap applies only to a Canvas")
			return
		}
		e.AllowOverlap = true
	})
)

// Defaults applies styles to the nodes created directly in this container.
// Each node's own options still win. Nested containers are not affected.
func Defaults(styles ...StyleOption) GroupOption {
	s := Style(styles...)
	return groupOpt(func(e *model.Element, c *ctx) { s.apply(&e.Defaults, c) })
}

// ---- Edge options ----

// Label sets an edge's label.
func Label(s string) EdgeOption {
	return edgeOpt(func(e *model.Edge, _ *ctx) { e.Label = s })
}

// Position is a place along an edge.
type Position uint8

const (
	Middle Position = Position(model.LabelMiddle)
	Start  Position = Position(model.LabelStart)
	End    Position = Position(model.LabelEnd)
)

// LabelAt moves an edge's label towards one end.
func LabelAt(p Position) EdgeOption {
	return edgeOpt(func(e *model.Edge, _ *ctx) { e.LabelAt = model.LabelPos(p) })
}

// Side is a side of a box.
type Side uint8

const (
	Top    Side = Side(model.SideTop)
	Right  Side = Side(model.SideRight)
	Bottom Side = Side(model.SideBottom)
	Left   Side = Side(model.SideLeft)
)

func (s Side) valid() bool { return s >= Top && s <= Left }

// Exit pins the side of the source an edge leaves from.
func Exit(s Side) EdgeOption {
	return edgeOpt(func(e *model.Edge, c *ctx) {
		if !s.valid() {
			c.errorf("Exit(%d): not a side", s)
			return
		}
		e.Exit = model.Side(s)
	})
}

// Enter pins the side of the target an edge arrives at.
func Enter(s Side) EdgeOption {
	return edgeOpt(func(e *model.Edge, c *ctx) {
		if !s.valid() {
			c.errorf("Enter(%d): not a side", s)
			return
		}
		e.Enter = model.Side(s)
	})
}

// Point is a position in diagram coordinates.
type Point struct{ X, Y float64 }

// Pt returns a Point.
func Pt(x, y float64) Point { return Point{x, y} }

// Via makes an edge pass through waypoints, in order.
func Via(points ...Point) EdgeOption {
	return edgeOpt(func(e *model.Edge, c *ctx) {
		for _, p := range points {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
				c.errorf("Via(%v, %v): want finite coordinates", p.X, p.Y)
				return
			}
			e.Via = append(e.Via, model.Point{X: p.X, Y: p.Y})
		}
	})
}

// Straight draws the edge as a straight line instead of routing it
// orthogonally.
var Straight EdgeOption = edgeOpt(func(e *model.Edge, _ *ctx) { e.Straight = true })

// CrossGroups lets an edge pass through groups that hold neither of its
// ends, instead of routing around them.
var CrossGroups EdgeOption = edgeOpt(func(e *model.Edge, _ *ctx) { e.CrossGroups = true })

// ArrowKind is an arrowhead shape.
type ArrowKind uint8

const (
	Triangle ArrowKind = ArrowKind(model.ArrowTriangle)
	Open     ArrowKind = ArrowKind(model.ArrowOpen)
	Diamond  ArrowKind = ArrowKind(model.ArrowDiamond)
	Dot      ArrowKind = ArrowKind(model.ArrowDot)
	None     ArrowKind = ArrowKind(model.ArrowNone)
)

var (
	// Arrow draws an arrowhead at the target only. This is the default.
	Arrow EdgeOption = edgeOpt(func(e *model.Edge, _ *ctx) {
		e.Head, e.Tail = model.ArrowTriangle, model.ArrowNone
	})
	// ArrowBoth draws arrowheads at both ends.
	ArrowBoth EdgeOption = edgeOpt(func(e *model.Edge, _ *ctx) {
		e.Head, e.Tail = model.ArrowTriangle, model.ArrowTriangle
	})
	// NoArrow draws no arrowheads.
	NoArrow EdgeOption = edgeOpt(func(e *model.Edge, _ *ctx) {
		e.Head, e.Tail = model.ArrowNone, model.ArrowNone
	})
)

// Head sets the arrowhead at the target end.
func Head(k ArrowKind) EdgeOption {
	return edgeOpt(func(e *model.Edge, _ *ctx) { e.Head = model.Arrow(k) })
}

// Tail sets the arrowhead at the source end.
func Tail(k ArrowKind) EdgeOption {
	return edgeOpt(func(e *model.Edge, _ *ctx) { e.Tail = model.Arrow(k) })
}
