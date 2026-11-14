package svgicon

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// rgba is an opaque colour plus a separate alpha in [0, 1].
type rgba struct {
	c color.RGBA // A is always 255
	a float64
}

// parseColor parses a CSS colour: #rgb, #rgba, #rrggbb, #rrggbbaa, rgb(),
// rgba(), hsl(), hsla(), a named colour, or transparent. currentColor is
// handled by callers.
func parseColor(s string) (rgba, error) {
	s = strings.TrimSpace(s)
	ls := strings.ToLower(s)
	switch {
	case strings.HasPrefix(ls, "#"):
		return parseHex(ls[1:])
	case strings.HasPrefix(ls, "rgb(") || strings.HasPrefix(ls, "rgba("):
		return parseFunc(ls, false)
	case strings.HasPrefix(ls, "hsl(") || strings.HasPrefix(ls, "hsla("):
		return parseFunc(ls, true)
	case ls == "transparent":
		return rgba{c: color.RGBA{0, 0, 0, 255}, a: 0}, nil
	}
	if v, ok := namedColors[ls]; ok {
		return rgba{c: color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, a: 1}, nil
	}
	return rgba{}, fmt.Errorf("bad colour %q", s)
}

func parseHex(h string) (rgba, error) {
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return rgba{}, fmt.Errorf("bad colour %q", "#"+h)
	}
	var r, g, b, a uint64
	switch len(h) {
	case 3:
		r, g, b, a = (v>>8&0xf)*17, (v>>4&0xf)*17, (v&0xf)*17, 255
	case 4:
		r, g, b, a = (v>>12&0xf)*17, (v>>8&0xf)*17, (v>>4&0xf)*17, (v&0xf)*17
	case 6:
		r, g, b, a = v>>16&0xff, v>>8&0xff, v&0xff, 255
	case 8:
		r, g, b, a = v>>24&0xff, v>>16&0xff, v>>8&0xff, v&0xff
	default:
		return rgba{}, fmt.Errorf("bad colour %q", "#"+h)
	}
	return rgba{c: color.RGBA{uint8(r), uint8(g), uint8(b), 255}, a: float64(a) / 255}, nil
}

// parseFunc parses rgb()/rgba() or hsl()/hsla() in either the legacy comma
// syntax or the CSS 4 space syntax with an optional "/ alpha".
func parseFunc(s string, hsl bool) (rgba, error) {
	bad := func() (rgba, error) { return rgba{}, fmt.Errorf("bad colour %q", s) }
	open := strings.IndexByte(s, '(')
	if !strings.HasSuffix(s, ")") {
		return bad()
	}
	inner := s[open+1 : len(s)-1]
	var parts []string
	alpha := "1"
	if strings.Contains(inner, ",") {
		parts = strings.Split(inner, ",")
		if len(parts) == 4 {
			alpha = parts[3]
			parts = parts[:3]
		}
	} else {
		main, a, hasAlpha := strings.Cut(inner, "/")
		if hasAlpha {
			alpha = a
		}
		parts = strings.Fields(main)
	}
	if len(parts) != 3 {
		return bad()
	}
	a, err := fraction(alpha)
	if err != nil {
		return bad()
	}
	var v [3]float64
	for i, p := range parts {
		p = strings.TrimSpace(p)
		pct := strings.HasSuffix(p, "%")
		if hsl && i == 0 {
			p = strings.TrimSuffix(p, "deg")
		}
		x, err := parseNumber(strings.TrimSuffix(p, "%"))
		if err != nil {
			return bad()
		}
		switch {
		case hsl && i == 0:
			if pct {
				return bad()
			}
		case hsl:
			x = clamp01(x / 100) // CSS 4 allows bare numbers as percentages
		case pct:
			x = clamp01(x/100) * 255
		default:
			x = math.Max(0, math.Min(255, x))
		}
		v[i] = x
	}
	if hsl {
		r, g, b := hslToRGB(v[0], v[1], v[2])
		v = [3]float64{r * 255, g * 255, b * 255}
	}
	return rgba{c: color.RGBA{uint8(math.Round(v[0])), uint8(math.Round(v[1])), uint8(math.Round(v[2])), 255}, a: a}, nil
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	f := func(n float64) float64 {
		k := math.Mod(n+h/30, 12)
		a := s * math.Min(l, 1-l)
		return l - a*math.Max(-1, math.Min(k-3, math.Min(9-k, 1)))
	}
	return f(0), f(8), f(4)
}

// namedColors is the CSS Color 4 named-colour list.
var namedColors = map[string]uint32{
	"aliceblue": 0xf0f8ff, "antiquewhite": 0xfaebd7, "aqua": 0x00ffff, "aquamarine": 0x7fffd4,
	"azure": 0xf0ffff, "beige": 0xf5f5dc, "bisque": 0xffe4c4, "black": 0x000000,
	"blanchedalmond": 0xffebcd, "blue": 0x0000ff, "blueviolet": 0x8a2be2, "brown": 0xa52a2a,
	"burlywood": 0xdeb887, "cadetblue": 0x5f9ea0, "chartreuse": 0x7fff00, "chocolate": 0xd2691e,
	"coral": 0xff7f50, "cornflowerblue": 0x6495ed, "cornsilk": 0xfff8dc, "crimson": 0xdc143c,
	"cyan": 0x00ffff, "darkblue": 0x00008b, "darkcyan": 0x008b8b, "darkgoldenrod": 0xb8860b,
	"darkgray": 0xa9a9a9, "darkgreen": 0x006400, "darkgrey": 0xa9a9a9, "darkkhaki": 0xbdb76b,
	"darkmagenta": 0x8b008b, "darkolivegreen": 0x556b2f, "darkorange": 0xff8c00, "darkorchid": 0x9932cc,
	"darkred": 0x8b0000, "darksalmon": 0xe9967a, "darkseagreen": 0x8fbc8f, "darkslateblue": 0x483d8b,
	"darkslategray": 0x2f4f4f, "darkslategrey": 0x2f4f4f, "darkturquoise": 0x00ced1, "darkviolet": 0x9400d3,
	"deeppink": 0xff1493, "deepskyblue": 0x00bfff, "dimgray": 0x696969, "dimgrey": 0x696969,
	"dodgerblue": 0x1e90ff, "firebrick": 0xb22222, "floralwhite": 0xfffaf0, "forestgreen": 0x228b22,
	"fuchsia": 0xff00ff, "gainsboro": 0xdcdcdc, "ghostwhite": 0xf8f8ff, "gold": 0xffd700,
	"goldenrod": 0xdaa520, "gray": 0x808080, "green": 0x008000, "greenyellow": 0xadff2f,
	"grey": 0x808080, "honeydew": 0xf0fff0, "hotpink": 0xff69b4, "indianred": 0xcd5c5c,
	"indigo": 0x4b0082, "ivory": 0xfffff0, "khaki": 0xf0e68c, "lavender": 0xe6e6fa,
	"lavenderblush": 0xfff0f5, "lawngreen": 0x7cfc00, "lemonchiffon": 0xfffacd, "lightblue": 0xadd8e6,
	"lightcoral": 0xf08080, "lightcyan": 0xe0ffff, "lightgoldenrodyellow": 0xfafad2, "lightgray": 0xd3d3d3,
	"lightgreen": 0x90ee90, "lightgrey": 0xd3d3d3, "lightpink": 0xffb6c1, "lightsalmon": 0xffa07a,
	"lightseagreen": 0x20b2aa, "lightskyblue": 0x87cefa, "lightslategray": 0x778899, "lightslategrey": 0x778899,
	"lightsteelblue": 0xb0c4de, "lightyellow": 0xffffe0, "lime": 0x00ff00, "limegreen": 0x32cd32,
	"linen": 0xfaf0e6, "magenta": 0xff00ff, "maroon": 0x800000, "mediumaquamarine": 0x66cdaa,
	"mediumblue": 0x0000cd, "mediumorchid": 0xba55d3, "mediumpurple": 0x9370db, "mediumseagreen": 0x3cb371,
	"mediumslateblue": 0x7b68ee, "mediumspringgreen": 0x00fa9a, "mediumturquoise": 0x48d1cc, "mediumvioletred": 0xc71585,
	"midnightblue": 0x191970, "mintcream": 0xf5fffa, "mistyrose": 0xffe4e1, "moccasin": 0xffe4b5,
	"navajowhite": 0xffdead, "navy": 0x000080, "oldlace": 0xfdf5e6, "olive": 0x808000,
	"olivedrab": 0x6b8e23, "orange": 0xffa500, "orangered": 0xff4500, "orchid": 0xda70d6,
	"palegoldenrod": 0xeee8aa, "palegreen": 0x98fb98, "paleturquoise": 0xafeeee, "palevioletred": 0xdb7093,
	"papayawhip": 0xffefd5, "peachpuff": 0xffdab9, "peru": 0xcd853f, "pink": 0xffc0cb,
	"plum": 0xdda0dd, "powderblue": 0xb0e0e6, "purple": 0x800080, "rebeccapurple": 0x663399,
	"red": 0xff0000, "rosybrown": 0xbc8f8f, "royalblue": 0x4169e1, "saddlebrown": 0x8b4513,
	"salmon": 0xfa8072, "sandybrown": 0xf4a460, "seagreen": 0x2e8b57, "seashell": 0xfff5ee,
	"sienna": 0xa0522d, "silver": 0xc0c0c0, "skyblue": 0x87ceeb, "slateblue": 0x6a5acd,
	"slategray": 0x708090, "slategrey": 0x708090, "snow": 0xfffafa, "springgreen": 0x00ff7f,
	"steelblue": 0x4682b4, "tan": 0xd2b48c, "teal": 0x008080, "thistle": 0xd8bfd8,
	"tomato": 0xff6347, "turquoise": 0x40e0d0, "violet": 0xee82ee, "wheat": 0xf5deb3,
	"white": 0xffffff, "whitesmoke": 0xf5f5f5, "yellow": 0xffff00, "yellowgreen": 0x9acd32,
}
