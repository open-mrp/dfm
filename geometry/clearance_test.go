package geometry

import (
	"math"
	"math/rand/v2"
	"testing"
)

// loopIndex returns the Loops index whose entity IDs start with id.
func loopIndex(t *testing.T, p *Part, id string) int {
	t.Helper()
	for i, l := range p.Loops {
		if len(l.EntityIDs) > 0 && l.EntityIDs[0] == id {
			return i
		}
	}
	t.Fatalf("no loop %s", id)
	return -1
}

func TestLoopClearance(t *testing.T) {
	p := NewPart([]Contour{square("plate", 0, 0, 100)}, []Circle{
		circ("a", 20, 50, 5), circ("b", 32.1, 50, 5), // 2.1 mm web
		circ("c", 7, 80, 5), // 2 mm from the left edge
		circ("far", 80, 20, 5),
	})
	a, b, c, plate := loopIndex(t, p, "a"), loopIndex(t, p, "b"), loopIndex(t, p, "c"), loopIndex(t, p, "plate")

	cl, ok := p.LoopClearance(a, b, 5)
	if !ok || math.Abs(cl.DistanceMM-2.1) > 1e-9 || cl.Midpoint().Dist(Point{26.05, 50}) > 1e-9 {
		t.Errorf("a-b clearance %+v ok=%v", cl, ok)
	}
	if _, ok := p.LoopClearance(a, b, 2); ok {
		t.Error("clearance beyond radius reported")
	}
	cl, ok = p.LoopClearance(c, plate, 5)
	if !ok || math.Abs(cl.DistanceMM-2) > 1e-9 || cl.PointB.Dist(Point{0, 80}) > 1e-9 {
		t.Errorf("c-edge clearance %+v ok=%v", cl, ok)
	}

	sib := p.SiblingClearances(3)
	if len(sib) != 1 || sib[0].A != a || sib[0].B != b {
		t.Errorf("siblings %+v", sib)
	}
}

func TestSiblingClearancesRespectRegions(t *testing.T) {
	// A hole inside an island is not compared with holes in the plate: the cut-out between them is empty space, not a web.
	p := NewPart([]Contour{square("plate", 0, 0, 100), square("window", 20, 20, 60)}, []Circle{
		circ("island", 50, 50, 20), circ("island-hole", 50, 50, 5),
		circ("plate-hole", 10, 50, 2),
	})
	for _, c := range p.SiblingClearances(100) {
		ids := p.Loops[c.A].EntityIDs[0] + "/" + p.Loops[c.B].EntityIDs[0]
		if ids == "island-hole/plate-hole" || ids == "plate-hole/island-hole" {
			t.Errorf("compared across regions: %s", ids)
		}
	}
}

func TestMinWidthExact(t *testing.T) {
	obround := closedPoly("o", [3]float64{-5, -1, 0}, [3]float64{5, -1, 1}, [3]float64{5, 1, 0}, [3]float64{-5, 1, 1})
	lShape := closedPoly("L", [3]float64{0, 0, 0}, [3]float64{100, 0, 0}, [3]float64{100, 10, 0}, [3]float64{10, 10, 0}, [3]float64{10, 100, 0}, [3]float64{0, 100, 0})
	tests := []struct {
		name string
		p    *Part
		want float64
	}{
		{"rectangle", NewPart([]Contour{closedPoly("r", [3]float64{0, 0, 0}, [3]float64{100, 0, 0}, [3]float64{100, 50, 0}, [3]float64{0, 50, 0})}, nil), 50},
		{"rotated rectangle", NewPart([]Contour{rotate(closedPoly("r", [3]float64{0, 0, 0}, [3]float64{100, 0, 0}, [3]float64{100, 6, 0}, [3]float64{0, 6, 0}), 30)}, nil), 6},
		{"disc", NewPart(nil, []Circle{circ("d", 3, 4, 7)}), 14},
		{"obround", NewPart([]Contour{obround}, nil), 2},
		{"rotated obround", NewPart([]Contour{rotate(obround, 71)}, nil), 2},
		// The hull of an L is a pentagon; its narrowest direction is across the diagonal edge (100,10)-(10,100): (100+10)/√2, not either leg.
		{"L shape", NewPart([]Contour{lShape}, nil), 55 * math.Sqrt2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.MinWidth(0); math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("MinWidth = %v, want %v", got, tt.want)
			}
		})
	}
}

// bruteWidth samples the width in n directions.
func bruteWidth(p *Part, i, n int) float64 {
	best := math.Inf(1)
	for k := range n {
		th := math.Pi * float64(k) / float64(n)
		u := Point{math.Cos(th), math.Sin(th)}
		best = math.Min(best, support(p.Loops[i].prims, u)+support(p.Loops[i].prims, Point{-u.X, -u.Y}))
	}
	return best
}

// TestMinWidthAgainstSampling checks random closed shapes mixing lines and arcs: the exact minimum can never exceed a sampled width, and sampling finely must approach it.
func TestMinWidthAgainstSampling(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	cases := 400
	if testing.Short() {
		cases = 60
	}
	for i := range cases {
		n := 3 + rng.IntN(6)
		var pts [][3]float64
		for k := range n {
			a := 2 * math.Pi * (float64(k) + rng.Float64()*0.8) / float64(n)
			r := 5 + rng.Float64()*20
			bulge := 0.0
			if rng.IntN(2) == 0 {
				bulge = (rng.Float64() - 0.3) * 1.5
			}
			pts = append(pts, [3]float64{r * math.Cos(a), r * math.Sin(a), bulge})
		}
		p := NewPart([]Contour{closedPoly("s", pts...)}, nil)
		if len(p.Loops) != 1 {
			continue // promoted to a circle or degenerate; covered elsewhere
		}
		exact := p.MinWidth(0)
		sampled := bruteWidth(p, 0, 7200)
		if exact > sampled+1e-9 || sampled-exact > 0.01 {
			t.Fatalf("case %d: exact %v, sampled %v, shape %v", i, exact, sampled, pts)
		}
	}
}

func TestRemovedFraction(t *testing.T) {
	p := NewPart([]Contour{square("plate", 0, 0, 100), square("window", 20, 20, 60)}, []Circle{
		circ("island", 50, 50, 10), circ("hole", 5, 5, 2),
	})
	plate := loopIndex(t, p, "plate")
	want := (3600 - math.Pi*100 + math.Pi*4) / 10000
	if got := p.RemovedFraction(plate); math.Abs(got-want) > 1e-12 {
		t.Errorf("removed fraction %v, want %v", got, want)
	}
}
