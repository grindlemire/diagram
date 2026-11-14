package text

import "testing"

func TestWidthAndWrap(t *testing.T) {
	f := FaceFor(Regular)
	if w := f.Width("WWW", 14); w <= f.Width("iii", 14)*2 {
		t.Errorf("WWW (%v) should be much wider than iii (%v)", w, f.Width("iii", 14))
	}
	if w := f.Width("Hello", 28); w < 1.99*f.Width("Hello", 14) || w > 2.01*f.Width("Hello", 14) {
		t.Errorf("width should scale linearly with size")
	}
	lines := f.Wrap("Payment processing service", 14, 120)
	if len(lines) < 2 {
		t.Errorf("want wrapping, got %q", lines)
	}
	for _, l := range lines {
		if f.Width(l, 14) > 120 {
			t.Errorf("line %q is %v wide", l, f.Width(l, 14))
		}
	}
	if got := f.Wrap("a\nb", 14, 0); len(got) != 2 {
		t.Errorf("explicit newline: got %q", got)
	}
	if f.Ascent(14) <= 0 || f.Descent(14) <= 0 {
		t.Errorf("metrics: ascent %v descent %v", f.Ascent(14), f.Descent(14))
	}
	segs, adv, err := f.Segments('A', 14)
	if err != nil || len(segs) == 0 || adv <= 0 {
		t.Errorf("Segments: %d segs, adv %v, err %v", len(segs), adv, err)
	}
}
