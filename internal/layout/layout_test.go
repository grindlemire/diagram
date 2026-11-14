package layout

import (
	"image/color"
	"math"
	"testing"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/themes"
)

func TestJustifyHelper(t *testing.T) {
	for _, c := range []struct {
		j         model.Justify
		n         int
		lead, gap float64
	}{
		{model.JustifyStart, 3, 0, 10},
		{model.JustifyCenter, 3, 30, 10},
		{model.JustifyEnd, 3, 60, 10},
		{model.JustifySpaceBetween, 3, 0, 40},
		{model.JustifySpaceBetween, 1, 0, 10},
	} {
		lead, gap := justify(c.j, 60, 10, c.n)
		if lead != c.lead || gap != c.gap {
			t.Errorf("justify(%d, n=%d) = %v, %v; want %v, %v", c.j, c.n, lead, gap, c.lead, c.gap)
		}
	}
}

func TestContrast(t *testing.T) {
	black, white := color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 255}
	if c := contrast(black, white); math.Abs(c-21) > 0.01 {
		t.Errorf("black on white = %v, want 21", c)
	}
	if c := contrast(white, white); c != 1 {
		t.Errorf("white on white = %v, want 1", c)
	}
	for _, bg := range []color.RGBA{{0xdc, 0xfc, 0xe7, 255}, {0x1e, 0x29, 0x3b, 255}, {0xff, 0x66, 0x00, 255}, {0x80, 0x80, 0x80, 255}} {
		text := darkText
		if contrast(lightText, bg) > contrast(darkText, bg) {
			text = lightText
		}
		m := muted(text, bg)
		want := minTextContrast
		if contrast(text, bg) < want {
			want = minIconContrast
		}
		if c := contrast(m, bg); c < want || m == text && contrast(text, bg) > 7 {
			t.Errorf("muted %v on %v: contrast %v (text %v)", m, bg, c, contrast(text, bg))
		}
	}
}

func TestTranslucentFillKeepsReadableTheme(t *testing.T) {
	d := model.New("t")
	n := &model.Element{Kind: model.KindNode, Label: "Faint", Parent: d.Root, Diagram: d,
		Style: model.Style{Fill: themes.RGBA(37, 99, 235, 40)}}
	d.Root.Children = append(d.Root.Children, n)
	res, err := Layout(d, themes.Light)
	if err != nil {
		t.Fatal(err)
	}
	if tk, ok := res.ByEl[n].Lines[0].Color.Token(); !ok || tk != themes.Foreground {
		t.Errorf("faint literal fill changed readable theme text to %v", res.ByEl[n].Lines[0].Color)
	}
}
