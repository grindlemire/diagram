package png

import (
	"image"
	"image/color"
	"math"

	"github.com/grindlemire/diagram/internal/svgicon"
)

// shadeGradient renders g over box as a premultiplied image. toPixels maps
// the icon's viewBox space to output pixels; current is the colour for
// currentColor stops; opacity multiplies every stop. It returns false when
// the gradient's transform is singular, which paints nothing.
func shadeGradient(g *svgicon.Gradient, kind svgicon.PaintKind, toPixels svgicon.Matrix, box image.Rectangle, current color.RGBA, opacity float64) (*image.RGBA, bool) {
	inv, ok := toPixels.Mul(g.Transform).Invert()
	if !ok {
		return nil, false
	}
	stops := make([]stop, len(g.Stops))
	for i, s := range g.Stops {
		c := s.Color
		a := s.Opacity * opacity
		if s.CurrentColor {
			c = current
			a *= float64(current.A) / 255
		}
		stops[i] = stop{s.Offset, [4]float64{float64(c.R), float64(c.G), float64(c.B), min(1, max(0, a))}}
	}
	img := image.NewRGBA(box)
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			gx, gy := inv.Apply(float64(x)+0.5, float64(y)+0.5)
			var t float64
			if kind == svgicon.Linear {
				t = linearT(g, gx, gy)
			} else {
				var ok bool
				if t, ok = radialT(g, gx, gy); !ok {
					continue
				}
			}
			c := colorAt(stops, spread(g.Spread, t))
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	return img, true
}

type stop struct {
	off  float64
	rgba [4]float64 // straight colour 0–255, alpha 0–1
}

func linearT(g *svgicon.Gradient, x, y float64) float64 {
	dx, dy := g.X2-g.X1, g.Y2-g.Y1
	return ((x-g.X1)*dx + (y-g.Y1)*dy) / (dx*dx + dy*dy)
}

// radialT solves for the largest t whose circle, interpolated from the
// focal circle (t=0) to the end circle (t=1), passes through the point
// with a non-negative radius, as SVG 2 and canvas do.
func radialT(g *svgicon.Gradient, x, y float64) (float64, bool) {
	cdx, cdy, dr := g.CX-g.FX, g.CY-g.FY, g.R-g.FR
	pdx, pdy := x-g.FX, y-g.FY
	a := cdx*cdx + cdy*cdy - dr*dr
	b := pdx*cdx + pdy*cdy + g.FR*dr
	c := pdx*pdx + pdy*pdy - g.FR*g.FR
	if math.Abs(a) < 1e-12 {
		if b == 0 {
			return 0, false
		}
		t := c / (2 * b)
		return t, g.FR+t*dr >= 0
	}
	disc := b*b - a*c
	if disc < 0 {
		return 0, false
	}
	sq := math.Sqrt(disc)
	t1, t2 := (b+sq)/a, (b-sq)/a
	if t1 < t2 {
		t1, t2 = t2, t1
	}
	if g.FR+t1*dr >= 0 {
		return t1, true
	}
	if g.FR+t2*dr >= 0 {
		return t2, true
	}
	return 0, false
}

func spread(s svgicon.Spread, t float64) float64 {
	switch s {
	case svgicon.SpreadRepeat:
		return t - math.Floor(t)
	case svgicon.SpreadReflect:
		t = math.Mod(math.Abs(t), 2)
		if t > 1 {
			t = 2 - t
		}
		return t
	}
	return max(0, min(1, t))
}

// colorAt interpolates the stops at t in straight sRGB and returns a
// premultiplied colour.
func colorAt(stops []stop, t float64) color.RGBA {
	var c [4]float64
	switch {
	case t <= stops[0].off:
		c = stops[0].rgba
	case t >= stops[len(stops)-1].off:
		c = stops[len(stops)-1].rgba
	default:
		i := 1
		for i < len(stops)-1 && stops[i].off <= t {
			i++
		}
		a, b := stops[i-1], stops[i]
		f := 0.0
		if b.off > a.off {
			f = (t - a.off) / (b.off - a.off)
		}
		for k := range c {
			c[k] = a.rgba[k] + (b.rgba[k]-a.rgba[k])*f
		}
	}
	al := c[3]
	return color.RGBA{
		R: uint8(c[0]*al + 0.5),
		G: uint8(c[1]*al + 0.5),
		B: uint8(c[2]*al + 0.5),
		A: uint8(al*255 + 0.5),
	}
}
