package rules

import (
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// WebRule checks the material left between two cut-outs in the same region against the web_width limit. A thin web is heated from both sides and distorts or burns away. Each cut-out reports only its narrowest web.
type WebRule struct{}

func (WebRule) Code() constants.FindingCode { return constants.FindingCodeWebTooThin }

func (WebRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	return siblingFindings(part, p, p.LaserCutting.Optional.WebWidth, constants.FindingCodeWebTooThin, "Material between cut-outs is below configured minimum")
}

// FeatureSpacingRule checks the gap between two cut-outs in the same region against the feature_spacing limit. It measures the same gap as WebRule against a separately configured spacing rule.
type FeatureSpacingRule struct{}

func (FeatureSpacingRule) Code() constants.FindingCode { return constants.FindingCodeFeaturesTooClose }

func (FeatureSpacingRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	return siblingFindings(part, p, p.LaserCutting.Optional.FeatureSpacing, constants.FindingCodeFeaturesTooClose, "Cut-outs are closer than configured minimum spacing")
}

func siblingFindings(part *geometry.Part, p *capability.Profile, lim *capability.Limit, code constants.FindingCode, msg string) []Finding {
	lc := p.LaserCutting
	clearances := part.SiblingClearances(searchRadius(*lim, p.Thickness, lc.KerfWidth))

	// Keep, for every cut-out, only its narrowest gap; a pair is reported once even when it is the narrowest for both of its cut-outs.
	nearest := map[int]int{}
	for i, c := range clearances {
		for _, loop := range []int{c.A, c.B} {
			if j, ok := nearest[loop]; !ok || c.DistanceMM < clearances[j].DistanceMM {
				nearest[loop] = i
			}
		}
	}
	keep := map[int]bool{}
	for _, i := range nearest {
		keep[i] = true
	}

	var out []Finding
	for i, c := range clearances {
		if !keep[i] {
			continue
		}
		d, err := measure.FromMM(c.DistanceMM)
		if err != nil || d <= contactLength {
			continue
		}
		if sev, req, ok := checkLimit(*lim, p.Thickness, lc.KerfWidth, d); ok {
			ids := append(append([]string(nil), part.Loops[c.A].EntityIDs...), part.Loops[c.B].EntityIDs...)
			f := limitFinding(code, msg, sev, req, d, ids)
			f.Location, f.Span = at(c.Midpoint()), span(c.PointA, c.PointB)
			out = append(out, f)
		}
	}
	return out
}

// EdgeDistanceRule checks the material between each cut-out and the boundary of the region around it against the edge_distance limit.
type EdgeDistanceRule struct{}

func (EdgeDistanceRule) Code() constants.FindingCode {
	return constants.FindingCodeFeatureTooCloseToEdge
}

func (EdgeDistanceRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := p.LaserCutting.Optional.EdgeDistance
	lc := p.LaserCutting
	radius := searchRadius(*lim, p.Thickness, lc.KerfWidth)
	var out []Finding
	for i, l := range part.Loops {
		if !l.IsCutout() {
			continue
		}
		c, ok := part.LoopClearance(i, l.Parent, radius)
		if !ok {
			continue
		}
		d, err := measure.FromMM(c.DistanceMM)
		if err != nil || d <= contactLength {
			continue
		}
		if sev, req, ok := checkLimit(*lim, p.Thickness, lc.KerfWidth, d); ok {
			ids := append(append([]string(nil), l.EntityIDs...), part.Loops[l.Parent].EntityIDs...)
			f := limitFinding(constants.FindingCodeFeatureTooCloseToEdge, "Cut-out is closer to the edge than configured minimum", sev, req, d, ids)
			f.Location, f.Span = at(c.Midpoint()), span(c.PointA, c.PointB)
			out = append(out, f)
		}
	}
	return out
}
