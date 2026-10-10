package rules

import (
	"math"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// CutoutWidthRule checks the narrowest passage within a cut-out or edge notch against the cutout_width limit. Recognized slots and round holes are left to their own rules.
type CutoutWidthRule struct{}

func (CutoutWidthRule) Code() constants.FindingCode { return constants.FindingCodeCutoutTooNarrow }

func (CutoutWidthRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := p.LaserCutting.Optional.CutoutWidth
	// Slots have their own limit.
	slots := map[int]bool{}
	for _, s := range part.Slots {
		slots[s.Contour] = true
	}
	return neckFindings(part, p, *lim, geometry.NeckVoid, constants.FindingCodeCutoutTooNarrow, "Cut-out is narrower than configured minimum",
		func(n geometry.Neck) bool { return !slots[part.Loops[n.Loop].Contour] })
}

// FeatureWidthRule checks the narrowest strip of material within one outline (a finger, tab or tongue) against the feature_width limit. When the strip is the whole part's narrowest width and part_width is set, the part_too_small finding already covers it.
type FeatureWidthRule struct{}

func (FeatureWidthRule) Code() constants.FindingCode { return constants.FindingCodeFeatureTooThin }

func (FeatureWidthRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := p.LaserCutting.Optional.FeatureWidth
	coveredByPartWidth := p.LaserCutting.Optional.PartWidth != nil
	return neckFindings(part, p, *lim, geometry.NeckMaterial, constants.FindingCodeFeatureTooThin, "Material feature is thinner than configured minimum",
		func(n geometry.Neck) bool {
			if !coveredByPartWidth || part.Loops[n.Loop].Depth != 0 {
				return true
			}
			neck, err1 := measure.FromMM(n.DistanceMM)
			width, err2 := measure.FromMM(part.MinWidth(n.Loop))
			return err1 != nil || err2 != nil || neck != width
		})
}

func neckFindings(part *geometry.Part, p *capability.Profile, lim capability.Limit, kind geometry.NeckKind, code constants.FindingCode, msg string, keep func(geometry.Neck) bool) []Finding {
	lc := p.LaserCutting
	// One search serves both width rules: at the larger radius either asks for, since the narrowest neck within a radius is the same within any larger one.
	radius := searchRadius(lim, p.Thickness, lc.KerfWidth)
	for _, other := range []*capability.Limit{lc.Optional.CutoutWidth, lc.Optional.FeatureWidth} {
		if other != nil {
			radius = max(radius, searchRadius(*other, p.Thickness, lc.KerfWidth))
		}
	}
	own := searchRadius(lim, p.Thickness, lc.KerfWidth)
	var out []Finding
	for _, n := range part.Necks(radius) {
		if n.Kind != kind || !keep(n) || n.DistanceMM > own {
			continue
		}
		d, err := measure.FromMM(n.DistanceMM)
		if err != nil {
			continue
		}
		if sev, req, ok := checkLimit(lim, p.Thickness, lc.KerfWidth, d); ok {
			f := limitFinding(code, msg, sev, req, d, part.Loops[n.Loop].EntityIDs)
			f.Location, f.Span = at(n.Midpoint()), span(n.PointA, n.PointB)
			out = append(out, f)
		}
	}
	return out
}

// CornerRule checks each outline's sharpest inside corner of the material against the internal_corner_radius limit. A sharp corner has radius 0; the finding points at the sharpest corner.
type CornerRule struct{}

func (CornerRule) Code() constants.FindingCode { return constants.FindingCodeInternalCornerTooSharp }

func (CornerRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := p.LaserCutting.Optional.InternalCornerRadius
	sharpest := map[int]geometry.Corner{}
	var order []int
	for _, c := range part.ConcaveCorners() {
		cur, seen := sharpest[c.Loop]
		if !seen {
			order = append(order, c.Loop)
		}
		if !seen || c.RadiusMM < cur.RadiusMM {
			sharpest[c.Loop] = c
		}
	}
	var out []Finding
	for _, li := range order {
		c := sharpest[li]
		r, err := measure.FromMM(c.RadiusMM)
		if err != nil {
			continue
		}
		if sev, req, ok := checkLimit(*lim, p.Thickness, p.LaserCutting.KerfWidth, r); ok {
			f := limitFinding(constants.FindingCodeInternalCornerTooSharp, "Inside corner is sharper than configured minimum radius", sev, req, r, part.Loops[li].EntityIDs)
			f.Location = at(c.At)
			out = append(out, f)
		}
	}
	return out
}

// TipRule warns when an outline comes to a point sharper than min_tip_angle, on the material side (a spike) or the cut-out side (a narrow V). The beam rounds or burns away such a point rather than failing to cut it, which is why this is a warning. The finding points at each outline's sharpest corner.
type TipRule struct{}

func (TipRule) Code() constants.FindingCode { return constants.FindingCodeTipTooSharp }

func (TipRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	limit := p.LaserCutting.Optional.MinTipAngle
	var out []Finding
	for _, t := range part.SharpestTips() {
		angle := measure.Ratio(math.Round(t.AngleDeg * 1e6))
		if angle >= *limit {
			continue
		}
		actual, required := angle.Decimal(), limit.Decimal()
		out = append(out, Finding{
			Code:      constants.FindingCodeTipTooSharp,
			Severity:  constants.SeverityWarning,
			Message:   "Corner comes to a sharper point than configured minimum angle",
			EntityIDs: part.Loops[t.Loop].EntityIDs,
			Actual:    &actual,
			Required:  &required,
			Unit:      constants.UnitDegree,
			Location:  at(t.At),
		})
	}
	return out
}
