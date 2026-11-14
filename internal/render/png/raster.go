package png

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// rect is a rectangle in output pixels, for clipping.
type rect struct{ x0, y0, x1, y1 float64 }

// rasterizer turns contours into coverage masks. Nonzero fills use
// x/image/vector; it has no even-odd mode, so even-odd fills use a small
// accumulation rasterizer of the same kind.
type rasterizer struct {
	z    vector.Rasterizer
	acc  []float64
	mask image.Alpha
}

// fill returns the coverage of contours within bounds (and clip, if set),
// and the pixel rectangle it covers. The mask is reused by the next call.
func (r *rasterizer) fill(contours [][]pt, evenOdd bool, bounds image.Rectangle, clip *rect) (*image.Alpha, image.Rectangle) {
	box, ok := contourBounds(contours)
	if !ok {
		return nil, image.Rectangle{}
	}
	if clip != nil {
		box = box.Intersect(clip.pixels())
	}
	box = box.Intersect(bounds)
	if box.Empty() {
		return nil, image.Rectangle{}
	}
	w, h := box.Dx(), box.Dy()
	ox, oy := float64(box.Min.X), float64(box.Min.Y)
	if cap(r.mask.Pix) < w*h {
		r.mask.Pix = make([]uint8, w*h)
	}
	r.mask.Pix = r.mask.Pix[:w*h]
	r.mask.Stride = w
	r.mask.Rect = image.Rect(0, 0, w, h)
	if evenOdd {
		r.evenOdd(contours, w, h, ox, oy)
	} else {
		r.z.Reset(w, h)
		r.z.DrawOp = draw.Src
		for _, c := range contours {
			r.z.MoveTo(float32(c[0].x-ox), float32(c[0].y-oy))
			for _, p := range c[1:] {
				r.z.LineTo(float32(p.x-ox), float32(p.y-oy))
			}
			r.z.ClosePath()
		}
		r.z.Draw(&r.mask, r.mask.Rect, image.Opaque, image.Point{})
	}
	if clip != nil {
		r.clipEdges(*clip, box)
	}
	return &r.mask, box
}

func (c rect) pixels() image.Rectangle {
	return image.Rect(int(math.Floor(c.x0)), int(math.Floor(c.y0)), int(math.Ceil(c.x1)), int(math.Ceil(c.y1)))
}

// clipEdges scales the mask's edge rows and columns by how much of each
// pixel lies inside the clip rectangle.
func (r *rasterizer) clipEdges(c rect, box image.Rectangle) {
	frac := func(lo, hi float64, i int) float64 {
		a, b := max(lo, float64(i)), min(hi, float64(i+1))
		return max(0, min(1, b-a))
	}
	w, h := box.Dx(), box.Dy()
	for y := range h {
		fy := frac(c.y0, c.y1, box.Min.Y+y)
		for x := range w {
			f := fy * frac(c.x0, c.x1, box.Min.X+x)
			if f < 1 {
				i := y*w + x
				r.mask.Pix[i] = uint8(float64(r.mask.Pix[i])*f + 0.5)
			}
		}
	}
}

func contourBounds(contours [][]pt) (image.Rectangle, bool) {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, c := range contours {
		for _, p := range c {
			x0, y0 = min(x0, p.x), min(y0, p.y)
			x1, y1 = max(x1, p.x), max(y1, p.y)
		}
	}
	if x0 > x1 || math.IsNaN(x0+y0+x1+y1) || math.IsInf(x0+y0+x1+y1, 0) {
		return image.Rectangle{}, false
	}
	// Keep integer conversion sane for absurd coordinates.
	const lim = 1 << 24
	x0, y0, x1, y1 = max(x0, -lim), max(y0, -lim), min(x1, lim), min(y1, lim)
	return image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1))), true
}

// evenOdd rasterizes with signed-area accumulation, like x/image/vector,
// and folds the accumulated winding with the even-odd rule.
func (r *rasterizer) evenOdd(contours [][]pt, w, h int, ox, oy float64) {
	n := w * h
	if cap(r.acc) < n+1 {
		r.acc = make([]float64, n+1)
	}
	r.acc = r.acc[:n+1]
	clear(r.acc)
	for _, c := range contours {
		for i := range c {
			a, b := c[i], c[(i+1)%len(c)]
			accLine(r.acc, w, h, a.x-ox, a.y-oy, b.x-ox, b.y-oy)
		}
	}
	sum := 0.0
	for i := range n {
		sum += r.acc[i]
		v := math.Mod(math.Abs(sum), 2)
		if v > 1 {
			v = 2 - v
		}
		r.mask.Pix[i] = uint8(v*255 + 0.5)
	}
}

// accLine adds a line's signed area contributions to buf, a w×h grid read
// as one running sum. Contributions left of the grid land in column 0 and
// those right of it at the start of the next row, which the running sum
// makes equivalent.
func accLine(buf []float64, w, h int, ax, ay, bx, by float64) {
	dir := 1.0
	if ay > by {
		dir, ax, ay, bx, by = -1, bx, by, ax, ay
	}
	if by-ay <= 1e-12 {
		return
	}
	dxdy := (bx - ax) / (by - ay)
	idx := func(row, x int) int {
		x = max(0, min(w, x))
		return row*w + x
	}
	x := ax
	yMax := min(int(math.Ceil(by)), h)
	for y := int(math.Floor(ay)); y < yMax; y++ {
		dy := min(float64(y+1), by) - max(float64(y), ay)
		xNext := x + dy*dxdy
		if y < 0 {
			x = xNext
			continue
		}
		d := dy * dir
		x0, x1 := min(x, xNext), max(x, xNext)
		x0i := int(math.Floor(x0))
		x0f := float64(x0i)
		x1i := int(math.Ceil(x1))
		if x1i <= x0i+1 {
			// Within one pixel column: split by the mean x.
			xm := 0.5*(x+xNext) - x0f
			buf[idx(y, x0i)] += d - d*xm
			buf[idx(y, x0i+1)] += d * xm
		} else {
			s := 1 / (x1 - x0)
			fx0 := x0 - x0f
			a0 := 0.5 * s * (1 - fx0) * (1 - fx0)
			fx1 := x1 - float64(x1i) + 1
			am := 0.5 * s * fx1 * fx1
			buf[idx(y, x0i)] += d * a0
			if x1i == x0i+2 {
				buf[idx(y, x0i+1)] += d * (1 - a0 - am)
			} else {
				a1 := s * (1.5 - fx0)
				buf[idx(y, x0i+1)] += d * (a1 - a0)
				for xi := x0i + 2; xi < x1i-1; xi++ {
					buf[idx(y, xi)] += d * s
				}
				a2 := a1 + s*float64(x1i-x0i-3)
				buf[idx(y, x1i-1)] += d * (1 - a2 - am)
			}
			buf[idx(y, x1i)] += d * am
		}
		x = xNext
	}
}

// premul converts a straight-alpha colour with an extra opacity factor to
// premultiplied form.
func premul(c color.RGBA, opacity float64) color.RGBA {
	a := float64(c.A) / 255 * opacity
	if a <= 0 {
		return color.RGBA{}
	}
	a = min(a, 1)
	return color.RGBA{
		R: uint8(float64(c.R)*a + 0.5),
		G: uint8(float64(c.G)*a + 0.5),
		B: uint8(float64(c.B)*a + 0.5),
		A: uint8(255*a + 0.5),
	}
}

// paintMask composites a uniform premultiplied colour through mask onto
// dst at box, source over.
func paintMask(dst *image.RGBA, box image.Rectangle, mask *image.Alpha, c color.RGBA) {
	if c.A == 0 || mask == nil {
		return
	}
	draw.DrawMask(dst, box, image.NewUniform(c), image.Point{}, mask, image.Point{}, draw.Over)
}

// paintImage composites a premultiplied source image, whose bounds equal
// box, through mask onto dst, source over.
func paintImage(dst *image.RGBA, box image.Rectangle, mask *image.Alpha, src *image.RGBA) {
	w, h := box.Dx(), box.Dy()
	for y := range h {
		di := dst.PixOffset(box.Min.X, box.Min.Y+y)
		si := src.PixOffset(box.Min.X, box.Min.Y+y)
		mi := y * mask.Stride
		for x := range w {
			m := uint32(mask.Pix[mi+x])
			if m == 0 {
				di += 4
				si += 4
				continue
			}
			sr := uint32(src.Pix[si]) * m
			sg := uint32(src.Pix[si+1]) * m
			sb := uint32(src.Pix[si+2]) * m
			sa := uint32(src.Pix[si+3]) * m
			inv := 255*255 - sa
			dst.Pix[di] = uint8((sr + uint32(dst.Pix[di])*inv/255 + 127) / 255)
			dst.Pix[di+1] = uint8((sg + uint32(dst.Pix[di+1])*inv/255 + 127) / 255)
			dst.Pix[di+2] = uint8((sb + uint32(dst.Pix[di+2])*inv/255 + 127) / 255)
			dst.Pix[di+3] = uint8((sa + uint32(dst.Pix[di+3])*inv/255 + 127) / 255)
			di += 4
			si += 4
		}
	}
}
