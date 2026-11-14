// Command readme renders the README's images into docs/: the banner and
// the quick-start example's output, each in a light and a dark version for
// GitHub's <picture> theme switch.
//
//	go run ./examples/readme
package main

import (
	"log"
	"os"

	"github.com/grindlemire/diagram"
	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/icons/brands"
	"github.com/grindlemire/diagram/icons/generic"
	"github.com/grindlemire/diagram/themes"
)

func main() {
	for name, d := range map[string]*diagram.Diagram{"header": banner(), "quickstart": quickstart()} {
		for mode, t := range map[string]*themes.Theme{"light": themes.Light, "dark": themes.Dark} {
			f, err := os.Create("docs/" + name + "-" + mode + ".svg")
			if err != nil {
				log.Fatal(err)
			}
			if err := d.RenderSVG(f, diagram.WithTheme(t), diagram.Transparent); err != nil {
				log.Fatal(err)
			}
			if err := f.Close(); err != nil {
				log.Fatal(err)
			}
		}
	}
}

// quickstart is the README's first example; keep the two in step.
func quickstart() *diagram.Diagram {
	d := diagram.New("Checkout")

	user := d.Node("Customer", diagram.Person)
	vpc := d.Column("Production VPC", diagram.Icon(generic.Cloud), diagram.Desc("us-east-1"))
	api := vpc.Node("API", diagram.Icon(brands.Go), diagram.IconLeft, diagram.Desc("3 replicas"))
	worker := vpc.Node("Worker", diagram.Icon(generic.Cog), diagram.IconLeft, diagram.Desc("payouts"))
	db := d.Node("Postgres", diagram.Cylinder, diagram.Desc("primary"))

	d.Connect(user, api, diagram.Label("HTTPS"))
	d.Connect(api, worker, diagram.Dashed, diagram.Label("jobs"))
	d.Connect(api, db)
	d.Connect(worker, db)
	return d
}

// banner is the README's header: the name and tagline beside a small,
// made-up service map.
func banner() *diagram.Diagram {
	d := diagram.New("diagram")
	row := d.Row("", diagram.AlignCenter, diagram.Gap(88))

	words := row.Column("", diagram.AlignStart, diagram.Gap(0))
	words.Node("diagram", diagram.Text, diagram.FontSize(76), diagram.Bold)
	words.Node("System diagrams as Go code", diagram.Text, diagram.FontSize(22),
		diagram.MaxWidth(600), diagram.TextColor(diagram.Muted))

	card := func(c *diagram.Column, label string, icon *icons.Icon, color diagram.Color, opts ...diagram.NodeOption) *diagram.Node {
		return c.Node(label, append([]diagram.NodeOption{diagram.Icon(icon), diagram.IconColor(color), diagram.IconLeft, diagram.IconSize(22)}, opts...)...)
	}
	m := row.Row("", diagram.AlignCenter)
	web := card(m.Column("", diagram.JustifyCenter), "web", brands.React, diagram.Hex(brands.ReactColor))
	cluster := m.Column("cluster", diagram.Icon(brands.Kubernetes), diagram.Uniform)
	api := card(cluster, "api", brands.Go, diagram.Hex(brands.GoColor), diagram.Status(diagram.Green))
	jobs := card(cluster, "jobs", brands.Kafka, diagram.Accent)
	data := m.Column("", diagram.Uniform, diagram.JustifyCenter)
	db := card(data, "postgres", brands.Postgres, diagram.Blue)
	cache := card(data, "redis", brands.Redis, diagram.Hex(brands.RedisColor))

	d.Connect(web, cluster, diagram.Label("HTTPS"))
	d.Connect(api, db)
	d.Connect(api, jobs, diagram.Dashed)
	d.Connect(jobs, cache)
	return d
}
