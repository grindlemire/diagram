package svgicon

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// Parse converts SVG source into shapes in the icon's viewBox coordinate
// space, with transforms applied. currentColor stays symbolic unless the
// icon sets the color property itself. Anything outside the supported
// subset returns an *UnsupportedError.
func Parse(data []byte) (*Icon, error) {
	root, err := buildTree(data)
	if err != nil {
		return nil, err
	}
	p := &parser{ids: make(map[string]*node), active: make(map[*node]bool)}
	if err := check(root, p.ids); err != nil {
		return nil, err
	}
	ic := &Icon{}
	if vb, ok, err := parseViewBox(root); err != nil {
		return nil, err
	} else if ok {
		if vb[2] <= 0 || vb[3] <= 0 {
			return nil, fmt.Errorf("svgicon: bad viewBox %q", root.attrs["viewBox"])
		}
		ic.ViewBox = vb
	} else {
		w, errW := parseNumber(strings.TrimSuffix(strings.TrimSpace(root.attrs["width"]), "px"))
		h, errH := parseNumber(strings.TrimSuffix(strings.TrimSpace(root.attrs["height"]), "px"))
		if errW != nil || errH != nil || w <= 0 || h <= 0 {
			return nil, errors.New("svgicon: svg needs a viewBox or a width and height")
		}
		ic.ViewBox = [4]float64{0, 0, w, h}
	}
	if _, ok := root.attrs["transform"]; ok {
		return nil, unsupported("transform on outermost svg", root)
	}
	vp := viewport{ic.ViewBox[2], ic.ViewBox[3]}
	def := defaultStyle()
	st, err := cascade(root, &def, vp)
	if err != nil {
		return nil, err
	}
	if !st.displayNone {
		for _, c := range root.children {
			if err := p.render(c, &st, Identity, st.opacity, vp); err != nil {
				return nil, err
			}
		}
	}
	ic.Shapes = p.shapes
	return ic, nil
}

type parser struct {
	ids    map[string]*node
	active map[*node]bool // elements being instantiated by <use>, to catch cycles
	uses   int            // <use> instantiations so far
	shapes []Shape
}

// Limits on <use>, so a "billion laughs" icon can't explode.
const (
	maxUseDepth = 32
	maxUses     = 10000
)

// render draws n and its children. ps is the parent's computed style, ctm
// maps n's parent's user space to the output space, and op is the product
// of the ancestors' opacities.
func (p *parser) render(n *node, ps *style, ctm Matrix, op float64, vp viewport) error {
	if n.foreign {
		return nil
	}
	switch n.name {
	case "svg", "g", "a", "use", "path", "rect", "circle", "ellipse", "line", "polyline", "polygon":
	default:
		// defs, symbol, gradients, stops and descriptive elements don't
		// render in place.
		return nil
	}
	st, err := cascade(n, ps, vp)
	if err != nil || st.displayNone {
		return err
	}
	if t, ok := n.attrs["transform"]; ok {
		m, err := parseTransform(t, n)
		if err != nil {
			return err
		}
		ctm = ctm.Mul(m)
	}
	if ctm.Det() == 0 {
		return nil // a singular transform disables rendering
	}
	op *= st.opacity
	switch n.name {
	case "g", "a":
		for _, c := range n.children {
			if err := p.render(c, &st, ctm, op, vp); err != nil {
				return err
			}
		}
		return nil
	case "svg":
		return p.viewportElement(n, &st, ctm, op, vp, nil)
	case "use":
		return p.use(n, &st, ctm, op, vp)
	}
	return p.shape(n, &st, ctm, op, vp)
}

func (p *parser) use(n *node, st *style, ctm Matrix, op float64, vp viewport) error {
	href := n.ref()
	if href == "" {
		return nil
	}
	id, ok := strings.CutPrefix(href, "#")
	if !ok {
		return unsupported("external reference "+href, n)
	}
	ref := p.ids[id]
	if ref == nil {
		return unsupported("missing reference #"+id, n)
	}
	if p.active[ref] || len(p.active) >= maxUseDepth {
		return fmt.Errorf("svgicon: <use> reference cycle through #%s", id)
	}
	if p.uses++; p.uses > maxUses {
		return fmt.Errorf("svgicon: more than %d <use> instantiations", maxUses)
	}
	x, err := attrLength(n, "x", axisX, vp)
	if err != nil {
		return err
	}
	y, err := attrLength(n, "y", axisY, vp)
	if err != nil {
		return err
	}
	ctm = ctm.Mul(translate(x, y))
	p.active[ref] = true
	defer delete(p.active, ref)
	switch {
	case ref.foreign:
		return nil
	case ref.name == "symbol":
		sst, err := cascade(ref, st, vp)
		if err != nil || sst.displayNone {
			return err
		}
		if _, ok := ref.attrs["transform"]; ok {
			return unsupported("transform on symbol", ref)
		}
		return p.viewportElement(ref, &sst, ctm, op*sst.opacity, vp, n)
	case ref.name == "svg":
		// render cascades the svg's own style, then reaches viewportElement
		// without the <use> sizes; handle it here to pass them.
		sst, err := cascade(ref, st, vp)
		if err != nil || sst.displayNone {
			return err
		}
		if t, ok := ref.attrs["transform"]; ok {
			m, err := parseTransform(t, ref)
			if err != nil {
				return err
			}
			ctm = ctm.Mul(m)
		}
		return p.viewportElement(ref, &sst, ctm, op*sst.opacity, vp, n)
	}
	return p.render(ref, st, ctm, op, vp)
}

// viewportElement draws a nested <svg>, or a <symbol> instantiated by use.
// use, when set, supplies width and height. The content is drawn in its own
// coordinates first so overflow can be checked against the viewport, which
// SVG clips to and this package can't.
func (p *parser) viewportElement(n *node, st *style, ctm Matrix, op float64, vp viewport, use *node) error {
	var x, y float64
	var err error
	if n.name == "svg" {
		if x, err = attrLength(n, "x", axisX, vp); err != nil {
			return err
		}
		if y, err = attrLength(n, "y", axisY, vp); err != nil {
			return err
		}
	}
	size := func(name string, a axis) (float64, error) {
		src := n
		if use != nil {
			if _, ok := use.attrs[name]; ok {
				src = use
			}
		}
		v, ok := src.attrs[name]
		if !ok || strings.TrimSpace(v) == "auto" {
			v = "100%"
		}
		l, err := length(v, a, vp, src)
		if err == nil && l < 0 {
			err = fmt.Errorf("svgicon: <%s>: negative %s", src.name, name)
		}
		return l, err
	}
	w, err := size("width", axisX)
	if err != nil {
		return err
	}
	h, err := size("height", axisY)
	if err != nil {
		return err
	}
	if w == 0 || h == 0 {
		return nil
	}

	m := translate(x, y)
	inner := viewport{w, h}
	clip := rectBox{0, 0, w, h}
	vb, hasVB, err := parseViewBox(n)
	if err != nil {
		return err
	}
	if hasVB {
		if vb[2] <= 0 || vb[3] <= 0 {
			return nil
		}
		vbm, err := viewBoxTransform(vb, w, h, n.attrs["preserveAspectRatio"], n)
		if err != nil {
			return err
		}
		inv, _ := vbm.Invert()
		x0, y0 := inv.Apply(0, 0)
		x1, y1 := inv.Apply(w, h)
		clip = rectBox{x0, y0, x1, y1}
		m = m.Mul(vbm)
		inner = viewport{vb[2], vb[3]}
	}

	saved := p.shapes
	p.shapes = nil
	for _, c := range n.children {
		if err := p.render(c, st, Identity, op, inner); err != nil {
			return err
		}
	}
	content := p.shapes
	p.shapes = saved

	if !st.overflowVisible {
		eps := 1e-6 * math.Max(clip.x1-clip.x0, clip.y1-clip.y0)
		for _, s := range content {
			b := s.extent()
			if b.x0 < clip.x0-eps || b.y0 < clip.y0-eps || b.x1 > clip.x1+eps || b.y1 > clip.y1+eps {
				return unsupported("content overflowing the viewport (clipping)", n)
			}
		}
	}
	full := ctm.Mul(m)
	for i := range content {
		content[i].transform(full)
	}
	p.shapes = append(p.shapes, content...)
	return nil
}

// extent is a conservative bound of what the shape paints.
func (s *Shape) extent() rectBox {
	b, _ := s.Path.bounds()
	if s.Stroke.Kind != None {
		e := s.StrokeWidth / 2
		if s.Cap == CapSquare {
			e *= math.Sqrt2
		}
		if s.Join == JoinMiter && s.Path.hasJoins() {
			e = math.Max(e, s.StrokeWidth/2*s.MiterLimit)
		}
		b = rectBox{b.x0 - e, b.y0 - e, b.x1 + e, b.y1 + e}
	}
	return b
}

// hasJoins reports whether any subpath has a corner: two segments or a
// close.
func (p Path) hasJoins() bool {
	segs := 0
	for _, c := range p {
		switch c.Op {
		case MoveTo:
			segs = 0
		case Close:
			return true
		default:
			if segs++; segs > 1 {
				return true
			}
		}
	}
	return false
}

// transform applies m to a finished shape.
func (s *Shape) transform(m Matrix) {
	s.Path = s.Path.transform(m)
	k := m.scale()
	s.StrokeWidth *= k
	s.DashOffset *= k
	for i := range s.Dash {
		s.Dash[i] *= k
	}
	for _, pt := range [...]*Paint{&s.Fill, &s.Stroke} {
		if pt.Gradient != nil {
			pt.Gradient.Transform = m.Mul(pt.Gradient.Transform)
		}
	}
}

func parseViewBox(n *node) ([4]float64, bool, error) {
	var vb [4]float64
	s, ok := n.attrs["viewBox"]
	if !ok {
		return vb, false, nil
	}
	f, err := numberList(s)
	if err != nil || len(f) != 4 || f[2] < 0 || f[3] < 0 {
		return vb, false, fmt.Errorf("svgicon: <%s>: bad viewBox %q", n.name, s)
	}
	copy(vb[:], f)
	return vb, true, nil
}

// viewBoxTransform maps a viewBox onto a w×h viewport at the origin,
// following preserveAspectRatio.
func viewBoxTransform(vb [4]float64, w, h float64, par string, n *node) (Matrix, error) {
	f := strings.Fields(par)
	if len(f) > 0 && f[0] == "defer" {
		f = f[1:]
	}
	align, slice := "xMidYMid", false
	if len(f) > 0 {
		align = f[0]
	}
	if len(f) > 1 {
		switch f[1] {
		case "meet":
		case "slice":
			slice = true
		default:
			return Matrix{}, fmt.Errorf("svgicon: <%s>: bad preserveAspectRatio %q", n.name, par)
		}
	}
	if len(f) > 2 {
		return Matrix{}, fmt.Errorf("svgicon: <%s>: bad preserveAspectRatio %q", n.name, par)
	}
	sx, sy := w/vb[2], h/vb[3]
	if align == "none" {
		return Matrix{sx, 0, 0, sy, -vb[0] * sx, -vb[1] * sy}, nil
	}
	if len(align) != 8 || align[0] != 'x' || align[4] != 'Y' {
		return Matrix{}, fmt.Errorf("svgicon: <%s>: bad preserveAspectRatio %q", n.name, par)
	}
	pos := func(s string) (float64, bool) {
		switch s {
		case "Min":
			return 0, true
		case "Mid":
			return 0.5, true
		case "Max":
			return 1, true
		}
		return 0, false
	}
	ax, okX := pos(align[1:4])
	ay, okY := pos(align[5:8])
	if !okX || !okY {
		return Matrix{}, fmt.Errorf("svgicon: <%s>: bad preserveAspectRatio %q", n.name, par)
	}
	s := math.Min(sx, sy)
	if slice {
		s = math.Max(sx, sy)
	}
	tx := -vb[0]*s + ax*(w-vb[2]*s)
	ty := -vb[1]*s + ay*(h-vb[3]*s)
	return Matrix{s, 0, 0, s, tx, ty}, nil
}

func attrLength(n *node, name string, a axis, vp viewport) (float64, error) {
	v, ok := n.attrs[name]
	if !ok {
		return 0, nil
	}
	return length(v, a, vp, n)
}

// geometry builds the element's path in its own user space.
func geometry(n *node, vp viewport) (Path, error) {
	lens := func(names ...string) ([]float64, error) {
		out := make([]float64, len(names))
		for i, name := range names {
			a := axisX
			switch name {
			case "y", "y1", "y2", "cy", "height", "ry":
				a = axisY
			case "r":
				a = axisDiag
			}
			v, err := attrLength(n, name, a, vp)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	}
	negative := func(name string) error { return fmt.Errorf("svgicon: <%s>: negative %s", n.name, name) }
	switch n.name {
	case "path":
		p, err := parsePathData(n.attrs["d"])
		if err != nil {
			return nil, fmt.Errorf("svgicon: <path>: %w", err)
		}
		return p, nil
	case "rect":
		v, err := lens("x", "y", "width", "height")
		if err != nil {
			return nil, err
		}
		x, y, w, h := v[0], v[1], v[2], v[3]
		if w < 0 || h < 0 {
			return nil, negative("size")
		}
		if w == 0 || h == 0 {
			return nil, nil
		}
		rx, rxSet, err := radius(n, "rx", axisX, vp)
		if err != nil {
			return nil, err
		}
		ry, rySet, err := radius(n, "ry", axisY, vp)
		if err != nil {
			return nil, err
		}
		switch {
		case rxSet && !rySet:
			ry = rx
		case rySet && !rxSet:
			rx = ry
		}
		return rectPath(x, y, w, h, math.Min(rx, w/2), math.Min(ry, h/2)), nil
	case "circle":
		v, err := lens("cx", "cy", "r")
		if err != nil {
			return nil, err
		}
		if v[2] < 0 {
			return nil, negative("r")
		}
		if v[2] == 0 {
			return nil, nil
		}
		return ellipsePath(v[0], v[1], v[2], v[2]), nil
	case "ellipse":
		v, err := lens("cx", "cy")
		if err != nil {
			return nil, err
		}
		rx, rxSet, err := radius(n, "rx", axisX, vp)
		if err != nil {
			return nil, err
		}
		ry, rySet, err := radius(n, "ry", axisY, vp)
		if err != nil {
			return nil, err
		}
		switch {
		case rxSet && !rySet:
			ry = rx
		case rySet && !rxSet:
			rx = ry
		}
		if rx == 0 || ry == 0 {
			return nil, nil
		}
		return ellipsePath(v[0], v[1], rx, ry), nil
	case "line":
		v, err := lens("x1", "y1", "x2", "y2")
		if err != nil {
			return nil, err
		}
		var p Path
		p.moveTo(v[0], v[1])
		p.lineTo(v[2], v[3])
		return p, nil
	case "polyline", "polygon":
		pts, err := numberList(n.attrs["points"])
		if err != nil {
			return nil, fmt.Errorf("svgicon: <%s>: points: %w", n.name, err)
		}
		if len(pts)%2 != 0 {
			return nil, fmt.Errorf("svgicon: <%s>: odd number of coordinates in points", n.name)
		}
		var p Path
		for i := 0; i < len(pts); i += 2 {
			if i == 0 {
				p.moveTo(pts[0], pts[1])
			} else {
				p.lineTo(pts[i], pts[i+1])
			}
		}
		if n.name == "polygon" && len(p) > 0 {
			p.close()
		}
		return p, nil
	}
	return nil, nil
}

// radius reads rx or ry; "auto" or absent means unset.
func radius(n *node, name string, a axis, vp viewport) (float64, bool, error) {
	v, ok := n.attrs[name]
	if !ok || strings.TrimSpace(v) == "auto" {
		return 0, false, nil
	}
	r, err := length(v, a, vp, n)
	if err != nil {
		return 0, false, err
	}
	if r < 0 {
		return 0, false, fmt.Errorf("svgicon: <%s>: negative %s", n.name, name)
	}
	return r, true, nil
}

func (p *parser) shape(n *node, st *style, ctm Matrix, op float64, vp viewport) error {
	path, err := geometry(n, vp)
	if err != nil || len(path) == 0 || st.hidden {
		return err
	}
	var fill Paint
	fillAlpha := 1.0
	if n.name != "line" { // a line has no interior
		if fill, fillAlpha, err = p.paint(st.fill, n, st, path, ctm, vp); err != nil {
			return err
		}
	}
	stroke, strokeAlpha, err := p.paint(st.stroke, n, st, path, ctm, vp)
	if err != nil {
		return err
	}
	if st.strokeWidth == 0 {
		stroke = Paint{}
	}
	if fill.Kind == None && stroke.Kind == None {
		return nil
	}
	if stroke.Kind != None {
		if st.strokeFirst && fill.Kind != None {
			return unsupported("paint-order", n)
		}
		if _, ok := n.attrs["pathLength"]; ok && st.dash != nil {
			return unsupported("pathLength", n)
		}
	}
	s := Shape{
		Path:          path,
		Fill:          fill,
		Stroke:        stroke,
		StrokeWidth:   st.strokeWidth,
		Cap:           st.cap,
		Join:          st.join,
		MiterLimit:    st.miter,
		DashOffset:    st.dashOffset,
		FillRule:      st.fillRule,
		Opacity:       op,
		FillOpacity:   st.fillOpacity * fillAlpha,
		StrokeOpacity: st.strokeOpacity * strokeAlpha,
	}
	if stroke.Kind == None {
		s.StrokeWidth, s.DashOffset = 0, 0
	} else if st.dash != nil {
		s.Dash = append([]float64(nil), st.dash...)
	}
	// Gradients already include ctm, so transform only the geometry.
	s.Path = path.transform(ctm)
	k := ctm.scale()
	s.StrokeWidth *= k
	s.DashOffset *= k
	for i := range s.Dash {
		s.Dash[i] *= k
	}
	p.shapes = append(p.shapes, s)
	return nil
}

// paint resolves a paint for one shape. It returns the paint and an alpha
// to fold into the shape's fill or stroke opacity.
func (p *parser) paint(spec paintSpec, n *node, st *style, path Path, ctm Matrix, vp viewport) (Paint, float64, error) {
	switch spec.kind {
	case specColor:
		if spec.col.a == 0 {
			return Paint{}, 1, nil
		}
		return Paint{Kind: Solid, Color: spec.col.c}, spec.col.a, nil
	case specCurrent:
		if st.color != nil {
			return Paint{Kind: Solid, Color: st.color.c}, st.color.a, nil
		}
		return Paint{Kind: CurrentColor}, 1, nil
	case specURL:
		ref := p.ids[spec.id]
		if ref == nil {
			if spec.fallback != nil {
				return p.paint(*spec.fallback, n, st, path, ctm, vp)
			}
			return Paint{}, 0, unsupported("missing reference #"+spec.id, n)
		}
		switch ref.name {
		case "linearGradient", "radialGradient":
			return p.gradient(ref, path, ctm, vp)
		}
		return Paint{}, 0, unsupported("paint server <"+ref.name+">", n)
	}
	return Paint{}, 1, nil
}
