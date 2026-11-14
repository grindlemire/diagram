// Package diagram draws system diagrams from Go code.
//
// A diagram is a tree of typed containers (Row, Column, Grid, Canvas) holding
// nodes, plus edges between any of them. The container type decides how its
// children are placed, so placing something wrongly fails to compile:
//
//	d := diagram.New("Payments")
//	web := d.Node("Web", diagram.Icon(generic.Browser))
//	vpc := d.Column("VPC")
//	api := vpc.Node("API", diagram.Desc("Go · 3 replicas"))
//	workers := vpc.Grid("Workers", 2)
//	w1 := workers.Node("Worker 1", 0, 0)
//	w2 := workers.Node("Worker 2", 0, 1)
//
//	d.Connect(web, api, diagram.Label("HTTPS"))
//	d.Connect(api, w1)
//	d.Connect(api, w2)
//
//	err := d.WriteFile("payments.svg")
//
// Builder methods never return errors. Mistakes that the compiler can't
// catch, such as two nodes in one grid cell, are recorded with the file and
// line of the call and returned together by Validate and every render
// method.
package diagram

import (
	"errors"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/pipeline"
)

// Diagram is a diagram under construction. Its top level lays children out
// left to right, or top to bottom with the Vertical option.
//
// A Diagram is not safe for concurrent use while it is being built.
type Diagram struct {
	m *model.Diagram
}

func init() {
	model.Of = func(d any) *model.Diagram { return d.(*Diagram).m }
}

// New starts a diagram.
func New(title string, opts ...DiagramOption) *Diagram {
	m := model.New(title)
	d := &Diagram{m: m}
	c := &ctx{d: m, src: model.Caller(1), what: "diagram"}
	for _, o := range opts {
		if o != nil {
			o.applyDiagram(m, c)
		}
	}
	return d
}

// Title returns the diagram's title.
func (d *Diagram) Title() string { return d.m.Title }

// Node adds a node at the top level.
func (d *Diagram) Node(label string, opts ...NodeOption) *Node {
	return addNode(d.m.Root, label, model.Caller(1), flowPlace, opts)
}

// Row adds a row at the top level. An empty label makes it an invisible
// layout box.
func (d *Diagram) Row(label string, opts ...GroupOption) *Row {
	return &Row{addGroup(d.m.Root, model.KindRow, label, 0, model.Caller(1), flowPlace, opts)}
}

// Column adds a column at the top level.
func (d *Diagram) Column(label string, opts ...GroupOption) *Column {
	return &Column{addGroup(d.m.Root, model.KindColumn, label, 0, model.Caller(1), flowPlace, opts)}
}

// Grid adds a grid with cols columns at the top level.
func (d *Diagram) Grid(label string, cols int, opts ...GroupOption) *Grid {
	return &Grid{addGroup(d.m.Root, model.KindGrid, label, cols, model.Caller(1), flowPlace, opts)}
}

// Canvas adds a canvas for absolute placement at the top level.
func (d *Diagram) Canvas(label string, opts ...GroupOption) *Canvas {
	return &Canvas{addGroup(d.m.Root, model.KindCanvas, label, 0, model.Caller(1), flowPlace, opts)}
}

// Edge is a connection between two endpoints.
type Edge struct{ e *model.Edge }

// Connect adds an edge from one endpoint to another. Either end can be a
// node or a container. Edges are routed orthogonally around other elements
// unless the Straight option is given.
func (d *Diagram) Connect(from, to Endpoint, opts ...EdgeOption) *Edge {
	return d.connect(model.Caller(1), "Connect", from, to, opts)
}

// connect adds an edge, recording errors at src. call names the builder
// method for errors that have no edge to name.
func (d *Diagram) connect(src model.Src, call string, from, to Endpoint, opts []EdgeOption) *Edge {
	e := &model.Edge{Src: src}
	c := &ctx{d: d.m, src: src}
	if isNil(from) || isNil(to) {
		c.what = call
		c.errorf("nil endpoint")
		return &Edge{e}
	}
	e.From, e.To = from.element(), to.element()
	c.what = e.Describe()
	for _, end := range []*model.Element{e.From, e.To} {
		if end.Diagram != d.m {
			c.errorf("%s belongs to a different diagram (%s)", end.Describe(), end.Src)
			return &Edge{e}
		}
		if end.Legend {
			c.errorf("%s is a legend, which can't be an edge endpoint", end.Describe())
			return &Edge{e}
		}
	}
	for _, o := range opts {
		if o != nil {
			o.applyEdge(e, c)
		}
	}
	d.m.Edges = append(d.m.Edges, e)
	return &Edge{e}
}

// isNil reports whether an endpoint is nil, including a typed nil handle
// such as a (*Node)(nil).
func isNil(ep Endpoint) bool {
	if ep == nil {
		return true
	}
	switch v := ep.(type) {
	case *Node:
		return v == nil || v.el == nil
	case *Row:
		return v == nil || v.el == nil
	case *Column:
		return v == nil || v.el == nil
	case *Grid:
		return v == nil || v.el == nil
	case *Canvas:
		return v == nil || v.el == nil
	}
	return false
}

// Edges is a set of edges made by one Path, Fan or FanIn call.
type Edges struct {
	d     *model.Diagram
	edges []*Edge
}

// Path connects its endpoints in a chain, a → b → c …, with one edge per
// consecutive pair. It needs at least two endpoints.
//
//	d.Path(web, api, db).With(diagram.Label("sync"))
func (d *Diagram) Path(endpoints ...Endpoint) *Edges {
	src := model.Caller(1)
	s := &Edges{d: d.m}
	if len(endpoints) < 2 {
		d.m.Errorf(src, "Path: want at least 2 endpoints, got %d", len(endpoints))
		return s
	}
	for i := 0; i+1 < len(endpoints); i++ {
		s.edges = append(s.edges, d.connect(src, "Path", endpoints[i], endpoints[i+1], nil))
	}
	return s
}

// Fan connects one endpoint to each of several others, in order.
//
//	d.Fan(api, ledger, payouts, audit)
func (d *Diagram) Fan(from Endpoint, to ...Endpoint) *Edges {
	src := model.Caller(1)
	s := &Edges{d: d.m}
	if len(to) == 0 {
		d.m.Errorf(src, "Fan: want at least one endpoint to connect to")
		return s
	}
	for _, t := range to {
		s.edges = append(s.edges, d.connect(src, "Fan", from, t, nil))
	}
	return s
}

// FanIn connects each of several endpoints to one, in order.
//
//	d.FanIn(bus, app, cdc, logs)
func (d *Diagram) FanIn(to Endpoint, from ...Endpoint) *Edges {
	src := model.Caller(1)
	s := &Edges{d: d.m}
	if len(from) == 0 {
		d.m.Errorf(src, "FanIn: want at least one endpoint to connect from")
		return s
	}
	for _, f := range from {
		s.edges = append(s.edges, d.connect(src, "FanIn", f, to, nil))
	}
	return s
}

// With applies options to every edge in the set and returns the set.
// Errors from the options are recorded at the With call.
func (s *Edges) With(opts ...EdgeOption) *Edges {
	src := model.Caller(1)
	for _, e := range s.edges {
		c := &ctx{d: s.d, src: src, what: e.e.Describe()}
		for _, o := range opts {
			if o != nil {
				o.applyEdge(e.e, c)
			}
		}
	}
	return s
}

// All returns the edges in the set, in the order they were made.
func (s *Edges) All() []*Edge {
	return append([]*Edge(nil), s.edges...)
}

// Legend adds a legend box at the top level. When the diagram is laid out,
// the legend fills itself with one entry for each named style (see Named)
// used anywhere in the diagram, in the order they were first used: a short
// line drawn in the style for styles used on edges, otherwise a small
// swatch box, with the style's name beside it. Children added to the
// returned Column come before the entries. A legend with no entries and no
// children draws nothing.
//
// A legend is drawn even with an empty title, takes its natural size
// rather than stretching to fill its parent, and can't be an edge
// endpoint. Legend is also available on every container.
func (d *Diagram) Legend(title string, opts ...GroupOption) *Column {
	return &Column{addLegend(d.m.Root, title, model.Caller(1), flowPlace, opts)}
}

// Validate returns every error recorded while building the diagram, plus
// text that no font can draw (see RegisterFont) and problems layout finds
// (such as overlapping Canvas children), joined with errors.Join, or nil.
// Rendering also routes edges, which can find further problems.
func (d *Diagram) Validate() error {
	_, lerr := layout.Layout(d.m, d.m.Theme)
	return errors.Join(append(append(append([]error{}, d.m.Errs()...), pipeline.Glyphs(d.m)...), lerr)...)
}
