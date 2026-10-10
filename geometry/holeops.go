package geometry

import (
	"slices"
)

// RoundHoles returns the part's round holes (circles that are cut-outs, including polygons read as circles) at their drawn size.
func (p *Part) RoundHoles() []Circle { return p.Holes() }

// HoleOp is an operation assigned to a round hole, which was resized from DrawnRadius to Radius (set by the check).
type HoleOp struct {
	EntityIDs           []string
	Center              Point
	DrawnRadius, Radius float64
	// Code names the operation in the capability; Side is "" for taps.
	Code, Side string
}

// HoleOpMiss is a hole assignment that selected no round hole: an entity that is not one (EntityIDs, Location where it has one), or a diameter no round hole has (DiameterMM).
type HoleOpMiss struct {
	Assignment int
	EntityIDs  []string
	DiameterMM float64
	Location   *Point
}

// HoleOpConflict is a hole selected by more than one assignment.
type HoleOpConflict struct {
	EntityIDs   []string
	Center      Point
	Assignments []int
}

// WithHoleOps returns the part rebuilt with each hole in ops resized to its operation's radius, keeping its Report. holes are the part's RoundHoles, ops parallel to them (nil for holes left alone).
func (p *Part) WithHoleOps(holes []Circle, ops []*HoleOp) *Part {
	circles := slices.Clone(p.Circles)
	var kept []HoleOp
	for i, h := range holes {
		op := ops[i]
		if op == nil {
			continue
		}
		kept = append(kept, *op)
		for j := range circles {
			if slices.Equal(circles[j].EntityIDs, h.EntityIDs) && circles[j].Center == h.Center {
				circles[j].Radius = op.Radius
			}
		}
	}
	// The paths were cleaned when the part was built; only the circles changed, so they are placed again.
	q := &Part{Contours: slices.Clone(p.Contours), Circles: circles, HoleOps: kept, Report: p.Report}
	place(q)
	return q
}
