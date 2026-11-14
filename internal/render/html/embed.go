package html

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/grindlemire/diagram/internal/scene"
)

// EmbedOptions control an embeddable fragment.
type EmbedOptions struct {
	Title     string
	EmbedFont bool
	// Name prefixes the embed's deep links, #<name>:<id>; empty turns
	// deep links off.
	Name string
	// Height is a CSS height; empty sizes the embed by the diagram's
	// aspect ratio at the container's width.
	Height string
	// Toolbar shows the compact toolbar.
	Toolbar bool
	// Theme starts the embed in "light" or "dark"; empty follows the
	// viewer's prefers-color-scheme. Only themes with a dark palette
	// switch, and the toolbar's theme button switches them.
	Theme string
	// Transparent leaves the embed's background to the page.
	Transparent bool
	// Nonce is added to the fragment's <style> and <script> elements.
	Nonce string
	// NoScript leaves the script out: the page loads Script itself.
	NoScript bool
}

// RenderEmbed writes sc as a fragment for another page's body: a
// <diagram-view> element whose <template> child holds the view (the
// interactive SVG, its stylesheet and controls) and the embedded font
// subsets, preceded by a rule that sizes it before the script runs and,
// unless NoScript, followed by the script, which defines the element once
// per page however many fragments include it.
func RenderEmbed(w io.Writer, sc *scene.Scene, opt EmbedOptions) error {
	light, dark := palettes(sc.Theme)
	c := chrome{embed: true, toolbar: opt.Toolbar, themed: dark != nil && opt.Toolbar, linking: opt.Name != ""}
	body, err := interactiveSVG(sc, Options{Title: opt.Title, EmbedFont: opt.EmbedFont}, c)
	if err != nil {
		return err
	}
	svgText, fonts := extractFonts(string(body))
	stack := fontStackRe.FindStringSubmatch(svgText)
	// The SVG's own rules join the embed's stylesheet, which carries the
	// nonce.
	var svgCSS string
	if m := svgStyleRe.FindStringSubmatchIndex(svgText); m != nil {
		svgCSS = svgText[m[2]:m[3]]
		svgText = svgText[:m[0]] + svgText[m[1]:]
	}

	nonce := ""
	if opt.Nonce != "" {
		nonce = ` nonce="` + esc(opt.Nonce) + `"`
	}
	size := fmt.Sprintf("aspect-ratio:%s/%s", num(sc.Width), num(sc.Height))
	if opt.Height != "" {
		size = "height:" + opt.Height
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%+v", svgText, size, opt))
	key := hex.EncodeToString(sum[:5])

	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "<style%s>diagram-view[data-dg=\"%s\"]{display:block;%s}</style>\n", nonce, key, size)
	fmt.Fprintf(bw, "<diagram-view data-dg=\"%s\"", key)
	if opt.Name != "" {
		fmt.Fprintf(bw, " hash=\"%s\"", esc(opt.Name))
	}
	if opt.Theme != "" {
		fmt.Fprintf(bw, " theme=\"%s\"", esc(opt.Theme))
	}
	bw.WriteString("><template data-diagram>\n")

	fmt.Fprintf(bw, "<style%s>\n", nonce)
	writeVars(bw, light, dark, ":host", ":host([theme=dark])", ":host(:not([theme=light]))")
	bw.WriteString(compact(viewCSS, "/*"))
	bw.WriteString(compact(embedCSS, "/*"))
	if stack != nil {
		// The UI uses the diagram's fonts unless the page says otherwise.
		fmt.Fprintf(bw, ":host{--dg-ui-font:var(--diagram-font, %s)}\n", stack[1])
	}
	bw.WriteString(svgCSS)
	if opt.Transparent {
		bw.WriteString("#dg-frame{background:var(--diagram-background, transparent)}\n")
	}
	bw.WriteString("</style>\n<div id=\"dg-frame\">\n")
	label := "Diagram"
	if opt.Title != "" {
		label = opt.Title + " diagram"
	}
	fmt.Fprintf(bw, "<div id=\"dg-stage\" tabindex=\"0\" role=\"group\" aria-label=\"%s; press ? for shortcuts\">\n", esc(label))
	bw.WriteString(svgText)
	bw.WriteString("</div>\n")
	writeControls(bw, c)
	bw.WriteString("</div>\n")
	for _, f := range fonts {
		fmt.Fprintf(bw, "<script type=\"font/ttf;base64\" data-family=\"%s\" data-weight=\"%s\">%s</script>\n", f.family, f.weight, f.data)
	}
	bw.WriteString("</template></diagram-view>\n")
	if !opt.NoScript {
		fmt.Fprintf(bw, "<script%s>\n%s</script>\n", nonce, Script())
	}
	return bw.Flush()
}

var (
	scriptOnce sync.Once
	script     string
)

// Script returns the script that defines <diagram-view>. Running it more
// than once is harmless: it defines the element only if no other copy has.
func Script() string {
	scriptOnce.Do(func() {
		script = "(() => {\nif (!window.customElements || customElements.get('diagram-view')) return;\n" +
			compact(viewJS, "//") + compact(elementJS, "//") + "})();\n"
	})
	return script
}

type font struct{ family, weight, data string }

var (
	fontFaceRe  = regexp.MustCompile(`@font-face\{font-family:([^;]+);font-weight:(\d+);src:url\(data:font/ttf;base64,([A-Za-z0-9+/=]+)\) format\("truetype"\)\}\n`)
	fontStackRe = regexp.MustCompile(`<svg [^>]*font-family="([^"]*)"`)
	svgStyleRe  = regexp.MustCompile(`(?s)<style>\n(.*?)</style>\n`)
)

// extractFonts takes the @font-face rules out of the SVG, which a shadow
// root would ignore, and renames each family after a hash of its subsets,
// so the script can register them with the document without two embeds'
// different subsets of Inter colliding.
func extractFonts(s string) (string, []font) {
	var fonts []font
	for _, m := range fontFaceRe.FindAllStringSubmatch(s, -1) {
		fonts = append(fonts, font{m[1], m[2], m[3]})
	}
	if len(fonts) == 0 {
		return s, nil
	}
	s = fontFaceRe.ReplaceAllString(s, "")
	names := map[string]string{}
	for _, f := range fonts {
		names[f.family] = ""
	}
	for fam := range names {
		h := sha256.New()
		for _, f := range fonts {
			if f.family == fam {
				fmt.Fprintf(h, "%s\x00%s\x00", f.weight, f.data)
			}
		}
		names[fam] = "dg-" + hex.EncodeToString(h.Sum(nil)[:6])
	}
	for i := range fonts {
		fonts[i].family = names[fonts[i].family]
	}
	if m := fontStackRe.FindStringSubmatchIndex(s); m != nil {
		parts := strings.Split(s[m[2]:m[3]], ", ")
		for i, p := range parts {
			if n, ok := names[p]; ok {
				parts[i] = n
			}
		}
		s = s[:m[2]] + strings.Join(parts, ", ") + s[m[3]:]
	}
	return s, fonts
}

// num formats a size for CSS.
func num(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", f), "0"), ".")
}
