package svgicon

import (
	"strings"
	"testing"
)

func TestUnsupported(t *testing.T) {
	tests := []struct {
		name, body, feature, element string
	}{
		{"style element", `<style>.a{fill:red}</style><path class="a" d="M0 0h1v1z"/>`, "style", "style"},
		{"style element CDATA", `<style><![CDATA[path{fill:red}]]></style>`, "style", "style"},
		{"filter element", `<defs><filter id="f"><feGaussianBlur stdDeviation="2"/></filter></defs>`, "filter", "filter"},
		{"mask element", `<mask id="m"><rect width="1" height="1"/></mask>`, "mask", "mask"},
		{"clipPath element", `<clipPath id="c"><rect width="1" height="1"/></clipPath>`, "clipPath", "clipPath"},
		{"pattern element", `<pattern id="p" width="1" height="1"/>`, "pattern", "pattern"},
		{"marker element", `<marker id="m"/>`, "marker", "marker"},
		{"text", `<text x="1" y="1">hi</text>`, "text", "text"},
		{"tspan", `<g><tspan>hi</tspan></g>`, "tspan", "tspan"},
		{"textPath", `<textPath/>`, "textPath", "textPath"},
		{"image", `<image href="x.png" width="1" height="1"/>`, "image", "image"},
		{"foreignObject", `<foreignObject width="1" height="1"><div xmlns="http://www.w3.org/1999/xhtml"/></foreignObject>`, "foreignObject", "foreignObject"},
		{"switch", `<switch><rect width="1" height="1"/></switch>`, "switch", "switch"},
		{"animate", `<rect width="1" height="1"><animate attributeName="x" to="5"/></rect>`, "animate", "animate"},
		{"script", `<script>alert(1)</script>`, "script", "script"},
		{"unknown svg element", `<blink/>`, "blink", "blink"},
		{"unsupported inside defs", `<defs><text>hi</text></defs>`, "text", "text"},

		{"clip-path attribute", `<path clip-path="url(#c)" d="M0 0h1v1z"/>`, "clip-path", "path"},
		{"clip-path in style", `<g style="clip-path: inset(1px)"><path d="M0 0h1v1z"/></g>`, "clip-path", "g"},
		{"mask attribute", `<path mask="url(#m)" d="M0 0h1v1z"/>`, "mask", "path"},
		{"filter attribute", `<g filter="url(#f)"/>`, "filter", "g"},
		{"filter function", `<path style="filter: blur(2px)" d="M0 0h1v1z"/>`, "filter", "path"},
		{"marker-start", `<path marker-start="url(#m)" d="M0 0h1"/>`, "marker-start", "path"},
		{"marker-mid", `<path marker-mid="url(#m)" d="M0 0h1"/>`, "marker-mid", "path"},
		{"marker-end", `<path marker-end="url(#m)" d="M0 0h1"/>`, "marker-end", "path"},
		{"marker shorthand", `<path style="marker: url(#m)" d="M0 0h1"/>`, "marker", "path"},
		{"marker inherited from group", `<g marker-end="url(#m)"><path d="M0 0h1"/></g>`, "marker-end", "g"},
		{"vector-effect", `<path vector-effect="non-scaling-stroke" d="M0 0h1"/>`, "vector-effect", "path"},
		{"mix-blend-mode", `<path style="mix-blend-mode: multiply" d="M0 0h1"/>`, "mix-blend-mode", "path"},
		{"transform-origin", `<path transform-origin="center" d="M0 0h1"/>`, "transform-origin", "path"},
		{"transform property", `<path style="transform: rotate(45deg)" d="M0 0h1"/>`, "transform property", "path"},
		{"geometry property", `<path style="d: path('M0 0h1')" d="M0 0h1"/>`, "d property", "path"},
		{"paint-order stroke first", `<path paint-order="stroke" stroke="red" d="M0 0h1v1z"/>`, "paint-order", "path"},
		{"pathLength with dashes", `<path pathLength="10" stroke="red" stroke-dasharray="1" d="M0 0h1"/>`, "pathLength", "path"},
		{"systemLanguage", `<path systemLanguage="en" d="M0 0h1"/>`, "systemLanguage", "path"},
		{"requiredExtensions", `<g requiredExtensions="http://example.com"/>`, "requiredExtensions", "g"},
		{"linejoin arcs", `<path stroke="red" stroke-linejoin="arcs" d="M0 0h1"/>`, "stroke-linejoin arcs", "path"},
		{"linejoin miter-clip", `<path stroke="red" stroke-linejoin="miter-clip" d="M0 0h1"/>`, "stroke-linejoin miter-clip", "path"},
		{"context-fill", `<path fill="context-fill" d="M0 0h1"/>`, "context-fill", "path"},
		{"em units", `<rect width="1em" height="1"/>`, "length unit em", "rect"},
		{"vw units", `<path stroke-width="1vw" d="M0 0h1"/>`, "length unit vw", "path"},
		{"CSS transform function", `<g transform="translateX(5)"/>`, "transform function translateX", "g"},

		{"missing gradient", `<path fill="url(#g)" d="M0 0h1v1z"/>`, "missing reference #g", "path"},
		{"missing gradient on stroke", `<path fill="none" stroke="url(#g)" d="M0 0h1"/>`, "missing reference #g", "path"},
		{"paint server that isn't one", `<rect id="r"/><path fill="url(#r)" d="M0 0h1v1z"/>`, "paint server <rect>", "path"},
		{"external paint", `<path fill="url(other.svg#g)" d="M0 0h1v1z"/>`, "external reference other.svg#g", "path"},
		{"external use", `<use href="other.svg#a"/>`, "external reference other.svg#a", "use"},
		{"missing use", `<use xlink:href="#a"/>`, "missing reference #a", "use"},
		{"nested svg overflow", `<svg width="10" height="10"><circle r="5"/></svg>`, "content overflowing the viewport (clipping)", "svg"},
		{"symbol overflow", `<symbol id="s" viewBox="0 0 1 1"><rect x="-1" width="1" height="1"/></symbol><use href="#s"/>`, "content overflowing the viewport (clipping)", "symbol"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(doc(tt.body)))
			if err == nil {
				t.Fatal("parsed without error")
			}
			if f := unsupportedFeature(t, err); f != tt.feature {
				t.Errorf("feature = %q, want %q (%v)", f, tt.feature, err)
			}
			if e := err.(*UnsupportedError).Element; e != tt.element {
				t.Errorf("element = %q, want %q", e, tt.element)
			}
			if !strings.Contains(err.Error(), tt.feature) {
				t.Errorf("message %q doesn't name the feature", err)
			}
		})
	}
	t.Run("transform on the root", func(t *testing.T) {
		_, err := Parse([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1" transform="scale(2)"/>`))
		if f := unsupportedFeature(t, err); f != "transform on outermost svg" {
			t.Errorf("feature = %q", f)
		}
	})
	t.Run("clip-path on the root", func(t *testing.T) {
		_, err := Parse([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1" clip-path="url(#c)"/>`))
		if f := unsupportedFeature(t, err); f != "clip-path" {
			t.Errorf("feature = %q", f)
		}
	})
}

// Things that look unsupported but don't change the picture.
func TestHarmless(t *testing.T) {
	for _, body := range []string{
		`<path clip-path="none" mask="none" filter="none" marker-end="none" vector-effect="none" mix-blend-mode="normal" d="M0 0h1v1z"/>`,
		`<path paint-order="stroke" fill="none" stroke="red" d="M0 0h1v1z"/>`, // nothing to reorder
		`<path paint-order="markers fill" stroke="red" d="M0 0h1v1z"/>`,
		`<path pathLength="10" stroke="red" d="M0 0h1"/>`, // no dashes
		`<path requiredFeatures="http://www.w3.org/TR/SVG11/feature#Shape" d="M0 0h1v1z"/>`,
		`<style></style><path d="M0 0h1v1z"/>`,
		`<path class="foo bar" d="M0 0h1v1z"/>`,
		`<path style="" d="M0 0h1v1z"/>`,
		`<g display="none"><path clip-path="url(#c)" d="M0 0h1v1z"/></g>`, // hidden, so never drawn
		`<defs><path clip-path="url(#c)" id="unused" d="M0 0h1v1z"/></defs><path d="M0 0h1v1z"/>`,
	} {
		if _, err := Parse([]byte(doc(body))); err != nil {
			t.Errorf("%s: %v", body, err)
		}
	}
}
