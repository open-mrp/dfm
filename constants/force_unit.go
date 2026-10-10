package constants

// ForceUnit is the unit of a force quantity.
type ForceUnit string

const ForceUnitKilonewton ForceUnit = "kN"

var forceUnits = []ForceUnit{ForceUnitKilonewton}

func (u ForceUnit) IsValid() bool        { return isOneOf(u, forceUnits) }
func (u ForceUnit) EnumValues() []string { return enumValues(forceUnits) }
