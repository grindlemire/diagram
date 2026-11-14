package svgicon

import (
	"fmt"
	"image/color"
	"strings"
)

type paintSpecKind uint8

const (
	specNone paintSpecKind = iota
	specColor
	specCurrent
	specURL
)

// paintSpec is a specified fill or stroke, resolved per shape.
type paintSpec struct {
	kind     paintSpecKind
	col      rgba
	id       string     // specURL
	fallback *paintSpec // specURL, optional
}

// style is the computed style of an element. The inherited properties carry
// over to children; the rest are reset per element.
type style struct {
	fill, stroke  paintSpec
	fillOpacity   float64
	strokeOpacity float64
	fillRule      FillRule
	strokeWidth   float64
	cap           LineCap
	join          LineJoin
	miter         float64
	dash          []float64 // user units, even count; nil for solid
	dashOffset    float64
	hidden        bool  // visibility: hidden or collapse
	color         *rgba // nil: currentColor stays symbolic
	strokeFirst   bool  // paint-order puts stroke before fill

	// Not inherited.
	opacity         float64
	displayNone     bool
	overflowVisible bool
}

func defaultStyle() style {
	return style{
		fill:          paintSpec{kind: specColor, col: rgba{c: color.RGBA{0, 0, 0, 255}, a: 1}},
		fillOpacity:   1,
		strokeOpacity: 1,
		strokeWidth:   1,
		miter:         4,
		opacity:       1,
	}
}

// properties lists the presentation properties in the order they're
// applied. A fixed order keeps the first reported error deterministic.
var properties = []string{
	"color", "fill", "fill-opacity", "fill-rule",
	"stroke", "stroke-opacity", "stroke-width", "stroke-linecap", "stroke-linejoin",
	"stroke-miterlimit", "stroke-dasharray", "stroke-dashoffset",
	"visibility", "display", "opacity", "overflow", "paint-order",
	// Rejected unless none or normal.
	"clip-path", "mask", "filter", "marker", "marker-start", "marker-mid", "marker-end",
	"vector-effect", "mix-blend-mode",
	// Gradient stops.
	"stop-color", "stop-opacity",
	// Always rejected when present.
	"transform-origin",
}

var isProperty = func() map[string]bool {
	m := make(map[string]bool, len(properties))
	for _, p := range properties {
		m[p] = true
	}
	return m
}()

// styleOnly are CSS properties that are only rejected in style="", since as
// attributes they're geometry the shape code reads itself.
var styleOnly = map[string]bool{
	"transform": true, "d": true, "x": true, "y": true, "width": true, "height": true,
	"r": true, "rx": true, "ry": true, "cx": true, "cy": true,
}

// decls merges an element's presentation attributes with its style
// attribute, which takes precedence. Unknown properties are dropped.
func decls(n *node) (map[string]string, error) {
	d := make(map[string]string)
	for k, v := range n.attrs {
		if isProperty[k] {
			d[k] = strings.TrimSpace(v)
		}
	}
	s, ok := n.attrs["style"]
	if !ok {
		return d, nil
	}
	// Drop CSS comments.
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			s = s[:i]
			break
		}
		s = s[:i] + s[i+2+j+2:]
	}
	for decl := range strings.SplitSeq(s, ";") {
		name, value, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		if v, ok := strings.CutSuffix(value, "!important"); ok {
			value = strings.TrimSpace(v)
		}
		switch {
		case styleOnly[name]:
			return nil, unsupported(name+" property", n)
		case isProperty[name]:
			d[name] = value
		}
	}
	return d, nil
}

// cascade computes n's style from its parent's.
func cascade(n *node, ps *style, vp viewport) (style, error) {
	st := *ps
	st.opacity, st.displayNone, st.overflowVisible = 1, false, false
	for _, a := range [...]string{"systemLanguage", "requiredExtensions"} {
		if _, ok := n.attrs[a]; ok {
			return st, unsupported(a, n)
		}
	}
	d, err := decls(n)
	if err != nil {
		return st, err
	}
	for _, name := range properties {
		v, ok := d[name]
		if !ok || v == "inherit" && name != "opacity" && name != "overflow" {
			// Inherited properties already hold the parent's value.
			continue
		}
		if err := st.set(name, v, ps, vp, n); err != nil {
			return st, err
		}
	}
	return st, nil
}

func (st *style) set(name, v string, ps *style, vp viewport, n *node) error {
	bad := func() error { return fmt.Errorf("svgicon: <%s>: bad %s %q", n.name, name, v) }
	kw := strings.ToLower(v)
	switch name {
	case "color":
		if kw == "currentcolor" {
			return nil
		}
		c, err := parseColor(v)
		if err != nil {
			return bad()
		}
		st.color = &c
	case "fill", "stroke":
		p, err := parsePaint(v, n)
		if err != nil {
			return err
		}
		if name == "fill" {
			st.fill = p
		} else {
			st.stroke = p
		}
	case "fill-opacity", "stroke-opacity", "opacity":
		if name == "opacity" && v == "inherit" {
			st.opacity = ps.opacity
			return nil
		}
		f, err := fraction(v)
		if err != nil {
			return bad()
		}
		switch name {
		case "fill-opacity":
			st.fillOpacity = f
		case "stroke-opacity":
			st.strokeOpacity = f
		default:
			st.opacity = f
		}
	case "fill-rule":
		switch kw {
		case "nonzero":
			st.fillRule = NonZero
		case "evenodd":
			st.fillRule = EvenOdd
		default:
			return bad()
		}
	case "stroke-width":
		w, err := length(v, axisDiag, vp, n)
		if err != nil {
			return err
		}
		if w < 0 {
			return bad()
		}
		st.strokeWidth = w
	case "stroke-linecap":
		switch kw {
		case "butt":
			st.cap = CapButt
		case "round":
			st.cap = CapRound
		case "square":
			st.cap = CapSquare
		default:
			return bad()
		}
	case "stroke-linejoin":
		switch kw {
		case "miter":
			st.join = JoinMiter
		case "round":
			st.join = JoinRound
		case "bevel":
			st.join = JoinBevel
		case "miter-clip", "arcs":
			return unsupported("stroke-linejoin "+kw, n)
		default:
			return bad()
		}
	case "stroke-miterlimit":
		m, err := parseNumber(v)
		if err != nil || m < 1 {
			return bad()
		}
		st.miter = m
	case "stroke-dasharray":
		dash, err := parseDash(v, vp, n)
		if err != nil {
			return err
		}
		st.dash = dash
	case "stroke-dashoffset":
		o, err := length(v, axisDiag, vp, n)
		if err != nil {
			return err
		}
		st.dashOffset = o
	case "visibility":
		switch kw {
		case "visible":
			st.hidden = false
		case "hidden", "collapse":
			st.hidden = true
		default:
			return bad()
		}
	case "display":
		st.displayNone = kw == "none"
	case "overflow":
		switch kw {
		case "inherit":
			st.overflowVisible = ps.overflowVisible
		case "visible", "auto":
			st.overflowVisible = true
		case "hidden", "scroll", "clip":
			st.overflowVisible = false
		default:
			return bad()
		}
	case "paint-order":
		st.strokeFirst = false
		if kw == "normal" {
			return nil
		}
		for _, f := range strings.Fields(kw) {
			if f == "fill" {
				break
			}
			if f == "stroke" {
				st.strokeFirst = true
				break
			}
		}
	case "clip-path", "mask", "filter", "marker", "marker-start", "marker-mid", "marker-end", "vector-effect":
		if kw != "none" {
			return unsupported(name, n)
		}
	case "mix-blend-mode":
		if kw != "normal" {
			return unsupported(name, n)
		}
	case "transform-origin":
		return unsupported(name, n)
	case "stop-color", "stop-opacity":
		// Read by the gradient code.
	}
	return nil
}

// parsePaint parses a fill or stroke value.
func parsePaint(v string, n *node) (paintSpec, error) {
	kw := strings.ToLower(v)
	switch kw {
	case "none", "transparent":
		return paintSpec{kind: specNone}, nil
	case "currentcolor":
		return paintSpec{kind: specCurrent}, nil
	case "context-fill", "context-stroke":
		return paintSpec{}, unsupported(kw, n)
	}
	if strings.HasPrefix(kw, "url(") {
		end := strings.IndexByte(v, ')')
		if end < 0 {
			return paintSpec{}, fmt.Errorf("svgicon: <%s>: bad paint %q", n.name, v)
		}
		ref := strings.Trim(strings.TrimSpace(v[4:end]), `'"`)
		id, ok := strings.CutPrefix(strings.TrimSpace(ref), "#")
		if !ok {
			return paintSpec{}, unsupported("external reference "+ref, n)
		}
		p := paintSpec{kind: specURL, id: id}
		if rest := strings.TrimSpace(v[end+1:]); rest != "" {
			fb, err := parsePaint(rest, n)
			if err != nil {
				return paintSpec{}, err
			}
			if fb.kind == specURL {
				return paintSpec{}, fmt.Errorf("svgicon: <%s>: bad paint %q", n.name, v)
			}
			p.fallback = &fb
		}
		return p, nil
	}
	c, err := parseColor(v)
	if err != nil {
		return paintSpec{}, fmt.Errorf("svgicon: <%s>: bad paint %q", n.name, v)
	}
	return paintSpec{kind: specColor, col: c}, nil
}

// parseDash parses stroke-dasharray into user units. An odd list repeats,
// and a list of zeros is solid.
func parseDash(v string, vp viewport, n *node) ([]float64, error) {
	if strings.EqualFold(v, "none") {
		return nil, nil
	}
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	if len(parts) == 0 {
		return nil, fmt.Errorf("svgicon: <%s>: bad stroke-dasharray %q", n.name, v)
	}
	dash := make([]float64, 0, 2*len(parts))
	sum := 0.0
	for _, p := range parts {
		l, err := length(p, axisDiag, vp, n)
		if err != nil {
			return nil, err
		}
		if l < 0 {
			return nil, fmt.Errorf("svgicon: <%s>: bad stroke-dasharray %q", n.name, v)
		}
		dash = append(dash, l)
		sum += l
	}
	if sum == 0 {
		return nil, nil
	}
	if len(dash)%2 == 1 {
		dash = append(dash, dash...)
	}
	return dash, nil
}
