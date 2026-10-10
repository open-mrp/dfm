package rules

import (
	"math"
	"slices"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// HoleOpNotAHoleRule reports hole assignments that select something other than a round hole: an entity that is not one, or a diameter no round hole is drawn at.
type HoleOpNotAHoleRule struct{}

func (HoleOpNotAHoleRule) Code() constants.FindingCode {
	return constants.FindingCodeHoleOperationNotAHole
}

func (HoleOpNotAHoleRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, m := range part.HoleOpMisses {
		f := Finding{
			Code:      constants.FindingCodeHoleOperationNotAHole,
			Severity:  constants.SeverityError,
			Message:   "Hole operation selects no round hole",
			EntityIDs: m.EntityIDs,
		}
		if m.Location != nil {
			f.Location = at(*m.Location)
		}
		if m.DiameterMM > 0 {
			f.Actual = mm(m.DiameterMM)
		}
		out = append(out, f)
	}
	return out
}

// HoleOpConflictRule reports holes that more than one assignment selects.
type HoleOpConflictRule struct{}

func (HoleOpConflictRule) Code() constants.FindingCode {
	return constants.FindingCodeHoleOperationConflict
}

func (HoleOpConflictRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, c := range part.HoleOpConflicts {
		out = append(out, Finding{
			Code:      constants.FindingCodeHoleOperationConflict,
			Severity:  constants.SeverityError,
			Message:   "Hole is given more than one operation",
			EntityIDs: c.EntityIDs,
			Location:  at(c.Center),
		})
	}
	return out
}

// HoleOpUnavailableRule reports operations that are not offered for the stock's material and thickness.
type HoleOpUnavailableRule struct{}

func (HoleOpUnavailableRule) Code() constants.FindingCode {
	return constants.FindingCodeHoleOperationUnavailable
}

func (HoleOpUnavailableRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	var out []Finding
	for _, h := range part.HoleOps {
		if p.HoleOperations[h.Code].Available {
			continue
		}
		out = append(out, Finding{
			Code:      constants.FindingCodeHoleOperationUnavailable,
			Severity:  constants.SeverityError,
			Message:   "Hole operation is not offered for this material and thickness",
			EntityIDs: h.EntityIDs,
			Actual:    dec(p.Thickness),
			Location:  at(h.Center),
		})
	}
	return out
}

// HoleResizedRule reports holes resized to their operation's hole size; every other check sees the new size. Actual is the drawn diameter and Required the new one.
type HoleResizedRule struct{}

func (HoleResizedRule) Code() constants.FindingCode { return constants.FindingCodeHoleResized }

func (HoleResizedRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, h := range part.HoleOps {
		drawn, err1 := measure.FromMM(2 * h.DrawnRadius)
		size, err2 := measure.FromMM(2 * h.Radius)
		if err1 != nil || err2 != nil || drawn == size {
			continue
		}
		out = append(out, Finding{
			Code:      constants.FindingCodeHoleResized,
			Severity:  constants.SeverityWarning,
			Message:   "Hole is resized for its operation",
			EntityIDs: h.EntityIDs,
			Actual:    dec(drawn),
			Required:  dec(size),
			Location:  at(h.Center),
		})
	}
	return out
}

// HoleOpEdgeRule checks the distance from each operated hole's center to the nearest other cut against the operation's edge_distance.
type HoleOpEdgeRule struct{}

func (HoleOpEdgeRule) Code() constants.FindingCode {
	return constants.FindingCodeHoleOperationTooCloseToEdge
}

func (HoleOpEdgeRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	var out []Finding
	for _, h := range part.HoleOps {
		lim := p.HoleOperations[h.Code].EdgeDistance
		if lim == nil {
			continue
		}
		d, err := measure.FromMM(part.Clearance(h.Center, h.EntityIDs))
		if err != nil {
			continue
		}
		if sev, req, ok := checkLimit(*lim, p.Thickness, kerfOf(p), d); ok {
			f := limitFinding(constants.FindingCodeHoleOperationTooCloseToEdge, "Hole with an operation is too close to another cut", sev, req, d, h.EntityIDs)
			f.Location = at(h.Center)
			out = append(out, f)
		}
	}
	return out
}

// HoleOpSpacingRule checks the distance between the centers of two operated holes against the larger of their operations' spacing.
type HoleOpSpacingRule struct{}

func (HoleOpSpacingRule) Code() constants.FindingCode {
	return constants.FindingCodeHoleOperationsTooClose
}

func (HoleOpSpacingRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	var out []Finding
	ops := part.HoleOps
	for i := range ops {
		for j := i + 1; j < len(ops); j++ {
			a, b := ops[i], ops[j]
			d, err := measure.FromMM(a.Center.Dist(b.Center))
			if err != nil {
				continue
			}
			var worst *Finding
			for _, code := range []string{a.Code, b.Code} {
				lim := p.HoleOperations[code].Spacing
				if lim == nil {
					continue
				}
				sev, req, ok := checkLimit(*lim, p.Thickness, kerfOf(p), d)
				if !ok {
					continue
				}
				f := limitFinding(constants.FindingCodeHoleOperationsTooClose, "Holes with operations are too close together", sev, req, d, append(append([]string{}, a.EntityIDs...), b.EntityIDs...))
				if worst == nil || sev == constants.SeverityError && worst.Severity != constants.SeverityError ||
					sev == worst.Severity && worst.Required.Cmp(*f.Required) < 0 {
					worst = &f
				}
			}
			if worst != nil {
				worst.Location = at(geometry.Point{X: (a.Center.X + b.Center.X) / 2, Y: (a.Center.Y + b.Center.Y) / 2})
				worst.Span = span(a.Center, b.Center)
				out = append(out, *worst)
			}
		}
	}
	return out
}

// CountersinkDepthRule checks that each countersink's cone, from the hole to its major diameter, is no deeper than the allowed share of the thickness.
type CountersinkDepthRule struct{}

func (CountersinkDepthRule) Code() constants.FindingCode {
	return constants.FindingCodeCountersinkTooDeep
}

func (CountersinkDepthRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	var out []Finding
	for _, h := range part.HoleOps {
		cs := p.HoleOperations[h.Code].Countersink
		if cs == nil {
			continue
		}
		half := float64(cs.AngleDeg) / 1e6 / 2 * math.Pi / 180
		depth, err := measure.FromMM((cs.MajorDiameter.MM()/2 - h.Radius) / math.Tan(half))
		if err != nil {
			continue
		}
		limit := measure.Length(math.Round(float64(p.Thickness) * float64(cs.MaxDepthRatio) / 1e6))
		if depth <= limit {
			continue
		}
		basis := constants.ThresholdBasisThicknessRatio
		out = append(out, Finding{
			Code:          constants.FindingCodeCountersinkTooDeep,
			Severity:      constants.SeverityError,
			Message:       "Countersink is deeper than the allowed share of the thickness",
			EntityIDs:     h.EntityIDs,
			Actual:        dec(depth),
			Required:      dec(limit),
			RequiredBasis: &basis,
			Location:      at(h.Center),
		})
	}
	return out
}

func kerfOf(p *capability.Profile) measure.Length {
	if p.LaserCutting == nil {
		return 0
	}
	return p.LaserCutting.KerfWidth
}

// HoleOpBendRule checks the holes of one kind of operation (the plan step it runs for) against their operation's bend_distance: the distance from the hole's edge to the nearest bend line.
type HoleOpBendRule struct{ Type constants.HoleOperationType }

func (HoleOpBendRule) Code() constants.FindingCode {
	return constants.FindingCodeHoleOperationTooCloseToBend
}

func (r HoleOpBendRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	bends := part.BendLines()
	if len(bends) == 0 {
		return nil
	}
	var kerf measure.Length
	if p.LaserCutting != nil {
		kerf = p.LaserCutting.KerfWidth
	}
	var out []Finding
	for _, op := range part.HoleOps {
		res := p.HoleOperations[op.Code]
		if res == nil || res.Type != r.Type || res.BendDistance == nil {
			continue
		}
		best, nearest := math.Inf(1), geometry.BendLine{}
		var on geometry.Point
		for _, b := range bends {
			q := closestOnSegment(op.Center, b.A, b.B)
			if d := op.Center.Dist(q) - op.Radius; d < best {
				best, nearest, on = d, b, q
			}
		}
		d, err := measure.FromMM(max(best, 0))
		if err != nil {
			continue
		}
		if sev, req, ok := checkLimit(*res.BendDistance, p.Thickness, kerf, d); ok {
			fd := limitFinding(constants.FindingCodeHoleOperationTooCloseToBend, "Hole with an operation is closer to a bend than the operation allows", sev, req, d, append(slices.Clone(op.EntityIDs), nearest.EntityID))
			fd.Location, fd.Span = at(op.Center), span(op.Center, on)
			out = append(out, fd)
		}
	}
	return out
}

// closestOnSegment is the point of segment a-b nearest q.
func closestOnSegment(q, a, b geometry.Point) geometry.Point {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := float64(dx*dx) + float64(dy*dy)
	if l2 == 0 {
		return a
	}
	t := (float64((q.X-a.X)*dx) + float64((q.Y-a.Y)*dy)) / l2
	t = max(0, min(1, t))
	return geometry.Point{X: a.X + float64(t*dx), Y: a.Y + float64(t*dy)}
}
