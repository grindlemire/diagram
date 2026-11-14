// Package model is the element tree that the public API builds and that
// layout, routing and rendering consume. It holds no geometry.
package model

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/themes"
)

// Kind is an element's kind.
type Kind uint8

const (
	KindNode Kind = iota
	KindRow
	KindColumn
	KindGrid
	KindCanvas
)

func (k Kind) String() string {
	return [...]string{"node", "row", "column", "grid", "canvas"}[k]
}

// IsGroup reports whether the element is a container.
func (k Kind) IsGroup() bool { return k != KindNode }

// Shape is a node's outline.
type Shape uint8

const (
	ShapeBox Shape = iota // rounded rectangle
	ShapeRect
	ShapePill
	ShapeCircle
	ShapeCylinder
	ShapeQueue
	ShapeDocument
	ShapePerson
	ShapeNote
	ShapeText // no border or fill
)

// IconPlace is where a node's icon sits relative to its text.
type IconPlace uint8

const (
	IconTop IconPlace = iota
	IconLeft
	IconOnly // icon without a box, label underneath
)

// Align is cross-axis alignment in a flow or grid container.
type Align uint8

const (
	AlignCenter Align = iota
	AlignStart
	AlignEnd
)

// Justify is main-axis placement of a container's children when the
// container is larger than they need.
type Justify uint8

const (
	JustifyStart Justify = iota
	JustifyCenter
	JustifyEnd
	JustifySpaceBetween
)

// Side is a side of a box, used for edge ports.
type Side uint8

const (
	SideAuto Side = iota
	SideTop
	SideRight
	SideBottom
	SideLeft
)

func (s Side) String() string {
	return [...]string{"auto", "top", "right", "bottom", "left"}[s]
}

// LabelPos is where an edge label sits along the edge.
type LabelPos uint8

const (
	LabelMiddle LabelPos = iota
	LabelStart
	LabelEnd
)

// Arrow is an arrowhead kind. ArrowUnset means "use the default".
type Arrow uint8

const (
	ArrowUnset Arrow = iota
	ArrowNone
	ArrowTriangle
	ArrowOpen
	ArrowDiamond
	ArrowDot
)

// Dash is a line pattern. DashUnset means "use the default".
type Dash uint8

const (
	DashUnset Dash = iota
	DashSolid
	DashDashed
	DashDotted
)

// Opt is an optional value.
type Opt[T any] struct {
	V   T
	Set bool
}

// Some returns a set Opt.
func Some[T any](v T) Opt[T] { return Opt[T]{V: v, Set: true} }

// Or returns the value if set, or def.
func (o Opt[T]) Or(def T) T {
	if o.Set {
		return o.V
	}
	return def
}

func (o Opt[T]) over(base Opt[T]) Opt[T] {
	if o.Set {
		return o
	}
	return base
}

// Style holds the visual properties a user can set. Unset fields fall back
// to the theme.
type Style struct {
	Fill        themes.Color
	Stroke      themes.Color
	TextColor   themes.Color
	StrokeWidth Opt[float64]
	Dash        Dash
	Radius      Opt[float64]
	FontSize    Opt[float64]
	Bold        Opt[bool]
	// Names lists the named styles (diagram.Named) applied, in the order
	// they were first applied. The slice is shared between copies of a
	// Style, so add to it only with AddName.
	Names []string
}

// AddName returns names with name added at the end, unless it is already
// there. It never writes into names' backing array, which other styles
// may share.
func AddName(names []string, name string) []string {
	if slices.Contains(names, name) {
		return names
	}
	return append(slices.Clip(names), name)
}

// SameLook reports whether s and o set the same visual properties, ignoring
// the names they carry.
func (s Style) SameLook(o Style) bool {
	s.Names, o.Names = nil, nil
	return reflect.DeepEqual(s, o)
}

// Over returns s with any unset fields taken from base.
func (s Style) Over(base Style) Style {
	out := s
	if out.Fill.IsZero() {
		out.Fill = base.Fill
	}
	if out.Stroke.IsZero() {
		out.Stroke = base.Stroke
	}
	if out.TextColor.IsZero() {
		out.TextColor = base.TextColor
	}
	if out.Dash == DashUnset {
		out.Dash = base.Dash
	}
	out.StrokeWidth = s.StrokeWidth.over(base.StrokeWidth)
	out.Radius = s.Radius.over(base.Radius)
	out.FontSize = s.FontSize.over(base.FontSize)
	out.Bold = s.Bold.over(base.Bold)
	if len(out.Names) == 0 {
		out.Names = base.Names
	}
	return out
}

// Point is a position in diagram coordinates.
type Point struct{ X, Y float64 }

// Src is the source location of a builder call.
type Src struct {
	File string
	Line int
}

func (s Src) String() string {
	if s.File == "" {
		return "?"
	}
	return fmt.Sprintf("%s:%d", s.File, s.Line)
}

// Caller returns the source location skip frames above its caller.
func Caller(skip int) Src {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return Src{}
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, file); err == nil && !strings.HasPrefix(rel, "..") {
			file = rel
		}
	}
	return Src{File: file, Line: line}
}

// Element is a node or a container.
type Element struct {
	Kind     Kind
	Label    string
	Desc     string
	Icon     *icons.Icon
	Parent   *Element
	Children []*Element
	Diagram  *Diagram
	Src      Src
	Depth    int // number of labelled (visible) group ancestors

	ID         string // final ID, set by Diagram.Finalize
	ExplicitID string

	Style Style

	// Placement within the parent.
	Row, Col         int // grid
	RowSpan, ColSpan int // grid; 0 means 1
	X, Y             float64

	// Node fields.
	Shape     Shape
	IconPlace IconPlace
	IconSize  Opt[float64]
	Width     Opt[float64] // fixed size
	Height    Opt[float64]
	MaxWidth  Opt[float64]

	// Container fields.
	Cols      int // grid
	Gap       Opt[float64]
	Padding   Opt[float64]
	Align     Align
	Uniform   bool
	Invisible Opt[bool]
	HeaderTop bool
	Defaults  Style // applied under the style of nodes created in this group
	Justify   Justify
	// AllowOverlap turns off the overlap check for a Canvas's children.
	AllowOverlap bool

	// IconColor overrides the theme's icon colour for monochrome icons.
	IconColor themes.Color

	// Tooltip is extra text shown on hover in SVG and HTML output, and as
	// the cell tooltip in draw.io.
	Tooltip string
	// Link is a URL the element opens, already validated.
	Link string

	// Badges are short texts drawn as pills on a node's top-right corner,
	// in the order given, the first rightmost.
	Badges []string
	// Status is the colour of a dot on a node's top-left corner; zero for
	// none.
	Status themes.Color

	// Legend marks a container made by Legend, which layout fills with an
	// entry for each named style the diagram uses.
	Legend bool
	// Generated marks an element layout made, such as a legend entry. It
	// is not in its parent's Children and is made afresh by every layout.
	Generated bool
}

// Visible reports whether a container draws a box. Labelled containers
// are visible unless marked invisible; unlabelled ones are invisible unless
// marked visible.
func (e *Element) Visible() bool {
	if e.Kind == KindNode {
		return true
	}
	return e.Invisible.Set && !e.Invisible.V || !e.Invisible.Set && e.Label != ""
}

// Describe names the element for error messages.
func (e *Element) Describe() string {
	if e.Label != "" {
		return fmt.Sprintf("%s %q", e.Kind, e.Label)
	}
	if e.ID != "" {
		return fmt.Sprintf("%s %s", e.Kind, e.ID)
	}
	return e.Kind.String()
}

// Edge connects two elements.
type Edge struct {
	From, To *Element
	Label    string
	LabelAt  LabelPos
	Exit     Side
	Enter    Side
	Via      []Point
	Straight bool
	// CrossGroups lets the edge pass through groups that hold neither end.
	CrossGroups bool
	Head        Arrow
	Tail        Arrow
	Style       Style
	Src         Src
	// Tooltip is extra text shown on hover, as for elements.
	Tooltip string

	ID         string
	ExplicitID string
}

// Describe names the edge for error messages.
func (e *Edge) Describe() string {
	return fmt.Sprintf("edge %s → %s", name(e.From), name(e.To))
}

func name(e *Element) string {
	if e == nil {
		return "<nil>"
	}
	if e.Label != "" {
		return e.Label
	}
	return e.ID
}

// Of returns the model behind a *diagram.Diagram. Package diagram sets it
// at init; it lets internal tests and tools reach the model without the
// public API exposing it.
var Of func(d any) *Diagram

// Diagram is the whole model.
type Diagram struct {
	Title    string
	Root     *Element
	Edges    []*Edge
	Theme    *themes.Theme
	Vertical bool
	// ExactSpacing keeps every gap as written: no spacing floor and no
	// widening of gaps too narrow for their edges.
	ExactSpacing bool

	// Named lists the named styles applied anywhere, in the order they
	// were first applied.
	Named []*NamedStyle

	errs []error
	ids  map[string]Src // explicit IDs, for duplicate detection
}

// NamedStyle is a style bundle with a name (diagram.Named).
type NamedStyle struct {
	Name  string
	Style Style // what the bundle sets on its own
	Src   Src   // where it was defined
}

// UseNamed records that the named style ns was applied. It returns the
// style already recorded under the same name when that one looks
// different, or nil.
func (d *Diagram) UseNamed(ns *NamedStyle) (conflict *NamedStyle) {
	for _, o := range d.Named {
		if o.Name != ns.Name {
			continue
		}
		if o != ns && !o.Style.SameLook(ns.Style) {
			return o
		}
		return nil
	}
	d.Named = append(d.Named, ns)
	return nil
}

// New returns an empty diagram.
func New(title string) *Diagram {
	d := &Diagram{Title: title, Theme: themes.Light, ids: map[string]Src{}}
	d.Root = &Element{Kind: KindRow, Diagram: d}
	return d
}

// Errorf records a builder error at src.
func (d *Diagram) Errorf(src Src, format string, args ...any) {
	d.errs = append(d.errs, fmt.Errorf("%s: %s", src, fmt.Sprintf(format, args...)))
}

// Errs returns the recorded errors in the order they happened.
func (d *Diagram) Errs() []error { return d.errs }

// ClaimID records an explicit ID, reporting a duplicate.
func (d *Diagram) ClaimID(id string, src Src, what string) {
	if prev, ok := d.ids[id]; ok {
		d.Errorf(src, "%s: ID %q already used at %s", what, id, prev)
		return
	}
	d.ids[id] = src
}

// Finalize assigns every element and edge its ID. It runs before layout
// and recomputes every ID from scratch each time, so elements added after
// an earlier Validate or render still get IDs, and IDs only depend on the
// final tree.
func (d *Diagram) Finalize() {
	used := map[string]bool{}
	for id := range d.ids {
		used[id] = true
	}
	var walk func(e *Element, prefix string)
	walk = func(e *Element, prefix string) {
		seen := map[string]int{}
		for i, c := range e.Children {
			if c.ExplicitID != "" {
				c.ID = c.ExplicitID
			} else {
				seg := Slug(c.Label)
				if seg == "" {
					seg = fmt.Sprintf("%s-%d", c.Kind, i+1)
				}
				seen[seg]++
				if n := seen[seg]; n > 1 {
					seg = fmt.Sprintf("%s-%d", seg, n)
				}
				c.ID = unique(prefix+seg, used)
			}
			walk(c, c.ID+"/")
		}
	}
	walk(d.Root, "")
	for _, e := range d.Edges {
		if e.ExplicitID != "" {
			e.ID = e.ExplicitID
			continue
		}
		if e.From == nil || e.To == nil {
			continue
		}
		e.ID = unique("edge:"+e.From.ID+"->"+e.To.ID, used)
	}
}

func unique(id string, used map[string]bool) string {
	cand := id
	for n := 2; used[cand]; n++ {
		cand = fmt.Sprintf("%s~%d", id, n)
	}
	used[cand] = true
	return cand
}

// Slug turns a label into an ID segment: lower case, letters and digits,
// runs of anything else collapsed to "-".
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r > 127 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// Walk visits e and its descendants in declaration order.
func Walk(e *Element, fn func(*Element)) {
	fn(e)
	for _, c := range e.Children {
		Walk(c, fn)
	}
}
