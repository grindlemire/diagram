package pipeline

import (
	"fmt"
	"strings"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/text"
)

// Glyphs returns one error per element or edge whose text has characters
// that neither Inter nor any registered fallback font can draw, so they
// never render as '?'. Invisible groups draw no header and are skipped.
// Badges count as a node's text, and when the diagram has a legend, so do
// the names of its named styles.
func Glyphs(m *model.Diagram) []error {
	var errs []error
	report := func(src model.Src, what string, texts ...string) {
		var missing []string
		seen := map[rune]bool{}
		for _, s := range texts {
			for _, r := range text.Missing(s) {
				if !seen[r] {
					seen[r] = true
					missing = append(missing, fmt.Sprintf("'%c' (U+%04X)", r, r))
				}
			}
		}
		if len(missing) > 0 {
			errs = append(errs, fmt.Errorf("%s: %s: no font has a glyph for %s; register one with diagram.RegisterFont",
				src, what, strings.Join(missing, ", ")))
		}
	}
	legend := false
	model.Walk(m.Root, func(e *model.Element) {
		legend = legend || e.Legend
		if e == m.Root || e.Kind.IsGroup() && !e.Visible() {
			return
		}
		report(e.Src, e.Describe(), append([]string{e.Label, e.Desc}, e.Badges...)...)
	})
	for _, e := range m.Edges {
		report(e.Src, e.Describe(), e.Label)
	}
	// A legend draws the name of every named style.
	if legend {
		for _, ns := range m.Named {
			report(ns.Src, fmt.Sprintf("named style %q", ns.Name), ns.Name)
		}
	}
	return errs
}
