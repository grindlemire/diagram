package svgicon

import (
	"image/color"
	"testing"
)

func fillGradient(t *testing.T, body string) (*Gradient, Shape) {
	t.Helper()
	s := one(t, body)
	if s.Fill.Kind != Linear && s.Fill.Kind != Radial {
		t.Fatalf("fill kind = %v, want a gradient", s.Fill.Kind)
	}
	return s.Fill.Gradient, s
}

func TestLinearGradient(t *testing.T) {
	two := `<stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/>`
	t.Run("objectBoundingBox defaults", func(t *testing.T) {
		g, s := fillGradient(t, `<linearGradient id="g">`+two+`</linearGradient><rect x="10" y="20" width="100" height="50" fill="url(#g)"/>`)
		if s.Fill.Kind != Linear || g.X1 != 0 || g.Y1 != 0 || g.X2 != 1 || g.Y2 != 0 || g.Spread != SpreadPad {
			t.Errorf("got %+v", g)
		}
		assertMatrix(t, g.Transform, Matrix{100, 0, 0, 50, 10, 20})
	})
	t.Run("objectBoundingBox percentages and numbers", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="g" x1="25%" y1=".5" x2="1" y2="100%">`+two+`</linearGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if g.X1 != 0.25 || g.Y1 != 0.5 || g.X2 != 1 || g.Y2 != 1 {
			t.Errorf("got %+v", g)
		}
	})
	t.Run("bounding box uses curve extrema in user space", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="g">`+two+`</linearGradient><g transform="translate(5 0)"><path d="M0 0C0 10 10 10 10 0z" fill="url(#g)"/></g>`)
		assertMatrix(t, g.Transform, Matrix{10, 0, 0, 7.5, 5, 0})
	})
	t.Run("userSpaceOnUse", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="10" y1="20" x2="50%" y2="90">`+two+`</linearGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if g.X1 != 10 || g.Y1 != 20 || g.X2 != 50 || g.Y2 != 90 {
			t.Errorf("got %+v", g)
		}
		assertMatrix(t, g.Transform, Identity)
	})
	t.Run("userSpaceOnUse picks up the element's transforms", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="g" gradientUnits="userSpaceOnUse" x2="10">`+two+`</linearGradient>
			<g transform="translate(10 20)"><rect transform="scale(2)" width="1" height="1" fill="url(#g)"/></g>`)
		assertMatrix(t, g.Transform, Matrix{2, 0, 0, 2, 10, 20})
	})
	t.Run("gradientTransform composes inside bbox and ctm", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="g" gradientTransform="rotate(90)">`+two+`</linearGradient>
			<g transform="translate(1 2)"><rect width="10" height="20" fill="url(#g)"/></g>`)
		// ctm × bbox × rotate(90)
		assertMatrix(t, g.Transform, translate(1, 2).Mul(Matrix{10, 0, 0, 20, 0, 0}).Mul(Matrix{0, 1, -1, 0, 0, 0}))
	})
	t.Run("spreadMethod", func(t *testing.T) {
		for v, want := range map[string]Spread{"pad": SpreadPad, "reflect": SpreadReflect, "repeat": SpreadRepeat} {
			g, _ := fillGradient(t, `<linearGradient id="g" spreadMethod="`+v+`">`+two+`</linearGradient><rect width="1" height="1" fill="url(#g)"/>`)
			if g.Spread != want {
				t.Errorf("%s: spread = %v", v, g.Spread)
			}
		}
	})
	t.Run("stroke", func(t *testing.T) {
		s := one(t, `<linearGradient id="g">`+two+`</linearGradient><path d="M0 0h10v10" fill="none" stroke="url(#g)"/>`)
		if s.Stroke.Kind != Linear {
			t.Errorf("stroke = %+v", s.Stroke)
		}
		assertMatrix(t, s.Stroke.Gradient.Transform, Matrix{10, 0, 0, 10, 0, 0})
	})
	t.Run("fill and stroke get separate gradients", func(t *testing.T) {
		s := one(t, `<linearGradient id="g">`+two+`</linearGradient><path d="M0 0h10v10z" fill="url(#g)" stroke="url(#g)"/>`)
		if s.Fill.Gradient == s.Stroke.Gradient {
			t.Error("fill and stroke share a *Gradient")
		}
	})
}

func TestGradientStops(t *testing.T) {
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	g, _ := fillGradient(t, `<linearGradient id="g">
		<stop offset="10%" stop-color="red" stop-opacity=".5"/>
		<stop offset="0.05" style="stop-color: blue; stop-opacity: 50%"/>
		<stop offset="2" stop-color="rgba(0,0,255,.5)" stop-opacity=".5"/>
		<stop stop-color="transparent"/>
	</linearGradient><rect width="1" height="1" fill="url(#g)"/>`)
	want := []Stop{
		{Offset: 0.1, Color: red, Opacity: 0.5},
		{Offset: 0.1, Color: blue, Opacity: 0.5}, // offsets never decrease
		{Offset: 1, Color: blue, Opacity: 0.25},  // clamped; colour alpha folds in
		{Offset: 1, Color: color.RGBA{0, 0, 0, 255}, Opacity: 0},
	}
	if len(g.Stops) != len(want) {
		t.Fatalf("stops = %+v", g.Stops)
	}
	for i := range want {
		got := g.Stops[i]
		if !near(got.Offset, want[i].Offset) || got.Color != want[i].Color || !near(got.Opacity, want[i].Opacity) || got.CurrentColor {
			t.Errorf("stop %d = %+v, want %+v", i, got, want[i])
		}
	}
	t.Run("defaults", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="g"><stop/><stop offset="1" stop-color="red"/></linearGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if s := g.Stops[0]; s.Offset != 0 || s.Color != (color.RGBA{0, 0, 0, 255}) || s.Opacity != 1 {
			t.Errorf("default stop = %+v", s)
		}
	})
	t.Run("currentColor stop", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="g"><stop stop-color="currentColor"/><stop offset="1" stop-color="red"/></linearGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if !g.Stops[0].CurrentColor {
			t.Errorf("stop = %+v", g.Stops[0])
		}
	})
	t.Run("currentColor stop with color set on the gradient", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="g" color="blue"><stop stop-color="currentColor"/><stop offset="1"/></linearGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if g.Stops[0].CurrentColor || g.Stops[0].Color != blue {
			t.Errorf("stop = %+v", g.Stops[0])
		}
	})
	t.Run("stop-color inherit", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="g" stop-color="red"><stop stop-color="inherit"/><stop offset="1"/></linearGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if g.Stops[0].Color != red {
			t.Errorf("stop = %+v", g.Stops[0])
		}
	})
}

func TestGradientHref(t *testing.T) {
	t.Run("stops and attributes from the referenced gradient", func(t *testing.T) {
		g, _ := fillGradient(t, `<defs>
			<linearGradient id="base" gradientUnits="userSpaceOnUse" x1="1" y1="2" x2="3" y2="4" spreadMethod="reflect" gradientTransform="translate(5)">
				<stop stop-color="red"/><stop offset="1" stop-color="blue"/>
			</linearGradient>
			<linearGradient id="g" href="#base" x2="30"/>
		</defs><rect width="1" height="1" fill="url(#g)"/>`)
		if g.X1 != 1 || g.Y1 != 2 || g.X2 != 30 || g.Y2 != 4 || g.Spread != SpreadReflect || len(g.Stops) != 2 {
			t.Errorf("got %+v", g)
		}
		assertMatrix(t, g.Transform, translate(5, 0))
	})
	t.Run("xlink href and own stops win", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="base"><stop/><stop offset="1"/></linearGradient>
			<linearGradient id="g" xlink:href="#base"><stop offset="0" stop-color="red"/><stop offset=".5"/><stop offset="1"/></linearGradient>
			<rect width="1" height="1" fill="url(#g)"/>`)
		if len(g.Stops) != 3 {
			t.Errorf("got %d stops", len(g.Stops))
		}
	})
	t.Run("chain", func(t *testing.T) {
		g, _ := fillGradient(t, `<linearGradient id="a" y2="1"><stop/><stop offset="1"/></linearGradient>
			<linearGradient id="b" href="#a" x1=".5"/>
			<linearGradient id="c" href="#b" gradientUnits="objectBoundingBox"/>
			<rect width="1" height="1" fill="url(#c)"/>`)
		if g.X1 != 0.5 || g.Y2 != 1 || len(g.Stops) != 2 {
			t.Errorf("got %+v", g)
		}
	})
	t.Run("radial from linear takes stops and shared attributes only", func(t *testing.T) {
		g, s := fillGradient(t, `<linearGradient id="a" x1=".3" spreadMethod="repeat"><stop/><stop offset="1"/></linearGradient>
			<radialGradient id="r" href="#a"/>
			<rect width="1" height="1" fill="url(#r)"/>`)
		if s.Fill.Kind != Radial || g.Spread != SpreadRepeat || len(g.Stops) != 2 || g.CX != 0.5 {
			t.Errorf("got %+v", g)
		}
	})
	t.Run("radial fx inherits from radial", func(t *testing.T) {
		g, _ := fillGradient(t, `<radialGradient id="a" fx=".2"><stop/><stop offset="1"/></radialGradient>
			<radialGradient id="r" href="#a" cx=".4"/>
			<rect width="1" height="1" fill="url(#r)"/>`)
		if g.CX != 0.4 || g.FX != 0.2 || g.FY != 0.5 {
			t.Errorf("got %+v", g)
		}
	})
	t.Run("cycle", func(t *testing.T) {
		_, err := Parse([]byte(doc(`<linearGradient id="a" href="#b"/><linearGradient id="b" href="#a"/><rect width="1" height="1" fill="url(#a)"/>`)))
		if err == nil {
			t.Error("cycle parsed")
		}
	})
	t.Run("missing href", func(t *testing.T) {
		_, err := Parse([]byte(doc(`<linearGradient id="a" href="#nope"/><rect width="1" height="1" fill="url(#a)"/>`)))
		if f := unsupportedFeature(t, err); f != "missing reference #nope" {
			t.Errorf("feature = %q", f)
		}
	})
	t.Run("href to a non-gradient", func(t *testing.T) {
		_, err := Parse([]byte(doc(`<rect id="x"/><linearGradient id="a" href="#x"/><rect width="1" height="1" fill="url(#a)"/>`)))
		if f := unsupportedFeature(t, err); f != "gradient href to <rect>" {
			t.Errorf("feature = %q", f)
		}
	})
}

func TestRadialGradient(t *testing.T) {
	two := `<stop/><stop offset="1" stop-color="white"/>`
	t.Run("defaults", func(t *testing.T) {
		g, s := fillGradient(t, `<radialGradient id="g">`+two+`</radialGradient><circle cx="10" cy="10" r="10" fill="url(#g)"/>`)
		if s.Fill.Kind != Radial || g.CX != 0.5 || g.CY != 0.5 || g.R != 0.5 || g.FX != 0.5 || g.FY != 0.5 || g.FR != 0 {
			t.Errorf("got %+v", g)
		}
		assertMatrix(t, g.Transform, Matrix{20, 0, 0, 20, 0, 0})
	})
	t.Run("focal point", func(t *testing.T) {
		g, _ := fillGradient(t, `<radialGradient id="g" cx=".4" fx=".3" fy="20%" fr=".1">`+two+`</radialGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if g.CX != 0.4 || g.FX != 0.3 || g.FY != 0.2 || g.FR != 0.1 {
			t.Errorf("got %+v", g)
		}
	})
	t.Run("userSpaceOnUse percent radius uses the diagonal", func(t *testing.T) {
		g, _ := fillGradient(t, `<radialGradient id="g" gradientUnits="userSpaceOnUse" cx="50" cy="50" r="10%">`+two+`</radialGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if g.CX != 50 || !near(g.R, 10) || g.FX != 50 {
			t.Errorf("got %+v", g)
		}
	})
}

func TestDegenerateGradients(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	t.Run("no stops is none", func(t *testing.T) {
		ic := mustParse(t, doc(`<linearGradient id="g"/><rect width="1" height="1" fill="url(#g)"/>`))
		if len(ic.Shapes) != 0 {
			t.Errorf("got %d shapes", len(ic.Shapes))
		}
	})
	t.Run("one stop is solid", func(t *testing.T) {
		s := one(t, `<linearGradient id="g"><stop stop-color="red" stop-opacity=".5"/></linearGradient><rect width="1" height="1" fill="url(#g)" fill-opacity=".5"/>`)
		if s.Fill.Kind != Solid || s.Fill.Color != red {
			t.Errorf("fill = %+v", s.Fill)
		}
		assertNear(t, "fill opacity", s.FillOpacity, 0.25)
	})
	t.Run("zero-length vector paints the last stop", func(t *testing.T) {
		s := one(t, `<linearGradient id="g" x2="0"><stop/><stop offset="1" stop-color="red"/></linearGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if s.Fill.Kind != Solid || s.Fill.Color != red {
			t.Errorf("fill = %+v", s.Fill)
		}
	})
	t.Run("zero radius paints the last stop", func(t *testing.T) {
		s := one(t, `<radialGradient id="g" r="0"><stop/><stop offset="1" stop-color="red"/></radialGradient><rect width="1" height="1" fill="url(#g)"/>`)
		if s.Fill.Kind != Solid || s.Fill.Color != red {
			t.Errorf("fill = %+v", s.Fill)
		}
	})
	t.Run("bounding box gradient on a flat stroke drops it", func(t *testing.T) {
		ic := mustParse(t, doc(`<linearGradient id="g"><stop/><stop offset="1"/></linearGradient><path d="M0 5h10" fill="none" stroke="url(#g)"/>`))
		if len(ic.Shapes) != 0 {
			t.Errorf("got %+v", ic.Shapes)
		}
	})
	t.Run("userSpaceOnUse on a flat stroke works", func(t *testing.T) {
		s := one(t, `<linearGradient id="g" gradientUnits="userSpaceOnUse" x2="10"><stop/><stop offset="1"/></linearGradient><path d="M0 5h10" fill="none" stroke="url(#g)"/>`)
		if s.Stroke.Kind != Linear {
			t.Errorf("stroke = %+v", s.Stroke)
		}
	})
	t.Run("fallback when missing", func(t *testing.T) {
		s := one(t, `<rect width="1" height="1" fill="url(#nope) red"/>`)
		if s.Fill.Kind != Solid || s.Fill.Color != red {
			t.Errorf("fill = %+v", s.Fill)
		}
		ic := mustParse(t, doc(`<rect width="1" height="1" fill="url(#nope) none"/>`))
		if len(ic.Shapes) != 0 {
			t.Errorf("fallback none drew %+v", ic.Shapes)
		}
	})
}
