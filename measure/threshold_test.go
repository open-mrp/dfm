package measure

import (
	"testing"

	"github.com/open-mrp/dfm/constants"
)

func TestThreshold(t *testing.T) {
	const T, K = Length(2_032_000), Length(152_400) // 2.032 mm stock, 0.1524 mm kerf
	tests := []struct {
		name      string
		th        Threshold
		wantValue string
		wantBasis constants.ThresholdBasis
	}{
		{"floor only", Threshold{Min: 1_500_000}, "1.5", constants.ThresholdBasisMin},
		{"thickness governs", Threshold{Min: 500_000, ThicknessRatio: 500_000}, "1.016", constants.ThresholdBasisThicknessRatio},
		{"kerf governs", Threshold{ThicknessRatio: 100_000, KerfRatio: 2_000_000}, "0.3048", constants.ThresholdBasisKerfRatio},
		{"tie goes to floor", Threshold{Min: 1_016_000, ThicknessRatio: 500_000}, "1.016", constants.ThresholdBasisMin},
		{"all zero", Threshold{}, "0", constants.ThresholdBasisMin},
		// min(2t, 3.175 mm): 2t = 4.064 mm is capped.
		{"cap governs", Threshold{ThicknessRatio: 2_000_000, Max: 3_175_000}, "3.175", constants.ThresholdBasisMax},
		{"cap above the requirement", Threshold{ThicknessRatio: 1_000_000, Max: 3_175_000}, "2.032", constants.ThresholdBasisThicknessRatio},
		{"cap equal to the requirement", Threshold{Min: 3_175_000, Max: 3_175_000}, "3.175", constants.ThresholdBasisMin},
	}
	for _, tt := range tests {
		r := tt.th.Required(T, K)
		if r.Value.String() != tt.wantValue || r.Basis != tt.wantBasis {
			t.Errorf("%s: got %s (%s), want %s (%s)", tt.name, r.Value, r.Basis, tt.wantValue, tt.wantBasis)
		}
	}

	// Exactly at a computed limit passes; one nanometer below fails.
	r := Threshold{ThicknessRatio: 500_000}.Required(T, K) // 1.016 mm
	if r.Below(1_016_000) || !r.Below(1_015_999) {
		t.Error("boundary comparison is not exact")
	}
	// A limit finer than 1 nm (0.677332656 mm): 0.677333 passes, 0.677332 fails.
	r = Threshold{ThicknessRatio: 333_333}.Required(T, K)
	if r.Below(677_333) || !r.Below(677_332) {
		t.Error("sub-nanometer limit comparison is not exact")
	}
}
