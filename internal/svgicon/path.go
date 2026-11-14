package svgicon

import (
	"fmt"
	"math"
)

// kappa places cubic control points for a quarter ellipse.
const kappa = 0.5522847498307936 // 4/3 · (√2 − 1)

// parsePathData parses the d attribute into absolute commands. Errors stop
// parsing; browsers would draw up to the error, but a broken icon should be
// loud.
func parsePathData(d string) (Path, error) {
	sc := scanner{s: d}
	var p Path
	var cx, cy, sx, sy float64 // current point, subpath start
	var ctrlX, ctrlY float64   // last control point, for S and T
	var prev byte              // previous command, upper case
	sc.skipWsp()
	for !sc.done() {
		c := d[sc.i]
		cmd := c &^ 0x20 // upper case
		if !isCommand(cmd) {
			return nil, fmt.Errorf("bad path command %q at offset %d", c, sc.i)
		}
		if prev == 0 && cmd != 'M' {
			return nil, fmt.Errorf("path must start with M, got %q", c)
		}
		sc.i++
		rel := c >= 'a'
		if cmd == 'Z' {
			p.close()
			cx, cy = sx, sy
			prev = 'Z'
			sc.skipWsp()
			continue
		}
		for set := 0; ; set++ {
			if set == 0 {
				sc.skipWsp()
			} else {
				save := sc.i
				comma := sc.skipCommaWsp()
				if !sc.atNumber() {
					if comma {
						return nil, fmt.Errorf("stray comma at offset %d", save)
					}
					sc.i = save
					sc.skipWsp()
					break
				}
			}
			var args [7]float64
			n := argCount[cmd]
			for k := range n {
				if k > 0 {
					sc.skipCommaWsp()
				}
				var err error
				if cmd == 'A' && (k == 3 || k == 4) {
					var f bool
					f, err = sc.flag()
					if f {
						args[k] = 1
					}
				} else {
					args[k], err = sc.number()
				}
				if err != nil {
					return nil, err
				}
			}
			// A command after Z starts a new subpath at the old start.
			if prev == 'Z' && cmd != 'M' {
				p.moveTo(cx, cy)
			}
			ox, oy := 0.0, 0.0
			if rel {
				ox, oy = cx, cy
			}
			switch cmd {
			case 'M':
				cx, cy = args[0]+ox, args[1]+oy
				sx, sy = cx, cy
				p.moveTo(cx, cy)
				// Further pairs are implicit line-tos.
				cmd = 'L'
				prev = 'M'
				continue
			case 'L':
				cx, cy = args[0]+ox, args[1]+oy
				p.lineTo(cx, cy)
			case 'H':
				cx = args[0] + ox
				p.lineTo(cx, cy)
			case 'V':
				cy = args[0] + oy
				p.lineTo(cx, cy)
			case 'C':
				x1, y1 := args[0]+ox, args[1]+oy
				ctrlX, ctrlY = args[2]+ox, args[3]+oy
				cx, cy = args[4]+ox, args[5]+oy
				p.cubicTo(x1, y1, ctrlX, ctrlY, cx, cy)
			case 'S':
				x1, y1 := cx, cy
				if prev == 'C' || prev == 'S' {
					x1, y1 = 2*cx-ctrlX, 2*cy-ctrlY
				}
				ctrlX, ctrlY = args[0]+ox, args[1]+oy
				cx, cy = args[2]+ox, args[3]+oy
				p.cubicTo(x1, y1, ctrlX, ctrlY, cx, cy)
			case 'Q':
				ctrlX, ctrlY = args[0]+ox, args[1]+oy
				cx, cy = args[2]+ox, args[3]+oy
				p.quadTo(ctrlX, ctrlY, cx, cy)
			case 'T':
				if prev == 'Q' || prev == 'T' {
					ctrlX, ctrlY = 2*cx-ctrlX, 2*cy-ctrlY
				} else {
					ctrlX, ctrlY = cx, cy
				}
				cx, cy = args[0]+ox, args[1]+oy
				p.quadTo(ctrlX, ctrlY, cx, cy)
			case 'A':
				x, y := args[5]+ox, args[6]+oy
				arcTo(&p, cx, cy, args[0], args[1], args[2], args[3] != 0, args[4] != 0, x, y)
				cx, cy = x, y
			}
			prev = cmd
		}
	}
	return p, nil
}

func isCommand(c byte) bool {
	_, ok := argCount[c]
	return ok
}

var argCount = map[byte]int{
	'M': 2, 'L': 2, 'H': 1, 'V': 1, 'C': 6, 'S': 4, 'Q': 4, 'T': 2, 'A': 7, 'Z': 0,
}

// arcTo appends an elliptical arc from (x1, y1) to (x2, y2) as cubics, using
// the endpoint-to-centre conversion of SVG's implementation notes (F.6).
func arcTo(p *Path, x1, y1, rx, ry, phiDeg float64, large, sweep bool, x2, y2 float64) {
	if x1 == x2 && y1 == y2 {
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 {
		p.lineTo(x2, y2)
		return
	}
	sin, cos := math.Sincos(phiDeg * math.Pi / 180)
	dx2, dy2 := (x1-x2)/2, (y1-y2)/2
	x1p := cos*dx2 + sin*dy2
	y1p := -sin*dx2 + cos*dy2
	if l := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry); l > 1 {
		s := math.Sqrt(l)
		rx, ry = rx*s, ry*s
	}
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	coef := 0.0
	if num > 0 && den > 0 {
		coef = math.Sqrt(num / den)
	}
	if large == sweep {
		coef = -coef
	}
	cxp := coef * rx * y1p / ry
	cyp := -coef * ry * x1p / rx
	cx := cos*cxp - sin*cyp + (x1+x2)/2
	cy := sin*cxp + cos*cyp + (y1+y2)/2

	ux, uy := (x1p-cxp)/rx, (y1p-cyp)/ry
	vx, vy := (-x1p-cxp)/rx, (-y1p-cyp)/ry
	theta := math.Atan2(uy, ux)
	dtheta := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	if !sweep && dtheta > 0 {
		dtheta -= 2 * math.Pi
	} else if sweep && dtheta < 0 {
		dtheta += 2 * math.Pi
	}

	n := max(1, int(math.Ceil(math.Abs(dtheta)/(math.Pi/2)-1e-9)))
	delta := dtheta / float64(n)
	t := 4.0 / 3 * math.Tan(delta/4)
	pt := func(ex, ey float64) (float64, float64) {
		return cx + rx*ex*cos - ry*ey*sin, cy + rx*ex*sin + ry*ey*cos
	}
	for i := range n {
		a1 := theta + float64(i)*delta
		a2 := a1 + delta
		s1, c1 := math.Sincos(a1)
		s2, c2 := math.Sincos(a2)
		c1x, c1y := pt(c1-t*s1, s1+t*c1)
		c2x, c2y := pt(c2+t*s2, s2-t*c2)
		ex, ey := pt(c2, s2)
		if i == n-1 {
			ex, ey = x2, y2
		}
		p.cubicTo(c1x, c1y, c2x, c2y, ex, ey)
	}
}

// ellipsePath starts at (cx+rx, cy) and runs clockwise on screen, as SVG 2
// specifies for circle and ellipse (it matters for dashes).
func ellipsePath(cx, cy, rx, ry float64) Path {
	var p Path
	kx, ky := kappa*rx, kappa*ry
	p.moveTo(cx+rx, cy)
	p.cubicTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry)
	p.cubicTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy)
	p.cubicTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry)
	p.cubicTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy)
	p.close()
	return p
}

// rectPath starts at (x+rx, y) and runs clockwise, as SVG 2 specifies.
// rx and ry are already clamped.
func rectPath(x, y, w, h, rx, ry float64) Path {
	var p Path
	if rx == 0 || ry == 0 {
		p.moveTo(x, y)
		p.lineTo(x+w, y)
		p.lineTo(x+w, y+h)
		p.lineTo(x, y+h)
		p.close()
		return p
	}
	kx, ky := kappa*rx, kappa*ry
	line := func(px, py float64) {
		last := p[len(p)-1]
		end := last.Pts[last.Op.npts()-1]
		if end.X != px || end.Y != py {
			p.lineTo(px, py)
		}
	}
	p.moveTo(x+rx, y)
	line(x+w-rx, y)
	p.cubicTo(x+w-rx+kx, y, x+w, y+ry-ky, x+w, y+ry)
	line(x+w, y+h-ry)
	p.cubicTo(x+w, y+h-ry+ky, x+w-rx+kx, y+h, x+w-rx, y+h)
	line(x+rx, y+h)
	p.cubicTo(x+rx-kx, y+h, x, y+h-ry+ky, x, y+h-ry)
	line(x, y+ry)
	p.cubicTo(x, y+ry-ky, x+rx-kx, y, x+rx, y)
	p.close()
	return p
}

// rectBox is an axis-aligned box.
type rectBox struct{ x0, y0, x1, y1 float64 }

// bounds returns the tight bounding box of the path's geometry, including
// curve extrema. ok is false for an empty path.
func (p Path) bounds() (b rectBox, ok bool) {
	b = rectBox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	add := func(x, y float64) {
		b.x0, b.y0 = math.Min(b.x0, x), math.Min(b.y0, y)
		b.x1, b.y1 = math.Max(b.x1, x), math.Max(b.y1, y)
		ok = true
	}
	var cur, start Point
	for _, c := range p {
		switch c.Op {
		case MoveTo:
			cur, start = c.Pts[0], c.Pts[0]
			add(cur.X, cur.Y)
		case LineTo:
			cur = c.Pts[0]
			add(cur.X, cur.Y)
		case QuadTo:
			p0, p1, p2 := cur, c.Pts[0], c.Pts[1]
			for _, t := range [2]float64{quadExtremum(p0.X, p1.X, p2.X), quadExtremum(p0.Y, p1.Y, p2.Y)} {
				if t > 0 && t < 1 {
					u := 1 - t
					add(u*u*p0.X+2*u*t*p1.X+t*t*p2.X, u*u*p0.Y+2*u*t*p1.Y+t*t*p2.Y)
				}
			}
			cur = p2
			add(cur.X, cur.Y)
		case CubicTo:
			p0, p1, p2, p3 := cur, c.Pts[0], c.Pts[1], c.Pts[2]
			var ts []float64
			ts = cubicExtrema(ts, p0.X, p1.X, p2.X, p3.X)
			ts = cubicExtrema(ts, p0.Y, p1.Y, p2.Y, p3.Y)
			for _, t := range ts {
				u := 1 - t
				a, bb, cc, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
				add(a*p0.X+bb*p1.X+cc*p2.X+d*p3.X, a*p0.Y+bb*p1.Y+cc*p2.Y+d*p3.Y)
			}
			cur = p3
			add(cur.X, cur.Y)
		case Close:
			cur = start
		}
	}
	return b, ok
}

func quadExtremum(a, b, c float64) float64 {
	den := a - 2*b + c
	if den == 0 {
		return -1
	}
	return (a - b) / den
}

// cubicExtrema appends the t in (0, 1) where the cubic's derivative is zero.
func cubicExtrema(ts []float64, p0, p1, p2, p3 float64) []float64 {
	a := -p0 + 3*p1 - 3*p2 + p3
	b := 2 * (p0 - 2*p1 + p2)
	c := p1 - p0
	keep := func(t float64) {
		if t > 0 && t < 1 {
			ts = append(ts, t)
		}
	}
	if math.Abs(a) < 1e-12 {
		if b != 0 {
			keep(-c / b)
		}
		return ts
	}
	disc := b*b - 4*a*c
	if disc < 0 {
		return ts
	}
	sq := math.Sqrt(disc)
	keep((-b + sq) / (2 * a))
	keep((-b - sq) / (2 * a))
	return ts
}
