// Package pipeline runs the stages every output format shares: validation,
// layout, routing and fitting the canvas to what was drawn.
package pipeline

import (
	"errors"

	"github.com/grindlemire/diagram/internal/layout"
	"github.com/grindlemire/diagram/internal/model"
	"github.com/grindlemire/diagram/internal/route"
	"github.com/grindlemire/diagram/themes"
)

// Lay validates m, lays it out with theme t, routes its edges and grows
// the canvas to contain everything. extra errors (such as bad render
// options) are reported together with the diagram's own.
func Lay(m *model.Diagram, t *themes.Theme, extra ...error) (*layout.Result, error) {
	// Layout runs even on a model with builder errors, so that its own
	// errors (overlapping canvas children) are reported with them.
	res, lerr := layout.Layout(m, t)
	if err := errors.Join(append(append(append(append([]error{}, m.Errs()...), Glyphs(m)...), extra...), lerr)...); err != nil {
		return nil, err
	}
	res, err := Routed(m, t, res)
	if err != nil {
		return nil, err
	}
	res.Fit()
	return res, nil
}

// Routed routes the edges of res, a layout of m with theme t, and returns
// the result to draw, not yet fitted to its canvas.
//
// Unless m asks for ExactSpacing, it first sketches the routes, and if
// they crowd any gap between children (more edges running along it than
// fit side by side), lays m out again with those gaps widened and sketches
// that. Whichever sketch scores better is routed in full: the widened
// layout only when it routes better.
func Routed(m *model.Diagram, t *themes.Theme, res *layout.Result) (*layout.Result, error) {
	if m.ExactSpacing {
		return res, route.Route(res)
	}
	s := route.Start(res)
	wide, grew := res.Crowded(nil, s.Sketches())
	if grew {
		if r2, err := layout.LayoutSpaced(m, t, wide); err == nil {
			s2 := route.Start(r2)
			if s2.Score() < s.Score() {
				if err := s2.Finish(); err == nil {
					return r2, nil
				}
			}
		}
	}
	return res, s.Finish()
}
