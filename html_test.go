package diagram

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/icons/generic"
	"github.com/grindlemire/diagram/themes"
)

// linked is a diagram that uses Tooltip and Link on nodes, groups and
// edges, and has an edge to an invisible container.
func linked() *Diagram {
	d := New("Links")
	web := d.Node("Web", Desc("browser"), Icon(generic.Browser), Tooltip("Served from the CDN"), Link("https://example.com/web?a=1&b=2"))
	vpc := d.Column("VPC", Tooltip(`Private "prod" network`), Link("/runbooks/vpc"))
	api := vpc.Node("API", Link("mailto:oncall@example.com"))
	hidden := d.Column("")
	db := hidden.Node("DB", Cylinder)
	d.Connect(web, api, Label("HTTPS"), Tooltip("TLS <1.3>"))
	d.Connect(api, db)
	d.Connect(web, hidden, Dashed)
	return d
}

var htmlCases = []struct {
	name  string
	build func() *Diagram
}{
	{"payments", cases[0].build},
	{"links", linked},
}

func renderHTML(t *testing.T, d *Diagram, opts ...HTMLOption) string {
	t.Helper()
	var b bytes.Buffer
	if err := d.RenderHTML(&b, opts...); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestHTMLGolden(t *testing.T) {
	for _, c := range htmlCases {
		t.Run(c.name, func(t *testing.T) {
			checkGolden(t, filepath.Join("testdata", c.name+".html"), []byte(renderHTML(t, c.build())))
		})
	}
}

func TestHTMLDeterministic(t *testing.T) {
	for _, c := range htmlCases {
		if renderHTML(t, c.build()) != renderHTML(t, c.build()) {
			t.Errorf("%s: two renders differ", c.name)
		}
	}
}

var (
	dataElemRe = regexp.MustCompile(`<g ([^>]*data-kind="[^"]*"[^>]*)>`)
	attrRe     = regexp.MustCompile(`([a-z-]+)="([^"]*)"`)
)

// elements returns the attributes of every element the HTML page's SVG
// describes with data-kind, in document order.
func elements(page string) []map[string]string {
	var out []map[string]string
	for _, m := range dataElemRe.FindAllStringSubmatch(page, -1) {
		attrs := map[string]string{}
		for _, a := range attrRe.FindAllStringSubmatch(m[1], -1) {
			attrs[a[1]] = a[2]
		}
		out = append(out, attrs)
	}
	return out
}

func TestHTMLStructure(t *testing.T) {
	for _, c := range append(htmlCases, cases...) {
		t.Run(c.name, func(t *testing.T) {
			page := renderHTML(t, c.build())
			els := elements(page)
			ids := map[string]string{} // id -> kind
			for _, e := range els {
				if e["data-kind"] == "edge-label" {
					continue
				}
				if e["data-id"] == "" || e["data-id"] != e["id"] {
					t.Errorf("element %v: data-id %q, id %q", e["data-kind"], e["data-id"], e["id"])
				}
				ids[e["data-id"]] = e["data-kind"]
			}
			edges := 0
			for _, e := range els {
				switch e["data-kind"] {
				case "edge":
					edges++
					for _, end := range []string{"data-from", "data-to"} {
						if k := ids[e[end]]; k != "node" && k != "group" {
							t.Errorf("edge %s: %s=%q names no node or group", e["id"], end, e[end])
						}
					}
				case "edge-label":
					if ids[e["data-for"]] != "edge" {
						t.Errorf("edge label for %q: no such edge", e["data-for"])
					}
				case "node":
					if e["tabindex"] != "0" || e["role"] != "button" || e["aria-label"] == "" {
						t.Errorf("node %s not focusable and named: %v", e["id"], e)
					}
				}
				if p := e["data-parent"]; p != "" && ids[p] != "group" {
					t.Errorf("%s: data-parent=%q names no group", e["id"], p)
				}
			}
			if want := len(c.build().m.Edges); edges != want {
				t.Errorf("%d edges in page, want %d", edges, want)
			}
			// The page leaves tooltips and links to its script and loads
			// nothing: the only URLs are the SVG namespace and links.
			body := page[strings.Index(page, "<body>"):]
			body = regexp.MustCompile(`https?://(www\.w3\.org/2000/svg|example\.com)[^"]*`).ReplaceAllString(body, "")
			for _, bad := range []string{"<title>", "<a ", "http://", "https://"} {
				if strings.Contains(body, bad) {
					t.Errorf("page body contains %q", bad)
				}
			}
		})
	}
}

func TestHTMLHiddenEndpoint(t *testing.T) {
	page := renderHTML(t, linked())
	if !strings.Contains(page, `<g id="column-3" class="group" data-kind="group" data-id="column-3" data-hidden="true"/>`) {
		t.Error("invisible container endpoint is not emitted as a hidden group")
	}
	if !strings.Contains(page, `data-from="web" data-to="column-3"`) {
		t.Error("edge to the invisible container doesn't name it")
	}
}

func TestTooltipAndLinkInEveryFormat(t *testing.T) {
	var svg, dio bytes.Buffer
	if err := linked().RenderSVG(&svg); err != nil {
		t.Fatal(err)
	}
	if err := linked().RenderDrawio(&dio); err != nil {
		t.Fatal(err)
	}
	page := renderHTML(t, linked())

	for _, want := range []string{
		"<a href=\"https://example.com/web?a=1&amp;b=2\">\n  <g id=\"web\" class=\"node\">\n    <title>Served from the CDN</title>\n",
		"<a href=\"/runbooks/vpc\">\n  <g id=\"vpc\" class=\"group\">\n    <title>Private &#34;prod&#34; network</title>\n",
		"<a href=\"mailto:oncall@example.com\">\n  <g id=\"vpc/api\" class=\"node\">\n",
		"<g id=\"edge:web-&gt;vpc/api\" class=\"edge\">\n  <title>TLS &lt;1.3&gt;</title>\n",
	} {
		if !strings.Contains(svg.String(), want) {
			t.Errorf("SVG lacks %q", want)
		}
	}
	if n := strings.Count(svg.String(), "<title>"); n != 4 { // the diagram's and three tooltips
		t.Errorf("SVG has %d <title>s, want 4", n)
	}
	if n := strings.Count(svg.String(), "</a>"); n != 3 {
		t.Errorf("SVG has %d </a>, want 3", n)
	}

	// draw.io: cells with properties are wrapped in a UserObject that
	// carries the id, label, tooltip and link.
	var file struct {
		Objects []struct {
			ID      string `xml:"id,attr"`
			Label   string `xml:"label,attr"`
			Tooltip string `xml:"tooltip,attr"`
			Link    string `xml:"link,attr"`
			Cell    struct {
				ID     string `xml:"id,attr"`
				Vertex string `xml:"vertex,attr"`
				Edge   string `xml:"edge,attr"`
				Source string `xml:"source,attr"`
			} `xml:"mxCell"`
		} `xml:"diagram>mxGraphModel>root>UserObject"`
	}
	if err := xml.Unmarshal(dio.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, o := range file.Objects {
		if o.Cell.ID != "" {
			t.Errorf("%s: inner mxCell has its own id", o.ID)
		}
		got[o.ID] = fmt.Sprintf("tooltip=%q link=%q vertex=%s edge=%s source=%s", o.Tooltip, o.Link, o.Cell.Vertex, o.Cell.Edge, o.Cell.Source)
	}
	want := map[string]string{
		"web":               `tooltip="Served from the CDN" link="https://example.com/web?a=1&b=2" vertex=1 edge= source=`,
		"vpc":               `tooltip="Private \"prod\" network" link="/runbooks/vpc" vertex=1 edge= source=`,
		"vpc/api":           `tooltip="" link="mailto:oncall@example.com" vertex=1 edge= source=`,
		"edge:web->vpc/api": `tooltip="TLS <1.3>" link="" vertex= edge=1 source=web`,
	}
	if len(got) != len(want) {
		t.Errorf("UserObjects: got %v", got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("UserObject %s:\n got %s\nwant %s", id, got[id], w)
		}
	}
	if !strings.Contains(dio.String(), `<UserObject label="Web&lt;br&gt;`) {
		t.Error("draw.io UserObject doesn't carry the cell's label")
	}

	// HTML: data attributes, with no <title> or <a> for the browser to act on.
	for _, want := range []string{
		`data-id="web" data-label="Web" data-desc="browser" data-tooltip="Served from the CDN" data-link="https://example.com/web?a=1&amp;b=2" tabindex="0" role="button" aria-label="Web, browser"`,
		`data-id="vpc" data-label="VPC" data-tooltip="Private &#34;prod&#34; network" data-link="/runbooks/vpc"`,
		`data-from="web" data-to="vpc/api" data-label="HTTPS" data-tooltip="TLS &lt;1.3&gt;"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
}

func TestStandaloneSVGUnchangedWithoutTooltipOrLink(t *testing.T) {
	var b bytes.Buffer
	if err := cases[0].build().RenderSVG(&b); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"data-", "tabindex", "<a ", "var(--"} {
		if strings.Contains(b.String(), s) {
			t.Errorf("standalone SVG contains %q", s)
		}
	}
}

func TestLinkErrors(t *testing.T) {
	d := New("t")
	l1 := line() + 1
	d.Node("A", Link("javascript:alert(1)"))
	l2 := line() + 1
	d.Column("G", Link(""))
	l3 := line() + 1
	d.Node("B", Link("https://"))
	l4 := line() + 1
	d.Node("C", Link("ht tp://x"))
	l5 := line() + 1
	d.Node("D", Link("mailto:"))
	for _, ok := range []string{"https://example.com/x?y=1#z", "http://localhost:8080", "/docs/a", "../b.html", "#frag", "c.html", "//cdn.example.com/x", "mailto:a@b.c", "HTTPS://EXAMPLE.COM"} {
		d.Node("ok "+ok, Link(ok))
	}
	err := d.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	want := []string{
		fmt.Sprintf(`html_test.go:%d: node "A": Link("javascript:alert(1)"): scheme "javascript" not allowed (want http, https, mailto or a relative URL)`, l1),
		fmt.Sprintf(`html_test.go:%d: column "G": Link(""): want a URL`, l2),
		fmt.Sprintf(`html_test.go:%d: node "B": Link("https://"): want a host`, l3),
		fmt.Sprintf(`html_test.go:%d: node "C": Link("ht tp://x"): not a valid URL`, l4),
		fmt.Sprintf(`html_test.go:%d: node "D": Link("mailto:"): want an address`, l5),
	}
	msgs := strings.Split(err.Error(), "\n")
	if len(msgs) != len(want) {
		t.Fatalf("got %d errors:\n%s", len(msgs), err)
	}
	for i := range want {
		if msgs[i] != want[i] {
			t.Errorf("error %d:\n got %s\nwant %s", i, msgs[i], want[i])
		}
	}
	var b bytes.Buffer
	if d.RenderHTML(&b) == nil {
		t.Error("RenderHTML succeeded with invalid links")
	}
}

func TestHTMLThemeToggle(t *testing.T) {
	darkBG := "--dg-background:#0b1120"
	for _, c := range []struct {
		name   string
		theme  *themes.Theme
		toggle bool
		light  string // the first palette's background
	}{
		{"light", themes.Light, true, "--dg-background:#ffffff"},
		{"auto", themes.Auto(themes.Light, themes.Dark), true, "--dg-background:#ffffff"},
		{"dark", themes.Dark, false, darkBG},
		{"blueprint", themes.Blueprint, false, "--dg-background:#123a6b"},
	} {
		t.Run(c.name, func(t *testing.T) {
			page := renderHTML(t, linked(), WithTheme(c.theme))
			if got := strings.Contains(page, `id="dg-theme"`); got != c.toggle {
				t.Errorf("theme toggle shown: %v, want %v", got, c.toggle)
			}
			if !strings.Contains(page, ":root{"+c.light) {
				t.Errorf("light palette doesn't start with %s", c.light)
			}
			hasDark := strings.Contains(page, ":root[data-theme=dark]{"+darkBG) &&
				strings.Contains(page, "@media (prefers-color-scheme: dark){:root:not([data-theme=light]){"+darkBG)
			if hasDark != c.toggle {
				t.Errorf("dark palette rules present: %v, want %v", hasDark, c.toggle)
			}
		})
	}
}

func TestHTMLOptions(t *testing.T) {
	embedded := renderHTML(t, linked())
	byName := renderHTML(t, linked(), FontByName, Margin(0))
	if !strings.Contains(embedded, "@font-face") || strings.Contains(byName, "@font-face") {
		t.Error("EmbedFont/FontByName not applied to HTML")
	}
	width := regexp.MustCompile(`<svg xmlns="http://www.w3.org/2000/svg" width="([0-9.]+)"`)
	if a, b := width.FindStringSubmatch(embedded), width.FindStringSubmatch(byName); a == nil || b == nil || a[1] == b[1] {
		t.Errorf("Margin(0) not applied to HTML: widths %v, %v", a, b)
	}
	var b bytes.Buffer
	if err := linked().RenderHTML(&b, Margin(-1)); err == nil {
		t.Error("Margin(-1) accepted")
	}

	dir := t.TempDir()
	for _, name := range []string{"x.html", "x.HTM"} {
		path := filepath.Join(dir, name)
		if err := linked().WriteFile(path); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(data, []byte("<!doctype html>")) || !bytes.Equal(data, []byte(embedded)) {
			t.Errorf("%s: not the HTML rendering", name)
		}
	}
}

// TestHTMLControls checks the page's own controls: the shortcut help (a
// dialog opened from the toolbar, listing the theme switch only when there
// is one), the path banner, and the script's deep-link, path-tracing and
// view hooks.
func TestHTMLControls(t *testing.T) {
	for _, c := range []struct {
		theme  *themes.Theme
		themed bool
	}{{themes.Light, true}, {themes.Dark, false}} {
		page := renderHTML(t, linked(), WithTheme(c.theme))
		for _, want := range []string{
			`<button type="button" id="dg-keys" aria-label="Keyboard shortcuts"`,
			`aria-haspopup="dialog" aria-controls="dg-help"`,
			`<dialog id="dg-help" aria-labelledby="dg-help-h">`,
			`<dt><kbd>Shift</kbd> click or <kbd>Shift</kbd><kbd>Enter</kbd></dt><dd>Trace the path from the focused node</dd>`,
			`<dt><kbd class="dg-mod">Ctrl</kbd> click or <kbd class="dg-mod">Ctrl</kbd><kbd>Enter</kbd></dt>`,
			`<dt><kbd>C</kbd></dt><dd>Copy a link to the focused element or path</dd>`,
			`<dt><kbd>?</kbd></dt><dd>Show or hide this help</dd>`,
			`<div id="dg-path" hidden>`,
			`class="p-x" aria-label="Clear path"`,
			"(window, 'hashchange',",
			"history.replaceState(",
			"window.diagramView = {get: page.get, set: page.set};",
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: page lacks %q", c.theme.Name, want)
			}
		}
		if got := strings.Contains(page, "<dt>theme button</dt>"); got != c.themed {
			t.Errorf("%s: help lists the theme switch: %v, want %v", c.theme.Name, got, c.themed)
		}
		// The page has no embed-only parts.
		for _, bad := range []string{"customElements"} {
			if strings.Contains(page, bad) {
				t.Errorf("%s: page contains %q", c.theme.Name, bad)
			}
		}
	}
}

// Embed-only options must not be HTMLOptions, so passing them to RenderHTML
// fails to compile; shared options must be both.
func TestEmbedOptionTyping(t *testing.T) {
	for name, o := range map[string]EmbedOption{
		"NoScript": NoScript, "Nonce": Nonce("n"), "Height": Height("1px"),
		"Toolbar": Toolbar(false), "EmbedTheme": EmbedTheme("dark"), "EmbedName": EmbedName("x"),
	} {
		if _, ok := o.(HTMLOption); ok {
			t.Errorf("%s satisfies HTMLOption; it should only apply to embeds", name)
		}
	}
	for name, o := range map[string]any{
		"Margin": Margin(1), "WithTheme": WithTheme(nil), "Transparent": Transparent,
		"EmbedFont": EmbedFont, "FontByName": FontByName,
	} {
		_, h := o.(HTMLOption)
		_, e := o.(EmbedOption)
		if !h || !e {
			t.Errorf("%s: HTMLOption %v, EmbedOption %v; want both", name, h, e)
		}
	}
}
