package main

import (
	"testing"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/resource"
)

// TestDisplay checks the display form of each unit.
func TestDisplay(t *testing.T) {
	for _, tt := range []struct {
		value string
		unit  constants.Unit
		want  string
	}{
		{"0.8", constants.Unit(constants.LengthUnitMillimeter), "0.8 mm"},
		{"25.4", constants.UnitInch, "25.4 in"},
		{"6.25", constants.UnitSquareMillimeter, "6.25 mm²"},
		{"55.25", constants.UnitPercent, "55.25%"},
		{"90", constants.UnitDegree, "90°"},
		{"36", constants.UnitSegment, "36 segments"},
	} {
		if got := display(&resource.Quantity{Value: tt.value, Unit: tt.unit}); got != tt.want {
			t.Errorf("display(%s %s) = %q, want %q", tt.value, tt.unit, got, tt.want)
		}
	}
}
