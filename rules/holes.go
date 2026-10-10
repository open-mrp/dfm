package rules

import (
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// HoleRule checks circular hole diameters against the hole_diameter limit. Holes are full circles that are cut-outs (odd containment depth), whether drawn as CIRCLE entities or as closed chains of co-circular arcs.
type HoleRule struct{}

func (HoleRule) Code() constants.FindingCode { return constants.FindingCodeHoleTooSmall }

func (HoleRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lc := p.LaserCutting
	var out []Finding
	// A hole with an operation is the size the operation needs.
	operated := map[string]bool{}
	for _, op := range part.HoleOps {
		operated[op.EntityIDs[0]] = true
	}
	for _, h := range part.Holes() {
		if len(h.EntityIDs) > 0 && operated[h.EntityIDs[0]] {
			continue
		}
		d, err := measure.FromMM(h.Diameter())
		if err != nil {
			continue // beyond any representable size, so never too small
		}
		if sev, req, ok := checkLimit(lc.HoleDiameter, p.Thickness, lc.KerfWidth, d); ok {
			out = append(out, limitFinding(constants.FindingCodeHoleTooSmall, "Hole diameter is below configured minimum", sev, req, d, h.EntityIDs))
		}
	}
	return out
}
