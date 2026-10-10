package rules

import (
	"math"

	"github.com/open-mrp/dfm/brake"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

// collisionSamples is about how many samples the flat part is folded with; the spacing follows the part's size.
const collisionSamples = 5000

// foldModel prepares the part for folding (see package brake): its bend lines at the angles and directions the bending check set, at the radius the tooling forms. Nil when there is nothing to fold.
func foldModel(part *geometry.Part, p *capability.Profile) (*brake.Model, []geometry.BendLine) {
	var lines []geometry.BendLine
	var bends []brake.Bend
	radius := p.Bending.InsideRadius.Float64()
	for _, b := range part.BendLines() {
		if b.Length() == 0 {
			continue
		}
		lines = append(lines, b)
		bends = append(bends, brake.Bend{A: b.A, B: b.B, Angle: float64(b.Spec.Angle) / 1e6, InsideRadius: radius, Up: b.Spec.Direction != constants.BendDirectionDown})
	}
	if len(bends) == 0 {
		return nil, nil
	}
	spacing := max(math.Sqrt(float64(part.Bounds.WidthMM()*part.Bounds.HeightMM())/collisionSamples), 0.5)
	m, err := brake.New(part, bends, p.Thickness.MM(), float64(p.Bending.KFactor)/1e6, spacing)
	if err != nil {
		return nil, nil
	}
	return m, lines
}

// FlangeCollisionRule folds every bend to its angle and fails a part whose flanges run into each other once formed: lips that overlap at a corner, a wall drawn into the next, a return deeper than its wall. The tooling is not needed: the part is folded at the radius the tooling forms.
type FlangeCollisionRule struct{}

func (FlangeCollisionRule) Code() constants.FindingCode {
	return constants.FindingCodeBendFlangesCollide
}

func (FlangeCollisionRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if p.Bending == nil {
		return nil
	}
	m, lines := foldModel(part, p)
	if m == nil {
		return nil
	}
	var out []Finding
	for _, c := range m.Collisions() {
		var ids []string
		for _, bi := range c.Bends {
			ids = append(ids, lines[bi].EntityID)
		}
		out = append(out, Finding{
			Code:      constants.FindingCodeBendFlangesCollide,
			Severity:  constants.SeverityError,
			Message:   "Two flanges run into each other once the part is folded",
			EntityIDs: ids,
			Location:  at(c.A),
			Span:      span(c.A, c.B),
		})
	}
	return out
}
