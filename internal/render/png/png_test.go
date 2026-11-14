package png

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	stdpng "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/icons"
	"github.com/grindlemire/diagram/icons/brands"
	"github.com/grindlemire/diagram/icons/generic"
	"github.com/grindlemire/diagram/internal/scene"
	"github.com/grindlemire/diagram/internal/text"
	"github.com/grindlemire/diagram/themes"
)

var update = flag.Bool("update", false, "rewrite golden PNGs in testdata/")

var black = themes.RGB(0, 0, 0)

func render(t *testing.T, w, h, scale float64, items ...scene.Item) *image.RGBA {
	t.Helper()
	img, err := Rasterize(&scene.Scene{Width: w, Height: h, Theme: themes.Light, Items: items}, Options{Scale: scale})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func alpha(img *image.RGBA, x, y int) uint8 { return img.RGBAAt(x, y).A }

func line(x0, y0, x1, y1 float64, p scene.Paint) *scene.Shape {
	var path scene.Path
	path.MoveTo(x0, y0)
	path.LineTo(x1, y1)
	return &scene.Shape{Path: path, Paint: p}
}

func stroked(w float64, c scene.LineCap) scene.Paint {
	return scene.Paint{Stroke: black, StrokeWidth: w, Cap: c}
}

// inkBounds returns the bounding box of pixels with alpha above min.
func inkBounds(img *image.RGBA, min uint8) image.Rectangle {
	var r image.Rectangle
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if alpha(img, x, y) > min {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}

func TestStrokeButtCoversExactPixels(t *testing.T) {
	img := render(t, 40, 20, 1, line(10, 10, 30, 10, stroked(4, scene.CapButt)))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			want := uint8(0)
			if x >= 10 && x < 30 && y >= 8 && y < 12 {
				want = 255
			}
			if got := alpha(img, x, y); got != want {
				t.Fatalf("pixel (%d,%d) alpha %d, want %d", x, y, got, want)
			}
		}
	}
}

func TestStrokeCaps(t *testing.T) {
	round := render(t, 40, 20, 1, line(10, 10, 30, 10, stroked(4, scene.CapRound)))
	if got := inkBounds(round, 0); got != image.Rect(8, 8, 32, 12) {
		t.Errorf("round cap ink %v, want (8,8)-(32,12)", got)
	}
	// The cap is a half disc: the outer corner pixel is mostly empty, the
	// pixel on the axis nearly full.
	if a := alpha(round, 8, 8); a > 80 {
		t.Errorf("round cap corner alpha %d, want small", a)
	}
	if a := alpha(round, 8, 10); a < 150 {
		t.Errorf("round cap axis alpha %d, want large", a)
	}
	square := render(t, 40, 20, 1, line(10, 10, 30, 10, stroked(4, scene.CapSquare)))
	if got := inkBounds(square, 254); got != image.Rect(8, 8, 32, 12) {
		t.Errorf("square cap solid ink %v, want (8,8)-(32,12)", got)
	}
}

func TestStrokeHalfPixelWidths(t *testing.T) {
	// A 1px line on a pixel boundary spreads over two rows at half
	// coverage, as in browsers; on a pixel centre it fills one row.
	img := render(t, 20, 20, 1, line(2, 10, 18, 10, stroked(1, scene.CapButt)))
	if a, b := alpha(img, 10, 9), alpha(img, 10, 10); a < 125 || a > 130 || b < 125 || b > 130 {
		t.Errorf("1px line on boundary: rows alpha %d, %d, want ~128", a, b)
	}
	img = render(t, 20, 20, 1, line(2, 10.5, 18, 10.5, stroked(1, scene.CapButt)))
	if a := alpha(img, 10, 10); a != 255 {
		t.Errorf("1px line on centre: alpha %d, want 255", a)
	}
	// 1.5px at scale 2 is 3px: rows 8.5–11.5 around y=10 → 3 full-ish rows.
	img = render(t, 20, 20, 2, line(2, 5, 18, 5, stroked(1.5, scene.CapButt)))
	col := []uint8{alpha(img, 20, 7), alpha(img, 20, 8), alpha(img, 20, 9), alpha(img, 20, 10), alpha(img, 20, 11), alpha(img, 20, 12)}
	if col[0] != 0 || col[1] < 125 || col[1] > 130 || col[2] != 255 || col[3] != 255 || col[4] < 125 || col[4] > 130 || col[5] != 0 {
		t.Errorf("1.5px at scale 2 column alphas %v", col)
	}
}

func TestStrokeDash(t *testing.T) {
	p := stroked(2, scene.CapButt)
	p.Dash = []float64{10, 10}
	img := render(t, 100, 20, 1, line(0, 10, 100, 10, p))
	for x := 0; x < 100; x++ {
		on := (x/10)%2 == 0
		got := alpha(img, x, 10)
		if on && got != 255 || !on && got != 0 {
			t.Fatalf("x=%d alpha %d, on=%v", x, got, on)
		}
	}
	// Odd-length arrays repeat: [5] is [5 5].
	p.Dash = []float64{5}
	img = render(t, 100, 20, 1, line(0, 10, 100, 10, p))
	if alpha(img, 2, 10) != 255 || alpha(img, 7, 10) != 0 || alpha(img, 12, 10) != 255 {
		t.Error("odd dash array not repeated")
	}
}

func TestStrokeDotted(t *testing.T) {
	// paint's dotted style: near-zero dashes with round caps make dots.
	p := stroked(2, scene.CapRound)
	p.Dash = []float64{0.01, 5}
	img := render(t, 40, 20, 4, line(2, 10, 38, 10, p))
	for _, x := range []float64{2, 7.01, 12.02, 37.07} {
		if a := alpha(img, int(x*4), 40); a < 200 {
			t.Errorf("dot at x=%v alpha %d", x, a)
		}
	}
	for _, x := range []float64{4.5, 9.5} {
		if a := alpha(img, int(x*4), 40); a != 0 {
			t.Errorf("gap at x=%v alpha %d", x, a)
		}
	}
	// A zero-length subpath with a round cap draws a dot too.
	img = render(t, 20, 20, 1, line(10, 10, 10, 10, stroked(4, scene.CapRound)))
	if got := inkBounds(img, 0); got != image.Rect(8, 8, 12, 12) {
		t.Errorf("zero-length round dot ink %v", got)
	}
	img = render(t, 20, 20, 1, line(10, 10, 10, 10, stroked(4, scene.CapButt)))
	if got := inkBounds(img, 0); !got.Empty() {
		t.Errorf("zero-length butt line drew %v", got)
	}
}

func corner(join scene.LineJoin) *scene.Shape {
	var path scene.Path
	path.MoveTo(10, 30)
	path.LineTo(10, 10)
	path.LineTo(30, 10)
	return &scene.Shape{Path: path, Paint: scene.Paint{Stroke: black, StrokeWidth: 4, Join: join}}
}

func TestStrokeJoins(t *testing.T) {
	miter := alpha(render(t, 40, 40, 1, corner(scene.JoinMiter)), 8, 8)
	round := alpha(render(t, 40, 40, 1, corner(scene.JoinRound)), 8, 8)
	bevel := alpha(render(t, 40, 40, 1, corner(scene.JoinBevel)), 8, 8)
	if miter != 255 || bevel > 10 || round <= bevel || round >= miter {
		t.Errorf("outer corner alpha miter %d round %d bevel %d", miter, round, bevel)
	}
	// The inner corner is solid for every join.
	for _, j := range []scene.LineJoin{scene.JoinMiter, scene.JoinRound, scene.JoinBevel} {
		img := render(t, 40, 40, 1, corner(j))
		if a := alpha(img, 11, 11); a != 255 {
			t.Errorf("join %d inner corner alpha %d", j, a)
		}
	}
}

func TestStrokeMiterLimit(t *testing.T) {
	// A 20° spike has miter ratio 1/sin(10°) ≈ 5.8 > 4, so it is bevelled
	// and nothing reaches far past the vertex.
	var path scene.Path
	path.MoveTo(10, 20)
	path.LineTo(50, 20)
	path.LineTo(50-40*math.Cos(20*math.Pi/180), 20-40*math.Sin(20*math.Pi/180))
	img := render(t, 80, 40, 1, &scene.Shape{Path: path, Paint: scene.Paint{Stroke: black, StrokeWidth: 4, Join: scene.JoinMiter}})
	if b := inkBounds(img, 0); b.Max.X > 53 {
		t.Errorf("miter limit not applied: ink reaches x=%d", b.Max.X)
	}
}

func TestTranslucentStrokeHasNoOverlap(t *testing.T) {
	// Joins and tight turns overlap in the outline; nonzero filling paints
	// every pixel once, so no pixel is darker than the stroke's alpha.
	var path scene.Path
	path.MoveTo(5, 5)
	for i := range 8 {
		path.LineTo(10+float64(i)*4, 35)
		path.LineTo(12+float64(i)*4, 5)
	}
	p := scene.Paint{Stroke: black, StrokeWidth: 6, Join: scene.JoinRound, Cap: scene.CapRound, Opacity: 0.5}
	img := render(t, 60, 40, 1, &scene.Shape{Path: path, Paint: p})
	maxA := uint8(0)
	for i := 3; i < len(img.Pix); i += 4 {
		maxA = max(maxA, img.Pix[i])
	}
	if maxA != 128 {
		t.Errorf("max alpha %d, want 128", maxA)
	}
}

func TestClosedStroke(t *testing.T) {
	img := render(t, 40, 40, 1, &scene.Rect{X: 10, Y: 10, W: 20, H: 20, Paint: scene.Paint{Stroke: black, StrokeWidth: 2}})
	if a := alpha(img, 20, 20); a != 0 {
		t.Errorf("interior alpha %d", a)
	}
	for _, p := range [][2]int{{9, 9}, {10, 20}, {20, 9}, {30, 30}, {29, 20}} {
		if a := alpha(img, p[0], p[1]); a != 255 {
			t.Errorf("ring pixel %v alpha %d", p, a)
		}
	}
	if got := inkBounds(img, 0); got != image.Rect(9, 9, 31, 31) {
		t.Errorf("ink %v", got)
	}
}

func TestCompositing(t *testing.T) {
	sc := &scene.Scene{Width: 10, Height: 10, Theme: themes.Light, Background: themes.RGB(255, 255, 255), Items: []scene.Item{
		&scene.Rect{X: 0, Y: 0, W: 10, H: 10, Paint: scene.Paint{Fill: themes.RGB(255, 0, 0), Opacity: 0.5}},
	}}
	img, err := Rasterize(sc, Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(5, 5); !near(got, color.RGBA{255, 127, 127, 255}, 1) {
		t.Errorf("50%% red over white = %v", got)
	}
	// Colour alpha and opacity multiply; transparent background stays
	// premultiplied in memory and straight in the PNG.
	img = render(t, 10, 10, 1,
		&scene.Rect{X: 0, Y: 0, W: 10, H: 10, Paint: scene.Paint{Fill: themes.RGBA(0, 0, 255, 128)}},
		&scene.Rect{X: 0, Y: 0, W: 5, H: 10, Paint: scene.Paint{Fill: themes.RGBA(0, 0, 255, 128)}},
	)
	if got := img.RGBAAt(8, 5); !near(got, color.RGBA{0, 0, 128, 128}, 1) {
		t.Errorf("translucent blue = %v", got)
	}
	// 0.5 over 0.5 gives 0.75.
	if got := img.RGBAAt(2, 5); !near(got, color.RGBA{0, 0, 192, 192}, 1) {
		t.Errorf("two layers = %v", got)
	}
	var buf bytes.Buffer
	if err := Render(&buf, &scene.Scene{Width: 4, Height: 4, Theme: themes.Light, Items: []scene.Item{
		&scene.Rect{X: 0, Y: 0, W: 4, H: 4, Paint: scene.Paint{Fill: themes.RGBA(0, 0, 255, 128)}},
	}}, Options{Scale: 1}); err != nil {
		t.Fatal(err)
	}
	dec, err := stdpng.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(dec.At(1, 1)).(color.NRGBA); got.B < 253 || got.A != 128 {
		t.Errorf("decoded PNG pixel %v, want straight blue at alpha 128", got)
	}
}

func near(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) bool { return abs(int(x)-int(y)) <= tol }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestImageSize(t *testing.T) {
	img := render(t, 100.2, 50, 2)
	if b := img.Bounds(); b.Dx() != 201 || b.Dy() != 100 {
		t.Errorf("size %v, want 201×100", b)
	}
	img = render(t, 10, 10, 0) // default scale
	if b := img.Bounds(); b.Dx() != 20 {
		t.Errorf("default scale size %v", b)
	}
}

func TestShapeCrossingEdges(t *testing.T) {
	// Coverage that falls off the right edge must not leak into the next
	// row, nor off the left edge disappear.
	img := render(t, 20, 20, 1, &scene.Rect{X: 15, Y: 5, W: 20, H: 5, Paint: scene.Paint{Fill: black}},
		&scene.Rect{X: -10, Y: 12, W: 15, H: 3, Paint: scene.Paint{Fill: black}})
	if got := inkBounds(img, 0); got != image.Rect(0, 5, 20, 15) {
		t.Errorf("ink %v", got)
	}
	if alpha(img, 0, 6) != 0 || alpha(img, 19, 6) != 255 || alpha(img, 0, 13) != 255 || alpha(img, 5, 13) != 0 {
		t.Error("edge-crossing fill leaked or was lost")
	}
}

func TestEvenOdd(t *testing.T) {
	sq := func(x0, y0, x1, y1 float64) []pt { return []pt{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}} }
	contours := [][]pt{sq(0, 0, 20, 20), sq(5, 5, 15, 15)} // same direction
	var r rasterizer
	b := image.Rect(0, 0, 20, 20)
	m, _ := r.fill(contours, false, b, nil)
	if m.AlphaAt(10, 10).A != 255 {
		t.Error("nonzero should fill the inner square")
	}
	m, _ = r.fill(contours, true, b, nil)
	if m.AlphaAt(10, 10).A != 0 || m.AlphaAt(2, 2).A != 255 {
		t.Error("even-odd should leave the inner square empty")
	}
	// Half-pixel edges get half coverage in both modes.
	m, _ = r.fill([][]pt{sq(0.5, 0, 10, 10)}, true, b, nil)
	if a := m.AlphaAt(0, 5).A; a < 126 || a > 129 {
		t.Errorf("even-odd half pixel alpha %d", a)
	}
}

func TestTextInkMatchesWidth(t *testing.T) {
	face := text.FaceFor(text.Regular)
	w := face.Width("Hello", 20)
	for _, a := range []scene.Anchor{scene.AnchorStart, scene.AnchorMiddle, scene.AnchorEnd} {
		x := 10.0
		switch a {
		case scene.AnchorMiddle:
			x = 10 + w/2
		case scene.AnchorEnd:
			x = 10 + w
		}
		img := render(t, 100, 40, 2, &scene.Text{X: x, Y: 30, Text: "Hello", Size: 20, Anchor: a, Color: black, Width: w})
		ink := inkBounds(img, 20)
		// Side bearings keep ink a little inside the advance box.
		if l := float64(ink.Min.X) / 2; l < 10 || l > 13 {
			t.Errorf("anchor %d: ink starts at %v, want just right of 10", a, l)
		}
		if r := float64(ink.Max.X) / 2; r > 10+w+0.5 || r < 10+w-3 {
			t.Errorf("anchor %d: ink ends at %v, want near %v", a, r, 10+w)
		}
		capH := face.Ascent(20)
		if top := float64(ink.Min.Y) / 2; top < 30-capH || top > 18 {
			t.Errorf("anchor %d: ink top %v", a, top)
		}
		if bot := float64(ink.Max.Y) / 2; bot < 29.5 || bot > 30.5 {
			t.Errorf("anchor %d: ink bottom %v, want on baseline 30", a, bot)
		}
	}
}

func svgIcon(t *testing.T, name, src string) *icons.Icon {
	t.Helper()
	ic, err := icons.FromSVG(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return ic.Named(name)
}

func TestIconCurrentColorAndMeet(t *testing.T) {
	// A 20×10 viewBox in a 40×40 box scales by 2 and centres vertically.
	ic := svgIcon(t, "bar", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 10"><rect width="20" height="10" fill="currentColor"/></svg>`)
	img := render(t, 40, 40, 1, &scene.Image{X: 0, Y: 0, W: 40, H: 40, Icon: ic, Color: themes.RGB(0, 128, 0)})
	if got := inkBounds(img, 0); got != image.Rect(0, 10, 40, 30) {
		t.Errorf("icon ink %v, want (0,10)-(40,30)", got)
	}
	if got := img.RGBAAt(20, 20); got != (color.RGBA{0, 128, 0, 255}) {
		t.Errorf("currentColor pixel %v", got)
	}
}

func TestIconClipsToBox(t *testing.T) {
	ic := svgIcon(t, "big", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect x="-5" y="-5" width="20" height="20" fill="#000"/></svg>`)
	img := render(t, 40, 40, 1, &scene.Image{X: 10, Y: 10, W: 20, H: 20, Icon: ic})
	if got := inkBounds(img, 0); got != image.Rect(10, 10, 30, 30) {
		t.Errorf("icon ink %v, want clipped to (10,10)-(30,30)", got)
	}
}

func TestIconGradients(t *testing.T) {
	ic := svgIcon(t, "grad", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
<defs><linearGradient id="g"><stop offset="0" stop-color="#ff0000"/><stop offset="1" stop-color="#0000ff"/></linearGradient>
<radialGradient id="r" cx="50" cy="50" r="50" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></radialGradient></defs>
<rect width="100" height="50" fill="url(#g)"/><rect y="50" width="100" height="50" fill="url(#r)"/></svg>`)
	img := render(t, 100, 100, 1, &scene.Image{X: 0, Y: 0, W: 100, H: 100, Icon: ic})
	if got := img.RGBAAt(0, 25); got.R < 250 || got.B > 5 {
		t.Errorf("linear start %v, want red", got)
	}
	if got := img.RGBAAt(99, 25); got.B < 250 || got.R > 5 {
		t.Errorf("linear end %v, want blue", got)
	}
	if got := img.RGBAAt(50, 25); abs(int(got.R)-128) > 3 || abs(int(got.B)-128) > 3 {
		t.Errorf("linear middle %v, want purple", got)
	}
	// Radial: 25px from the centre is half way from white to black.
	if got := img.RGBAAt(75, 50); abs(int(got.R)-128) > 4 {
		t.Errorf("radial half way %v", got)
	}
}

func TestIconSpread(t *testing.T) {
	for _, c := range []struct {
		method string
		want   uint8 // red at x=75, where t = 1.5
	}{{"pad", 0}, {"reflect", 128}, {"repeat", 128}} {
		ic := svgIcon(t, c.method, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 10"><defs><linearGradient id="g" x2="50" gradientUnits="userSpaceOnUse" spreadMethod="`+c.method+`"><stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#000"/></linearGradient></defs><rect width="100" height="10" fill="url(#g)"/></svg>`)
		img := render(t, 100, 10, 1, &scene.Image{X: 0, Y: 0, W: 100, H: 10, Icon: ic})
		if got := img.RGBAAt(75, 5).R; abs(int(got)-int(c.want)) > 4 {
			t.Errorf("%s: red at t=1.5 is %d, want %d", c.method, got, c.want)
		}
	}
}

func TestIconOpacityLayer(t *testing.T) {
	// opacity on an element with fill and stroke composites them together,
	// so the stroke over the fill is not darker than either.
	ic := svgIcon(t, "op", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20"><rect x="5" y="5" width="10" height="10" fill="#000" stroke="#000" stroke-width="4" opacity="0.5"/></svg>`)
	img := render(t, 20, 20, 1, &scene.Image{X: 0, Y: 0, W: 20, H: 20, Icon: ic})
	if a, b := alpha(img, 5, 5), alpha(img, 10, 10); a != 128 || b != 128 {
		t.Errorf("alpha over stroke %d, over fill %d, want 128 both", a, b)
	}
}

func TestIconUnsupportedNamesIcon(t *testing.T) {
	ic := svgIcon(t, "fancy", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><text>hi</text></svg>`)
	_, err := Rasterize(&scene.Scene{Width: 10, Height: 10, Theme: themes.Light, Items: []scene.Item{
		&scene.Image{X: 0, Y: 0, W: 10, H: 10, Icon: ic},
	}}, Options{})
	if err == nil || !strings.HasPrefix(err.Error(), `icon "fancy": `) {
		t.Errorf("err = %v, want one naming the icon", err)
	}
}

func TestPNGIcon(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for i := range src.Pix {
		src.Pix[i] = 255
	}
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	ic, err := icons.FromPNG(&buf)
	if err != nil {
		t.Fatal(err)
	}
	img := render(t, 40, 40, 1, &scene.Image{X: 0, Y: 0, W: 40, H: 40, Icon: ic})
	got := inkBounds(img, 128)
	if got != image.Rect(0, 10, 40, 30) {
		t.Errorf("png icon ink %v, want (0,10)-(40,30)", got)
	}
}

// pngIcon is a small coloured PNG icon for the golden scene.
func pngIcon(t *testing.T) *icons.Icon {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			c := color.NRGBA{uint8(x * 32), uint8(y * 32), 200, 255}
			if (x-4)*(x-4)+(y-4)*(y-4) > 12 {
				c.A = 0
			}
			src.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	ic, err := icons.FromPNG(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return ic
}

func goldenScenes(t *testing.T) map[string]*scene.Scene {
	fg := themes.Of(themes.Foreground)
	border := themes.Of(themes.Border)
	accent := themes.Of(themes.Accent)
	var items []scene.Item
	// Strokes at the widths diagrams use, with every cap and join.
	for i, w := range []float64{1, 1.5, 2, 4} {
		y := 20 + float64(i)*30
		for j, c := range []scene.LineCap{scene.CapButt, scene.CapRound, scene.CapSquare} {
			var p scene.Path
			x := 20 + float64(j)*90
			p.MoveTo(x, y+15)
			p.LineTo(x+20, y)
			p.LineTo(x+40, y+15)
			p.CubicTo(x+50, y-10, x+70, y+25, x+75, y)
			items = append(items, &scene.Shape{Path: p, Paint: scene.Paint{Stroke: fg, StrokeWidth: w, Cap: c, Join: scene.LineJoin(j)}})
		}
	}
	dashed := scene.Paint{Stroke: accent, StrokeWidth: 1.5, Dash: []float64{7.5, 4.5}, Join: scene.JoinRound}
	dotted := scene.Paint{Stroke: themes.Of(themes.Red), StrokeWidth: 1.5, Dash: []float64{0.01, 3.75}, Cap: scene.CapRound, Join: scene.JoinRound}
	var e1, e2 scene.Path
	e1.MoveTo(300, 20)
	e1.LineTo(380, 20)
	e1.LineTo(380, 80)
	e2.MoveTo(300, 40)
	e2.LineTo(360, 40)
	e2.LineTo(360, 100)
	items = append(items, &scene.Shape{Path: e1, Paint: dashed}, &scene.Shape{Path: e2, Paint: dotted})
	// Filled and bordered shapes.
	items = append(items,
		&scene.Rect{X: 20, Y: 150, W: 80, H: 40, R: 8, Paint: scene.Paint{Fill: themes.Of(themes.Surface), Stroke: border, StrokeWidth: 1}},
		&scene.Rect{X: 110, Y: 150, W: 80, H: 40, R: 20, Paint: scene.Paint{Fill: themes.Hex("#fef3c7"), Stroke: themes.Of(themes.Orange), StrokeWidth: 1, Dash: []float64{5, 3}}},
		&scene.Ellipse{CX: 240, CY: 170, RX: 40, RY: 20, Paint: scene.Paint{Fill: themes.RGBA(37, 99, 235, 100), Stroke: accent, StrokeWidth: 2}},
		&scene.Rect{X: 300, Y: 150, W: 80, H: 40, Paint: scene.Paint{Fill: themes.Of(themes.Green), Opacity: 0.4}},
	)
	prims := &scene.Scene{Width: 400, Height: 210, Theme: themes.Light, Background: themes.Of(themes.Background), Items: items}

	txt := &scene.Scene{Width: 300, Height: 120, Theme: themes.Light, Background: themes.Of(themes.Background)}
	for i, s := range []struct {
		text string
		size float64
		w    scene.Weight
		a    scene.Anchor
	}{{"Payments API", 14, scene.SemiBold, scene.AnchorStart}, {"Go · 3 replicas", 12, scene.Regular, scene.AnchorMiddle}, {"HTTPS", 11, scene.Regular, scene.AnchorEnd}, {"Large Heading", 24, scene.SemiBold, scene.AnchorMiddle}} {
		face := text.FaceFor(text.Weight(s.w))
		x := map[scene.Anchor]float64{scene.AnchorStart: 10, scene.AnchorMiddle: 150, scene.AnchorEnd: 290}[s.a]
		txt.Items = append(txt.Items, &scene.Text{X: x, Y: 22 + float64(i)*28, Text: s.text, Size: s.size, Weight: s.w, Anchor: s.a, Color: fg, Width: face.Width(s.text, s.size)})
	}

	grad := svgIcon(t, "gradient", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><defs><linearGradient id="a" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#f97316"/><stop offset="1" stop-color="#7c3aed"/></linearGradient><radialGradient id="b" fx="0.3" fy="0.3"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#0ea5e9" stop-opacity="0.6"/></radialGradient></defs><rect x="2" y="2" width="20" height="20" rx="4" fill="url(#a)"/><circle cx="12" cy="12" r="6" fill="url(#b)" stroke="#fff" stroke-width="1.5"/><path fill-rule="evenodd" d="M4 4h6v6H4zM5.5 5.5h3v3h-3z" fill="#fff"/></svg>`)
	ics := &scene.Scene{Width: 360, Height: 60, Theme: themes.Light, Background: themes.Of(themes.Background)}
	for i, ic := range []*icons.Icon{generic.Server, generic.Database, generic.Cog, generic.Cloud, brands.Go, brands.Kubernetes, brands.Postgres, grad, pngIcon(t)} {
		ics.Items = append(ics.Items, &scene.Image{X: 10 + float64(i)*38, Y: 14, W: 32, H: 32, Icon: ic, Color: themes.Of(themes.Muted)})
	}
	return map[string]*scene.Scene{"primitives": prims, "text": txt, "icons": ics}
}

// TestGolden compares renders with testdata/*.png, allowing small
// per-channel differences on a few pixels (arm64 fuses multiply-adds).
func TestGolden(t *testing.T) {
	for name, sc := range goldenScenes(t) {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Render(&buf, sc, Options{Scale: 2}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", name+".png")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("%v (run go test -update to create it)", err)
			}
			defer f.Close()
			want, err := stdpng.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := stdpng.Decode(bytes.NewReader(buf.Bytes()))
			if got.Bounds() != want.Bounds() {
				t.Fatalf("size %v, golden %v", got.Bounds(), want.Bounds())
			}
			diff, worst := 0, 0
			b := got.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					g := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
					w := color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
					d := max(abs(int(g.R)-int(w.R)), abs(int(g.G)-int(w.G)), abs(int(g.B)-int(w.B)), abs(int(g.A)-int(w.A)))
					if g.A == 0 && w.A == 0 {
						d = 0
					}
					worst = max(worst, d)
					if d > 8 {
						diff++
					}
				}
			}
			if limit := b.Dx() * b.Dy() / 200; diff > limit || worst > 64 {
				// Kept after the test so the render can be inspected.
				out := filepath.Join(os.TempDir(), "diagram-png-"+name+".png")
				_ = os.WriteFile(out, buf.Bytes(), 0o644)
				t.Errorf("%s: %d pixels differ by more than 8 (limit %d), worst %d; compare with %s, then run go test -update",
					path, diff, limit, worst, out)
			}
		})
	}
}

func TestDeterministic(t *testing.T) {
	for name, sc := range goldenScenes(t) {
		var a, b bytes.Buffer
		if err := Render(&a, sc, Options{}); err != nil {
			t.Fatal(err)
		}
		if err := Render(&b, sc, Options{}); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Errorf("%s: two renders differ", name)
		}
	}
}

func BenchmarkGolden(b *testing.B) {
	t := &testing.T{}
	scenes := goldenScenes(t)
	for b.Loop() {
		for _, sc := range scenes {
			if err := Render(&bytes.Buffer{}, sc, Options{Scale: 2}); err != nil {
				b.Fatal(err)
			}
		}
	}
}
