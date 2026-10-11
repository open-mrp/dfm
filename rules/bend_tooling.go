package rules

import (
	"math"
	"slices"
	"strings"

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

// BendCollisionRule simulates forming the bends on the press brake's tooling and fails a part no bend order forms without running it into the punch, the die or the ram above the punch (see package brake). It reports from the part's BendPlan when the check has set one.
type BendCollisionRule struct{}

func (BendCollisionRule) Code() constants.FindingCode { return constants.FindingCodeBendToolCollision }

// collisionBudget is how many bends the sequence search forms in all before it stops and asks for review.
const collisionBudget = 1500

func (BendCollisionRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	plan := part.BendPlan
	if plan == nil {
		plan = PlanBends(part, p)
	}
	if plan == nil || plan.Found {
		return nil
	}
	lines := map[string]geometry.BendLine{}
	for _, b := range part.BendLines() {
		lines[b.EntityID] = b
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
		f := finding(lines[plan.EntityIDs[0]], constants.SeverityReview, "No bend order clear of the tooling was found before the search stopped; whether one exists is not known")
		f.EntityIDs, f.Location, f.Span = slices.Clone(plan.EntityIDs), nil, nil
		return []Finding{f}
	}
	var out []Finding
	for _, b := range plan.Blocked {
		var hit []string
		if b.Punch {
			hit = append(hit, "the punch")
		}
		if b.Die {
			hit = append(hit, "the die")
		}
		if b.Ram {
			hit = append(hit, "the ram above the punch")
		}
		msg := "No bend order forms the part: forming this bend runs it into " + strings.Join(hit, " and ")
		out = append(out, finding(lines[b.EntityID], constants.SeverityError, msg))
	}
	return out
}

// PlanBends searches for an order that forms every bend of the part clear of the press brake's tooling (see brake.Model.Sequence), and says how each bend was formed or which could not be. It is nil when the capability gives no tooling or the part has nothing to bend.
func PlanBends(part *geometry.Part, p *capability.Profile) *geometry.BendPlan {
	if p.Bending == nil || p.Bending.Tooling == nil {
		return nil
	}
	m, lines := foldModel(part, p)
	if m == nil {
		return nil
	}
	tool := toolingMM(p.Bending.Tooling)
	plan := m.Sequence(tool, collisionBudget)
	out := &geometry.BendPlan{Found: plan.Found, Exhausted: plan.Exhausted, Forming: map[string]geometry.BendForming{}, Blocked: []geometry.BendBlocked{}}
	for _, b := range lines {
		out.EntityIDs = append(out.EntityIDs, b.EntityID)
	}
	if plan.Found {
		positions := map[brake.Place]constants.PunchPosition{brake.PlaceStart: constants.PunchPositionStart, brake.PlaceMiddle: constants.PunchPositionMiddle, brake.PlaceEnd: constants.PunchPositionEnd}
		for i, bi := range plan.Order {
			step, line := plan.Steps[i], lines[bi]
			f := geometry.BendForming{Sequence: i + 1, PunchPosition: positions[step.Place]}
			if tool.PunchThroatDepth > 0 {
				f.ThroatSide = throatSide(line, step.Mirrored)
			}
			out.Forming[line.EntityID] = f
		}
		return out
	}
	var blocked []int
	for bi := range plan.Blocked {
		blocked = append(blocked, bi)
	}
	slices.Sort(blocked)
	for _, bi := range blocked {
		r := plan.Blocked[bi]
		out.Blocked = append(out.Blocked, geometry.BendBlocked{EntityID: lines[bi].EntityID, Punch: r.Punch, Die: r.Die, Ram: r.Ram})
	}
	return out
}

// throatSide is the side of the bend line, from its start to its end, that a gooseneck's throat opens toward. The punch stands on the inside of the bend, and its throat opens to the right of the line for a bend up (toward the viewer) unless the punch faced the other way round; down, the inside is under the sheet and the sides swap (see brake.Tooling.punch).
func throatSide(b geometry.BendLine, mirrored bool) constants.LineSide {
	up := b.Spec.Direction != constants.BendDirectionDown
	if up != mirrored {
		return constants.LineSideRight
	}
	return constants.LineSideLeft
}

// toolingMM converts the capability's tooling for the simulation.
func toolingMM(t *capability.ResolvedTooling) brake.Tooling {
	return brake.Tooling{
		PunchTipAngle: float64(t.PunchTipAngle) / 1e6, PunchWidth: t.PunchWidth.MM(), PunchHeight: t.PunchHeight.MM(), PunchLength: t.PunchLength.MM(),
		PunchNose: t.PunchNose.MM(), PunchThroatDepth: t.PunchThroatDepth.MM(), PunchThroatHeight: t.PunchThroatHeight.MM(),
		DieOpening: t.DieOpeningMM.Float64(), DieAngle: float64(t.DieAngle) / 1e6, DieWidth: t.DieWidth.MM(), DieHeight: t.DieHeight.MM(), DieLength: t.DieLength.MM(),
	}
}
