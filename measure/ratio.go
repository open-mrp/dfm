package measure

import "math/big"

// Ratio is an exact dimensionless value in millionths, e.g. 0.5 = 500000.
type Ratio int64

// ParseRatio parses a decimal string with at most 6 decimal places.
func ParseRatio(s string) (Ratio, error) {
	n, err := parseScaled(s, s, microPerUnit, "has more than 6 decimal places")
	return Ratio(n), err
}

// Decimal returns the exact value.
func (r Ratio) Decimal() Decimal { return fixed(int64(r)) }

// String formats the ratio, e.g. "0.5".
func (r Ratio) String() string { return r.Decimal().String() }

// Times returns ratio × length exactly, in millimeters. Millionths times nanometers is finer than either, so the product cannot overflow int64 and keeps both scales.
func (r Ratio) Times(l Length) Decimal {
	v := new(big.Int).Mul(big.NewInt(int64(r)), big.NewInt(int64(l)))
	return Decimal{unscaled: v, scale: 2 * microScale}
}
