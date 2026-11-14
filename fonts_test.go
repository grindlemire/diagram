package diagram_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/grindlemire/diagram"
	"golang.org/x/image/font/sfnt"
)

// registerNoto registers the Noto Sans SC subsets vendored for the
// examples (支付服务日本語のテキスト数据→). Registration is global and
// idempotent.
func registerNoto(t *testing.T) {
	t.Helper()
	for _, w := range []string{"Regular", "SemiBold"} {
		data, err := os.ReadFile("examples/fonts/NotoSansSC-" + w + "-subset.ttf")
		if err != nil {
			t.Fatal(err)
		}
		if err := diagram.RegisterFont(data); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegisterFontRejects(t *testing.T) {
	err := diagram.RegisterFont([]byte("not a font, just some bytes"))
	if err == nil || err.Error() != "diagram: RegisterFont: not a TrueType font" {
		t.Errorf("garbage: %v", err)
	}
	otf := append([]byte("OTTO"), make([]byte, 64)...)
	if err := diagram.RegisterFont(otf); err == nil || !strings.Contains(err.Error(), "CFF-based OpenType (.otf) fonts are not supported") {
		t.Errorf("CFF: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Error("MustRegisterFont didn't panic")
		}
	}()
	diagram.MustRegisterFont(nil)
}

var (
	fontFaceRe   = regexp.MustCompile(`@font-face\{font-family:([^;]+);font-weight:(\d+);src:url\(data:font/ttf;base64,([^)]+)\)`)
	fontFamilyRe = regexp.MustCompile(`<svg [^>]*font-family="([^"]*)"`)
)

func TestFallbackFontSVG(t *testing.T) {
	registerNoto(t)
	d := diagram.New("CJK")
	a := d.Node("支付 API", diagram.Desc("日本語 docs")) // SemiBold label, Regular description
	b := d.Node("Ledger")
	d.Connect(a, b, diagram.Label("数据"))
	var buf bytes.Buffer
	if err := d.RenderSVG(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if m := fontFamilyRe.FindStringSubmatch(out); m == nil || !strings.HasPrefix(m[1], "Inter, diagram-fallback-1, ui-sans-serif") {
		t.Errorf("font-family stack: %q", m)
	}

	type face struct {
		family, weight string
		font           *sfnt.Font
	}
	var faces []face
	for _, m := range fontFaceRe.FindAllStringSubmatch(out, -1) {
		data, err := base64.StdEncoding.DecodeString(m[3])
		if err != nil {
			t.Fatal(err)
		}
		f, err := sfnt.Parse(data)
		if err != nil {
			t.Fatalf("%s %s: %v", m[1], m[2], err)
		}
		faces = append(faces, face{m[1], m[2], f})
	}
	var got []string
	for _, f := range faces {
		got = append(got, f.family+"/"+f.weight)
	}
	if want := "Inter/400 diagram-fallback-1/400 Inter/600 diagram-fallback-1/600"; strings.Join(got, " ") != want {
		t.Fatalf("@font-face rules %q, want %q", got, want)
	}
	has := func(f *sfnt.Font, r rune) bool {
		var b sfnt.Buffer
		i, err := f.GlyphIndex(&b, r)
		return err == nil && i != 0
	}
	// Each subset holds exactly the runes it draws at that weight.
	for _, c := range []struct {
		face     int
		has, not string
	}{
		{0, "docs", "日本語数据支"},    // Inter 400: description and edge label
		{1, "日本語数据", "支付docs"},   // Noto Regular
		{2, "APILedger", "支付日数"}, // Inter 600: labels
		{3, "支付", "日本語APIL"},     // Noto SemiBold
	} {
		for _, r := range c.has {
			if !has(faces[c.face].font, r) {
				t.Errorf("%s lacks %q", got[c.face], r)
			}
		}
		for _, r := range c.not {
			if has(faces[c.face].font, r) {
				t.Errorf("%s has %q, which another font draws", got[c.face], r)
			}
		}
	}

	buf.Reset()
	if err := d.RenderSVG(&buf, diagram.FontByName); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "@font-face") {
		t.Error("FontByName embedded a font")
	}
	if m := fontFamilyRe.FindStringSubmatch(buf.String()); m == nil || !strings.HasPrefix(m[1], "Inter, 'Noto Sans SC', ui-sans-serif") {
		t.Errorf("FontByName stack: %q", m)
	}

	// Text Inter covers lists no fallback.
	buf.Reset()
	latin := diagram.New("Latin")
	latin.Node("Ledger")
	if err := latin.RenderSVG(&buf); err != nil {
		t.Fatal(err)
	}
	if m := fontFamilyRe.FindStringSubmatch(buf.String()); m == nil || strings.Contains(m[1], "fallback") {
		t.Errorf("Latin-only stack: %q", m)
	}
}

func TestFallbackFontDeterministic(t *testing.T) {
	registerNoto(t)
	build := func() *diagram.Diagram {
		d := diagram.New("CJK")
		col := d.Column("支付服务")
		a := col.Node("日本語のテキスト")
		b := col.Node("Payments → 数据")
		d.Connect(a, b, diagram.Label("テキスト"))
		return d
	}
	for _, ext := range []string{"svg", "png"} {
		var x, y bytes.Buffer
		r := func(d *diagram.Diagram, b *bytes.Buffer) error {
			if ext == "svg" {
				return d.RenderSVG(b)
			}
			return d.RenderPNG(b)
		}
		if err := r(build(), &x); err != nil {
			t.Fatal(err)
		}
		if err := r(build(), &y); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(x.Bytes(), y.Bytes()) {
			t.Errorf("%s output differs between renders", ext)
		}
	}
}

func TestMissingGlyphError(t *testing.T) {
	registerNoto(t)
	d := diagram.New("Emoji")
	_, _, line, _ := runtime.Caller(0)
	a := d.Node("Launch 🚀", diagram.Desc("party 🎉🚀")) // line+1
	b := d.Node("支付")
	d.Connect(a, b, diagram.Label("ship it 🚀")) // line+3
	d.Node("ok\ttabs\nand newlines")

	want := []string{
		fmt.Sprintf(`fonts_test.go:%d: node "Launch 🚀": no font has a glyph for '🚀' (U+1F680), '🎉' (U+1F389); register one with diagram.RegisterFont`, line+1),
		fmt.Sprintf(`fonts_test.go:%d: edge Launch 🚀 → 支付: no font has a glyph for '🚀' (U+1F680); register one with diagram.RegisterFont`, line+3),
	}
	check := func(what string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: no error", what)
		}
		if got := strings.Split(err.Error(), "\n"); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s:\n got %q\nwant %q", what, got, want)
		}
	}
	check("Validate", d.Validate())
	check("RenderSVG", d.RenderSVG(&bytes.Buffer{}))
	check("RenderPNG", d.RenderPNG(&bytes.Buffer{}))
}
