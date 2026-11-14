package diagram

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/themes"
)

func line() int {
	_, _, l, _ := runtime.Caller(1)
	return l
}

func TestErrorsCarrySourcePositions(t *testing.T) {
	d := New("t")
	g := d.Grid("Workers", 2)
	l1 := line() + 1
	g.Node("Worker 1", 0, 0)
	l2 := line() + 1
	g.Node("Worker 2", 0, 0)
	l3 := line() + 1
	g.Node("Worker 3", 0, 1, Span(1, 2))

	err := d.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	msgs := strings.Split(err.Error(), "\n")
	want := []string{
		fmt.Sprintf(`builder_test.go:%d: node "Worker 2": cell (0,0) already occupied by node "Worker 1" (builder_test.go:%d)`, l2, l1),
		fmt.Sprintf(`builder_test.go:%d: node "Worker 3": cell (0,1) with span 1×2 outside 2-column grid`, l3),
	}
	if len(msgs) != len(want) {
		t.Fatalf("got %d errors:\n%s", len(msgs), err)
	}
	for i := range want {
		if msgs[i] != want[i] {
			t.Errorf("error %d:\n got %s\nwant %s", i, msgs[i], want[i])
		}
	}
}

func TestOptionErrors(t *testing.T) {
	d := New("t")
	n := d.Node("A", Size(-1, 10), Fill(Hex("#zz")))
	d.Node("B", ID("x"))
	d.Node("C", ID("x"))
	d.Node("D", Span(2, 1))
	d.Connect(n, New("other").Node("Elsewhere"))
	d.Connect(nil, n)

	err := d.Validate()
	if err == nil {
		t.Fatal("want errors")
	}
	for _, sub := range []string{
		`node "A": Size(-1): want a finite, non-negative number`,
		`node "A": Fill: invalid hex colour "#zz"`,
		`node "C": ID "x" already used at builder_test.go:`,
		`node "D": Span used outside a Grid`,
		`edge A → Elsewhere: node "Elsewhere" belongs to a different diagram`,
		`Connect: nil endpoint`,
	} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("missing %q in:\n%s", sub, err)
		}
	}
}

func TestValidDiagramHasNoErrors(t *testing.T) {
	d := New("Payments")
	web := d.Node("Web")
	vpc := d.Column("VPC")
	api := vpc.Node("API", Desc("Go"), Cylinder, Style(Stroke(Red), Dashed))
	g := vpc.Grid("Workers", 2, Uniform)
	w1 := g.Node("Worker 1", 0, 0)
	w2 := g.Node("Worker 2", 0, 1)
	c := d.Canvas("Legacy")
	c.Node("Mainframe", 10, 20)
	d.Connect(web, api, Label("HTTPS"), Exit(Right), Enter(Left), Via(Pt(1, 2)))
	d.Connect(api, g)
	d.Connect(w1, w2, ArrowBoth, Dotted)
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestIDs(t *testing.T) {
	d := New("t")
	vpc := d.Column("VPC")
	a := vpc.Node("API Server")
	b := vpc.Node("API Server")
	vpc.Row("")
	x := d.Node("Custom", ID("custom"))
	d.Connect(a, b)
	d.Connect(a, b)
	d.Connect(a, x, ID("e1"))
	d.m.Finalize()

	var got []string
	model.Walk(d.m.Root, func(e *model.Element) {
		if e != d.m.Root {
			got = append(got, e.ID)
		}
	})
	for _, e := range d.m.Edges {
		got = append(got, e.ID)
	}
	want := []string{
		"vpc", "vpc/api-server", "vpc/api-server-2", "vpc/row-3", "custom",
		"edge:vpc/api-server->vpc/api-server-2", "edge:vpc/api-server->vpc/api-server-2~2", "e1",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("IDs:\n got %v\nwant %v", got, want)
	}
}

func TestDefaultsAndDepth(t *testing.T) {
	d := New("t")
	outer := d.Column("Region", Defaults(Fill(Red)))
	n := outer.Node("A", Fill(Blue))
	m := outer.Node("B")
	inner := outer.Row("VPC")
	k := inner.Node("C")
	hidden := inner.Row("")
	deep := hidden.Column("Subnet")

	tok := func(c Color) themes.Token { t, _ := c.Token(); return t }
	if tok(n.el.Style.Fill) != tok(Blue) {
		t.Errorf("own option should win over Defaults")
	}
	if tok(m.el.Style.Fill) != tok(Red) {
		t.Errorf("Defaults should apply to direct children")
	}
	if !k.el.Style.Fill.IsZero() {
		t.Errorf("Defaults must not reach nested containers")
	}
	if outer.el.Depth != 0 || inner.el.Depth != 1 || deep.el.Depth != 2 {
		t.Errorf("depths: outer %d inner %d deep %d", outer.el.Depth, inner.el.Depth, deep.el.Depth)
	}
}

func TestValidateDoesNotFreezeIDs(t *testing.T) {
	d := New("t")
	d.Node("A")
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	b := d.Node("B")
	d.m.Finalize()
	if b.el.ID != "b" {
		t.Fatalf("node added after Validate got ID %q, want \"b\"", b.el.ID)
	}
}
