// Package scene is the list of drawing primitives that the SVG and PNG
// renderers consume. Layout and routing produce it; it carries no knowledge
// of nodes or edges beyond the IDs, classes and Meta that SVG and HTML
// output need.
//
// Coordinates are pixels with y pointing down. Colours stay as theme tokens
// so SVG output can emit CSS variables; renderers resolve them.
package scene

import (
	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/themes"
)

// Scene is a complete drawing.
type Scene struct {
	Width, Height float64
	Theme         *themes.Theme
	Background    themes.Color // zero for transparent
	Items         []Item
}

// Item is one primitive or a group of them.
type Item interface{ isItem() }

// Point is a position.
type Point struct{ X, Y float64 }

// Op is a path command.
type Op uint8

const (
	MoveTo  Op = iota // Pts[0]
	LineTo            // Pts[0]
	QuadTo            // control Pts[0], end Pts[1]
	CubicTo           // controls Pts[0], Pts[1], end Pts[2]
	Close
)

// Cmd is one path command with absolute coordinates.
type Cmd struct {
	Op  Op
	Pts [3]Point
}

// Path is a sequence of commands.
type Path []Cmd

func (p *Path) MoveTo(x, y float64) { *p = append(*p, Cmd{Op: MoveTo, Pts: [3]Point{{x, y}}}) }
func (p *Path) LineTo(x, y float64) { *p = append(*p, Cmd{Op: LineTo, Pts: [3]Point{{x, y}}}) }
func (p *Path) QuadTo(cx, cy, x, y float64) {
	*p = append(*p, Cmd{Op: QuadTo, Pts: [3]Point{{cx, cy}, {x, y}}})
}
func (p *Path) CubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	*p = append(*p, Cmd{Op: CubicTo, Pts: [3]Point{{c1x, c1y}, {c2x, c2y}, {x, y}}})
}
func (p *Path) Close() { *p = append(*p, Cmd{Op: Close}) }

// LineCap and LineJoin follow SVG.
type LineCap uint8

const (
	CapButt LineCap = iota
	CapRound
	CapSquare
)

type LineJoin uint8

const (
	JoinMiter LineJoin = iota
	JoinRound
	JoinBevel
)

// Paint describes how a shape is filled and stroked. A zero colour means
// no fill or no stroke.
type Paint struct {
	Fill        themes.Color
	Stroke      themes.Color
	StrokeWidth float64
	Dash        []float64 // on/off lengths; nil for solid
	Cap         LineCap
	Join        LineJoin
	Opacity     float64 // 0 means 1
}

// Group gathers items under an ID and class, which become a <g> in SVG.
type Group struct {
	ID    string
	Class string
	Items []Item
	// Meta describes the diagram element the group draws; nil for groups
	// that draw none.
	Meta *Meta
}

// Kind is the kind of diagram element a Group draws.
type Kind uint8

const (
	KindNode  Kind = iota + 1
	KindGroup      // a container
	KindEdge
	KindLabel // an edge's label, drawn apart from its line
)

func (k Kind) String() string {
	return [...]string{"", "node", "group", "edge", "edge-label"}[k]
}

// Meta is what output formats with tooltips, links or interaction need to
// know about a drawn element.
type Meta struct {
	Kind    Kind
	Label   string // as written, with forced line breaks as "\n"
	Desc    string
	Tooltip string
	Link    string
	// Parent is the ID of the enclosing container, empty at the top level.
	Parent string
	// From and To are an edge's endpoint IDs; For is the ID of the edge an
	// edge label belongs to.
	From, To, For string
	// Hidden marks an invisible container, which draws nothing but is
	// emitted for interactive output so that every endpoint and parent ID
	// names an element.
	Hidden bool
}

// Rect is a rectangle with optional rounded corners.
type Rect struct {
	X, Y, W, H float64
	R          float64 // corner radius
	Paint
}

// Ellipse is an ellipse given by its centre and radii.
type Ellipse struct {
	CX, CY, RX, RY float64
	Paint
}

// Shape is an arbitrary path.
type Shape struct {
	Path Path
	Paint
}

// Anchor is horizontal text alignment.
type Anchor uint8

const (
	AnchorStart Anchor = iota
	AnchorMiddle
	AnchorEnd
)

// Weight is a font weight.
type Weight uint8

const (
	Regular Weight = iota
	SemiBold
)

// Text is a single line of text. X is the anchor point and Y the baseline.
type Text struct {
	X, Y   float64
	Text   string
	Size   float64
	Weight Weight
	Anchor Anchor
	Color  themes.Color
	Width  float64 // measured width, for renderers that position glyphs themselves
}

// Image draws an icon scaled into the box. Color resolves currentColor in
// monochrome SVG icons.
type Image struct {
	X, Y, W, H float64
	Icon       *icons.Icon
	Color      themes.Color
}

func (*Group) isItem()   {}
func (*Rect) isItem()    {}
func (*Ellipse) isItem() {}
func (*Shape) isItem()   {}
func (*Text) isItem()    {}
func (*Image) isItem()   {}
