package svgicon

import (
	"math"
	"testing"
)

func TestPathData(t *testing.T) {
	semicircleUp := "M 0 0 C 0 -5.522847498 4.477152502 -10 10 -10 C 15.522847498 -10 20 -5.522847498 20 0"
	tests := []struct {
		name, d, want string
	}{
		{"empty", "", ""},
		{"move line", "M10 20L30 40", "M 10 20 L 30 40"},
		{"relative", "m10 20l5 5h10v-5z", "M 10 20 L 15 25 L 25 25 L 25 20 Z"},
		{"absolute H V", "M1 2H10V20H0", "M 1 2 L 10 2 L 10 20 L 0 20"},
		{"implicit lineto after M", "M0 0 10 10 20 0", "M 0 0 L 10 10 L 20 0"},
		{"implicit relative lineto after m", "m1 1 2 2 3 3", "M 1 1 L 3 3 L 6 6"},
		{"implicit repeat L", "M0 0L1 1 2 2", "M 0 0 L 1 1 L 2 2"},
		{"implicit repeat h", "M0 0h1 2 3", "M 0 0 L 1 0 L 3 0 L 6 0"},
		{"commas", "M 0,0 L 1,2, 3,4", "M 0 0 L 1 2 L 3 4"},
		{"whitespace kinds", "\tM0\n0\r\nL 1 1 ", "M 0 0 L 1 1"},
		{"compact decimals", "M1.5.5L-1-2", "M 1.5 0.5 L -1 -2"},
		{"leading dot and plus", "M.5+.5l+1-.25", "M 0.5 0.5 L 1.5 0.25"},
		{"exponents", "M1e1 2E-1L1e+1,0", "M 10 0.2 L 10 0"},
		{"exponent then command", "M1e1 1e1L0 0", "M 10 10 L 0 0"},
		{"cubic", "M0 0C1 2 3 4 5 6", "M 0 0 C 1 2 3 4 5 6"},
		{"relative cubic", "M10 10c1 2 3 4 5 6", "M 10 10 C 11 12 13 14 15 16"},
		{"cubic repeat", "M0 0C1 1 2 2 3 3 4 4 5 5 6 6", "M 0 0 C 1 1 2 2 3 3 C 4 4 5 5 6 6"},
		{"smooth reflects cubic", "M0 0C0 10 10 10 10 0S20 -10 20 0", "M 0 0 C 0 10 10 10 10 0 C 10 -10 20 -10 20 0"},
		{"smooth relative", "M0 0c0 10 10 10 10 0s10 -10 10 0", "M 0 0 C 0 10 10 10 10 0 C 10 -10 20 -10 20 0"},
		{"smooth without cubic", "M0 0L5 5S10 10 20 0", "M 0 0 L 5 5 C 5 5 10 10 20 0"},
		{"quad", "M0 0Q5 10 10 0", "M 0 0 Q 5 10 10 0"},
		{"relative quad", "M1 1q5 10 10 0", "M 1 1 Q 6 11 11 1"},
		{"smooth quad reflects", "M0 0Q5 10 10 0T20 0", "M 0 0 Q 5 10 10 0 Q 15 -10 20 0"},
		{"smooth quad chain", "M0 0Q5 10 10 0T20 0T30 0", "M 0 0 Q 5 10 10 0 Q 15 -10 20 0 Q 25 10 30 0"},
		{"smooth quad relative", "M0 0q5 10 10 0t10 0", "M 0 0 Q 5 10 10 0 Q 15 -10 20 0"},
		{"smooth quad without quad", "M0 0L10 0T20 0", "M 0 0 L 10 0 Q 10 0 20 0"},
		{"smooth quad after cubic", "M0 0C1 1 2 2 10 0T20 0", "M 0 0 C 1 1 2 2 10 0 Q 10 0 20 0"},
		{"close then draw starts new subpath", "M0 0L10 0L10 10ZL5 5", "M 0 0 L 10 0 L 10 10 Z M 0 0 L 5 5"},
		{"relative after close", "M10 10L20 10zm5 5l1 1", "M 10 10 L 20 10 Z M 15 15 L 16 16"},
		{"two subpaths", "M0 0L1 0ZM5 5L6 5Z", "M 0 0 L 1 0 Z M 5 5 L 6 5 Z"},
		{"arc semicircle", "M0 0A10 10 0 0 1 20 0", semicircleUp},
		{"arc compact flags", "M0 0a10 10 0 0120 0", semicircleUp},
		{"arc commas", "M0,0A10,10,0,0,1,20,0", semicircleUp},
		{"arc radius scaled up", "M0 0A1 1 0 0 1 20 0", semicircleUp},
		{"arc negative radii", "M0 0A-10 -10 0 0 1 20 0", semicircleUp},
		{"arc other sweep", "M0 0A10 10 0 0 0 20 0",
			"M 0 0 C 0 5.522847498 4.477152502 10 10 10 C 15.522847498 10 20 5.522847498 20 0"},
		{"arc quarter", "M10 0A10 10 0 0 1 0 10", "M 10 0 C 10 5.522847498 5.522847498 10 0 10"},
		{"arc large", "M10 0A10 10 0 1 0 0 10",
			"M 10 0 C 10 -5.522847498 5.522847498 -10 0 -10 C -5.522847498 -10 -10 -5.522847498 -10 0 C -10 5.522847498 -5.522847498 10 0 10"},
		{"arc zero radius is a line", "M0 0A0 5 0 0 1 10 10", "M 0 0 L 10 10"},
		{"arc to same point is dropped", "M5 5A10 10 0 0 1 5 5", "M 5 5"},
		{"arc rotation of a circle is a no-op", "M0 0A10 10 45 0 1 20 0", semicircleUp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parsePathData(tt.d)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if len(p) != 0 {
					t.Fatalf("got %s, want empty", p)
				}
				return
			}
			assertPath(t, p, tt.want)
		})
	}
}

func TestPathDataErrors(t *testing.T) {
	for _, d := range []string{
		"L0 0",               // must start with M
		"M0",                 // missing coordinate
		"M0 0L",              // command without arguments
		"M0 0X1 1",           // unknown command
		"M0,,0",              // double comma
		"M0 0L1 1,",          // trailing comma
		"M0 0A1 1 0 2 0 5 5", // bad flag
		"M0 0 #",
	} {
		if _, err := parsePathData(d); err == nil {
			t.Errorf("parsePathData(%q) succeeded, want error", d)
		}
	}
}

// Arc segments must lie on the ellipse for rotated, non-circular arcs.
func TestArcOnEllipse(t *testing.T) {
	var p Path
	p.moveTo(0, 0)
	cx, cy, rx, ry, phi := 30.0, 10.0, 40.0, 15.0, 30.0
	// Start and end on the ellipse at angles 200° and 20°.
	at := func(a float64) (float64, float64) {
		sa, ca := math.Sincos(a * math.Pi / 180)
		sp, cp := math.Sincos(phi * math.Pi / 180)
		return cx + rx*ca*cp - ry*sa*sp, cy + rx*ca*sp + ry*sa*cp
	}
	x1, y1 := at(200)
	x2, y2 := at(60)
	p = nil
	p.moveTo(x1, y1)
	arcTo(&p, x1, y1, rx, ry, phi, true, true, x2, y2) // 220° clockwise
	if len(p) != 4 {                                   // three segments of at most 90°
		t.Fatalf("got %d commands: %s", len(p), p)
	}
	sp, cp := math.Sincos(phi * math.Pi / 180)
	for _, c := range p[1:] {
		end := c.Pts[2]
		dx, dy := end.X-cx, end.Y-cy
		u := (dx*cp + dy*sp) / rx
		v := (-dx*sp + dy*cp) / ry
		if math.Abs(u*u+v*v-1) > 1e-9 {
			t.Errorf("segment end %v is off the ellipse (%v)", end, u*u+v*v)
		}
	}
	// The first segment ends a third of the way round.
	mx, my := at(200 + 220.0/3)
	if e := p[1].Pts[2]; !near(e.X, mx) || !near(e.Y, my) {
		t.Errorf("mid point = %v, want (%v, %v)", e, mx, my)
	}
}

func TestArcRotatedEllipse(t *testing.T) {
	// rx lies along the screen's y axis after rotate 90; the chord is that
	// diameter, and a clockwise sweep from the top bulges right by ry.
	p, err := parsePathData("M0 0A10 5 90 0 1 0 20")
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 3 {
		t.Fatalf("got %s", p)
	}
	if e := p[1].Pts[2]; !near(e.X, 5) || !near(e.Y, 10) {
		t.Errorf("mid point = %v, want (5, 10)", e)
	}
	if e := p[2].Pts[2]; e.X != 0 || e.Y != 20 {
		t.Errorf("end point = %v, want exactly (0, 20)", e)
	}
}

func TestBounds(t *testing.T) {
	tests := []struct {
		path string
		want rectBox
	}{
		{"M 0 0 L 10 5", rectBox{0, 0, 10, 5}},
		// A quarter circle's control hull is the full square; the curve is too.
		{"M 10 0 C 10 5.52 5.52 10 0 10", rectBox{0, 0, 10, 10}},
		// A bulging cubic: hull goes to y=10, the curve peaks at 7.5.
		{"M 0 0 C 0 10 10 10 10 0", rectBox{0, 0, 10, 7.5}},
		// Quadratic peak at half the control height.
		{"M 0 0 Q 5 10 10 0", rectBox{0, 0, 10, 5}},
	}
	for _, tt := range tests {
		b, ok := P(tt.path).bounds()
		if !ok || !near(b.x0, tt.want.x0) || !near(b.y0, tt.want.y0) || !near(b.x1, tt.want.x1) || !near(b.y1, tt.want.y1) {
			t.Errorf("bounds(%s) = %+v, want %+v", tt.path, b, tt.want)
		}
	}
}
