package svgicon

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isWsp(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

// scanNumber reads an SVG number at s[i:], returning its value and the index
// after it. It stops at the first character that can't continue the number,
// so "1.5.5" reads as 1.5 then .5 and "1-2" as 1 then -2.
func scanNumber(s string, i int) (float64, int, bool) {
	start := i
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s) && isDigit(s[i]) {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
			digits++
		}
	}
	if digits == 0 {
		return 0, start, false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && isDigit(s[j]) {
			for j < len(s) && isDigit(s[j]) {
				j++
			}
			i = j
		}
	}
	v, err := strconv.ParseFloat(s[start:i], 64)
	if err != nil {
		return 0, start, false
	}
	return v, i, true
}

// scanner walks a list of numbers separated by whitespace and/or one comma.
type scanner struct {
	s string
	i int
}

func (sc *scanner) skipWsp() {
	for sc.i < len(sc.s) && isWsp(sc.s[sc.i]) {
		sc.i++
	}
}

// skipCommaWsp skips whitespace, at most one comma, and whitespace, and
// reports whether it saw a comma.
func (sc *scanner) skipCommaWsp() bool {
	sc.skipWsp()
	comma := false
	if sc.i < len(sc.s) && sc.s[sc.i] == ',' {
		sc.i++
		comma = true
		sc.skipWsp()
	}
	return comma
}

func (sc *scanner) done() bool { return sc.i >= len(sc.s) }

func (sc *scanner) atNumber() bool {
	if sc.done() {
		return false
	}
	c := sc.s[sc.i]
	return isDigit(c) || c == '.' || c == '+' || c == '-'
}

func (sc *scanner) number() (float64, error) {
	v, j, ok := scanNumber(sc.s, sc.i)
	if !ok {
		return 0, fmt.Errorf("expected number at offset %d in %q", sc.i, sc.s)
	}
	sc.i = j
	return v, nil
}

// flag reads an arc flag, which is a single 0 or 1 and needs no separator.
func (sc *scanner) flag() (bool, error) {
	if !sc.done() {
		switch sc.s[sc.i] {
		case '0':
			sc.i++
			return false, nil
		case '1':
			sc.i++
			return true, nil
		}
	}
	return false, fmt.Errorf("expected arc flag at offset %d in %q", sc.i, sc.s)
}

// numberList parses numbers separated by whitespace and/or commas.
func numberList(s string) ([]float64, error) {
	sc := scanner{s: s}
	sc.skipWsp()
	var out []float64
	for !sc.done() {
		if len(out) > 0 {
			if sc.skipCommaWsp() && sc.done() {
				return nil, fmt.Errorf("trailing comma in %q", s)
			}
		}
		v, err := sc.number()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		sc.skipWsp()
	}
	return out, nil
}

// parseNumber parses a whole string as one number.
func parseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	v, j, ok := scanNumber(s, 0)
	if !ok || j != len(s) {
		return 0, fmt.Errorf("bad number %q", s)
	}
	return v, nil
}

type axis uint8

const (
	axisX axis = iota
	axisY
	axisDiag // percentages of sqrt((w²+h²)/2), per SVG
)

// viewport is the size percentages resolve against: the nearest <svg>'s
// viewBox, or its width and height when it has none.
type viewport struct{ w, h float64 }

func (v viewport) of(a axis) float64 {
	switch a {
	case axisX:
		return v.w
	case axisY:
		return v.h
	}
	return math.Sqrt((v.w*v.w + v.h*v.h) / 2)
}

// length parses an SVG length. Absolute units convert at 96 dpi;
// percentages resolve against the viewport; font- and viewport-relative
// units are unsupported.
func length(s string, a axis, vp viewport, n *node) (float64, error) {
	s = strings.TrimSpace(s)
	v, j, ok := scanNumber(s, 0)
	if !ok {
		return 0, fmt.Errorf("svgicon: <%s>: bad length %q", n.name, s)
	}
	switch unit := strings.ToLower(s[j:]); unit {
	case "", "px":
		return v, nil
	case "%":
		return v / 100 * vp.of(a), nil
	case "pt":
		return v * 96 / 72, nil
	case "pc":
		return v * 16, nil
	case "in":
		return v * 96, nil
	case "cm":
		return v * 96 / 2.54, nil
	case "mm":
		return v * 96 / 25.4, nil
	case "q":
		return v * 96 / 101.6, nil
	case "em", "ex", "ch", "rem", "vw", "vh", "vmin", "vmax", "cap", "ic", "lh", "rlh":
		return 0, unsupported("length unit "+unit, n)
	default:
		return 0, fmt.Errorf("svgicon: <%s>: bad length %q", n.name, s)
	}
}

// fraction parses a number or percentage as a fraction, as opacities and
// stop offsets are.
func fraction(s string) (float64, error) {
	s = strings.TrimSpace(s)
	pct := strings.HasSuffix(s, "%")
	v, err := parseNumber(strings.TrimSuffix(s, "%"))
	if err != nil {
		return 0, err
	}
	if pct {
		v /= 100
	}
	return clamp01(v), nil
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// parseTransform parses an SVG transform list.
func parseTransform(s string, n *node) (Matrix, error) {
	m := Identity
	sc := scanner{s: s}
	sc.skipWsp()
	for !sc.done() {
		start := sc.i
		for sc.i < len(s) && (s[sc.i] >= 'a' && s[sc.i] <= 'z' || s[sc.i] >= 'A' && s[sc.i] <= 'Z') {
			sc.i++
		}
		name := s[start:sc.i]
		sc.skipWsp()
		if sc.done() || s[sc.i] != '(' {
			return m, fmt.Errorf("svgicon: <%s>: bad transform %q", n.name, s)
		}
		sc.i++
		sc.skipWsp()
		var args []float64
		for !sc.done() && s[sc.i] != ')' {
			if len(args) > 0 {
				sc.skipCommaWsp()
			}
			v, err := sc.number()
			if err != nil {
				return m, fmt.Errorf("svgicon: <%s>: bad transform %q", n.name, s)
			}
			args = append(args, v)
			sc.skipWsp()
		}
		if sc.done() {
			return m, fmt.Errorf("svgicon: <%s>: bad transform %q", n.name, s)
		}
		sc.i++ // ')'
		t, err := transformFunc(name, args, n)
		if err != nil {
			return m, err
		}
		m = m.Mul(t)
		sc.skipCommaWsp()
	}
	return m, nil
}

func transformFunc(name string, a []float64, n *node) (Matrix, error) {
	bad := func() (Matrix, error) {
		return Matrix{}, fmt.Errorf("svgicon: <%s>: %s() takes %s, got %d arguments", n.name, name, argCounts[name], len(a))
	}
	switch name {
	case "matrix":
		if len(a) != 6 {
			return bad()
		}
		return Matrix{a[0], a[1], a[2], a[3], a[4], a[5]}, nil
	case "translate":
		switch len(a) {
		case 1:
			return translate(a[0], 0), nil
		case 2:
			return translate(a[0], a[1]), nil
		}
		return bad()
	case "scale":
		switch len(a) {
		case 1:
			return Matrix{a[0], 0, 0, a[0], 0, 0}, nil
		case 2:
			return Matrix{a[0], 0, 0, a[1], 0, 0}, nil
		}
		return bad()
	case "rotate":
		if len(a) != 1 && len(a) != 3 {
			return bad()
		}
		sin, cos := math.Sincos(a[0] * math.Pi / 180)
		r := Matrix{cos, sin, -sin, cos, 0, 0}
		if len(a) == 3 {
			r = translate(a[1], a[2]).Mul(r).Mul(translate(-a[1], -a[2]))
		}
		return r, nil
	case "skewX":
		if len(a) != 1 {
			return bad()
		}
		return Matrix{1, 0, math.Tan(a[0] * math.Pi / 180), 1, 0, 0}, nil
	case "skewY":
		if len(a) != 1 {
			return bad()
		}
		return Matrix{1, math.Tan(a[0] * math.Pi / 180), 0, 1, 0, 0}, nil
	}
	return Matrix{}, unsupported("transform function "+name, n)
}

var argCounts = map[string]string{
	"matrix": "6", "translate": "1 or 2", "scale": "1 or 2",
	"rotate": "1 or 3", "skewX": "1", "skewY": "1",
}
