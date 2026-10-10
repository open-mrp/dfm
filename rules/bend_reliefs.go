package rules

import (
	"math"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// BendReliefRule fails a bend line that ends where the material beside it stays flat (see geometry.BendEnds): without a relief the bend tears it.
type BendReliefRule struct{}

func (BendReliefRule) Code() constants.FindingCode { return constants.FindingCodeBendReliefMissing }

func (BendReliefRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if p.Bending == nil {
		return nil
	}
	var out []Finding
	corners := bendCorners(part.BendLines())
	for _, b := range part.BendLines() {
		for _, e := range part.BendEnds(b) {
			if corners.at(e.At) {
				continue // BendCornerRule reports bends meeting
			}
			var msg string
			switch e.Kind {
			case geometry.BendEndInsideCorner:
				msg = "Bend ends at an inside corner without a relief"
			case geometry.BendEndAlongEdge:
				msg = "Bend ends along an edge without a relief"
			case geometry.BendEndInMaterial:
				msg = "Bend ends inside the material without a relief"
			default:
				continue
			}
			out = append(out, Finding{
				Code:      constants.FindingCodeBendReliefMissing,
				Severity:  constants.SeverityError,
				Message:   msg,
				EntityIDs: []string{b.EntityID},
				Location:  at(e.At),
			})
		}
	}
	return out
}

// reliefWindow is how far past a bend line's end, in multiples of the thickness and at least reliefWindowMM, a gap counts as its relief.
const (
	reliefWindow   = 5
	reliefWindowMM = 5
)

// BendReliefWidthRule checks the reliefs bend lines end at against the relief_width limit (see geometry.BendReliefs).
type BendReliefWidthRule struct{}

func (BendReliefWidthRule) Code() constants.FindingCode {
	return constants.FindingCodeBendReliefTooNarrow
}

func (BendReliefWidthRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	return checkReliefs(part, p, *p.Bending.ReliefWidth, constants.FindingCodeBendReliefTooNarrow,
		"Bend relief is narrower than configured minimum",
		func(r geometry.BendRelief) (float64, [2]geometry.Point) { return r.WidthMM, r.Width })
}

// BendReliefDepthRule checks the reliefs bend lines end at against the relief_depth limit (see geometry.BendReliefs).
type BendReliefDepthRule struct{}

func (BendReliefDepthRule) Code() constants.FindingCode {
	return constants.FindingCodeBendReliefTooShallow
}

func (BendReliefDepthRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	return checkReliefs(part, p, *p.Bending.ReliefDepth, constants.FindingCodeBendReliefTooShallow,
		"Bend relief reaches past the bend less than configured minimum",
		func(r geometry.BendRelief) (float64, [2]geometry.Point) { return r.DepthMM, r.Depth })
}

func checkReliefs(part *geometry.Part, p *capability.Profile, lim capability.BendLimit, code constants.FindingCode, msg string,
	measureOf func(geometry.BendRelief) (float64, [2]geometry.Point)) []Finding {
	window := max(reliefWindowMM, reliefWindow*p.Thickness.MM())
	var out []Finding
	for _, b := range part.BendLines() {
		for _, r := range part.BendReliefs(b, window) {
			mm, pts := measureOf(r)
			d, err := measure.FromMM(mm)
			if err != nil {
				continue
			}
			if sev, req, ok := checkBendLimit(lim, p, d); ok {
				fd := limitFinding(code, msg, sev, req, d, []string{b.EntityID})
				fd.Location, fd.Span = at(r.Width[0]), span(pts[0], pts[1])
				out = append(out, fd)
			}
		}
	}
	return out
}

// cornerTolerance is how close the ends of two bend lines must be to meet.
const cornerTolerance = 0.01

// bendMeeting is two bend lines whose ends meet.
type bendMeeting struct {
	a, b geometry.BendLine
	at   geometry.Point
}

type meetings []bendMeeting

func (m meetings) at(q geometry.Point) bool {
	for _, c := range m {
		if c.at.Dist(q) <= cornerTolerance {
			return true
		}
	}
	return false
}

// bendCorners lists the pairs of bend lines with an end in common.
func bendCorners(bends []geometry.BendLine) meetings {
	var out meetings
	for i, a := range bends {
		for _, b := range bends[i+1:] {
			for _, p := range []geometry.Point{a.A, a.B} {
				for _, q := range []geometry.Point{b.A, b.B} {
					if p.Dist(q) <= cornerTolerance {
						out = append(out, bendMeeting{a, b, p})
					}
				}
			}
		}
	}
	return out
}

// BendCornerRule fails bends that meet where the material stays whole: a box corner with no corner relief, where the flanges pull the corner both ways and tear it.
type BendCornerRule struct{}

func (BendCornerRule) Code() constants.FindingCode {
	return constants.FindingCodeBendCornerReliefMissing
}

func (BendCornerRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if p.Bending == nil {
		return nil
	}
	bends := part.BendLines()
	var out []Finding
	for _, c := range bendCorners(bends) {
		free := true
		for _, b := range []geometry.BendLine{c.a, c.b} {
			for _, e := range part.BendEnds(b) {
				if e.At.Dist(c.at) <= cornerTolerance && e.Kind != geometry.BendEndFree {
					free = false
				}
			}
		}
		if free {
			continue
		}
		out = append(out, Finding{
			Code:      constants.FindingCodeBendCornerReliefMissing,
			Severity:  constants.SeverityError,
			Message:   "Bends meet at a corner without a corner relief",
			EntityIDs: []string{c.a.EntityID, c.b.EntityID},
			Location:  at(c.at),
		})
	}
	return out
}

// BendCrossRule fails bend lines that cross each other: no brake can form both.
type BendCrossRule struct{}

func (BendCrossRule) Code() constants.FindingCode { return constants.FindingCodeBendLinesCross }

func (BendCrossRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if p.Bending == nil {
		return nil
	}
	bends := part.BendLines()
	var out []Finding
	for i, a := range bends {
		for _, b := range bends[i+1:] {
			q, ok := crossing(a, b)
			if !ok {
				continue
			}
			out = append(out, Finding{
				Code:      constants.FindingCodeBendLinesCross,
				Severity:  constants.SeverityError,
				Message:   "Bend lines cross; both cannot be formed",
				EntityIDs: []string{a.EntityID, b.EntityID},
				Location:  at(q),
			})
		}
	}
	return out
}

// crossing returns where two bend lines cross, away from both lines' ends.
func crossing(a, b geometry.BendLine) (geometry.Point, bool) {
	dx1, dy1 := a.B.X-a.A.X, a.B.Y-a.A.Y
	dx2, dy2 := b.B.X-b.A.X, b.B.Y-b.A.Y
	den := float64(dx1*dy2) - float64(dy1*dx2)
	if math.Abs(den) < 1e-12 {
		return geometry.Point{}, false
	}
	ex, ey := b.A.X-a.A.X, b.A.Y-a.A.Y
	t := (float64(ex*dy2) - float64(ey*dx2)) / den
	u := (float64(ex*dy1) - float64(ey*dx1)) / den
	la, lb := a.Length(), b.Length()
	margin := func(s, l float64) bool { return s*l > cornerTolerance && (1-s)*l > cornerTolerance }
	if !margin(t, la) || !margin(u, lb) {
		return geometry.Point{}, false
	}
	return geometry.Point{X: a.A.X + float64(t*dx1), Y: a.A.Y + float64(t*dy1)}, true
}
