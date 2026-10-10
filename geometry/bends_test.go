package geometry

import (
	"math"
	"testing"
)

func TestFlangesAndBendFeatures(t *testing.T) {
	// A 100 x 60 plate bent at y = 20, with a hole 5 mm from the bend and a window the bend crosses.
	plate := closedPoly("p", [3]float64{0, 0, 0}, [3]float64{100, 0, 0}, [3]float64{100, 60, 0}, [3]float64{0, 60, 0})
	window := closedPoly("w", [3]float64{70, 15, 0}, [3]float64{80, 15, 0}, [3]float64{80, 25, 0}, [3]float64{70, 25, 0})
	p := NewPart([]Contour{plate, window}, []Circle{circ("h", 30, 27, 2)})
	p.NonCut = []NonCut{{EntityID: "b", Kind: NonCutBend, Line: &[2]Point{{0, 20}, {100, 20}}}}
	bends := p.BendLines()
	if len(bends) != 1 {
		t.Fatalf("bend lines %v", bends)
	}
	left, right, ok := p.Flanges(bends[0])
	short := math.Min(left.LengthMM, right.LengthMM)
	long := math.Max(left.LengthMM, right.LengthMM)
	if !ok || math.Abs(short-20) > 1e-9 || math.Abs(long-40) > 1e-9 {
		t.Errorf("flanges %v %v %v, want 20 and 40", left, right, ok)
	}
	got := map[string]float64{}
	for _, f := range p.BendFeatures(bends[0], 10) {
		got[p.Loops[f.Loop].EntityIDs[0]] = f.DistanceMM
	}
	if math.Abs(got["h"]-5) > 1e-9 || got["w"] != 0 || len(got) != 2 {
		t.Errorf("bend features %v, want h at 5 and w at 0", got)
	}
}

func TestBendEnds(t *testing.T) {
	pt := func(x, y float64) [3]float64 { return [3]float64{x, y, 0} }
	// A plate with a tab on top (inside corners at (30,60) and (70,60)).
	tab := closedPoly("p", pt(0, 0), pt(100, 0), pt(100, 60), pt(70, 60), pt(70, 90), pt(30, 90), pt(30, 60), pt(0, 60))
	cases := []struct {
		name   string
		loops  []Contour
		holes  []Circle
		a, b   Point
		ka, kb BendEndKind
	}{
		{"across the plate", []Contour{tab}, nil, Point{0, 30}, Point{100, 30}, BendEndFree, BendEndFree},
		{"tab at inside corners", []Contour{tab}, nil, Point{30, 60}, Point{70, 60}, BendEndInsideCorner, BendEndInsideCorner},
		{"tab reversed", []Contour{tab}, nil, Point{70, 60}, Point{30, 60}, BendEndInsideCorner, BendEndInsideCorner},
		{"stops in material", []Contour{tab}, nil, Point{0, 30}, Point{60, 30}, BendEndFree, BendEndInMaterial},
		{"along the top edge", []Contour{tab}, nil, Point{0, 60}, Point{20, 60}, BendEndAlongEdge, BendEndAlongEdge},
		{"to relief holes", []Contour{tab}, []Circle{circ("h1", 30, 30, 2), circ("h2", 70, 30, 2)}, Point{32, 30}, Point{68, 30}, BendEndFree, BendEndFree},
		{"tangent to a hole", []Contour{tab}, []Circle{circ("h1", 30, 32, 2)}, Point{30, 30}, Point{100, 30}, BendEndAlongEdge, BendEndFree},
	}
	for _, c := range cases {
		p := NewPart(c.loops, c.holes)
		p.NonCut = []NonCut{{EntityID: "b", Kind: NonCutBend, Line: &[2]Point{c.a, c.b}}}
		ends := p.BendEnds(p.BendLines()[0])
		if ends[0].Kind != c.ka || ends[1].Kind != c.kb {
			t.Errorf("%s: ends %v, want %v %v", c.name, ends, c.ka, c.kb)
		}
	}
	// A clockwise outline classifies the same.
	cw := closedPoly("p", pt(0, 60), pt(30, 60), pt(30, 90), pt(70, 90), pt(70, 60), pt(100, 60), pt(100, 0), pt(0, 0))
	p := NewPart([]Contour{cw}, nil)
	p.NonCut = []NonCut{{EntityID: "b", Kind: NonCutBend, Line: &[2]Point{{30, 60}, {70, 60}}}}
	if ends := p.BendEnds(p.BendLines()[0]); ends[0].Kind != BendEndInsideCorner || ends[1].Kind != BendEndInsideCorner {
		t.Errorf("clockwise tab: ends %v", ends)
	}
}

func TestBendReliefs(t *testing.T) {
	pt := func(x, y float64) [3]float64 { return [3]float64{x, y, 0} }
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	// A tab bent along y = 60 between slots 2 mm wide reaching 3 mm below.
	slots := closedPoly("p", pt(0, 0), pt(100, 0), pt(100, 60), pt(72, 60), pt(72, 57), pt(70, 57), pt(70, 90),
		pt(30, 90), pt(30, 57), pt(28, 57), pt(28, 60), pt(0, 60))
	p := NewPart([]Contour{slots}, nil)
	p.NonCut = []NonCut{{EntityID: "b", Kind: NonCutBend, Line: &[2]Point{{30, 60}, {70, 60}}}}
	rs := p.BendReliefs(p.BendLines()[0], 5)
	if len(rs) != 2 {
		t.Fatalf("slots: reliefs %v", rs)
	}
	for _, r := range rs {
		if !near(r.WidthMM, 2) || !near(r.DepthMM, 3) {
			t.Errorf("slots: relief %+v, want 2 wide and 3 deep", r)
		}
	}
	// A bend inside a plate ending at relief holes 4 mm across, centered on the line: 4 wide, 2 deep (the radius) on either side.
	plate := closedPoly("p", pt(0, 0), pt(100, 0), pt(100, 60), pt(0, 60))
	p = NewPart([]Contour{plate}, []Circle{circ("h1", 20, 30, 2), circ("h2", 80, 30, 2)})
	p.NonCut = []NonCut{{EntityID: "b", Kind: NonCutBend, Line: &[2]Point{{22, 30}, {78, 30}}}}
	rs = p.BendReliefs(p.BendLines()[0], 5)
	if len(rs) != 2 || !near(rs[0].WidthMM, 4) || !near(rs[0].DepthMM, 2) || !near(rs[1].WidthMM, 4) {
		t.Errorf("holes: reliefs %+v", rs)
	}
	// A bend across the plate has nothing beyond its ends; a gap wider than the window is not a relief.
	p = NewPart([]Contour{plate}, nil)
	p.NonCut = []NonCut{{EntityID: "b", Kind: NonCutBend, Line: &[2]Point{{0, 30}, {100, 30}}}}
	if rs := p.BendReliefs(p.BendLines()[0], 5); len(rs) != 0 {
		t.Errorf("across: reliefs %+v", rs)
	}
	// Edge notches the bend runs out through open on both sides.
	notched := closedPoly("p", pt(0, 0), pt(100, 0), pt(100, 28), pt(96, 28), pt(96, 32), pt(100, 32), pt(100, 60),
		pt(0, 60), pt(0, 32), pt(4, 32), pt(4, 28), pt(0, 28))
	p = NewPart([]Contour{notched}, nil)
	p.NonCut = []NonCut{{EntityID: "b", Kind: NonCutBend, Line: &[2]Point{{4, 30}, {96, 30}}}}
	if rs := p.BendReliefs(p.BendLines()[0], 5); len(rs) != 0 {
		t.Errorf("notches: reliefs %+v", rs)
	}
}

func TestFlangeBetweenBends(t *testing.T) {
	plate := closedPoly("p", [3]float64{0, 0, 0}, [3]float64{100, 0, 0}, [3]float64{100, 60, 0}, [3]float64{0, 60, 0})
	p := NewPart([]Contour{plate}, nil)
	p.NonCut = []NonCut{
		{EntityID: "b1", Kind: NonCutBend, Line: &[2]Point{{0, 20}, {100, 20}}},
		{EntityID: "b2", Kind: NonCutBend, Line: &[2]Point{{0, 26}, {100, 26}}},
	}
	left, right, ok := p.Flanges(p.BendLines()[0])
	f := left
	if right.LengthMM < f.LengthMM {
		f = right
	}
	if !ok || math.Abs(f.LengthMM-6) > 1e-9 || f.ToBend != "b2" {
		t.Errorf("flanges %+v %+v, want 6 mm to b2", left, right)
	}
}
