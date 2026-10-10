package constants

// HoleSide is the face of the sheet a countersink opens on or a fastener is pressed in from.
type HoleSide string

const (
	// HoleSideTop is the face toward the viewer of the drawing.
	HoleSideTop HoleSide = "top"
	// HoleSideBottom is the face away from the viewer.
	HoleSideBottom HoleSide = "bottom"
)

var holeSides = []HoleSide{HoleSideTop, HoleSideBottom}

func (s HoleSide) IsValid() bool        { return isOneOf(s, holeSides) }
func (s HoleSide) EnumValues() []string { return enumValues(holeSides) }
