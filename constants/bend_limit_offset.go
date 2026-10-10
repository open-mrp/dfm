package constants

// BendLimitOffset says what a bending limit's thresholds are measured from: nothing, or the inside radius the tooling forms, which the thresholds are added to (e.g. 2 x thickness + radius) so the limit grows with the bend.
type BendLimitOffset string

const (
	// BendLimitOffsetNone adds nothing to the thresholds.
	BendLimitOffsetNone BendLimitOffset = "none"
	// BendLimitOffsetInsideRadius adds the inside radius the tooling forms to the thresholds.
	BendLimitOffsetInsideRadius BendLimitOffset = "inside_radius"
)

var bendLimitOffsets = []BendLimitOffset{BendLimitOffsetNone, BendLimitOffsetInsideRadius}

func (o BendLimitOffset) IsValid() bool        { return isOneOf(o, bendLimitOffsets) }
func (o BendLimitOffset) EnumValues() []string { return enumValues(bendLimitOffsets) }
