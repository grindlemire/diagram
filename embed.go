package diagram

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/paint"
	"github.com/grindlemire/diagram/internal/render/html"
)

// An EmbedOption configures an embedded diagram: RenderEmbed, Embed and
// HTMLFragment. Margin, WithTheme, Transparent, EmbedFont and FontByName are
// EmbedOptions as well as options for other formats; NoScript, Nonce,
// Height, Toolbar, EmbedTheme and EmbedName apply only to embeds, so passing
// them to RenderHTML doesn't compile.
type EmbedOption interface{ applyEmbed(*embedConfig) }

type embedConfig struct {
	renderConfig
	noScript bool
	nonce    string
	height   string
	toolbar  bool
	mode     string
	name     *string
}

// embedOpt is an option only embeds have.
type embedOpt struct {
	name string
	f    func(*embedConfig)
}

func (o embedOpt) applyEmbed(c *embedConfig) { o.f(c) }

// Options shared with other formats apply to an embed's render settings.
func (f anyOpt) applyEmbed(c *embedConfig)    { f(&c.renderConfig) }
func (f fontOpt) applyEmbed(c *embedConfig)   { f(&c.renderConfig) }
func (f rasterOpt) applyEmbed(c *embedConfig) { f(&c.renderConfig) }

// Transparent leaves an HTML page's or embed's background to the page.
func (f rasterOpt) applyHTML(c *renderConfig) { f(c) }

// NoScript leaves the script that defines <diagram-view> out of the
// fragment, for pages that load it once themselves, from ScriptHandler or
// Script. Without the script the element stays empty.
var NoScript EmbedOption = embedOpt{"NoScript", func(c *embedConfig) { c.noScript = true }}

var nonceRe = regexp.MustCompile(`^[A-Za-z0-9+/_=-]+$`)

// Nonce adds a nonce attribute to the fragment's <style> and <script>
// elements, for pages whose Content-Security-Policy allows inline styles
// and scripts by nonce.
func Nonce(n string) EmbedOption {
	return embedOpt{"Nonce", func(c *embedConfig) {
		if !nonceRe.MatchString(n) {
			c.errs = append(c.errs, fmt.Errorf("Nonce(%q): want a base64 value", n))
			return
		}
		c.nonce = n
	}}
}

var heightRe = regexp.MustCompile(`^[A-Za-z0-9 .%()+*/-]+$`)

// Height fixes the embed's height to a CSS length, such as "480px",
// "60vh" or "calc(100vh - 4rem)"; the diagram is fitted inside it, and
// pan and zoom stay within it. By default the embed is as wide as its
// container and as tall as the diagram's aspect ratio makes it.
func Height(css string) EmbedOption {
	return embedOpt{"Height", func(c *embedConfig) {
		if !heightRe.MatchString(strings.TrimSpace(css)) {
			c.errs = append(c.errs, fmt.Errorf("Height(%q): want a CSS length", css))
			return
		}
		c.height = strings.TrimSpace(css)
	}}
}

// Toolbar shows or hides the embed's compact toolbar (search, zoom, fit,
// the light/dark toggle and shortcut help), which by default appears while the pointer or
// keyboard focus is in the embed. Keyboard shortcuts work either way.
func Toolbar(show bool) EmbedOption {
	return embedOpt{"Toolbar", func(c *embedConfig) { c.toolbar = show }}
}

// EmbedTheme starts the embed in a palette: "light" or "dark". By default,
// or with "system", it follows the viewer's prefers-color-scheme until they
// toggle it. Only a
// theme with a dark palette (themes.Auto) or themes.Light, which is
// paired with themes.Dark, switches. Viewers switch it with the toolbar's
// theme toggle, and the page can set the element's theme attribute, to
// follow its own dark mode, say.
func EmbedTheme(mode string) EmbedOption {
	return embedOpt{"EmbedTheme", func(c *embedConfig) {
		switch mode {
		case "light", "dark":
			c.mode = mode
		case "system", "":
			c.mode = ""
		default:
			c.errs = append(c.errs, fmt.Errorf("EmbedTheme(%q): want \"light\", \"dark\" or \"system\"", mode))
		}
	}}
}

// EmbedName names the embed in deep links: focusing an element sets the
// page's URL fragment to #<name>:<element-id>, and tracing a path to
// #<name>:from=<id>&to=<id>, so links to several embeds on one page don't
// collide. The default is the diagram's title as a slug ("diagram" if it
// has none); give embeds of the same diagram different names. An empty
// name turns deep links off.
func EmbedName(name string) EmbedOption {
	return embedOpt{"EmbedName", func(c *embedConfig) { c.name = &name }}
}

// RenderEmbed writes the diagram as an HTML fragment to place in another
// page: a <diagram-view> element that shows the interactive diagram, with
// the same pan and zoom, highlighting, tooltips, links, path tracing,
// search and deep links as RenderHTML's page, in a shadow root so the
// page's styles and IDs and the diagram's don't meet. Any number of
// fragments can share a page, and fragments added later (by htmx, say)
// come alive as they are inserted once the script has run.
//
// The fragment includes the script unless NoScript is given; it defines
// the element once per page. It makes no network requests. The embed is
// as wide as its container, as tall as the diagram's aspect ratio makes
// it (or Height), and has no layout shift. A plain mouse wheel scrolls the
// page; Ctrl or ⌘ with the wheel, or a pinch, zooms. Pages can set
// --diagram-background and --diagram-font on the element to blend it in.
func (d *Diagram) RenderEmbed(w io.Writer, opts ...EmbedOption) error {
	cfg := embedConfig{renderConfig: defaultConfig(), toolbar: true}
	for _, o := range opts {
		if o != nil {
			o.applyEmbed(&cfg)
		}
	}
	res, err := d.lay(cfg.renderConfig)
	if err != nil {
		return err
	}
	name := model.Slug(d.m.Title)
	if name == "" {
		name = "diagram"
	}
	if cfg.name != nil {
		name = *cfg.name
	}
	sc := paint.Paint(res, paint.Options{Margin: cfg.margin, Transparent: true, Interactive: true})
	var b bytes.Buffer
	err = html.RenderEmbed(&b, sc, html.EmbedOptions{
		Title: d.m.Title, EmbedFont: cfg.embedFont, Name: name, Height: cfg.height, Toolbar: cfg.toolbar,
		Theme: cfg.mode, Transparent: cfg.transparent, Nonce: cfg.nonce, NoScript: cfg.noScript,
	})
	if err != nil {
		return err
	}
	_, err = w.Write(b.Bytes())
	return err
}

// HTMLFragment returns RenderEmbed's fragment for html/template, which
// inserts it as is.
func (d *Diagram) HTMLFragment(opts ...EmbedOption) (template.HTML, error) {
	var b strings.Builder
	if err := d.RenderEmbed(&b, opts...); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

// EmbedComponent renders an embedded diagram when asked. Its Render method
// matches templ.Component, so templ templates can use it directly:
//
//	@diagram.Embed(d)
type EmbedComponent struct {
	d    *Diagram
	opts []EmbedOption
}

// Embed returns a component that renders d as RenderEmbed does.
func Embed(d *Diagram, opts ...EmbedOption) EmbedComponent {
	return EmbedComponent{d, opts}
}

// Render writes the fragment to w, or returns the diagram's errors
// without writing anything.
func (c EmbedComponent) Render(ctx context.Context, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.d.RenderEmbed(w, c.opts...)
}

// Script returns the script that defines the <diagram-view> element, for
// pages that bundle it or serve it themselves and render fragments with
// NoScript.
func Script() string { return html.Script() }

// ScriptHandler serves Script as JavaScript, cacheable for a day and then
// revalidated by its ETag:
//
//	http.Handle("/diagram.js", diagram.ScriptHandler())
//	// <script src="/diagram.js" defer></script>, and fragments with NoScript
func ScriptHandler() http.Handler {
	s := Script()
	sum := sha256.Sum256([]byte(s))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/javascript; charset=utf-8")
		h.Set("Cache-Control", "public, max-age=86400")
		h.Set("ETag", etag)
		http.ServeContent(w, r, "diagram.js", time.Time{}, strings.NewReader(s))
	})
}
