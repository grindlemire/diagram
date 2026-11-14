package themes

import (
	"image/color"
	"testing"
)

func TestPalettesDefineEveryToken(t *testing.T) {
	for _, th := range []*Theme{Light, Dark, Blueprint} {
		for _, tok := range Tokens() {
			if th.Palette[tok] == (color.RGBA{}) {
				t.Errorf("%s: token %s has no colour", th.Name, tok)
			}
		}
	}
}

func TestAuto(t *testing.T) {
	a := Auto(Light, Dark)
	if a.Dark == nil || a.Dark[Accent] != Dark.Palette[Accent] || a.Palette[Accent] != Light.Palette[Accent] {
		t.Fatal("Auto should carry both palettes")
	}
	a.Dark[Accent] = color.RGBA{}
	if Dark.Palette[Accent] == (color.RGBA{}) {
		t.Fatal("Auto must not alias the dark theme's palette")
	}
}

func TestHex(t *testing.T) {
	for in, want := range map[string]color.RGBA{
		"#fff":     {255, 255, 255, 255},
		"#4169E1":  {0x41, 0x69, 0xe1, 255},
		"10203040": {0x10, 0x20, 0x30, 0x40},
	} {
		c := Hex(in)
		if c.Err() != nil || c.Resolve(Light) != want {
			t.Errorf("Hex(%q) = %v, %v; want %v", in, c.Resolve(Light), c.Err(), want)
		}
	}
	for _, bad := range []string{"", "#ff", "#ggg", "#12345"} {
		if Hex(bad).Err() == nil {
			t.Errorf("Hex(%q) should be invalid", bad)
		}
	}
}
