package geometry

import (
	"math"
)

// Corner is a place where the material boundary turns inward (an inside corner of the material), with the radius it is rounded to; 0 when sharp.
type Corner struct {
	// Loop indexes Part.Loops.
	Loop     int
	RadiusMM float64
	At       Point
}

// cornerTolerance is the smallest change of direction treated as a corner.
const cornerTolerance = math.Pi / 180

// ConcaveCorners returns every inside corner of the material: sharp vertices and rounding arcs (less than a half turn) that bend into the material. Full circles and slot ends are not corners.
func (p *Part) ConcaveCorners() []Corner {
	var out []Corner
	for li, l := range p.Loops {
		n := len(l.prims)
		if n < 2 {
			continue
		}
		materialInside := l.Depth%2 == 0
		for k, pr := range l.prims {
			prev := l.prims[(k-1+n)%n]
			t1, t2 := endTangent(prev), startTangent(pr)
			turn := math.Atan2(cross(t1.X, t1.Y, t2.X, t2.Y), dot(t1, t2))
			if math.Abs(turn) > cornerTolerance {
				convex := (turn > 0) == l.ccw
				if convex != materialInside {
					out = append(out, Corner{Loop: li, RadiusMM: 0, At: pr.a})
				}
			}
			if !pr.line && math.Abs(pr.sweep) < math.Pi-cornerTolerance {
				convex := (pr.sweep > 0) == l.ccw
				if convex != materialInside {
					out = append(out, Corner{Loop: li, RadiusMM: pr.r, At: pr.pointAt(pr.start + pr.sweep/2)})
				}
			}
		}
	}
	return out
}

// Tip is a corner where the boundary comes to a point: the angle between its two sides, on whichever side of the boundary it is less than a half turn.
type Tip struct {
	// Loop indexes Part.Loops.
	Loop     int
	AngleDeg float64
	At       Point
}

// SharpestTips returns, for every loop with a corner, its sharpest one. Tangent joins (fillets, slot ends) are not corners.
func (p *Part) SharpestTips() []Tip {
	var out []Tip
	for li, l := range p.Loops {
		n := len(l.prims)
		if n < 2 {
			continue
		}
		best := Tip{Loop: li, AngleDeg: 180}
		for k, pr := range l.prims {
			t1, t2 := endTangent(l.prims[(k-1+n)%n]), startTangent(pr)
			turn := math.Atan2(cross(t1.X, t1.Y, t2.X, t2.Y), dot(t1, t2))
			if a := 180 - math.Abs(turn)*180/math.Pi; a < best.AngleDeg {
				best.AngleDeg, best.At = a, pr.a
			}
		}
		if best.AngleDeg < 180-cornerTolerance*180/math.Pi {
			out = append(out, best)
		}
	}
	return out
}
