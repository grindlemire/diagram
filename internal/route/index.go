package route

import (
	"math"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
)

// spatial is a uniform grid of buckets over item bounding boxes, for
// finding the items near a rectangle without scanning them all. Boxes
// beyond the grid's area fall into its edge buckets. Queries visit
// buckets in a fixed order, so results are deterministic.
type spatial struct {
	x0, y0 float64
	nx, ny int
	cells  [][]int32
	stamp  []uint32
	gen    uint32
}

const indexCell = 64

func newSpatial(area layout.Rect, n int) *spatial {
	s := &spatial{x0: area.X, y0: area.Y, stamp: make([]uint32, n)}
	s.nx = max(1, int(math.Ceil(area.W/indexCell)))
	s.ny = max(1, int(math.Ceil(area.H/indexCell)))
	s.cells = make([][]int32, s.nx*s.ny)
	return s
}

func (s *spatial) span(rc layout.Rect) (x0, y0, x1, y1 int) {
	c := func(v float64, n int) int { return min(max(int(math.Floor(v/indexCell)), 0), n-1) }
	return c(rc.X-s.x0, s.nx), c(rc.Y-s.y0, s.ny), c(rc.Right()-s.x0, s.nx), c(rc.Bottom()-s.y0, s.ny)
}

func (s *spatial) insert(id int, rc layout.Rect) {
	if id >= len(s.stamp) {
		s.stamp = append(s.stamp, make([]uint32, id+1-len(s.stamp))...)
	}
	x0, y0, x1, y1 := s.span(rc)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			s.cells[y*s.nx+x] = append(s.cells[y*s.nx+x], int32(id))
		}
	}
}

// query calls fn once for every item whose box may touch rc.
func (s *spatial) query(rc layout.Rect, fn func(id int)) {
	s.gen++
	if s.gen == 0 {
		clear(s.stamp)
		s.gen = 1
	}
	x0, y0, x1, y1 := s.span(rc)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			for _, id := range s.cells[y*s.nx+x] {
				if s.stamp[id] != s.gen {
					s.stamp[id] = s.gen
					fn(int(id))
				}
			}
		}
	}
}

func segRect(a, b model.Point) layout.Rect {
	x, y := math.Min(a.X, b.X), math.Min(a.Y, b.Y)
	return layout.Rect{X: x, Y: y, W: math.Abs(b.X - a.X), H: math.Abs(b.Y - a.Y)}
}

// segsChanged marks the routed segments as changed.
func (r *router) segsChanged() { r.segVer++ }

// segAdded records that seg i was appended to r.segs.
func (r *router) segAdded(i int) {
	if r.segIdx != nil && r.segIdxVer == r.segVer {
		r.segIdx.insert(i, segRect(r.segs[i].a, r.segs[i].b))
		return
	}
	r.segsChanged()
}

// indexArea is the area spatial indexes cover: the diagram and a margin.
func (r *router) indexArea() layout.Rect { return inflate(r.bounds, 4*r.c) }

// segsNear calls fn for every routed segment that may touch rc.
func (r *router) segsNear(rc layout.Rect, fn func(s seg)) {
	if r.segIdx == nil || r.segIdxVer != r.segVer {
		r.segIdx = newSpatial(r.indexArea(), len(r.segs))
		for i, s := range r.segs {
			r.segIdx.insert(i, segRect(s.a, s.b))
		}
		r.segIdxVer = r.segVer
	}
	r.segIdx.query(rc, func(i int) { fn(r.segs[i]) })
}

// nodesNear calls fn for every node whose shape may touch rc.
func (r *router) nodesNear(rc layout.Rect, fn func(n *layout.Box)) {
	if r.nodeIdx == nil {
		r.nodeIdx = newSpatial(r.indexArea(), len(r.nodes))
		for i, n := range r.nodes {
			r.nodeIdx.insert(i, n.Shape)
		}
	}
	r.nodeIdx.query(rc, func(i int) { fn(r.nodes[i]) })
}
