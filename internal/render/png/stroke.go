package png

import (
	"math"

	"github.com/grindlemire/diagram/internal/scene"
)

// strokeStyle describes a stroke in output pixels.
type strokeStyle struct {
	width      float64
	cap        scene.LineCap
	join       scene.LineJoin
	miterLimit float64   // ≥ 1; SVG's default is 4
	dash       []float64 // on/off lengths in output pixels; nil for solid
	dashOffset float64
}

// stroke turns flattened subpaths into closed contours whose nonzero fill
// is the stroke.
//
// Each subpath becomes one outline: the left offset forwards, the end cap,
// the right offset backwards and the start cap. Joins are added on the
// outer side of each turn; on the inner side the outline goes through the
// vertex itself. As chains, that outline is exactly the sum of the
// boundaries of each segment's rectangle, each join wedge and each cap, all
// with the same orientation, so its winding number is positive inside the
// stroke and zero outside, and the pieces' shared edges cancel instead of
// leaving seams.
func stroke(polys []poly, st strokeStyle) [][]pt {
	if st.width <= 0 {
		return nil
	}
	s := stroker{hw: st.width / 2, st: st}
	if st.miterLimit < 1 {
		s.st.miterLimit = 4
	}
	for _, p := range polys {
		pts := dedupe(p.pts, p.closed)
		if p.closed && len(pts) < 3 {
			// A closed path that collapses to a line or a point strokes like
			// an open one that goes there and back.
			if len(pts) == 2 {
				pts = append(pts, pts[0])
			}
			p.closed = false
		}
		if dash := dashPattern(st.dash); dash != nil {
			for _, d := range dashes(pts, p.closed, dash, st.dashOffset) {
				s.open(d.pts, d.dir)
			}
			continue
		}
		if p.closed {
			s.closed(pts)
		} else {
			s.open(pts, pt{1, 0})
		}
	}
	return s.out
}

type stroker struct {
	hw  float64
	st  strokeStyle
	out [][]pt
	cur []pt
}

// dedupe drops repeated points, and a closed path's final point when it
// repeats the first.
func dedupe(in []pt, closed bool) []pt {
	const eps = 1e-6
	out := make([]pt, 0, len(in))
	for _, p := range in {
		if len(out) > 0 && p.sub(out[len(out)-1]).len() < eps {
			continue
		}
		out = append(out, p)
	}
	if closed && len(out) > 1 && out[0].sub(out[len(out)-1]).len() < eps {
		out = out[:len(out)-1]
	}
	return out
}

func unit(v pt) pt { return v.mul(1 / v.len()) }

// open strokes an open polyline. A single point draws only its caps, facing
// dir.
func (s *stroker) open(pts []pt, dir pt) {
	if len(pts) == 0 {
		return
	}
	if len(pts) == 1 {
		s.dot(pts[0], dir)
		return
	}
	hw := s.hw
	n := len(pts) - 1
	norm := make([]pt, n)
	for i := range n {
		norm[i] = unit(pts[i+1].sub(pts[i])).left()
	}
	s.cur = []pt{pts[0].add(norm[0].mul(hw))}
	for i := range n {
		s.lineTo(pts[i+1].add(norm[i].mul(hw)))
		if i+1 < n {
			s.join(pts[i+1], norm[i], norm[i+1])
		}
	}
	s.cap(pts[n], norm[n-1])
	for i := n - 1; i >= 0; i-- {
		s.lineTo(pts[i].sub(norm[i].mul(hw)))
		if i > 0 {
			s.join(pts[i], norm[i].mul(-1), norm[i-1].mul(-1))
		}
	}
	s.cap(pts[0], norm[0].mul(-1))
	s.finish()
}

// closed strokes a closed polygon as two loops: the left offset forwards
// and the right offset backwards.
func (s *stroker) closed(pts []pt) {
	s.loop(pts)
	rev := make([]pt, len(pts))
	for i, p := range pts {
		rev[len(pts)-1-i] = p
	}
	s.loop(rev)
}

func (s *stroker) loop(pts []pt) {
	n := len(pts)
	norm := make([]pt, n)
	for i := range n {
		norm[i] = unit(pts[(i+1)%n].sub(pts[i])).left()
	}
	s.cur = []pt{pts[0].add(norm[0].mul(s.hw))}
	for i := range n {
		j := (i + 1) % n
		s.lineTo(pts[j].add(norm[i].mul(s.hw)))
		s.join(pts[j], norm[i], norm[j])
	}
	s.finish()
}

func (s *stroker) lineTo(p pt) { s.cur = append(s.cur, p) }

func (s *stroker) finish() {
	if len(s.cur) > 2 {
		s.out = append(s.out, s.cur)
	}
	s.cur = nil
}

// join connects the offset at p for normal na to the one for nb. The
// offset is on the left of travel, so it is the outside of the turn when
// the path turns right (clockwise on screen).
func (s *stroker) join(p, na, nb pt) {
	hw := s.hw
	cr, dt := na.cross(nb), na.dot(nb)
	end := p.add(nb.mul(hw))
	const eps = 1e-9
	switch {
	case cr < -eps:
		// Inside of the turn: pass through the vertex.
		s.lineTo(p)
		s.lineTo(end)
		return
	case cr <= eps && dt > 0:
		s.lineTo(end) // straight on
		return
	}
	theta := math.Atan2(cr, dt) // turn angle, in (0, π]
	if cr <= eps {
		theta = math.Pi // reversal
	}
	switch s.st.join {
	case scene.JoinRound:
		s.arc(p, na, theta)
	case scene.JoinMiter:
		// The miter ratio is 1/sin(φ/2) for the angle φ between the segments,
		// which is 1/cos(θ/2) for the turn θ.
		if c := math.Cos(theta / 2); c > 0 && 1/c <= s.st.miterLimit {
			bis := unit(na.add(nb))
			s.lineTo(p.add(bis.mul(hw / c)))
		}
	}
	s.lineTo(end)
}

// arc adds points on the circle of radius hw around c, from direction from
// turning clockwise (on screen) by sweep, excluding the start point.
func (s *stroker) arc(c, from pt, sweep float64) {
	n := arcSteps(s.hw, sweep)
	a0 := math.Atan2(from.y, from.x)
	for i := 1; i <= n; i++ {
		a := a0 + sweep*float64(i)/float64(n)
		s.lineTo(pt{c.x + s.hw*math.Cos(a), c.y + s.hw*math.Sin(a)})
	}
}

// cap goes from p+n·hw to p−n·hw around the end of the line, where n is
// the left normal of travel.
func (s *stroker) cap(p, n pt) {
	hw := s.hw
	switch s.st.cap {
	case scene.CapRound:
		s.arc(p, n, math.Pi)
	case scene.CapSquare:
		d := pt{-n.y, n.x} // direction of travel
		s.lineTo(p.add(n.mul(hw)).add(d.mul(hw)))
		s.lineTo(p.sub(n.mul(hw)).add(d.mul(hw)))
	}
	s.lineTo(p.sub(n.mul(hw)))
}

// dot draws the caps of a zero-length subpath: a circle for round caps, a
// square facing dir for square caps and nothing for butt caps.
func (s *stroker) dot(p, dir pt) {
	hw := s.hw
	switch s.st.cap {
	case scene.CapRound:
		s.cur = []pt{{p.x + hw, p.y}}
		s.arc(p, pt{1, 0}, 2*math.Pi)
		s.finish()
	case scene.CapSquare:
		if dir.len() == 0 {
			dir = pt{1, 0}
		}
		d := unit(dir).mul(hw)
		n := d.left()
		s.cur = []pt{p.add(d).add(n), p.add(d).sub(n), p.sub(d).sub(n), p.sub(d).add(n)}
		s.finish()
	}
}

// dashPattern normalizes a dash array as SVG does: odd lengths repeat, and
// an array with a negative entry or no length draws a solid line.
func dashPattern(d []float64) []float64 {
	if len(d) == 0 {
		return nil
	}
	sum := 0.0
	for _, v := range d {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
		sum += v
	}
	if sum <= 0 {
		return nil
	}
	if len(d)%2 == 1 {
		d = append(append([]float64(nil), d...), d...)
	}
	return d
}

// dashSeg is one "on" piece of a dashed polyline. dir is the direction of
// travel, for pieces that have no length.
type dashSeg struct {
	pts []pt
	dir pt
}

// dashes cuts a polyline into its "on" pieces. The pattern restarts at the
// start of each subpath.
func dashes(pts []pt, closed bool, dash []float64, offset float64) []dashSeg {
	if len(pts) == 0 {
		return nil
	}
	if len(pts) == 1 {
		return nil // a lone point has no length to dash along
	}
	if closed {
		pts = append(append([]pt(nil), pts...), pts[0])
	}
	total := 0.0
	for _, v := range dash {
		total += v
	}
	// Find where in the pattern the path starts.
	phase := math.Mod(offset, total)
	if phase < 0 {
		phase += total
	}
	idx := 0
	for phase >= dash[idx] && !(phase == 0 && dash[idx] == 0) {
		phase -= dash[idx]
		idx = (idx + 1) % len(dash)
		if phase == 0 {
			break
		}
	}
	left := dash[idx] - phase // length left in the current dash element
	on := idx%2 == 0

	var out []dashSeg
	var cur []pt
	if on {
		cur = []pt{pts[0]}
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		seg := b.sub(a)
		L := seg.len()
		dir := seg.mul(1 / L)
		pos := 0.0
		for L-pos > left {
			pos += left
			q := a.add(dir.mul(pos))
			if on {
				cur = append(cur, q)
				out = append(out, dashSeg{pts: dedupe(cur, false), dir: dir})
				cur = nil
			} else {
				cur = []pt{q}
			}
			on = !on
			idx = (idx + 1) % len(dash)
			left = dash[idx]
		}
		left -= L - pos
		if on {
			cur = append(cur, b)
		}
	}
	if on && len(cur) > 0 {
		last := pts[len(pts)-1].sub(pts[len(pts)-2])
		out = append(out, dashSeg{pts: dedupe(cur, false), dir: unit(last)})
	}
	return out
}
