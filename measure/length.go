package measure

import (
	"fmt"
	"strings"

	"github.com/open-mrp/dfm/constants"
)

// Length is an exact length in nanometers.
type Length int64

const (
	Nanometer  Length = 1
	Millimeter Length = Length(microPerUnit)
)

// ParseLength parses a decimal string in the given unit into an exact length. The value must be a whole number of nanometers.
func ParseLength(value string, unit constants.LengthUnit) (Length, error) {
	if !unit.IsValid() {
		return 0, fmt.Errorf("unit %q is not one of %s", unit, strings.Join(unit.EnumValues(), ", "))
	}
	n, err := parseScaled(value, value+" "+string(unit), unit.Nanometers(), "is finer than 1 nanometer")
	return Length(n), err
}

// FromMM converts a measured millimeter value to the nearest nanometer, rounding halves away from zero.
func FromMM(mm float64) (Length, error) {
	n, err := roundMicro(mm)
	return Length(n), err
}

// MustFromMM is FromMM for values known to be in range. It panics otherwise.
func MustFromMM(mm float64) Length {
	l, err := FromMM(mm)
	if err != nil {
		panic(err)
	}
	return l
}

// MM returns the length in millimeters as a float64, for geometry.
func (l Length) MM() float64 { return float64(l) / float64(Millimeter) }

// Decimal returns the exact value in millimeters.
func (l Length) Decimal() Decimal { return fixed(int64(l)) }

// String formats the length in millimeters, e.g. "1.5".
func (l Length) String() string { return l.Decimal().String() }
