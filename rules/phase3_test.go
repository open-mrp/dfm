package rules

import (
	"math"
	"slices"
	"testing"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

func rectContour(id string, x, y, w, h float64) geometry.Contour {
	pts := []geometry.Point{{X: x, Y: y}, {X: x + w, Y: y}, {X: x + w, Y: y + h}, {X: x, Y: y + h}}
	c := geometry.Contour{Closed: true}
	for i := range pts {
		c.Segments = append(c.Segments, geometry.Segment{Start: pts[i], End: pts[(i+1)%4], EntityID: id})
	}
	return c
}

func hole(id string, x, y, d float64) geometry.Circle {
	return geometry.Circle{Center: geometry.Point{X: x, Y: y}, Radius: d / 2, EntityIDs: []string{id}}
}

func minLimit(mm measure.Length) *capability.Limit {
	return &capability.Limit{Error: &measure.Threshold{Min: mm}}
}

type got struct {
	ids    []string
	actual string
}

func summarize(fs []Finding) []got {
	var out []got
	for _, f := range fs {
		g := got{ids: f.EntityIDs}
		if f.Actual != nil {
			g.actual = f.Actual.String()
		}
		out = append(out, g)
	}
	return out
}

func sameFindings(a, b []got) bool {
	return slices.EqualFunc(a, b, func(x, y got) bool { return slices.Equal(x.ids, y.ids) && x.actual == y.actual })
}

func TestWebAndSpacingRules(t *testing.T) {
	plate := rectContour("plate", 0, 0, 200, 100)
	tests := []struct {
		name  string
		holes []geometry.Circle
		want  []got
	}{
		{"web above limit", []geometry.Circle{hole("a", 20, 50, 10), hole("b", 32.1, 50, 10)}, nil},
		{"web exactly at limit", []geometry.Circle{hole("a", 20, 50, 10), hole("b", 32, 50, 10)}, nil},
		{"web below limit", []geometry.Circle{hole("a", 20, 50, 10), hole("b", 31.9, 50, 10)}, []got{{[]string{"a", "b"}, "1.9"}}},
		{"touching is the contact rule's", []geometry.Circle{hole("a", 20, 50, 10), hole("b", 30, 50, 10)}, nil},
		{
			// a-b is narrowest for both a and b; b-c is narrowest for c.
			"worst gap per cut-out",
			[]geometry.Circle{hole("a", 20, 50, 10), hole("b", 31, 50, 10), hole("c", 42.5, 50, 10)},
			[]got{{[]string{"a", "b"}, "1"}, {[]string{"b", "c"}, "1.5"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part := geometry.NewPart([]geometry.Contour{plate}, tt.holes)
			p := testProfile()
			p.LaserCutting.Optional.WebWidth = minLimit(2_000_000)
			p.LaserCutting.Optional.FeatureSpacing = minLimit(2_000_000)
			for _, r := range []Rule{WebRule{}, FeatureSpacingRule{}} {
				fs := r.Check(part, p)
				if !sameFindings(summarize(fs), tt.want) {
					t.Errorf("%s: got %+v, want %+v", r.Code(), summarize(fs), tt.want)
				}
				for _, f := range fs {
					if f.Code != r.Code() || f.Location == nil || *f.RequiredBasis != constants.ThresholdBasisMin {
						t.Errorf("%s: finding %+v", r.Code(), f)
					}
				}
			}
		})
	}
	if Enabled(WebRule{}, testProfile()) {
		t.Error("web rule enabled without a web_width limit")
	}
}

func TestEdgeDistanceRule(t *testing.T) {
	tests := []struct {
		name  string
		loops []geometry.Contour
		holes []geometry.Circle
		want  []got
	}{
		{"above limit", []geometry.Contour{rectContour("plate", 0, 0, 100, 100)}, []geometry.Circle{hole("h", 7.1, 50, 10)}, nil},
		{"exactly at limit", []geometry.Contour{rectContour("plate", 0, 0, 100, 100)}, []geometry.Circle{hole("h", 7, 50, 10)}, nil},
		{"below limit", []geometry.Contour{rectContour("plate", 0, 0, 100, 100)}, []geometry.Circle{hole("h", 6.9, 50, 10)}, []got{{[]string{"h", "plate"}, "1.9"}}},
		{
			// The hole in the island is measured to the island's edge.
			"hole near an island's edge",
			[]geometry.Contour{rectContour("plate", 0, 0, 100, 100), rectContour("window", 20, 20, 60, 60)},
			[]geometry.Circle{hole("island", 50, 50, 40), hole("h", 50, 33, 4)},
			[]got{{[]string{"h", "island"}, "1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testProfile()
			p.LaserCutting.Optional.EdgeDistance = minLimit(2_000_000)
			fs := EdgeDistanceRule{}.Check(geometry.NewPart(tt.loops, tt.holes), p)
			if !sameFindings(summarize(fs), tt.want) {
				t.Errorf("got %+v, want %+v", summarize(fs), tt.want)
			}
		})
	}
}

func TestPartWidthRule(t *testing.T) {
	rot := func(c geometry.Contour, deg float64) geometry.Contour {
		a := deg * math.Pi / 180
		for i, s := range c.Segments {
			r := func(p geometry.Point) geometry.Point {
				return geometry.Point{X: p.X*math.Cos(a) - p.Y*math.Sin(a), Y: p.X*math.Sin(a) + p.Y*math.Cos(a)}
			}
			c.Segments[i].Start, c.Segments[i].End = r(s.Start), r(s.End)
		}
		return c
	}
	tests := []struct {
		name string
		c    geometry.Contour
		want []got
	}{
		{"wide enough", rectContour("p", 0, 0, 6.1, 20), nil},
		{"exactly at limit", rectContour("p", 0, 0, 6, 20), nil},
		{"too narrow", rectContour("p", 0, 0, 5.9, 20), []got{{[]string{"p"}, "5.9"}}},
		// Its bounding box is about 18 x 18 mm, but the part is 5.9 mm wide.
		{"rotated slender part", rot(rectContour("p", 0, 0, 5.9, 20), 45), []got{{[]string{"p"}, "5.9"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testProfile()
			p.LaserCutting.Optional.PartWidth = minLimit(6_000_000)
			fs := PartWidthRule{}.Check(geometry.NewPart([]geometry.Contour{tt.c}, nil), p)
			if !sameFindings(summarize(fs), tt.want) {
				t.Errorf("got %+v, want %+v", summarize(fs), tt.want)
			}
		})
	}
}

func TestDensityRule(t *testing.T) {
	tests := []struct {
		name     string
		w, h     float64 // window removed from a 100 x 100 plate
		severity constants.Severity
		actual   string
	}{
		{"below threshold", 70, 70, "", ""},
		{"exactly at threshold warns", 80, 62.5, constants.SeverityWarning, "50"},
		{"above threshold", 75, 74, constants.SeverityWarning, "55.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part := geometry.NewPart([]geometry.Contour{rectContour("plate", 0, 0, 100, 100), rectContour("window", 10, 10, tt.w, tt.h)}, nil)
			p := testProfile()
			half := measure.Ratio(50_000_000)
			p.LaserCutting.Optional.MaxRemovedAreaPercent = &half
			fs := DensityRule{}.Check(part, p)
			if tt.severity == "" {
				if len(fs) != 0 {
					t.Errorf("unexpected %+v", fs)
				}
				return
			}
			if len(fs) != 1 || fs[0].Severity != tt.severity || fs[0].Actual.String() != tt.actual || fs[0].Required.String() != "50" || fs[0].Unit != constants.UnitPercent {
				t.Errorf("got %+v", fs)
			}
		})
	}
}
