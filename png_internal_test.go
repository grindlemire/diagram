package diagram

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grindlemire/diagram/icons/brands"
	"github.com/grindlemire/diagram/icons/generic"
	"github.com/grindlemire/diagram/icons/network"
	"github.com/grindlemire/diagram/internal/paint"
	pngr "github.com/grindlemire/diagram/internal/render/png"
)

var pngDir = flag.String("pngdir", "", "write each case as SVG and PNG, plus a side-by-side index.html, to this directory")

// renderPNGInternal runs the PNG pipeline directly, until RenderPNG is
// wired to it.
func renderPNGInternal(d *Diagram, scale float64) ([]byte, error) {
	cfg := defaultConfig()
	res, err := d.lay(cfg)
	if err != nil {
		return nil, err
	}
	sc := paint.Paint(res, paint.Options{Margin: cfg.margin})
	var buf bytes.Buffer
	if err := pngr.Render(&buf, sc, pngr.Options{Scale: scale}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// pngCases adds a diagram that uses every shape, edge style and icon set.
var pngCases = append(cases[:len(cases):len(cases)], struct {
	name  string
	build func() *Diagram
}{"everything", func() *Diagram {
	d := New("Everything", Vertical)
	g := d.Grid("Shapes", 5, Uniform)
	shapes := []Shape{Box, Rect, Pill, Circle, Cylinder, Queue, Document, Person, Note, Text}
	var ns []*Node
	for i, s := range shapes {
		ns = append(ns, g.Node(fmt.Sprintf("Shape %d", i+1), i/5, i%5, s, Desc("desc")))
	}
	ic := d.Row("Icons")
	a := ic.Node("Server", Icon(generic.Server))
	b := ic.Node("Database", Icon(generic.Database), IconLeft, Desc("generic"))
	c := ic.Node("K8s", Icon(brands.Kubernetes), IconOnly, IconSize(40))
	e := ic.Node("Router", Icon(network.Router), Fill(Hex("#fef3c7")), Stroke(Orange), Dashed, Radius(16))
	f := ic.Node("Go", Icon(brands.Go), IconLeft, Stroke(Purple), Dotted, StrokeWidth(2))
	d.Connect(ns[0], ns[1], Label("solid"))
	d.Connect(ns[2], ns[3], Dashed, Label("dashed"))
	d.Connect(ns[3], ns[4], Dotted)
	d.Connect(ns[5], ns[6], ArrowBoth, Stroke(Red), StrokeWidth(2))
	d.Connect(ns[7], ns[8], Head(Diamond), Tail(Dot))
	d.Connect(ns[8], ns[9], Head(Open), NoArrow)
	d.Connect(a, b, Label("HTTPS"))
	d.Connect(b, c, Dashed, Stroke(Blue))
	d.Connect(c, e, Dotted, Label("dotted"))
	d.Connect(e, f, Head(Open))
	d.Connect(ns[2], a, Exit(Bottom))
	return d
}})

func TestPNGCases(t *testing.T) {
	for _, c := range pngCases {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			data, err := renderPNGInternal(c.build(), 2)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %d bytes in %v", c.name, len(data), time.Since(start))
			again, err := renderPNGInternal(c.build(), 2)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, again) {
				t.Error("two renders differ")
			}
			if *pngDir != "" {
				var svgBuf bytes.Buffer
				if err := c.build().RenderSVG(&svgBuf); err != nil {
					t.Fatal(err)
				}
				write(t, filepath.Join(*pngDir, c.name+".svg"), svgBuf.Bytes())
				write(t, filepath.Join(*pngDir, c.name+".png"), data)
			}
		})
	}
	if *pngDir != "" {
		var page bytes.Buffer
		page.WriteString(`<!doctype html><meta charset="utf-8"><title>svg vs png</title><style>body{font-family:system-ui;margin:16px;background:#ddd}.p{display:flex;gap:12px;align-items:flex-start}figure{margin:0;background:#fff}img{display:block}</style>`)
		for _, c := range pngCases {
			fmt.Fprintf(&page, `<h3>%s</h3><div class=p><figure><figcaption>svg</figcaption><img src="%[1]s.svg" id="%[1]s-svg"></figure><figure><figcaption>png</figcaption><img src="%[1]s.png" style="zoom:0.5" id="%[1]s-png"></figure></div>`, c.name)
		}
		write(t, filepath.Join(*pngDir, "index.html"), page.Bytes())
	}
}

// TestPNGBundledIcons draws every bundled icon, so an icon outside the PNG
// subset fails here rather than in a user's render.
func TestPNGBundledIcons(t *testing.T) {
	d := New("Icons")
	g := d.Grid("", 8)
	i := 0
	for _, e := range generic.All() {
		g.Node(e.Name, i/8, i%8, Icon(e.Icon))
		i++
	}
	for _, e := range network.All() {
		g.Node(e.Name, i/8, i%8, Icon(e.Icon))
		i++
	}
	for _, e := range brands.All() {
		g.Node(e.Name, i/8, i%8, Icon(e.Icon))
		i++
	}
	data, err := renderPNGInternal(d, 2)
	if err != nil {
		t.Fatal(err)
	}
	if *pngDir != "" {
		var svgBuf bytes.Buffer
		if err := d.RenderSVG(&svgBuf); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(*pngDir, "icons.svg"), svgBuf.Bytes())
		write(t, filepath.Join(*pngDir, "icons.png"), data)
	}
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// BenchmarkPNG times the PNG renderer alone (rasterize and encode) on the
// largest case, at the default scale.
func BenchmarkPNG(b *testing.B) {
	d := pngCases[len(pngCases)-1].build()
	cfg := defaultConfig()
	res, err := d.lay(cfg)
	if err != nil {
		b.Fatal(err)
	}
	sc := paint.Paint(res, paint.Options{Margin: cfg.margin})
	b.Run("rasterize", func(b *testing.B) {
		for b.Loop() {
			if _, err := pngr.Rasterize(sc, pngr.Options{Scale: 2}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("render", func(b *testing.B) {
		for b.Loop() {
			if err := pngr.Render(io.Discard, sc, pngr.Options{Scale: 2}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
