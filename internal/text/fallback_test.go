package text

import (
	"math"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// testdata holds Noto Sans SC Regular and SemiBold (OFL, see
// testdata/OFL-NotoSansSC.txt) subset to 支付服务日本語のテキスト数据→.

func notoData(t *testing.T, weight string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/NotoSansSC-" + weight + "-subset.ttf")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// withFallbacks registers fonts for the test and restores the registry
// afterwards.
func withFallbacks(t *testing.T, fonts ...[]byte) {
	t.Helper()
	load()
	old := registry.Load()
	t.Cleanup(func() { registry.Store(old) })
	for _, f := range fonts {
		if err := Register(f); err != nil {
			t.Fatal(err)
		}
	}
}

// advance returns r's advance in pixels in a raw font.
func advance(t *testing.T, data []byte, r rune, size float64) float64 {
	t.Helper()
	f, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var b sfnt.Buffer
	idx, err := f.GlyphIndex(&b, r)
	if err != nil || idx == 0 {
		t.Fatalf("font lacks %q", r)
	}
	adv, err := f.GlyphAdvance(&b, idx, fixed.I(int(f.UnitsPerEm())), font.HintingNone)
	if err != nil {
		t.Fatal(err)
	}
	return float64(adv) / 64 * size / float64(f.UnitsPerEm())
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestFallbackWidth(t *testing.T) {
	reg := FaceFor(Regular)
	if got, q := reg.Width("支", 14), reg.Width("?", 14); !near(got, q) {
		t.Fatalf("before registering, 支 should measure as '?': %v vs %v", got, q)
	}
	noto := notoData(t, "Regular")
	withFallbacks(t, noto)

	if got, want := reg.Width("支", 14), advance(t, noto, '支', 14); !near(got, want) {
		t.Errorf("Width(支) = %v, want Noto's advance %v", got, want)
	}
	// Mixed text sums each font's advances.
	want := reg.Width("A", 14) + advance(t, noto, '支', 14) + advance(t, noto, '付', 14)
	if got := reg.Width("A支付", 14); !near(got, want) {
		t.Errorf("Width(A支付) = %v, want %v", got, want)
	}
	// Inter wins for runes it has, even when a fallback has them too.
	if fn := reg.FontFor('→'); fn != reg.inter {
		t.Errorf("→ resolved to %s, want Inter", fn.Family)
	}
	if got, want := reg.Width("→", 14), advance(t, interRegular, '→', 14); !near(got, want) {
		t.Errorf("Width(→) = %v, want Inter's %v", got, want)
	}
	// Line metrics stay Inter's.
	if got, want := reg.Ascent(14), reg.inter.ascent*14/reg.inter.upem; got != want {
		t.Errorf("Ascent = %v, want Inter's %v", got, want)
	}
}

func TestFallbackWeights(t *testing.T) {
	t.Run("single weight serves both", func(t *testing.T) {
		noto := notoData(t, "Regular")
		withFallbacks(t, noto)
		for _, w := range []Weight{Regular, SemiBold} {
			fn := FaceFor(w).FontFor('支')
			if fn == nil || fn.Weight != 400 || fn.Family != "diagram-fallback-1" || fn.Name != "Noto Sans SC" {
				t.Errorf("weight %d: got %+v", w, fn)
			}
		}
	})
	t.Run("closest weight", func(t *testing.T) {
		// SemiBold first, to show the match doesn't depend on order.
		withFallbacks(t, notoData(t, "SemiBold"), notoData(t, "Regular"))
		if got := len(registry.Load().families); got != 1 {
			t.Errorf("two weights of one family: %d families", got)
		}
		for w, want := range map[Weight]int{Regular: 400, SemiBold: 600} {
			fn := FaceFor(w).FontFor('支')
			if fn == nil || fn.Weight != want || fn.Family != "diagram-fallback-1" {
				t.Errorf("weight %d: got %+v, want weight %d", w, fn, want)
			}
		}
		if got, want := FaceFor(SemiBold).Width("支", 20), advance(t, notoData(t, "SemiBold"), '支', 20); !near(got, want) {
			t.Errorf("SemiBold width %v, want %v", got, want)
		}
	})
	t.Run("pick", func(t *testing.T) {
		fam := &family{fonts: []*Font{{Weight: 500}, {Weight: 700}, {Weight: 300}}}
		if got := fam.pick(600).Weight; got != 700 {
			t.Errorf("600 between 500 and 700: got %d, want the heavier", got)
		}
		if got := fam.pick(400).Weight; got != 300 {
			t.Errorf("400 between 300 and 500: got %d, want the lighter", got)
		}
	})
}

func TestWrapMixedScripts(t *testing.T) {
	withFallbacks(t, notoData(t, "Regular"))
	f := FaceFor(Regular)
	s := "支付服务 Payments 日本語のテキスト 数据"
	lines := f.Wrap(s, 14, 90)
	if len(lines) < 3 {
		t.Fatalf("want wrapping, got %q", lines)
	}
	for _, l := range lines {
		if w := f.Width(l, 14); w > 90 && strings.Contains(l, " ") {
			t.Errorf("line %q is %v wide", l, w)
		}
	}
	if got := strings.Join(lines, " "); got != s {
		t.Errorf("lines rejoin to %q", got)
	}
}

func TestSegmentsFallback(t *testing.T) {
	noto := notoData(t, "Regular")
	f := FaceFor(Regular)
	_, qAdv, err := f.Segments('?', 20)
	if err != nil {
		t.Fatal(err)
	}
	withFallbacks(t, noto)
	segs, adv, err := f.Segments('支', 20)
	if err != nil || len(segs) == 0 {
		t.Fatalf("Segments(支): %d segs, err %v", len(segs), err)
	}
	if want := advance(t, noto, '支', 20); !near(adv, want) || near(adv, qAdv) {
		t.Errorf("advance %v, want Noto's %v", adv, want)
	}
	// The outline is CJK-sized: about an em wide, not a '?'.
	minX, maxX := math.Inf(1), math.Inf(-1)
	for _, s := range segs {
		for _, p := range s.Args[:1] {
			minX, maxX = math.Min(minX, float64(p.X)/64), math.Max(maxX, float64(p.X)/64)
		}
	}
	if maxX-minX < 12 {
		t.Errorf("outline only %v px wide", maxX-minX)
	}
	// Default-ignorable runes draw nothing.
	if segs, adv, err := f.Segments('️', 20); err != nil || segs != nil || adv != 0 {
		t.Errorf("VS16: %d segs, adv %v, err %v", len(segs), adv, err)
	}
}

func TestMissing(t *testing.T) {
	if got := string(Missing("A 支\n🚀")); got != "支🚀" {
		t.Errorf("without fallback: %q", got)
	}
	withFallbacks(t, notoData(t, "Regular"))
	if got := string(Missing("A 支\t🚀 ✓🚀️‍")); got != "🚀" {
		t.Errorf("Missing = %q, want just the rocket once", got)
	}
	if got := FaceFor(Regular).Width("️‍", 14); got != 0 {
		t.Errorf("ignorable runes measure %v", got)
	}
}

func TestRegisterRejects(t *testing.T) {
	load()
	old := registry.Load()
	sub, err := Subset(notoData(t, "Regular"), []rune("支"))
	if err != nil {
		t.Fatal(err)
	}
	withTable := func(tag string, drop ...string) []byte {
		f, err := parseTables(sub)
		if err != nil {
			t.Fatal(err)
		}
		tables := map[string][]byte{}
		for k, v := range f.tables {
			tables[k] = v
		}
		h := append([]byte(nil), tables["head"]...)
		clear(h[8:12])
		tables["head"] = h
		if tag != "" {
			tables[tag] = make([]byte, 16)
		}
		for _, d := range drop {
			delete(tables, d)
		}
		return writeFont(tables)
	}
	otto := append([]byte("OTTO"), sub[4:]...)
	for _, tt := range []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "too short"},
		{"garbage", []byte("definitely not a font file"), "not a TrueType font"},
		{"cff", otto, "CFF-based OpenType"},
		{"collection", append([]byte("ttcf"), sub[4:]...), "collections"},
		{"woff", append([]byte("wOFF"), sub[4:]...), "WOFF"},
		{"cbdt", withTable("CBDT"), "colour emoji"},
		{"sbix", withTable("sbix"), "colour emoji"},
		{"colr", withTable("COLR"), "colour emoji"},
		{"no glyf", withTable("", "glyf"), `missing "glyf"`},
		{"truncated", sub[:len(sub)-100], "out of bounds"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := Register(tt.data)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if registry.Load() != old {
				t.Error("a rejected font changed the registry")
			}
		})
	}
}

func TestRegisterDeterministic(t *testing.T) {
	noto := notoData(t, "Regular")
	withFallbacks(t)
	// Register copies its input.
	cp := append([]byte(nil), noto...)
	if err := Register(cp); err != nil {
		t.Fatal(err)
	}
	w := FaceFor(Regular).Width("支付", 14)
	clear(cp)
	if got := FaceFor(Regular).Width("支付", 14); got != w {
		t.Errorf("width changed after the caller reused its buffer")
	}
	st := registry.Load()
	if err := Register(noto); err != nil {
		t.Fatal(err)
	}
	if registry.Load() != st {
		t.Error("registering the same bytes again changed the registry")
	}
}

func TestRegisterConcurrent(t *testing.T) {
	withFallbacks(t)
	fonts := [][]byte{notoData(t, "Regular"), notoData(t, "SemiBold")}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			if i%2 == 0 {
				if err := Register(fonts[i/2%2]); err != nil {
					t.Error(err)
				}
				return
			}
			for range 100 {
				FaceFor(Weight(i/2%2)).Width("支付 Payments", 14)
			}
		})
	}
	wg.Wait()
	st := registry.Load()
	if len(st.families) != 1 || len(st.families[0].fonts) != 2 {
		t.Errorf("want one family of two fonts, got %d families", len(st.families))
	}
}
