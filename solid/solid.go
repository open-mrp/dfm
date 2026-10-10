// Package solid is the neutral 3D model of a design: bodies bounded by faces, each face on a surface and bounded by loops of edges on curves. Design readers produce it (package design); processes interpret it, such as sheet metal unfolding it into a flat part (package sheet). Lengths are in millimeters.
package solid

import "math"

// Vec is a point or direction in space.
type Vec struct{ X, Y, Z float64 }

// The vector operations convert each product before adding it, so the compiler does not fuse them into one instruction on some CPUs and not others: results must be the same everywhere.

func (a Vec) Add(b Vec) Vec                { return Vec{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec) Sub(b Vec) Vec                { return Vec{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec) Scale(k float64) Vec          { return Vec{float64(a.X * k), float64(a.Y * k), float64(a.Z * k)} }
func (a Vec) Dot(b Vec) float64            { return float64(a.X*b.X) + float64(a.Y*b.Y) + float64(a.Z*b.Z) }
func (a Vec) Len() float64                 { return math.Sqrt(a.Dot(a)) }
func (a Vec) Dist(b Vec) float64           { return a.Sub(b).Len() }
func (a Vec) Near(b Vec, tol float64) bool { return a.Dist(b) <= tol }

// Cross is the cross product a × b.
func (a Vec) Cross(b Vec) Vec {
	return Vec{float64(a.Y*b.Z) - float64(a.Z*b.Y), float64(a.Z*b.X) - float64(a.X*b.Z), float64(a.X*b.Y) - float64(a.Y*b.X)}
}

// Unit is a scaled to length 1, or the zero vector when a is zero.
func (a Vec) Unit() Vec {
	l := a.Len()
	if l == 0 {
		return Vec{}
	}
	return a.Scale(1 / l)
}

// AddScaled is a + b × k.
func (a Vec) AddScaled(b Vec, k float64) Vec {
	return Vec{a.X + float64(b.X*k), a.Y + float64(b.Y*k), a.Z + float64(b.Z*k)}
}

// Frame is a right-handed placement: an origin and three unit axes.
type Frame struct {
	Origin  Vec
	X, Y, Z Vec
}

// At is the point at local coordinates (u, v, w).
func (f Frame) At(u, v, w float64) Vec {
	return f.Origin.AddScaled(f.X, u).AddScaled(f.Y, v).AddScaled(f.Z, w)
}

// SurfaceKind is the type of a face's surface.
type SurfaceKind int

const (
	SurfacePlane SurfaceKind = iota
	SurfaceCylinder
	SurfaceCone
	SurfaceSphere
	SurfaceTorus
	// SurfaceOther is any other surface (B-spline, swept, offset), kept only by kind.
	SurfaceOther
)

// Surface is a face's surface in its frame: a plane through the origin normal to Z; a cylinder, cone, sphere or torus about Z.
type Surface struct {
	Kind  SurfaceKind
	Frame Frame
	// Radius is the cylinder's, the cone's at the origin, the sphere's, or the torus's major radius; Minor is the torus's minor radius and SemiAngle the cone's half angle in radians.
	Radius, Minor, SemiAngle float64
	// Name is the file's entity type, for messages.
	Name string
}

// CurveKind is the type of an edge's curve.
type CurveKind int

const (
	CurveLine CurveKind = iota
	CurveCircle
	CurveEllipse
	CurveBSpline
	// CurveOther is any other curve, known only by its end points.
	CurveOther
)

// Curve is an edge's curve: a line through Frame.Origin along Frame.Z; a circle or ellipse about Frame.Z (Radius, and Minor along Frame.Y for an ellipse); or a B-spline.
type Curve struct {
	Kind          CurveKind
	Frame         Frame
	Radius, Minor float64
	BSpline       *BSpline
	Name          string
}

// BSpline is a non-uniform rational B-spline curve.
type BSpline struct {
	Degree  int
	Points  []Vec
	Weights []float64 // nil when not rational
	Knots   []float64 // full knot vector, multiplicities expanded
}

// Point evaluates the spline at u (de Boor's algorithm).
func (s *BSpline) Point(u float64) Vec {
	p := s.Degree
	n := len(s.Points)
	k := p
	for k < n-1 && u >= s.Knots[k+1] {
		k++
	}
	type hp struct {
		v Vec
		w float64
	}
	d := make([]hp, p+1)
	for j := 0; j <= p; j++ {
		w := 1.0
		if s.Weights != nil {
			w = s.Weights[j+k-p]
		}
		d[j] = hp{s.Points[j+k-p].Scale(w), w}
	}
	for r := 1; r <= p; r++ {
		for j := p; j >= r; j-- {
			lo, hi := s.Knots[j+k-p], s.Knots[j+1+k-r]
			a := 0.0
			if hi > lo {
				a = (u - lo) / (hi - lo)
			}
			d[j] = hp{d[j-1].v.Scale(1-a).AddScaled(d[j].v, a), float64(d[j-1].w*(1-a)) + float64(d[j].w*a)}
		}
	}
	return d[p].v.Scale(1 / d[p].w)
}

// Range is the spline's parameter range.
func (s *BSpline) Range() (float64, float64) {
	return s.Knots[s.Degree], s.Knots[len(s.Knots)-1-s.Degree]
}

// Edge is a bounded piece of a curve from Start to End.
type Edge struct {
	ID         string
	Start, End Vec
	Curve      Curve
	// SameSense is false when the edge runs against its curve's direction.
	SameSense bool
	// t0 and t1 bound the edge on the curve (set by Prepare), so that Point(0) is Start and Point(1) is End.
	t0, t1 float64
}

// Loop is a closed chain of edges bounding a face, each with the direction it is used in.
type Loop struct {
	Edges    []*Edge
	Reversed []bool
	Outer    bool
}

// Face is a bounded piece of a surface. SameSense is false when the face's outward normal is opposite the surface's.
type Face struct {
	ID        string
	Surface   Surface
	SameSense bool
	Loops     []Loop
}

// Body is a solid bounded by faces.
type Body struct {
	ID    string
	Faces []*Face
}

// Model is a design's solids.
type Model struct {
	Bodies []*Body
}
