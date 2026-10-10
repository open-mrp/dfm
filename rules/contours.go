package rules

import (
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

// OpenContourRule fails every cut path whose ends do not meet. Endpoints closer than geometry.EndpointTolerance are considered joined, and gaps within the capability's closure tolerance were already bridged. Actual is the gap that keeps the path open: from one of its ends to the nearest other loose end; Location is the middle of that gap.
type OpenContourRule struct{}

func (OpenContourRule) Code() constants.FindingCode { return constants.FindingCodeGeometryOpenContour }

func (OpenContourRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, o := range part.Openings {
		out = append(out, Finding{
			Code:      constants.FindingCodeGeometryOpenContour,
			Severity:  constants.SeverityError,
			Message:   "Cut contour is not closed",
			EntityIDs: part.Contours[o.Contour].EntityIDs(),
			Actual:    mm(o.GapMM),
			Location:  at(o.Location),
		})
	}
	return out
}

// GapClosedRule warns about gaps between path ends that were bridged because they were within the capability's closure tolerance. Actual is the gap.
type GapClosedRule struct{}

func (GapClosedRule) Code() constants.FindingCode { return constants.FindingCodeGeometryGapClosed }

func (GapClosedRule) Check(part *geometry.Part, _ *capability.Profile) []Finding {
	var out []Finding
	for _, g := range part.ClosedGaps {
		out = append(out, Finding{
			Code:      constants.FindingCodeGeometryGapClosed,
			Severity:  constants.SeverityWarning,
			Message:   "Gap between path ends was closed",
			EntityIDs: g.EntityIDs,
			Actual:    mm(g.LengthMM),
			Location:  at(g.Location),
		})
	}
	return out
}
