package constants

// AreaUnit is the unit of an area quantity.
type AreaUnit string

const (
	AreaUnitSquareMillimeter AreaUnit = "mm2"
	AreaUnitSquareCentimeter AreaUnit = "cm2"
	AreaUnitSquareMeter      AreaUnit = "m2"
	AreaUnitSquareInch       AreaUnit = "in2"
	AreaUnitSquareFoot       AreaUnit = "ft2"
)

var areaUnits = []AreaUnit{
	AreaUnitSquareMillimeter, AreaUnitSquareCentimeter, AreaUnitSquareMeter,
	AreaUnitSquareInch, AreaUnitSquareFoot,
}

func (u AreaUnit) IsValid() bool        { return isOneOf(u, areaUnits) }
func (u AreaUnit) EnumValues() []string { return enumValues(areaUnits) }

// SquareMicrometers is the exact number of square micrometers in one unit.
func (u AreaUnit) SquareMicrometers() int64 {
	switch u {
	case AreaUnitSquareMillimeter:
		return 1_000_000
	case AreaUnitSquareCentimeter:
		return 100_000_000
	case AreaUnitSquareMeter:
		return 1_000_000_000_000
	case AreaUnitSquareInch:
		return 645_160_000
	case AreaUnitSquareFoot:
		return 92_903_040_000
	default:
		return 0
	}
}
