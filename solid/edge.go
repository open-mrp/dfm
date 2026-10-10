package solid

import "math"

// closedTol is how close (mm) an edge's ends must be for it to be a closed curve.
const closedTol = 1e-9

// Prepare finds where the edge lies on its curve. Readers call it once the edge's curve and end points are set.
func (e *Edge) Prepare() {
	switch e.Curve.Kind {
	case CurveCircle, CurveEllipse:
		a, b := e.angle(e.Start), e.angle(e.End)
		var sweep float64
		if e.SameSense {
			sweep = math.Mod(b-a+4*math.Pi, 2*math.Pi)
		} else {
			sweep = -math.Mod(a-b+4*math.Pi, 2*math.Pi)
		}
		if math.Abs(sweep) < closedTol || e.Start.Near(e.End, closedTol) {
			sweep = 2 * math.Pi
			if !e.SameSense {
				sweep = -sweep
			}
		}
		e.t0, e.t1 = a, a+sweep
	case CurveBSpline:
		lo, hi := e.Curve.BSpline.Range()
		if e.Start.Near(e.End, closedTol) {
			e.t0, e.t1 = lo, hi
			if !e.SameSense {
				e.t0, e.t1 = hi, lo
			}
			return
		}
		e.t0, e.t1 = e.Curve.BSpline.closest(e.Start), e.Curve.BSpline.closest(e.End)
	}
}

// angle is q's parameter on a circle or ellipse.
func (e *Edge) angle(q Vec) float64 {
	f := e.Curve.Frame
	d := q.Sub(f.Origin)
	x, y := d.Dot(f.X), d.Dot(f.Y)
	if e.Curve.Kind == CurveEllipse && e.Curve.Minor > 0 {
		x, y = x/e.Curve.Radius, y/e.Curve.Minor
	}
	return math.Atan2(y, x)
}

// Point is the point a fraction s along the edge, from Start (s = 0) to End (s = 1).
func (e *Edge) Point(s float64) Vec {
	switch e.Curve.Kind {
	case CurveCircle, CurveEllipse:
		t := e.t0 + float64((e.t1-e.t0)*s)
		minor := e.Curve.Radius
		if e.Curve.Kind == CurveEllipse {
			minor = e.Curve.Minor
		}
		f := e.Curve.Frame
		return f.Origin.AddScaled(f.X, e.Curve.Radius*math.Cos(t)).AddScaled(f.Y, minor*math.Sin(t))
	case CurveBSpline:
		return e.Curve.BSpline.Point(e.t0 + float64((e.t1-e.t0)*s))
	}
	return e.Start.AddScaled(e.End.Sub(e.Start), s)
}

// Sweep is a circular edge's signed angle about its circle's axis, in radians (2π for a full circle).
func (e *Edge) Sweep() float64 { return e.t1 - e.t0 }

// Closed reports whether the edge ends where it starts.
func (e *Edge) Closed() bool { return e.Start.Near(e.End, closedTol) && e.Curve.Kind != CurveLine }

// closest is the parameter of the point on the spline nearest q.
func (s *BSpline) closest(q Vec) float64 {
	lo, hi := s.Range()
	const n = 256
	best, bestD := lo, math.Inf(1)
	for i := 0; i <= n; i++ {
		u := lo + float64((hi-lo)*float64(i)/n)
		if d := s.Point(u).Dist(q); d < bestD {
			best, bestD = u, d
		}
	}
	step := (hi - lo) / n
	a, b := max(lo, best-step), min(hi, best+step)
	for range 60 {
		m1, m2 := a+(b-a)/3, b-(b-a)/3
		if s.Point(m1).Dist(q) < s.Point(m2).Dist(q) {
			b = m2
		} else {
			a = m1
		}
	}
	return (a + b) / 2
}

// Normal is the surface's unit normal at q (toward +Z of a plane, away from the axis of a cylinder), not yet turned by the face's sense.
func (s Surface) Normal(q Vec) Vec {
	f := s.Frame
	switch s.Kind {
	case SurfacePlane:
		return f.Z
	case SurfaceCylinder:
		d := q.Sub(f.Origin)
		return d.AddScaled(f.Z, -d.Dot(f.Z)).Unit()
	case SurfaceSphere:
		return q.Sub(f.Origin).Unit()
	}
	return Vec{}
}

// Normal is the face's outward unit normal at q, for planes and cylinders.
func (f *Face) Normal(q Vec) Vec {
	n := f.Surface.Normal(q)
	if !f.SameSense {
		n = n.Scale(-1)
	}
	return n
}
