package rules

import (
	"math"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

// MultiplePartsRule reports a drawing with more than one part, such as several parts nested on a sheet: an error when the capability requires one part per drawing, otherwise a warning that each part is checked separately, so nobody quotes the sheet as one part. EntityIDs lists the outline entities of every part (see geometry.Parts).
type MultiplePartsRule struct{}

func (MultiplePartsRule) Code() constants.FindingCode { return constants.FindingCodeFileMultipleParts }

func (MultiplePartsRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	mode := multipleParts(p)
	parts := part.Parts(mode == constants.MultiplePartsAcceptedInCutouts)
	if len(parts) < 2 {
		return nil
	}
	var ids []string
	for _, pt := range parts {
		ids = append(ids, pt.Outline...)
	}
	sev := constants.SeverityWarning
	if mode == constants.MultiplePartsRejected {
		sev = constants.SeverityError
	}
	return []Finding{{
		Code:      constants.FindingCodeFileMultipleParts,
		Severity:  sev,
		Message:   "Drawing contains more than one part",
		EntityIDs: ids,
	}}
}

// NestedPartRule warns about each part found inside another part's cut-out when the capability accepts parts nested in cut-outs. Material there falls free when it is cut; that is intended for a nested part and fatal for a letter's center, so it is pointed out rather than assumed.
type NestedPartRule struct{}

func (NestedPartRule) Code() constants.FindingCode { return constants.FindingCodePartNestedInCutout }

func (NestedPartRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if multipleParts(p) != constants.MultiplePartsAcceptedInCutouts {
		return nil
	}
	var out []Finding
	for _, ids := range part.LoopIDs(func(d int) bool { return d >= 2 && d%2 == 0 }) {
		out = append(out, Finding{
			Code:      constants.FindingCodePartNestedInCutout,
			Severity:  constants.SeverityWarning,
			Message:   "Part lies inside another part's cut-out and is cut free as a separate part",
			EntityIDs: ids,
		})
	}
	return out
}

// multipleParts is the profile's multiple-parts mode.
func multipleParts(p *capability.Profile) constants.MultipleParts {
	if p != nil && p.LaserCutting != nil && p.LaserCutting.MultipleParts != "" {
		return p.LaserCutting.MultipleParts
	}
	return constants.MultiplePartsAccepted
}

// FloatingIslandRule fails material enclosed by a cut-out, such as the center of an outlined letter O. Once the surrounding cut is made the island falls away as a separate loose piece. EntityIDs is the island's boundary.
type FloatingIslandRule struct{}

func (FloatingIslandRule) Code() constants.FindingCode { return constants.FindingCodeFloatingIsland }

func (FloatingIslandRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	// An island no wider than the kerf (a center mark in a hole) is burnt away entirely.
	kerf := kerfOf(p).MM()
	island := func(depth int, b geometry.Bounds) bool {
		return depth >= 2 && depth%2 == 0 && math.Max(b.WidthMM(), b.HeightMM()) > kerf
	}
	var out []Finding
	add := func(ids []string) {
		out = append(out, Finding{
			Code:      constants.FindingCodeFloatingIsland,
			Severity:  constants.SeverityError,
			Message:   "Material island is disconnected from the part",
			EntityIDs: ids,
		})
	}
	for _, c := range part.Contours {
		if c.Closed && island(c.Depth, c.Bounds()) {
			add(c.EntityIDs())
		}
	}
	for _, c := range part.Circles {
		if island(c.Depth, c.Bounds()) {
			add(c.EntityIDs)
		}
	}
	return out
}
