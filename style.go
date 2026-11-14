package diagram

import (
	"strings"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/themes"
)

// Color is a theme token or a literal colour. See package themes.
type Color = themes.Color

// Theme colour tokens. They resolve against the active theme.
var (
	Background = themes.Of(themes.Background)
	Surface    = themes.Of(themes.Surface)
	Foreground = themes.Of(themes.Foreground)
	Muted      = themes.Of(themes.Muted)
	Border     = themes.Of(themes.Border)
	Accent     = themes.Of(themes.Accent)
	Red        = themes.Of(themes.Red)
	Orange     = themes.Of(themes.Orange)
	Yellow     = themes.Of(themes.Yellow)
	Green      = themes.Of(themes.Green)
	Teal       = themes.Of(themes.Teal)
	Blue       = themes.Of(themes.Blue)
	Purple     = themes.Of(themes.Purple)
	Pink       = themes.Of(themes.Pink)
	Gray       = themes.Of(themes.Gray)
)

// Hex parses "#rgb", "#rrggbb" or "#rrggbbaa" into a literal colour. Using
// an invalid colour records an error on the diagram.
func Hex(s string) Color { return themes.Hex(s) }

// RGB returns a literal colour.
func RGB(r, g, b uint8) Color { return themes.RGB(r, g, b) }

// StyleOption sets visual properties. It applies to nodes, groups and
// edges. Combine several with Style.
type StyleOption struct {
	fns []func(*model.Style, *ctx)
}

func (s StyleOption) apply(st *model.Style, c *ctx) {
	for _, f := range s.fns {
		f(st, c)
	}
}

func (s StyleOption) applyNode(e *model.Element, c *ctx)  { s.apply(&e.Style, c) }
func (s StyleOption) applyGroup(e *model.Element, c *ctx) { s.apply(&e.Style, c) }
func (s StyleOption) applyEdge(e *model.Edge, c *ctx)     { s.apply(&e.Style, c) }

func styleFn(f func(*model.Style, *ctx)) StyleOption {
	return StyleOption{fns: []func(*model.Style, *ctx){f}}
}

// Style bundles style options into one reusable value. Options apply in
// order, so later ones win.
//
//	critical := diagram.Style(diagram.Stroke(diagram.Red), diagram.StrokeWidth(2))
func Style(opts ...StyleOption) StyleOption {
	var out StyleOption
	for _, o := range opts {
		out.fns = append(out.fns, o.fns...)
	}
	return out
}

// Fill sets the background colour.
func Fill(c Color) StyleOption {
	return styleFn(func(s *model.Style, x *ctx) {
		if x.checkColor("Fill", c) {
			s.Fill = c
		}
	})
}

// Stroke sets the border or line colour.
func Stroke(c Color) StyleOption {
	return styleFn(func(s *model.Style, x *ctx) {
		if x.checkColor("Stroke", c) {
			s.Stroke = c
		}
	})
}

// TextColor sets the label colour.
func TextColor(c Color) StyleOption {
	return styleFn(func(s *model.Style, x *ctx) {
		if x.checkColor("TextColor", c) {
			s.TextColor = c
		}
	})
}

// StrokeWidth sets the border or line width in pixels.
func StrokeWidth(px float64) StyleOption {
	return styleFn(func(s *model.Style, x *ctx) {
		if x.checkLen("StrokeWidth", px) {
			s.StrokeWidth = model.Some(px)
		}
	})
}

// Radius sets the corner radius.
func Radius(px float64) StyleOption {
	return styleFn(func(s *model.Style, x *ctx) {
		if x.checkLen("Radius", px) {
			s.Radius = model.Some(px)
		}
	})
}

// FontSize sets the label size in pixels.
func FontSize(px float64) StyleOption {
	return styleFn(func(s *model.Style, x *ctx) {
		if x.checkLen("FontSize", px) {
			s.FontSize = model.Some(px)
		}
	})
}

var (
	// Solid draws solid lines (the default).
	Solid = styleFn(func(s *model.Style, _ *ctx) { s.Dash = model.DashSolid })
	// Dashed draws dashed lines and borders.
	Dashed = styleFn(func(s *model.Style, _ *ctx) { s.Dash = model.DashDashed })
	// Dotted draws dotted lines and borders.
	Dotted = styleFn(func(s *model.Style, _ *ctx) { s.Dash = model.DashDotted })
	// Bold sets the label in the semibold face.
	Bold = styleFn(func(s *model.Style, _ *ctx) { s.Bold = model.Some(true) })
)

// Named returns a style bundle with a name, for styles that mean something
// in the diagram, such as "critical path" or "async". It applies like
// Style(opts...) and also records the name on each node, group or edge it
// is used on:
//
//   - A Legend lists every named style the diagram uses, with a sample
//     drawn in that style and the name beside it.
//   - In SVG and HTML output, elements using it get the CSS class
//     style-<name>, with the name lower-cased and runs of anything but
//     letters and digits turned into "-" ("critical path" becomes
//     style-critical-path), so a stylesheet can pick them out.
//
// Define each named style once and reuse the value. Two different bundles
// with the same name are an error where the second is used.
//
//	critical := diagram.Named("critical path", diagram.Stroke(diagram.Red), diagram.StrokeWidth(2))
//	d.Connect(api, db, critical)
func Named(name string, opts ...StyleOption) StyleOption {
	inner := Style(opts...)
	ns := &model.NamedStyle{Name: name, Src: model.Caller(1)}
	// The bundle's own properties, for legend samples. Errors in them are
	// recorded where the style is used.
	inner.apply(&ns.Style, &ctx{d: model.New("")})
	return styleFn(func(s *model.Style, c *ctx) {
		inner.apply(s, c)
		if strings.TrimSpace(name) == "" {
			c.errorf(`Named(%q): want a name`, name)
			return
		}
		if prev := c.d.UseNamed(ns); prev != nil {
			c.errorf("Named(%q): a different style has this name (%s)", name, prev.Src)
			return
		}
		s.Names = model.AddName(s.Names, name)
	})
}
