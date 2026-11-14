// Package text measures and wraps text with the embedded Inter font and any
// registered fallback fonts, and provides glyph outlines for rasterization.
package text

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

//go:embed fonts/Inter-Regular.ttf
var interRegular []byte

//go:embed fonts/Inter-SemiBold.ttf
var interSemiBold []byte

// Weight selects a face.
type Weight uint8

const (
	Regular Weight = iota
	SemiBold
)

// CSS returns the CSS font-weight of w.
func (w Weight) CSS() int {
	if w == SemiBold {
		return 600
	}
	return 400
}

// Font is one parsed TrueType font with cached glyph lookups. It is safe
// for concurrent use.
type Font struct {
	// Family is the CSS family name an embedded subset of the font is
	// declared under: "Inter", or "diagram-fallback-N" for the Nth
	// registered fallback family.
	Family string
	// Name is the family name from the font's name table, for referring to
	// an installed copy of the font.
	Name string
	// Weight is the font's OS/2 usWeightClass (400 when absent).
	Weight int
	Data   []byte // raw TTF, for subsetting

	font *sfnt.Font
	upem float64

	mu      sync.Mutex
	buf     sfnt.Buffer
	glyphs  map[rune]glyph
	ascent  float64 // font units
	descent float64 // font units, positive
}

// glyph is a cached cmap lookup. idx 0 means the font has no glyph.
type glyph struct {
	idx sfnt.GlyphIndex
	adv float64 // font units
}

// parse loads a TrueType font.
func parse(family string, data []byte) (*Font, error) {
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil, err
	}
	fn := &Font{Family: family, Data: data, font: f, upem: float64(f.UnitsPerEm()), glyphs: map[rune]glyph{}, Weight: 400}
	m, err := f.Metrics(&fn.buf, fixed.I(int(f.UnitsPerEm())), font.HintingNone)
	if err != nil {
		return nil, err
	}
	fn.ascent = float64(m.Ascent) / 64
	fn.descent = float64(m.Descent) / 64
	for _, id := range []sfnt.NameID{sfnt.NameIDTypographicFamily, sfnt.NameIDFamily} {
		if s, err := f.Name(&fn.buf, id); err == nil && s != "" {
			fn.Name = s
			break
		}
	}
	return fn, nil
}

// lookup returns r's glyph, caching it.
func (f *Font) lookup(r rune) glyph {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g, ok := f.glyphs[r]; ok {
		return g
	}
	var g glyph
	if idx, err := f.font.GlyphIndex(&f.buf, r); err == nil && idx != 0 {
		g.idx = idx
		if adv, err := f.font.GlyphAdvance(&f.buf, idx, fixed.I(int(f.upem)), font.HintingNone); err == nil {
			g.adv = float64(adv) / 64
		}
	}
	f.glyphs[r] = g
	return g
}

// Has reports whether the font has a glyph for r.
func (f *Font) Has(r rune) bool { return f.lookup(r).idx != 0 }

// outline returns glyph idx scaled to size pixels.
func (f *Font) outline(idx sfnt.GlyphIndex, size float64) (sfnt.Segments, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	segs, err := f.font.LoadGlyph(&f.buf, idx, fixed.Int26_6(size*64), nil)
	if err != nil {
		return nil, err
	}
	out := make(sfnt.Segments, len(segs))
	copy(out, segs) // segs aliases f.buf
	return out, nil
}

// Face is the text face for one weight: Inter followed by the registered
// fallback fonts. Each rune is drawn with the first font in the chain that
// has a glyph for it. It is safe for concurrent use.
type Face struct {
	weight Weight
	inter  *Font
}

var (
	loadOnce sync.Once
	faces    [2]*Face
	loadErr  error
)

func load() {
	loadOnce.Do(func() {
		var reg, semi *Font
		if reg, loadErr = parse("Inter", interRegular); loadErr != nil {
			return
		}
		if semi, loadErr = parse("Inter", interSemiBold); loadErr != nil {
			return
		}
		reg.Weight, semi.Weight = 400, 600
		faces[Regular] = &Face{weight: Regular, inter: reg}
		faces[SemiBold] = &Face{weight: SemiBold, inter: semi}
		registry.Store(&fallbacks{chains: [2][]*Font{{reg}, {semi}}})
	})
	if loadErr != nil {
		panic(fmt.Sprintf("text: embedded font: %v", loadErr)) // build is broken
	}
}

// FaceFor returns the face for a weight.
func FaceFor(w Weight) *Face {
	load()
	return faces[w]
}

// Fonts returns the fallback chain: Inter, then the registered fallback
// families' best match for the face's weight, in registration order. The
// slice must not be modified.
func (f *Face) Fonts() []*Font { return registry.Load().chains[f.weight] }

// FontFor returns the font that draws r, or nil when no font in the chain
// has a glyph for it.
func (f *Face) FontFor(r rune) *Font {
	fn, _ := f.resolve(r)
	return fn
}

func (f *Face) resolve(r rune) (*Font, glyph) {
	if g := f.inter.lookup(r); g.idx != 0 {
		return f.inter, g
	}
	for _, fn := range f.Fonts()[1:] {
		if g := fn.lookup(r); g.idx != 0 {
			return fn, g
		}
	}
	return nil, glyph{}
}

// draw returns the font and glyph that draw r. A rune no font has is drawn
// as Inter's '?', except default-ignorable code points (variation
// selectors, joiners and other format characters), which draw nothing.
func (f *Face) draw(r rune) (*Font, glyph, bool) {
	if fn, g := f.resolve(r); fn != nil {
		return fn, g, true
	}
	if ignorable(r) {
		return nil, glyph{}, false
	}
	return f.inter, f.inter.lookup('?'), true
}

// Ascent returns the distance from the baseline to the top of the line at
// the given size in pixels. Lines use Inter's metrics whatever fonts draw
// their glyphs.
func (f *Face) Ascent(size float64) float64 { return f.inter.ascent * size / f.inter.upem }

// Descent returns the distance from the baseline to the bottom of the line.
func (f *Face) Descent(size float64) float64 { return f.inter.descent * size / f.inter.upem }

// Width returns the width of s at size pixels. Inter's kerning lives in
// GPOS, which sfnt doesn't read, so widths are unkerned; the difference is
// well under a pixel for typical labels.
func (f *Face) Width(s string, size float64) float64 {
	var w float64
	for _, r := range s {
		if fn, g, ok := f.draw(r); ok {
			w += g.adv * size / fn.upem
		}
	}
	return w
}

// Segments returns the outline of r scaled to size pixels, with y pointing
// down and the origin at the glyph's baseline start, plus its advance. The
// outline comes from the first font in the chain with a glyph for r.
func (f *Face) Segments(r rune, size float64) (sfnt.Segments, float64, error) {
	fn, g, ok := f.draw(r)
	if !ok {
		return nil, 0, nil
	}
	segs, err := fn.outline(g.idx, size)
	if err != nil {
		return nil, 0, err
	}
	return segs, g.adv * size / fn.upem, nil
}

// Wrap breaks s into lines no wider than max pixels, breaking at spaces and
// at explicit newlines. A single word wider than max gets its own line
// rather than being split. max <= 0 disables wrapping.
func (f *Face) Wrap(s string, size, max float64) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.FieldsFunc(para, unicode.IsSpace)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			cand := line + " " + w
			if max > 0 && f.Width(cand, size) > max {
				lines = append(lines, line)
				line = w
				continue
			}
			line = cand
		}
		lines = append(lines, line)
	}
	return lines
}

// Missing returns the runes of s, each once in order of appearance, that no
// font can draw at some weight. Whitespace, control characters and
// default-ignorable code points are never missing.
func Missing(s string) []rune {
	var out []rune
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) || ignorable(r) {
			continue
		}
		if FaceFor(Regular).FontFor(r) != nil && FaceFor(SemiBold).FontFor(r) != nil {
			continue
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// ignorable approximates Unicode's Default_Ignorable_Code_Point: code
// points that render as nothing when a font lacks them.
func ignorable(r rune) bool {
	if unicode.In(r, unicode.Variation_Selector, unicode.Other_Default_Ignorable_Code_Point) {
		return true
	}
	return unicode.Is(unicode.Cf, r) && !unicode.Is(unicode.Prepended_Concatenation_Mark, r)
}
