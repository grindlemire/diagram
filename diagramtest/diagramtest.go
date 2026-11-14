// Package diagramtest checks diagrams against golden files in tests.
//
//	func TestPaymentsDiagram(t *testing.T) {
//		diagramtest.Golden(t, docs.Payments())
//	}
//
// Golden validates the diagram, renders it and compares the output with
// testdata/<name>.<ext>, where name is the test's name unless Name gives
// another. Create or update the golden files with either
//
//	go test ./... -diagram.update
//	DIAGRAM_UPDATE=1 go test ./...
//
// The flag is registered on the default flag set when this package is
// imported, so it works with any test binary that imports it. The
// environment variable also works where the flag can't be passed, such as
// go test ./... across packages that don't all import diagramtest.
//
// On a mismatch Golden writes the new output next to the golden file as
// testdata/<name>.got.<ext> and fails the test. Open the two files side by
// side (in a browser, for SVG) to see what changed. Passing runs remove
// stale .got files.
package diagramtest

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/grindlemire/diagram"
)

var updateFlag = flag.Bool("diagram.update", false, "rewrite diagramtest golden files in testdata/ (or set DIAGRAM_UPDATE=1)")

// updating reports whether golden files should be rewritten.
func updating() bool {
	if *updateFlag {
		return true
	}
	switch strings.ToLower(os.Getenv("DIAGRAM_UPDATE")) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// An Option configures Golden.
type Option func(*config)

type config struct {
	name    string
	formats []string
	errs    []string
}

// Name sets the golden file name, without extension, instead of the test's
// name. Use it when one test checks several diagrams. It may contain
// slashes to put files in a subdirectory of testdata.
func Name(name string) Option {
	return func(c *config) {
		clean := filepath.ToSlash(filepath.Clean(name))
		if name == "" || filepath.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") {
			c.errs = append(c.errs, fmt.Sprintf("Name(%q): want a relative name inside testdata", name))
			return
		}
		c.name = clean
	}
}

// Formats sets the formats compared, from "svg" (the default), "drawio",
// "html" and "png". SVG, draw.io and HTML must match byte for byte. PNG
// allows small per-pixel differences, so anti-aliasing that differs
// between platforms doesn't fail the test; the image size must match
// exactly.
func Formats(formats ...string) Option {
	return func(c *config) {
		c.formats = nil
		for _, f := range formats {
			f = strings.ToLower(strings.TrimPrefix(f, "."))
			if !slices.Contains([]string{"svg", "drawio", "html", "png"}, f) {
				c.errs = append(c.errs, fmt.Sprintf("Formats: unknown format %q (want svg, drawio, html or png)", f))
				continue
			}
			if !slices.Contains(c.formats, f) {
				c.formats = append(c.formats, f)
			}
		}
		if len(c.formats) == 0 {
			c.errs = append(c.errs, "Formats: no formats")
		}
	}
}

// Golden validates d, failing the test with every validation error if it is
// invalid, renders it in each format (SVG unless Formats says otherwise) and
// compares the output with testdata/<name>.<ext>. See the package
// documentation for updating golden files.
func Golden(t testing.TB, d *diagram.Diagram, opts ...Option) {
	t.Helper()
	c := config{name: sanitize(t.Name()), formats: []string{"svg"}}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if len(c.errs) > 0 {
		t.Fatalf("diagramtest: %s", strings.Join(c.errs, "; "))
	}
	if d == nil {
		t.Fatalf("diagramtest: nil diagram")
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("diagramtest: diagram %q is invalid:\n%v", d.Title(), err)
	}
	update := updating()
	for _, f := range c.formats {
		var buf bytes.Buffer
		if err := render(d, f, &buf); err != nil {
			t.Fatalf("diagramtest: rendering %q as %s:\n%v", d.Title(), f, err)
		}
		golden := filepath.Join("testdata", filepath.FromSlash(c.name)+"."+f)
		gotPath := filepath.Join("testdata", filepath.FromSlash(c.name)+".got."+f)
		if update {
			if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
				t.Fatalf("diagramtest: %v", err)
			}
			if err := os.WriteFile(golden, buf.Bytes(), 0o644); err != nil {
				t.Fatalf("diagramtest: %v", err)
			}
			os.Remove(gotPath)
			t.Logf("diagramtest: wrote %s", golden)
			continue
		}
		var problem string
		switch want, err := os.ReadFile(golden); {
		case err == nil:
			diff := compare(f, buf.Bytes(), want)
			if diff == "" {
				os.Remove(gotPath)
				continue
			}
			problem = fmt.Sprintf("%s differs from the rendered diagram: %s", golden, diff)
		case os.IsNotExist(err):
			problem = golden + " does not exist"
		default:
			problem = err.Error()
		}
		if werr := os.MkdirAll(filepath.Dir(gotPath), 0o755); werr == nil {
			werr = os.WriteFile(gotPath, buf.Bytes(), 0o644)
		}
		t.Errorf("diagramtest: %v\n"+
			"\twrote the new output to %s\n"+
			"\tto see the difference, open both files side by side (in a browser for SVG and HTML, in draw.io for .drawio)\n"+
			"\tto accept the new output, rerun with -diagram.update or DIAGRAM_UPDATE=1, e.g.\n"+
			"\t\tgo test -run '^%s$' -diagram.update", problem, gotPath, runPattern(t.Name()))
	}
}

func render(d *diagram.Diagram, format string, buf *bytes.Buffer) error {
	switch format {
	case "svg":
		return d.RenderSVG(buf)
	case "drawio":
		return d.RenderDrawio(buf)
	case "html":
		return d.RenderHTML(buf)
	case "png":
		return d.RenderPNG(buf)
	}
	return fmt.Errorf("unknown format %q", format)
}

// compare returns "" if got matches want, or what differs.
func compare(format string, got, want []byte) string {
	if bytes.Equal(got, want) {
		return ""
	}
	if format == "png" {
		return comparePNG(got, want)
	}
	// Report the first differing line, skipping embedded data (the font
	// subset changes with every new character) unless nothing else differs.
	gl, wl := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	embedded := 0
	for i := 0; i < max(len(gl), len(wl)); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g == w {
			continue
		}
		if strings.Contains(g, ";base64,") && strings.Contains(w, ";base64,") {
			if embedded == 0 {
				embedded = i + 1
			}
			continue
		}
		return fmt.Sprintf("first difference at line %d:\n\t\twant: %s\n\t\t got: %s", i+1, clip(w, g), clip(g, w))
	}
	if embedded > 0 {
		return fmt.Sprintf("only embedded data (fonts or images) differs, first at line %d", embedded)
	}
	return "outputs differ"
}

// clip shortens s around the first byte where it differs from other.
func clip(s, other string) string {
	const before, total = 40, 120
	i := 0
	for i < len(s) && i < len(other) && s[i] == other[i] {
		i++
	}
	start := max(0, i-before)
	end := min(len(s), start+total)
	out := s[start:end]
	if start > 0 {
		out = "…" + out
	}
	if end < len(s) {
		out += "…"
	}
	return out
}

// PNG tolerance: a pixel differs when any channel differs by more than
// pngChannelTolerance, and the images match when at most pngPixelFraction
// of the pixels differ.
const (
	pngChannelTolerance = 16
	pngPixelFraction    = 0.001
)

func comparePNG(got, want []byte) string {
	g, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		return "rendered PNG: " + err.Error()
	}
	w, err := png.Decode(bytes.NewReader(want))
	if err != nil {
		return "golden PNG: " + err.Error()
	}
	if g.Bounds().Size() != w.Bounds().Size() {
		return fmt.Sprintf("size %v, golden file is %v", g.Bounds().Size(), w.Bounds().Size())
	}
	n := differingPixels(g, w)
	total := g.Bounds().Dx() * g.Bounds().Dy()
	if float64(n) <= pngPixelFraction*float64(total) {
		return ""
	}
	return fmt.Sprintf("%d of %d pixels differ", n, total)
}

func differingPixels(a, b image.Image) int {
	n := 0
	ab, bb := a.Bounds(), b.Bounds()
	for y := 0; y < ab.Dy(); y++ {
		for x := 0; x < ab.Dx(); x++ {
			r1, g1, b1, a1 := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			r2, g2, b2, a2 := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			if far(r1, r2) || far(g1, g2) || far(b1, b2) || far(a1, a2) {
				n++
			}
		}
	}
	return n
}

func far(a, b uint32) bool {
	d := int(a>>8) - int(b>>8)
	return d > pngChannelTolerance || d < -pngChannelTolerance
}

// sanitize turns a test name into a file name: subtests become
// "TestX-sub", and anything but letters, digits, '.', '_' and '-' becomes
// '_'.
func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/':
			b.WriteByte('-')
		case r == '.' || r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), ".")
	if s == "" {
		return "diagram"
	}
	return s
}

// runPattern returns a -run pattern matching the test name: each subtest
// level anchored, with regexp metacharacters escaped.
func runPattern(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		var b strings.Builder
		for _, r := range p {
			if strings.ContainsRune(`\.+*?()|[]{}^$`, r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		parts[i] = b.String()
	}
	return strings.Join(parts, "$/^")
}
