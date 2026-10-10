package constants

// BendDirection is the way a bend turns its flange.
type BendDirection string

const (
	// BendDirectionUp turns the flange toward the viewer of the drawing.
	BendDirectionUp BendDirection = "up"
	// BendDirectionDown turns it away from the viewer.
	BendDirectionDown BendDirection = "down"
)

var bendDirections = []BendDirection{BendDirectionUp, BendDirectionDown}

func (d BendDirection) IsValid() bool        { return isOneOf(d, bendDirections) }
func (d BendDirection) EnumValues() []string { return enumValues(bendDirections) }
