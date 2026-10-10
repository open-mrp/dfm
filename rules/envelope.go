package rules

import (
	"slices"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// EnvelopeRule fails a part whose bounding box does not fit the usable cut area. A 90° rotation is allowed, so the part's long side is compared with the area's long side and its short side with the area's short side. The first failing pair is reported (long side first).
type EnvelopeRule struct{}

func (EnvelopeRule) Code() constants.FindingCode { return constants.FindingCodePartExceedsEnvelope }

func (EnvelopeRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lc := p.LaserCutting
	areaLong, areaShort := max(lc.UsableWidth, lc.UsableHeight), min(lc.UsableWidth, lc.UsableHeight)
	finding := Finding{
		Code:     constants.FindingCodePartExceedsEnvelope,
		Severity: constants.SeverityError,
		Message:  "Part bounding box exceeds usable cut area",
		// The outline, so the part can be pointed at.
		EntityIDs: part.Outline,
	}
	w, errW := measure.FromMM(part.Bounds.WidthMM())
	h, errH := measure.FromMM(part.Bounds.HeightMM())
	if errW != nil || errH != nil {
		// Too large to represent, so certainly too large to cut.
		finding.Required = dec(areaLong)
		return []Finding{finding}
	}
	partLong, partShort := max(w, h), min(w, h)
	switch {
	case partLong > areaLong:
		finding.Actual, finding.Required = dec(partLong), dec(areaLong)
	case partShort > areaShort:
		finding.Actual, finding.Required = dec(partShort), dec(areaShort)
	default:
		return nil
	}
	return []Finding{finding}
}

// UnitsRule warns when the drawing's size is implausible in the units it was read in but plausible in the other common unit: read in millimeters and under 6 mm across, but within the usable area in inches (an inch drawing read as millimeters, the usual reason a part is "too small to cut"); or read in inches and larger than the usable area, but within it in millimeters. Actual is the drawing's largest dimension as read. Units are not changed: a design's units are confirmed by whoever submits it.
type UnitsRule struct{}

func (UnitsRule) Code() constants.FindingCode { return constants.FindingCodeUnitsSuspect }

// tinyDrawingMM is the size below which a whole drawing is more likely in the wrong units than a real part: about the smallest part fabricators cut.
const tinyDrawingMM = 6

func (UnitsRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if part.Bounds.IsEmpty() {
		return nil
	}
	lc := p.LaserCutting
	areaLong, areaShort := max(lc.UsableWidth, lc.UsableHeight).MM(), min(lc.UsableWidth, lc.UsableHeight).MM()
	long, short := max(part.Bounds.WidthMM(), part.Bounds.HeightMM()), min(part.Bounds.WidthMM(), part.Bounds.HeightMM())
	fits := func(f float64) bool { return long*f <= areaLong && short*f <= areaShort }
	// Suspect only when the other unit gives a plausible size.
	var suspect bool
	var other constants.LengthUnit
	switch part.Units {
	case constants.LengthUnitMillimeter:
		suspect, other = long < tinyDrawingMM && long*25.4 >= tinyDrawingMM && fits(25.4), constants.LengthUnitInch
	case constants.LengthUnitInch:
		suspect, other = !fits(1) && fits(1/25.4) && long/25.4 >= tinyDrawingMM, constants.LengthUnitMillimeter
	}
	if !suspect {
		return nil
	}
	return []Finding{{
		Code:     constants.FindingCodeUnitsSuspect,
		Severity: constants.SeverityWarning,
		Message:  "Drawing's size is implausible in its units; it may be drawn in inches or millimeters instead",
		Actual:   mm(long),
		Fix:      &Fix{Action: constants.FixActionSetUnits, Units: other},
	}}
}

// UnitsAssumedRule warns when part of the drawing's size had to be assumed, such as the size of an SVG pixel, which differs between the programs that write them: whoever submits the design should confirm its size.
type UnitsAssumedRule struct{}

func (UnitsAssumedRule) Code() constants.FindingCode { return constants.FindingCodeUnitsAssumed }

func (UnitsAssumedRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	if !part.UnitsAssumed {
		return nil
	}
	return []Finding{{
		Code:     constants.FindingCodeUnitsAssumed,
		Severity: constants.SeverityWarning,
		Message:  "Drawing's size is in pixels, whose size was assumed from the program that wrote it",
	}}
}

// UnitsUnsupportedRule fails drawings in units the capability does not accept.
type UnitsUnsupportedRule struct{}

func (UnitsUnsupportedRule) Code() constants.FindingCode {
	return constants.FindingCodeUnitsUnsupported
}

func (UnitsUnsupportedRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if p.LaserCutting == nil || len(p.LaserCutting.DrawingUnits) == 0 || part.Units == "" || slices.Contains(p.LaserCutting.DrawingUnits, part.Units) {
		return nil
	}
	return []Finding{{
		Code:     constants.FindingCodeUnitsUnsupported,
		Severity: constants.SeverityError,
		Message:  "Drawing units (" + string(part.Units) + ") are not accepted",
	}}
}
