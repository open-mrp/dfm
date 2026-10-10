package dfm

import (
	"fmt"
	"math"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// Bend is one bend line of a part as the bending check bent it.
type Bend struct {
	EntityID string
	Length   measure.Length
	// Angle is from flat, in millionths of a degree.
	Angle     measure.Ratio
	Direction constants.BendDirection
	// InsideRadius is the radius the design is drawn for, exact in millimeters.
	InsideRadius measure.Decimal
	// Allowance is the length of flat material the bend takes up, at the radius the tooling forms.
	Allowance measure.Length
	// Assumed names what the request did not give and was defaulted: "angle", "direction", "inside_radius".
	Assumed []string
}

// applyBends sets how every bend line of the drawing is bent: as an assignment gives, or else 90 degrees up at the brake's inside radius. An assignment naming an entity that is not a bend line is an error.
func applyBends(part *geometry.Part, as []capability.BendAssignment, model map[string]modelBend, profile *capability.Profile) *apierror.Error {
	part.SnapBendEnds(profile.Thickness.MM())
	lines := map[string]bool{}
	for _, b := range part.BendLines() {
		lines[b.EntityID] = true
	}
	given := map[string]capability.BendAssignment{}
	for i, a := range as {
		for j, id := range a.EntityIDs {
			if !lines[id] {
				return apierror.ParameterInvalid(fmt.Sprintf("bends[%d].entity_ids[%d]", i, j), "%q is not a bend line of the design", id)
			}
			given[id] = a
		}
	}
	part.BendSpecs = map[string]geometry.BendSpec{}
	for id := range lines {
		a := given[id]
		if mb, ok := model[id]; ok {
			// A 3D model gives every bend; the request may still override.
			if a.Angle == 0 {
				a.Angle = mb.Angle
			}
			if a.Direction == "" {
				a.Direction = mb.Direction
			}
		}
		s := geometry.BendSpec{Angle: a.Angle, Direction: a.Direction, AngleGiven: a.Angle > 0, DirectionGiven: a.Direction != "", RadiusGiven: a.InsideRadius > 0}
		if !s.AngleGiven {
			s.Angle = capability.DefaultBendAngle
		}
		if !s.DirectionGiven {
			s.Direction = constants.BendDirectionUp
		}
		if mb, ok := model[id]; ok && !s.RadiusGiven {
			s.InsideRadius, s.RadiusGiven = mb.InsideRadius, true
		} else if s.RadiusGiven {
			s.InsideRadius = a.InsideRadius.Decimal()
		} else {
			s.InsideRadius = profile.Bending.InsideRadius
		}
		s.AllowanceMM = BendAllowanceMM(s.Angle, profile.Bending.InsideRadius, profile.Bending.KFactor, profile.Thickness)
		part.BendSpecs[id] = s
	}
	return nil
}

// BendAllowanceMM is the length of the neutral axis through a bend of angle (from flat, in millionths of a degree) at inside radius r (mm) in stock thickness t with K-factor k: angle × (r + k t), in millimeters.
func BendAllowanceMM(angle measure.Ratio, r measure.Decimal, k measure.Ratio, t measure.Length) float64 {
	rad := float64(angle) / 1e6 * math.Pi / 180
	kt := float64(k) / 1e6 * t.MM()
	return rad * (float64(kt) + r.Float64())
}

// partBends lists the bend lines lying in part p of the drawing, in drawing order.
func partBends(drawing, p *geometry.Part) []Bend {
	var out []Bend
	for _, b := range drawing.BendLines() {
		if !p.Covers(b.Midpoint(), false) {
			continue
		}
		l, err := measure.FromMM(b.Length())
		if err != nil {
			continue
		}
		allowance, _ := measure.FromMM(b.Spec.AllowanceMM)
		bend := Bend{EntityID: b.EntityID, Length: l, Angle: b.Spec.Angle, Direction: b.Spec.Direction, InsideRadius: b.Spec.InsideRadius, Allowance: allowance, Assumed: []string{}}
		for _, x := range []struct {
			given bool
			name  string
		}{{b.Spec.AngleGiven, "angle"}, {b.Spec.DirectionGiven, "direction"}, {b.Spec.RadiusGiven, "inside_radius"}} {
			if !x.given {
				bend.Assumed = append(bend.Assumed, x.name)
			}
		}
		out = append(out, bend)
	}
	return out
}
