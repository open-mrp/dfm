package measure

import (
	"math/big"
	"strings"
)

// Decimal is an exact finite decimal: unscaled × 10^-scale. The zero value is 0. No method mutates unscaled, each allocates its result, so a Decimal is safe to copy and share.
type Decimal struct {
	unscaled *big.Int
	scale    int
}

// bigInt returns the unscaled value, reading the zero value as 0. Callers must not mutate it.
func (d Decimal) bigInt() *big.Int {
	if d.unscaled == nil {
		return new(big.Int)
	}
	return d.unscaled
}

// Cmp compares d and e exactly, returning -1, 0 or +1.
func (d Decimal) Cmp(e Decimal) int {
	a, b := d.bigInt(), e.bigInt()
	switch {
	case d.scale < e.scale:
		a = new(big.Int).Mul(a, pow10(e.scale-d.scale))
	case e.scale < d.scale:
		b = new(big.Int).Mul(b, pow10(d.scale-e.scale))
	}
	return a.Cmp(b)
}

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

// Shift returns d × 10^n exactly.
func (d Decimal) Shift(n int) Decimal {
	if n <= d.scale {
		return Decimal{unscaled: new(big.Int).Set(d.bigInt()), scale: d.scale - n}
	}
	return Decimal{unscaled: new(big.Int).Mul(d.bigInt(), pow10(n-d.scale)), scale: 0}
}

// Add returns d + e exactly.
func (d Decimal) Add(e Decimal) Decimal {
	scale := max(d.scale, e.scale)
	a := new(big.Int).Mul(d.bigInt(), pow10(scale-d.scale))
	b := new(big.Int).Mul(e.bigInt(), pow10(scale-e.scale))
	return Decimal{unscaled: a.Add(a, b), scale: scale}
}

// Float64 returns the nearest float64, for bounding geometric searches only.
func (d Decimal) Float64() float64 {
	f, _ := new(big.Rat).SetFrac(d.bigInt(), pow10(d.scale)).Float64()
	return f
}

// String formats the decimal exactly with no trailing zeros, e.g. "0.677332656".
func (d Decimal) String() string {
	v := d.bigInt()
	digits := new(big.Int).Abs(v).String()
	if d.scale > 0 {
		if len(digits) <= d.scale {
			digits = strings.Repeat("0", d.scale-len(digits)+1) + digits
		}
		whole, frac := digits[:len(digits)-d.scale], strings.TrimRight(digits[len(digits)-d.scale:], "0")
		digits = whole
		if frac != "" {
			digits += "." + frac
		}
	}
	if v.Sign() < 0 {
		return "-" + digits
	}
	return digits
}
