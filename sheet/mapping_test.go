package sheet

import (
	"errors"
	"testing"

	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/solid"
)

// TestUnfittableEdgeRefused unfolds an edge the arc fitter cannot follow within tolerance (a B-spline zigzagging through thousands of sharp corners, each of which costs the fitter more halvings than its budget allows): the model is refused, as a drawing with such a curve is, rather than the edge being cut as a polygon.
func TestUnfittableEdgeRefused(t *testing.T) {
	const corners = 2000
	s := &solid.BSpline{Degree: 1}
	for i := 0; i <= corners; i++ {
		y := 0.0
		if i%2 == 1 {
			y = 1
		}
		s.Points = append(s.Points, solid.Vec{X: float64(i) / corners, Y: y})
	}
	s.Knots = append(s.Knots, 0)
	for i := 0; i <= corners; i++ {
		s.Knots = append(s.Knots, float64(i))
	}
	s.Knots = append(s.Knots, corners)
	e := &solid.Edge{ID: "#1", Start: s.Points[0], End: s.Points[corners], Curve: solid.Curve{Kind: solid.CurveBSpline, BSpline: s, Name: "B_SPLINE_CURVE"}, SameSense: true}
	e.Prepare()
	m := &mapping{U3: solid.Vec{X: 1}, V3: solid.Vec{Y: 1}, U2: geometry.Point{X: 1}, V2: geometry.Point{Y: 1}}
	_, err := m.segments(e)
	if !errors.Is(err, ErrNotSheetMetal) {
		t.Fatalf("got %v, want ErrNotSheetMetal", err)
	}
	// A smooth edge of the same kind is fitted.
	arc := &solid.Edge{ID: "#2", Start: solid.Vec{X: 10}, End: solid.Vec{Y: 5}, Curve: solid.Curve{Kind: solid.CurveEllipse, Frame: solid.Frame{X: solid.Vec{X: 1}, Y: solid.Vec{Y: 1}, Z: solid.Vec{Z: 1}}, Radius: 10, Minor: 5}, SameSense: true}
	arc.Prepare()
	segs, err := m.segments(arc)
	if err != nil || len(segs) == 0 || !segs[0].Start.Near(geometry.Point{X: 10}) || !segs[len(segs)-1].End.Near(geometry.Point{Y: 5}) {
		t.Errorf("ellipse: %v, %d segments", err, len(segs))
	}
}
