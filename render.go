package diagram

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/paint"
	"github.com/grindlemire/diagram/internal/pipeline"
	"github.com/grindlemire/diagram/internal/render/drawio"
	"github.com/grindlemire/diagram/internal/render/html"
	pngr "github.com/grindlemire/diagram/internal/render/png"
	"github.com/grindlemire/diagram/internal/render/svg"
	"github.com/grindlemire/diagram/themes"
)

// renderConfig is the union of every format's options.
type renderConfig struct {
	margin      float64
	theme       *themes.Theme
	scale       float64
	transparent bool
	embedFont   bool
	errs        []error
}

func defaultConfig() renderConfig {
	return renderConfig{margin: 24, scale: 2, embedFont: true}
}

// An SVGOption configures RenderSVG.
type SVGOption interface{ applySVG(*renderConfig) }

// A PNGOption configures RenderPNG.
type PNGOption interface{ applyPNG(*renderConfig) }

// A DrawioOption configures RenderDrawio and WriteDrawio.
type DrawioOption interface{ applyDrawio(*renderConfig) }

// An HTMLOption configures RenderHTML.
type HTMLOption interface{ applyHTML(*renderConfig) }

// A RenderOption applies to every format.
type RenderOption interface {
	SVGOption
	PNGOption
	DrawioOption
	HTMLOption
	EmbedOption
}

type anyOpt func(*renderConfig)

func (f anyOpt) applySVG(c *renderConfig)    { f(c) }
func (f anyOpt) applyPNG(c *renderConfig)    { f(c) }
func (f anyOpt) applyDrawio(c *renderConfig) { f(c) }
func (f anyOpt) applyHTML(c *renderConfig)   { f(c) }

// fontOpt applies to the formats that embed fonts: SVG and HTML.
type fontOpt func(*renderConfig)

func (f fontOpt) applySVG(c *renderConfig)  { f(c) }
func (f fontOpt) applyHTML(c *renderConfig) { f(c) }

type pngOpt func(*renderConfig)

func (f pngOpt) applyPNG(c *renderConfig) { f(c) }

type rasterOpt func(*renderConfig)

func (f rasterOpt) applySVG(c *renderConfig) { f(c) }
func (f rasterOpt) applyPNG(c *renderConfig) { f(c) }

// Margin sets the empty space around the diagram, in pixels. The default
// is 24.
func Margin(px float64) RenderOption {
	return anyOpt(func(c *renderConfig) {
		if px < 0 {
			c.errs = append(c.errs, fmt.Errorf("Margin(%v): want a non-negative number", px))
			return
		}
		c.margin = px
	})
}

// WithTheme renders with a different theme than the diagram was built
// with, so one diagram can be drawn in several themes.
func WithTheme(t *themes.Theme) RenderOption {
	return anyOpt(func(c *renderConfig) { c.theme = t })
}

// Scale sets the PNG's pixel density. The default is 2, which is sharp on
// high-density screens.
func Scale(s float64) PNGOption {
	return pngOpt(func(c *renderConfig) {
		if s <= 0 {
			c.errs = append(c.errs, fmt.Errorf("Scale(%v): want a positive number", s))
			return
		}
		c.scale = s
	})
}

// Transparent leaves the background unpainted in SVG and PNG output, and
// leaves an HTML page's or embed's background to the page.
var Transparent interface {
	SVGOption
	PNGOption
	HTMLOption
	EmbedOption
} = rasterOpt(func(c *renderConfig) { c.transparent = true })

// EmbedFont embeds the subset of Inter, and of any fallback font from
// RegisterFont, that the diagram uses, so text looks the same in every
// viewer. This is the default. It applies to SVG and HTML output.
var EmbedFont interface {
	SVGOption
	HTMLOption
	EmbedOption
} = fontOpt(func(c *renderConfig) { c.embedFont = true })

// FontByName refers to Inter and any fallback fonts by family name instead
// of embedding them. Files are smaller, but viewers without those fonts use
// a system font and text may not fit its box exactly. It applies to SVG
// and HTML output.
var FontByName interface {
	SVGOption
	HTMLOption
	EmbedOption
} = fontOpt(func(c *renderConfig) { c.embedFont = false })

// lay validates the diagram, then lays it out and routes its edges.
func (d *Diagram) lay(cfg renderConfig) (*layout.Result, error) {
	t := d.m.Theme
	if cfg.theme != nil {
		t = cfg.theme
	}
	return pipeline.Lay(d.m, t, cfg.errs...)
}

// RenderSVG writes the diagram as SVG.
func (d *Diagram) RenderSVG(w io.Writer, opts ...SVGOption) error {
	cfg := defaultConfig()
	for _, o := range opts {
		o.applySVG(&cfg)
	}
	res, err := d.lay(cfg)
	if err != nil {
		return err
	}
	sc := paint.Paint(res, paint.Options{Margin: cfg.margin, Transparent: cfg.transparent})
	return svg.Render(w, sc, svg.Options{Title: d.m.Title, EmbedFont: cfg.embedFont})
}

// RenderHTML writes the diagram as a self-contained interactive HTML page:
// the SVG with pan and zoom, highlighting of each element's connections,
// tooltips with descriptions, Tooltip text and Link URLs, keyboard
// navigation, search, and a light, dark and system theme switch when the
// theme has a dark palette (themes.Auto) or is themes.Light, which is
// paired with themes.Dark. The page makes no network requests.
func (d *Diagram) RenderHTML(w io.Writer, opts ...HTMLOption) error {
	cfg := defaultConfig()
	for _, o := range opts {
		o.applyHTML(&cfg)
	}
	res, err := d.lay(cfg)
	if err != nil {
		return err
	}
	sc := paint.Paint(res, paint.Options{Margin: cfg.margin, Transparent: true, Interactive: true})
	return html.Render(w, sc, html.Options{Title: d.m.Title, EmbedFont: cfg.embedFont})
}

// RenderPNG writes the diagram as PNG.
func (d *Diagram) RenderPNG(w io.Writer, opts ...PNGOption) error {
	cfg := defaultConfig()
	for _, o := range opts {
		o.applyPNG(&cfg)
	}
	res, err := d.lay(cfg)
	if err != nil {
		return err
	}
	sc := paint.Paint(res, paint.Options{Margin: cfg.margin, Transparent: cfg.transparent})
	return pngr.Render(w, sc, pngr.Options{Scale: cfg.scale})
}

// RenderDrawio writes the diagram as an editable draw.io file.
func (d *Diagram) RenderDrawio(w io.Writer, opts ...DrawioOption) error {
	return WriteDrawio(w, []*Diagram{d}, opts...)
}

// WriteDrawio writes several diagrams into one draw.io file, one page each.
func WriteDrawio(w io.Writer, ds []*Diagram, opts ...DrawioOption) error {
	cfg := defaultConfig()
	for _, o := range opts {
		o.applyDrawio(&cfg)
	}
	if len(ds) == 0 {
		return errors.New("diagram: WriteDrawio needs at least one diagram")
	}
	pages := make([]drawio.Page, len(ds))
	var errs []error
	for i, d := range ds {
		res, err := d.lay(cfg)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		pages[i] = drawio.Page{Name: d.m.Title, Result: res, Margin: cfg.margin}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	return drawio.Render(w, pages)
}

// WriteFile renders the diagram to path, choosing the format from its
// extension: .svg, .png, .html (also .htm) or .drawio (also .drawio.xml).
// The file is only written if rendering succeeds.
func (d *Diagram) WriteFile(path string, opts ...RenderOption) error {
	var buf bytes.Buffer
	var err error
	switch ext := strings.ToLower(filepath.Ext(path)); {
	case ext == ".svg":
		err = d.RenderSVG(&buf, asSVG(opts)...)
	case ext == ".png":
		err = d.RenderPNG(&buf, asPNG(opts)...)
	case ext == ".html", ext == ".htm":
		err = d.RenderHTML(&buf, asHTML(opts)...)
	case ext == ".drawio", strings.HasSuffix(strings.ToLower(path), ".drawio.xml"):
		err = d.RenderDrawio(&buf, asDrawio(opts)...)
	default:
		return fmt.Errorf("diagram: %s: unknown format (want .svg, .png, .html or .drawio)", path)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func asSVG(o []RenderOption) []SVGOption {
	out := make([]SVGOption, len(o))
	for i := range o {
		out[i] = o[i]
	}
	return out
}

func asPNG(o []RenderOption) []PNGOption {
	out := make([]PNGOption, len(o))
	for i := range o {
		out[i] = o[i]
	}
	return out
}

func asHTML(o []RenderOption) []HTMLOption {
	out := make([]HTMLOption, len(o))
	for i := range o {
		out[i] = o[i]
	}
	return out
}

func asDrawio(o []RenderOption) []DrawioOption {
	out := make([]DrawioOption, len(o))
	for i := range o {
		out[i] = o[i]
	}
	return out
}
