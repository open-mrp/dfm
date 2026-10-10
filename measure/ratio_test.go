package measure

import (
	"testing"
)

func TestParseRatio(t *testing.T) {
	for in, want := range map[string]Ratio{"0.5": 500_000, "1": 1_000_000, "0.333333": 333_333, "0": 0} {
		if got, err := ParseRatio(in); err != nil || got != want {
			t.Errorf("ParseRatio(%q) = %d, %v", in, got, err)
		}
	}
	for _, bad := range []string{"0.3333333", "1e-3", "abc", ""} {
		if _, err := ParseRatio(bad); err == nil {
			t.Errorf("ParseRatio(%q) accepted", bad)
		}
	}
}
