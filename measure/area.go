package measure

import (
	"fmt"
	"strings"

	"github.com/open-mrp/dfm/constants"
)

// Area is an exact area in square micrometers, so square inches and feet are whole numbers of it.
type Area int64

const (
	SquareMicrometer Area = 1
	SquareMillimeter Area = Area(microPerUnit)
)

// ParseArea parses a decimal string in the given unit into an exact area. The value must be a whole number of square micrometers.
func ParseArea(value string, unit constants.AreaUnit) (Area, error) {
	if !unit.IsValid() {
		return 0, fmt.Errorf("unit %q is not one of %s", unit, strings.Join(unit.EnumValues(), ", "))
	}
	n, err := parseScaled(value, value+" "+string(unit), unit.SquareMicrometers(), "is finer than 1 square micrometer")
	return Area(n), err
}

// AreaFromMM2 converts a measured area in square millimeters to the nearest square micrometer, rounding halves away from zero.
func AreaFromMM2(mm2 float64) (Area, error) {
	n, err := roundMicro(mm2)
	return Area(n), err
}

// Decimal returns the exact value in square millimeters.
func (a Area) Decimal() Decimal { return fixed(int64(a)) }

// String formats the area in square millimeters, e.g. "6.25".
func (a Area) String() string { return a.Decimal().String() }
