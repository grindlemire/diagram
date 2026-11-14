package themes

import "image/color"

// Palette maps every token to a concrete colour.
type Palette [numTokens]color.RGBA

// Theme holds the palette, typography, spacing and default looks that a
// diagram uses for anything its code doesn't set.
type Theme struct {
	Name    string
	Palette Palette

	// Dark, when set, is the palette SVG output switches to under
	// prefers-color-scheme: dark. Build it with Auto.
	Dark *Palette

	// Typography, in pixels.
	FontFamily     string
	FontSize       float64 // node labels
	DescFontSize   float64 // node descriptions
	HeaderFontSize float64 // group headers
	LabelFontSize  float64 // edge labels
	LineHeight     float64 // multiple of font size

	// Nodes.
	NodePaddingX float64
	NodePaddingY float64
	NodeRadius   float64
	NodeMinWidth float64
	NodeMaxWidth float64 // labels wrap at this width (minus padding)
	NodeStroke   float64
	IconSize     float64
	IconGap      float64 // between icon and text
	DescGap      float64 // between label and description

	// Groups and flow.
	Gap          float64 // between siblings
	GroupPadding float64
	GroupRadius  float64
	GroupStroke  float64
	HeaderGap    float64 // between header and children

	// Edges.
	EdgeWidth      float64
	EdgeCornerRad  float64 // rounding at orthogonal bends
	ArrowSize      float64
	LabelPaddingX  float64
	LabelPaddingY  float64
	PortSpacing    float64 // minimum distance between edges sharing a side
	EdgeClearance  float64 // how far routes keep from obstacles
	ParallelOffset float64

	// Default colours.
	NodeFill      Color
	NodeBorder    Color
	NodeText      Color
	DescText      Color
	IconColor     Color
	GroupFills    []Color // by depth, outermost first; the last repeats
	GroupBorder   Color
	GroupText     Color
	EdgeColor     Color
	EdgeLabelText Color
	EdgeLabelFill Color
}

// Light is the default theme: white surfaces, slate greys and an indigo
// accent.
var Light = &Theme{
	Name: "light",
	Palette: Palette{
		Background: hex(0xffffff),
		Surface:    hex(0xffffff),
		Foreground: hex(0x0f172a),
		Muted:      hex(0x64748b),
		Border:     hex(0xcbd5e1),
		Accent:     hex(0x4f46e5),
		Level1:     hex(0xf8fafc),
		Level2:     hex(0xf1f5f9),
		Level3:     hex(0xe2e8f0),
		Red:        hex(0xdc2626),
		Orange:     hex(0xea580c),
		Yellow:     hex(0xca8a04),
		Green:      hex(0x16a34a),
		Teal:       hex(0x0d9488),
		Blue:       hex(0x2563eb),
		Purple:     hex(0x9333ea),
		Pink:       hex(0xdb2777),
		Gray:       hex(0x6b7280),
	},
	FontFamily:     "Inter",
	FontSize:       14,
	DescFontSize:   12,
	HeaderFontSize: 13,
	LabelFontSize:  11,
	LineHeight:     1.3,

	NodePaddingX: 16,
	NodePaddingY: 12,
	NodeRadius:   8,
	NodeMinWidth: 96,
	NodeMaxWidth: 200,
	NodeStroke:   1,
	IconSize:     32,
	IconGap:      8,
	DescGap:      2,

	Gap:          40,
	GroupPadding: 20,
	GroupRadius:  12,
	GroupStroke:  1,
	HeaderGap:    16,

	EdgeWidth:      1.5,
	EdgeCornerRad:  6,
	ArrowSize:      8,
	LabelPaddingX:  6,
	LabelPaddingY:  2,
	PortSpacing:    12,
	EdgeClearance:  12,
	ParallelOffset: 14,

	NodeFill:      Of(Surface),
	NodeBorder:    Of(Border),
	NodeText:      Of(Foreground),
	DescText:      Of(Muted),
	IconColor:     Of(Accent),
	GroupFills:    []Color{Of(Level1), Of(Level2), Of(Level3)},
	GroupBorder:   Of(Border),
	GroupText:     Of(Foreground),
	EdgeColor:     Of(Muted),
	EdgeLabelText: Of(Muted),
	EdgeLabelFill: Of(Background),
}

// GroupFill returns the background for a group at the given nesting depth,
// where 0 is a top-level group.
func (t *Theme) GroupFill(depth int) Color {
	if len(t.GroupFills) == 0 {
		return Color{}
	}
	if depth >= len(t.GroupFills) {
		depth = len(t.GroupFills) - 1
	}
	return t.GroupFills[depth]
}

func hex(v uint32) color.RGBA {
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
}
