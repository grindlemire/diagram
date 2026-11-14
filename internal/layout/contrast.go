package layout

import (
	"image/color"
	"math"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/themes"
)

// Text colours for literal fills. They are literals themselves, so they
// look the same in every theme and in both palettes of an Auto theme.
var (
	darkText  = color.RGBA{0x0f, 0x17, 0x2a, 0xff}
	lightText = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

const (
	minTextContrast = 4.5 // WCAG AA for body text
	minIconContrast = 3.0 // WCAG AA for graphics
)

// textColors returns the label and description colours for an element
// drawn on fill, and may change look's icon colour. Themes pair their token
// fills with readable token text, so only a literal fill needs care: the
// label becomes dark or white, whichever contrasts more with the fill; the
// description is the label colour mixed towards the fill while it stays
// readable; and a theme icon colour that would be hard to see takes the
// label colour. A translucent literal fill is judged as it looks over the
// theme's background, in each palette the theme can show, and keeps the
// theme's colours if they read well on all of them. An explicit TextColor
// or IconColor always wins.
func (l *layouter) textColors(e *model.Element, fill, defText themes.Color, look *Look) (label, desc themes.Color) {
	label, desc = e.Style.TextColor, l.t.DescText
	if label.IsZero() {
		label = defText
	}
	f, ok := literal(fill)
	if !ok {
		return label, desc
	}
	pals := l.palettes()
	bgs := make([]color.RGBA, len(pals))
	for i, p := range pals {
		bgs[i] = over(f, p[themes.Background])
	}
	if f.A != 0xff && worst(label, pals, bgs) >= minTextContrast {
		return label, desc
	}
	text, bg := darkText, bgs[0]
	if worstOf(lightText, bgs) > worstOf(darkText, bgs) {
		text = lightText
	}
	for _, b := range bgs[1:] {
		if contrast(text, b) < contrast(text, bg) {
			bg = b
		}
	}
	if e.Style.TextColor.IsZero() {
		label = rgb(text)
	}
	desc = rgb(muted(text, bg))
	if e.IconColor.IsZero() && worst(l.t.IconColor, pals, bgs) < minIconContrast {
		look.IconColor = rgb(text)
	}
	return label, desc
}

// palettes lists the palettes the theme can be shown in.
func (l *layouter) palettes() []*themes.Palette {
	ps := []*themes.Palette{&l.t.Palette}
	if l.t.Dark != nil {
		ps = append(ps, l.t.Dark)
	}
	return ps
}

// worst is the lowest contrast c has with bgs[i] in palette pals[i].
func worst(c themes.Color, pals []*themes.Palette, bgs []color.RGBA) float64 {
	low := math.Inf(1)
	for i, p := range pals {
		v, ok := literal(c)
		if tok, isTok := c.Token(); isTok {
			v, ok = p[tok], true
		}
		if !ok {
			continue
		}
		low = math.Min(low, contrast(v, bgs[i]))
	}
	return low
}

func worstOf(c color.RGBA, bgs []color.RGBA) float64 {
	low := math.Inf(1)
	for _, b := range bgs {
		low = math.Min(low, contrast(c, b))
	}
	return low
}

// literal returns a literal colour's value.
func literal(c themes.Color) (color.RGBA, bool) {
	if _, tok := c.Token(); tok || c.IsZero() || c.Err() != nil {
		return color.RGBA{}, false
	}
	return c.Resolve(nil), true
}

// over composites the non-premultiplied colour c over an opaque bg.
func over(c, bg color.RGBA) color.RGBA {
	return mix(bg, c, float64(c.A)/255)
}

func rgb(c color.RGBA) themes.Color { return themes.RGB(c.R, c.G, c.B) }

// muted mixes text towards bg as far as it can while keeping body-text
// contrast, and never so far that it drops below graphics contrast.
func muted(text, bg color.RGBA) color.RGBA {
	floor := minTextContrast
	if contrast(text, bg) < floor {
		floor = minIconContrast
	}
	for k := 8; k > 0; k-- {
		c := mix(text, bg, float64(k)*0.05)
		if contrast(c, bg) >= floor {
			return c
		}
	}
	return text
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	ch := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return color.RGBA{ch(a.R, b.R), ch(a.G, b.G), ch(a.B, b.B), 0xff}
}

// contrast is the WCAG contrast ratio between two colours.
func contrast(a, b color.RGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}
