package text

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"sort"
)

// Subset returns a TrueType font containing only the glyphs needed to draw
// runes (plus .notdef). The result must load in browsers (Chrome's OTS
// sanitizer is strict) and in golang.org/x/image/font/sfnt.
//
// Kept glyphs are renumbered densely: .notdef stays 0 and the rest follow
// in their original order, with composite components pulled in. Layout
// tables (GSUB, GPOS, GDEF, ...) are dropped, so the subset renders
// unkerned text, which matches Face.Width. Runes the font lacks are
// skipped. The output is deterministic.
func Subset(data []byte, runes []rune) ([]byte, error) {
	f, err := parseTables(data)
	if err != nil {
		return nil, err
	}
	for _, tag := range []string{"head", "hhea", "hmtx", "maxp", "loca", "glyf", "cmap"} {
		if f.tables[tag] == nil {
			return nil, fmt.Errorf("text: subset: missing %q table", tag)
		}
	}
	head, hhea, maxp := f.tables["head"], f.tables["hhea"], f.tables["maxp"]
	if len(head) < 54 || len(hhea) < 36 || len(maxp) < 6 {
		return nil, errors.New("text: subset: truncated head, hhea or maxp")
	}
	numGlyphs := int(be.Uint16(maxp[4:]))
	numHMetrics := int(be.Uint16(hhea[34:]))
	if numGlyphs == 0 || numHMetrics == 0 || numHMetrics > numGlyphs {
		return nil, errors.New("text: subset: bad glyph counts")
	}
	if len(f.tables["hmtx"]) < 4*numHMetrics+2*(numGlyphs-numHMetrics) {
		return nil, errors.New("text: subset: truncated hmtx")
	}
	glyphs, err := splitGlyphs(f.tables["glyf"], f.tables["loca"], numGlyphs, be.Uint16(head[50:]) == 1)
	if err != nil {
		return nil, err
	}
	cmap, err := parseCmap(f.tables["cmap"])
	if err != nil {
		return nil, err
	}

	// Runes to keep, sorted and deduplicated, with their old glyph IDs.
	var mapped []runeGlyph
	keep := map[int]bool{0: true}
	sorted := slices.Clone(runes)
	slices.Sort(sorted)
	for i, r := range sorted {
		if i > 0 && r == sorted[i-1] {
			continue
		}
		g, ok := cmap[r]
		if !ok || g == 0 || int(g) >= numGlyphs {
			continue
		}
		mapped = append(mapped, runeGlyph{r, g})
		keep[int(g)] = true
	}
	// Composite closure.
	queue := make([]int, 0, len(keep))
	for g := range keep {
		queue = append(queue, g)
	}
	for len(queue) > 0 {
		g := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		comps, err := components(glyphs[g])
		if err != nil {
			return nil, fmt.Errorf("text: subset: glyph %d: %w", g, err)
		}
		for _, c := range comps {
			if c >= numGlyphs {
				return nil, fmt.Errorf("text: subset: glyph %d references glyph %d", g, c)
			}
			if !keep[c] {
				keep[c] = true
				queue = append(queue, c)
			}
		}
	}
	oldIDs := make([]int, 0, len(keep))
	for g := range keep {
		oldIDs = append(oldIDs, g)
	}
	sort.Ints(oldIDs) // oldIDs[new] = old; .notdef stays first
	newID := make(map[int]uint16, len(oldIDs))
	for n, o := range oldIDs {
		newID[o] = uint16(n)
	}

	// glyf and loca. Glyphs are padded to 4 bytes.
	var glyf []byte
	offsets := make([]int, 0, len(oldIDs)+1)
	for _, o := range oldIDs {
		offsets = append(offsets, len(glyf))
		g, err := remapComponents(glyphs[o], newID)
		if err != nil {
			return nil, fmt.Errorf("text: subset: glyph %d: %w", o, err)
		}
		glyf = append(glyf, g...)
		glyf = pad4(glyf)
	}
	offsets = append(offsets, len(glyf))
	shortLoca := len(glyf)/2 <= 0xFFFF
	var loca []byte
	for _, off := range offsets {
		if shortLoca {
			loca = be.AppendUint16(loca, uint16(off/2))
		} else {
			loca = be.AppendUint32(loca, uint32(off))
		}
	}

	// hmtx: drop the trailing run of equal advances into the lsb-only part.
	hmtxIn := f.tables["hmtx"]
	metric := func(g int) (adv, lsb uint16) {
		if g < numHMetrics {
			return be.Uint16(hmtxIn[4*g:]), be.Uint16(hmtxIn[4*g+2:])
		}
		return be.Uint16(hmtxIn[4*(numHMetrics-1):]), be.Uint16(hmtxIn[4*numHMetrics+2*(g-numHMetrics):])
	}
	advs := make([]uint16, len(oldIDs))
	lsbs := make([]uint16, len(oldIDs))
	for n, o := range oldIDs {
		advs[n], lsbs[n] = metric(o)
	}
	nh := len(advs)
	for nh > 1 && advs[nh-2] == advs[nh-1] {
		nh--
	}
	var hmtx []byte
	for n := range advs {
		if n < nh {
			hmtx = be.AppendUint16(hmtx, advs[n])
		}
		hmtx = be.AppendUint16(hmtx, lsbs[n])
	}

	out := map[string][]byte{
		"cmap": buildCmap(mapped, newID),
		"glyf": glyf,
		"hmtx": hmtx,
		"loca": loca,
	}

	h := slices.Clone(head)
	clear(h[8:12]) // checkSumAdjustment, set at the end
	if shortLoca {
		be.PutUint16(h[50:], 0)
	} else {
		be.PutUint16(h[50:], 1)
	}
	out["head"] = h

	hh := slices.Clone(hhea)
	be.PutUint16(hh[34:], uint16(nh))
	out["hhea"] = hh

	mp := slices.Clone(maxp)
	be.PutUint16(mp[4:], uint16(len(oldIDs)))
	out["maxp"] = mp

	if post := f.tables["post"]; len(post) >= 32 {
		p := slices.Clone(post[:32])
		be.PutUint32(p[0:], 0x00030000) // no glyph names
		out["post"] = p
	}

	if os2 := f.tables["OS/2"]; len(os2) >= 68 {
		o := slices.Clone(os2)
		first, last := uint16(0xFFFF), uint16(0)
		for _, m := range mapped {
			c := uint16(min(m.r, 0xFFFF))
			first, last = min(first, c), max(last, c)
		}
		if len(mapped) == 0 {
			first, last = 0, 0
		}
		be.PutUint16(o[64:], first) // usFirstCharIndex
		be.PutUint16(o[66:], last)  // usLastCharIndex
		out["OS/2"] = o
	}

	for _, tag := range []string{"name", "cvt ", "fpgm", "prep", "gasp"} {
		if t := f.tables[tag]; t != nil {
			out[tag] = t
		}
	}
	return writeFont(out), nil
}

var be = binary.BigEndian

type runeGlyph struct {
	r rune
	g uint16
}

type fontTables struct {
	tables map[string][]byte
}

func parseTables(data []byte) (*fontTables, error) {
	if len(data) < 12 {
		return nil, errors.New("text: subset: font too short")
	}
	if v := be.Uint32(data); v != 0x00010000 && v != 0x74727565 { // 'true'
		return nil, fmt.Errorf("text: subset: unsupported sfnt version %#x (TrueType outlines required)", v)
	}
	n := int(be.Uint16(data[4:]))
	if len(data) < 12+16*n {
		return nil, errors.New("text: subset: truncated table directory")
	}
	f := &fontTables{tables: map[string][]byte{}}
	for i := range n {
		rec := data[12+16*i:]
		tag := string(rec[:4])
		off, length := uint64(be.Uint32(rec[8:])), uint64(be.Uint32(rec[12:]))
		if off+length > uint64(len(data)) {
			return nil, fmt.Errorf("text: subset: table %q out of bounds", tag)
		}
		f.tables[tag] = data[off : off+length]
	}
	return f, nil
}

func splitGlyphs(glyf, loca []byte, numGlyphs int, long bool) ([][]byte, error) {
	off := func(i int) int {
		if long {
			return int(be.Uint32(loca[4*i:]))
		}
		return 2 * int(be.Uint16(loca[2*i:]))
	}
	size := 2
	if long {
		size = 4
	}
	if len(loca) < size*(numGlyphs+1) {
		return nil, errors.New("text: subset: truncated loca")
	}
	glyphs := make([][]byte, numGlyphs)
	for i := range numGlyphs {
		a, b := off(i), off(i+1)
		if a > b || b > len(glyf) {
			return nil, fmt.Errorf("text: subset: bad loca entry for glyph %d", i)
		}
		glyphs[i] = glyf[a:b]
	}
	return glyphs, nil
}

// parseCmap reads the best Unicode subtable: format 12 if present, else
// format 4.
func parseCmap(t []byte) (map[rune]uint16, error) {
	if len(t) < 4 {
		return nil, errors.New("text: subset: truncated cmap")
	}
	n := int(be.Uint16(t[2:]))
	if len(t) < 4+8*n {
		return nil, errors.New("text: subset: truncated cmap")
	}
	var best []byte
	bestRank := 0
	for i := range n {
		rec := t[4+8*i:]
		pid, eid, off := be.Uint16(rec), be.Uint16(rec[2:]), int(be.Uint32(rec[4:]))
		if off+2 > len(t) {
			continue
		}
		sub := t[off:]
		format := be.Uint16(sub)
		unicode := pid == 0 || (pid == 3 && (eid == 1 || eid == 10))
		rank := 0
		switch {
		case unicode && format == 12:
			rank = 2
		case unicode && format == 4:
			rank = 1
		}
		if rank > bestRank {
			best, bestRank = sub, rank
		}
	}
	m := map[rune]uint16{}
	switch bestRank {
	case 2:
		if len(best) < 16 {
			return nil, errors.New("text: subset: truncated cmap format 12")
		}
		ng := int(be.Uint32(best[12:]))
		if len(best) < 16+12*ng {
			return nil, errors.New("text: subset: truncated cmap format 12")
		}
		for i := range ng {
			g := best[16+12*i:]
			start, end, gid := be.Uint32(g), be.Uint32(g[4:]), be.Uint32(g[8:])
			if end < start || end > 0x10FFFF {
				continue
			}
			for c := start; c <= end; c++ {
				if id := gid + (c - start); id <= 0xFFFF {
					m[rune(c)] = uint16(id)
				}
			}
		}
	case 1:
		if len(best) < 14 {
			return nil, errors.New("text: subset: truncated cmap format 4")
		}
		segX2 := int(be.Uint16(best[6:]))
		segs := segX2 / 2
		ends := 14
		starts := ends + segX2 + 2
		deltas := starts + segX2
		ranges := deltas + segX2
		if len(best) < ranges+segX2 {
			return nil, errors.New("text: subset: truncated cmap format 4")
		}
		for s := range segs {
			end := int(be.Uint16(best[ends+2*s:]))
			start := int(be.Uint16(best[starts+2*s:]))
			delta := be.Uint16(best[deltas+2*s:])
			ro := int(be.Uint16(best[ranges+2*s:]))
			for c := start; c <= end && c != 0xFFFF; c++ {
				var g uint16
				if ro == 0 {
					g = uint16(c) + delta
				} else {
					p := ranges + 2*s + ro + 2*(c-start)
					if p+2 > len(best) {
						continue
					}
					if g = be.Uint16(best[p:]); g != 0 {
						g += delta
					}
				}
				if g != 0 {
					m[rune(c)] = g
				}
			}
		}
	default:
		return nil, errors.New("text: subset: no Unicode cmap subtable")
	}
	return m, nil
}

// Composite glyph flags.
const (
	argsAreWords    = 0x0001
	haveScale       = 0x0008
	moreComponents  = 0x0020
	haveXYScale     = 0x0040
	haveTwoByTwo    = 0x0080
	compositeHeader = 10
)

// componentOffsets returns the byte offsets of each component's glyph index
// in a composite glyph, or nil for a simple or empty glyph.
func componentOffsets(g []byte) ([]int, error) {
	if len(g) < compositeHeader || int16(be.Uint16(g)) >= 0 {
		return nil, nil
	}
	var offs []int
	p := compositeHeader
	for {
		if p+4 > len(g) {
			return nil, errors.New("truncated composite glyph")
		}
		flags := be.Uint16(g[p:])
		offs = append(offs, p+2)
		p += 4
		if flags&argsAreWords != 0 {
			p += 4
		} else {
			p += 2
		}
		switch {
		case flags&haveScale != 0:
			p += 2
		case flags&haveXYScale != 0:
			p += 4
		case flags&haveTwoByTwo != 0:
			p += 8
		}
		if flags&moreComponents == 0 {
			break
		}
	}
	if p > len(g) {
		return nil, errors.New("truncated composite glyph")
	}
	return offs, nil
}

func components(g []byte) ([]int, error) {
	offs, err := componentOffsets(g)
	if err != nil {
		return nil, err
	}
	ids := make([]int, len(offs))
	for i, o := range offs {
		ids[i] = int(be.Uint16(g[o:]))
	}
	return ids, nil
}

// remapComponents returns g with composite component IDs renumbered.
func remapComponents(g []byte, newID map[int]uint16) ([]byte, error) {
	offs, err := componentOffsets(g)
	if err != nil || offs == nil {
		return g, err
	}
	g = slices.Clone(g)
	for _, o := range offs {
		be.PutUint16(g[o:], newID[int(be.Uint16(g[o:]))])
	}
	return g, nil
}

// buildCmap writes a cmap with a format 4 subtable (platform 0 encoding 3,
// platform 3 encoding 1) and, when a rune is outside the BMP, a format 12
// subtable (platform 0 encoding 4, platform 3 encoding 10). mapped is
// sorted by rune.
func buildCmap(mapped []runeGlyph, newID map[int]uint16) []byte {
	type pair struct {
		c uint32
		g uint16
	}
	var all, bmp []pair
	for _, m := range mapped {
		p := pair{uint32(m.r), newID[int(m.g)]}
		all = append(all, p)
		if m.r < 0xFFFF {
			bmp = append(bmp, p)
		}
	}
	type run struct {
		start, end uint32
		g          uint16
	}
	runs := func(ps []pair) []run {
		var rs []run
		for _, p := range ps {
			if n := len(rs); n > 0 && rs[n-1].end+1 == p.c && uint32(rs[n-1].g)+p.c-rs[n-1].start == uint32(p.g) {
				rs[n-1].end = p.c
				continue
			}
			rs = append(rs, run{p.c, p.c, p.g})
		}
		return rs
	}

	// Format 4.
	segs := append(runs(bmp), run{0xFFFF, 0xFFFF, 0}) // 0xFFFF -> glyph 0
	n := len(segs)
	sr := 1
	es := 0
	for sr*2 <= n {
		sr *= 2
		es++
	}
	var f4 []byte
	f4 = be.AppendUint16(f4, 4)
	f4 = be.AppendUint16(f4, uint16(16+8*n))
	f4 = be.AppendUint16(f4, 0) // language
	f4 = be.AppendUint16(f4, uint16(2*n))
	f4 = be.AppendUint16(f4, uint16(2*sr))
	f4 = be.AppendUint16(f4, uint16(es))
	f4 = be.AppendUint16(f4, uint16(2*n-2*sr))
	for _, s := range segs {
		f4 = be.AppendUint16(f4, uint16(s.end))
	}
	f4 = be.AppendUint16(f4, 0) // reservedPad
	for _, s := range segs {
		f4 = be.AppendUint16(f4, uint16(s.start))
	}
	for _, s := range segs {
		f4 = be.AppendUint16(f4, s.g-uint16(s.start)) // idDelta, mod 65536
	}
	for range segs {
		f4 = be.AppendUint16(f4, 0) // idRangeOffset
	}

	// Format 12, only when needed.
	var f12 []byte
	if len(all) > 0 && all[len(all)-1].c > 0xFFFF {
		groups := runs(all)
		f12 = be.AppendUint16(f12, 12)
		f12 = be.AppendUint16(f12, 0)
		f12 = be.AppendUint32(f12, uint32(16+12*len(groups)))
		f12 = be.AppendUint32(f12, 0) // language
		f12 = be.AppendUint32(f12, uint32(len(groups)))
		for _, g := range groups {
			f12 = be.AppendUint32(f12, g.start)
			f12 = be.AppendUint32(f12, g.end)
			f12 = be.AppendUint32(f12, uint32(g.g))
		}
	}

	type rec struct {
		pid, eid uint16
		sub      []byte
	}
	recs := []rec{{0, 3, f4}}
	if f12 != nil {
		recs = append(recs, rec{0, 4, f12})
	}
	recs = append(recs, rec{3, 1, f4})
	if f12 != nil {
		recs = append(recs, rec{3, 10, f12})
	}
	f4Off := 4 + 8*len(recs)
	f12Off := f4Off + len(f4)
	var t []byte
	t = be.AppendUint16(t, 0)
	t = be.AppendUint16(t, uint16(len(recs)))
	for _, r := range recs {
		t = be.AppendUint16(t, r.pid)
		t = be.AppendUint16(t, r.eid)
		if be.Uint16(r.sub) == 4 {
			t = be.AppendUint32(t, uint32(f4Off))
		} else {
			t = be.AppendUint32(t, uint32(f12Off))
		}
	}
	t = append(t, f4...)
	t = append(t, f12...)
	return t
}

func pad4(b []byte) []byte {
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func checksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		sum += be.Uint32(w[:])
	}
	return sum
}

// writeFont assembles tables (head's checkSumAdjustment zeroed) into an
// sfnt file with a sorted directory, then sets checkSumAdjustment.
func writeFont(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	n := len(tags)
	sr, es := 1, 0
	for sr*2 <= n {
		sr *= 2
		es++
	}
	var out []byte
	out = be.AppendUint32(out, 0x00010000)
	out = be.AppendUint16(out, uint16(n))
	out = be.AppendUint16(out, uint16(16*sr))
	out = be.AppendUint16(out, uint16(es))
	out = be.AppendUint16(out, uint16(16*n-16*sr))
	off := 12 + 16*n
	headOff := 0
	for _, tag := range tags {
		t := tables[tag]
		if tag == "head" {
			headOff = off
		}
		out = append(out, tag...)
		out = be.AppendUint32(out, checksum(t))
		out = be.AppendUint32(out, uint32(off))
		out = be.AppendUint32(out, uint32(len(t)))
		off += (len(t) + 3) &^ 3
	}
	for _, tag := range tags {
		out = pad4(append(out, tables[tag]...))
	}
	be.PutUint32(out[headOff+8:], 0xB1B0AFBA-checksum(out))
	return out
}
