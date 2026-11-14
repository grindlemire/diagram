package diagram

import (
	"fmt"

	"github.com/grindlemire/diagram/internal/text"
)

// RegisterFont adds a TrueType font to the fallback chain used for every
// diagram. Inter (embedded) draws each character it has; any other
// character is drawn with the first registered font that has a glyph for
// it, in registration order. Layout measures, the SVG embeds and the PNG
// draws with the same font, so text fits its box in every format. A
// character no font has is a collected error at render time.
//
// Fonts with the same family name (from the font's name table) form one
// family, and each weight uses the family's font closest to it: label
// text (SemiBold, 600) prefers a SemiBold or Bold file, other text
// (Regular, 400) a Regular one. A single-weight font serves both. Lines
// keep Inter's ascent and descent whatever font draws their glyphs.
//
// Only fonts with TrueType outlines (.ttf with a glyf table) are
// supported. CFF-based OpenType (.otf), collections (.ttc), WOFF files and
// colour emoji fonts (CBDT, sbix, COLR or SVG tables) are rejected with an
// error. Characters that need complex shaping (Arabic, Indic scripts) are
// drawn glyph by glyph and won't look right.
//
// Register fonts before rendering, typically from an init function.
// RegisterFont is safe for concurrent use; registering the same bytes
// twice has no effect. The data is copied.
//
// draw.io output can't embed fonts: it names Inter and leaves other
// characters to the viewer's system fonts.
func RegisterFont(data []byte) error {
	if err := text.Register(data); err != nil {
		return fmt.Errorf("diagram: RegisterFont: %w", err)
	}
	return nil
}

// MustRegisterFont is like RegisterFont but panics if the font can't be
// registered. It suits fonts embedded with go:embed.
func MustRegisterFont(data []byte) {
	if err := RegisterFont(data); err != nil {
		panic(err)
	}
}
