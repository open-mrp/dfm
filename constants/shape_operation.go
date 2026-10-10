package constants

// ShapeOperation is what a shape in a vector drawing (SVG) is for.
type ShapeOperation string

const (
	// ShapeOperationCut cuts along the shape.
	ShapeOperationCut ShapeOperation = "cut"
	// ShapeOperationEngrave engraves or marks it; it is left out of the part.
	ShapeOperationEngrave ShapeOperation = "engrave"
	// ShapeOperationIgnore leaves it out as construction geometry.
	ShapeOperationIgnore ShapeOperation = "ignore"
)

var shapeOperations = []ShapeOperation{ShapeOperationCut, ShapeOperationEngrave, ShapeOperationIgnore}

func (o ShapeOperation) IsValid() bool        { return isOneOf(o, shapeOperations) }
func (o ShapeOperation) EnumValues() []string { return enumValues(shapeOperations) }
