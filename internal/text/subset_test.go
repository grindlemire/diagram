package text

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

func diagramRunes() []rune {
	var rs []rune
	for r := rune(0x20); r < 0x7F; r++ {
		rs = append(rs, r)
	}
	return append(rs, '·', '→', 'é')
}

func subsetFonts() map[string][]byte {
	return map[string][]byte{"Regular": interRegular, "SemiBold": interSemiBold}
}

func TestSubsetGlyphs(t *testing.T) {
	runes := diagramRunes()
	for name, data := range subsetFonts() {
		t.Run(name, func(t *testing.T) {
			sub, err := Subset(data, runes)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %d bytes (original %d)", name, len(sub), len(data))
			if len(sub) < 5_000 || len(sub) > 25_000 {
				t.Errorf("subset size %d outside 5–25 KB", len(sub))
			}
			orig, err := sfnt.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			got, err := sfnt.Parse(sub)
			if err != nil {
				t.Fatalf("parse subset: %v", err)
			}
			var ob, gb sfnt.Buffer
			ppem := fixed.I(int(orig.UnitsPerEm()))
			for _, r := range runes {
				oi, err := orig.GlyphIndex(&ob, r)
				if err != nil || oi == 0 {
					t.Fatalf("original lacks %q", r)
				}
				gi, err := got.GlyphIndex(&gb, r)
				if err != nil || gi == 0 {
					t.Errorf("%q: subset glyph index %d, err %v", r, gi, err)
					continue
				}
				oa, _ := orig.GlyphAdvance(&ob, oi, ppem, font.HintingNone)
				ga, err := got.GlyphAdvance(&gb, gi, ppem, font.HintingNone)
				if err != nil || ga != oa {
					t.Errorf("%q: advance %v, want %v (err %v)", r, ga, oa, err)
				}
				os, err := orig.LoadGlyph(&ob, oi, ppem, nil)
				if err != nil {
					t.Fatal(err)
				}
				gs, err := got.LoadGlyph(&gb, gi, ppem, nil)
				if err != nil {
					t.Errorf("%q: LoadGlyph: %v", r, err)
					continue
				}
				if !slices.Equal(os, gs) {
					t.Errorf("%q: outline differs (%d vs %d segments)", r, len(gs), len(os))
				}
			}
			// Font-wide metrics are unchanged.
			om, _ := orig.Metrics(&ob, ppem, font.HintingNone)
			gm, err := got.Metrics(&gb, ppem, font.HintingNone)
			if err != nil || om != gm {
				t.Errorf("metrics %+v, want %+v (err %v)", gm, om, err)
			}
			checkStructure(t, sub)
		})
	}
}

// checkStructure verifies the directory, alignment and checksums.
func checkStructure(t *testing.T, b []byte) {
	t.Helper()
	be := binary.BigEndian
	n := int(be.Uint16(b[4:]))
	sr, es := 1, 0
	for sr*2 <= n {
		sr *= 2
		es++
	}
	if be.Uint16(b[6:]) != uint16(16*sr) || be.Uint16(b[8:]) != uint16(es) || be.Uint16(b[10:]) != uint16(16*n-16*sr) {
		t.Errorf("bad searchRange/entrySelector/rangeShift")
	}
	var prev string
	for i := range n {
		rec := b[12+16*i:]
		tag := string(rec[:4])
		if tag <= prev {
			t.Errorf("tags not sorted: %q after %q", tag, prev)
		}
		prev = tag
		off, length := be.Uint32(rec[8:]), be.Uint32(rec[12:])
		if off%4 != 0 {
			t.Errorf("%q offset %d not aligned", tag, off)
		}
		data := slices.Clone(b[off : off+length])
		if tag == "head" {
			clear(data[8:12])
		}
		if got := checksum(data); got != be.Uint32(rec[4:]) {
			t.Errorf("%q checksum %#x, directory says %#x", tag, got, be.Uint32(rec[4:]))
		}
		switch tag {
		case "GSUB", "GPOS", "GDEF", "kern", "DSIG", "STAT":
			t.Errorf("table %q should be dropped", tag)
		}
	}
	if len(b)%4 != 0 {
		t.Errorf("length %d not padded", len(b))
	}
	if checksum(b) != 0xB1B0AFBA {
		t.Errorf("whole-font checksum %#x, want 0xB1B0AFBA", checksum(b))
	}
}

func TestSubsetDeterministic(t *testing.T) {
	runes := diagramRunes()
	shuffled := slices.Clone(runes)
	slices.Reverse(shuffled)
	shuffled = append(shuffled, 'a', 'a', '→')
	for name, data := range subsetFonts() {
		a, err := Subset(data, runes)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Subset(data, shuffled)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s: output depends on rune order or duplicates", name)
		}
	}
}

func TestSubsetEmptyAndMissing(t *testing.T) {
	for _, runes := range [][]rune{nil, {'\U0001F600', 0x10FFFF, -1, 0x4E00}, {'A', '\U0001F600'}} {
		sub, err := Subset(interRegular, runes)
		if err != nil {
			t.Fatalf("%q: %v", runes, err)
		}
		f, err := sfnt.Parse(sub)
		if err != nil {
			t.Fatalf("%q: parse: %v", runes, err)
		}
		checkStructure(t, sub)
		var buf sfnt.Buffer
		for _, r := range runes {
			gi, err := f.GlyphIndex(&buf, r)
			if r == 'A' {
				if gi == 0 || err != nil {
					t.Errorf("A missing from subset")
				}
			} else if gi != 0 {
				t.Errorf("%q: got glyph %d, want .notdef", r, gi)
			}
		}
		if _, err := f.LoadGlyph(&buf, 0, fixed.I(16), nil); err != nil {
			t.Errorf(".notdef: %v", err)
		}
		t.Logf("%q: %d glyphs, %d bytes", runes, f.NumGlyphs(), len(sub))
	}
}

// TestSubsetComposite checks that a composite glyph's components are kept.
func TestSubsetComposite(t *testing.T) {
	orig, _ := sfnt.Parse(interRegular)
	var buf sfnt.Buffer
	found := false
	for r := rune(0xC0); r < 0x180; r++ {
		sub, err := Subset(interRegular, []rune{r})
		if err != nil {
			t.Fatal(err)
		}
		f, err := sfnt.Parse(sub)
		if err != nil {
			t.Fatal(err)
		}
		if f.NumGlyphs() > 2 {
			found = true
		}
		oi, _ := orig.GlyphIndex(&buf, r)
		if oi == 0 {
			continue
		}
		os, _ := orig.LoadGlyph(&buf, oi, fixed.I(64), nil)
		os = slices.Clone(os)
		gi, _ := f.GlyphIndex(&buf, r)
		gs, err := f.LoadGlyph(&buf, gi, fixed.I(64), nil)
		if err != nil || !slices.Equal(os, gs) {
			t.Errorf("%q: outline mismatch (err %v)", r, err)
		}
	}
	if !found {
		t.Errorf("no composite glyphs found in Latin-1/Latin Extended-A; test exercises nothing")
	}
}

func TestBuildCmapFormat12(t *testing.T) {
	mapped := []runeGlyph{{'A', 10}, {'B', 11}, {'C', 13}, {'→', 20}, {0x1F600, 30}, {0x1F601, 31}}
	newID := map[int]uint16{10: 1, 11: 2, 13: 3, 20: 4, 30: 5, 31: 6}
	cm := buildCmap(mapped, newID)
	if got := binary.BigEndian.Uint16(cm[2:]); got != 4 {
		t.Fatalf("%d encoding records, want 4", got)
	}
	m, err := parseCmap(cm)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != len(mapped) {
		t.Errorf("got %d mappings, want %d", len(m), len(mapped))
	}
	for _, rg := range mapped {
		if m[rg.r] != newID[int(rg.g)] {
			t.Errorf("%q -> %d, want %d", rg.r, m[rg.r], newID[int(rg.g)])
		}
	}
	// Without supplementary runes there is no format 12 subtable.
	if got := binary.BigEndian.Uint16(buildCmap(mapped[:4], newID)[2:]); got != 2 {
		t.Errorf("%d encoding records, want 2", got)
	}
}
