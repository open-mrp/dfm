package constants

// Unit is the unit of a reported quantity: a length unit, mm2 for areas, percent for shares (a whole number: 55.25 is 55.25%), deg for angles, kN for forces, or segment for a number of path segments.
type Unit string

const (
	UnitMicrometer       Unit = Unit(LengthUnitMicrometer)
	UnitMillimeter       Unit = Unit(LengthUnitMillimeter)
	UnitCentimeter       Unit = Unit(LengthUnitCentimeter)
	UnitMeter            Unit = Unit(LengthUnitMeter)
	UnitInch             Unit = Unit(LengthUnitInch)
	UnitFoot             Unit = Unit(LengthUnitFoot)
	UnitSquareMillimeter Unit = Unit(AreaUnitSquareMillimeter)
	UnitPercent          Unit = "percent"
	UnitDegree           Unit = Unit(AngleUnitDegree)
	UnitSegment          Unit = "segment"
	UnitKilonewton       Unit = Unit(ForceUnitKilonewton)
)

var units = []Unit{
	UnitMicrometer, UnitMillimeter, UnitCentimeter, UnitMeter, UnitInch, UnitFoot,
	UnitSquareMillimeter, UnitPercent, UnitDegree, UnitSegment, UnitKilonewton,
}

func (u Unit) IsValid() bool        { return isOneOf(u, units) }
func (u Unit) EnumValues() []string { return enumValues(units) }

// Open marks the enum as gaining values: processes bring their units.
func (u Unit) Open() bool { return true }
