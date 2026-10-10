package constants

// LengthUnit is the unit of a length quantity.
type LengthUnit string

const (
	LengthUnitMicrometer LengthUnit = "um"
	LengthUnitMillimeter LengthUnit = "mm"
	LengthUnitCentimeter LengthUnit = "cm"
	LengthUnitMeter      LengthUnit = "m"
	LengthUnitInch       LengthUnit = "in"
	LengthUnitFoot       LengthUnit = "ft"
)

var lengthUnits = []LengthUnit{
	LengthUnitMicrometer, LengthUnitMillimeter, LengthUnitCentimeter,
	LengthUnitMeter, LengthUnitInch, LengthUnitFoot,
}

func (u LengthUnit) IsValid() bool        { return isOneOf(u, lengthUnits) }
func (u LengthUnit) EnumValues() []string { return enumValues(lengthUnits) }

// Nanometers is the exact number of nanometers in one unit.
func (u LengthUnit) Nanometers() int64 {
	switch u {
	case LengthUnitMicrometer:
		return 1_000
	case LengthUnitMillimeter:
		return 1_000_000
	case LengthUnitCentimeter:
		return 10_000_000
	case LengthUnitMeter:
		return 1_000_000_000
	case LengthUnitInch:
		return 25_400_000
	case LengthUnitFoot:
		return 304_800_000
	default:
		return 0
	}
}
