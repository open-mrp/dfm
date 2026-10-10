package measure

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
)

// Length, Area and Ratio are all whole numbers of a millionth of their unit (nm per mm, µm² per mm², millionths), so they share one scale.
const (
	microScale          = 6
	microPerUnit  int64 = 1_000_000 // 10^microScale
	maxMicroUnits       = 9e12      // the largest magnitude, in units, that rounds into int64 micro units with room to spare
)

// decimalString is the wire grammar for decimal values: an optional minus sign, digits, and an optional fraction. No exponents, no leading '+' or '.'.
var decimalString = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// ParseDecimal parses a decimal string exactly.
func ParseDecimal(s string) (*big.Rat, error) {
	if !decimalString.MatchString(s) {
		return nil, fmt.Errorf("%q is not a decimal number (expected digits with an optional fraction, e.g. \"1.5\")", s)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("%q is not a decimal number", s)
	}
	return r, nil
}

// parseScaled parses value exactly and returns value × scale as a whole number. subject names the value in errors ("1.5 mm") and finer says what a fractional product means ("is finer than 1 nanometer").
func parseScaled(value, subject string, scale int64, finer string) (int64, error) {
	r, err := ParseDecimal(value)
	if err != nil {
		return 0, err
	}
	r.Mul(r, new(big.Rat).SetInt64(scale))
	if !r.IsInt() {
		return 0, fmt.Errorf("%s %s", subject, finer)
	}
	if !r.Num().IsInt64() {
		return 0, fmt.Errorf("%s is out of range", subject)
	}
	return r.Num().Int64(), nil
}

// ErrNotFinite is returned by FromMM and AreaFromMM2 for NaN, infinite or out-of-range input.
var ErrNotFinite = errors.New("measurement is not a finite number in range")

// roundMicro converts a measured value in units to the nearest whole number of micro units, rounding halves away from zero.
func roundMicro(v float64) (int64, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > maxMicroUnits {
		return 0, ErrNotFinite
	}
	return int64(math.Round(v * float64(microPerUnit))), nil
}

// fixed returns n × 10^-microScale as a Decimal: the exact value of a Length in mm, an Area in mm² or a Ratio.
func fixed(n int64) Decimal { return Decimal{unscaled: big.NewInt(n), scale: microScale} }
