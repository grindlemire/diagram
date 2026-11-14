package diagram

import (
	"math"
	"testing"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

func lay(t *testing.T, d *Diagram) *layout.Result {
	t.Helper()
	res, err := d.lay(defaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// checkGeometry asserts invariants that hold for every diagram: children
// sit inside their parent's content box, and siblings in flow and grid
// containers never overlap.
func checkGeometry(t *testing.T, res *layout.Result) {
	t.Helper()
	for _, b := range append([]*layout.Box{res.Root}, res.Boxes...) {
		if b.El.Kind == model.KindNode {
			continue
		}
		var kids []*layout.Box
		for _, c := range b.El.Children {
			k := res.ByEl[c]
			kids = append(kids, k)
			if !b.Content.Contains(k.Rect) {
				t.Errorf("%s at %+v is outside %s's content %+v", c.Describe(), k.Rect, b.El.Describe(), b.Content)
			}
		}
		if b.El.Kind == model.KindCanvas {
			continue
		}
		for i := range kids {
			for j := i + 1; j < len(kids); j++ {
				if kids[i].Rect.Overlaps(kids[j].Rect) {
					t.Errorf("%s %+v overlaps %s %+v", kids[i].El.Describe(), kids[i].Rect, kids[j].El.Describe(), kids[j].Rect)
				}
			}
		}
	}
	if !(layout.Rect{W: res.Width, H: res.Height}).Contains(res.Root.Rect) {
		t.Errorf("root %+v exceeds diagram %vx%v", res.Root.Rect, res.Width, res.Height)
	}
}

func TestLayoutInvariantsOnCases(t *testing.T) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { checkGeometry(t, lay(t, c.build())) })
	}
}

func TestRowAndColumnOrder(t *testing.T) {
	d := New("t")
	a, b := d.Node("A"), d.Node("B")
	col := d.Column("C")
	x, y := col.Node("X"), col.Node("Y")
	res := lay(t, d)
	r := func(n *Node) layout.Rect { return res.ByEl[n.el].Rect }
	if !(r(a).Right() <= r(b).X && r(b).Right() <= res.ByEl[col.el].Rect.X) {
		t.Errorf("row order wrong: A %+v B %+v col %+v", r(a), r(b), res.ByEl[col.el].Rect)
	}
	if !(r(x).Bottom() <= r(y).Y) {
		t.Errorf("column order wrong: X %+v Y %+v", r(x), r(y))
	}
	gap := r(b).X - r(a).Right()
	if math.Abs(gap-d.m.Theme.Gap) > 0.01 {
		t.Errorf("gap between A and B = %v, want %v", gap, d.m.Theme.Gap)
	}
}

func TestGridCellsAndSpans(t *testing.T) {
	d := New("t")
	g := d.Grid("", 3)
	a := g.Node("A", 0, 0)
	b := g.Node("B", 0, 2)
	wide := g.Node("A wide node spanning two columns", 1, 0, Span(1, 2))
	c := g.Node("C", 1, 2)
	res := lay(t, d)
	r := func(n *Node) layout.Rect { return res.ByEl[n.el].Rect }
	if !(r(a).Bottom() <= r(wide).Y && r(a).Right() <= r(b).X) {
		t.Errorf("cells misplaced: A %+v B %+v wide %+v", r(a), r(b), r(wide))
	}
	if r(wide).Right() > r(c).X {
		t.Errorf("span overlaps column 2: wide %+v C %+v", r(wide), r(c))
	}
	// Column 2 lines up across rows.
	if math.Abs(r(b).Center().X-r(c).Center().X) > 0.01 {
		t.Errorf("column 2 misaligned: B %v C %v", r(b).Center().X, r(c).Center().X)
	}
	checkGeometry(t, res)
}

func TestUniformSizes(t *testing.T) {
	d := New("t")
	row := d.Row("", Uniform)
	a := row.Node("A")
	b := row.Node("A much longer label", Desc("and a description"))
	res := lay(t, d)
	ra, rb := res.ByEl[a.el].Rect, res.ByEl[b.el].Rect
	if ra.W != rb.W || ra.H != rb.H {
		t.Errorf("Uniform: A %vx%v, B %vx%v", ra.W, ra.H, rb.W, rb.H)
	}
}

func TestCanvasPositions(t *testing.T) {
	d := New("t")
	c := d.Canvas("Legacy")
	a := c.Node("A", 0, 0)
	b := c.Node("B", 200, 80)
	res := lay(t, d)
	ra, rb := res.ByEl[a.el].Rect, res.ByEl[b.el].Rect
	if math.Abs(rb.X-ra.X-200) > 0.01 || math.Abs(rb.Y-ra.Y-80) > 0.01 {
		t.Errorf("canvas offsets: A %+v B %+v", ra, rb)
	}
	checkGeometry(t, res)
}

func TestTextWrapsAndStaysInside(t *testing.T) {
	d := New("t")
	n := d.Node("A node with a long label that has to wrap onto several lines", MaxWidth(120))
	res := lay(t, d)
	b := res.ByEl[n.el]
	if len(b.Lines) < 3 {
		t.Fatalf("want at least 3 lines, got %d", len(b.Lines))
	}
	for _, ln := range b.Lines {
		if ln.Width > 120.01 || ln.X-ln.Width/2 < b.Rect.X || ln.X+ln.Width/2 > b.Rect.Right() {
			t.Errorf("line %q (w %v at %v) escapes node %+v", ln.Text, ln.Width, ln.X, b.Rect)
		}
	}
}
