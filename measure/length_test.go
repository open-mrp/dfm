package measure

import (
	"strings"
	"testing"

	"github.com/open-mrp/dfm/constants"
)

func TestParseLength(t *testing.T) {
	tests := []struct {
		value string
		unit  constants.LengthUnit
		want  Length
		err   string
	}{
		{"1.5", "mm", 1_500_000, ""},
		{"0.1", "mm", 100_000, ""},     // not representable in binary floating point
		{"2.032", "mm", 2_032_000, ""}, // ditto
		{"1500", "mm", 1_500_000_000, ""},
		{"0.000001", "mm", 1, ""},
		{"0.0625", "in", 1_587_500, ""}, // 1/16 in, exact
		{"0.001", "in", 25_400, ""},
		{"1", "ft", 304_800_000, ""},
		{"2.5", "um", 2_500, ""},
		{"1.5", "m", 1_500_000_000, ""},
		{"-2", "mm", -2_000_000, ""},
		{"0.0000001", "mm", 0, "finer than 1 nanometer"},
		{"0.0000005", "in", 0, "finer than 1 nanometer"}, // 12.7 nm
		{"1.5e3", "mm", 0, "not a decimal number"},
		{".5", "mm", 0, "not a decimal number"},
		{"+1", "mm", 0, "not a decimal number"},
		{"1.", "mm", 0, "not a decimal number"},
		{"1/2", "mm", 0, "not a decimal number"},
		{"NaN", "mm", 0, "not a decimal number"},
		{"", "mm", 0, "not a decimal number"},
		{" 1", "mm", 0, "not a decimal number"},
		{"1", "MM", 0, "is not one of"},
		{"1", "", 0, "is not one of"},
		{"99999999999999", "m", 0, "out of range"},
	}
	for _, tt := range tests {
		got, err := ParseLength(tt.value, tt.unit)
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("ParseLength(%q, %q) error %v, want %q", tt.value, tt.unit, err, tt.err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseLength(%q, %q) = %d, %v; want %d", tt.value, tt.unit, got, err, tt.want)
		}
	}
}

func TestFromMM(t *testing.T) {
	tests := []struct {
		in   float64
		want Length
	}{
		{1.4999999999999858, 1_500_000}, // a 1.5 mm hole as written by a CAD exporter
		{0.1 + 0.2, 300_000},            // 0.30000000000000004
		{-2.0000005, -2_000_001},
		{1e6, 1_000_000_000_000},
	}
	for _, tt := range tests {
		if got := MustFromMM(tt.in); got != tt.want {
			t.Errorf("FromMM(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
	zero := 0.0
	for _, bad := range []float64{1 / zero, zero / zero, 1e13} {
		if _, err := FromMM(bad); err == nil {
			t.Errorf("FromMM(%v) accepted", bad)
		}
	}
}
