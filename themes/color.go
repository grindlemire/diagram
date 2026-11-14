// Package themes defines colours and the themes that resolve them.
//
// A Color is either a theme token, such as Accent or Red, or a literal RGB
// value. Tokens resolve against the active theme, so switching themes
// recolours a diagram without touching its code.
package themes

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// Token names a colour role that each theme defines.
type Token uint8

// Colour tokens. Level1 to Level3 are the backgrounds used for nested
// groups, from outermost to innermost.
const (
	tokenNone Token = iota
	Background
	Surface
	Foreground
	Muted
	Border
	Accent
	Level1
	Level2
	Level3
	Red
	Orange
	Yellow
	Green
	Teal
	Blue
	Purple
	Pink
	Gray
	numTokens
)

var tokenNames = [numTokens]string{
	"", "background", "surface", "foreground", "muted", "border", "accent",
	"level1", "level2", "level3",
	"red", "orange", "yellow", "green", "teal", "blue", "purple", "pink", "gray",
}

// String returns the token's lower-case name, which is also its CSS
// variable suffix in SVG output.
func (t Token) String() string {
	if t >= numTokens {
		return fmt.Sprintf("token(%d)", t)
	}
	return tokenNames[t]
}

// Tokens returns every defined token in a fixed order.
func Tokens() []Token {
	ts := make([]Token, 0, numTokens-1)
	for t := tokenNone + 1; t < numTokens; t++ {
		ts = append(ts, t)
	}
	return ts
}

type colorKind uint8

const (
	kindUnset colorKind = iota
	kindToken
	kindLiteral
	kindInvalid
)

// Color is a theme token or a literal colour. The zero Color is unset.
type Color struct {
	kind colorKind
	tok  Token
	rgba color.RGBA
	bad  string
}

// Of returns the Color for a theme token.
func Of(t Token) Color { return Color{kind: kindToken, tok: t} }

// RGB returns an opaque literal colour.
func RGB(r, g, b uint8) Color {
	return Color{kind: kindLiteral, rgba: color.RGBA{r, g, b, 0xff}}
}

// RGBA returns a literal colour with alpha. The components are not
// premultiplied.
func RGBA(r, g, b, a uint8) Color {
	return Color{kind: kindLiteral, rgba: color.RGBA{r, g, b, a}}
}

// Hex parses "#rgb", "#rrggbb" or "#rrggbbaa". An invalid string gives a
// Color whose Err reports the problem; using it in a diagram records that
// error instead of panicking.
func Hex(s string) Color {
	h := strings.TrimPrefix(s, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) == 6 {
		h += "ff"
	}
	if len(h) != 8 {
		return Color{kind: kindInvalid, bad: s}
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return Color{kind: kindInvalid, bad: s}
	}
	return Color{kind: kindLiteral, rgba: color.RGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}}
}

// IsZero reports whether the colour is unset.
func (c Color) IsZero() bool { return c.kind == kindUnset }

// Token returns the colour's token, if it is one.
func (c Color) Token() (Token, bool) { return c.tok, c.kind == kindToken }

// Err reports why a colour built with Hex is invalid.
func (c Color) Err() error {
	if c.kind == kindInvalid {
		return fmt.Errorf("invalid hex colour %q", c.bad)
	}
	return nil
}

// Resolve returns the concrete colour under theme t. Unset and invalid
// colours resolve to transparent.
func (c Color) Resolve(t *Theme) color.RGBA {
	switch c.kind {
	case kindToken:
		return t.Palette[c.tok]
	case kindLiteral:
		return c.rgba
	}
	return color.RGBA{}
}

// HexString formats an RGBA colour as #rrggbb, or #rrggbbaa when it is not
// opaque.
func HexString(c color.RGBA) string {
	if c.A == 0xff {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}
