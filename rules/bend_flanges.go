package rules

import (
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// FlangeRule checks the shorter flange beside each bend against the flange_length limit: a flange too short to reach across the die cannot be bent.
type FlangeRule struct{}

func (FlangeRule) Code() constants.FindingCode { return constants.FindingCodeFlangeTooShort }

func (FlangeRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	var out []Finding
	// A flange between two bends is reported once, for the first bend.
	between := map[[2]string]bool{}
	for _, b := range part.BendLines() {
		left, right, ok := part.Flanges(b)
		if !ok {
			continue
		}
		f := left
		if right.LengthMM < left.LengthMM {
			f = right
		}
		d, err := measure.FromMM(f.LengthMM)
		if err != nil {
			continue
		}
		if sev, req, ok := checkBendLimit(*p.Bending.FlangeLength, p, d); ok {
			msg, ids := "Flange beside the bend is shorter than configured minimum", []string{b.EntityID}
			if f.ToBend != "" {
				pair := [2]string{min(b.EntityID, f.ToBend), max(b.EntityID, f.ToBend)}
				if between[pair] {
					continue
				}
				between[pair] = true
				msg, ids = "Flange between bends is shorter than configured minimum", append(ids, f.ToBend)
			}
			fd := limitFinding(constants.FindingCodeFlangeTooShort, msg, sev, req, d, ids)
			fd.Location, fd.Span = at(f.From), span(f.From, f.To)
			out = append(out, fd)
		}
	}
	return out
}

// BendFeatureRule checks the distance from each bend line to the holes and cut-outs near it against the feature_distance limit: the bend stretches material near it, distorting them.
type BendFeatureRule struct{}

func (BendFeatureRule) Code() constants.FindingCode {
	return constants.FindingCodeFeatureTooCloseToBend
}

func (BendFeatureRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := *p.Bending.FeatureDistance
	var out []Finding
	for _, b := range part.BendLines() {
		for _, f := range part.BendFeatures(b, bendSearchRadius(lim, p)) {
			d, err := measure.FromMM(f.DistanceMM)
			if err != nil {
				continue
			}
			if sev, req, ok := checkBendLimit(lim, p, d); ok {
				ids := append([]string{b.EntityID}, part.Loops[f.Loop].EntityIDs...)
				fd := limitFinding(constants.FindingCodeFeatureTooCloseToBend, "Hole or cut-out is closer to the bend than configured minimum", sev, req, d, ids)
				fd.Location, fd.Span = at(f.OnFeature), span(f.OnBend, f.OnFeature)
				out = append(out, fd)
			}
		}
	}
	return out
}

// BendZonesRule fails neighboring bends whose bend zones overlap: each takes up its allowance of flat material, centered on its line.
type BendZonesRule struct{}

func (BendZonesRule) Code() constants.FindingCode { return constants.FindingCodeBendZonesOverlap }

func (BendZonesRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if p.Bending == nil {
		return nil
	}
	var out []Finding
	seen := map[[2]string]bool{}
	for _, b := range part.BendLines() {
		left, right, ok := part.Flanges(b)
		if !ok {
			continue
		}
		for _, f := range []geometry.Flange{left, right} {
			if f.ToBend == "" {
				continue
			}
			pair := [2]string{min(b.EntityID, f.ToBend), max(b.EntityID, f.ToBend)}
			if seen[pair] {
				continue
			}
			seen[pair] = true
			need := (b.Spec.AllowanceMM + part.BendSpecs[f.ToBend].AllowanceMM) / 2
			if f.LengthMM >= need {
				continue
			}
			d, err1 := measure.FromMM(f.LengthMM)
			r, err2 := measure.FromMM(need)
			if err1 != nil || err2 != nil {
				continue
			}
			out = append(out, Finding{
				Code:      constants.FindingCodeBendZonesOverlap,
				Severity:  constants.SeverityError,
				Message:   "Bends are closer together than their bend zones are wide",
				EntityIDs: []string{b.EntityID, f.ToBend},
				Actual:    dec(d),
				Required:  dec(r),
				Location:  at(f.From),
				Span:      span(f.From, f.To),
			})
		}
	}
	return out
}

// bendRequirement resolves one tier of a bending limit for the stock: the threshold, plus the inside radius the tooling forms when the limit says.
func bendRequirement(l capability.BendLimit, t *measure.Threshold, p *capability.Profile) measure.Requirement {
	r := t.Required(p.Thickness, 0)
	if l.PlusInsideRadius {
		r.Value = r.Value.Add(p.Bending.InsideRadius)
	}
	return r
}

// checkBendLimit is checkLimit for a bending limit.
func checkBendLimit(l capability.BendLimit, p *capability.Profile, actual measure.Length) (constants.Severity, measure.Requirement, bool) {
	for _, tier := range []struct {
		t   *measure.Threshold
		sev constants.Severity
	}{{l.Error, constants.SeverityError}, {l.Warning, constants.SeverityWarning}} {
		if tier.t == nil {
			continue
		}
		if r := bendRequirement(l, tier.t, p); r.Below(actual) {
			return tier.sev, r, true
		}
	}
	return "", measure.Requirement{}, false
}

// bendSearchRadius is searchRadius for a bending limit.
func bendSearchRadius(l capability.BendLimit, p *capability.Profile) float64 {
	r := searchRadius(l.Limit, p.Thickness, 0)
	if l.PlusInsideRadius {
		r += p.Bending.InsideRadius.Float64()
	}
	return r
}
