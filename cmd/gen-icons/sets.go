package main

// An icon maps an exported Go name to an upstream icon slug (its file name
// without .svg).
type icon struct {
	Name string // exported Go identifier
	Slug string // upstream file name without .svg
}

// A set is one generated package.
type set struct {
	Pkg     string // package name and directory under icons/
	Source  string // directory under sources/: "lucide" or "simple-icons"
	Blurb   string // first sentence of the package doc, after "Package <pkg> "
	Example string // an icon name used in doc comments
	Icons   []icon
}

// The hand-maintained name map. Adding an icon here and running
// `go run . -import-lucide ... -import-simple-icons ...` vendors its SVG;
// a plain `go run .` then regenerates the packages.
var sets = []set{
	{
		Pkg:     "generic",
		Example: "HardDrive",
		Source:  "lucide",
		Blurb:   "provides general infrastructure icons: servers, storage, clients, people, files, messaging and tooling.",
		Icons: []icon{
			{"Activity", "activity"},
			{"Archive", "archive"},
			{"BarChart", "chart-column"},
			{"Bell", "bell"},
			{"Bot", "bot"},
			{"Box", "box"},
			{"Browser", "app-window"},
			{"CPU", "cpu"},
			{"Calendar", "calendar"},
			{"Clock", "clock"},
			{"Cloud", "cloud"},
			{"CloudDownload", "cloud-download"},
			{"CloudUpload", "cloud-upload"},
			{"Code", "code"},
			{"Cog", "cog"},
			{"Container", "container"},
			{"Database", "database"},
			{"File", "file"},
			{"FileText", "file-text"},
			{"Folder", "folder"},
			{"Function", "square-function"},
			{"Gauge", "gauge"},
			{"GitBranch", "git-branch"},
			{"Globe", "globe"},
			{"HardDrive", "hard-drive"},
			{"Key", "key"},
			{"Laptop", "laptop"},
			{"Layers", "layers"},
			{"LineChart", "chart-line"},
			{"Lock", "lock"},
			{"Mail", "mail"},
			{"Memory", "memory-stick"},
			{"Message", "message-square"},
			{"Monitor", "monitor"},
			{"Package", "package"},
			{"Queue", "inbox"},
			{"Search", "search"},
			{"Server", "server"},
			{"Settings", "settings"},
			{"Shield", "shield"},
			{"Smartphone", "smartphone"},
			{"Terminal", "terminal"},
			{"User", "user"},
			{"Users", "users"},
			{"Webhook", "webhook"},
			{"Workflow", "workflow"},
			{"Zap", "zap"},
		},
	},
	{
		Pkg:     "network",
		Example: "LoadBalancer",
		Source:  "lucide",
		Blurb:   "provides networking icons: routers, links, wireless, firewalls, load balancers and DNS.",
		Icons: []icon{
			{"ArrowLeftRight", "arrow-left-right"},
			{"Cable", "cable"},
			{"CloudCog", "cloud-cog"},
			{"DNS", "book-open-text"},
			{"EthernetPort", "ethernet-port"},
			{"Firewall", "brick-wall-fire"},
			{"Globe", "globe"},
			{"GlobeLock", "globe-lock"},
			{"Internet", "earth"},
			{"LoadBalancer", "split"},
			{"Network", "network"},
			{"RadioTower", "radio-tower"},
			{"Route", "route"},
			{"Router", "router"},
			{"SatelliteDish", "satellite-dish"},
			{"ServerCog", "server-cog"},
			{"Shield", "shield"},
			{"Signal", "signal"},
			{"Waypoints", "waypoints"},
			{"WiFi", "wifi"},
		},
	},
	{
		Pkg:     "brands",
		Example: "Postgres",
		Source:  "simple-icons",
		Blurb:   "provides logos of common infrastructure technologies.",
		Icons: []icon{
			{"Cloudflare", "cloudflare"},
			{"Docker", "docker"},
			{"Elasticsearch", "elasticsearch"},
			{"GitHub", "github"},
			{"GitLab", "gitlab"},
			{"Go", "go"},
			{"Grafana", "grafana"},
			{"GraphQL", "graphql"},
			{"Kafka", "apachekafka"},
			{"Kubernetes", "kubernetes"},
			{"Linux", "linux"},
			{"MongoDB", "mongodb"},
			{"MySQL", "mysql"},
			{"Nginx", "nginx"},
			{"NodeJS", "nodedotjs"},
			{"Postgres", "postgresql"},
			{"Prometheus", "prometheus"},
			{"Python", "python"},
			{"RabbitMQ", "rabbitmq"},
			{"React", "react"},
			{"Redis", "redis"},
			{"SQLite", "sqlite"},
			{"Stripe", "stripe"},
			{"Terraform", "terraform"},
			{"TypeScript", "typescript"},
			{"Vercel", "vercel"},
		},
	},
}
