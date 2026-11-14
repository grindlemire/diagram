// Package png rasterizes a scene to a PNG image.
//
// Paths are flattened to polylines at 0.1px tolerance in output pixels,
// strokes are converted to fill outlines by the stroker in stroke.go, and
// outlines become coverage masks with x/image/vector (nonzero) or a small
// accumulation rasterizer (even-odd). Colours are composited source-over
// in premultiplied 8-bit RGBA. Output is deterministic on one machine;
// floating-point differences between architectures (arm64 fuses
// multiply-adds) can change a few pixels slightly.
package png

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"

	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/internal/scene"
	"github.com/grindlemire/diagram/internal/svgicon"
	"github.com/grindlemire/diagram/internal/text"
	"github.com/grindlemire/diagram/themes"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/f64"
)

// Options control PNG output.
type Options struct {
	Scale float64 // pixel density; 0 means 2
}

// Render rasterizes sc and writes it as PNG.
func Render(w io.Writer, sc *scene.Scene, opt Options) error {
	img, err := Rasterize(sc, opt)
	if err != nil {
		return err
	}
	return png.Encode(w, img)
}

// Rasterize draws sc into a new image of ceil(Width·Scale) ×
// ceil(Height·Scale) pixels.
func Rasterize(sc *scene.Scene, opt Options) (*image.RGBA, error) {
	s := opt.Scale
	if s == 0 {
		s = 2
	}
	if s < 0 || math.IsNaN(s) || math.IsInf(s, 0) {
		return nil, fmt.Errorf("png: bad scale %v", opt.Scale)
	}
	W, H := pixels(sc.Width*s), pixels(sc.Height*s)
	if W <= 0 || H <= 0 {
		return nil, fmt.Errorf("png: empty image (%v×%v at scale %v)", sc.Width, sc.Height, s)
	}
	if int64(W)*int64(H) > 1<<28 {
		return nil, fmt.Errorf("png: image too large (%d×%d pixels)", W, H)
	}
	r := &renderer{
		img:   image.NewRGBA(image.Rect(0, 0, W, H)),
		theme: sc.Theme,
		m:     svgicon.Matrix{s, 0, 0, s, 0, 0},
		scale: s,
		icons: map[*icons.Icon]*svgicon.Icon{},
	}
	if r.theme == nil {
		return nil, errors.New("png: scene has no theme")
	}
	if !sc.Background.IsZero() {
		bg := premul(sc.Background.Resolve(r.theme), 1)
		draw.Draw(r.img, r.img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	}
	for _, it := range sc.Items {
		if err := r.item(it); err != nil {
			return nil, err
		}
	}
	return r.img, nil
}

// pixels rounds a size up, ignoring float noise just above an integer.
func pixels(v float64) int { return int(math.Ceil(v - 1e-9)) }

const textGammaExp = 0.75

type renderer struct {
	img   *image.RGBA
	theme *themes.Theme
	m     svgicon.Matrix // scene → output pixels
	scale float64
	ras   rasterizer
	icons map[*icons.Icon]*svgicon.Icon
}

func (r *renderer) item(it scene.Item) error {
	switch v := it.(type) {
	case *scene.Group:
		for _, c := range v.Items {
			if err := r.item(c); err != nil {
				return err
			}
		}
	case *scene.Rect:
		if v.W > 0 && v.H > 0 {
			r.shape(rectPath(v.X, v.Y, v.W, v.H, v.R), v.Paint)
		}
	case *scene.Ellipse:
		if v.RX > 0 && v.RY > 0 {
			r.shape(ellipsePath(v.CX, v.CY, v.RX, v.RY), v.Paint)
		}
	case *scene.Shape:
		r.shape(v.Path, v.Paint)
	case *scene.Text:
		return r.text(v)
	case *scene.Image:
		return r.image(v)
	default:
		return fmt.Errorf("png: unknown scene item %T", it)
	}
	return nil
}

func opacity(p scene.Paint) float64 {
	if p.Opacity == 0 {
		return 1
	}
	return p.Opacity
}

// shape fills and then strokes a scene path, as SVG does.
func (r *renderer) shape(p scene.Path, pt scene.Paint) {
	polys := flatten(p, r.m)
	op := opacity(pt)
	if !pt.Fill.IsZero() {
		r.fillColor(closedContours(polys), false, nil, premul(pt.Fill.Resolve(r.theme), op))
	}
	if !pt.Stroke.IsZero() && pt.StrokeWidth > 0 {
		st := strokeStyle{width: pt.StrokeWidth * r.scale, cap: pt.Cap, join: pt.Join, miterLimit: 4}
		for _, d := range pt.Dash {
			st.dash = append(st.dash, d*r.scale)
		}
		r.fillColor(stroke(polys, st), false, nil, premul(pt.Stroke.Resolve(r.theme), op))
	}
}

// closedContours returns the subpaths for filling, which closes them all.
func closedContours(polys []poly) [][]pt {
	out := make([][]pt, 0, len(polys))
	for _, p := range polys {
		if len(p.pts) > 2 {
			out = append(out, p.pts)
		}
	}
	return out
}

func (r *renderer) fillColor(contours [][]pt, evenOdd bool, clip *rect, c color.RGBA) {
	if c.A == 0 || len(contours) == 0 {
		return
	}
	mask, box := r.ras.fill(contours, evenOdd, r.img.Bounds(), clip)
	paintMask(r.img, box, mask, c)
}

// text fills the glyph outlines of one line. Glyphs advance by the same
// unkerned widths layout measured.
func (r *renderer) text(v *scene.Text) error {
	if v.Text == "" || v.Size <= 0 || v.Color.IsZero() {
		return nil
	}
	face := text.FaceFor(text.Regular)
	if v.Weight == scene.SemiBold {
		face = text.FaceFor(text.SemiBold)
	}
	width := v.Width
	if width == 0 {
		width = face.Width(v.Text, v.Size)
	}
	x := v.X
	switch v.Anchor {
	case scene.AnchorMiddle:
		x -= width / 2
	case scene.AnchorEnd:
		x -= width
	}
	// Ask for outlines at the output size so their 1/64 px grid is fine.
	px := v.Size * r.scale
	ox, oy := r.m.Apply(x, v.Y)
	var path scene.Path
	pen := 0.0
	for _, c := range v.Text {
		segs, adv, err := face.Segments(c, px)
		if err != nil {
			return fmt.Errorf("png: glyph %q: %w", c, err)
		}
		glyphPath(&path, segs, ox+pen, oy)
		pen += adv
	}
	polys := flatten(path, svgicon.Identity)
	c := premul(v.Color.Resolve(r.theme), 1)
	if c.A == 0 {
		return nil
	}
	mask, box := r.ras.fill(closedContours(polys), false, r.img.Bounds(), nil)
	if mask == nil {
		return nil
	}
	for i, a := range mask.Pix {
		mask.Pix[i] = textGamma[a]
	}
	paintMask(r.img, box, mask, c)
	return nil
}

// textGamma thickens glyph edges slightly. Browsers darken text edges
// (stem darkening and gamma-adjusted coverage), so plain linear coverage
// looks about a tenth lighter than the SVG next to it.
var textGamma = func() (lut [256]uint8) {
	for i := range lut {
		lut[i] = uint8(255*math.Pow(float64(i)/255, textGammaExp) + 0.5)
	}
	return lut
}()

func glyphPath(p *scene.Path, segs sfnt.Segments, ox, oy float64) {
	f := func(i int, s sfnt.Segment) (float64, float64) {
		return ox + float64(s.Args[i].X)/64, oy + float64(s.Args[i].Y)/64
	}
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			p.MoveTo(f(0, s))
		case sfnt.SegmentOpLineTo:
			p.LineTo(f(0, s))
		case sfnt.SegmentOpQuadTo:
			x0, y0 := f(0, s)
			x1, y1 := f(1, s)
			p.QuadTo(x0, y0, x1, y1)
		case sfnt.SegmentOpCubeTo:
			x0, y0 := f(0, s)
			x1, y1 := f(1, s)
			x2, y2 := f(2, s)
			p.CubicTo(x0, y0, x1, y1, x2, y2)
		}
	}
}

// fit maps a w×h content box into the image box like preserveAspectRatio
// xMidYMid meet, returning the scale and the content origin in scene
// coordinates.
func fit(v *scene.Image, w, h float64) (s, x, y float64) {
	s = min(v.W/w, v.H/h)
	return s, v.X + (v.W-w*s)/2, v.Y + (v.H-h*s)/2
}

func (r *renderer) image(v *scene.Image) error {
	ic := v.Icon
	if ic == nil || v.W <= 0 || v.H <= 0 {
		return nil
	}
	if ic.Kind() == icons.PNG {
		return r.pngIcon(v)
	}
	parsed, ok := r.icons[ic]
	if !ok {
		var err error
		if parsed, err = svgicon.Parse(ic.Data()); err != nil {
			return iconErr(ic, err)
		}
		r.icons[ic] = parsed
	}
	return r.svgIcon(v, parsed)
}

func iconErr(ic *icons.Icon, err error) error {
	if ic.Name() == "" {
		return fmt.Errorf("icon: %w", err)
	}
	return fmt.Errorf("icon %q: %w", ic.Name(), err)
}

func (r *renderer) pngIcon(v *scene.Image) error {
	src := v.Icon.Image()
	if src == nil {
		return iconErr(v.Icon, errors.New("png icon has no image"))
	}
	b := src.Bounds()
	if b.Empty() {
		return nil
	}
	s, x, y := fit(v, float64(b.Dx()), float64(b.Dy()))
	k := s * r.scale
	ox, oy := r.m.Apply(x, y)
	// Maps source pixels (from b.Min) to output pixels.
	m := f64.Aff3{k, 0, ox - k*float64(b.Min.X), 0, k, oy - k*float64(b.Min.Y)}
	xdraw.CatmullRom.Transform(r.img, m, src, b, xdraw.Over, nil)
	return nil
}

func (r *renderer) svgIcon(v *scene.Image, ic *svgicon.Icon) error {
	vb := ic.ViewBox
	s, x, y := fit(v, vb[2], vb[3])
	toPixels := r.m.Mul(svgicon.Matrix{s, 0, 0, s, x - vb[0]*s, y - vb[1]*s})
	cx0, cy0 := r.m.Apply(v.X, v.Y)
	cx1, cy1 := r.m.Apply(v.X+v.W, v.Y+v.H)
	clip := &rect{cx0, cy0, cx1, cy1}
	current := color.RGBA{A: 255} // SVG's default colour is black
	if !v.Color.IsZero() {
		current = v.Color.Resolve(r.theme)
	}
	for i := range ic.Shapes {
		sh := &ic.Shapes[i]
		path := iconPath(sh.Path)
		polys := flatten(path, toPixels)
		hasFill, hasStroke := sh.Fill.Kind != svgicon.None, sh.Stroke.Kind != svgicon.None && sh.StrokeWidth > 0
		var strokeC [][]pt
		if hasStroke {
			st := strokeStyle{
				width:      sh.StrokeWidth * s * r.scale,
				cap:        scene.LineCap(sh.Cap),
				join:       scene.LineJoin(sh.Join),
				miterLimit: sh.MiterLimit,
				dashOffset: sh.DashOffset * s * r.scale,
			}
			for _, d := range sh.Dash {
				st.dash = append(st.dash, d*s*r.scale)
			}
			strokeC = stroke(polys, st)
		}
		fillC := closedContours(polys)
		evenOdd := sh.FillRule == svgicon.EvenOdd
		op := sh.Opacity
		if hasFill && hasStroke && op < 1 {
			// Element opacity applies to fill and stroke together, so paint
			// both at full opacity on a layer and composite it once.
			dst := r.img
			box, ok := contourBounds(append(append([][]pt{}, fillC...), strokeC...))
			box = box.Intersect(clip.pixels()).Intersect(dst.Bounds())
			if !ok || box.Empty() || op <= 0 {
				continue
			}
			layer := image.NewRGBA(box)
			r.img = layer
			r.iconPaint(fillC, evenOdd, clip, sh.Fill, sh.FillOpacity, toPixels, current)
			r.iconPaint(strokeC, false, clip, sh.Stroke, sh.StrokeOpacity, toPixels, current)
			r.img = dst
			draw.DrawMask(dst, box, layer, box.Min, image.NewUniform(color.Alpha{uint8(op*255 + 0.5)}), image.Point{}, draw.Over)
			continue
		}
		if hasFill {
			r.iconPaint(fillC, evenOdd, clip, sh.Fill, sh.FillOpacity*op, toPixels, current)
		}
		if hasStroke {
			r.iconPaint(strokeC, false, clip, sh.Stroke, sh.StrokeOpacity*op, toPixels, current)
		}
	}
	return nil
}

func (r *renderer) iconPaint(contours [][]pt, evenOdd bool, clip *rect, p svgicon.Paint, op float64, toPixels svgicon.Matrix, current color.RGBA) {
	if op <= 0 || len(contours) == 0 {
		return
	}
	switch p.Kind {
	case svgicon.Solid:
		r.fillColor(contours, evenOdd, clip, premul(p.Color, op))
	case svgicon.CurrentColor:
		r.fillColor(contours, evenOdd, clip, premul(current, op))
	case svgicon.Linear, svgicon.Radial:
		if p.Gradient == nil {
			return
		}
		mask, box := r.ras.fill(contours, evenOdd, r.img.Bounds(), clip)
		if mask == nil {
			return
		}
		src, ok := shadeGradient(p.Gradient, p.Kind, toPixels, box, current, op)
		if !ok {
			return
		}
		paintImage(r.img, box, mask, src)
	}
}
