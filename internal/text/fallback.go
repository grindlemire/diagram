package text

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// fallbacks is an immutable snapshot of the registered fonts. Register
// replaces it whole, so readers never lock.
type fallbacks struct {
	families []*family
	// chains[w] is Inter at weight w followed by each family's best match
	// for w.
	chains [2][]*Font
}

// family is the registered fonts that share a name-table family name.
type family struct {
	name  string
	fonts []*Font // registration order
}

var (
	registerMu sync.Mutex
	registry   atomic.Pointer[fallbacks]
)

// Register adds a TrueType font to the fallback chain. Fonts are grouped
// into families by their name-table family name; families come after
// Inter in the order they were first registered. Within a family each
// weight uses the font whose usWeightClass is closest to it (400 for
// Regular, 600 for SemiBold; ties go to the lighter font for Regular and
// the heavier one for SemiBold, then to the earlier registration), so a
// single-weight font serves both weights. Registering the same bytes again
// does nothing.
//
// Only fonts with TrueType (glyf) outlines are accepted: CFF-based
// OpenType (.otf), collections (.ttc), WOFF and colour bitmap or layered
// fonts (CBDT, sbix, COLR, SVG) are rejected, because the SVG subsetter
// and the PNG rasterizer can't draw them.
func Register(data []byte) error {
	load()
	if err := check(data); err != nil {
		return err
	}
	registerMu.Lock()
	defer registerMu.Unlock()
	old := registry.Load()
	for _, fam := range old.families {
		for _, fn := range fam.fonts {
			if bytes.Equal(fn.Data, data) {
				return nil
			}
		}
	}
	fn, err := parse("", bytes.Clone(data))
	if err != nil {
		return fmt.Errorf("not a usable TrueType font: %w", err)
	}
	fn.Weight = weightClass(fn.Data)

	st := &fallbacks{}
	for _, fam := range old.families {
		c := *fam
		if fn.Family == "" && fn.Name != "" && fam.name == fn.Name {
			c.fonts = append(append([]*Font{}, fam.fonts...), fn)
			fn.Family = fam.fonts[0].Family
		}
		st.families = append(st.families, &c)
	}
	if fn.Family == "" {
		fn.Family = fmt.Sprintf("diagram-fallback-%d", len(st.families)+1)
		st.families = append(st.families, &family{name: fn.Name, fonts: []*Font{fn}})
	}
	for w := range st.chains {
		st.chains[w] = []*Font{faces[w].inter}
		for _, fam := range st.families {
			st.chains[w] = append(st.chains[w], fam.pick(Weight(w).CSS()))
		}
	}
	registry.Store(st)
	return nil
}

// pick returns the family's font closest to the CSS weight target.
func (fam *family) pick(target int) *Font {
	best := fam.fonts[0]
	for _, f := range fam.fonts[1:] {
		d, bd := abs(f.Weight-target), abs(best.Weight-target)
		heavier := f.Weight > best.Weight
		if d < bd || d == bd && f.Weight != best.Weight && heavier == (target > 500) {
			best = f
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// check rejects data that the subsetter or rasterizer can't handle.
func check(data []byte) error {
	if len(data) < 12 {
		return errors.New("not a font: too short")
	}
	switch string(data[:4]) {
	case "OTTO":
		return errors.New("CFF-based OpenType (.otf) fonts are not supported; use a TrueType (.ttf, glyf outlines) font")
	case "ttcf":
		return errors.New("font collections (.ttc) are not supported; extract a single .ttf")
	case "wOFF", "wOF2":
		return errors.New("WOFF fonts are not supported; use the .ttf")
	case "\x00\x01\x00\x00", "true":
	default:
		return errors.New("not a TrueType font")
	}
	t, err := parseTables(data)
	if err != nil {
		return trim(err)
	}
	for _, tag := range []string{"CBDT", "sbix", "COLR", "SVG "} {
		if t.tables[tag] != nil {
			return fmt.Errorf("colour emoji fonts (%s table) are not supported; use a monochrome TrueType font", tag)
		}
	}
	// A trial subset proves the subsetter can read every table it needs.
	if _, err := Subset(data, nil); err != nil {
		return trim(err)
	}
	return nil
}

// trim drops the subsetter's package prefix; Register's caller adds its own.
func trim(err error) error {
	return errors.New(strings.TrimPrefix(err.Error(), "text: subset: "))
}

// weightClass returns OS/2 usWeightClass, or 400 if it is missing.
func weightClass(data []byte) int {
	t, err := parseTables(data)
	if err != nil || len(t.tables["OS/2"]) < 6 {
		return 400
	}
	w := int(be.Uint16(t.tables["OS/2"][4:]))
	switch {
	case w == 0:
		return 400
	case w < 10: // some old fonts use 1–9
		return w * 100
	}
	return w
}
