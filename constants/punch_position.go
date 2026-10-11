package constants

// PunchPosition is where along a bend line the punch stands while it forms the bend.
type PunchPosition string

const (
	// PunchPositionStart puts the punch's end at the bend line's start.
	PunchPositionStart PunchPosition = "start"
	// PunchPositionMiddle centers the punch on the bend line.
	PunchPositionMiddle PunchPosition = "middle"
	// PunchPositionEnd puts the punch's end at the bend line's end.
	PunchPositionEnd PunchPosition = "end"
)

var punchPositions = []PunchPosition{PunchPositionStart, PunchPositionMiddle, PunchPositionEnd}

func (p PunchPosition) IsValid() bool        { return isOneOf(p, punchPositions) }
func (p PunchPosition) EnumValues() []string { return enumValues(punchPositions) }

// LineSide is a side of a line in the drawing, looking along it from its start to its end, with the drawing seen from its viewer.
type LineSide string

const (
	LineSideLeft  LineSide = "left"
	LineSideRight LineSide = "right"
)

var lineSides = []LineSide{LineSideLeft, LineSideRight}

func (s LineSide) IsValid() bool        { return isOneOf(s, lineSides) }
func (s LineSide) EnumValues() []string { return enumValues(lineSides) }
