package geometry

import (
	"math"
	"slices"
	"testing"
)

// poly builds a closed contour; reverse flips its orientation.
func poly(id string, reverse bool, pts ...[3]float64) Contour {
	c := closedPoly(id, pts...)
	if reverse {
		c.Segments = reversed(c.Segments)
	}
	return c
}

func pts(xy ...float64) [][3]float64 {
	var out [][3]float64
	for i := 0; i+1 < len(xy); i += 2 {
		out = append(out, [3]float64{xy[i], xy[i+1], 0})
	}
	return out
}

const fillet = -0.41421356237309503 // bulge of a clockwise quarter arc

type neckWant struct {
	id   string
	kind NeckKind
	mm   float64
}

func TestNecks(t *testing.T) {
	plate := func(rev bool) Contour { return poly("plate", rev, pts(0, 0, 100, 0, 100, 60, 0, 60)...) }
	tests := []struct {
		name     string
		contours func(rev bool) []Contour
		circles  []Circle
		want     []neckWant
	}{
		{"finger", func(rev bool) []Contour {
			return []Contour{poly("finger", rev, pts(0, 0, 40, 0, 40, 20, 21.5, 20, 21.5, 40, 18.5, 40, 18.5, 20, 0, 20)...)}
		}, nil, []neckWant{{"finger", NeckMaterial, 3}}},
		{"finger with filleted base", func(rev bool) []Contour {
			return []Contour{poly("finger", rev,
				[3]float64{0, 0, 0}, [3]float64{40, 0, 0}, [3]float64{40, 20, 0}, [3]float64{22, 20, fillet},
				[3]float64{21.5, 20.5, 0}, [3]float64{21.5, 40, 0}, [3]float64{18.5, 40, 0}, [3]float64{18.5, 20.5, fillet},
				[3]float64{18, 20, 0}, [3]float64{0, 20, 0})}
		}, nil, []neckWant{{"finger", NeckMaterial, 3}}},
		{"rounded rectangle", func(rev bool) []Contour {
			return []Contour{poly("r", rev,
				[3]float64{0.5, 0, 0}, [3]float64{99.5, 0, -fillet}, [3]float64{100, 0.5, 0}, [3]float64{100, 49.5, -fillet},
				[3]float64{99.5, 50, 0}, [3]float64{0.5, 50, -fillet}, [3]float64{0, 49.5, 0}, [3]float64{0, 0.5, -fillet})}
		}, nil, nil},
		{"chamfered rectangle", func(rev bool) []Contour {
			return []Contour{poly("r", rev, pts(0.5, 0, 99.5, 0, 100, 0.5, 100, 49.5, 99.5, 50, 0.5, 50, 0, 49.5, 0, 0.5)...)}
		}, nil, nil},
		{"hourglass cut-out", func(rev bool) []Contour {
			return []Contour{plate(rev), poly("glass", rev, pts(40, 10, 60, 10, 51, 25, 60, 40, 40, 40, 49, 25)...)}
		}, nil, []neckWant{{"glass", NeckVoid, 2}}},
		{"tongue inside a U cut-out", func(rev bool) []Contour {
			return []Contour{plate(rev), poly("u", rev, pts(20, 10, 60, 10, 60, 40, 42, 40, 42, 20, 38, 20, 38, 40, 20, 40)...)}
		}, nil, []neckWant{{"u", NeckMaterial, 4}}},
		{"notch in the edge", func(rev bool) []Contour {
			return []Contour{poly("p", rev, pts(0, 0, 100, 0, 100, 50, 51, 50, 51, 40, 49, 40, 49, 50, 0, 50)...)}
		}, nil, []neckWant{{"p", NeckVoid, 2}}},
		// A sharp notch whose tip is split, so its long sides are not adjacent: it narrows to a point, a tip rather than a neck.
		{"notch tapering to a point", func(rev bool) []Contour {
			return []Contour{poly("p", rev, pts(0, 0, 100, 0, 100, 50, 52, 50, 50.2, 40.4, 50, 40, 48, 50, 0, 50)...)}
		}, nil, nil},
		// A slot with faceted ends: across an end, the line from one facet to another leans off both, so the slot's width is its neck.
		{"faceted slot", func(rev bool) []Contour {
			return []Contour{plate(rev), poly("slot", rev, facetedSlot(30, 0.6, 8, 18)...)}
		}, nil, []neckWant{{"slot", NeckVoid, 1.2}}},
		// The line across the strip would pass through the hole, so the strip has no single neck; the hole's webs are the edge rule's.
		{"neck blocked by a hole", func(rev bool) []Contour {
			return []Contour{poly("strip", rev, pts(0, 0, 100, 0, 100, 8, 0, 8)...)}
		}, []Circle{circ("h", 50, 4, 1)}, nil},
	}
	for _, tt := range tests {
		for _, rev := range []bool{false, true} {
			t.Run(tt.name, func(t *testing.T) {
				p := NewPart(tt.contours(rev), tt.circles)
				var got []neckWant
				for _, n := range p.Necks(5) {
					got = append(got, neckWant{p.Loops[n.Loop].EntityIDs[0], n.Kind, math.Round(n.DistanceMM*1e9) / 1e9})
				}
				if !slices.Equal(got, tt.want) {
					t.Errorf("reversed=%v: necks %+v, want %+v", rev, got, tt.want)
				}
			})
		}
	}
}

// facetedSlot is a slot of radius r between centers length apart along y = 30, starting at x = cx, with each end drawn as n straight segments.
func facetedSlot(cx, r, length float64, n int) [][3]float64 {
	var out [][3]float64
	for i := 0; i <= n; i++ {
		a := -math.Pi/2 + math.Pi*float64(i)/float64(n)
		out = append(out, [3]float64{cx + length + r*math.Cos(a), 30 + r*math.Sin(a), 0})
	}
	for i := 0; i <= n; i++ {
		a := math.Pi/2 + math.Pi*float64(i)/float64(n)
		out = append(out, [3]float64{cx + r*math.Cos(a), 30 + r*math.Sin(a), 0})
	}
	return out
}

func TestConcaveCorners(t *testing.T) {
	plate := pts(0, 0, 100, 0, 100, 60, 0, 60)
	tests := []struct {
		name     string
		contours func(rev bool) []Contour
		circles  []Circle
		radii    []float64
	}{
		{"plain rectangle", func(rev bool) []Contour { return []Contour{poly("p", rev, plate...)} }, nil, nil},
		{"rectangular cut-out", func(rev bool) []Contour {
			return []Contour{poly("p", rev, plate...), poly("c", rev, pts(10, 10, 30, 10, 30, 20, 10, 20)...)}
		}, nil, []float64{0, 0, 0, 0}},
		{"L shape", func(rev bool) []Contour {
			return []Contour{poly("L", rev, pts(0, 0, 100, 0, 100, 10, 10, 10, 10, 100, 0, 100)...)}
		}, nil, []float64{0}},
		{"filleted finger base", func(rev bool) []Contour {
			return []Contour{poly("finger", rev,
				[3]float64{0, 0, 0}, [3]float64{40, 0, 0}, [3]float64{40, 20, 0}, [3]float64{22, 20, fillet},
				[3]float64{21.5, 20.5, 0}, [3]float64{21.5, 40, 0}, [3]float64{18.5, 40, 0}, [3]float64{18.5, 20.5, fillet},
				[3]float64{18, 20, 0}, [3]float64{0, 20, 0})}
		}, nil, []float64{0.5, 0.5}},
		{"slot and hole are not corners", func(rev bool) []Contour {
			return []Contour{poly("p", rev, plate...), poly("s", rev, [3]float64{20, 20, 0}, [3]float64{40, 20, 1}, [3]float64{40, 22, 0}, [3]float64{20, 22, 1})}
		}, []Circle{circ("h", 70, 30, 2)}, nil},
	}
	for _, tt := range tests {
		for _, rev := range []bool{false, true} {
			t.Run(tt.name, func(t *testing.T) {
				var radii []float64
				for _, c := range NewPart(tt.contours(rev), tt.circles).ConcaveCorners() {
					radii = append(radii, math.Round(c.RadiusMM*1e9)/1e9)
				}
				if !slices.Equal(radii, tt.radii) {
					t.Errorf("reversed=%v: corner radii %v, want %v", rev, radii, tt.radii)
				}
			})
		}
	}
}

func TestSharpestTips(t *testing.T) {
	tests := []struct {
		name     string
		contours func(rev bool) []Contour
		circles  []Circle
		angles   []float64
	}{
		{"rectangle", func(rev bool) []Contour { return []Contour{poly("p", rev, pts(0, 0, 100, 0, 100, 60, 0, 60)...)} }, nil, []float64{90}},
		{"spike", func(rev bool) []Contour {
			return []Contour{poly("p", rev, pts(0, 0, 80, 0, 80, 20, 0, 20, -50, 10)...)}
		}, nil, []float64{22.619865}},
		{"V notch in the edge", func(rev bool) []Contour {
			return []Contour{poly("p", rev, pts(0, 0, 100, 0, 100, 50, 55, 50, 50, 20, 45, 50, 0, 50)...)}
		}, nil, []float64{18.924644}},
		{"slot and hole have no corners", func(rev bool) []Contour {
			return []Contour{poly("p", rev, [3]float64{0, 0, 0}, [3]float64{40, 0, 1}, [3]float64{40, 2, 0}, [3]float64{0, 2, 1})}
		}, []Circle{circ("h", 70, 30, 2)}, nil},
	}
	for _, tt := range tests {
		for _, rev := range []bool{false, true} {
			t.Run(tt.name, func(t *testing.T) {
				var got []float64
				for _, tip := range NewPart(tt.contours(rev), tt.circles).SharpestTips() {
					got = append(got, math.Round(tip.AngleDeg*1e6)/1e6)
				}
				if !slices.Equal(got, tt.angles) {
					t.Errorf("reversed=%v: tip angles %v, want %v", rev, got, tt.angles)
				}
			})
		}
	}
}
