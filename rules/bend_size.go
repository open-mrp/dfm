package rules

import (
	"math"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// BendLengthRule fails a bend longer than the press brake bends.
type BendLengthRule struct{}

func (BendLengthRule) Code() constants.FindingCode { return constants.FindingCodeBendTooLong }

func (BendLengthRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if p.Bending == nil {
		return nil
	}
	var out []Finding
	for _, b := range part.BendLines() {
		l, err := measure.FromMM(b.Length())
		if err != nil || l <= p.Bending.MaxBendLength {
			continue
		}
		out = append(out, Finding{
			Code:      constants.FindingCodeBendTooLong,
			Severity:  constants.SeverityError,
			Message:   "Bend is longer than the press brake bends",
			EntityIDs: []string{b.EntityID},
			Actual:    dec(l),
			Required:  dec(p.Bending.MaxBendLength),
			Location:  at(b.Midpoint()),
			Span:      span(b.A, b.B),
		})
	}
	return out
}

// BendShortRule fails a bend shorter than the tooling holds.
type BendShortRule struct{}

func (BendShortRule) Code() constants.FindingCode { return constants.FindingCodeBendTooShort }

func (BendShortRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	var out []Finding
	for _, b := range part.BendLines() {
		l, err := measure.FromMM(b.Length())
		if err != nil || l >= p.Bending.MinBendLength {
			continue
		}
		out = append(out, Finding{
			Code:      constants.FindingCodeBendTooShort,
			Severity:  constants.SeverityError,
			Message:   "Bend is shorter than the tooling holds",
			EntityIDs: []string{b.EntityID},
			Actual:    dec(l),
			Required:  dec(p.Bending.MinBendLength),
			Location:  at(b.Midpoint()),
			Span:      span(b.A, b.B),
		})
	}
	return out
}

// kNPerTonne converts metric tonnes-force to kilonewtons.
const kNPerTonne = 9.80665

// BendForce returns the air-bending force for a bend length long in stock thickness thick, scaled by the material's force factor: the common rule 60 t² L / V tonnes (t in mm, L in m, V the die opening in mm) with a die opening of 8 t, which is 7.5 t L tonnes for mild steel.
func BendForce(thick, length measure.Length, factor measure.Ratio) float64 {
	tonnes := 7.5 * thick.MM() * (length.MM() / 1000)
	return tonnes * kNPerTonne * (float64(factor) / 1e6)
}

// BendForceRule fails a bend that needs more force than the press brake has (see BendForce).
type BendForceRule struct{}

func (BendForceRule) Code() constants.FindingCode { return constants.FindingCodeBendForceTooHigh }

func (BendForceRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	var out []Finding
	for _, b := range part.BendLines() {
		l, err := measure.FromMM(b.Length())
		if err != nil {
			continue
		}
		need := measure.Ratio(math.Round(BendForce(p.Thickness, l, p.Bending.ForceFactor) * 1e6))
		if need <= p.Bending.MaxForce {
			continue
		}
		actual, required := need.Decimal(), p.Bending.MaxForce.Decimal()
		out = append(out, Finding{
			Code:      constants.FindingCodeBendForceTooHigh,
			Severity:  constants.SeverityError,
			Message:   "Bend needs more force than the press brake has",
			EntityIDs: []string{b.EntityID},
			Actual:    &actual,
			Required:  &required,
			Unit:      constants.UnitKilonewton,
			Location:  at(b.Midpoint()),
			Span:      span(b.A, b.B),
		})
	}
	return out
}

// BendAngleRule fails a bend sharper than the tooling forms.
type BendAngleRule struct{}

func (BendAngleRule) Code() constants.FindingCode { return constants.FindingCodeBendAngleTooLarge }

func (BendAngleRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	var out []Finding
	for _, b := range part.BendLines() {
		if b.Spec.Angle <= p.Bending.MaxAngle {
			continue
		}
		actual, required := b.Spec.Angle.Decimal(), p.Bending.MaxAngle.Decimal()
		out = append(out, Finding{
			Code:      constants.FindingCodeBendAngleTooLarge,
			Severity:  constants.SeverityError,
			Message:   "Bend angle is larger than the tooling forms",
			EntityIDs: []string{b.EntityID},
			Actual:    &actual,
			Required:  &required,
			Unit:      constants.UnitDegree,
			Location:  at(b.Midpoint()),
			Span:      span(b.A, b.B),
		})
	}
	return out
}

// BendAngleMinRule fails a bend shallower than the brake holds.
type BendAngleMinRule struct{}

func (BendAngleMinRule) Code() constants.FindingCode { return constants.FindingCodeBendAngleTooSmall }

func (BendAngleMinRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	var out []Finding
	for _, b := range part.BendLines() {
		if b.Spec.Angle >= p.Bending.MinAngle {
			continue
		}
		actual, required := b.Spec.Angle.Decimal(), p.Bending.MinAngle.Decimal()
		out = append(out, Finding{
			Code:      constants.FindingCodeBendAngleTooSmall,
			Severity:  constants.SeverityError,
			Message:   "Bend angle is smaller than the brake holds",
			EntityIDs: []string{b.EntityID},
			Actual:    &actual,
			Required:  &required,
			Unit:      constants.UnitDegree,
			Location:  at(b.Midpoint()),
			Span:      span(b.A, b.B),
		})
	}
	return out
}

// radiusMatchMM is how far a design's inside radius may be from the radius the tooling forms and still count as the same.
const radiusMatchMM = 0.01

// BendRadiusRule warns about a bend drawn for an inside radius the tooling does not form: it is made at the tooling's radius, which changes the bent part's dimensions.
type BendRadiusRule struct{}

func (BendRadiusRule) Code() constants.FindingCode { return constants.FindingCodeBendRadiusNotFormed }

func (BendRadiusRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if p.Bending == nil {
		return nil
	}
	var out []Finding
	for _, b := range part.BendLines() {
		if !b.Spec.RadiusGiven || math.Abs(b.Spec.InsideRadius.Float64()-p.Bending.InsideRadius.Float64()) <= radiusMatchMM {
			continue
		}
		actual, required := b.Spec.InsideRadius, p.Bending.InsideRadius
		out = append(out, Finding{
			Code:      constants.FindingCodeBendRadiusNotFormed,
			Severity:  constants.SeverityWarning,
			Message:   "Bend is drawn for an inside radius the tooling does not form; it is made at the tooling's radius",
			EntityIDs: []string{b.EntityID},
			Actual:    &actual,
			Required:  &required,
			Location:  at(b.Midpoint()),
			Span:      span(b.A, b.B),
		})
	}
	return out
}

// BendRadiusMinRule checks the inside radius each bend is drawn for against the min_inside_radius limit.
type BendRadiusMinRule struct{}

func (BendRadiusMinRule) Code() constants.FindingCode { return constants.FindingCodeBendRadiusTooSmall }

func (BendRadiusMinRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := *p.Bending.MinInsideRadius
	lim.PlusInsideRadius = false
	var out []Finding
	for _, b := range part.BendLines() {
		if !b.Spec.RadiusGiven {
			continue
		}
		r, err := measure.FromMM(b.Spec.InsideRadius.Float64())
		if err != nil {
			continue
		}
		if sev, req, ok := checkBendLimit(lim, p, r); ok {
			fd := limitFinding(constants.FindingCodeBendRadiusTooSmall, "Bend is drawn tighter than configured minimum inside radius", sev, req, r, []string{b.EntityID})
			fd.Location, fd.Span = at(b.Midpoint()), span(b.A, b.B)
			out = append(out, fd)
		}
	}
	return out
}
