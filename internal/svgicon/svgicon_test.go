package svgicon

import (
	"image/color"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestShapes(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"path", `<path d="M10 10h20v20z"/>`, "M 10 10 L 30 10 L 30 30 Z"},
		{"rect", `<rect x="10" y="20" width="30" height="40"/>`, "M 10 20 L 40 20 L 40 60 L 10 60 Z"},
		{"rect defaults to origin", `<rect width="5" height="6"/>`, "M 0 0 L 5 0 L 5 6 L 0 6 Z"},
		{"rect rx and ry", `<rect width="40" height="20" rx="4" ry="2"/>`, rounded(0, 0, 40, 20, 4, 2)},
		{"rect rx only", `<rect width="40" height="20" rx="4"/>`, rounded(0, 0, 40, 20, 4, 4)},
		{"rect ry only", `<rect width="40" height="20" ry="3"/>`, rounded(0, 0, 40, 20, 3, 3)},
		{"rect rx clamped, ry follows then clamps", `<rect width="40" height="20" rx="30"/>`, rounded(0, 0, 40, 20, 20, 10)},
		{"rect percent", `<rect x="10%" y="20%" width="50%" height="25%"/>`, "M 10 20 L 60 20 L 60 45 L 10 45 Z"},
		{"rect units", `<rect width="1in" height="12pt"/>`, "M 0 0 L 96 0 L 96 16 L 0 16 Z"},
		{"circle", `<circle cx="50" cy="40" r="10"/>`, ellipse(50, 40, 10, 10)},
		{"circle percent r", `<circle r="10%"/>`, ellipse(0, 0, 10, 10)},
		{"ellipse", `<ellipse cx="50" cy="40" rx="20" ry="10"/>`, ellipse(50, 40, 20, 10)},
		{"ellipse auto ry", `<ellipse rx="20" ry="auto"/>`, ellipse(0, 0, 20, 20)},
		{"line", `<line x1="1" y1="2" x2="3" y2="4" stroke="red"/>`, "M 1 2 L 3 4"},
		{"polyline", `<polyline points="0,0 10,0 10,10"/>`, "M 0 0 L 10 0 L 10 10"},
		{"polyline loose syntax", `<polyline points=" 0 0,10 0 , 10-10"/>`, "M 0 0 L 10 0 L 10 -10"},
		{"polygon", `<polygon points="0 0 10 0 10 10"/>`, "M 0 0 L 10 0 L 10 10 Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPath(t, one(t, tt.body).Path, tt.want)
		})
	}
}

// rounded and ellipse give the expected outlines; TestRectPathShape pins
// the builders themselves down to exact coordinates.
func rounded(x, y, w, h, rx, ry float64) string { return rectPath(x, y, w, h, rx, ry).String() }

func ellipse(cx, cy, rx, ry float64) string { return ellipsePath(cx, cy, rx, ry).String() }

func TestRectPathShape(t *testing.T) {
	k := kappa
	got := rectPath(0, 0, 40, 20, 4, 2)
	want := P("M 4 0 L 36 0 C " + f(36+4*k) + " 0 40 " + f(2-2*k) + " 40 2 L 40 18 C 40 " + f(18+2*k) + " " + f(36+4*k) +
		" 20 36 20 L 4 20 C " + f(4-4*k) + " 20 0 " + f(18+2*k) + " 0 18 L 0 2 C 0 " + f(2-2*k) + " " + f(4-4*k) + " 0 4 0 Z")
	if !pathEqual(got, want) {
		t.Errorf("rounded rect\n got %s\nwant %s", got, want)
	}
	// Fully rounded sides have no zero-length lines.
	if p := rectPath(0, 0, 40, 20, 20, 10); len(p) != 6 {
		t.Errorf("ellipse-shaped rect = %s, want M, 4 cubics, Z", p)
	}
	e := ellipsePath(0, 0, 10, 5)
	want = P("M 10 0 C 10 " + f(5*k) + " " + f(10*k) + " 5 0 5 C " + f(-10*k) + " 5 -10 " + f(5*k) + " -10 0 C -10 " + f(-5*k) + " " +
		f(-10*k) + " -5 0 -5 C " + f(10*k) + " -5 10 " + f(-5*k) + " 10 0 Z")
	if !pathEqual(e, want) {
		t.Errorf("ellipse\n got %s\nwant %s", e, want)
	}
}

func f(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func TestNotRendered(t *testing.T) {
	for _, body := range []string{
		`<rect width="0" height="10"/>`,
		`<rect width="10"/>`,
		`<circle r="0"/>`,
		`<ellipse rx="0" ry="5"/>`,
		`<path/>`,
		`<path d=""/>`,
		`<polyline points=""/>`,
		`<line x2="10"/>`, // no stroke, and lines never fill
		`<path d="M0 0h10v10z" fill="none"/>`,
		`<path d="M0 0h10v10z" fill="none" stroke="red" stroke-width="0"/>`,
		`<path d="M0 0h10v10z" fill="transparent"/>`,
		`<path d="M0 0h10v10z" display="none"/>`,
		`<g display="none"><path d="M0 0h10v10z"/></g>`,
		`<g style="display:none"><path d="M0 0h10v10z"/></g>`,
		`<path d="M0 0h10v10z" visibility="hidden"/>`,
		`<g transform="scale(0)"><path d="M0 0h10v10z"/></g>`,
		`<defs><path id="a" d="M0 0h10v10z"/></defs>`,
		`<symbol id="s"><path d="M0 0h10v10z"/></symbol>`,
		`<linearGradient id="g"><stop/></linearGradient>`,
	} {
		ic := mustParse(t, doc(body))
		if len(ic.Shapes) != 0 {
			t.Errorf("%s: got %d shapes, want 0", body, len(ic.Shapes))
		}
	}
}

func TestVisibilityOverride(t *testing.T) {
	ic := mustParse(t, doc(`<g visibility="hidden"><path d="M0 0h1v1z"/><path d="M5 5h1v1z" visibility="visible"/></g>`))
	if len(ic.Shapes) != 1 {
		t.Fatalf("got %d shapes, want 1", len(ic.Shapes))
	}
	assertPath(t, ic.Shapes[0].Path, "M 5 5 L 6 5 L 6 6 Z")
}

func TestDefaults(t *testing.T) {
	s := one(t, `<path d="M0 0h10v10z"/>`)
	want := Shape{
		Fill:          Paint{Kind: Solid, Color: color.RGBA{0, 0, 0, 255}},
		MiterLimit:    4,
		Opacity:       1,
		FillOpacity:   1,
		StrokeOpacity: 1,
	}
	s.Path = nil
	if !shapeEqual(s, want) {
		t.Errorf("got %+v\nwant %+v", s, want)
	}
	if s := one(t, `<path d="M0 0h10" fill="none" stroke="red"/>`); s.StrokeWidth != 1 || s.Join != JoinMiter || s.Cap != CapButt {
		t.Errorf("stroke defaults: width %v cap %v join %v", s.StrokeWidth, s.Cap, s.Join)
	}
}

func shapeEqual(a, b Shape) bool {
	if !pathEqual(a.Path, b.Path) || a.Fill.Kind != b.Fill.Kind || a.Fill.Color != b.Fill.Color ||
		a.Stroke.Kind != b.Stroke.Kind || a.Stroke.Color != b.Stroke.Color || a.Cap != b.Cap || a.Join != b.Join ||
		a.FillRule != b.FillRule || len(a.Dash) != len(b.Dash) {
		return false
	}
	for i := range a.Dash {
		if !near(a.Dash[i], b.Dash[i]) {
			return false
		}
	}
	for _, p := range [][2]float64{
		{a.StrokeWidth, b.StrokeWidth}, {a.MiterLimit, b.MiterLimit}, {a.DashOffset, b.DashOffset},
		{a.Opacity, b.Opacity}, {a.FillOpacity, b.FillOpacity}, {a.StrokeOpacity, b.StrokeOpacity},
	} {
		if !near(p[0], p[1]) {
			return false
		}
	}
	return true
}

func TestTransforms(t *testing.T) {
	sq := `<path d="M0 0L10 0L10 10"/>`
	tests := []struct {
		name, transform, want string
	}{
		{"translate x", "translate(10)", "M 10 0 L 20 0 L 20 10"},
		{"translate xy", "translate(10 20)", "M 10 20 L 20 20 L 20 30"},
		{"translate comma", "translate(10,20)", "M 10 20 L 20 20 L 20 30"},
		{"scale", "scale(2)", "M 0 0 L 20 0 L 20 20"},
		{"scale xy", "scale(2, 3)", "M 0 0 L 20 0 L 20 30"},
		{"rotate", "rotate(90)", "M 0 0 L 0 10 L -10 10"},
		{"rotate about centre", "rotate(90 10 0)", "M 10 -10 L 10 0 L 0 0"},
		{"skewX", "skewX(45)", "M 0 0 L 10 0 L 20 10"},
		{"skewY", "skewY(45)", "M 0 0 L 10 10 L 10 20"},
		{"matrix", "matrix(1 0 0 1 5 6)", "M 5 6 L 15 6 L 15 16"},
		{"matrix general", "matrix(2,1,0,1,0,0)", "M 0 0 L 20 10 L 20 20"},
		{"list applies right to left", "translate(10 0) scale(2)", "M 10 0 L 30 0 L 30 20"},
		{"list with commas", "scale(2),translate(10 0)", "M 20 0 L 40 0 L 40 20"},
		{"compact numbers", "translate(1.5.5)", "M 1.5 0.5 L 11.5 0.5 L 11.5 10.5"},
		{"whitespace", "  translate ( 1 , 2 )  ", "M 1 2 L 11 2 L 11 12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPath(t, one(t, `<g transform="`+tt.transform+`">`+sq+`</g>`).Path, tt.want)
		})
	}
	t.Run("nested groups compose", func(t *testing.T) {
		s := one(t, `<g transform="translate(10 0)"><g transform="scale(2)"><path transform="translate(0 5)" d="M0 0L1 0"/></g></g>`)
		assertPath(t, s.Path, "M 10 10 L 12 10")
	})
	t.Run("transform on the shape itself", func(t *testing.T) {
		assertPath(t, one(t, `<rect transform="translate(5 5)" width="1" height="1"/>`).Path, "M 5 5 L 6 5 L 6 6 L 5 6 Z")
	})
	t.Run("bad transform", func(t *testing.T) {
		for _, tr := range []string{"translate(", "scale()", "rotate(1 2)", "matrix(1 2 3)", "translate(1 2) junk"} {
			if _, err := Parse([]byte(doc(`<g transform="` + tr + `"/>`))); err == nil {
				t.Errorf("transform %q parsed", tr)
			}
		}
	})
}

func TestStrokeScaling(t *testing.T) {
	tests := []struct {
		name, transform string
		width, offset   float64
		dash            []float64
	}{
		{"none", "", 2, 1, []float64{4, 2}},
		{"uniform", "scale(3)", 6, 3, []float64{12, 6}},
		{"non-uniform uses sqrt det", "scale(2 8)", 8, 4, []float64{16, 8}},
		{"rotation keeps width", "rotate(30)", 2, 1, []float64{4, 2}},
		{"nested", "scale(2)", 4, 2, []float64{8, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := one(t, `<g transform="`+tt.transform+`"><path d="M0 0h10" stroke="red" stroke-width="2" stroke-dasharray="4 2" stroke-dashoffset="1"/></g>`)
			assertNear(t, "width", s.StrokeWidth, tt.width)
			assertNear(t, "dash offset", s.DashOffset, tt.offset)
			if len(s.Dash) != 2 || !near(s.Dash[0], tt.dash[0]) || !near(s.Dash[1], tt.dash[1]) {
				t.Errorf("dash = %v, want %v", s.Dash, tt.dash)
			}
		})
	}
}

func TestColours(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	tests := []struct {
		value   string
		kind    PaintKind
		col     color.RGBA
		opacity float64
	}{
		{"red", Solid, red, 1},
		{"Red", Solid, red, 1},
		{"#f00", Solid, red, 1},
		{"#F00", Solid, red, 1},
		{"#ff0000", Solid, red, 1},
		{"#FF0000", Solid, red, 1},
		{"#f008", Solid, red, 0x88 / 255.0},
		{"#ff000080", Solid, red, 0x80 / 255.0},
		{"rgb(255,0,0)", Solid, red, 1},
		{"rgb( 255 , 0 , 0 )", Solid, red, 1},
		{"rgb(100%, 0%, 0%)", Solid, red, 1},
		{"rgb(300, -5, 0)", Solid, red, 1},
		{"rgb(255 0 0)", Solid, red, 1},
		{"rgb(255 0 0 / 50%)", Solid, red, 0.5},
		{"rgba(255,0,0,0.25)", Solid, red, 0.25},
		{"RGBA(255,0,0,.5)", Solid, red, 0.5},
		{"rgb(127.5, 0, 0)", Solid, color.RGBA{128, 0, 0, 255}, 1},
		{"hsl(0, 100%, 50%)", Solid, red, 1},
		{"hsl(120deg 100% 25%)", Solid, color.RGBA{0, 128, 0, 255}, 1},
		{"hsla(240, 100%, 50%, 0.5)", Solid, color.RGBA{0, 0, 255, 255}, 0.5},
		{"cornflowerblue", Solid, color.RGBA{0x64, 0x95, 0xed, 255}, 1},
		{"rebeccapurple", Solid, color.RGBA{0x66, 0x33, 0x99, 255}, 1},
		{"lightgoldenrodyellow", Solid, color.RGBA{0xfa, 0xfa, 0xd2, 255}, 1},
		{"grey", Solid, color.RGBA{128, 128, 128, 255}, 1},
		{"currentColor", CurrentColor, color.RGBA{}, 1},
		{"currentcolor", CurrentColor, color.RGBA{}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			s := one(t, `<path d="M0 0h1v1z" fill="`+tt.value+`"/>`)
			if s.Fill.Kind != tt.kind || s.Fill.Color != tt.col {
				t.Errorf("fill = %+v, want kind %v colour %v", s.Fill, tt.kind, tt.col)
			}
			assertNear(t, "fill opacity", s.FillOpacity, tt.opacity)
			// The same value works as a stroke.
			s = one(t, `<path d="M0 0h1" fill="none" stroke="`+tt.value+`"/>`)
			if s.Stroke.Kind != tt.kind || s.Stroke.Color != tt.col {
				t.Errorf("stroke = %+v, want kind %v colour %v", s.Stroke, tt.kind, tt.col)
			}
			assertNear(t, "stroke opacity", s.StrokeOpacity, tt.opacity)
		})
	}
	if len(namedColors) != 148 {
		t.Errorf("namedColors has %d entries, want the 148 CSS named colours", len(namedColors))
	}
	for _, bad := range []string{"#ff", "#ggg", "rgb(1,2)", "notacolour", "rgb(1,2,3", "url(#a"} {
		if _, err := Parse([]byte(doc(`<path d="M0 0h1v1z" fill="` + bad + `"/>`))); err == nil {
			t.Errorf("fill %q parsed", bad)
		}
	}
}

func TestInheritance(t *testing.T) {
	blue := color.RGBA{0, 0, 255, 255}
	red := color.RGBA{255, 0, 0, 255}
	t.Run("fill from group", func(t *testing.T) {
		if s := one(t, `<g fill="blue"><path d="M0 0h1v1z"/></g>`); s.Fill.Color != blue {
			t.Errorf("fill = %v", s.Fill)
		}
	})
	t.Run("child overrides", func(t *testing.T) {
		if s := one(t, `<g fill="blue"><path fill="red" d="M0 0h1v1z"/></g>`); s.Fill.Color != red {
			t.Errorf("fill = %v", s.Fill)
		}
	})
	t.Run("from root svg", func(t *testing.T) {
		ic := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M0 0h1"/></svg>`)
		s := ic.Shapes[0]
		if s.Fill.Kind != None || s.Stroke.Kind != CurrentColor || s.StrokeWidth != 2 || s.Cap != CapRound || s.Join != JoinRound {
			t.Errorf("got %+v", s)
		}
	})
	t.Run("deep", func(t *testing.T) {
		s := one(t, `<g stroke="red" stroke-width="3" stroke-dasharray="1 2" fill-rule="evenodd" fill-opacity=".5"><g><g><path d="M0 0h1v1z"/></g></g></g>`)
		if s.Stroke.Color != red || s.StrokeWidth != 3 || len(s.Dash) != 2 || s.FillRule != EvenOdd || s.FillOpacity != 0.5 {
			t.Errorf("got %+v", s)
		}
	})
	t.Run("inherit keyword", func(t *testing.T) {
		if s := one(t, `<g fill="blue"><path fill="inherit" d="M0 0h1v1z"/></g>`); s.Fill.Color != blue {
			t.Errorf("fill = %v", s.Fill)
		}
		if s := one(t, `<g fill="blue"><path fill="red" style="fill:inherit" d="M0 0h1v1z"/></g>`); s.Fill.Color != blue {
			t.Errorf("style inherit: fill = %v", s.Fill)
		}
	})
	t.Run("opacity multiplies and isn't inherited as a value", func(t *testing.T) {
		s := one(t, `<g opacity=".5"><g opacity="50%"><path opacity=".8" d="M0 0h1v1z"/></g></g>`)
		assertNear(t, "opacity", s.Opacity, 0.2)
	})
	t.Run("root opacity", func(t *testing.T) {
		ic := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1" opacity="0.5"><path d="M0 0h1v1z"/></svg>`)
		assertNear(t, "opacity", ic.Shapes[0].Opacity, 0.5)
	})
	t.Run("fill and stroke opacity", func(t *testing.T) {
		s := one(t, `<g fill-opacity="0.5"><path stroke="red" stroke-opacity="0.25" fill="rgba(0,0,255,.5)" d="M0 0h1v1z"/></g>`)
		assertNear(t, "fill opacity", s.FillOpacity, 0.25)
		assertNear(t, "stroke opacity", s.StrokeOpacity, 0.25)
	})
	t.Run("opacity clamps", func(t *testing.T) {
		s := one(t, `<path opacity="2" fill-opacity="-1" d="M0 0h1v1z"/>`)
		if s.Opacity != 1 || s.FillOpacity != 0 {
			t.Errorf("opacity %v fill-opacity %v", s.Opacity, s.FillOpacity)
		}
	})
	t.Run("color property resolves currentColor", func(t *testing.T) {
		s := one(t, `<g color="red"><path fill="currentColor" d="M0 0h1v1z"/></g>`)
		if s.Fill.Kind != Solid || s.Fill.Color != red {
			t.Errorf("fill = %+v", s.Fill)
		}
	})
	t.Run("currentColor inherits as a keyword", func(t *testing.T) {
		s := one(t, `<g fill="currentColor" color="red"><path color="blue" d="M0 0h1v1z"/></g>`)
		if s.Fill.Color != blue {
			t.Errorf("fill = %+v, want the child's colour", s.Fill)
		}
	})
	t.Run("color with alpha", func(t *testing.T) {
		s := one(t, `<path style="color: rgba(255,0,0,.5)" fill="currentColor" d="M0 0h1v1z"/>`)
		if s.Fill.Color != red {
			t.Errorf("fill = %+v", s.Fill)
		}
		assertNear(t, "fill opacity", s.FillOpacity, 0.5)
	})
	t.Run("currentColor stays symbolic", func(t *testing.T) {
		s := one(t, `<g color="currentColor"><path fill="currentColor" d="M0 0h1v1z"/></g>`)
		if s.Fill.Kind != CurrentColor {
			t.Errorf("fill = %+v", s.Fill)
		}
	})
}

func TestStyleAttribute(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	tests := []struct {
		name, body string
		check      func(Shape) bool
	}{
		{"fill", `<path style="fill:red" d="M0 0h1v1z"/>`, func(s Shape) bool { return s.Fill.Color == red }},
		{"overrides attribute", `<path fill="blue" style="fill: red" d="M0 0h1v1z"/>`, func(s Shape) bool { return s.Fill.Color == red }},
		{"important", `<path fill="blue" style="fill: red !important" d="M0 0h1v1z"/>`, func(s Shape) bool { return s.Fill.Color == red }},
		{"case-insensitive names", `<path style="FILL:red" d="M0 0h1v1z"/>`, func(s Shape) bool { return s.Fill.Color == red }},
		{"comments and empty declarations", `<path style="/* x */ ;fill:red;;" d="M0 0h1v1z"/>`, func(s Shape) bool { return s.Fill.Color == red }},
		{"inherited from group style", `<g style="fill:red"><path d="M0 0h1v1z"/></g>`, func(s Shape) bool { return s.Fill.Color == red }},
		{"unknown properties ignored", `<path style="font-family:Arial;fill:red;-inkscape-font-specification:x" d="M0 0h1v1z"/>`, func(s Shape) bool { return s.Fill.Color == red }},
		{"many properties", `<path style="fill:none;stroke:#f00;stroke-width:3px;stroke-linecap:square;stroke-linejoin:bevel;stroke-miterlimit:10;stroke-dasharray:1,2,3;stroke-dashoffset:2;stroke-opacity:.5;opacity:.5;fill-rule:evenodd" d="M0 0h1"/>`,
			func(s Shape) bool {
				return s.Fill.Kind == None && s.Stroke.Color == red && s.StrokeWidth == 3 && s.Cap == CapSquare &&
					s.Join == JoinBevel && s.MiterLimit == 10 && len(s.Dash) == 6 && s.Dash[3] == 1 && s.DashOffset == 2 &&
					s.StrokeOpacity == 0.5 && s.Opacity == 0.5 && s.FillRule == EvenOdd
			}},
		{"url with quotes", `<defs><linearGradient id="g"><stop/><stop offset="1"/></linearGradient></defs><path style="fill:url('#g')" d="M0 0h1v1z"/>`,
			func(s Shape) bool { return s.Fill.Kind == Linear }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if s := one(t, tt.body); !tt.check(s) {
				t.Errorf("got %+v", s)
			}
		})
	}
}

func TestStrokeProperties(t *testing.T) {
	stroke := func(attrs string) Shape {
		return one(t, `<path d="M0 0h10" fill="none" stroke="red" `+attrs+`/>`)
	}
	for _, c := range []struct {
		v    string
		want LineCap
	}{{"butt", CapButt}, {"round", CapRound}, {"square", CapSquare}} {
		if s := stroke(`stroke-linecap="` + c.v + `"`); s.Cap != c.want {
			t.Errorf("cap %s = %v", c.v, s.Cap)
		}
	}
	for _, j := range []struct {
		v    string
		want LineJoin
	}{{"miter", JoinMiter}, {"round", JoinRound}, {"bevel", JoinBevel}} {
		if s := stroke(`stroke-linejoin="` + j.v + `"`); s.Join != j.want {
			t.Errorf("join %s = %v", j.v, s.Join)
		}
	}
	if s := stroke(`stroke-miterlimit="2.5"`); s.MiterLimit != 2.5 {
		t.Errorf("miterlimit = %v", s.MiterLimit)
	}
	if s := stroke(`stroke-width="10%"`); !near(s.StrokeWidth, 10) {
		t.Errorf("percent width = %v", s.StrokeWidth)
	}
	for _, d := range []struct {
		v    string
		want []float64
	}{
		{"5 3", []float64{5, 3}},
		{"5,3,2", []float64{5, 3, 2, 5, 3, 2}},
		{"5, 3 ,2 ,1", []float64{5, 3, 2, 1}},
		{"none", nil},
		{"0 0", nil},
		{"1%", []float64{1, 1}},
	} {
		s := stroke(`stroke-dasharray="` + d.v + `"`)
		if len(s.Dash) != len(d.want) {
			t.Errorf("dasharray %q = %v, want %v", d.v, s.Dash, d.want)
			continue
		}
		for i := range d.want {
			if !near(s.Dash[i], d.want[i]) {
				t.Errorf("dasharray %q = %v, want %v", d.v, s.Dash, d.want)
			}
		}
	}
	if s := stroke(`stroke-dasharray="2" stroke-dashoffset="-3"`); s.DashOffset != -3 {
		t.Errorf("dashoffset = %v", s.DashOffset)
	}
	if s := one(t, `<path d="M0 0h10v10z" fill-rule="evenodd"/>`); s.FillRule != EvenOdd {
		t.Errorf("fill-rule = %v", s.FillRule)
	}
	if s := one(t, `<g fill-rule="evenodd"><path d="M0 0h10v10z" fill-rule="nonzero"/></g>`); s.FillRule != NonZero {
		t.Errorf("fill-rule override = %v", s.FillRule)
	}
	for _, bad := range []string{
		`stroke-width="-1"`, `stroke-miterlimit="0.5"`, `stroke-dasharray="1 -2"`, `stroke-linecap="pointy"`,
		`stroke-linejoin="sharp"`, `fill-rule="odd"`, `stroke-width="wide"`, `opacity="half"`,
	} {
		if _, err := Parse([]byte(doc(`<path d="M0 0h10" stroke="red" ` + bad + `/>`))); err == nil {
			t.Errorf("%s parsed", bad)
		}
	}
}

func TestViewBox(t *testing.T) {
	ic := mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-5,-5 10 20"><circle r="1"/></svg>`)
	if ic.ViewBox != [4]float64{-5, -5, 10, 20} {
		t.Errorf("viewBox = %v", ic.ViewBox)
	}
	// Shapes stay in viewBox coordinates; the root's width doesn't scale them.
	ic = mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24"><path d="M1 1h1"  stroke="red"/></svg>`)
	assertPath(t, ic.Shapes[0].Path, "M 1 1 L 2 1")
	ic = mustParse(t, `<svg xmlns="http://www.w3.org/2000/svg" width="32px" height="16"><rect width="50%" height="50%"/></svg>`)
	if ic.ViewBox != [4]float64{0, 0, 32, 16} {
		t.Errorf("viewBox from size = %v", ic.ViewBox)
	}
	assertPath(t, ic.Shapes[0].Path, "M 0 0 L 16 0 L 16 8 L 0 8 Z")
	for _, bad := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 10"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="100%"/>`,
		`<html/>`,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path></svg>`,
		``,
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestNoNamespace(t *testing.T) {
	// Hand-written icons often omit xmlns.
	ic := mustParse(t, `<svg viewBox="0 0 10 10"><rect width="1" height="1"/></svg>`)
	if len(ic.Shapes) != 1 {
		t.Errorf("got %d shapes", len(ic.Shapes))
	}
	// A prefixed SVG namespace works too.
	ic = mustParse(t, `<s:svg xmlns:s="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><s:rect width="1" height="1"/></s:svg>`)
	if len(ic.Shapes) != 1 {
		t.Errorf("prefixed: got %d shapes", len(ic.Shapes))
	}
}

func TestIgnored(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<!-- Created with Inkscape -->
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
     xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"
     xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape"
     xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:dc="http://purl.org/dc/elements/1.1/"
     viewBox="0 0 10 10" version="1.1" id="svg1" sodipodi:docname="x.svg" inkscape:version="1.3"
     class="icon" data-name="thing" aria-hidden="true" xml:space="preserve">
  <title>Title</title>
  <desc>A <b>description</b></desc>
  <metadata><rdf:RDF><dc:title>x</dc:title><text>not svg</text></rdf:RDF></metadata>
  <sodipodi:namedview id="nv" pagecolor="#ffffff" inkscape:zoom="1"><inkscape:grid/></sodipodi:namedview>
  <style/>
  <style>  </style>
  <defs id="defs1"/>
  <g inkscape:label="Layer 1" inkscape:groupmode="layer" class="layer" shape-rendering="crispEdges">
    <!-- a comment -->
    <path class="st0" d="M0 0h1v1z" sodipodi:nodetypes="cccc" unknown-attr="5" clip-rule="evenodd" font-family="Arial"/>
  </g>
  <view id="v" viewBox="0 0 5 5"/>
</svg>`
	ic := mustParse(t, src)
	if len(ic.Shapes) != 1 {
		t.Fatalf("got %d shapes, want 1", len(ic.Shapes))
	}
}

func TestUse(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	t.Run("href with x y", func(t *testing.T) {
		ic := mustParse(t, doc(`<defs><rect id="r" width="1" height="1"/></defs><use href="#r" x="10" y="20"/>`))
		if len(ic.Shapes) != 1 {
			t.Fatalf("got %d shapes", len(ic.Shapes))
		}
		assertPath(t, ic.Shapes[0].Path, "M 10 20 L 11 20 L 11 21 L 10 21 Z")
	})
	t.Run("xlink href", func(t *testing.T) {
		s := one(t, `<defs><rect id="r" width="1" height="1"/></defs><use xlink:href="#r" x="5"/>`)
		assertPath(t, s.Path, "M 5 0 L 6 0 L 6 1 L 5 1 Z")
	})
	t.Run("href wins over xlink href", func(t *testing.T) {
		s := one(t, `<defs><rect id="a" width="1" height="1"/><rect id="b" width="2" height="2"/></defs><use href="#b" xlink:href="#a"/>`)
		assertPath(t, s.Path, "M 0 0 L 2 0 L 2 2 L 0 2 Z")
	})
	t.Run("inherits from the use, not from the original", func(t *testing.T) {
		s := one(t, `<defs fill="blue"><rect id="r" width="1" height="1"/></defs><use href="#r" fill="red"/>`)
		if s.Fill.Color != red {
			t.Errorf("fill = %v", s.Fill)
		}
	})
	t.Run("referenced element keeps its own properties and transform", func(t *testing.T) {
		s := one(t, `<defs><rect id="r" width="1" height="1" fill="red" transform="scale(2)"/></defs><use href="#r" fill="blue" x="10"/>`)
		if s.Fill.Color != red {
			t.Errorf("fill = %v", s.Fill)
		}
		assertPath(t, s.Path, "M 10 0 L 12 0 L 12 2 L 10 2 Z")
	})
	t.Run("use transform applies before x y", func(t *testing.T) {
		s := one(t, `<defs><path id="p" d="M0 0h1"/></defs><use href="#p" transform="scale(2)" x="1" stroke="red"/>`)
		assertPath(t, s.Path, "M 2 0 L 4 0")
	})
	t.Run("group", func(t *testing.T) {
		ic := mustParse(t, doc(`<defs><g id="g"><rect width="1" height="1"/><circle r="1"/></g></defs><use href="#g"/><use href="#g" x="5"/>`))
		if len(ic.Shapes) != 4 {
			t.Fatalf("got %d shapes", len(ic.Shapes))
		}
	})
	t.Run("reuse of a visible element", func(t *testing.T) {
		ic := mustParse(t, doc(`<rect id="r" width="1" height="1"/><use href="#r" y="2"/>`))
		if len(ic.Shapes) != 2 {
			t.Fatalf("got %d shapes", len(ic.Shapes))
		}
		assertPath(t, ic.Shapes[1].Path, "M 0 2 L 1 2 L 1 3 L 0 3 Z")
	})
	t.Run("nested use", func(t *testing.T) {
		s := one(t, `<defs><rect id="r" width="1" height="1"/><use id="u" href="#r" x="1"/></defs><use href="#u" x="1"/>`)
		assertPath(t, s.Path, "M 2 0 L 3 0 L 3 1 L 2 1 Z")
	})
	t.Run("opacity", func(t *testing.T) {
		s := one(t, `<defs><rect id="r" width="1" height="1" opacity=".5"/></defs><use href="#r" opacity=".5"/>`)
		assertNear(t, "opacity", s.Opacity, 0.25)
	})
	t.Run("symbol with viewBox", func(t *testing.T) {
		s := one(t, `<defs><symbol id="s" viewBox="0 0 10 10"><rect width="10" height="10"/></symbol></defs><use href="#s" x="20" y="20" width="50" height="50"/>`)
		assertPath(t, s.Path, "M 20 20 L 70 20 L 70 70 L 20 70 Z")
	})
	t.Run("symbol letterboxed", func(t *testing.T) {
		s := one(t, `<symbol id="s" viewBox="0 0 10 10"><rect width="10" height="10"/></symbol><use href="#s" width="50" height="30"/>`)
		assertPath(t, s.Path, "M 10 0 L 40 0 L 40 30 L 10 30 Z")
	})
	t.Run("symbol stroke scales", func(t *testing.T) {
		s := one(t, `<symbol id="s" viewBox="0 0 10 10"><path d="M1 5h8" stroke="red" stroke-width="2"/></symbol><use href="#s" width="50" height="50"/>`)
		assertNear(t, "stroke width", s.StrokeWidth, 10)
	})
	t.Run("symbol without size fills the viewport", func(t *testing.T) {
		s := one(t, `<symbol id="s" viewBox="0 0 10 10"><rect width="10" height="10"/></symbol><use href="#s"/>`)
		assertPath(t, s.Path, "M 0 0 L 100 0 L 100 100 L 0 100 Z")
	})
	t.Run("missing", func(t *testing.T) {
		_, err := Parse([]byte(doc(`<use href="#nope"/>`)))
		if f := unsupportedFeature(t, err); f != "missing reference #nope" {
			t.Errorf("feature = %q", f)
		}
	})
	t.Run("cycle", func(t *testing.T) {
		for _, body := range []string{
			`<use id="a" href="#a"/>`,
			`<g id="g"><use href="#g"/></g>`,
			`<defs><use id="a" href="#b"/><use id="b" href="#a"/></defs><use href="#a"/>`,
		} {
			if _, err := Parse([]byte(doc(body))); err == nil {
				t.Errorf("%s parsed", body)
			}
		}
	})
	t.Run("exponential fan-out is bounded", func(t *testing.T) {
		body := `<defs><rect id="l0" width="1" height="1"/>`
		for i := 1; i <= 12; i++ {
			prev := "#l" + strconv.Itoa(i-1)
			body += `<g id="l` + strconv.Itoa(i) + `">` + strings.Repeat(`<use href="`+prev+`"/>`, 10) + `</g>`
		}
		body += `</defs><use href="#l12"/>`
		if _, err := Parse([]byte(doc(body))); err == nil {
			t.Error("parsed 10^12 instances")
		}
	})
	t.Run("no href", func(t *testing.T) {
		if ic := mustParse(t, doc(`<use/>`)); len(ic.Shapes) != 0 {
			t.Errorf("got shapes")
		}
	})
}

func TestNestedSVG(t *testing.T) {
	t.Run("viewport and viewBox", func(t *testing.T) {
		s := one(t, `<svg x="10" y="20" width="40" height="40" viewBox="0 0 4 4"><rect x="1" y="1" width="2" height="2"/></svg>`)
		assertPath(t, s.Path, "M 20 30 L 40 30 L 40 50 L 20 50 Z")
	})
	t.Run("viewBox offset", func(t *testing.T) {
		s := one(t, `<svg width="10" height="10" viewBox="5 5 10 10"><rect x="5" y="5" width="10" height="10"/></svg>`)
		assertPath(t, s.Path, "M 0 0 L 10 0 L 10 10 L 0 10 Z")
	})
	t.Run("no viewBox translates only", func(t *testing.T) {
		s := one(t, `<svg x="5" y="5" width="10" height="10"><rect width="10" height="10"/></svg>`)
		assertPath(t, s.Path, "M 5 5 L 15 5 L 15 15 L 5 15 Z")
	})
	t.Run("preserveAspectRatio none", func(t *testing.T) {
		s := one(t, `<svg width="20" height="10" viewBox="0 0 1 1" preserveAspectRatio="none"><rect width="1" height="1"/></svg>`)
		assertPath(t, s.Path, "M 0 0 L 20 0 L 20 10 L 0 10 Z")
	})
	t.Run("preserveAspectRatio xMaxYMin", func(t *testing.T) {
		s := one(t, `<svg width="20" height="10" viewBox="0 0 1 1" preserveAspectRatio="xMaxYMin meet"><rect width="1" height="1"/></svg>`)
		assertPath(t, s.Path, "M 10 0 L 20 0 L 20 10 L 10 10 Z")
	})
	t.Run("percentages resolve against the nested viewBox", func(t *testing.T) {
		s := one(t, `<svg width="50" height="50" viewBox="0 0 10 10"><rect width="50%" height="100%"/></svg>`)
		assertPath(t, s.Path, "M 0 0 L 25 0 L 25 50 L 0 50 Z")
	})
	t.Run("inherits", func(t *testing.T) {
		s := one(t, `<g fill="red"><svg width="10" height="10"><rect width="1" height="1"/></svg></g>`)
		if s.Fill.Color != (color.RGBA{255, 0, 0, 255}) {
			t.Errorf("fill = %v", s.Fill)
		}
	})
	t.Run("overflow is clipped and so unsupported", func(t *testing.T) {
		_, err := Parse([]byte(doc(`<svg width="10" height="10"><rect x="5" width="10" height="10"/></svg>`)))
		if f := unsupportedFeature(t, err); f != "content overflowing the viewport (clipping)" {
			t.Errorf("feature = %q", f)
		}
	})
	t.Run("stroke overflow counts", func(t *testing.T) {
		_, err := Parse([]byte(doc(`<svg width="10" height="10"><rect width="10" height="10" fill="none" stroke="red"/></svg>`)))
		unsupportedFeature(t, err)
	})
	t.Run("overflow visible", func(t *testing.T) {
		one(t, `<svg width="10" height="10" overflow="visible"><rect x="5" width="10" height="10"/></svg>`)
		one(t, `<svg width="10" height="10" style="overflow:visible"><rect x="5" width="10" height="10"/></svg>`)
	})
	t.Run("gradient follows the nested transform", func(t *testing.T) {
		s := one(t, `<linearGradient id="g" gradientUnits="userSpaceOnUse" x2="1"><stop/><stop offset="1"/></linearGradient>
			<svg x="10" width="20" height="20" viewBox="0 0 2 2"><rect width="2" height="2" fill="url(#g)"/></svg>`)
		assertMatrix(t, s.Fill.Gradient.Transform, Matrix{10, 0, 0, 10, 10, 0})
	})
}

func TestMatrix(t *testing.T) {
	m := Matrix{2, 1, -1, 3, 5, 7}
	inv, ok := m.Invert()
	if !ok {
		t.Fatal("not invertible")
	}
	assertMatrix(t, m.Mul(inv), Identity)
	x, y := m.Apply(1, 1)
	if x != 6 || y != 11 {
		t.Errorf("Apply = %v, %v", x, y)
	}
	if _, ok := (Matrix{1, 2, 2, 4, 0, 0}).Invert(); ok {
		t.Error("singular matrix inverted")
	}
	assertNear(t, "scale", Matrix{2, 0, 0, 8, 0, 0}.scale(), 4)
	_ = math.Pi
}
