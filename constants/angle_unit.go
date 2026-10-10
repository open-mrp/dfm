package constants

// AngleUnit is the unit of an angle quantity.
type AngleUnit string

const AngleUnitDegree AngleUnit = "deg"

var angleUnits = []AngleUnit{AngleUnitDegree}

func (u AngleUnit) IsValid() bool        { return isOneOf(u, angleUnits) }
func (u AngleUnit) EnumValues() []string { return enumValues(angleUnits) }
