package svgicon_test

import (
	"errors"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/internal/svgicon"
)

// lucideDatabase is Lucide's "database" icon (ISC): a stroke icon that
// paints with currentColor and draws its cylinder with elliptical arcs.
const lucideDatabase = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-database"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5V19A9 3 0 0 0 21 19V5"/><path d="M3 12A9 3 0 0 0 21 12"/></svg>`

// lucideClock mixes element types and a relative arc-free path.
const lucideClock = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/><path d="M12 2a10 10 0 0 1 0 20"/></svg>`

func TestLucideStrokeIcon(t *testing.T) {
	ic, err := svgicon.Parse([]byte(lucideDatabase))
	if err != nil {
		t.Fatal(err)
	}
	if ic.ViewBox != [4]float64{0, 0, 24, 24} || len(ic.Shapes) != 3 {
		t.Fatalf("viewBox %v, %d shapes", ic.ViewBox, len(ic.Shapes))
	}
	for i, s := range ic.Shapes {
		if s.Fill.Kind != svgicon.None || s.Stroke.Kind != svgicon.CurrentColor || s.StrokeWidth != 2 ||
			s.Cap != svgicon.CapRound || s.Join != svgicon.JoinRound || s.Opacity != 1 || s.StrokeOpacity != 1 {
			t.Errorf("shape %d = %+v", i, s)
		}
	}
	// Second path: M3 5 L3 19, then the lower half of the 9×3 ellipse about
	// (12, 19) as two cubics through its bottom (12, 22), then L21 5.
	p := ic.Shapes[1].Path
	ops := []svgicon.Op{svgicon.MoveTo, svgicon.LineTo, svgicon.CubicTo, svgicon.CubicTo, svgicon.LineTo}
	if len(p) != len(ops) {
		t.Fatalf("path has %d commands", len(p))
	}
	for i, op := range ops {
		if p[i].Op != op {
			t.Fatalf("command %d is %v, want %v", i, p[i].Op, op)
		}
	}
	checkPt(t, p[2].Pts[2], 12, 22)
	checkPt(t, p[3].Pts[2], 21, 19)
	checkPt(t, p[2].Pts[0], 3, 19+3*0.5522847498)
	checkPt(t, p[4].Pts[0], 21, 5)

	ic, err = svgicon.Parse([]byte(lucideClock))
	if err != nil {
		t.Fatal(err)
	}
	if len(ic.Shapes) != 3 {
		t.Fatalf("clock: %d shapes", len(ic.Shapes))
	}
	arc := ic.Shapes[2].Path
	checkPt(t, arc[1].Pts[2], 22, 12) // clockwise half circle from the top passes the right
	checkPt(t, arc[len(arc)-1].Pts[2], 12, 22)
}

const logo = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 64 64">
  <title>Acme</title>
  <defs>
    <linearGradient id="base" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2="64">
      <stop offset="0" stop-color="#4f46e5"/>
      <stop offset="1" style="stop-color:#06b6d4;stop-opacity:.8"/>
    </linearGradient>
    <linearGradient id="badge" xlink:href="#base" gradientUnits="objectBoundingBox" x2="1" y2="0"/>
    <radialGradient id="glow" cx="50%" cy="40%" r="60%">
      <stop offset="0%" stop-color="white" stop-opacity="0.9"/>
      <stop offset="100%" stop-color="white" stop-opacity="0"/>
    </radialGradient>
  </defs>
  <rect width="64" height="64" rx="12" fill="url(#base)"/>
  <g transform="translate(32 28)">
    <circle r="14" fill="#fff"/>
    <circle r="14" fill="url(#glow)" opacity=".5"/>
  </g>
  <path d="M20 46h24l-4 8H24z" fill="url(#badge)" stroke="#1e1b4b" stroke-width="1.5" stroke-linejoin="round"/>
</svg>`

func TestMultiColourLogo(t *testing.T) {
	ic, err := svgicon.Parse([]byte(logo))
	if err != nil {
		t.Fatal(err)
	}
	if len(ic.Shapes) != 4 {
		t.Fatalf("%d shapes", len(ic.Shapes))
	}
	bg := ic.Shapes[0]
	if bg.Fill.Kind != svgicon.Linear {
		t.Fatalf("background fill = %+v", bg.Fill)
	}
	g := bg.Fill.Gradient
	if g.X1 != 0 || g.Y1 != 0 || g.X2 != 0 || g.Y2 != 64 || g.Transform != svgicon.Identity || len(g.Stops) != 2 {
		t.Errorf("background gradient = %+v", g)
	}
	if s := g.Stops[1]; s.Color != (color.RGBA{0x06, 0xb6, 0xd4, 255}) || math.Abs(s.Opacity-0.8) > 1e-9 {
		t.Errorf("stop = %+v", s)
	}

	glow := ic.Shapes[2]
	if glow.Fill.Kind != svgicon.Radial || glow.Opacity != 0.5 {
		t.Errorf("glow = %+v", glow)
	}
	// Bounding box of the circle is (-14,-14) 28×28, then translate(32 28).
	if want := (svgicon.Matrix{28, 0, 0, 28, 18, 14}); glow.Fill.Gradient.Transform != want {
		t.Errorf("glow transform = %v, want %v", glow.Fill.Gradient.Transform, want)
	}

	badge := ic.Shapes[3]
	bgr := badge.Fill.Gradient
	if badge.Fill.Kind != svgicon.Linear || bgr.X2 != 1 || bgr.Y2 != 0 || len(bgr.Stops) != 2 {
		t.Errorf("badge gradient = %+v", bgr)
	}
	// Path bbox is x 20..44, y 46..54.
	if want := (svgicon.Matrix{24, 0, 0, 8, 20, 46}); bgr.Transform != want {
		t.Errorf("badge transform = %v, want %v", bgr.Transform, want)
	}
	if badge.Stroke.Kind != svgicon.Solid || badge.Stroke.Color != (color.RGBA{0x1e, 0x1b, 0x4b, 255}) || badge.StrokeWidth != 1.5 {
		t.Errorf("badge stroke = %+v", badge)
	}
}

func checkPt(t *testing.T, p svgicon.Point, x, y float64) {
	t.Helper()
	if math.Abs(p.X-x) > 1e-6 || math.Abs(p.Y-y) > 1e-6 {
		t.Errorf("point = %v, want (%v, %v)", p, x, y)
	}
}

func TestCheckPNG(t *testing.T) {
	ok := icons.MustSVG("db", lucideDatabase)
	if err := ok.CheckPNG(); err != nil {
		t.Errorf("CheckPNG: %v", err)
	}
	const bad = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><text>hi</text></svg>`
	ic, err := icons.FromSVG(strings.NewReader(bad))
	if err != nil {
		t.Fatal(err)
	}
	err = ic.CheckPNG()
	var ue *svgicon.UnsupportedError
	if !errors.As(err, &ue) || ue.Feature != "text" {
		t.Fatalf("CheckPNG = %v, want an UnsupportedError for text", err)
	}
	err = ic.Named("greeting").CheckPNG()
	if !errors.As(err, &ue) || !strings.Contains(err.Error(), `"greeting"`) {
		t.Errorf("named CheckPNG = %v, want the icon's name", err)
	}
}
