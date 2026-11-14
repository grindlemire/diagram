// Package html renders a scene as interactive HTML: a self-contained page,
// or a fragment to embed in another page.
//
// Both are the interactive SVG from package svg (data attributes on every
// element, token colours as CSS variables) with a stylesheet and script
// that add pan and zoom, highlighting of connections, tooltips and links,
// path tracing, deep links, search, keyboard navigation and, on a page, a
// theme switch. They make no network requests: fonts are embedded by the
// SVG. The script's core (view.js) is shared: the page is one view of the
// document (page.js), and each embed is a <diagram-view> element that
// mounts a view in its shadow root (element.js).
package html

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"image/color"
	"io"
	"strings"

	"github.com/grindlemire/diagram/internal/render/svg"
	"github.com/grindlemire/diagram/internal/scene"
	"github.com/grindlemire/diagram/themes"
)

var (
	//go:embed view.css
	viewCSS string
	//go:embed page.css
	pageCSS string
	//go:embed embed.css
	embedCSS string
	//go:embed view.js
	viewJS string
	//go:embed page.js
	pageJS string
	//go:embed element.js
	elementJS string
)

// Options control HTML output.
type Options struct {
	Title     string
	EmbedFont bool
}

// uiText and uiBold are the strings the page's own controls and tooltips
// draw, in the regular and semibold weights, so their characters are
// embedded with the diagram's fonts.
var (
	uiText = []string{"⌘-click or ⌘-Enter opens directly", "Ctrl-click or Ctrl-Enter opens directly",
		"Search", "No matches", "0123456789/%·,.:-", "Copy link", "Copied", "Copy failed", "C",
		"No path between and", " → ", "hop hops", "No directed path; showing undirected"}
	uiBold   = []string{"Open ↗", "→", "Keyboard shortcuts"}
	hintText = []string{"Use ⌘ + scroll to zoom", "Use Ctrl + scroll to zoom"}
)

// chrome says which controls a view has.
type chrome struct {
	embed   bool // an embed: compact toolbar, Ctrl/⌘-wheel zoom
	toolbar bool
	themed  bool // a theme switch
	linking bool // deep links
}

// Help rows are the shortcut overlay's rows: the keys or gestures, and
// what they do. [K] is drawn as a key; [Mod] is ⌘ on Apple devices and
// Ctrl elsewhere.
func helpFor(c chrome) [][2]string {
	zoom := "scroll, pinch or [+][−]"
	if c.embed {
		zoom = "[Mod] scroll, pinch or [+][−]"
	}
	rows := [][2]string{
		{"drag or [←][↑][→][↓]", "Pan; hold [Shift] for bigger steps"},
		{zoom, "Zoom"},
		{"[0]", "Fit to screen"},
		{"click or [Enter]", "Focus an element and its connections"},
		{"[Tab] or [Shift][Tab]", "Move between nodes"},
		{"[Shift] click or [Shift][Enter]", "Trace the path from the focused node"},
		{"[Esc]", "Clear the path, focus or search"},
	}
	if c.toolbar {
		rows = append(rows, [2]string{"[/]", "Search; [Enter] steps through matches"})
	}
	rows = append(rows, [2]string{"[Mod] click or [Mod][Enter]", "Open an element’s link"})
	if c.linking {
		rows = append(rows, [2]string{"[C]", "Copy a link to the focused element or path"})
	}
	if c.themed {
		rows = append(rows, [2]string{"theme button", "Switch between light and dark"})
	}
	return append(rows, [2]string{"[?]", "Show or hide this help"})
}

const helpNote = "The page’s address links to what is focused or traced."

// interactiveSVG renders sc as the interactive SVG, with the characters
// of the view's own text added to its fonts.
func interactiveSVG(sc *scene.Scene, opt Options, c chrome) ([]byte, error) {
	regular, bold := texts(sc.Items)
	extra := append(regular, uiText...)
	for _, r := range helpFor(c) {
		extra = append(extra, plain(r[0]), plain(r[1]))
	}
	if c.linking {
		extra = append(extra, helpNote)
	}
	if c.embed {
		extra = append(extra, hintText...)
	}
	so := svg.Options{
		Title: opt.Title, EmbedFont: opt.EmbedFont, Interactive: true,
		ExtraText: extra, ExtraBold: append(bold, uiBold...),
	}
	var b bytes.Buffer
	if err := svg.Render(&b, sc, so); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Render writes sc as an HTML page.
func Render(w io.Writer, sc *scene.Scene, opt Options) error {
	light, dark := palettes(sc.Theme)
	c := chrome{toolbar: true, themed: dark != nil, linking: true}
	body, err := interactiveSVG(sc, opt, c)
	if err != nil {
		return err
	}

	title := opt.Title
	if title == "" {
		title = "Diagram"
	}
	bw := bufio.NewWriter(w)
	bw.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	bw.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<link rel=\"icon\" href=\"data:,\">\n")
	fmt.Fprintf(bw, "<title>%s</title>\n", esc(title))
	bw.WriteString("<style>\n")
	writeVars(bw, light, dark, ":root", ":root[data-theme=dark]", ":root:not([data-theme=light])")
	bw.WriteString(compact(viewCSS, "/*"))
	bw.WriteString(compact(pageCSS, "/*"))
	bw.WriteString("</style>\n</head>\n<body>\n")
	bw.WriteString("<main id=\"dg-stage\">\n")
	bw.Write(body)
	bw.WriteString("</main>\n")
	writeControls(bw, c)
	bw.WriteString("<script>\n(() => {\n")
	bw.WriteString(compact(viewJS, "//"))
	bw.WriteString(compact(pageJS, "//"))
	bw.WriteString("})();\n</script>\n</body>\n</html>\n")
	return bw.Flush()
}

// writeControls writes the view's controls after its stage: the toolbar,
// path banner, tooltip, wheel hint and shortcut help.
func writeControls(w *bufio.Writer, c chrome) {
	if c.toolbar {
		writeToolbar(w, c)
	}
	writePathBanner(w)
	w.WriteString("<div id=\"dg-tip\" role=\"tooltip\" hidden></div>\n")
	if c.embed {
		w.WriteString("<div id=\"dg-hint\" aria-hidden=\"true\" hidden></div>\n")
	}
	writeHelp(w, c)
}

// palettes returns the page's light palette and, when the page can switch
// themes, its dark one: the theme's own dark palette (themes.Auto), or
// themes.Dark's for themes.Light. Other themes have one palette and no
// switch.
func palettes(t *themes.Theme) (*themes.Palette, *themes.Palette) {
	switch {
	case t.Dark != nil:
		return &t.Palette, t.Dark
	case t == themes.Light:
		return &t.Palette, &themes.Dark.Palette
	}
	return &t.Palette, nil
}

// writeVars defines a --dg-<token> variable for every token on base. With
// a dark palette, the forced selector forces it and the viewer's
// prefers-color-scheme picks it when the system selector matches: a
// data-theme attribute on the page's root element, or an embed's theme
// attribute.
func writeVars(w *bufio.Writer, light, dark *themes.Palette, base, forced, system string) {
	fmt.Fprintf(w, "%s{%s}\n", base, vars(light))
	if dark == nil {
		return
	}
	d := vars(dark)
	fmt.Fprintf(w, "%s{%s}\n", forced, d)
	fmt.Fprintf(w, "@media (prefers-color-scheme: dark){%s{%s}}\n", system, d)
}

func vars(p *themes.Palette) string {
	var b strings.Builder
	for _, tok := range themes.Tokens() {
		c := p[tok]
		fmt.Fprintf(&b, "--dg-%s:#%02x%02x%02x;", tok, c.R, c.G, c.B)
	}
	scheme := "light"
	if isDark(p[themes.Background]) {
		scheme = "dark"
	}
	b.WriteString("color-scheme:" + scheme)
	return b.String()
}

// isDark reports whether c is a dark background, by relative luminance.
func isDark(c color.RGBA) bool {
	return 0.2126*float64(c.R)+0.7152*float64(c.G)+0.0722*float64(c.B) < 128
}

// texts collects the strings the page may show in tooltips, by weight:
// titles are semibold, the rest regular.
func texts(items []scene.Item) (regular, bold []string) {
	for _, it := range items {
		g, ok := it.(*scene.Group)
		if !ok {
			continue
		}
		if m := g.Meta; m != nil {
			bold = append(bold, m.Label)
			regular = append(regular, m.Desc, m.Tooltip, m.Link)
			if m.Kind == scene.KindEdge {
				regular = append(regular, m.Label)
			}
		}
		r, b := texts(g.Items)
		regular, bold = append(regular, r...), append(bold, b...)
	}
	return regular, bold
}

// compact drops indentation, blank lines and lines that are only a
// comment starting with marker from the page's stylesheet or script.
func compact(src, marker string) string {
	var b strings.Builder
	for ln := range strings.Lines(src) {
		t := strings.TrimLeft(ln, " \t")
		if strings.TrimSpace(t) == "" || strings.HasPrefix(t, marker) {
			continue
		}
		b.WriteString(t)
	}
	return b.String()
}

// Toolbar icons, after Lucide (ISC licence), drawn on a 24×24 grid.
const (
	iconSearch = `<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`
	iconMinus  = `<path d="M5 12h14"/>`
	iconPlus   = `<path d="M5 12h14M12 5v14"/>`
	iconFit    = `<path d="M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M16 21h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"/>`
	iconSun    = `<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/>`
	iconMoon   = `<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>`
	iconHelp   = `<circle cx="12" cy="12" r="10"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3M12 17h.01"/>`
	iconLink   = `<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>`
	iconClose  = `<path d="M18 6 6 18M6 6l12 12"/>`
)

func icon(cls, paths string) string {
	return `<svg class="` + cls + `" viewBox="0 0 24 24" aria-hidden="true">` + paths + `</svg>`
}

// writeToolbar writes the toolbar. An embed's is compact: no zoom level.
func writeToolbar(w *bufio.Writer, c chrome) {
	w.WriteString("<div id=\"dg-bar\" role=\"toolbar\" aria-label=\"Diagram controls\">\n")
	fmt.Fprintf(w, "<label class=\"dg-search\">%s<input id=\"dg-q\" type=\"search\" placeholder=\"Search\" aria-label=\"Search nodes\" autocomplete=\"off\" spellcheck=\"false\"><span id=\"dg-count\" aria-live=\"polite\"></span><kbd>/</kbd></label>\n", icon("", iconSearch))
	w.WriteString("<span class=\"dg-sep\"></span>\n")
	fmt.Fprintf(w, "<button type=\"button\" id=\"dg-out\" aria-label=\"Zoom out\" title=\"Zoom out (−)\">%s</button>\n", icon("", iconMinus))
	if !c.embed {
		w.WriteString("<button type=\"button\" id=\"dg-zoom\" aria-label=\"Actual size\" title=\"Actual size\">100%</button>\n")
	}
	fmt.Fprintf(w, "<button type=\"button\" id=\"dg-in\" aria-label=\"Zoom in\" title=\"Zoom in (+)\">%s</button>\n", icon("", iconPlus))
	fmt.Fprintf(w, "<button type=\"button\" id=\"dg-fit\" aria-label=\"Fit to screen\" title=\"Fit to screen (0)\">%s</button>\n", icon("", iconFit))
	if c.themed {
		w.WriteString("<span class=\"dg-sep\"></span>\n")
		fmt.Fprintf(w, "<button type=\"button\" id=\"dg-theme\" aria-label=\"Dark theme\" aria-pressed=\"false\" title=\"Dark theme\">%s%s</button>\n",
			icon("dg-i-light", iconSun), icon("dg-i-dark", iconMoon))
	}
	w.WriteString("<span class=\"dg-sep\"></span>\n")
	fmt.Fprintf(w, "<button type=\"button\" id=\"dg-keys\" aria-label=\"Keyboard shortcuts\" title=\"Keyboard shortcuts (?)\" aria-haspopup=\"dialog\" aria-controls=\"dg-help\" aria-expanded=\"false\">%s</button>\n", icon("", iconHelp))
	w.WriteString("</div>\n")
}

// writePathBanner writes the banner that describes a traced path; the
// script fills it in.
func writePathBanner(w *bufio.Writer) {
	fmt.Fprintf(w, "<div id=\"dg-path\" hidden><div class=\"p-text\" role=\"status\"><span class=\"p-route\"></span><span class=\"p-note\"></span></div>"+
		"<button type=\"button\" class=\"p-copy c-copy\">%s<span class=\"c-label\" aria-live=\"polite\">Copy link</span></button>"+
		"<button type=\"button\" class=\"p-x\" aria-label=\"Clear path\" title=\"Clear path (Esc)\">%s</button></div>\n",
		icon("", iconLink), icon("", iconClose))
}

// writeHelp writes the shortcut overlay, a dialog the script opens.
func writeHelp(w *bufio.Writer, c chrome) {
	fmt.Fprintf(w, "<dialog id=\"dg-help\" aria-labelledby=\"dg-help-h\"><div class=\"h-box\">\n<div class=\"h-head\"><h2 id=\"dg-help-h\">Keyboard shortcuts</h2><button type=\"button\" class=\"h-x\" aria-label=\"Close\" title=\"Close (Esc)\">%s</button></div>\n<dl>\n", icon("", iconClose))
	for _, r := range helpFor(c) {
		fmt.Fprintf(w, "<div><dt>%s</dt><dd>%s</dd></div>\n", keys(r[0]), keys(r[1]))
	}
	w.WriteString("</dl>\n")
	if c.linking {
		fmt.Fprintf(w, "<p class=\"h-note\">%s</p>\n", esc(helpNote))
	}
	w.WriteString("</div></dialog>\n")
}

// keys renders a help row's text with each [K] as a key.
func keys(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '[')
		j := strings.IndexByte(s, ']')
		if i < 0 || j < i {
			b.WriteString(esc(s))
			return b.String()
		}
		b.WriteString(esc(s[:i]))
		if k := s[i+1 : j]; k == "Mod" {
			b.WriteString(`<kbd class="dg-mod">Ctrl</kbd>`)
		} else {
			b.WriteString("<kbd>" + esc(k) + "</kbd>")
		}
		s = s[j+1:]
	}
}

// plain is a help row's text as the page shows it, for font subsetting.
func plain(s string) string {
	return strings.NewReplacer("[Mod]", "Ctrl⌘", "[", "", "]", "").Replace(s)
}

var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

func esc(s string) string { return escaper.Replace(s) }
