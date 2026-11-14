// Package svgicon parses the subset of SVG that PNG output can rasterize.
//
// Parse turns an icon into flat shapes: absolute paths in the icon's viewBox
// coordinate space with every transform applied, and paints with inheritance,
// style attributes, href chains and gradient units already resolved. Anything
// outside the subset is an *UnsupportedError naming the feature, never a
// silent drop.
//
// The package mirrors the path types of internal/scene instead of importing
// them: scene imports icons, and icons imports this package.
package svgicon

import (
	"image/color"
	"math"
)

// Icon is a parsed SVG icon.
type Icon struct {
	// ViewBox is min-x, min-y, width, height. Shapes use these coordinates
	// directly; the rasterizer maps the viewBox onto the icon's box and
	// clips to it, as the outermost <svg> does.
	ViewBox [4]float64
	// Shapes in painting order. Each shape paints its fill, then its stroke.
	Shapes []Shape
}

// Shape is one filled and/or stroked path.
type Shape struct {
	Path        Path // absolute viewBox coordinates; arcs are cubics
	Fill        Paint
	Stroke      Paint // Kind None when there is no stroke
	StrokeWidth float64
	Cap         LineCap
	Join        LineJoin
	MiterLimit  float64
	Dash        []float64 // on/off lengths, even count; nil for solid
	DashOffset  float64
	FillRule    FillRule
	// Opacity is the product of the element's opacity and every ancestor's.
	// SVG composites group opacity per group, so this is exact only when
	// shapes in a translucent group don't overlap. A shape with both fill
	// and stroke should ideally be drawn to a layer and composited once.
	Opacity float64
	// FillOpacity and StrokeOpacity include any alpha from the colour
	// itself (rgba(), #rrggbbaa) or from a single-stop gradient.
	FillOpacity, StrokeOpacity float64
}

// Op is a path command. Values match scene.Op.
type Op uint8

const (
	MoveTo  Op = iota // Pts[0]
	LineTo            // Pts[0]
	QuadTo            // control Pts[0], end Pts[1]
	CubicTo           // controls Pts[0], Pts[1], end Pts[2]
	Close
)

// Point is a position.
type Point struct{ X, Y float64 }

// Cmd is one path command with absolute coordinates.
type Cmd struct {
	Op  Op
	Pts [3]Point
}

// Path is a sequence of commands. Every subpath starts with a MoveTo,
// including one that follows a Close.
type Path []Cmd

func (p *Path) moveTo(x, y float64) { *p = append(*p, Cmd{Op: MoveTo, Pts: [3]Point{{x, y}}}) }
func (p *Path) lineTo(x, y float64) { *p = append(*p, Cmd{Op: LineTo, Pts: [3]Point{{x, y}}}) }
func (p *Path) quadTo(cx, cy, x, y float64) {
	*p = append(*p, Cmd{Op: QuadTo, Pts: [3]Point{{cx, cy}, {x, y}}})
}
func (p *Path) cubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	*p = append(*p, Cmd{Op: CubicTo, Pts: [3]Point{{c1x, c1y}, {c2x, c2y}, {x, y}}})
}
func (p *Path) close() { *p = append(*p, Cmd{Op: Close}) }

// npts is the number of points an op uses.
func (o Op) npts() int {
	switch o {
	case MoveTo, LineTo:
		return 1
	case QuadTo:
		return 2
	case CubicTo:
		return 3
	}
	return 0
}

// LineCap values match scene.LineCap.
type LineCap uint8

const (
	CapButt LineCap = iota
	CapRound
	CapSquare
)

// LineJoin values match scene.LineJoin.
type LineJoin uint8

const (
	JoinMiter LineJoin = iota
	JoinRound
	JoinBevel
)

// FillRule is the SVG fill-rule.
type FillRule uint8

const (
	NonZero FillRule = iota
	EvenOdd
)

// PaintKind says what a Paint is.
type PaintKind uint8

const (
	None         PaintKind = iota
	Solid                  // Color
	CurrentColor           // the node's icon colour, chosen at render time
	Linear                 // Gradient, linear
	Radial                 // Gradient, radial
)

// Paint is a resolved fill or stroke.
type Paint struct {
	Kind PaintKind
	// Color is opaque (A is 255) for Solid. Colour alpha is folded into
	// the shape's FillOpacity or StrokeOpacity.
	Color    color.RGBA
	Gradient *Gradient // Linear and Radial only
}

// Spread is a gradient's spreadMethod.
type Spread uint8

const (
	SpreadPad Spread = iota
	SpreadReflect
	SpreadRepeat
)

// Gradient is a resolved linear or radial gradient.
//
// Coordinates are in gradient space. Transform maps gradient space to the
// icon's viewBox space and already includes gradientTransform, the
// referencing element's transforms and, for objectBoundingBox units, the
// element's bounding box. To shade a point P in viewBox space, map it
// through Transform's inverse and evaluate the gradient there.
//
// Degenerate gradients never get here: no stops is None, one stop or a
// zero-length vector or zero radius is Solid, and an objectBoundingBox
// gradient on a zero-width or zero-height shape is None.
type Gradient struct {
	Spread    Spread
	Transform Matrix
	// Linear: the gradient vector.
	X1, Y1, X2, Y2 float64
	// Radial: end circle (CX, CY, R) and focal circle (FX, FY, FR). The
	// focal point may lie outside the end circle (SVG 2 draws a cone).
	CX, CY, R, FX, FY, FR float64
	// Stops have non-decreasing offsets in [0, 1]. At least two.
	Stops []Stop
}

// Stop is a gradient stop. Interpolate in non-premultiplied sRGB, as
// browsers do.
type Stop struct {
	Offset       float64
	Color        color.RGBA // opaque; ignore when CurrentColor is set
	CurrentColor bool
	Opacity      float64 // stop-opacity times any colour alpha
}

// UnsupportedError reports the first feature outside the subset.
type UnsupportedError struct {
	Feature string // e.g. "clipPath", "clip-path", "missing reference #g1"
	Element string // local name of the element it was found on
}

func (e *UnsupportedError) Error() string {
	if e.Feature == e.Element {
		return "svgicon: unsupported element <" + e.Element + ">"
	}
	return "svgicon: unsupported " + e.Feature + " on <" + e.Element + ">"
}

func unsupported(feature string, n *node) error {
	return &UnsupportedError{Feature: feature, Element: n.name}
}

// Matrix is an affine transform {a, b, c, d, e, f}, as in SVG's
// matrix(a b c d e f): x' = a·x + c·y + e, y' = b·x + d·y + f.
type Matrix [6]float64

// Identity is the identity transform.
var Identity = Matrix{1, 0, 0, 1, 0, 0}

// Mul returns m × n, which applies n first and then m.
func (m Matrix) Mul(n Matrix) Matrix {
	return Matrix{
		m[0]*n[0] + m[2]*n[1],
		m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3],
		m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4],
		m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

// Apply transforms a point.
func (m Matrix) Apply(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// Det returns the determinant of the linear part.
func (m Matrix) Det() float64 { return m[0]*m[3] - m[1]*m[2] }

// Invert returns the inverse transform, or false if m is singular.
func (m Matrix) Invert() (Matrix, bool) {
	det := m.Det()
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return Matrix{}, false
	}
	a, b, c, d := m[3]/det, -m[1]/det, -m[2]/det, m[0]/det
	return Matrix{a, b, c, d, -(a*m[4] + c*m[5]), -(b*m[4] + d*m[5])}, true
}

func translate(x, y float64) Matrix { return Matrix{1, 0, 0, 1, x, y} }

// scale returns the factor stroke widths and dash lengths scale by.
func (m Matrix) scale() float64 { return math.Sqrt(math.Abs(m.Det())) }

func (p Path) transform(m Matrix) Path {
	out := make(Path, len(p))
	for i, c := range p {
		out[i].Op = c.Op
		for k := range c.Op.npts() {
			out[i].Pts[k].X, out[i].Pts[k].Y = m.Apply(c.Pts[k].X, c.Pts[k].Y)
		}
	}
	return out
}
