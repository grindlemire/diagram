package examples

import (
	"github.com/grindlemire/diagram"
	"github.com/grindlemire/diagram/icons/brands"
	"github.com/grindlemire/diagram/icons/generic"
	"github.com/grindlemire/diagram/icons/network"
	"github.com/grindlemire/diagram/themes"
)

var showcase = []Example{
	{
		Name: "payments", Title: "Payments", Kind: Showcase,
		Notes: "The README example: icons in every placement, a grid with Uniform sizing, a Person and a Note, labels, and named styles (critical path and async edges, managed services) listed by a legend. Badges mark the ledger worker's replica count and the beta web app.",
		Build: payments,
	},
	{
		Name: "kubernetes", Title: "Kubernetes platform", Kind: Showcase,
		Notes: "Brand icons with IconLeft cards inside nested namespaces, edges into a group (Ingress → apps namespace and GitHub → cluster), and an observability column whose nodes start under its header (Justify defaults to start). Status dots show each workload's health (green, amber, red) and a badge marks graphql as beta; the request chain is one Path and the API's and Grafana's fan-outs are Fan calls.",
		Build: kubernetes,
	},
	{
		Name: "pipeline", Title: "Data pipeline", Kind: Showcase,
		Notes: "A left-to-right pipeline: sources stacked in an invisible column, queue and cylinder shapes, a dead-letter path with a pinned exit, and edge labels.",
		Build: dataPipeline,
	},
	{
		Name: "cicd", Title: "CI/CD", Kind: Showcase,
		Notes: "A vertical diagram (diagram.Vertical) with a Person, a Note annotation, Pill stages and a manual approval gate pinned with Exit/Enter.",
		Build: cicd,
	},
	{
		Name: "network", Title: "Network topology", Kind: Showcase,
		Notes: "Network icons, IconOnly nodes, HeaderTop group headers, and depth shading three levels deep (VPC → subnet → tier). The web → app edges still take a longer route than a person would draw.",
		Build: networkTopology,
	},
	{
		Name: "c4-context", Title: "C4 system context", Kind: Showcase,
		Notes: "A C4-style context diagram: people, the system in focus with an accent border, dashed external systems, and descriptions on every element. Labels on the short edges sit on their lines and overlap their own end nodes' borders slightly; the pinned emails edge runs under the Support agent's text, not through it.",
		Build: c4Context,
	},
	{
		Name: "multi-region", Title: "Multi-region", Kind: Showcase,
		Notes: "Nested regions and subnets, edges between groups, a parallel edge pair (replication), edges to whole groups (scrape), and two self-loops.",
		Build: multiRegion,
	},
	{
		Name: "pinned", Title: "Pinned ports and waypoints", Kind: Showcase,
		Notes: "A Canvas with hand-placed nodes; Exit/Enter pins, a two-waypoint edge (bad), LabelAt start and end, and one Straight edge (Enrich → Audit), which ignores obstacles by design.",
		Build: pinned,
	},
	{
		Name: "theme-light", Title: "Theme: Light", Kind: Showcase,
		Notes: "One diagram in each theme. Light is the default.",
		Build: func() *diagram.Diagram { return themed(themes.Light) },
	},
	{
		Name: "theme-dark", Title: "Theme: Dark", Kind: Showcase,
		Notes: "themes.Dark. Literal colours (the green fill) stay the same in every theme; token colours (Red, Accent) follow the theme.",
		Build: func() *diagram.Diagram { return themed(themes.Dark) },
	},
	{
		Name: "theme-blueprint", Title: "Theme: Blueprint", Kind: Showcase,
		Notes: "themes.Blueprint.",
		Build: func() *diagram.Diagram { return themed(themes.Blueprint) },
	},
	{
		Name: "theme-auto", Title: "Theme: Auto", Kind: Showcase,
		Notes: "themes.Auto(Light, Dark). The SVG switches palettes with your OS dark-mode setting (toggle it to check). PNG and draw.io always use the light palette.",
		Build: func() *diagram.Diagram { return themed(themes.Auto(themes.Light, themes.Dark)) },
	},
	{
		Name: "icons", Title: "Bundled icons", Kind: Showcase,
		Notes: "Every bundled icon, as IconOnly nodes in grids. Monochrome icons take the theme's icon colour; brands are drawn in their brand colours with IconColor, which recolours only the icon, never a node's border.",
		Build: iconCatalog,
	},
}

func payments() *diagram.Diagram {
	d := diagram.New("Payments")

	user := d.Node("Customer", diagram.Person)
	web := d.Node("Web app", diagram.Desc("Next.js"), diagram.Icon(generic.Browser), diagram.Badge("beta"))
	vpc := d.Column("Production VPC", diagram.Desc("us-east-1"), diagram.Icon(generic.Cloud))
	api := vpc.Node("API", diagram.Desc("Go · 3 replicas"), diagram.Icon(brands.Go), diagram.IconLeft,
		diagram.Link("https://example.com/runbooks/api"))
	workers := vpc.Grid("Workers", 2, diagram.Uniform)
	w1 := workers.Node("Ledger worker", 0, 0, diagram.Icon(generic.Cog), diagram.IconLeft, diagram.IconSize(20), diagram.Badge("×3"))
	w2 := workers.Node("Payout worker", 0, 1, diagram.Icon(generic.Cog), diagram.IconLeft, diagram.IconSize(20))
	vpc.Node("Workers scale on queue depth", diagram.Note, diagram.MaxWidth(140))
	critical := diagram.Named("critical path", diagram.Stroke(diagram.Red), diagram.StrokeWidth(2))
	async := diagram.Named("async", diagram.Dashed)
	managed := diagram.Named("managed service", diagram.Stroke(diagram.Teal), diagram.Dashed)

	data := d.Column("")
	db := data.Node("Postgres", diagram.Cylinder, diagram.Desc("primary"), managed)
	q := data.Node("Events", diagram.Queue, managed)
	d.Legend("Legend")

	d.Connect(user, web)
	d.Connect(web, api, diagram.Label("HTTPS"), diagram.Tooltip("TLS 1.3 via the load balancer; p99 latency 120 ms"))
	d.Connect(api, w1)
	d.Connect(api, w2, async)
	d.Connect(w1, db, critical, diagram.Label("writes"), diagram.Tooltip("Ledger writes are synchronous; a failure here blocks payments"))
	d.Connect(w2, q, async)
	return d
}

func kubernetes() *diagram.Diagram {
	d := diagram.New("Kubernetes platform")
	card := func(c *diagram.Column, label, desc string, opts ...diagram.NodeOption) *diagram.Node {
		return c.Node(label, append([]diagram.NodeOption{diagram.Desc(desc), diagram.IconLeft, diagram.IconSize(24)}, opts...)...)
	}

	edge := d.Column("")
	cf := edge.Node("Cloudflare", diagram.Icon(brands.Cloudflare), diagram.IconLeft, diagram.Desc("CDN · WAF"))
	gh := edge.Node("GitHub", diagram.Icon(brands.GitHub), diagram.IconLeft, diagram.Desc("Actions"))

	cluster := d.Column("Production cluster", diagram.Icon(brands.Kubernetes), diagram.Desc("EKS · 12 nodes"),
		diagram.Link("https://example.com/runbooks/cluster"))
	ingress := cluster.Node("Ingress", diagram.Icon(brands.Nginx), diagram.IconLeft, diagram.Desc("nginx"))
	nss := cluster.Row("")
	apps := nss.Column("namespace: apps", diagram.Uniform)
	healthy, degraded, down := diagram.Status(diagram.Green), diagram.Status(diagram.Yellow), diagram.Status(diagram.Red)
	web := card(apps, "storefront", "React · 4 pods", diagram.Icon(brands.React), healthy)
	api := card(apps, "api", "Go · 6 pods", diagram.Icon(brands.Go), healthy)
	gql := card(apps, "graphql", "Node · 3 pods", diagram.Icon(brands.GraphQL), degraded, diagram.Badge("beta"))
	data := nss.Column("namespace: data", diagram.Uniform)
	pg := card(data, "postgres", "primary + 2 replicas", diagram.Icon(brands.Postgres), healthy)
	redis := card(data, "redis", "sessions, cache", diagram.Icon(brands.Redis),
		diagram.Tooltip("Evictions alert above 5% of keys per hour"), degraded)
	kafka := card(data, "kafka", "3 brokers", diagram.Icon(brands.Kafka), down)

	obs := d.Column("Observability")
	prom := card(obs, "Prometheus", "metrics", diagram.Icon(brands.Prometheus))
	graf := card(obs, "Grafana", "dashboards", diagram.Icon(brands.Grafana))
	es := card(obs, "Elasticsearch", "logs", diagram.Icon(brands.Elasticsearch))

	d.Connect(cf, ingress, diagram.Label("HTTPS"))
	d.Connect(gh, cluster, diagram.Dashed, diagram.Label("deploy"))
	d.Connect(ingress, apps)
	d.Path(web, gql, api)
	d.Fan(api, pg, redis)
	d.Connect(api, kafka, diagram.Dashed)
	d.Connect(prom, cluster, diagram.Dotted, diagram.Label("scrape"))
	d.Fan(graf, prom, es)
	return d
}

func dataPipeline() *diagram.Diagram {
	d := diagram.New("Data pipeline")

	src := d.Column("", diagram.Uniform, diagram.Gap(16))
	app := src.Node("App events", diagram.Icon(generic.Smartphone), diagram.IconLeft)
	cdc := src.Node("Postgres CDC", diagram.Icon(brands.Postgres), diagram.IconLeft)
	logs := src.Node("Access logs", diagram.Icon(generic.FileText), diagram.IconLeft)

	bus := d.Node("Kafka", diagram.Queue, diagram.Desc("raw.* topics"))

	proc := d.Column("Stream processing")
	clean := proc.Node("Clean", diagram.Desc("schema check"))
	enrich := proc.Node("Enrich", diagram.Desc("geo, user"))
	dlq := d.Node("Dead letters", diagram.Queue)

	store := d.Column("")
	lake := store.Node("Lake", diagram.Cylinder, diagram.Desc("S3 · parquet"))
	wh := store.Node("Warehouse", diagram.Cylinder, diagram.Desc("columnar"))
	bi := store.Node("Dashboards", diagram.Icon(generic.BarChart), diagram.IconLeft)

	d.Connect(app, bus)
	d.Connect(cdc, bus)
	d.Connect(logs, bus)
	d.Connect(bus, clean, diagram.Label("consume"))
	d.Connect(clean, enrich)
	d.Connect(clean, dlq, diagram.Exit(diagram.Bottom), diagram.Label("invalid"), diagram.Stroke(diagram.Red), diagram.Dashed)
	d.Connect(enrich, lake)
	d.Connect(lake, wh, diagram.Label("hourly"))
	d.Connect(wh, bi)
	return d
}

func cicd() *diagram.Diagram {
	d := diagram.New("CI/CD", diagram.Vertical)

	top := d.Row("")
	dev := top.Node("Developer", diagram.Person)
	repo := top.Node("Repository", diagram.Icon(brands.GitHub), diagram.IconLeft, diagram.Desc("main"))

	ci := d.Row("CI", diagram.Icon(generic.Workflow))
	build := ci.Node("Build", diagram.Pill)
	test := ci.Node("Test", diagram.Pill)
	scan := ci.Node("Scan", diagram.Pill)
	ci.Node("Runs on every push; ~6 min", diagram.Note, diagram.MaxWidth(120))

	cd := d.Row("CD", diagram.Icon(generic.CloudUpload))
	staging := cd.Node("Staging", diagram.Desc("auto"))
	approve := cd.Node("Approval", diagram.Person)
	prod := cd.Node("Production", diagram.Desc("canary 10% → 100%"), diagram.Stroke(diagram.Green), diagram.StrokeWidth(2))

	d.Connect(dev, repo, diagram.Label("push"))
	d.Connect(repo, build)
	d.Connect(build, test)
	d.Connect(test, scan)
	d.Connect(scan, staging, diagram.Label("artifact"))
	d.Connect(staging, approve)
	d.Connect(approve, prod, diagram.Exit(diagram.Right), diagram.Enter(diagram.Left))
	d.Connect(prod, staging, diagram.Exit(diagram.Bottom), diagram.Enter(diagram.Bottom), diagram.Dashed, diagram.Label("rollback"))
	return d
}

func networkTopology() *diagram.Diagram {
	d := diagram.New("Network")

	inet := d.Node("Internet", diagram.Icon(network.Internet), diagram.IconOnly, diagram.IconSize(48))
	vpc := d.Column("VPC 10.0.0.0/16", diagram.Icon(network.Network), diagram.HeaderTop)
	fw := vpc.Node("Firewall", diagram.Icon(network.Firewall), diagram.IconLeft)
	lb := vpc.Node("Load balancer", diagram.Icon(network.LoadBalancer), diagram.IconLeft)
	subs := vpc.Row("")
	pub := subs.Column("Public 10.0.1.0/24")
	web := pub.Row("Web tier", diagram.Gap(16))
	w1 := web.Node("web-1", diagram.Icon(generic.Server), diagram.IconSize(24))
	w2 := web.Node("web-2", diagram.Icon(generic.Server), diagram.IconSize(24))
	priv := subs.Column("Private 10.0.2.0/24")
	app := priv.Row("App tier", diagram.Gap(16))
	a1 := app.Node("app-1", diagram.Icon(network.ServerCog), diagram.IconSize(24))
	a2 := app.Node("app-2", diagram.Icon(network.ServerCog), diagram.IconSize(24))
	db := priv.Node("db", diagram.Icon(generic.Database), diagram.IconLeft)
	vpn := d.Node("Office VPN", diagram.Icon(network.Router), diagram.IconOnly, diagram.IconSize(48))

	d.Connect(inet, fw)
	d.Connect(fw, lb)
	d.Connect(lb, w1)
	d.Connect(lb, w2)
	d.Connect(w1, a1)
	d.Connect(w2, a2)
	d.Connect(a1, db)
	d.Connect(a2, db)
	d.Connect(vpn, priv, diagram.Dashed, diagram.Label("IPsec"))
	return d
}

func c4Context() *diagram.Diagram {
	d := diagram.New("C4 context")
	ext := diagram.Style(diagram.Dashed, diagram.Fill(diagram.Hex("#f8fafc")))

	people := d.Column("", diagram.Gap(60))
	cust := people.Node("Customer", diagram.Person, diagram.Desc("Buys things"))
	staff := people.Node("Support agent", diagram.Person, diagram.Desc("Handles refunds"))

	sys := d.Node("Online store", diagram.Desc("Lets customers browse, buy and track orders"),
		diagram.Rect, diagram.Stroke(diagram.Accent), diagram.StrokeWidth(2), diagram.Size(220, 120))

	exts := d.Column("", diagram.Uniform, diagram.Gap(24))
	pay := exts.Node("Stripe", diagram.Desc("Card payments"), diagram.Rect, ext, diagram.Icon(brands.Stripe), diagram.IconLeft)
	mail := exts.Node("Email service", diagram.Desc("Order emails"), diagram.Rect, ext, diagram.Icon(generic.Mail), diagram.IconLeft)
	wms := exts.Node("Warehouse system", diagram.Desc("Picks and ships"), diagram.Rect, ext, diagram.Icon(generic.Package), diagram.IconLeft)

	d.Connect(cust, sys, diagram.Label("browses, buys"))
	d.Connect(staff, sys, diagram.Label("issues refunds"))
	d.Connect(sys, pay, diagram.Label("charges cards"))
	d.Connect(sys, mail, diagram.Label("sends"))
	d.Connect(sys, wms, diagram.Label("creates shipments"))
	d.Connect(mail, cust, diagram.Dotted, diagram.Label("emails"), diagram.Exit(diagram.Bottom), diagram.Enter(diagram.Bottom))
	return d
}

func multiRegion() *diagram.Diagram {
	d := diagram.New("Multi-region")

	users := d.Node("Users", diagram.Person)
	global := d.Column("Global")
	dns := global.Node("DNS", diagram.Circle)
	waf := global.Node("WAF")

	regions := d.Column("")
	east := regions.Row("us-east-1", diagram.Desc("primary"))
	eastPub := east.Column("Public subnet")
	eastAPI := eastPub.Node("API")
	eastGW := eastPub.Node("Gateway")
	eastPriv := east.Column("Private subnet")
	eastDB := eastPriv.Node("Primary DB", diagram.Cylinder)
	eastCache := eastPriv.Node("Cache")

	west := regions.Row("us-west-2", diagram.Desc("standby"))
	westPub := west.Column("Public subnet")
	westAPI := westPub.Node("API")
	westPriv := west.Column("Private subnet")
	westDB := westPriv.Node("Replica DB", diagram.Cylinder)

	ops := d.Column("Ops")
	mon := ops.Node("Monitoring")
	pager := ops.Node("Pager", diagram.Pill)

	d.Connect(users, dns)
	d.Connect(dns, waf)
	d.Connect(waf, eastGW, diagram.Label("active"))
	d.Connect(waf, westAPI, diagram.Dashed, diagram.Label("failover"))
	d.Connect(eastGW, eastAPI)
	d.Connect(eastAPI, eastDB)
	d.Connect(eastAPI, eastCache)
	d.Connect(eastDB, westDB, diagram.Label("replication"))
	d.Connect(eastDB, westDB, diagram.Dashed)
	d.Connect(westAPI, westDB)
	d.Connect(mon, east, diagram.Dotted, diagram.Label("scrape"))
	d.Connect(mon, west, diagram.Dotted)
	d.Connect(mon, pager)
	d.Connect(pager, pager, diagram.Label("retry"))
	d.Connect(eastCache, eastCache)
	return d
}

func pinned() *diagram.Diagram {
	d := diagram.New("Pipeline")

	c := d.Canvas("")
	src := c.Node("Source", 0, 120)
	parse := c.Node("Parse", 200, 0)
	validate := c.Node("Validate", 200, 240)
	enrich := c.Node("Enrich", 420, 120)
	sink := c.Node("Sink", 640, 120, diagram.Cylinder)
	dlq := c.Node("Dead letters", 420, 330, diagram.Queue)
	audit := c.Node("Audit", 640, 0, diagram.Document)

	d.Connect(src, parse, diagram.Exit(diagram.Top), diagram.Enter(diagram.Left))
	d.Connect(src, validate, diagram.Exit(diagram.Bottom), diagram.Enter(diagram.Left))
	d.Connect(parse, enrich, diagram.Label("ok"), diagram.LabelAt(diagram.Start))
	d.Connect(validate, enrich, diagram.Label("ok"), diagram.LabelAt(diagram.End))
	d.Connect(parse, dlq, diagram.Exit(diagram.Right), diagram.Via(diagram.Pt(360, 70), diagram.Pt(360, 300)), diagram.Label("bad"))
	d.Connect(validate, dlq, diagram.Enter(diagram.Top))
	d.Connect(enrich, sink)
	d.Connect(enrich, audit, diagram.Straight, diagram.Dashed)
	d.Connect(sink, audit, diagram.Exit(diagram.Top), diagram.Enter(diagram.Bottom))
	d.Connect(dlq, src, diagram.Exit(diagram.Left), diagram.Enter(diagram.Bottom), diagram.Label("replay"), diagram.Dashed)
	return d
}

func themed(t *themes.Theme) *diagram.Diagram {
	d := diagram.New("Theme: "+t.Name, diagram.Theme(t))
	client := d.Node("Client", diagram.Icon(generic.Laptop))
	backend := d.Column("Backend", diagram.Icon(generic.Server))
	api := backend.Node("API", diagram.Desc("Go"), diagram.Icon(brands.Go), diagram.IconLeft)
	inner := backend.Row("Storage")
	db := inner.Node("Postgres", diagram.Cylinder)
	cache := inner.Node("Cache", diagram.Fill(diagram.Hex("#dcfce7")), diagram.Desc("literal fill"))
	alerts := d.Node("Alerts", diagram.Stroke(diagram.Red), diagram.Icon(generic.Bell))
	d.Connect(client, api, diagram.Label("HTTPS"))
	d.Connect(api, db)
	d.Connect(api, cache, diagram.Dashed)
	d.Connect(api, alerts, diagram.Stroke(diagram.Red), diagram.Dotted, diagram.Label("errors"))
	return d
}

func iconCatalog() *diagram.Diagram {
	d := diagram.New("Bundled icons", diagram.Vertical)
	const cols = 10
	add := func(title string, n int, at func(i int) (string, diagram.NodeOption, diagram.NodeOption)) {
		g := d.Grid(title, cols, diagram.Gap(12), diagram.AlignCenter)
		for i := range n {
			name, icon, extra := at(i)
			opts := []diagram.NodeOption{icon, diagram.IconOnly, diagram.IconSize(32), diagram.FontSize(11), diagram.MaxWidth(90)}
			if extra != nil {
				opts = append(opts, extra)
			}
			g.Node(name, i/cols, i%cols, opts...)
		}
	}
	gen := generic.All()
	add("icons/generic", len(gen), func(i int) (string, diagram.NodeOption, diagram.NodeOption) {
		return gen[i].Name, diagram.Icon(gen[i].Icon), nil
	})
	net := network.All()
	add("icons/network", len(net), func(i int) (string, diagram.NodeOption, diagram.NodeOption) {
		return net[i].Name, diagram.Icon(net[i].Icon), nil
	})
	br := brands.All()
	add("icons/brands", len(br), func(i int) (string, diagram.NodeOption, diagram.NodeOption) {
		return br[i].Name, diagram.Icon(br[i].Icon), diagram.IconColor(diagram.Hex(br[i].Color))
	})
	return d
}
