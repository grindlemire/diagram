package diagram

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/themes"
)

func renderEmbed(t *testing.T, d *Diagram, opts ...EmbedOption) string {
	t.Helper()
	var b bytes.Buffer
	if err := d.RenderEmbed(&b, opts...); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestEmbedGolden(t *testing.T) {
	checkGolden(t, filepath.Join("testdata", "payments-embed.html"), []byte(renderEmbed(t, cases[0].build())))
}

func TestEmbedDeterministic(t *testing.T) {
	if renderEmbed(t, cases[0].build()) != renderEmbed(t, cases[0].build()) {
		t.Error("two renders differ")
	}
}

var templateRe = regexp.MustCompile(`(?s)<template data-diagram>.*?</template></diagram-view>`)

// lightDOM is what a fragment puts in the page itself: everything but its
// templates' content.
func lightDOM(s string) string { return templateRe.ReplaceAllString(s, "</diagram-view>") }

// TestEmbedTwoOnOnePage checks that fragments of two diagrams can share a
// page: they put no IDs in the page, each keeps its view in its own
// template, they size themselves by different rules, and they register
// fonts under different names.
func TestEmbedTwoOnOnePage(t *testing.T) {
	a, b := renderEmbed(t, cases[0].build()), renderEmbed(t, linked())
	page := a + b
	light := lightDOM(page)
	if strings.Contains(light, ` id="`) {
		t.Errorf("fragments put IDs in the page:\n%s", light)
	}
	if n := strings.Count(page, "<template data-diagram>"); n != 2 {
		t.Errorf("%d templates, want 2", n)
	}
	keyRe := regexp.MustCompile(`<diagram-view data-dg="([0-9a-f]+)" hash="([^"]+)"`)
	ka, kb := keyRe.FindStringSubmatch(a), keyRe.FindStringSubmatch(b)
	if ka == nil || kb == nil || ka[1] == kb[1] || ka[2] == kb[2] {
		t.Errorf("fragments not told apart: %v, %v", ka, kb)
	}
	if !strings.Contains(a, `diagram-view[data-dg="`+ka[1]+`"]{display:block;aspect-ratio:`) {
		t.Error("fragment not sized by its aspect ratio before the script runs")
	}
	famRe := regexp.MustCompile(`data-family="(dg-[0-9a-f]{12})"`)
	fa, fb := famRe.FindAllStringSubmatch(a, -1), famRe.FindAllStringSubmatch(b, -1)
	if len(fa) == 0 || len(fb) == 0 || fa[0][1] == fb[0][1] {
		t.Errorf("font families not unique per subset: %v, %v", fa, fb)
	}
	// Shadow roots ignore @font-face, so fonts are registered by the
	// script, and the SVG refers to them by their hashed names.
	if strings.Contains(a, "@font-face") {
		t.Error("fragment declares fonts with @font-face")
	}
	if !strings.Contains(a, `font-family="`+fa[0][1]+`, `) {
		t.Error("SVG doesn't use the registered family")
	}
	// Both include the script, which defines the element only once.
	if n := strings.Count(page, Script()); n != 2 {
		t.Errorf("script included %d times, want 2", n)
	}
	s := Script()
	if !strings.HasPrefix(s, "(() => {\nif (!window.customElements || customElements.get('diagram-view')) return;\n") ||
		strings.Count(s, "customElements.define(") != 1 {
		t.Error("script doesn't define the element exactly once per page")
	}
}

func TestEmbedOptions(t *testing.T) {
	d := cases[0].build()
	plain := renderEmbed(t, d)
	if !strings.Contains(plain, `<div id="dg-bar"`) || !strings.Contains(plain, `hash="payments"`) || strings.Contains(plain, ` theme="`) {
		t.Error("default embed: want a toolbar, deep links named after the title and no forced theme")
	}

	s := renderEmbed(t, d, NoScript, Toolbar(false), Height("60vh"), EmbedTheme("dark"), EmbedName("pay"), Margin(0))
	if strings.Contains(s, "<script>") || strings.Contains(s, "customElements") {
		t.Error("NoScript fragment includes the script")
	}
	if strings.Contains(s, `id="dg-bar"`) {
		t.Error("Toolbar(false) fragment has a toolbar")
	}
	if !strings.Contains(plain, `id="dg-theme"`) || !strings.Contains(plain, "<dt>theme button</dt>") {
		t.Error("default embed of a light theme: want the theme switch on the toolbar and in the help")
	}
	if strings.Contains(s, `id="dg-theme"`) || strings.Contains(s, "<dt>theme button</dt>") {
		t.Error("Toolbar(false) fragment has a theme switch")
	}
	if s := renderEmbed(t, d, WithTheme(themes.Dark)); strings.Contains(s, `id="dg-theme"`) {
		t.Error("embed of a theme with one palette has a theme switch")
	}
	for _, want := range []string{`{display:block;height:60vh}`, ` hash="pay"`, ` theme="dark"`} {
		if !strings.Contains(s, want) {
			t.Errorf("fragment lacks %q", want)
		}
	}
	if s := renderEmbed(t, d, EmbedName("")); strings.Contains(s, " hash=") || strings.Contains(s, "Copy a link") {
		t.Error(`EmbedName("") leaves deep links on`)
	}

	// Nonce: every style and script the browser would act on carries it.
	s = renderEmbed(t, d, Nonce("r4nd0m+/="))
	for _, tag := range regexp.MustCompile(`<(style|script)[^>]*>`).FindAllString(s, -1) {
		if strings.Contains(tag, `type="font/ttf;base64"`) {
			continue // inert data
		}
		if !strings.Contains(tag, ` nonce="r4nd0m+/="`) {
			t.Errorf("%s lacks the nonce", tag)
		}
	}

	for _, c := range []struct {
		opt  EmbedOption
		want string
	}{
		{Height("1px;}body{display:none"), "Height("},
		{Height(""), "Height("},
		{Nonce(`a"b`), "Nonce("},
		{EmbedTheme("blue"), "EmbedTheme("},
	} {
		var b bytes.Buffer
		err := d.RenderEmbed(&b, c.opt)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v", c.want, err)
		}
		if b.Len() != 0 {
			t.Errorf("%s: wrote output despite the error", c.want)
		}
	}
}

func TestEmbedInTemplates(t *testing.T) {
	frag, err := linked().HTMLFragment(NoScript)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := template.Must(template.New("p").Parse(`<main>{{.}}</main>`))
	var b bytes.Buffer
	if err := tmpl.Execute(&b, frag); err != nil {
		t.Fatal(err)
	}
	if b.String() != "<main>"+string(frag)+"</main>" {
		t.Error("html/template escaped the fragment")
	}

	// The component renders the same fragment, and reports errors and
	// cancellation without writing.
	var c bytes.Buffer
	if err := Embed(linked(), NoScript).Render(context.Background(), &c); err != nil || c.String() != string(frag) {
		t.Errorf("Embed(...).Render: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Reset()
	if err := Embed(linked()).Render(ctx, &c); err == nil || c.Len() != 0 {
		t.Errorf("Render with a cancelled context: %v, wrote %d bytes", err, c.Len())
	}
	bad := New("bad")
	bad.Node("A", Link("javascript:alert(1)"))
	if err := Embed(bad).Render(context.Background(), &c); err == nil || c.Len() != 0 {
		t.Errorf("Render of an invalid diagram: %v, wrote %d bytes", err, c.Len())
	}
}

func TestScriptHandler(t *testing.T) {
	h := ScriptHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/diagram.js", nil))
	res := rec.Result()
	if res.StatusCode != 200 || rec.Body.String() != Script() {
		t.Fatalf("status %d, body matches: %v", res.StatusCode, rec.Body.String() == Script())
	}
	etag := res.Header.Get("ETag")
	if ct := res.Header.Get("Content-Type"); ct != "text/javascript; charset=utf-8" || etag == "" ||
		!strings.Contains(res.Header.Get("Cache-Control"), "max-age=") {
		t.Errorf("headers: %v", res.Header)
	}
	req := httptest.NewRequest("GET", "/diagram.js", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("revalidation: status %d, want 304", rec.Code)
	}
}
