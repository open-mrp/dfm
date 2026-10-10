package sheet

import (
	"fmt"
	"math"

	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/solid"
)

// mapping takes points of one skin face to the flat pattern. A flat face maps as O2 + U2 (q − O3)·U3 + V2 (q − O3)·V3. A bend maps as O2 + U2 (q − O3)·U3 + V2 φ(q) Rn, where φ is the angle about the axis U3 from the radial direction R0 toward T0, and Rn the neutral radius.
type mapping struct {
	face           *solid.Face
	cyl            bool
	O3, U3, V3     solid.Vec
	R0, T0         solid.Vec
	O2, U2, V2     geometry.Point
	Rn, innerR     float64
	convex         bool
	phiMin, phiMax float64
}

func p2(x, y float64) geometry.Point { return geometry.Point{X: x, Y: y} }

// phiWrap is the angle below which phi wraps round: a bend starts at angle 0 and its face lies toward T0, so an angle a little below 0 is rounding at the start and one well below it is the far side of a bend past 270°. A bend cannot sweep so far that its end comes back within phiWrap of its start.
const phiWrap = -0.5

// phi is the angle of q about the bend's axis, from the start of the bend toward T0.
func (m *mapping) phi(q solid.Vec) float64 {
	d := q.Sub(m.O3)
	a := math.Atan2(d.Dot(m.T0), d.Dot(m.R0))
	if a < phiWrap {
		a += 2 * math.Pi
	}
	return a
}

func (m *mapping) flat(q solid.Vec) geometry.Point {
	d := q.Sub(m.O3)
	a := d.Dot(m.U3)
	b := 0.0
	if m.cyl {
		b = float64(m.phi(q) * m.Rn)
	} else {
		b = d.Dot(m.V3)
	}
	return p2(m.O2.X+float64(m.U2.X*a)+float64(m.V2.X*b), m.O2.Y+float64(m.U2.Y*a)+float64(m.V2.Y*b))
}

// linear maps a direction in a flat face's plane to the flat pattern.
func (m *mapping) linear(v solid.Vec) geometry.Point {
	a, b := v.Dot(m.U3), v.Dot(m.V3)
	return p2(float64(m.U2.X*a)+float64(m.V2.X*b), float64(m.U2.Y*a)+float64(m.V2.Y*b))
}

// viewNormal is the direction, at a flat face, the flat pattern is seen from.
func (m *mapping) viewNormal() solid.Vec {
	n := m.U3.Cross(m.V3)
	if float64(m.U2.X*m.V2.Y)-float64(m.U2.Y*m.V2.X) < 0 {
		n = n.Scale(-1)
	}
	return n
}

// circle maps a full circle on a flat face to a circle of the flat part.
func (m *mapping) circle(e *solid.Edge) (geometry.Circle, bool) {
	if m.cyl || e.Curve.Kind != solid.CurveCircle || !e.Closed() {
		return geometry.Circle{}, false
	}
	return geometry.Circle{Center: m.flat(e.Curve.Frame.Origin), Radius: e.Curve.Radius, EntityIDs: []string{e.ID}}, true
}

// segments maps an edge to lines and arcs of the flat part: exactly for lines and arcs on a flat face, otherwise fitted to the unrolled curve within geometry.CurveTolerance. An edge the fitter cannot follow refuses the model, as a drawing's curve would, rather than being cut as a polygon; one with no length gives no segments.
func (m *mapping) segments(e *solid.Edge) ([]geometry.Segment, error) {
	a, b := m.flat(e.Start), m.flat(e.End)
	if !m.cyl {
		switch e.Curve.Kind {
		case solid.CurveLine:
			return []geometry.Segment{{Start: a, End: b, EntityID: e.ID}}, nil
		case solid.CurveCircle:
			sweep := e.Sweep()
			if e.Curve.Frame.Z.Dot(m.viewNormal()) < 0 {
				sweep = -sweep
			}
			if math.Abs(sweep) <= math.Pi {
				return []geometry.Segment{{Start: a, End: b, Bulge: math.Tan(sweep / 4), EntityID: e.ID}}, nil
			}
			mid := m.flat(e.Point(0.5))
			return []geometry.Segment{
				{Start: a, End: mid, Bulge: math.Tan(sweep / 8), EntityID: e.ID},
				{Start: mid, End: b, Bulge: math.Tan(sweep / 8), EntityID: e.ID},
			}, nil
		}
	}
	const h = 1e-6
	curve := func(s float64) (geometry.Point, geometry.Point) {
		p := m.flat(e.Point(s))
		lo, hi := max(0, s-h), min(1, s+h)
		pl, ph := m.flat(e.Point(lo)), m.flat(e.Point(hi))
		return p, p2((ph.X-pl.X)/(hi-lo), (ph.Y-pl.Y)/(hi-lo))
	}
	segs, err := geometry.FitArcs(curve, 0, 1, e.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: edge %s (%s) cannot be unfolded as arcs within tolerance: %v", ErrNotSheetMetal, e.ID, e.Curve.Name, err)
	}
	if len(segs) == 0 {
		return nil, nil
	}
	segs[0].Start, segs[len(segs)-1].End = a, b
	return segs, nil
}
