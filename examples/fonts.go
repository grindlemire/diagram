package examples

import (
	_ "embed"

	"github.com/grindlemire/diagram"
)

// Noto Sans SC Regular and SemiBold (SIL OFL 1.1, fonts/OFL-NotoSansSC.txt),
// cut down with the library's own subsetter to the CJK characters the text
// example uses. The full static fonts come from Google Fonts
// (fonts.google.com/noto/specimen/Noto+Sans+SC, v41 static instances).
//
//go:embed fonts/NotoSansSC-Regular-subset.ttf
var notoSansSCRegular []byte

//go:embed fonts/NotoSansSC-SemiBold-subset.ttf
var notoSansSCSemiBold []byte

// The text example's CJK labels need a fallback font; registering it at
// init shows how a program would set one up. Inter still draws everything
// it has, so other examples are unaffected.
func init() {
	diagram.MustRegisterFont(notoSansSCRegular)
	diagram.MustRegisterFont(notoSansSCSemiBold)
}
