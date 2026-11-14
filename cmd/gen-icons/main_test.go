package main

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestUpToDate fails when the checked-in packages differ from what the
// vendored sources generate.
func TestUpToDate(t *testing.T) {
	for _, s := range sets {
		files, err := generate(s, sourcesDir)
		if err != nil {
			t.Fatalf("%s: %v", s.Pkg, err)
		}
		for _, name := range sortedKeys(files) {
			got, err := os.ReadFile(filepath.Join("../../icons", s.Pkg, name))
			if err != nil {
				t.Errorf("%s: %v (run go run .)", s.Pkg, err)
				continue
			}
			if !bytes.Equal(got, files[name]) {
				t.Errorf("icons/%s/%s is stale; run go run .", s.Pkg, name)
			}
		}
	}
}

// TestSourcesMatchSets fails when sources/ holds SVGs that sets.go doesn't
// use, or lacks ones it does.
func TestSourcesMatchSets(t *testing.T) {
	for _, src := range []string{"lucide", "simple-icons"} {
		paths, err := filepath.Glob(filepath.Join(sourcesDir, src, "icons", "*.svg"))
		if err != nil {
			t.Fatal(err)
		}
		var have []string
		for _, p := range paths {
			have = append(have, strings.TrimSuffix(filepath.Base(p), ".svg"))
		}
		slices.Sort(have)
		if want := slugsFor(src); !slices.Equal(have, want) {
			t.Errorf("%s: vendored %v, sets.go wants %v", src, have, want)
		}
	}
}

// TestMinifyKeepsGeometry checks that minifying every vendored SVG keeps
// each element and attribute, apart from the ones it is meant to change.
func TestMinifyKeepsGeometry(t *testing.T) {
	for _, s := range sets {
		opts := lucideOpts
		if s.Source == "simple-icons" {
			opts = simpleIconsOpts
		}
		for _, ic := range s.Icons {
			src, err := os.ReadFile(filepath.Join(sourcesDir, s.Source, "icons", ic.Slug+".svg"))
			if err != nil {
				t.Fatal(err)
			}
			out, err := minify(src, opts)
			if err != nil {
				t.Fatalf("%s: %v", ic.Slug, err)
			}
			want := elements(t, src, opts)
			got := elements(t, []byte(out), minifyOpts{})
			if len(opts.rootAttrs) > 0 {
				want[0] += " " + attrString(opts.rootAttrs)
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s: elements changed\n got %q\nwant %q", ic.Slug, got, want)
			}
			if strings.Contains(out, "\n") || strings.Contains(out, "<!--") || strings.Contains(out, "class=") {
				t.Errorf("%s: not minified: %s", ic.Slug, out)
			}
		}
	}
}

// elements lists each element with its attributes, leaving out what opts
// drops.
func elements(t *testing.T, src []byte, opts minifyOpts) []string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(src))
	var list []string
	skip := 0
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			return list
		}
		if err != nil {
			t.Fatal(err)
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			if skip > 0 || slices.Contains(opts.dropElems, tk.Name.Local) {
				skip++
				continue
			}
			var attrs []xml.Attr
			for _, a := range tk.Attr {
				if !slices.Contains(opts.dropAttrs, a.Name.Local) {
					attrs = append(attrs, a)
				}
			}
			e := qname(tk.Name)
			if len(attrs) > 0 {
				e += " " + attrString(attrs)
			}
			list = append(list, e)
		case xml.EndElement:
			if skip > 0 {
				skip--
			}
		}
	}
}

func attrString(attrs []xml.Attr) string {
	var parts []string
	for _, a := range attrs {
		parts = append(parts, qname(a.Name)+"="+a.Value)
	}
	return strings.Join(parts, " ")
}

func TestMinify(t *testing.T) {
	src := `<?xml version="1.0"?>
<!-- comment -->
<svg class="x" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" stroke="currentColor">
  <g><path d="M0 0h1" /></g>
  <text>a &amp; b</text>
</svg>
`
	got, err := minify([]byte(src), lucideOpts)
	if err != nil {
		t.Fatal(err)
	}
	want := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" stroke="currentColor"><g><path d="M0 0h1"/></g><text>a &amp; b</text></svg>`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if _, err := minify([]byte(`<svg viewBox="0 0 1 1"/>`), lucideOpts); err == nil {
		t.Error("want an error for a Lucide icon without stroke=currentColor")
	}
	if _, err := minify([]byte(`<svg fill="red" viewBox="0 0 1 1"/>`), simpleIconsOpts); err == nil {
		t.Error("want an error for a Simple Icons root that already has a fill")
	}
}
