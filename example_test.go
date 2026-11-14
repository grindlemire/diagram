package diagram_test

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/grindlemire/diagram"
	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/icons/brands"
	"github.com/grindlemire/diagram/icons/generic"
	"github.com/grindlemire/diagram/themes"
)

func Example() {
	d := diagram.New("Payments")

	web := d.Node("Web app", diagram.Icon(generic.Browser))
	vpc := d.Column("Production VPC", diagram.Desc("us-east-1"))
	api := vpc.Node("API", diagram.Desc("Go · 3 replicas"), diagram.Icon(brands.Go), diagram.IconLeft)
	workers := vpc.Grid("Workers", 2, diagram.Uniform)
	w1 := workers.Node("Ledger", 0, 0)
	w2 := workers.Node("Payouts", 0, 1)
	db := d.Node("Postgres", diagram.Cylinder)

	critical := diagram.Style(diagram.Stroke(diagram.Red), diagram.StrokeWidth(2))
	d.Connect(web, api, diagram.Label("HTTPS"))
	d.Connect(api, w1)
	d.Connect(api, w2)
	d.Connect(w1, db, critical, diagram.Label("writes"))

	if err := d.RenderSVG(io.Discard); err != nil {
		fmt.Println(err)
	}
	// Output:
}

// Builder methods never return errors. Mistakes are collected with the file
// and line of the call that made them, and returned by Validate and by
// every render method.
func ExampleDiagram_Validate() {
	d := diagram.New("t")
	g := d.Grid("Workers", 2)
	g.Node("A", 0, 0)
	g.Node("B", 0, 0) // same cell

	fmt.Println(d.Validate() != nil)
	// Output: true
}

// Styles are values. Bundle options with Style and reuse them.
func ExampleStyle() {
	async := diagram.Style(diagram.Dashed, diagram.Stroke(diagram.Muted))
	critical := diagram.Style(diagram.Stroke(diagram.Red), diagram.StrokeWidth(2))

	d := diagram.New("Events")
	api, q, db := d.Node("API"), d.Node("Queue", diagram.Queue), d.Node("DB", diagram.Cylinder)
	d.Connect(api, q, async, diagram.Label("publish"))
	d.Connect(api, db, critical)
	fmt.Println(d.Validate())
	// Output: <nil>
}

// Grids place children in cells; Span covers several.
func ExampleGrid() {
	d := diagram.New("Cluster")
	g := d.Grid("Nodes", 3, diagram.Uniform)
	g.Node("Control plane", 0, 0, diagram.Span(1, 3))
	for i := range 3 {
		g.Node(fmt.Sprintf("Worker %d", i+1), 1, i)
	}
	fmt.Println(d.Validate())
	// Output: <nil>
}

// A Canvas places children at explicit coordinates, as an escape hatch
// inside otherwise automatic flow layout.
func ExampleCanvas() {
	d := diagram.New("Legacy")
	c := d.Canvas("Datacenter")
	c.Node("Mainframe", 0, 0)
	c.Node("Tape library", 180, 60, diagram.Document)
	fmt.Println(d.Validate())
	// Output: <nil>
}

// Edges can pin the side they leave or enter, and pass through waypoints.
func ExampleExit() {
	d := diagram.New("Ports")
	a, b := d.Node("A"), d.Node("B")
	d.Connect(a, b, diagram.Exit(diagram.Bottom), diagram.Enter(diagram.Bottom), diagram.Via(diagram.Pt(150, 120)))
	fmt.Println(d.Validate())
	// Output: <nil>
}

// Your own SVG or PNG files work as icons at any size.
func ExampleIcon() {
	logo, err := icons.FromSVG(bytesReader(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="currentColor"/></svg>`))
	if err != nil {
		fmt.Println(err)
		return
	}
	d := diagram.New("Custom icon")
	d.Node("Our service", diagram.Icon(logo), diagram.IconSize(48))
	fmt.Println(d.Validate())
	// Output: <nil>
}

// themes.Auto makes SVG output follow the viewer's light or dark mode.
func ExampleTheme() {
	d := diagram.New("Docs", diagram.Theme(themes.Auto(themes.Light, themes.Dark)))
	d.Node("Follows prefers-color-scheme")
	_ = d.RenderSVG(os.Stdout)
}

func bytesReader(s string) io.Reader { return strings.NewReader(s) }
