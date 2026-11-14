package examples

import (
	"fmt"
	"math/rand/v2"

	"github.com/grindlemire/diagram"
	"github.com/grindlemire/diagram/icons/generic"
)

var stress = []Example{
	{
		Name: "mesh", Title: "Dense mesh", Kind: Stress,
		Notes: "A 4×3 grid of services with 21 edges at the default 40px gap. Most edges route cleanly; look at where several edges share a channel and at the crossings that remain. The receipt label sits slightly over Billing's and Email's borders because it is wider than the gap between them (labels may spill over their own end nodes, never their text).",
		Build: mesh,
	},
	{
		Name: "tight-gaps", Title: "Tight gaps", Kind: Stress,
		Notes: "A mesh written with Gap(14), narrower than an edge and its clearance. By default the spacing floor raises every gap edges use to room for one edge (2×EdgeClearance + ParallelOffset), and gaps still too crowded after routing are widened again, so routes keep clear of nodes and labels sit on their lines. Compare with tight-gaps-exact for the gaps as written.",
		Build: func() *diagram.Diagram { return tightGaps() },
	},
	{
		Name: "hub", Title: "Hub with many spokes", Kind: Stress,
		Notes: "One hub connected to 16 nodes. Ports spread along each side at PortSpacing; the Queue's right side still collects several arrowheads close together, and a few routes cross on the way in.",
		Build: hub,
	},
	{
		Name: "detours", Title: "Forced detours", Kind: Stress,
		Notes: "Nodes on either side of a wide group that holds neither of them. Edges may not enter that group, so they route around it. Add diagram.CrossGroups to an edge to let it cut through instead.",
		Build: detours,
	},
	{
		Name: "self-loops", Title: "Self-loops", Kind: Stress,
		Notes: "Self-loops on small nodes, with labels, in a row with a 24px gap. Layout reserves space for each node's loops (on the top here, since the sides face neighbours), so loops and labels never touch neighbours, and the nodes stay in line.",
		Build: selfLoops,
	},
	{
		Name: "text", Title: "Text edge cases", Kind: Stress,
		Notes: "An unbreakable URL wider than MaxWidth (the node grows to fit it rather than breaking mid-word), a long description, explicit newlines, accents, symbols, and CJK. Inter has no CJK glyphs, so the examples package registers a Noto Sans SC subset with RegisterFont: layout measures CJK with it, the SVG embeds it as a second font family, and the PNG draws its outlines, so SVG and PNG should match and the text should fit its boxes. SemiBold labels use the SemiBold Noto file. draw.io can't embed fonts and falls back to the viewer's system CJK font.",
		Build: text,
	},
	{
		Name: "canvas-overlap", Title: "Canvas overlaps", Kind: Stress,
		Notes: "Overlapping Canvas children are a collected error unless the canvas has AllowOverlap, which this one does: Overlaps B and B are stacked on purpose. Negative coordinates shift the canvas origin. Edges must route through the crowd. The errors example shows the overlap error without AllowOverlap.",
		Build: canvasOverlap,
	},
	{
		Name: "contradictions", Title: "Awkward pins and straight edges", Kind: Stress,
		Notes: "Exit pins that point away from the target, waypoints behind the source, and Straight edges, which ignore obstacles by design and cut through nodes. Valid input, ugly output; the router follows the pins and the canvas grows to fit routes outside the layout.",
		Build: contradictions,
	},
	{
		Name: "uniform-shapes", Title: "Shapes under Uniform", Kind: Stress,
		Notes: "Uniform gives every node the same slot. Boxes, pills and queues grow to fill it; Circle stays round, and Person and IconOnly figures keep their size, centred in the slot.",
		Build: uniformShapes,
	},
	{
		Name: "large", Title: "Large diagram (60 nodes, 90 edges)", Kind: Stress,
		Notes: "Randomly wired (seeded, so stable) to show routing time and clutter at scale. By default the channels between and inside the teams are widened until the edges running along them fit side by side (about 0.45s per render, two layouts and routings), so few edges share a line. Compare with large-exact for the written spacing.",
		Build: func() *diagram.Diagram { return large() },
	},
	{
		Name: "errors", Title: "Collected errors", Kind: Stress,
		Notes: "A deliberately broken diagram. Nothing renders; the gallery shows the joined error messages with file:line positions. Check they're clear and point at the right lines.",
		Build: broken,
	},
	{
		Name: "drawio-parity", Title: "draw.io parity", Kind: Stress,
		Notes: "Features where draw.io output is known to differ slightly from SVG: multi-line labels (draw.io's line height is tighter), edge label backgrounds (rectangles, not pills), cylinder and document curves, IconOnly and Person labels. Compare the draw.io column with the SVG column.",
		Build: drawioParity,
	},
	{
		Name: "tight-gaps-exact", Title: "Tight gaps, exact spacing", Kind: Stress,
		Notes: "The tight-gaps diagram with diagram.ExactSpacing: what you get with ExactSpacing: the gaps you wrote, even when edges don't fit. Routes squeeze through 14px channels, labels spill over their nodes, and some edges go around the outside.",
		Build: func() *diagram.Diagram { return tightGaps(diagram.ExactSpacing) },
	},
	{
		Name: "large-exact", Title: "Large diagram, exact spacing", Kind: Stress,
		Notes: "The large diagram with diagram.ExactSpacing: the written gaps (24px inside teams, 40px between them) with no widening. Compare with large: crowded channels make parallel edges share lines.",
		Build: func() *diagram.Diagram { return large(diagram.ExactSpacing) },
	},
}

func mesh() *diagram.Diagram {
	d := diagram.New("Service mesh", diagram.Vertical)

	edge := d.Row("")
	lb := edge.Node("Load balancer", diagram.Pill)
	cdn := edge.Node("CDN", diagram.Pill)

	svc := d.Grid("Services", 4, diagram.Uniform)
	names := [][]string{
		{"Auth", "Users", "Orders", "Catalog"},
		{"Search", "Cart", "Payments", "Shipping"},
		{"Email", "Billing", "Reports", "Inventory"},
	}
	n := map[string]*diagram.Node{}
	for r, row := range names {
		for c, name := range row {
			n[name] = svc.Node(name, r, c)
		}
	}

	stores := d.Row("Data")
	pg := stores.Node("Postgres", diagram.Cylinder)
	redis := stores.Node("Redis", diagram.Cylinder)
	kafka := stores.Node("Kafka", diagram.Queue)

	d.Connect(lb, n["Auth"])
	d.Connect(lb, n["Orders"])
	d.Connect(lb, n["Search"], diagram.Label("/search"))
	d.Connect(cdn, n["Catalog"])
	d.Connect(n["Orders"], n["Payments"], diagram.Label("charge"))
	d.Connect(n["Orders"], n["Cart"])
	d.Connect(n["Orders"], n["Inventory"])
	d.Connect(n["Orders"], n["Shipping"])
	d.Connect(n["Cart"], n["Catalog"])
	d.Connect(n["Auth"], n["Users"])
	d.Connect(n["Payments"], n["Billing"])
	d.Connect(n["Billing"], n["Email"], diagram.Label("receipt"))
	d.Connect(n["Reports"], n["Billing"], diagram.Dashed)
	d.Connect(n["Search"], n["Catalog"], diagram.Dashed)
	d.Connect(n["Shipping"], n["Email"], diagram.Dotted)
	d.Connect(n["Users"], pg)
	d.Connect(n["Payments"], pg)
	d.Connect(n["Cart"], redis)
	d.Connect(n["Inventory"], kafka)
	d.Connect(n["Reports"], kafka, diagram.Label("consume"))
	d.Connect(n["Payments"], kafka)
	return d
}

func tightGaps(opts ...diagram.DiagramOption) *diagram.Diagram {
	d := diagram.New("Tight gaps", opts...)
	g := d.Grid("", 4, diagram.Gap(14))
	var n []*diagram.Node
	for i := range 12 {
		n = append(n, g.Node(fmt.Sprintf("S%d", i+1), i/4, i%4))
	}
	pairs := [][2]int{{0, 5}, {1, 4}, {2, 7}, {3, 6}, {4, 9}, {5, 8}, {6, 11}, {7, 10}, {0, 10}, {3, 8}, {1, 2}, {9, 10}, {5, 6}, {0, 1}}
	for i, p := range pairs {
		opts := []diagram.EdgeOption{}
		if i%4 == 0 {
			opts = append(opts, diagram.Label(fmt.Sprintf("e%d", i)))
		}
		d.Connect(n[p[0]], n[p[1]], opts...)
	}
	return d
}

func hub() *diagram.Diagram {
	d := diagram.New("Hub")
	g := d.Grid("", 5, diagram.Uniform, diagram.Gap(28))
	center := g.Node("Message bus", 2, 2, diagram.Queue)
	k := 0
	for r := range 5 {
		for c := range 5 {
			if r == 2 && c == 2 || (r == 1 || r == 3) && (c == 1 || c == 3) {
				continue
			}
			if r > 0 && r < 4 && c > 0 && c < 4 {
				continue
			}
			k++
			n := g.Node(fmt.Sprintf("svc-%02d", k), r, c)
			if k%2 == 0 {
				d.Connect(center, n)
			} else {
				d.Connect(n, center, diagram.Dashed)
			}
		}
	}
	return d
}

func detours() *diagram.Diagram {
	d := diagram.New("Detours", diagram.Vertical)
	top := d.Row("")
	a := top.Node("Left client")
	wall := top.Grid("Platform", 3)
	for i := range 9 {
		wall.Node(fmt.Sprintf("svc %d", i+1), i/3, i%3)
	}
	b := top.Node("Right client")
	bottom := d.Row("")
	c := bottom.Node("Batch job")
	e := bottom.Node("Archive", diagram.Cylinder)
	d.Connect(a, b, diagram.Label("peer sync"))
	d.Connect(b, a, diagram.Dashed)
	d.Connect(a, e)
	d.Connect(c, b)
	return d
}

func selfLoops() *diagram.Diagram {
	d := diagram.New("Self-loops")
	row := d.Row("", diagram.Gap(24))
	a := row.Node("A")
	b := row.Node("Retry", diagram.Pill)
	c := row.Node("Poller", diagram.Circle)
	tiny := row.Node("x", diagram.Size(40, 28))
	d.Connect(a, a)
	d.Connect(b, b, diagram.Label("backoff ×3"))
	d.Connect(c, c, diagram.Exit(diagram.Top), diagram.Label("every 5s"))
	d.Connect(tiny, tiny, diagram.Label("a longer loop label"))
	d.Connect(a, b)
	d.Connect(b, c)
	return d
}

func text() *diagram.Diagram {
	d := diagram.New("Text")
	col := d.Column("", diagram.Gap(24))
	url := col.Node("https://api.example.com/v2/organisations/billing/invoices", diagram.MaxWidth(140))
	desc := col.Node("Ledger", diagram.Desc("Double-entry ledger with idempotent writes, nightly reconciliation against the bank feed, and a 7-year retention policy"))
	nl := col.Node("Line one\nLine two\nLine three")
	col2 := d.Column("", diagram.Gap(24))
	acc := col2.Node("Café · Zürich · São Paulo", diagram.Desc("Ångström, naïve, façade"))
	cjk := col2.Node("支付服务 → Payments", diagram.Desc("日本語のテキスト"))
	sym := col2.Node("Deploy ✓ / rollback ✗")
	d.Connect(url, acc, diagram.Label("a long edge label that will likely collide with something"))
	d.Connect(desc, cjk, diagram.Label("数据"))
	d.Connect(nl, sym, diagram.Label("short"))
	return d
}

func canvasOverlap() *diagram.Diagram {
	d := diagram.New("Canvas overlap")
	in := d.Node("Client")
	c := d.Canvas("Hand placed", diagram.AllowOverlap)
	a := c.Node("Overlaps B", 0, 0)
	b := c.Node("B", 60, 20)
	n := c.Node("Negative", -80, 100)
	e := c.Node("Far", 260, 160)
	d.Connect(in, a)
	d.Connect(in, e)
	d.Connect(n, e)
	d.Connect(b, n)
	return d
}

func contradictions() *diagram.Diagram {
	d := diagram.New("Contradictions")
	c := d.Canvas("")
	a := c.Node("Source", 0, 100)
	b := c.Node("Target", 320, 100)
	m := c.Node("Middle", 160, 100)
	t := c.Node("Top", 160, 0)
	d.Connect(a, b, diagram.Exit(diagram.Left), diagram.Enter(diagram.Right), diagram.Label("backwards pins"))
	d.Connect(a, b, diagram.Via(diagram.Pt(-60, 260), diagram.Pt(400, 260)), diagram.Label("via behind"))
	d.Connect(a, b, diagram.Straight, diagram.Stroke(diagram.Red), diagram.Label("straight"))
	d.Connect(t, m, diagram.Exit(diagram.Top), diagram.Enter(diagram.Bottom))
	return d
}

func uniformShapes() *diagram.Diagram {
	d := diagram.New("Uniform shapes")
	r := d.Row("Uniform row", diagram.Uniform)
	r.Node("Circle", diagram.Circle)
	r.Node("Pill", diagram.Pill)
	r.Node("A box with a much longer label", diagram.Desc("sets the size"))
	r.Node("Person", diagram.Person)
	r.Node("Icon only", diagram.Icon(generic.Server), diagram.IconOnly)
	r.Node("Queue", diagram.Queue)
	return d
}

func large(opts ...diagram.DiagramOption) *diagram.Diagram {
	d := diagram.New("Large", append([]diagram.DiagramOption{diagram.Vertical}, opts...)...)
	rows := []*diagram.Row{d.Row(""), d.Row("")}
	rng := rand.New(rand.NewPCG(1, 2))
	var nodes []*diagram.Node
	for gi := range 6 {
		g := rows[gi/3].Grid(fmt.Sprintf("Team %d", gi+1), 3, diagram.Gap(24))
		for i := range 10 {
			nodes = append(nodes, g.Node(fmt.Sprintf("t%d-svc%d", gi+1, i+1), i/3, i%3))
		}
	}
	for i := range 90 {
		a, b := rng.IntN(len(nodes)), rng.IntN(len(nodes))
		if a == b {
			continue
		}
		var opts []diagram.EdgeOption
		if i%10 == 0 {
			opts = append(opts, diagram.Label(fmt.Sprintf("e%d", i)))
		}
		d.Connect(nodes[a], nodes[b], opts...)
	}
	return d
}

func broken() *diagram.Diagram {
	d := diagram.New("Broken")
	g := d.Grid("Workers", 2)
	g.Node("A", 0, 0)
	g.Node("B", 0, 0)                              // occupied cell
	g.Node("C", 0, 1, diagram.Span(1, 2))          // outside the grid
	d.Node("D", diagram.Fill(diagram.Hex("#12g"))) // bad colour
	d.Node("E", diagram.ID("dup"))
	d.Node("F", diagram.ID("dup"))  // duplicate ID
	d.Node("G", diagram.Span(2, 1)) // Span outside a grid
	other := diagram.New("Other").Node("Elsewhere")
	d.Connect(d.Node("H"), other) // edge to another diagram
	c := d.Canvas("Hand placed")
	c.Node("A", 0, 0)
	c.Node("B", 40, 20) // overlaps A (found at layout time)
	d.Node("Launch 🚀")  // no registered font has the emoji
	return d
}

func drawioParity() *diagram.Diagram {
	d := diagram.New("draw.io parity")
	a := d.Node("A label that wraps onto two or three lines", diagram.Desc("plus a description"), diagram.MaxWidth(120))
	col := d.Column("Shapes")
	db := col.Node("Cylinder", diagram.Cylinder, diagram.Desc("rim curve"))
	doc := col.Node("Document", diagram.Document, diagram.Desc("wave"))
	p := d.Node("Person label", diagram.Person)
	ic := d.Node("Icon only", diagram.Icon(generic.Database), diagram.IconOnly, diagram.IconSize(40))
	d.Connect(a, db, diagram.Label("label pill"))
	d.Connect(db, doc, diagram.Label("another"))
	d.Connect(doc, p, diagram.Dashed)
	d.Connect(p, ic, diagram.Head(diagram.Diamond), diagram.Tail(diagram.Dot))
	return d
}
