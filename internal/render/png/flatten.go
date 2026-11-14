package png

import (
	"math"

	"github.com/grindlemire/diagram/internal/scene"
	"github.com/grindlemire/diagram/internal/svgicon"
)

// tolerance is the maximum distance, in output pixels, between a curve and
// the polyline that replaces it.
const tolerance = 0.1

type pt struct{ x, y float64 }

func (a pt) add(b pt) pt         { return pt{a.x + b.x, a.y + b.y} }
func (a pt) sub(b pt) pt         { return pt{a.x - b.x, a.y - b.y} }
func (a pt) mul(k float64) pt    { return pt{a.x * k, a.y * k} }
func (a pt) dot(b pt) float64    { return a.x*b.x + a.y*b.y }
func (a pt) cross(b pt) float64  { return a.x*b.y - a.y*b.x }
func (a pt) len() float64        { return math.Hypot(a.x, a.y) }
func (a pt) left() pt            { return pt{a.y, -a.x} } // y points down, so this is to the left of travel
func lerp(a, b pt, t float64) pt { return pt{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t} }
func apply(m svgicon.Matrix, p scene.Point) pt {
	x, y := m.Apply(p.X, p.Y)
	return pt{x, y}
}

// poly is a flattened subpath in output pixels.
type poly struct {
	pts    []pt
	closed bool
}

// flatten transforms a path to output pixels and replaces its curves with
// line segments.
func flatten(p scene.Path, m svgicon.Matrix) []poly {
	var out []poly
	var cur poly
	var pen, start pt
	open := false
	begin := func(at pt) {
		if open && len(cur.pts) > 0 {
			out = append(out, cur)
		}
		cur = poly{pts: []pt{at}}
		start, pen, open = at, at, true
	}
	ensure := func() {
		if !open {
			begin(pen)
		}
	}
	for _, c := range p {
		switch c.Op {
		case scene.MoveTo:
			begin(apply(m, c.Pts[0]))
		case scene.LineTo:
			ensure()
			pen = apply(m, c.Pts[0])
			cur.pts = append(cur.pts, pen)
		case scene.QuadTo:
			ensure()
			p1, p2 := apply(m, c.Pts[0]), apply(m, c.Pts[1])
			cur.pts = flattenQuad(cur.pts, pen, p1, p2)
			pen = p2
		case scene.CubicTo:
			ensure()
			p1, p2, p3 := apply(m, c.Pts[0]), apply(m, c.Pts[1]), apply(m, c.Pts[2])
			cur.pts = flattenCubic(cur.pts, pen, p1, p2, p3)
			pen = p3
		case scene.Close:
			if open {
				cur.closed = true
				out = append(out, cur)
				open = false
				cur = poly{}
			}
			pen = start
		}
	}
	if open && len(cur.pts) > 0 {
		out = append(out, cur)
	}
	return out
}

// segments returns how many uniform steps keep a Bézier whose largest
// second difference is dd within tolerance (Wang's formula); k is
// n(n-1)/8 for degree n.
func segments(dd, k float64) int {
	n := math.Ceil(math.Sqrt(k * dd / tolerance))
	if n < 1 || math.IsNaN(n) {
		return 1
	}
	return int(min(n, 1000))
}

func flattenQuad(dst []pt, p0, p1, p2 pt) []pt {
	n := segments(p0.sub(p1.mul(2)).add(p2).len(), 0.25)
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		if i == n {
			dst = append(dst, p2)
			break
		}
		dst = append(dst, lerp(lerp(p0, p1, t), lerp(p1, p2, t), t))
	}
	return dst
}

func flattenCubic(dst []pt, p0, p1, p2, p3 pt) []pt {
	dd := max(p0.sub(p1.mul(2)).add(p2).len(), p1.sub(p2.mul(2)).add(p3).len())
	n := segments(dd, 0.75)
	for i := 1; i <= n; i++ {
		if i == n {
			dst = append(dst, p3)
			break
		}
		t := float64(i) / float64(n)
		a, b, c := lerp(p0, p1, t), lerp(p1, p2, t), lerp(p2, p3, t)
		dst = append(dst, lerp(lerp(a, b, t), lerp(b, c, t), t))
	}
	return dst
}

// arcSteps returns how many segments approximate an arc of the given
// radius and sweep within tolerance.
func arcSteps(radius, sweep float64) int {
	if radius <= tolerance {
		return max(1, int(math.Ceil(math.Abs(sweep)/(math.Pi/2))))
	}
	step := 2 * math.Acos(1-tolerance/radius)
	return max(1, int(math.Ceil(math.Abs(sweep)/step)))
}

// kappa places cubic control points for a quarter circle.
const kappa = 0.5522847498

// rectPath returns a rectangle with corners rounded as SVG's <rect> does,
// starting at the same point so dash patterns line up.
func rectPath(x, y, w, h, r float64) scene.Path {
	var p scene.Path
	rx, ry := min(r, w/2), min(r, h/2)
	if r <= 0 {
		rx, ry = 0, 0
	}
	if rx <= 0 || ry <= 0 {
		p.MoveTo(x, y)
		p.LineTo(x+w, y)
		p.LineTo(x+w, y+h)
		p.LineTo(x, y+h)
		p.Close()
		return p
	}
	kx, ky := rx*kappa, ry*kappa
	p.MoveTo(x+rx, y)
	p.LineTo(x+w-rx, y)
	p.CubicTo(x+w-rx+kx, y, x+w, y+ry-ky, x+w, y+ry)
	p.LineTo(x+w, y+h-ry)
	p.CubicTo(x+w, y+h-ry+ky, x+w-rx+kx, y+h, x+w-rx, y+h)
	p.LineTo(x+rx, y+h)
	p.CubicTo(x+rx-kx, y+h, x, y+h-ry+ky, x, y+h-ry)
	p.LineTo(x, y+ry)
	p.CubicTo(x, y+ry-ky, x+rx-kx, y, x+rx, y)
	p.Close()
	return p
}

// ellipsePath starts at the rightmost point and runs clockwise, like SVG.
func ellipsePath(cx, cy, rx, ry float64) scene.Path {
	var p scene.Path
	kx, ky := rx*kappa, ry*kappa
	p.MoveTo(cx+rx, cy)
	p.CubicTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry)
	p.CubicTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy)
	p.CubicTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry)
	p.CubicTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy)
	p.Close()
	return p
}

// iconPath converts an svgicon path, which has the same layout as a scene
// path.
func iconPath(p svgicon.Path) scene.Path {
	out := make(scene.Path, len(p))
	for i, c := range p {
		out[i].Op = scene.Op(c.Op)
		for k := range 3 {
			out[i].Pts[k] = scene.Point{X: c.Pts[k].X, Y: c.Pts[k].Y}
		}
	}
	return out
}
