package measure

import (
	"testing"
)

func TestDecimalString(t *testing.T) {
	tests := []struct {
		d    Decimal
		want string
	}{
		{Length(800_000).Decimal(), "0.8"},
		{Length(1_500_000_000).Decimal(), "1500"},
		{Length(1).Decimal(), "0.000001"},
		{Length(-1_250_000).Decimal(), "-1.25"},
		{Length(0).Decimal(), "0"},
		{Decimal{}, "0"},
		{Ratio(333_333).Times(2_032_000), "0.677332656"},
		{Ratio(500_000).Times(2_032_000), "1.016"},
		{Ratio(552_500).Decimal().Shift(2), "55.25"},
		{Length(3).Decimal().Shift(8), "300"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

func TestDecimalCmp(t *testing.T) {
	ratio := func(s string) Decimal {
		r, err := ParseRatio(s)
		if err != nil {
			t.Fatal(err)
		}
		return r.Decimal()
	}
	tests := []struct {
		a, b Decimal
		want int
	}{
		{Decimal{}, Decimal{}, 0},
		{Decimal{}, ratio("0.000001"), -1},
		{ratio("1"), ratio("1"), 0},
		{ratio("1").Shift(3), ratio("1000"), 0},                      // scales differ, values equal
		{ratio("-1"), ratio("-2"), 1},                                // negative
		{ratio("0.5"), ratio("0.5").Shift(-6), 1},                    // 0.5 vs 0.0000005
		{Ratio(333_333).Times(Millimeter * 2), ratio("0.666666"), 0}, // scale 12 vs scale 6
		{Ratio(333_333).Times(Millimeter*2 + 32_000), ratio("0.677332"), 1},
		{Ratio(333_333).Times(Millimeter*2 + 32_000), ratio("0.677333"), -1},
	}
	for _, tt := range tests {
		if got := tt.a.Cmp(tt.b); got != tt.want {
			t.Errorf("Cmp(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
		if got := tt.b.Cmp(tt.a); got != -tt.want {
			t.Errorf("Cmp(%s, %s) = %d, want %d", tt.b, tt.a, got, -tt.want)
		}
	}
}
