package measure

import (
	"strings"
	"testing"

	"github.com/open-mrp/dfm/constants"
)

func TestParseArea(t *testing.T) {
	tests := []struct {
		value string
		unit  constants.AreaUnit
		want  Area
		err   string
	}{
		{"1000", "mm2", 1_000_000_000, ""},
		{"0.000001", "mm2", 1, ""},
		{"6", "in2", 3_870_960_000, ""}, // 6 in² = 3870.96 mm², exact
		{"1", "ft2", 92_903_040_000, ""},
		{"2.5", "cm2", 250_000_000, ""},
		{"0.01", "m2", 10_000_000_000, ""},
		{"0.0000001", "mm2", 0, "finer than 1 square micrometer"},
		{"1", "mm", 0, "is not one of"},
		{"1e3", "mm2", 0, "not a decimal number"},
	}
	for _, tt := range tests {
		got, err := ParseArea(tt.value, tt.unit)
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("ParseArea(%q, %q) error %v, want %q", tt.value, tt.unit, err, tt.err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseArea(%q, %q) = %d, %v; want %d", tt.value, tt.unit, got, err, tt.want)
		}
	}
	if a, err := AreaFromMM2(895.9999996); err != nil || a.String() != "896" {
		t.Errorf("AreaFromMM2 = %s, %v; want 896", a, err)
	}
}
