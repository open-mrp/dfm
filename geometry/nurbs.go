package geometry

import (
	"errors"
	"math"
)

// NURBS is a planar rational B-spline: control Points with Weights (all 1 when nil) over a non-decreasing Knots vector of len(Points)+Degree+1.
type NURBS struct {
	Degree  int
	Knots   []float64
	Points  []Point
	Weights []float64
}

// MaxDegree is the highest spline degree accepted; CAD software writes far lower degrees, and evaluation cost grows with its square.
const MaxDegree = 30

// Validate checks the spline is well-formed.
func (n NURBS) Validate() error {
	switch {
	case n.Degree < 1:
		return errors.New("degree must be at least 1")
	case n.Degree > MaxDegree:
		return errors.New("degree above 30 is not supported")
	case len(n.Points) < n.Degree+1:
		return errors.New("too few control points for the degree")
	case len(n.Knots) != len(n.Points)+n.Degree+1:
		return errors.New("knot count must be control points + degree + 1")
	case n.Weights != nil && len(n.Weights) != len(n.Points):
		return errors.New("weight count must match control points")
	}
	for i := 1; i < len(n.Knots); i++ {
		if !(n.Knots[i] >= n.Knots[i-1]) {
			return errors.New("knots must be non-decreasing")
		}
	}
	for _, w := range n.Weights {
		if !(w > 0) || math.IsInf(w, 0) {
			return errors.New("weights must be positive")
		}
	}
	if !(n.Knots[len(n.Points)] > n.Knots[n.Degree]) {
		return errors.New("spline has an empty parameter range")
	}
	return nil
}

// Spans returns the knot spans of the curve's parameter range that have positive length; each is smooth, so corners can only fall between spans.
func (n NURBS) Spans() []int {
	var out []int
	for k := n.Degree; k < len(n.Points); k++ {
		if n.Knots[k+1] > n.Knots[k] {
			out = append(out, k)
		}
	}
	return out
}

// SpanRange returns the parameter range of span k.
func (n NURBS) SpanRange(k int) (float64, float64) { return n.Knots[k], n.Knots[k+1] }

// hpoint is a control point in homogeneous coordinates (x·w, y·w, w).
type hpoint struct{ x, y, w float64 }

func (n NURBS) homogeneous() []hpoint {
	out := make([]hpoint, len(n.Points))
	for i, p := range n.Points {
		w := 1.0
		if n.Weights != nil {
			w = n.Weights[i]
		}
		out[i] = hpoint{p.X * w, p.Y * w, w}
	}
	return out
}

// deBoor evaluates the B-spline of degree p with control points pts over knots at u, using span k (so a span end gives that span's one-sided limit).
func deBoor(p int, knots []float64, pts []hpoint, k int, u float64) hpoint {
	d := make([]hpoint, p+1)
	copy(d, pts[k-p:k+1])
	for r := 1; r <= p; r++ {
		for j := p; j >= r; j-- {
			lo, hi := knots[j+k-p], knots[j+1+k-r]
			a := 0.0
			if hi > lo {
				a = (u - lo) / (hi - lo)
			}
			d[j] = hpoint{(1-a)*d[j-1].x + a*d[j].x, (1-a)*d[j-1].y + a*d[j].y, (1-a)*d[j-1].w + a*d[j].w}
		}
	}
	return d[p]
}

// Curve returns the point and derivative of span k as a Curve.
func (n NURBS) Curve(k int) Curve {
	p := n.Degree
	pts := n.homogeneous()
	// The derivative is a spline of degree p-1 over the inner knots.
	dpts := make([]hpoint, len(pts)-1)
	for i := range dpts {
		span := n.Knots[i+p+1] - n.Knots[i+1]
		if span > 0 {
			f := float64(p) / span
			dpts[i] = hpoint{f * (pts[i+1].x - pts[i].x), f * (pts[i+1].y - pts[i].y), f * (pts[i+1].w - pts[i].w)}
		}
	}
	dknots := n.Knots[1 : len(n.Knots)-1]
	return func(u float64) (Point, Point) {
		a := deBoor(p, n.Knots, pts, k, u)
		da := deBoor(p-1, dknots, dpts, k-1, u)
		c := Point{a.x / a.w, a.y / a.w}
		return c, Point{(da.x - da.w*c.X) / a.w, (da.y - da.w*c.Y) / a.w}
	}
}

// Segments fits the whole spline with arcs (see FitArcs).
func (n NURBS) Segments(id string) ([]Segment, error) {
	var out []Segment
	for _, k := range n.Spans() {
		a, b := n.SpanRange(k)
		segs, err := FitArcs(n.Curve(k), a, b, id)
		if err != nil {
			return nil, err
		}
		if len(out) > 0 && len(segs) > 0 {
			segs[0].Start = out[len(out)-1].End // spans meet exactly
		}
		out = append(out, segs...)
	}
	return out, nil
}
