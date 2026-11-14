package examples_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grindlemire/diagram"
	"github.com/grindlemire/diagram/examples"
	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/pipeline"
	"github.com/grindlemire/diagram/internal/quality"
	"github.com/grindlemire/diagram/internal/route"
)

var updateQuality = flag.Bool("update-quality", false, "rewrite testdata/quality.json from the current router")

const qualityFile = "testdata/quality.json"

// TestQuality routes every example and compares its route metrics with the
// committed baseline, failing when a diagram gets noticeably worse.
// Improvements are logged; run with -update-quality to record them.
func TestQuality(t *testing.T) {
	got := map[string]quality.Metrics{}
	var names []string
	var table strings.Builder
	fmt.Fprintf(&table, "\n%-20s %5s %5s %6s %4s %4s %4s %4s %4s %4s %7s %8s %6s %7s\n",
		"example", "bends", "cross", "ratio", "thru", "lNod", "lOwn", "lTxt", "lbl", "det", "shared", "outLen", "outMax", "route")
	var total quality.Metrics
	nratio := 0
	for _, ex := range examples.All() {
		if only := os.Getenv("QUALITY_ONLY"); only != "" && only != ex.Name {
			continue
		}
		d := ex.Build()
		if d.Validate() != nil {
			continue
		}
		m := model.Of(d)
		res, err := layout.Layout(m, m.Theme)
		if err != nil {
			continue
		}
		start := time.Now()
		res, err = pipeline.Routed(m, m.Theme, res)
		if err != nil {
			t.Errorf("%s: %v", ex.Name, err)
			continue
		}
		took := time.Since(start)
		q := quality.Measure(res)
		if os.Getenv("QUALITY_EDGES") == ex.Name {
			for _, e := range q.Edges {
				t.Logf("%-40s jogs %d bends %d cross %d ratio %.2f thru %d lbl %d/%d/%d/%d det %d shared %.0f out %.0f/%.0f",
					e.ID, e.Jogs, e.Bends, e.Crossings, e.Ratio, e.Through, e.LabelOverNode, e.LabelOverOwnEnd, e.LabelOverText, e.LabelOverlaps, e.Detached, e.Shared, e.OutsideLen, e.OutsideDist)
			}
		}
		got[ex.Name] = q
		names = append(names, ex.Name)
		fmt.Fprintf(&table, "%-20s %5d %5d %6.3f %4d %4d %4d %4d %4d %4d %4d %7.1f %8.1f %6.1f %5dms\n",
			ex.Name, q.Bends, q.Crossings, q.Ratio, q.Jogs, q.Through, q.LabelOverNode, q.LabelOverOwnEnd, q.LabelOverText, q.LabelOverlaps, q.Detached, q.Shared, q.OutsideLen, q.OutsideDist, took.Milliseconds())
		total.Bends += q.Bends
		total.Crossings += q.Crossings
		if q.Ratio > 0 {
			total.Ratio += q.Ratio
			nratio++
		}
		total.Through += q.Through
		total.LabelOverNode += q.LabelOverNode
		total.LabelOverOwnEnd += q.LabelOverOwnEnd
		total.LabelOverText += q.LabelOverText
		total.LabelOverlaps += q.LabelOverlaps
		total.Detached += q.Detached
		total.Jogs += q.Jogs
		total.Shared += q.Shared
		total.OutsideLen += q.OutsideLen
		total.OutsideDist = math.Max(total.OutsideDist, q.OutsideDist)
	}
	if nratio > 0 {
		total.Ratio /= float64(nratio)
	}
	fmt.Fprintf(&table, "%-20s %5d %5d %6.3f %4d %4d %4d %4d %4d %4d %4d %7.1f %8.1f %6.1f\n",
		"TOTAL", total.Bends, total.Crossings, total.Ratio, total.Jogs, total.Through, total.LabelOverNode, total.LabelOverOwnEnd, total.LabelOverText, total.LabelOverlaps, total.Detached, total.Shared, total.OutsideLen, total.OutsideDist)
	t.Log(table.String())

	if *updateQuality {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(qualityFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(qualityFile, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	file := qualityFile
	if f := os.Getenv("QUALITY_BASE"); f != "" {
		file = f // compare against another baseline while experimenting
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("no baseline (run with -update-quality): %v", err)
	}
	var base map[string]quality.Metrics
	if err := json.Unmarshal(data, &base); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		b, ok := base[name]
		if !ok {
			t.Errorf("%s: not in the baseline (run with -update-quality)", name)
			continue
		}
		g := got[name]
		var worse, better []string
		cmpInt := func(what string, bv, gv int) {
			switch {
			case gv > bv+bv/20:
				worse = append(worse, fmt.Sprintf("%s %d → %d", what, bv, gv))
			case gv < bv:
				better = append(better, fmt.Sprintf("%s %d → %d", what, bv, gv))
			}
		}
		cmpF := func(what string, bv, gv, slack float64) {
			switch {
			case gv > bv*1.05+slack:
				worse = append(worse, fmt.Sprintf("%s %.3g → %.3g", what, bv, gv))
			case gv < bv-1e-6 && gv < bv*0.99:
				better = append(better, fmt.Sprintf("%s %.3g → %.3g", what, bv, gv))
			}
		}
		cmpInt("bends", b.Bends, g.Bends)
		cmpInt("crossings", b.Crossings, g.Crossings)
		cmpInt("through", b.Through, g.Through)
		cmpInt("labels over nodes", b.LabelOverNode, g.LabelOverNode)
		cmpInt("labels over text", b.LabelOverText, g.LabelOverText)
		cmpInt("labels over own ends", b.LabelOverOwnEnd, g.LabelOverOwnEnd)
		cmpInt("label overlaps", b.LabelOverlaps, g.LabelOverlaps)
		cmpInt("detached labels", b.Detached, g.Detached)
		cmpInt("jogs", b.Jogs, g.Jogs)
		cmpF("ratio", b.Ratio, g.Ratio, 0)
		cmpF("shared", b.Shared, g.Shared, 5)
		cmpF("outside length", b.OutsideLen, g.OutsideLen, 10)
		cmpF("outside distance", b.OutsideDist, g.OutsideDist, 5)
		if len(worse) > 0 {
			t.Errorf("%s got worse: %s", name, strings.Join(worse, ", "))
		}
		if len(better) > 0 {
			t.Logf("%s improved: %s", name, strings.Join(better, ", "))
		}
	}
}

// worstCase is the large example scaled up: 100 nodes in ten teams and
// 200 random edges.
func worstCase() *diagram.Diagram {
	d := diagram.New("Worst case", diagram.Vertical)
	rows := []*diagram.Row{d.Row(""), d.Row("")}
	rng := rand.New(rand.NewPCG(3, 4))
	var nodes []*diagram.Node
	for gi := range 10 {
		g := rows[gi/5].Grid(fmt.Sprintf("Team %d", gi+1), 3, diagram.Gap(24))
		for i := range 10 {
			nodes = append(nodes, g.Node(fmt.Sprintf("t%d-svc%d", gi+1, i+1), i/3, i%3))
		}
	}
	for i := 0; i < 200; {
		a, b := rng.IntN(len(nodes)), rng.IntN(len(nodes))
		if a == b {
			continue
		}
		var opts []diagram.EdgeOption
		if i%10 == 0 {
			opts = append(opts, diagram.Label(fmt.Sprintf("e%d", i)))
		}
		d.Connect(nodes[a], nodes[b], opts...)
		i++
	}
	return d
}

func benchRoute(b *testing.B, d *diagram.Diagram) {
	m := model.Of(d)
	for b.Loop() {
		res, err := layout.Layout(m, m.Theme)
		if err != nil {
			b.Fatal(err)
		}
		if err := route.Route(res); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRouteLarge(b *testing.B) {
	for _, ex := range examples.All() {
		if ex.Name == "large" {
			benchRoute(b, ex.Build())
		}
	}
}

func BenchmarkRouteWorstCase(b *testing.B) { benchRoute(b, worstCase()) }

// TestSpacing checks the default spacing against ExactSpacing: the floor
// raises gaps that edges use, leaves others alone, ExactSpacing keeps
// gaps as written, and the default routes tight-gaps better.
func TestSpacing(t *testing.T) {
	byName := map[string]examples.Example{}
	for _, ex := range examples.All() {
		byName[ex.Name] = ex
	}
	lay := func(d *diagram.Diagram) *layout.Result {
		m := model.Of(d)
		res, err := pipeline.Lay(m, m.Theme)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	// colGap is the gap between the first two nodes of the first grid.
	colGap := func(res *layout.Result) float64 {
		for _, b := range res.Boxes {
			if b.El.Kind == model.KindGrid && len(b.El.Children) > 1 {
				return res.ByEl[b.El.Children[1]].Rect.X - res.ByEl[b.El.Children[0]].Rect.Right()
			}
		}
		t.Fatal("no grid")
		return 0
	}
	spaced, exact := lay(byName["tight-gaps"].Build()), lay(byName["tight-gaps-exact"].Build())
	th := spaced.Theme
	if g := colGap(spaced); g < 2*th.EdgeClearance+th.ParallelOffset-0.01 {
		t.Errorf("tight-gaps: column gap %.1f is below the floor", g)
	}
	if g := colGap(exact); math.Abs(g-14) > 0.01 {
		t.Errorf("tight-gaps-exact: column gap %.1f, want the written 14", g)
	}

	// No edges, no floor.
	d := diagram.New("no edges")
	row := d.Row("", diagram.Gap(10))
	a, b := row.Node("A"), row.Node("B")
	res := lay(d)
	ra, rb := res.ByEl[model.Of(d).Root.Children[0].Children[0]].Rect, res.ByEl[model.Of(d).Root.Children[0].Children[1]].Rect
	if g := rb.X - ra.Right(); math.Abs(g-10) > 0.01 {
		t.Errorf("unwired row: gap %.1f, want the written 10", g)
	}
	// One edge between them raises it to the floor.
	d.Connect(a, b)
	res = lay(d)
	ra, rb = res.ByEl[model.Of(d).Root.Children[0].Children[0]].Rect, res.ByEl[model.Of(d).Root.Children[0].Children[1]].Rect
	if g := rb.X - ra.Right(); g < 2*th.EdgeClearance+th.ParallelOffset-0.01 {
		t.Errorf("wired row: gap %.1f is below the floor", g)
	}

	qs, qe := quality.Measure(unfit(t, byName["tight-gaps"])), quality.Measure(unfit(t, byName["tight-gaps-exact"]))
	if qs.Score() >= qe.Score() || qs.OutsideLen > qe.OutsideLen || qs.LabelOverOwnEnd > qe.LabelOverOwnEnd {
		t.Errorf("tight-gaps doesn't route better than tight-gaps-exact: score %.0f vs %.0f", qs.Score(), qe.Score())
	}

	var svg1, svg2 bytes.Buffer
	if err := byName["tight-gaps"].Build().RenderSVG(&svg1); err != nil {
		t.Fatal(err)
	}
	if err := byName["tight-gaps"].Build().RenderSVG(&svg2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(svg1.Bytes(), svg2.Bytes()) {
		t.Error("tight-gaps: two renders differ")
	}
}

// unfit lays out and routes an example the way rendering does, without
// fitting the canvas, so its quality can be measured.
func unfit(t *testing.T, ex examples.Example) *layout.Result {
	m := model.Of(ex.Build())
	res, err := layout.Layout(m, m.Theme)
	if err != nil {
		t.Fatal(err)
	}
	if res, err = pipeline.Routed(m, m.Theme, res); err != nil {
		t.Fatal(err)
	}
	return res
}

// BenchmarkPipelineLarge lays out, routes and (by default) re-spaces the
// large example: everything a render does before drawing.
func BenchmarkPipelineLarge(b *testing.B) {
	for _, ex := range examples.All() {
		if ex.Name == "large" {
			m := model.Of(ex.Build())
			for b.Loop() {
				if _, err := pipeline.Lay(m, m.Theme); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
}
