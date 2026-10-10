package geometry

import "testing"

func TestSpansMaterial(t *testing.T) {
	// A 100 x 60 plate with a 20 x 20 window in the middle, and a 0.05 mm wide sliver cut-out near its top edge.
	plate := closedPoly("o", [3]float64{0, 0, 0}, [3]float64{100, 0, 0}, [3]float64{100, 60, 0}, [3]float64{0, 60, 0})
	window := square("w", 40, 20, 20)
	sliver := closedPoly("s", [3]float64{20, 50, 0}, [3]float64{60, 50, 0}, [3]float64{60, 50.05, 0}, [3]float64{20, 50.05, 0})
	p := Build([]Contour{plate, window, sliver}, nil, BuildOptions{})
	tol := EndpointTolerance
	for _, tt := range []struct {
		name string
		a, b Point
		want bool
	}{
		{"edge to edge through material", Point{0, 10}, Point{100, 10}, true},
		{"edge to edge across the window", Point{0, 30}, Point{100, 30}, true},
		{"edge to the window", Point{0, 30}, Point{40, 30}, true},
		{"one end in material", Point{0, 10}, Point{50, 10}, false},
		{"ends on a sliver", Point{0, 50}, Point{20, 50.02}, false},
		{"runs along an edge outside the part", Point{0, 60}, Point{100, 60}, false},
		{"starts in the window", Point{40, 30}, Point{60, 30}, false},
		{"too short", Point{0, 10}, Point{0.2, 10}, false},
	} {
		if got := p.SpansMaterial(tt.a, tt.b, nil, tol); got != tt.want {
			t.Errorf("%s: SpansMaterial = %v, want %v", tt.name, got, tt.want)
		}
	}
	// A line between two plates leaves the part.
	two := Build([]Contour{square("a", 0, 0, 10), square("b", 20, 0, 10)}, nil, BuildOptions{})
	if two.SpansMaterial(Point{0, 5}, Point{30, 5}, nil, tol) {
		t.Error("a line across the gap between two parts spans material")
	}
	// The line's own path is left out when it is named.
	withLine := Build([]Contour{plate, line(0, 10, 100, 10, "l")}, nil, BuildOptions{})
	if !withLine.SpansMaterial(Point{0, 10}, Point{100, 10}, []string{"l"}, tol) {
		t.Error("a drawn line does not span material when it is left out")
	}
}

func TestStraightEnds(t *testing.T) {
	if a, b, ok := StraightEnds(line(0, 0, 10, 0, "l")); !ok || a != (Point{0, 0}) || b != (Point{10, 0}) {
		t.Errorf("line: %v %v %v", a, b, ok)
	}
	collinear := Contour{Segments: append(line(0, 0, 4, 0, "p").Segments, line(4, 0, 10, 0, "p").Segments...)}
	if _, _, ok := StraightEnds(collinear); !ok {
		t.Error("collinear polyline is not straight")
	}
	bent := Contour{Segments: append(line(0, 0, 4, 0, "p").Segments, line(4, 0, 10, 1, "p").Segments...)}
	if _, _, ok := StraightEnds(bent); ok {
		t.Error("bent polyline is straight")
	}
	if _, _, ok := StraightEnds(arc(0, 0, 10, 0, 0.5, "a")); ok {
		t.Error("arc is straight")
	}
	if _, _, ok := StraightEnds(square("q", 0, 0, 10)); ok {
		t.Error("closed square is straight")
	}
}
