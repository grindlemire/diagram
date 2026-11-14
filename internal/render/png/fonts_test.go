package png

import (
	"image"
	"os"
	"testing"

	"github.com/grindlemire/diagram/internal/scene"
	"github.com/grindlemire/diagram/internal/text"
)

// TestTextFallbackFont draws CJK with a registered fallback font: the ink
// is the fallback's glyph, not Inter's '?', and spans the measured width.
func TestTextFallbackFont(t *testing.T) {
	draw := func(s string) *image.RGBA {
		w := text.FaceFor(text.Regular).Width(s, 20)
		return render(t, 80, 40, 2, &scene.Text{X: 10, Y: 30, Text: s, Size: 20, Color: black, Width: w})
	}
	question := draw("?")
	before := draw("支")
	if !sameImage(before, question) {
		t.Fatal("without a fallback, 支 should draw as '?'")
	}

	data, err := os.ReadFile("../../text/testdata/NotoSansSC-Regular-subset.ttf")
	if err != nil {
		t.Fatal(err)
	}
	if err := text.Register(data); err != nil {
		t.Fatal(err)
	}
	after := draw("支")
	if sameImage(after, question) {
		t.Fatal("with a fallback, 支 still draws as '?'")
	}
	ink := inkBounds(after, 20)
	w := text.FaceFor(text.Regular).Width("支", 20)
	if got := float64(ink.Dx()) / 2; got < 0.8*w || got > w {
		t.Errorf("ink %v px wide, want most of the %v px advance", got, w)
	}
	if got := float64(ink.Dy()) / 2; got < 14 {
		t.Errorf("ink %v px tall, want a full CJK glyph", got)
	}
	if !sameImage(draw("支"), after) {
		t.Error("drawing is not deterministic")
	}
}

func sameImage(a, b *image.RGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}
