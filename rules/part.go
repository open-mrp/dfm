package rules

import (
	"math"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// PartWidthRule checks each part's narrowest width over every direction against the part_width limit. Parts narrower than the support spacing can tip or fall through the cutting bed.
type PartWidthRule struct{}

func (PartWidthRule) Code() constants.FindingCode { return constants.FindingCodePartTooSmall }

func (PartWidthRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := p.LaserCutting.Optional.PartWidth
	var out []Finding
	for i, l := range part.Loops {
		if l.Depth != 0 {
			continue
		}
		w, err := measure.FromMM(part.MinWidth(i))
		if err != nil {
			continue
		}
		if sev, req, ok := checkLimit(*lim, p.Thickness, p.LaserCutting.KerfWidth, w); ok {
			out = append(out, limitFinding(constants.FindingCodePartTooSmall, "Part is narrower than configured minimum", sev, req, w, l.EntityIDs))
		}
	}
	return out
}

// PartLengthRule checks each part's length at right angles to its narrowest width against the part_length limit. With part_width it sets a minimum part size such as 0.25 x 0.375 in, the way fabricators publish it.
type PartLengthRule struct{}

func (PartLengthRule) Code() constants.FindingCode { return constants.FindingCodePartTooShort }

func (PartLengthRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := p.LaserCutting.Optional.PartLength
	var out []Finding
	for i, l := range part.Loops {
		if l.Depth != 0 {
			continue
		}
		_, length := part.WidthAndLength(i)
		v, err := measure.FromMM(length)
		if err != nil {
			continue
		}
		if sev, req, ok := checkLimit(*lim, p.Thickness, p.LaserCutting.KerfWidth, v); ok {
			out = append(out, limitFinding(constants.FindingCodePartTooShort, "Part is shorter than configured minimum", sev, req, v, l.EntityIDs))
		}
	}
	return out
}

// PartAreaRule checks each finished part's area (its outline less its cut-outs) against the part_area limit. A part can pass part_width and part_length and still be too small: a ring, a frame or a thin L.
type PartAreaRule struct{}

func (PartAreaRule) Code() constants.FindingCode { return constants.FindingCodePartAreaTooSmall }

func (PartAreaRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := p.LaserCutting.Optional.PartArea
	var out []Finding
	for i, l := range part.Loops {
		if l.Depth != 0 {
			continue
		}
		a, err := measure.AreaFromMM2(part.NetAreaMM2(i))
		if err != nil {
			continue
		}
		var sev constants.Severity
		var req measure.Area
		switch {
		case lim.Error != nil && a < *lim.Error:
			sev, req = constants.SeverityError, *lim.Error
		case lim.Warning != nil && a < *lim.Warning:
			sev, req = constants.SeverityWarning, *lim.Warning
		default:
			continue
		}
		actual, required := a.Decimal(), req.Decimal()
		out = append(out, Finding{
			Code:      constants.FindingCodePartAreaTooSmall,
			Severity:  sev,
			Message:   "Part's area is below configured minimum",
			EntityIDs: l.EntityIDs,
			Actual:    &actual,
			Required:  &required,
			Unit:      constants.UnitSquareMillimeter,
		})
	}
	return out
}

// SlenderPartRule checks each part's narrowest width against the slender_part tier for its length: long narrow strips warp from the heat of cutting, so the longer the part the wider it must be.
type SlenderPartRule struct{}

func (SlenderPartRule) Code() constants.FindingCode { return constants.FindingCodePartTooSlender }

func (SlenderPartRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	tiers := p.LaserCutting.Optional.SlenderPart
	var out []Finding
	for i, l := range part.Loops {
		if l.Depth != 0 {
			continue
		}
		width, length := part.WidthAndLength(i)
		w, errW := measure.FromMM(width)
		ln, errL := measure.FromMM(length)
		if errW != nil || errL != nil {
			continue
		}
		tier := tierFor(tiers, ln)
		if tier == nil {
			continue
		}
		if sev, req, ok := checkLimit(tier.Width, p.Thickness, p.LaserCutting.KerfWidth, w); ok {
			out = append(out, limitFinding(constants.FindingCodePartTooSlender, "Part is too narrow for its length", sev, req, w, l.EntityIDs))
		}
	}
	return out
}

// tierFor is the slender tier for a length: the last whose minimum length it reaches, or nil below the first.
func tierFor(tiers []capability.SlenderTier, length measure.Length) *capability.SlenderTier {
	var tier *capability.SlenderTier
	for k := range tiers {
		if length >= tiers[k].MinLength {
			tier = &tiers[k]
		}
	}
	return tier
}

// SlenderCutoutRule checks each cut-out's width (its narrowest width over every direction) against the slender_cutout tier for its length: long narrow cut-outs let the part warp from the heat of cutting. Round holes are not checked.
type SlenderCutoutRule struct{}

func (SlenderCutoutRule) Code() constants.FindingCode { return constants.FindingCodeCutoutTooSlender }

func (SlenderCutoutRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	tiers := p.LaserCutting.Optional.SlenderCutout
	var out []Finding
	for i, l := range part.Loops {
		if !l.IsCutout() || l.Circle >= 0 {
			continue
		}
		width, length := part.WidthAndLength(i)
		w, errW := measure.FromMM(width)
		ln, errL := measure.FromMM(length)
		if errW != nil || errL != nil {
			continue
		}
		tier := tierFor(tiers, ln)
		if tier == nil {
			continue
		}
		if sev, req, ok := checkLimit(tier.Width, p.Thickness, p.LaserCutting.KerfWidth, w); ok {
			out = append(out, limitFinding(constants.FindingCodeCutoutTooSlender, "Cut-out is too narrow for its length", sev, req, w, l.EntityIDs))
		}
	}
	return out
}

// DensityRule warns when cut-outs remove at least max_removed_area_percent of a part's area. Heavy material removal concentrates heat and releases stress, so the part may distort; the outcome depends on cut order and stock, which is why this is a warning.
type DensityRule struct{}

func (DensityRule) Code() constants.FindingCode { return constants.FindingCodeDensePatternHeatRisk }

func (DensityRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	limit := p.LaserCutting.Optional.MaxRemovedAreaPercent
	var out []Finding
	for i, l := range part.Loops {
		if l.Depth != 0 {
			continue
		}
		removed := percentOf(part.RemovedFraction(i))
		msg := "Cut-outs remove a large share of the part, which may distort it"
		var loc *Location
		if w := p.LaserCutting.Optional.RemovedAreaWindow; removed < *limit && w != nil {
			// The whole part passes: look for a dense patch.
			f, c, ok := part.DensestWindow(i, w.MM())
			if !ok {
				continue
			}
			removed, msg, loc = percentOf(f), "Cut-outs remove a large share of one area of the part, which may distort it", at(c)
		}
		if removed < *limit {
			continue
		}
		actual, required := removed.Decimal(), limit.Decimal()
		out = append(out, Finding{
			Code:      constants.FindingCodeDensePatternHeatRisk,
			Severity:  constants.SeverityWarning,
			Message:   msg,
			EntityIDs: l.EntityIDs,
			Actual:    &actual,
			Required:  &required,
			Unit:      constants.UnitPercent,
			Location:  loc,
		})
	}
	return out
}

// percentOf is a fraction as a percentage in millionths of a percent, rounded to the nearest millionth of a percent.
func percentOf(fraction float64) measure.Ratio {
	return measure.Ratio(math.Round(fraction * 1e8))
}

// PartFallRule warns about a part shorter than the distance between the bed's supports: it can drop between them or tip into the beam when it is cut free, unless the shop tabs it in.
type PartFallRule struct{}

func (PartFallRule) Code() constants.FindingCode { return constants.FindingCodePartMayFallThrough }

func (PartFallRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	spacing := p.LaserCutting.SupportSpacing
	var out []Finding
	for i, l := range part.Loops {
		if l.Depth != 0 {
			continue
		}
		_, length := part.WidthAndLength(i)
		v, err := measure.FromMM(length)
		if err != nil || v >= spacing {
			continue
		}
		out = append(out, Finding{
			Code:      constants.FindingCodePartMayFallThrough,
			Severity:  constants.SeverityWarning,
			Message:   "Part is shorter than the distance between the bed's supports; it can drop or tip when cut free",
			EntityIDs: l.EntityIDs,
			Actual:    dec(v),
			Required:  dec(spacing),
		})
	}
	return out
}

// NotchSlenderRule checks each edge notch (a bay of a part's outline) against the slender_cutout tiers, as SlenderCutoutRule checks closed cut-outs: a long narrow notch leaves long narrow fingers either side, which warp in the heat of the cut.
type NotchSlenderRule struct{}

func (NotchSlenderRule) Code() constants.FindingCode { return constants.FindingCodeNotchTooSlender }

func (NotchSlenderRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	tiers := p.LaserCutting.Optional.SlenderCutout
	var out []Finding
	for i, l := range part.Loops {
		if l.Depth != 0 || l.Circle >= 0 {
			continue
		}
		for _, n := range part.Notches(i) {
			w, errW := measure.FromMM(n.WidthMM)
			ln, errL := measure.FromMM(n.LengthMM)
			if errW != nil || errL != nil {
				continue
			}
			tier := tierFor(tiers, ln)
			if tier == nil {
				continue
			}
			if sev, req, ok := checkLimit(tier.Width, p.Thickness, p.LaserCutting.KerfWidth, w); ok {
				fd := limitFinding(constants.FindingCodeNotchTooSlender, "Edge notch is too narrow for its length", sev, req, w, l.EntityIDs)
				fd.Location = at(n.Deepest)
				fd.Span = span(n.Mouth[0], n.Mouth[1])
				out = append(out, fd)
			}
		}
	}
	return out
}
