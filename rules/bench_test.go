package rules

import (
	"testing"

	"github.com/open-mrp/dfm/geometry"
)

// BenchmarkPhase3Grid checks a 100 x 100 grid of holes with every spacing, edge, width and density limit set.
func BenchmarkPhase3Grid(b *testing.B) {
	var holes []geometry.Circle
	for r := range 100 {
		for c := range 100 {
			holes = append(holes, hole("h", 5+float64(c)*10, 5+float64(r)*10, 6))
		}
	}
	part := geometry.NewPart([]geometry.Contour{rectContour("plate", 0, 0, 1000, 1000)}, holes)
	p := testProfile()
	p.LaserCutting.Optional.WebWidth = minLimit(5_000_000)
	p.LaserCutting.Optional.FeatureSpacing = minLimit(5_000_000)
	p.LaserCutting.Optional.EdgeDistance = minLimit(3_000_000)
	p.LaserCutting.Optional.PartWidth = minLimit(6_000_000)
	b.ResetTimer()
	for range b.N {
		for _, r := range []Rule{WebRule{}, FeatureSpacingRule{}, EdgeDistanceRule{}, PartWidthRule{}, DensityRule{}} {
			if Enabled(r, p) {
				r.Check(part, p)
			}
		}
	}
}
