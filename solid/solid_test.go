package solid

import (
	"math"
	"testing"
)

func near(a, b Vec) bool { return a.Near(b, 1e-9) }

// TestPrepareCircle places an arc on its circle: a quarter turn each way, and a closed edge as the full circle.
func TestPrepareCircle(t *testing.T) {
	circle := Curve{Kind: CurveCircle, Radius: 10, Frame: Frame{X: Vec{X: 1}, Y: Vec{Y: 1}, Z: Vec{Z: 1}}}
	e := &Edge{Start: Vec{X: 10}, End: Vec{Y: 10}, Curve: circle, SameSense: true}
	e.Prepare()
	if math.Abs(e.Sweep()-math.Pi/2) > 1e-12 || !near(e.Point(0.5), Vec{X: 10 * math.Sqrt2 / 2, Y: 10 * math.Sqrt2 / 2}) {
		t.Errorf("quarter turn: sweep %v, mid %v", e.Sweep(), e.Point(0.5))
	}
	e.SameSense = false
	e.Prepare()
	if math.Abs(e.Sweep()+3*math.Pi/2) > 1e-12 || !near(e.Point(1), Vec{Y: 10}) {
		t.Errorf("the long way round: sweep %v, end %v", e.Sweep(), e.Point(1))
	}
	full := &Edge{Start: Vec{X: 10}, End: Vec{X: 10}, Curve: circle, SameSense: true}
	full.Prepare()
	if !full.Closed() || math.Abs(full.Sweep()-2*math.Pi) > 1e-12 || !near(full.Point(0.5), Vec{X: -10}) {
		t.Errorf("full circle: closed %v, sweep %v, mid %v", full.Closed(), full.Sweep(), full.Point(0.5))
	}
}

// TestBSpline evaluates a degree-1 spline (a polyline) and a rational quadratic (a quarter circle), and finds an edge's bounds on them.
func TestBSpline(t *testing.T) {
	poly := &BSpline{Degree: 1, Points: []Vec{{0, 0, 0}, {1, 1, 0}, {2, 0, 0}}, Knots: []float64{0, 0, 1, 2, 2}}
	if lo, hi := poly.Range(); lo != 0 || hi != 2 {
		t.Errorf("range %v %v", lo, hi)
	}
	if p := poly.Point(0.5); !near(p, Vec{0.5, 0.5, 0}) {
		t.Errorf("point(0.5) = %v", p)
	}
	if p := poly.Point(1.5); !near(p, Vec{1.5, 0.5, 0}) {
		t.Errorf("point(1.5) = %v", p)
	}
	e := &Edge{Start: Vec{0.5, 0.5, 0}, End: Vec{2, 0, 0}, Curve: Curve{Kind: CurveBSpline, BSpline: poly}, SameSense: true}
	e.Prepare()
	if !near(e.Point(0), e.Start) || !near(e.Point(1), e.End) || e.Point(0.5).Dist(Vec{1.25, 0.75, 0}) > 1e-6 {
		t.Errorf("edge on the polyline: %v %v %v", e.Point(0), e.Point(0.5), e.Point(1))
	}
	// A quarter circle of radius 1 as a rational quadratic: weights 1, √2/2, 1.
	w := math.Sqrt2 / 2
	quarter := &BSpline{Degree: 2, Points: []Vec{{1, 0, 0}, {1, 1, 0}, {0, 1, 0}}, Weights: []float64{1, w, 1}, Knots: []float64{0, 0, 0, 1, 1, 1}}
	for _, u := range []float64{0, 0.25, 0.5, 0.75, 1} {
		if r := quarter.Point(u).Len(); math.Abs(r-1) > 1e-12 {
			t.Errorf("point(%v) is %v from the center", u, r)
		}
	}
	if p := quarter.Point(0.5); !near(p, Vec{w, w, 0}) {
		t.Errorf("point(0.5) = %v", p)
	}
}

// TestFrameAndNormals checks the frame's placement and the normals of a plane and a cylinder, turned by a face's sense.
func TestFrameAndNormals(t *testing.T) {
	f := Frame{Origin: Vec{1, 2, 3}, X: Vec{Y: 1}, Y: Vec{Z: 1}, Z: Vec{X: 1}}
	if p := f.At(1, 2, 3); !near(p, Vec{4, 3, 5}) {
		t.Errorf("at = %v", p)
	}
	plane := &Face{Surface: Surface{Kind: SurfacePlane, Frame: f}, SameSense: false}
	if n := plane.Normal(Vec{}); !near(n, Vec{X: -1}) {
		t.Errorf("plane normal %v", n)
	}
	cyl := &Face{Surface: Surface{Kind: SurfaceCylinder, Radius: 2, Frame: Frame{X: Vec{X: 1}, Y: Vec{Y: 1}, Z: Vec{Z: 1}}}, SameSense: true}
	if n := cyl.Normal(Vec{0, 2, 7}); !near(n, Vec{Y: 1}) {
		t.Errorf("cylinder normal %v", n)
	}
}
