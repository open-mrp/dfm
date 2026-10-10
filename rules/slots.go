package rules

import (
	"math"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// SlotRule checks the width of recognized slots against the slot_width limit. Recognition lives in geometry and covers obrounds and rectangular cut-outs only.
type SlotRule struct{}

func (SlotRule) Code() constants.FindingCode { return constants.FindingCodeSlotTooNarrow }

func (SlotRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lc := p.LaserCutting
	var out []Finding
	for _, s := range part.Slots {
		w, err := measure.FromMM(s.WidthMM)
		if err != nil {
			continue // beyond any representable size, so never too narrow
		}
		if sev, req, ok := checkLimit(lc.SlotWidth, p.Thickness, lc.KerfWidth, w); ok {
			out = append(out, limitFinding(constants.FindingCodeSlotTooNarrow, "Slot width is below configured minimum", sev, req, w, s.EntityIDs))
		}
	}
	return out
}

// slotFitBand is how close to the stock thickness a slot's width must be to be read as receiving the stock (a tab-and-slot joint).
const slotFitBand = 0.25

// SlotFitRule checks slots that receive the stock (within 25% of the thickness wide) against the slot_fit_clearance limit: actual is the slot's width less the thickness, negative when it is narrower.
type SlotFitRule struct{}

func (SlotFitRule) Code() constants.FindingCode { return constants.FindingCodeSlotFitTooTight }

func (SlotFitRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	lim := p.LaserCutting.Optional.SlotFitClearance
	t := p.Thickness.MM()
	var out []Finding
	for _, s := range part.Slots {
		if math.Abs(s.WidthMM-t) > slotFitBand*t {
			continue
		}
		c, err := measure.FromMM(s.WidthMM - t)
		if err != nil {
			continue
		}
		if sev, req, ok := checkLimit(*lim, p.Thickness, p.LaserCutting.KerfWidth, c); ok {
			out = append(out, limitFinding(constants.FindingCodeSlotFitTooTight, "Slot leaves less clearance around the stock it receives than configured minimum", sev, req, c, s.EntityIDs))
		}
	}
	return out
}
