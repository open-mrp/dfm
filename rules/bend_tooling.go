package rules

import (
	"math"
	"slices"

	"github.com/open-mrp/dfm/brake"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

// BendToolingRule asks for review of each pair of parallel bends that turn the same way and overlap along their length: a channel, whose second bend may bring the first flange into the punch or the ram. Tooling and bend order are not modeled, so the engine cannot decide.
type BendToolingRule struct{}

func (BendToolingRule) Code() constants.FindingCode { return constants.FindingCodeBendToolingUnchecked }

func (BendToolingRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if p.Bending == nil || p.Bending.Tooling != nil {
		return nil
	}
	bends := part.BendLines()
	var out []Finding
	for i, a := range bends {
		la := a.Length()
		if la == 0 {
			continue
		}
		ux, uy := (a.B.X-a.A.X)/la, (a.B.Y-a.A.Y)/la
		for _, b := range bends[i+1:] {
			lb := b.Length()
			if lb == 0 || a.Spec.Direction != b.Spec.Direction {
				continue
			}
			vx, vy := (b.B.X-b.A.X)/lb, (b.B.Y-b.A.Y)/lb
			if math.Abs(float64(ux*vy)-float64(uy*vx)) > math.Sin(math.Pi/180) {
				continue // not parallel
			}
			along := func(q geometry.Point) float64 { return float64((q.X-a.A.X)*ux) + float64((q.Y-a.A.Y)*uy) }
			lo, hi := min(along(b.A), along(b.B)), max(along(b.A), along(b.B))
			if min(hi, la)-max(lo, 0) <= geometry.EndpointTolerance {
				continue // side by side, not facing
			}
			ma, mb := a.Midpoint(), b.Midpoint()
			out = append(out, Finding{
				Code:      constants.FindingCodeBendToolingUnchecked,
				Severity:  constants.SeverityReview,
				Message:   "Bends turning the same way form a channel; whether the tooling clears it is not checked",
				EntityIDs: []string{a.EntityID, b.EntityID},
				Location:  at(geometry.Point{X: (ma.X + mb.X) / 2, Y: (ma.Y + mb.Y) / 2}),
				Span:      span(ma, mb),
			})
		}
	}
	return out
}

// BendCollisionRule simulates forming the bends on the press brake's tooling and fails a part no bend order forms without running it into the punch or the die (see package brake).
type BendCollisionRule struct{}

func (BendCollisionRule) Code() constants.FindingCode { return constants.FindingCodeBendToolCollision }

// collisionBudget is how many bends the sequence search forms in all before it stops and asks for review.
const collisionBudget = 1500

func (BendCollisionRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	m, lines := foldModel(part, p)
	if m == nil {
		return nil
	}
	plan := m.Sequence(toolingMM(p.Bending.Tooling), collisionBudget)
	if plan.Found {
		return nil
	}
	finding := func(b geometry.BendLine, sev constants.Severity, msg string) Finding {
		return Finding{
			Code:      constants.FindingCodeBendToolCollision,
			Severity:  sev,
			Message:   msg,
			EntityIDs: []string{b.EntityID},
			Location:  at(b.Midpoint()),
			Span:      span(b.A, b.B),
		}
	}
	if !plan.Exhausted {
		var ids []string
		for _, b := range lines {
			ids = append(ids, b.EntityID)
		}
		f := finding(lines[0], constants.SeverityReview, "No bend order clear of the tooling was found before the search stopped; whether one exists is not known")
		f.EntityIDs, f.Location, f.Span = ids, nil, nil
		return []Finding{f}
	}
	var blocked []int
	for bi := range plan.Blocked {
		blocked = append(blocked, bi)
	}
	slices.Sort(blocked)
	var out []Finding
	for _, bi := range blocked {
		r := plan.Blocked[bi]
		msg := "No bend order forms the part: forming this bend runs it into the punch"
		switch {
		case r.Punch && r.Die:
			msg = "No bend order forms the part: forming this bend runs it into the punch and the die"
		case r.Die:
			msg = "No bend order forms the part: forming this bend runs it into the die"
		}
		out = append(out, finding(lines[bi], constants.SeverityError, msg))
	}
	return out
}

// toolingMM converts the capability's tooling for the simulation.
func toolingMM(t *capability.ResolvedTooling) brake.Tooling {
	return brake.Tooling{
		PunchTipAngle: float64(t.PunchTipAngle) / 1e6, PunchWidth: t.PunchWidth.MM(), PunchHeight: t.PunchHeight.MM(), PunchLength: t.PunchLength.MM(),
		PunchNose: t.PunchNose.MM(), PunchThroatDepth: t.PunchThroatDepth.MM(), PunchThroatHeight: t.PunchThroatHeight.MM(),
		DieOpening: t.DieOpeningMM.Float64(), DieAngle: float64(t.DieAngle) / 1e6, DieWidth: t.DieWidth.MM(), DieHeight: t.DieHeight.MM(), DieLength: t.DieLength.MM(),
	}
}
