package themes

// Dark is a dark theme: slate surfaces with brighter accents.
var Dark = derive(Light, "dark", Palette{
	Background: hex(0x0b1120),
	Surface:    hex(0x131c31),
	Foreground: hex(0xe2e8f0),
	Muted:      hex(0x94a3b8),
	Border:     hex(0x334155),
	Accent:     hex(0x818cf8),
	Level1:     hex(0x0f172a),
	Level2:     hex(0x162036),
	Level3:     hex(0x1e2a44),
	Red:        hex(0xf87171),
	Orange:     hex(0xfb923c),
	Yellow:     hex(0xfacc15),
	Green:      hex(0x4ade80),
	Teal:       hex(0x2dd4bf),
	Blue:       hex(0x60a5fa),
	Purple:     hex(0xc084fc),
	Pink:       hex(0xf472b6),
	Gray:       hex(0x9ca3af),
})

// Blueprint is white linework on deep blue.
var Blueprint = derive(Light, "blueprint", Palette{
	Background: hex(0x123a6b),
	Surface:    hex(0x174577),
	Foreground: hex(0xf8fafc),
	Muted:      hex(0xb7cdea),
	Border:     hex(0x8fb3e3),
	Accent:     hex(0xffffff),
	Level1:     hex(0x154070),
	Level2:     hex(0x19497d),
	Level3:     hex(0x1d528a),
	Red:        hex(0xff8a8a),
	Orange:     hex(0xffb36b),
	Yellow:     hex(0xffe06b),
	Green:      hex(0x7ee2a1),
	Teal:       hex(0x6be3d3),
	Blue:       hex(0x9cc9ff),
	Purple:     hex(0xd3a8ff),
	Pink:       hex(0xff9ccc),
	Gray:       hex(0xc4ccd8),
})

func derive(base *Theme, name string, p Palette) *Theme {
	t := *base
	t.Name = name
	t.Palette = p
	t.GroupFills = append([]Color(nil), base.GroupFills...)
	return &t
}

// Auto returns a theme that uses light's palette and switches to dark's
// palette when the viewer prefers a dark colour scheme. Only SVG output
// can switch; PNG and draw.io output use the light palette. Everything but
// the palette comes from light.
func Auto(light, dark *Theme) *Theme {
	t := *light
	t.Name = light.Name + "+" + dark.Name
	p := dark.Palette
	t.Dark = &p
	t.GroupFills = append([]Color(nil), light.GroupFills...)
	return &t
}
