package rules

import (
	"testing"

	"github.com/open-mrp/dfm/geometry"
)

// polyContour builds a closed contour from x, y, bulge triples.
func polyContour(id string, v ...float64) geometry.Contour {
	c := geometry.Contour{Closed: true}
	n := len(v) / 3
	for i := range n {
		a, b := v[3*i:3*i+3], v[3*((i+1)%n):3*((i+1)%n)+3]
		c.Segments = append(c.Segments, geometry.Segment{
			Start: geometry.Point{X: a[0], Y: a[1]}, End: geometry.Point{X: b[0], Y: b[1]}, Bulge: a[2], EntityID: id,
		})
	}
	return c
}

func hourglass(waist float64) geometry.Contour {
	h := waist / 2
	return polyContour("glass", 40, 10, 0, 60, 10, 0, 50+h, 25, 0, 60, 40, 0, 40, 40, 0, 50-h, 25, 0)
}

func finger(w float64) geometry.Contour {
	return polyContour("p", 0, 0, 0, 40, 0, 0, 40, 20, 0, 20+w/2, 20, 0, 20+w/2, 40, 0, 20-w/2, 40, 0, 20-w/2, 20, 0, 0, 20, 0)
}

func TestLocalWidthRules(t *testing.T) {
	plate := rectContour("plate", 0, 0, 100, 60)
	tests := []struct {
		name      string
		contours  []geometry.Contour
		rule      Rule
		partWidth bool
		want      []got
	}{
		{"cut-out waist above limit", []geometry.Contour{plate, hourglass(1.1)}, CutoutWidthRule{}, false, nil},
		{"cut-out waist at limit", []geometry.Contour{plate, hourglass(1)}, CutoutWidthRule{}, false, nil},
		{"cut-out waist below limit", []geometry.Contour{plate, hourglass(0.9)}, CutoutWidthRule{}, false, []got{{[]string{"glass"}, "0.9"}}},
		{"narrow slot is the slot rule's", []geometry.Contour{plate, rectContour("slot", 10, 10, 30, 0.5)}, CutoutWidthRule{}, false, nil},
		{"narrow notch in the edge", []geometry.Contour{polyContour("p", 0, 0, 0, 100, 0, 0, 100, 50, 0, 50.4, 50, 0, 50.4, 40, 0, 49.6, 40, 0, 49.6, 50, 0, 0, 50, 0)}, CutoutWidthRule{}, false, []got{{[]string{"p"}, "0.8"}}},
		{"finger above limit", []geometry.Contour{finger(1.1)}, FeatureWidthRule{}, false, nil},
		{"finger below limit", []geometry.Contour{finger(0.8)}, FeatureWidthRule{}, false, []got{{[]string{"p"}, "0.8"}}},
		{"whole thin part, no part_width", []geometry.Contour{rectContour("strip", 0, 0, 0.8, 40)}, FeatureWidthRule{}, false, []got{{[]string{"strip"}, "0.8"}}},
		{"whole thin part is part_width's", []geometry.Contour{rectContour("strip", 0, 0, 0.8, 40)}, FeatureWidthRule{}, true, nil},
		{"finger still reported with part_width", []geometry.Contour{finger(0.8)}, FeatureWidthRule{}, true, []got{{[]string{"p"}, "0.8"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testProfile()
			p.LaserCutting.Optional.CutoutWidth = minLimit(1_000_000)
			p.LaserCutting.Optional.FeatureWidth = minLimit(1_000_000)
			if tt.partWidth {
				p.LaserCutting.Optional.PartWidth = minLimit(500_000)
			}
			fs := tt.rule.Check(geometry.NewPart(tt.contours, nil), p)
			if !sameFindings(summarize(fs), tt.want) {
				t.Errorf("got %+v, want %+v", summarize(fs), tt.want)
			}
			for _, f := range fs {
				if f.Location == nil || f.Code != tt.rule.Code() {
					t.Errorf("finding %+v", f)
				}
			}
		})
	}
}

func TestCornerRule(t *testing.T) {
	const q = 0.41421356237309503 // bulge of a counter-clockwise quarter arc
	rounded := func(r float64) geometry.Contour {
		return polyContour("c", 10+r, 10, 0, 30-r, 10, q, 30, 10+r, 0, 30, 20-r, q, 30-r, 20, 0, 10+r, 20, q, 10, 20-r, 0, 10, 10+r, q)
	}
	tests := []struct {
		name string
		cut  geometry.Contour
		want []got
	}{
		{"sharp corners", polyContour("c", 10, 10, 0, 30, 10, 0, 40, 20, 0, 10, 20, 0), []got{{[]string{"c"}, "0"}}},
		{"fillets above limit", rounded(0.6), nil},
		{"fillets at limit", rounded(0.5), nil},
		{"fillets below limit", rounded(0.4), []got{{[]string{"c"}, "0.4"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testProfile()
			p.LaserCutting.Optional.InternalCornerRadius = minLimit(500_000)
			fs := CornerRule{}.Check(geometry.NewPart([]geometry.Contour{rectContour("plate", 0, 0, 100, 60), tt.cut}, nil), p)
			if !sameFindings(summarize(fs), tt.want) {
				t.Errorf("got %+v, want %+v", summarize(fs), tt.want)
			}
		})
	}
}
