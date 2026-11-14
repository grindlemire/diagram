package paint

import (
	"math"
	"testing"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/scene"
	"github.com/grindlemire/diagram/themes"
)

func pt(x, y float64) model.Point { return model.Point{X: x, Y: y} }

// paintEdge paints an edge along pts and returns its line and arrowheads.
func paintEdge(t *testing.T, pts []model.Point, head, tail model.Arrow) (scene.Path, []scene.Item) {
	t.Helper()
	e := &layout.Edge{E: &model.Edge{}, Points: pts, Rounded: true, Head: head, Tail: tail, Width: 1.5, Color: themes.Of(themes.Muted)}
	g := edge(themes.Light, e).(*scene.Group)
	return g.Items[0].(*scene.Shape).Path, g.Items[1:]
}

func endOf(p scene.Path) model.Point {
	c := p[len(p)-1]
	pt := c.Pts[0]
	if c.Op == scene.QuadTo {
		pt = c.Pts[1]
	}
	return model.Point{X: pt.X, Y: pt.Y}
}

// headLength returns how far back from tip, along the final segment
// (which points down in these tests), the triangle arrowhead reaches.
func headLength(t *testing.T, it scene.Item, tip model.Point) float64 {
	t.Helper()
	far := 0.0
	for _, c := range it.(*scene.Shape).Path {
		if c.Op == scene.Close {
			continue
		}
		far = math.Max(far, tip.Y-c.Pts[0].Y)
	}
	return far
}

func TestRoundedBendUsesTrimmedLastSegment(t *testing.T) {
	// The last segment is 12px; the arrowhead trims 6.4px, leaving 5.6px.
	// The bend may round over all of that but no more.
	path, _ := paintEdge(t, []model.Point{pt(0, 0), pt(100, 0), pt(100, 12)}, model.ArrowTriangle, model.ArrowNone)
	var quad *scene.Cmd
	for i := range path {
		if path[i].Op == scene.QuadTo {
			quad = &path[i]
		}
	}
	if quad == nil {
		t.Fatalf("bend not rounded: %+v", path)
	}
	end := endOf(path)
	if quad.Pts[1].Y > end.Y+1e-9 {
		t.Errorf("curve ends at y %v, past the line end %v", quad.Pts[1].Y, end.Y)
	}
	if r := quad.Pts[1].Y - 0; r > themes.Light.EdgeCornerRad+1e-9 {
		t.Errorf("radius %v exceeds the theme's %v", r, themes.Light.EdgeCornerRad)
	}
}

func TestTinyRemainderLeavesBendSharp(t *testing.T) {
	// 5px last segment: the arrowhead shrinks to half size, which leaves
	// under 2px of line: too little to round.
	path, heads := paintEdge(t, []model.Point{pt(0, 0), pt(100, 0), pt(100, 5)}, model.ArrowTriangle, model.ArrowNone)
	for _, c := range path {
		if c.Op == scene.QuadTo {
			t.Errorf("bend rounded with almost no room: %+v", path)
		}
	}
	if l := headLength(t, heads[0], pt(100, 5)); l > 5+1e-6 {
		t.Errorf("arrowhead reaches %v back, past the bend", l)
	}
	// 9px: the arrowhead shrinks so the whole head sits on the segment.
	_, heads = paintEdge(t, []model.Point{pt(0, 0), pt(100, 0), pt(100, 9)}, model.ArrowTriangle, model.ArrowNone)
	if l := headLength(t, heads[0], pt(100, 9)); l > 9-minStraight+1e-6 || l < themes.Light.ArrowSize/2 {
		t.Errorf("9px segment: arrowhead length %v", l)
	}
}

func TestArrowheadNeverReversesLastSegment(t *testing.T) {
	for _, last := range []float64{1, 3, 5, 7} {
		path, heads := paintEdge(t, []model.Point{pt(0, 0), pt(100, 0), pt(100, last)}, model.ArrowTriangle, model.ArrowNone)
		end := endOf(path)
		if end.Y < -1e-9 || end.Y > last+1e-9 || math.Abs(end.X-100) > 1e-9 {
			t.Errorf("last=%v: line ends at %v, off the final segment", last, end)
		}
		if len(heads) != 1 {
			t.Fatalf("last=%v: %d arrowheads", last, len(heads))
		}
		if l := headLength(t, heads[0], pt(100, last)); l > themes.Light.ArrowSize+1e-6 || l < themes.Light.ArrowSize/2-1e-6 {
			t.Errorf("last=%v: arrowhead length %v outside [size/2, size]", last, l)
		}
	}
}

func TestLongSegmentKeepsFullArrowhead(t *testing.T) {
	_, heads := paintEdge(t, []model.Point{pt(0, 0), pt(100, 0), pt(100, 40)}, model.ArrowTriangle, model.ArrowNone)
	if l := headLength(t, heads[0], pt(100, 40)); math.Abs(l-themes.Light.ArrowSize) > 0.5 {
		t.Errorf("arrowhead length %v, want about %v", l, themes.Light.ArrowSize)
	}
}

func TestShortStraightEdgeWithTwoHeads(t *testing.T) {
	path, heads := paintEdge(t, []model.Point{pt(0, 0), pt(10, 0)}, model.ArrowTriangle, model.ArrowTriangle)
	if len(heads) != 2 {
		t.Fatalf("%d arrowheads", len(heads))
	}
	start, end := path[0].Pts[0], endOf(path)
	if start.X > end.X+1e-9 {
		t.Errorf("line reversed: starts at %v, ends at %v", start.X, end.X)
	}
}
