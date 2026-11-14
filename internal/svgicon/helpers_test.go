package svgicon

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
)

const tol = 1e-6

// doc wraps body in a 100×100 root svg.
func doc(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 100 100">` + body + `</svg>`
}

func mustParse(t *testing.T, src string) *Icon {
	t.Helper()
	ic, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, src)
	}
	return ic
}

// one parses body and requires exactly one shape.
func one(t *testing.T, body string) Shape {
	t.Helper()
	ic := mustParse(t, doc(body))
	if len(ic.Shapes) != 1 {
		t.Fatalf("got %d shapes, want 1\n%s", len(ic.Shapes), body)
	}
	return ic.Shapes[0]
}

// P builds a path from "M x y L x y Q cx cy x y C c1x c1y c2x c2y x y Z".
func P(s string) Path {
	f := strings.Fields(s)
	var p Path
	num := func(i int) float64 {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			panic(err)
		}
		return v
	}
	for i := 0; i < len(f); {
		switch f[i] {
		case "M":
			p.moveTo(num(i+1), num(i+2))
			i += 3
		case "L":
			p.lineTo(num(i+1), num(i+2))
			i += 3
		case "Q":
			p.quadTo(num(i+1), num(i+2), num(i+3), num(i+4))
			i += 5
		case "C":
			p.cubicTo(num(i+1), num(i+2), num(i+3), num(i+4), num(i+5), num(i+6))
			i += 7
		case "Z":
			p.close()
			i++
		default:
			panic("bad path op " + f[i])
		}
	}
	return p
}

func (p Path) String() string {
	var b strings.Builder
	for _, c := range p {
		b.WriteString([...]string{"M", "L", "Q", "C", "Z"}[c.Op])
		for k := range c.Op.npts() {
			b.WriteString(" " + strconv.FormatFloat(c.Pts[k].X, 'g', -1, 64) + " " + strconv.FormatFloat(c.Pts[k].Y, 'g', -1, 64))
		}
		b.WriteString(" ")
	}
	return strings.TrimSpace(b.String())
}

func pathEqual(a, b Path) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Op != b[i].Op {
			return false
		}
		for k := range a[i].Op.npts() {
			if !near(a[i].Pts[k].X, b[i].Pts[k].X) || !near(a[i].Pts[k].Y, b[i].Pts[k].Y) {
				return false
			}
		}
	}
	return true
}

func assertPath(t *testing.T, got Path, want string) {
	t.Helper()
	if w := P(want); !pathEqual(got, w) {
		t.Errorf("path\n got %s\nwant %s", got, w)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) <= tol*math.Max(1, math.Abs(b)) }

func assertNear(t *testing.T, what string, got, want float64) {
	t.Helper()
	if !near(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func assertMatrix(t *testing.T, got, want Matrix) {
	t.Helper()
	for i := range got {
		if !near(got[i], want[i]) {
			t.Errorf("matrix = %v, want %v", got, want)
			return
		}
	}
}

// unsupportedFeature returns the Feature of an *UnsupportedError, or fails.
func unsupportedFeature(t *testing.T, err error) string {
	t.Helper()
	var ue *UnsupportedError
	if !errors.As(err, &ue) {
		t.Fatalf("error %v (%T) is not an *UnsupportedError", err, err)
	}
	return ue.Feature
}
