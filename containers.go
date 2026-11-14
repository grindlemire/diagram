package diagram

import (
	"math"

	"github.com/grindlemire/diagram/internal/model"
)

// An Endpoint is something an edge can connect: a *Node or any container.
type Endpoint interface {
	element() *model.Element
}

// Node is a box with a label, and optionally a description and an icon.
type Node struct{ el *model.Element }

// Row lays out its children left to right.
type Row struct{ el *model.Element }

// Column lays out its children top to bottom.
type Column struct{ el *model.Element }

// Grid places its children in cells. It has a fixed number of columns and
// grows rows as needed.
type Grid struct{ el *model.Element }

// Canvas places its children at explicit coordinates relative to its own
// origin, and sizes itself to fit them.
type Canvas struct{ el *model.Element }

func (n *Node) element() *model.Element   { return n.el }
func (r *Row) element() *model.Element    { return r.el }
func (c *Column) element() *model.Element { return c.el }
func (g *Grid) element() *model.Element   { return g.el }
func (c *Canvas) element() *model.Element { return c.el }

// placement sets where a new child sits in its parent.
type placement func(*model.Element)

func flowPlace(*model.Element) {}

func cellPlace(row, col int) placement {
	return func(e *model.Element) { e.Row, e.Col = row, col }
}

func xyPlace(x, y float64) placement {
	return func(e *model.Element) { e.X, e.Y = x, y }
}

func newChild(parent *model.Element, kind model.Kind, label string, src model.Src, place placement) *model.Element {
	d := parent.Diagram
	depth := parent.Depth
	if parent != d.Root && parent.Visible() {
		depth++
	}
	e := &model.Element{Kind: kind, Label: label, Parent: parent, Diagram: d, Src: src, Depth: depth}
	place(e)
	return e
}

func addNode(parent *model.Element, label string, src model.Src, place placement, opts []NodeOption) *Node {
	e := newChild(parent, model.KindNode, label, src, place)
	e.Style = parent.Defaults
	c := &ctx{d: parent.Diagram, src: src, what: e.Describe()}
	for _, o := range opts {
		if o != nil {
			o.applyNode(e, c)
		}
	}
	attach(parent, e, c)
	return &Node{e}
}

func addGroup(parent *model.Element, kind model.Kind, label string, cols int, src model.Src, place placement, opts []GroupOption) *model.Element {
	e := newChild(parent, kind, label, src, place)
	e.Cols = cols
	c := &ctx{d: parent.Diagram, src: src, what: e.Describe()}
	if kind == model.KindGrid && cols < 1 {
		c.errorf("Grid with %d columns: want at least 1", cols)
		e.Cols = 1
	}
	for _, o := range opts {
		if o != nil {
			o.applyGroup(e, c)
		}
	}
	attach(parent, e, c)
	return e
}

// addLegend adds a legend: a Column, drawn even without a title, that
// layout fills with an entry per named style.
func addLegend(parent *model.Element, title string, src model.Src, place placement, opts []GroupOption) *model.Element {
	vis := groupOpt(func(e *model.Element, _ *ctx) {
		e.Legend = true
		e.Invisible = model.Some(false)
	})
	return addGroup(parent, model.KindColumn, title, 0, src, place, append([]GroupOption{vis}, opts...))
}

// attach validates a new child's placement and adds it to its parent.
func attach(parent, e *model.Element, c *ctx) {
	switch parent.Kind {
	case model.KindGrid:
		rs, cs := span(e)
		switch {
		case e.Row < 0 || e.Col < 0:
			c.errorf("cell (%d,%d): want non-negative row and column", e.Row, e.Col)
		case e.Col+cs > parent.Cols:
			c.errorf("cell (%d,%d) with span %d×%d outside %d-column grid", e.Row, e.Col, rs, cs, parent.Cols)
		default:
			for _, o := range parent.Children {
				ors, ocs := span(o)
				if e.Row < o.Row+ors && o.Row < e.Row+rs && e.Col < o.Col+ocs && o.Col < e.Col+cs {
					c.errorf("cell (%d,%d) already occupied by %s (%s)", e.Row, e.Col, o.Describe(), o.Src)
					break
				}
			}
		}
	case model.KindCanvas:
		if math.IsNaN(e.X) || math.IsNaN(e.Y) || math.IsInf(e.X, 0) || math.IsInf(e.Y, 0) {
			c.errorf("position (%v, %v): want finite coordinates", e.X, e.Y)
		}
	}
	parent.Children = append(parent.Children, e)
}

func span(e *model.Element) (rows, cols int) {
	rows, cols = e.RowSpan, e.ColSpan
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	return rows, cols
}

// ---- Row ----

// Node adds a node to the row.
func (r *Row) Node(label string, opts ...NodeOption) *Node {
	return addNode(r.el, label, model.Caller(1), flowPlace, opts)
}

// Row adds a nested row. An empty label makes it an invisible layout box.
func (r *Row) Row(label string, opts ...GroupOption) *Row {
	return &Row{addGroup(r.el, model.KindRow, label, 0, model.Caller(1), flowPlace, opts)}
}

// Column adds a nested column.
func (r *Row) Column(label string, opts ...GroupOption) *Column {
	return &Column{addGroup(r.el, model.KindColumn, label, 0, model.Caller(1), flowPlace, opts)}
}

// Grid adds a nested grid with cols columns.
func (r *Row) Grid(label string, cols int, opts ...GroupOption) *Grid {
	return &Grid{addGroup(r.el, model.KindGrid, label, cols, model.Caller(1), flowPlace, opts)}
}

// Canvas adds a nested canvas for absolute placement.
func (r *Row) Canvas(label string, opts ...GroupOption) *Canvas {
	return &Canvas{addGroup(r.el, model.KindCanvas, label, 0, model.Caller(1), flowPlace, opts)}
}

// Legend adds a legend box to the row. See Diagram.Legend.
func (r *Row) Legend(title string, opts ...GroupOption) *Column {
	return &Column{addLegend(r.el, title, model.Caller(1), flowPlace, opts)}
}

// ---- Column ----

// Node adds a node to the column.
func (c *Column) Node(label string, opts ...NodeOption) *Node {
	return addNode(c.el, label, model.Caller(1), flowPlace, opts)
}

// Row adds a nested row. An empty label makes it an invisible layout box.
func (c *Column) Row(label string, opts ...GroupOption) *Row {
	return &Row{addGroup(c.el, model.KindRow, label, 0, model.Caller(1), flowPlace, opts)}
}

// Column adds a nested column.
func (c *Column) Column(label string, opts ...GroupOption) *Column {
	return &Column{addGroup(c.el, model.KindColumn, label, 0, model.Caller(1), flowPlace, opts)}
}

// Grid adds a nested grid with cols columns.
func (c *Column) Grid(label string, cols int, opts ...GroupOption) *Grid {
	return &Grid{addGroup(c.el, model.KindGrid, label, cols, model.Caller(1), flowPlace, opts)}
}

// Canvas adds a nested canvas for absolute placement.
func (c *Column) Canvas(label string, opts ...GroupOption) *Canvas {
	return &Canvas{addGroup(c.el, model.KindCanvas, label, 0, model.Caller(1), flowPlace, opts)}
}

// Legend adds a legend box to the column. See Diagram.Legend.
func (c *Column) Legend(title string, opts ...GroupOption) *Column {
	return &Column{addLegend(c.el, title, model.Caller(1), flowPlace, opts)}
}

// ---- Grid ----

// Node adds a node in cell (row, col). Use Span to cover more cells.
func (g *Grid) Node(label string, row, col int, opts ...NodeOption) *Node {
	return addNode(g.el, label, model.Caller(1), cellPlace(row, col), opts)
}

// Row adds a nested row in cell (row, col).
func (g *Grid) Row(label string, row, col int, opts ...GroupOption) *Row {
	return &Row{addGroup(g.el, model.KindRow, label, 0, model.Caller(1), cellPlace(row, col), opts)}
}

// Column adds a nested column in cell (row, col).
func (g *Grid) Column(label string, row, col int, opts ...GroupOption) *Column {
	return &Column{addGroup(g.el, model.KindColumn, label, 0, model.Caller(1), cellPlace(row, col), opts)}
}

// Grid adds a nested grid with cols columns in cell (row, col).
func (g *Grid) Grid(label string, row, col, cols int, opts ...GroupOption) *Grid {
	return &Grid{addGroup(g.el, model.KindGrid, label, cols, model.Caller(1), cellPlace(row, col), opts)}
}

// Canvas adds a nested canvas in cell (row, col).
func (g *Grid) Canvas(label string, row, col int, opts ...GroupOption) *Canvas {
	return &Canvas{addGroup(g.el, model.KindCanvas, label, 0, model.Caller(1), cellPlace(row, col), opts)}
}

// Legend adds a legend box in cell (row, col). See Diagram.Legend.
func (g *Grid) Legend(title string, row, col int, opts ...GroupOption) *Column {
	return &Column{addLegend(g.el, title, model.Caller(1), cellPlace(row, col), opts)}
}

// ---- Canvas ----

// Node adds a node with its top-left corner at (x, y) relative to the
// canvas's content origin.
func (c *Canvas) Node(label string, x, y float64, opts ...NodeOption) *Node {
	return addNode(c.el, label, model.Caller(1), xyPlace(x, y), opts)
}

// Row adds a nested row at (x, y).
func (c *Canvas) Row(label string, x, y float64, opts ...GroupOption) *Row {
	return &Row{addGroup(c.el, model.KindRow, label, 0, model.Caller(1), xyPlace(x, y), opts)}
}

// Column adds a nested column at (x, y).
func (c *Canvas) Column(label string, x, y float64, opts ...GroupOption) *Column {
	return &Column{addGroup(c.el, model.KindColumn, label, 0, model.Caller(1), xyPlace(x, y), opts)}
}

// Grid adds a nested grid with cols columns at (x, y).
func (c *Canvas) Grid(label string, x, y float64, cols int, opts ...GroupOption) *Grid {
	return &Grid{addGroup(c.el, model.KindGrid, label, cols, model.Caller(1), xyPlace(x, y), opts)}
}

// Canvas adds a nested canvas at (x, y).
func (c *Canvas) Canvas(label string, x, y float64, opts ...GroupOption) *Canvas {
	return &Canvas{addGroup(c.el, model.KindCanvas, label, 0, model.Caller(1), xyPlace(x, y), opts)}
}

// Legend adds a legend box at (x, y). See Diagram.Legend.
func (c *Canvas) Legend(title string, x, y float64, opts ...GroupOption) *Column {
	return &Column{addLegend(c.el, title, model.Caller(1), xyPlace(x, y), opts)}
}
