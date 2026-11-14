package generic

import (
	"slices"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/icons"
)

func TestAll(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("All() is empty")
	}
	if !slices.IsSortedFunc(all, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) }) {
		t.Error("All() is not sorted by name")
	}
	seen := map[string]bool{}
	for _, e := range all {
		if seen[e.Name] {
			t.Errorf("%s listed twice", e.Name)
		}
		seen[e.Name] = true
		ic := e.Icon
		if ic == nil {
			t.Errorf("%s: nil icon", e.Name)
			continue
		}
		if ic.Kind() != icons.SVG || ic.Name() == "" {
			t.Errorf("%s: kind %v, name %q", e.Name, ic.Kind(), ic.Name())
		}
		if w, h := ic.Size(); w <= 0 || h <= 0 {
			t.Errorf("%s: size %vx%v", e.Name, w, h)
		}
		if !ic.Monochrome() {
			t.Errorf("%s: not monochrome", e.Name)
		}
	}
}
