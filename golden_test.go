package diagram

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"testing"

	"github.com/grindlemire/diagram/icons/brands"
	"github.com/grindlemire/diagram/icons/generic"
	"github.com/grindlemire/diagram/themes"
)

var (
	update  = flag.Bool("update", false, "rewrite golden files in testdata/")
	gallery = flag.String("gallery", "", "write an HTML review gallery comparing current output with the golden files to this path")
)

// cases are the diagrams covered by golden files and the gallery.
var cases = []struct {
	name  string
	build func() *Diagram
}{
	{"payments", func() *Diagram {
		d := New("Payments")
		web := d.Node("Web app", Desc("Next.js"), Icon(generic.Browser))
		vpc := d.Column("Production VPC", Desc("us-east-1"), Icon(generic.Cloud))
		api := vpc.Node("API", Desc("Go · 3 replicas"), Icon(brands.Go), IconLeft)
		workers := vpc.Grid("Workers", 2, Uniform)
		w1 := workers.Node("Ledger worker", 0, 0, Icon(generic.Cog), IconLeft, IconSize(20))
		w2 := workers.Node("Payout worker", 0, 1, Icon(generic.Cog), IconLeft, IconSize(20))
		data := d.Column("")
		db := data.Node("Postgres", Cylinder, Desc("primary"))
		q := data.Node("Events", Queue)
		critical := Style(Stroke(Red), StrokeWidth(2))
		d.Connect(web, api, Label("HTTPS"))
		d.Connect(api, w1)
		d.Connect(api, w2, Dashed)
		d.Connect(w1, db, critical, Label("writes"))
		d.Connect(w2, q, Dotted)
		return d
	}},
	{"shapes", func() *Diagram {
		d := New("Shapes", Vertical)
		g := d.Grid("Every shape", 5, Uniform)
		for i, s := range []Shape{Box, Rect, Pill, Circle, Cylinder, Queue, Document, Person, Note, Text} {
			g.Node(fmt.Sprintf("Shape %d", i+1), i/5, i%5, s)
		}
		r := d.Row("Icons")
		r.Node("Icon top", Icon(generic.Server))
		r.Node("Icon left", Icon(generic.Database), IconLeft, Desc("with a description"))
		r.Node("Icon only", Icon(brands.Kubernetes), IconOnly, IconSize(40))
		r.Node("Styled", Fill(Hex("#fef3c7")), Stroke(Orange), Dashed, Radius(16))
		return d
	}},
	{"nesting", func() *Diagram {
		d := New("Nesting")
		region := d.Column("Region", HeaderTop)
		vpc := region.Row("VPC", Icon(generic.Shield))
		pub := vpc.Column("Public subnet")
		lb := pub.Node("Load balancer")
		priv := vpc.Column("Private subnet")
		svc := priv.Node("Service")
		priv.Node("Cache", Cylinder)
		empty := region.Row("Reserved")
		canvas := d.Canvas("Legacy", Dashed)
		mf := canvas.Node("Mainframe", 0, 0)
		canvas.Node("Tape", 140, 90, Document)
		d.Connect(lb, svc)
		d.Connect(svc, mf, ArrowBoth, Label("batch"))
		d.Connect(lb, empty, Head(Diamond))
		return d
	}},
	{"auto-theme", func() *Diagram {
		d := New("Auto theme", Theme(themes.Auto(themes.Light, themes.Dark)))
		a := d.Node("Client", Icon(generic.Laptop))
		g := d.Column("Backend")
		b := g.Node("API", Stroke(Accent))
		c := g.Node("Store", Cylinder, Fill(Hex("#dcfce7")))
		d.Connect(a, b, Label("calls"))
		d.Connect(b, c, Stroke(Green))
		return d
	}},
}

func TestGolden(t *testing.T) {
	formats := []struct {
		ext    string
		render func(*Diagram, *bytes.Buffer) error
	}{
		{".svg", func(d *Diagram, b *bytes.Buffer) error { return d.RenderSVG(b) }},
		{".drawio", func(d *Diagram, b *bytes.Buffer) error { return d.RenderDrawio(b) }},
	}
	for _, c := range cases {
		for _, f := range formats {
			t.Run(c.name+f.ext, func(t *testing.T) {
				var buf bytes.Buffer
				if err := f.render(c.build(), &buf); err != nil {
					t.Fatal(err)
				}
				checkGolden(t, filepath.Join("testdata", c.name+f.ext), buf.Bytes())
			})
		}
	}
}

func checkGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from golden file; review with -gallery, then run go test -update", path)
	}
}

func TestDeterministic(t *testing.T) {
	for _, c := range cases {
		var a, b bytes.Buffer
		if err := c.build().RenderSVG(&a); err != nil {
			t.Fatal(err)
		}
		if err := c.build().RenderSVG(&b); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Errorf("%s: two renders differ", c.name)
		}
	}
}

// TestGallery writes an HTML page showing each case's golden file next to
// its current rendering:
//
//	go test -run TestGallery -gallery /tmp/gallery.html
func TestGallery(t *testing.T) {
	if *gallery == "" {
		t.Skip("set -gallery to write the review page")
	}
	var page bytes.Buffer
	page.WriteString(`<!doctype html><meta charset="utf-8"><title>diagram gallery</title>
<style>body{font-family:system-ui,sans-serif;margin:24px;background:#f1f5f9;color:#0f172a}
h2{margin:32px 0 8px}.pair{display:flex;gap:16px;flex-wrap:wrap}
figure{margin:0;background:#fff;padding:12px;border-radius:8px;box-shadow:0 1px 3px #0002}
figcaption{font-size:12px;color:#64748b;margin-bottom:8px}.changed{color:#dc2626;font-weight:600}
img{max-width:100%}</style>`)
	for _, c := range cases {
		var cur bytes.Buffer
		if err := c.build().RenderSVG(&cur); err != nil {
			fmt.Fprintf(&page, "<h2>%s</h2><p class=changed>%s</p>", c.name, html.EscapeString(err.Error()))
			continue
		}
		old, _ := os.ReadFile(filepath.Join("testdata", c.name+".svg"))
		status := "unchanged"
		if !bytes.Equal(old, cur.Bytes()) {
			status = `<span class=changed>changed</span>`
		}
		fmt.Fprintf(&page, "<h2>%s — %s</h2><div class=pair>", c.name, status)
		if old != nil {
			fmt.Fprintf(&page, `<figure><figcaption>golden</figcaption><img src="data:image/svg+xml;base64,%s"></figure>`, base64.StdEncoding.EncodeToString(old))
		}
		fmt.Fprintf(&page, `<figure><figcaption>current</figcaption><img src="data:image/svg+xml;base64,%s"></figure></div>`, base64.StdEncoding.EncodeToString(cur.Bytes()))
	}
	if err := os.WriteFile(*gallery, page.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", *gallery)
}
