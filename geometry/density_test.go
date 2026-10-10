package geometry

import (
	"math"
	"testing"
)

func TestDensestWindow(t *testing.T) {
	// A 100 x 100 plate with a 40 x 40 cut-out: a 40 mm window on the cut-out is all removed, though the plate loses 16%.
	p := NewPart([]Contour{square("plate", 0, 0, 100), square("cut", 10, 10, 40)}, nil)
	f, c, ok := p.DensestWindow(0, 40)
	if !ok || math.Abs(f-1) > 1e-9 || c != (Point{30, 30}) {
		t.Errorf("densest window %v at %v (%v), want 1 at (30, 30)", f, c, ok)
	}
	// A part smaller than half a window has none.
	if _, _, ok := NewPart([]Contour{square("tiny", 0, 0, 10)}, nil).DensestWindow(0, 40); ok {
		t.Error("a 10 mm part has a 40 mm window")
	}
}

func TestClippedArea(t *testing.T) {
	circle := loopPolygon(NewPart(nil, []Circle{circ("c", 0, 0, 10)}).Loops[0])
	if a := clippedArea(circle, Bounds{-20, -20, 20, 20}); math.Abs(a-math.Pi*100)/(math.Pi*100) > 0.01 {
		t.Errorf("whole circle area %v, want %v within 1%%", a, math.Pi*100)
	}
	if a := clippedArea(circle, Bounds{0, -20, 20, 20}); math.Abs(a-math.Pi*50)/(math.Pi*50) > 0.01 {
		t.Errorf("half circle area %v, want %v within 1%%", a, math.Pi*50)
	}
	if a := clippedArea(circle, Bounds{30, 30, 40, 40}); a != 0 {
		t.Errorf("area outside the window %v", a)
	}
}
