// Package svg renders a scene as an SVG document.
//
// With a single palette, colours are written as plain attributes for the
// widest viewer support. With an automatic dark palette, token colours
// become CSS classes whose values switch under prefers-color-scheme.
package svg

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/internal/num"
	"github.com/grindlemire/diagram/internal/scene"
	"github.com/grindlemire/diagram/internal/text"
	"github.com/grindlemire/diagram/themes"
)

// Options control SVG output.
type Options struct {
	Title     string
	EmbedFont bool

	// Interactive prepares the SVG for embedding in an interactive HTML
	// page. Elements carry data-* attributes describing them (data-kind,
	// data-id, data-parent, data-label, data-desc, data-tooltip, data-link,
	// and data-from and data-to on edges), nodes are focusable, tooltips
	// and links are left to the page instead of becoming <title> and <a>,
	// and every token colour becomes a class whose value is the CSS
	// variable --dg-<token>, which the page defines.
	Interactive bool
	// ExtraText and ExtraBold are text the page draws outside the SVG with
	// the same fonts, in the regular and semibold weights; their
	// characters are added to the embedded font subsets.
	ExtraText, ExtraBold []string
}

// systemFonts follows Inter and any fallback families in the font-family
// stack, for viewers that block the embedded fonts.
const systemFonts = "ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"

// Render writes sc as SVG.
func Render(w io.Writer, sc *scene.Scene, opt Options) error {
	r := &renderer{sc: sc, t: sc.Theme, auto: sc.Theme.Dark != nil || opt.Interactive, vars: opt.Interactive,
		icons: map[*icons.Icon]string{}, runes: map[fontKey]map[rune]bool{}}
	body := &bytes.Buffer{}
	r.out = body
	if !sc.Background.IsZero() {
		fmt.Fprintf(body, `<rect width="%s" height="%s"%s/>`+"\n", num.Fmt(sc.Width), num.Fmt(sc.Height), r.paint("fill", sc.Background, 0))
	}
	for _, it := range sc.Items {
		r.item(it, 0)
	}
	for _, s := range opt.ExtraText {
		r.addRunes(text.Regular, s, false)
	}
	for _, s := range opt.ExtraBold {
		r.addRunes(text.SemiBold, s, false)
	}

	bw := bufio.NewWriter(w)
	label := ""
	if r.vars && opt.Title != "" {
		label = fmt.Sprintf(` role="group" aria-label="%s"`, esc(opt.Title))
	}
	fmt.Fprintf(bw, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" font-family="%s"%s>`+"\n",
		num.Fmt(sc.Width), num.Fmt(sc.Height), num.Fmt(sc.Width), num.Fmt(sc.Height), r.fontStack(opt), label)
	if opt.Title != "" && !r.vars {
		fmt.Fprintf(bw, "<title>%s</title>\n", esc(opt.Title))
	}
	if css, err := r.css(opt); err != nil {
		return err
	} else if css != "" {
		fmt.Fprintf(bw, "<style>\n%s</style>\n", css)
	}
	if len(r.defs) > 0 {
		bw.WriteString("<defs>\n")
		for _, d := range r.defs {
			bw.WriteString(d)
		}
		bw.WriteString("</defs>\n")
	}
	bw.Write(body.Bytes())
	bw.WriteString("</svg>\n")
	return bw.Flush()
}

type renderer struct {
	sc    *scene.Scene
	t     *themes.Theme
	auto  bool // token colours are classes
	vars  bool // interactive: classes resolve to CSS variables
	out   *bytes.Buffer
	defs  []string
	icons map[*icons.Icon]string
	used  [3]map[themes.Token]bool  // fill, stroke, color classes in auto mode
	runes map[fontKey]map[rune]bool // the runes each font draws
}

// fontKey is a font at the weight it is declared under. A single-weight
// fallback font serves both weights.
type fontKey struct {
	w text.Weight
	f *text.Font
}

// paint returns the attribute (or class) for one colour property. kind is
// "fill", "stroke" or "color".
func (r *renderer) paint(kind string, c themes.Color, opacity float64) string {
	if c.IsZero() {
		if kind == "color" {
			return ""
		}
		return fmt.Sprintf(` %s="none"`, kind)
	}
	if tok, ok := c.Token(); ok && r.auto {
		idx := map[string]int{"fill": 0, "stroke": 1, "color": 2}[kind]
		if r.used[idx] == nil {
			r.used[idx] = map[themes.Token]bool{}
		}
		r.used[idx][tok] = true
		return fmt.Sprintf(` class="%s-%s"`, kind[:1], tok)
	}
	rgba := c.Resolve(r.t)
	s := fmt.Sprintf(` %s="%s"`, kind, hexOpaque(rgba.R, rgba.G, rgba.B))
	a := float64(rgba.A) / 255
	if opacity > 0 {
		a *= opacity
	}
	if a < 1 && kind != "color" {
		s += fmt.Sprintf(` %s-opacity="%s"`, kind, num.Fmt(a))
	}
	return s
}

func hexOpaque(r, g, b uint8) string { return fmt.Sprintf("#%02x%02x%02x", r, g, b) }

// paintAttrs writes fill and stroke attributes. Two classes from auto mode
// are merged into one class attribute.
func (r *renderer) paintAttrs(p scene.Paint) string {
	f := r.paint("fill", p.Fill, p.Opacity)
	s := ""
	if !p.Stroke.IsZero() && p.StrokeWidth > 0 {
		s = r.paint("stroke", p.Stroke, p.Opacity)
		s += fmt.Sprintf(` stroke-width="%s"`, num.Fmt(p.StrokeWidth))
		if p.Dash != nil {
			parts := make([]string, len(p.Dash))
			for i, d := range p.Dash {
				parts[i] = num.Fmt(d)
			}
			s += fmt.Sprintf(` stroke-dasharray="%s"`, strings.Join(parts, " "))
		}
		switch p.Cap {
		case scene.CapRound:
			s += ` stroke-linecap="round"`
		case scene.CapSquare:
			s += ` stroke-linecap="square"`
		}
		switch p.Join {
		case scene.JoinRound:
			s += ` stroke-linejoin="round"`
		case scene.JoinBevel:
			s += ` stroke-linejoin="bevel"`
		}
	}
	return mergeClasses(f + s)
}

var classRe = regexp.MustCompile(` class="([^"]*)"`)

func mergeClasses(attrs string) string {
	ms := classRe.FindAllStringSubmatch(attrs, -1)
	if len(ms) < 2 {
		return attrs
	}
	var names []string
	for _, m := range ms {
		names = append(names, m[1])
	}
	attrs = classRe.ReplaceAllString(attrs, "")
	return fmt.Sprintf(` class="%s"`, strings.Join(names, " ")) + attrs
}

func (r *renderer) indent(depth int) { r.out.WriteString(strings.Repeat("  ", depth)) }

func (r *renderer) item(it scene.Item, depth int) {
	r.indent(depth)
	switch v := it.(type) {
	case *scene.Group:
		r.group(v, depth)
	case *scene.Rect:
		rr := ""
		if v.R > 0 {
			rr = fmt.Sprintf(` rx="%s"`, num.Fmt(v.R))
		}
		fmt.Fprintf(r.out, `<rect x="%s" y="%s" width="%s" height="%s"%s%s/>`+"\n",
			num.Fmt(v.X), num.Fmt(v.Y), num.Fmt(v.W), num.Fmt(v.H), rr, r.paintAttrs(v.Paint))
	case *scene.Ellipse:
		if v.RX == v.RY {
			fmt.Fprintf(r.out, `<circle cx="%s" cy="%s" r="%s"%s/>`+"\n",
				num.Fmt(v.CX), num.Fmt(v.CY), num.Fmt(v.RX), r.paintAttrs(v.Paint))
		} else {
			fmt.Fprintf(r.out, `<ellipse cx="%s" cy="%s" rx="%s" ry="%s"%s/>`+"\n",
				num.Fmt(v.CX), num.Fmt(v.CY), num.Fmt(v.RX), num.Fmt(v.RY), r.paintAttrs(v.Paint))
		}
	case *scene.Shape:
		fmt.Fprintf(r.out, `<path d="%s"%s/>`+"\n", pathData(v.Path), r.paintAttrs(v.Paint))
	case *scene.Text:
		r.text(v)
	case *scene.Image:
		r.image(v)
	default:
		panic(fmt.Sprintf("svg: unknown scene item %T", it))
	}
}

// group writes a <g>. Standalone, a tooltip becomes its <title> and a link
// wraps it in an <a>; interactive, both become data attributes.
func (r *renderer) group(v *scene.Group, depth int) {
	attrs := ""
	if v.ID != "" {
		attrs += fmt.Sprintf(` id="%s"`, esc(v.ID))
	}
	if v.Class != "" {
		attrs += fmt.Sprintf(` class="%s"`, esc(v.Class))
	}
	m := v.Meta
	if m == nil {
		m = &scene.Meta{}
	}
	if r.vars {
		attrs += dataAttrs(v.ID, m)
		if m.Hidden {
			fmt.Fprintf(r.out, "<g%s/>\n", attrs)
			return
		}
	} else if m.Link != "" {
		fmt.Fprintf(r.out, "<a href=\"%s\">\n", esc(m.Link))
		depth++
		r.indent(depth)
	}
	fmt.Fprintf(r.out, "<g%s>\n", attrs)
	if m.Tooltip != "" && !r.vars {
		r.indent(depth + 1)
		fmt.Fprintf(r.out, "<title>%s</title>\n", esc(m.Tooltip))
	}
	for _, c := range v.Items {
		r.item(c, depth+1)
	}
	r.indent(depth)
	r.out.WriteString("</g>\n")
	if m.Link != "" && !r.vars {
		r.indent(depth - 1)
		r.out.WriteString("</a>\n")
	}
}

// dataAttrs describes an element for an interactive page. Nodes are
// focusable buttons named by their label and description.
func dataAttrs(id string, m *scene.Meta) string {
	if m.Kind == 0 {
		return ""
	}
	var b strings.Builder
	attr := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, ` %s="%s"`, k, esc(v))
		}
	}
	attr("data-kind", m.Kind.String())
	attr("data-id", id)
	attr("data-parent", m.Parent)
	attr("data-from", m.From)
	attr("data-to", m.To)
	attr("data-for", m.For)
	attr("data-label", m.Label)
	attr("data-desc", m.Desc)
	attr("data-tooltip", m.Tooltip)
	attr("data-link", m.Link)
	if m.Hidden {
		attr("data-hidden", "true")
	}
	if m.Kind == scene.KindNode {
		name := strings.Join(strings.Fields(m.Label), " ")
		if d := strings.Join(strings.Fields(m.Desc), " "); d != "" {
			if name != "" {
				name += ", "
			}
			name += d
		}
		b.WriteString(` tabindex="0" role="button"`)
		attr("aria-label", name)
	}
	return b.String()
}

func pathData(p scene.Path) string {
	var b strings.Builder
	pt := func(q scene.Point) { b.WriteString(num.Fmt(q.X) + " " + num.Fmt(q.Y)) }
	for i, c := range p {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch c.Op {
		case scene.MoveTo:
			b.WriteString("M")
			pt(c.Pts[0])
		case scene.LineTo:
			b.WriteString("L")
			pt(c.Pts[0])
		case scene.QuadTo:
			b.WriteString("Q")
			pt(c.Pts[0])
			b.WriteByte(' ')
			pt(c.Pts[1])
		case scene.CubicTo:
			b.WriteString("C")
			pt(c.Pts[0])
			b.WriteByte(' ')
			pt(c.Pts[1])
			b.WriteByte(' ')
			pt(c.Pts[2])
		case scene.Close:
			b.WriteString("Z")
		}
	}
	return b.String()
}

func (r *renderer) text(v *scene.Text) {
	anchor := ""
	switch v.Anchor {
	case scene.AnchorMiddle:
		anchor = ` text-anchor="middle"`
	case scene.AnchorEnd:
		anchor = ` text-anchor="end"`
	}
	weight := ""
	w := text.Regular
	if v.Weight == scene.SemiBold {
		weight = ` font-weight="600"`
		w = text.SemiBold
	}
	r.addRunes(w, v.Text, true)
	fmt.Fprintf(r.out, `<text x="%s" y="%s" font-size="%s"%s%s%s>%s</text>`+"\n",
		num.Fmt(v.X), num.Fmt(v.Y), num.Fmt(v.Size), weight, anchor, r.paint("fill", v.Color, 0), esc(v.Text))
}

// addRunes records the runes of s that each font of weight w draws, for
// subsetting. Runes no font has are drawn by Inter as '?' when drawn is
// set, and skipped otherwise.
func (r *renderer) addRunes(w text.Weight, s string, drawn bool) {
	face := text.FaceFor(w)
	for _, c := range s {
		fn := face.FontFor(c)
		if fn == nil {
			if !drawn {
				continue
			}
			fn = face.Fonts()[0] // Inter draws '?' in its place
		}
		k := fontKey{w, fn}
		if r.runes[k] == nil {
			r.runes[k] = map[rune]bool{}
		}
		r.runes[k][c] = true
	}
}

func (r *renderer) image(v *scene.Image) {
	id, ok := r.icons[v.Icon]
	if !ok {
		id = fmt.Sprintf("_icon-%d", len(r.icons)+1) // "_" never starts an element ID
		r.icons[v.Icon] = id
		r.defs = append(r.defs, symbol(id, v.Icon))
	}
	col := ""
	if v.Icon.Monochrome() {
		col = r.paint("color", v.Color, 0)
	}
	fmt.Fprintf(r.out, `<use href="#%s" x="%s" y="%s" width="%s" height="%s"%s/>`+"\n",
		id, num.Fmt(v.X), num.Fmt(v.Y), num.Fmt(v.W), num.Fmt(v.H), col)
}

// symbol wraps an icon in a <symbol>. SVG icons keep their content
// verbatim with IDs prefixed so icons can't collide; the root element's
// presentation attributes move to an inner <g>.
func symbol(id string, ic *icons.Icon) string {
	if ic.Kind() == icons.PNG {
		w, h := ic.Size()
		return fmt.Sprintf(`<symbol id="%s" viewBox="0 0 %s %s"><image width="%s" height="%s" href="data:image/png;base64,%s"/></symbol>`+"\n",
			id, num.Fmt(w), num.Fmt(h), num.Fmt(w), num.Fmt(h), base64.StdEncoding.EncodeToString(ic.Data()))
	}
	attrs, inner := splitRoot(ic.Data())
	inner = prefixIDs(inner, id+"-")
	vb := ic.ViewBox()
	return fmt.Sprintf(`<symbol id="%s" viewBox="%s %s %s %s"><g%s>%s</g></symbol>`+"\n",
		id, num.Fmt(vb[0]), num.Fmt(vb[1]), num.Fmt(vb[2]), num.Fmt(vb[3]), attrs, strings.TrimSpace(inner))
}

// rootSkip lists root attributes that don't belong on the inner group.
var rootSkip = map[string]bool{"width": true, "height": true, "viewBox": true, "x": true, "y": true, "version": true, "preserveAspectRatio": true, "id": true, "class": true}

// splitRoot returns the root <svg> element's presentation attributes and
// the raw markup between its start and end tags.
func splitRoot(data []byte) (attrs, inner string) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.RawToken()
		if err != nil {
			return "", ""
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		start := dec.InputOffset()
		var b strings.Builder
		for _, a := range se.Attr {
			if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" || rootSkip[a.Name.Local] {
				continue
			}
			name := a.Name.Local
			if a.Name.Space != "" {
				name = a.Name.Space + ":" + name
			}
			fmt.Fprintf(&b, ` %s="%s"`, name, esc(a.Value))
		}
		rest := data[start:]
		end := bytes.LastIndex(rest, []byte("</svg>"))
		if end < 0 { // self-closing root
			return b.String(), ""
		}
		return b.String(), string(rest[:end])
	}
}

var (
	idAttrRe = regexp.MustCompile(`\bid="([^"]+)"`)
	hrefRe   = regexp.MustCompile(`((?:xlink:)?href)="#([^"]+)"`)
	urlRe    = regexp.MustCompile(`url\(\s*['"]?#([^'")\s]+)['"]?\s*\)`)
)

func prefixIDs(s, prefix string) string {
	s = idAttrRe.ReplaceAllString(s, `id="`+prefix+`$1"`)
	s = hrefRe.ReplaceAllString(s, `$1="#`+prefix+`$2"`)
	s = urlRe.ReplaceAllString(s, `url(#`+prefix+`$1)`)
	return s
}

func (r *renderer) css(opt Options) (string, error) {
	var b strings.Builder
	if opt.EmbedFont {
		// Each font that drew text gets a subset of exactly the runes it
		// drew, declared under its own family, so the browser's per-glyph
		// fallback through the font-family stack picks the font layout
		// measured with.
		for _, w := range []text.Weight{text.Regular, text.SemiBold} {
			for _, fn := range text.FaceFor(w).Fonts() {
				used := r.runes[fontKey{w, fn}]
				if len(used) == 0 {
					continue
				}
				runes := slices.Sorted(maps.Keys(used))
				data, err := text.Subset(fn.Data, runes)
				if err != nil {
					return "", fmt.Errorf("svg: subset font %s: %w", fn.Family, err)
				}
				fmt.Fprintf(&b, "@font-face{font-family:%s;font-weight:%d;src:url(data:font/ttf;base64,%s) format(\"truetype\")}\n",
					fn.Family, w.CSS(), base64.StdEncoding.EncodeToString(data))
			}
		}
	}
	switch {
	case r.vars:
		b.WriteString(r.palette(nil))
	case r.auto:
		b.WriteString(r.palette(&r.t.Palette))
		fmt.Fprintf(&b, "@media (prefers-color-scheme: dark){\n%s}\n", r.palette(r.t.Dark))
	}
	return b.String(), nil
}

// fontStack returns the font-family list: Inter, then each fallback family
// that drew a glyph, in fallback order, then system fonts. Embedded
// fallbacks are named by their @font-face family; with FontByName they are
// named by the font's own family name.
func (r *renderer) fontStack(opt Options) string {
	stack := []string{"Inter"}
	// Both weights' chains list the fallback families in the same order.
	reg, semi := text.FaceFor(text.Regular).Fonts(), text.FaceFor(text.SemiBold).Fonts()
	for i := 1; i < min(len(reg), len(semi)); i++ {
		if len(r.runes[fontKey{text.Regular, reg[i]}]) == 0 && len(r.runes[fontKey{text.SemiBold, semi[i]}]) == 0 {
			continue
		}
		name := reg[i].Family
		if !opt.EmbedFont {
			name = cssName(reg[i].Name)
		}
		if name != "" && !slices.Contains(stack, name) {
			stack = append(stack, name)
		}
	}
	return strings.Join(append(stack, systemFonts), ", ")
}

// cssName quotes a font's family name for a CSS font-family list, dropping
// characters that could break out of it.
func cssName(s string) string {
	s = strings.Map(func(c rune) rune {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == ' ' || c == '-' || c == '_' {
			return c
		}
		return -1
	}, s)
	if s = strings.TrimSpace(s); s == "" {
		return ""
	}
	return "'" + s + "'"
}

// palette writes a class for each token colour used, with values from p,
// or referring to the --dg-<token> variables when p is nil.
func (r *renderer) palette(p *themes.Palette) string {
	var b strings.Builder
	for i, prop := range []string{"fill", "stroke", "color"} {
		for _, tok := range themes.Tokens() {
			if !r.used[i][tok] {
				continue
			}
			v := "var(--dg-" + tok.String() + ")"
			if p != nil {
				c := p[tok]
				v = hexOpaque(c.R, c.G, c.B)
			}
			fmt.Fprintf(&b, ".%s-%s{%s:%s}\n", prop[:1], tok, prop, v)
		}
	}
	return b.String()
}

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
