package svgicon

import (
	"fmt"
	"image/color"
	"strings"
)

// gradient resolves a gradient element for one shape: follows href
// inheritance, applies units and transforms, and reduces degenerate cases to
// None or Solid.
func (p *parser) gradient(g *node, path Path, ctm Matrix, vp viewport) (Paint, float64, error) {
	chain := []*node{g}
	seen := map[*node]bool{g: true}
	for cur := g; ; {
		href := cur.ref()
		if href == "" {
			break
		}
		id, ok := strings.CutPrefix(href, "#")
		if !ok {
			return Paint{}, 0, unsupported("external reference "+href, cur)
		}
		next := p.ids[id]
		if next == nil {
			return Paint{}, 0, unsupported("missing reference #"+id, cur)
		}
		if next.name != "linearGradient" && next.name != "radialGradient" {
			return Paint{}, 0, unsupported("gradient href to <"+next.name+">", cur)
		}
		if seen[next] {
			return Paint{}, 0, fmt.Errorf("svgicon: gradient href cycle through #%s", id)
		}
		seen[next] = true
		chain = append(chain, next)
		cur = next
	}
	// attr finds an attribute on the gradient or the nearest referenced
	// one. Geometry attributes only inherit between gradients of one kind.
	attr := func(name string, sameKind bool) (string, bool) {
		for _, c := range chain {
			if sameKind && c.name != g.name {
				continue
			}
			if v, ok := c.attrs[name]; ok {
				return strings.TrimSpace(v), true
			}
		}
		return "", false
	}

	obb := true
	if v, ok := attr("gradientUnits", false); ok {
		switch v {
		case "objectBoundingBox":
		case "userSpaceOnUse":
			obb = false
		default:
			return Paint{}, 0, fmt.Errorf("svgicon: <%s>: bad gradientUnits %q", g.name, v)
		}
	}
	var spread Spread
	if v, ok := attr("spreadMethod", false); ok {
		switch v {
		case "pad":
		case "reflect":
			spread = SpreadReflect
		case "repeat":
			spread = SpreadRepeat
		default:
			return Paint{}, 0, fmt.Errorf("svgicon: <%s>: bad spreadMethod %q", g.name, v)
		}
	}
	gt := Identity
	if v, ok := attr("gradientTransform", false); ok {
		m, err := parseTransform(v, g)
		if err != nil {
			return Paint{}, 0, err
		}
		gt = m
	}

	var stops []Stop
	for _, c := range chain {
		var err error
		if stops, err = gradientStops(c); err != nil {
			return Paint{}, 0, err
		}
		if len(stops) > 0 {
			break
		}
	}
	switch len(stops) {
	case 0:
		return Paint{}, 1, nil
	case 1:
		return stopPaint(stops[0])
	}

	coord := func(name, def string, a axis) (float64, error) {
		v, ok := attr(name, true)
		if !ok {
			v = def
		}
		if obb {
			if f, ok := strings.CutSuffix(v, "%"); ok {
				x, err := parseNumber(f)
				return x / 100, err
			}
			return parseNumber(v)
		}
		return length(v, a, vp, g)
	}
	gr := &Gradient{Spread: spread, Stops: stops}
	kind := Linear
	var err error
	if g.name == "linearGradient" {
		for _, c := range [...]struct {
			dst  *float64
			name string
			def  string
			a    axis
		}{
			{&gr.X1, "x1", "0%", axisX}, {&gr.Y1, "y1", "0%", axisY},
			{&gr.X2, "x2", "100%", axisX}, {&gr.Y2, "y2", "0%", axisY},
		} {
			if *c.dst, err = coord(c.name, c.def, c.a); err != nil {
				return Paint{}, 0, err
			}
		}
		if gr.X1 == gr.X2 && gr.Y1 == gr.Y2 {
			return stopPaint(stops[len(stops)-1])
		}
	} else {
		kind = Radial
		for _, c := range [...]struct {
			dst  *float64
			name string
			def  string
			a    axis
		}{
			{&gr.CX, "cx", "50%", axisX}, {&gr.CY, "cy", "50%", axisY},
			{&gr.R, "r", "50%", axisDiag}, {&gr.FR, "fr", "0%", axisDiag},
		} {
			if *c.dst, err = coord(c.name, c.def, c.a); err != nil {
				return Paint{}, 0, err
			}
		}
		gr.FX, gr.FY = gr.CX, gr.CY
		if _, ok := attr("fx", true); ok {
			if gr.FX, err = coord("fx", "", axisX); err != nil {
				return Paint{}, 0, err
			}
		}
		if _, ok := attr("fy", true); ok {
			if gr.FY, err = coord("fy", "", axisY); err != nil {
				return Paint{}, 0, err
			}
		}
		if gr.R < 0 || gr.FR < 0 {
			return Paint{}, 0, fmt.Errorf("svgicon: <%s>: negative radius", g.name)
		}
		if gr.R == 0 {
			return stopPaint(stops[len(stops)-1])
		}
	}

	m := ctm
	if obb {
		b, ok := path.bounds()
		if !ok || b.x1 == b.x0 || b.y1 == b.y0 {
			return Paint{}, 1, nil // a bounding-box gradient needs an area
		}
		m = m.Mul(Matrix{b.x1 - b.x0, 0, 0, b.y1 - b.y0, b.x0, b.y0})
	}
	gr.Transform = m.Mul(gt)
	if gr.Transform.Det() == 0 {
		return Paint{}, 1, nil
	}
	return Paint{Kind: kind, Gradient: gr}, 1, nil
}

// stopPaint paints a whole shape with one stop.
func stopPaint(s Stop) (Paint, float64, error) {
	if s.CurrentColor {
		return Paint{Kind: CurrentColor}, s.Opacity, nil
	}
	return Paint{Kind: Solid, Color: s.Color}, s.Opacity, nil
}

// gradientStops reads a gradient element's own <stop> children.
func gradientStops(g *node) ([]Stop, error) {
	var stops []Stop
	last := 0.0
	for _, c := range g.children {
		if c.foreign || c.name != "stop" {
			continue
		}
		d, err := decls(c)
		if err != nil {
			return nil, err
		}
		s := Stop{Opacity: 1}
		if v, ok := c.attrs["offset"]; ok {
			if s.Offset, err = fraction(v); err != nil {
				return nil, fmt.Errorf("svgicon: <stop>: bad offset %q", v)
			}
		}
		s.Offset = max(s.Offset, last)
		last = s.Offset

		col, err := stopProperty(c, "stop-color", d)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(col) {
		case "":
			s.Color = color.RGBA{0, 0, 0, 255}
		case "currentcolor":
			cc, err := inheritedColor(c)
			if err != nil {
				return nil, err
			}
			if cc == nil {
				s.CurrentColor = true
			} else {
				s.Color, s.Opacity = cc.c, cc.a
			}
		default:
			rc, err := parseColor(col)
			if err != nil {
				return nil, fmt.Errorf("svgicon: <stop>: bad stop-color %q", col)
			}
			s.Color, s.Opacity = rc.c, rc.a
		}
		op, err := stopProperty(c, "stop-opacity", d)
		if err != nil {
			return nil, err
		}
		if op != "" {
			f, err := fraction(op)
			if err != nil {
				return nil, fmt.Errorf("svgicon: <stop>: bad stop-opacity %q", op)
			}
			s.Opacity *= f
		}
		stops = append(stops, s)
	}
	return stops, nil
}

// stopProperty returns a stop's specified value for a non-inherited
// property, following explicit "inherit" up the tree. "" means the default.
func stopProperty(n *node, name string, d map[string]string) (string, error) {
	for {
		v := d[name]
		if v != "inherit" {
			return v, nil
		}
		n = n.parent
		if n == nil {
			return "", nil
		}
		var err error
		if d, err = decls(n); err != nil {
			return "", err
		}
	}
}

// inheritedColor returns the color property in effect at n in the document
// tree, or nil if nothing sets it.
func inheritedColor(n *node) (*rgba, error) {
	for ; n != nil; n = n.parent {
		d, err := decls(n)
		if err != nil {
			return nil, err
		}
		v, ok := d["color"]
		if !ok || v == "inherit" || strings.EqualFold(v, "currentcolor") {
			continue
		}
		c, err := parseColor(v)
		if err != nil {
			return nil, fmt.Errorf("svgicon: <%s>: bad color %q", n.name, v)
		}
		return &c, nil
	}
	return nil, nil
}
