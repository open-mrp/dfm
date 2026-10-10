package geometry

import (
	"math"
	"slices"
	"testing"
)

func square(id string, x, y, w float64) Contour {
	return closedPoly(id, [3]float64{x, y, 0}, [3]float64{x + w, y, 0}, [3]float64{x + w, y + w, 0}, [3]float64{x, y + w, 0})
}

func circ(id string, x, y, r float64) Circle {
	return Circle{Center: Point{x, y}, Radius: r, EntityIDs: []string{id}}
}

func TestContourArea(t *testing.T) {
	obround := closedPoly("s", [3]float64{-5, -1, 0}, [3]float64{5, -1, 1}, [3]float64{5, 1, 0}, [3]float64{-5, 1, 1})
	tests := []struct {
		name string
		c    Contour
		want float64
	}{
		{"square", square("a", 0, 0, 10), 100},
		{"square clockwise", closedPoly("a", [3]float64{0, 0, 0}, [3]float64{0, 10, 0}, [3]float64{10, 10, 0}, [3]float64{10, 0, 0}), 100},
		{"obround", obround, 20 + math.Pi},
		{"obround reversed", Contour{Closed: true, Segments: reversed(obround.Segments)}, 20 + math.Pi},
		// A square with one side bulging inward by a semicircle of radius 2.
		{"square with notch", closedPoly("n", [3]float64{0, 0, 0}, [3]float64{10, 0, 0}, [3]float64{10, 10, 0}, [3]float64{7, 10, -1}, [3]float64{3, 10, 0}, [3]float64{0, 10, 0}), 100 - 2*math.Pi},
	}
	for _, tt := range tests {
		if got := contourArea(tt.c); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("%s: area %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestContainment(t *testing.T) {
	tests := []struct {
		name     string
		contours []Contour
		circles  []Circle
		want     map[string]int // entity ID → depth
	}{
		{
			name:     "plate with hole and slot",
			contours: []Contour{square("plate", 0, 0, 100), closedPoly("slot", [3]float64{40, 40, 0}, [3]float64{60, 40, 1}, [3]float64{60, 42, 0}, [3]float64{40, 42, 1})},
			circles:  []Circle{circ("hole", 20, 20, 3)},
			want:     map[string]int{"plate": 0, "slot": 1, "hole": 1},
		},
		{
			name:     "island inside a cut-out, with its own hole",
			contours: []Contour{square("plate", 0, 0, 100), square("window", 20, 20, 60)},
			circles:  []Circle{circ("island", 50, 50, 20), circ("island-hole", 50, 50, 5)},
			want:     map[string]int{"plate": 0, "window": 1, "island": 2, "island-hole": 3},
		},
		{
			name:     "two separate parts",
			contours: []Contour{square("a", 0, 0, 10), square("b", 20, 0, 10)},
			circles:  []Circle{circ("a-hole", 5, 5, 1)},
			want:     map[string]int{"a": 0, "b": 0, "a-hole": 1},
		},
		{
			name:    "disc with hole",
			circles: []Circle{circ("disc", 0, 0, 50), circ("hole", 10, 0, 5)},
			want:    map[string]int{"disc": 0, "hole": 1},
		},
		{
			// The probe ray from the inner square passes exactly through a vertex of the L-shaped outline; the half-open rule must still count it once.
			name: "ray through vertex of concave outline",
			contours: []Contour{
				closedPoly("L", [3]float64{0, 0, 0}, [3]float64{30, 0, 0}, [3]float64{30, 10, 0}, [3]float64{10, 10, 0}, [3]float64{10, 30, 0}, [3]float64{0, 30, 0}),
				closedPoly("cut", [3]float64{2, 8, 0}, [3]float64{6, 8, 0}, [3]float64{6, 12, 0}, [3]float64{2, 12, 0}),
			},
			want: map[string]int{"L": 0, "cut": 1},
		},
		{
			// A hole sitting in the bite of a C-shaped outline (inside its bounding box but outside the material) is a separate part.
			name: "loop in a concave bite is not inside",
			contours: []Contour{
				closedPoly("C", [3]float64{0, 0, 0}, [3]float64{30, 0, 0}, [3]float64{30, 5, 0}, [3]float64{5, 5, 0}, [3]float64{5, 25, 0}, [3]float64{30, 25, 0}, [3]float64{30, 30, 0}, [3]float64{0, 30, 0}),
			},
			circles: []Circle{circ("bite", 20, 15, 3)},
			want:    map[string]int{"C": 0, "bite": 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart(tt.contours, tt.circles)
			got := map[string]int{}
			for _, c := range p.Contours {
				got[c.EntityIDs()[0]] = c.Depth
			}
			for _, c := range p.Circles {
				got[c.EntityIDs[0]] = c.Depth
			}
			for id, want := range tt.want {
				if got[id] != want {
					t.Errorf("%s: depth %d, want %d", id, got[id], want)
				}
			}
		})
	}
}

func TestDuplicates(t *testing.T) {
	tests := []struct {
		name     string
		contours []Contour
		circles  []Circle
		wantIDs  [][]string
		wantOpen int // open contours left after duplicates are dropped
	}{
		{"line drawn twice", []Contour{line(0, 0, 10, 0, "a"), line(10, 0, 0, 0, "b")}, nil, [][]string{{"a", "b"}}, 1},
		{"circle drawn twice", nil, []Circle{circ("a", 0, 0, 5), circ("b", 0, 0, 5)}, [][]string{{"a", "b"}}, 0},
		{"rectangle drawn twice", []Contour{square("a", 0, 0, 10), square("b", 0, 0, 10)}, nil, [][]string{{"a", "b"}}, 0},
		// The single line is the copy whether it comes before or after the rectangle whose edge it repeats.
		{"line before the edge it repeats", []Contour{line(0, 0, 10, 0, "l"), square("sq", 0, 0, 10)}, nil, [][]string{{"sq", "l"}}, 0},
		{"line after the edge it repeats", []Contour{square("sq", 0, 0, 10), line(10, 0, 0, 0, "l")}, nil, [][]string{{"sq", "l"}}, 0},
		{"line drawn three times", []Contour{line(0, 0, 10, 0, "a"), line(0, 0, 10, 0, "b"), line(0, 0, 10, 0, "c")}, nil, [][]string{{"a", "b"}, {"a", "c"}}, 1},
		// The overlap is reported; the shared stretch joins the lines into one path.
		{"partial overlap is not a duplicate", []Contour{line(0, 0, 10, 0, "a"), line(8, 0, 20, 0, "b")}, nil, nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart(tt.contours, tt.circles)
			var got [][]string
			for _, d := range p.Duplicates {
				got = append(got, append(slices.Clone(d.Kept), d.Copies...))
			}
			if !slices.EqualFunc(got, tt.wantIDs, slices.Equal) {
				t.Errorf("duplicates %v, want %v", got, tt.wantIDs)
			}
			if len(p.Overlaps) != 0 && tt.wantIDs != nil {
				t.Errorf("duplicates also reported as overlaps: %+v", p.Overlaps)
			}
			open := 0
			for _, c := range p.Contours {
				if !c.Closed {
					open++
				}
			}
			if open != tt.wantOpen {
				t.Errorf("%d open contours, want %d", open, tt.wantOpen)
			}
		})
	}
}

func TestOverlaps(t *testing.T) {
	tests := []struct {
		name     string
		contours []Contour
		circles  []Circle
		wantIDs  []string
		wantLen  float64
		wantOpen int // open contours left after duplicates are dropped
	}{
		{"partial collinear overlap", []Contour{line(0, 0, 10, 0, "a"), line(8, 0, 20, 0, "b")}, nil, []string{"a", "b"}, 2, 1},
		{"collinear lines meeting at a point do not overlap", []Contour{line(0, 0, 10, 0, "a"), line(10, 0, 20, 0, "b")}, nil, nil, 0, 1},
		{"parallel lines do not overlap", []Contour{line(0, 0, 10, 0, "a"), line(0, 0.5, 10, 0.5, "b")}, nil, nil, 0, 2},
		{"arc lying on a circle", []Contour{arc(5, 0, -5, 0, 1, "arc")}, []Circle{circ("c", 0, 0, 5)}, []string{"arc", "c"}, 5 * math.Pi, 1},
		{"concentric circles do not overlap", nil, []Circle{circ("a", 0, 0, 5), circ("b", 0, 0, 4)}, nil, 0, 0},
		{"rectangle drawn twice offset is an overlap", []Contour{square("a", 0, 0, 10), square("b", 5, 0, 10)}, nil, []string{"a", "b"}, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart(tt.contours, tt.circles)
			if tt.wantIDs == nil {
				if len(p.Overlaps) != 0 {
					t.Fatalf("unexpected overlaps %+v", p.Overlaps)
				}
				return
			}
			if len(p.Overlaps) != 1 {
				t.Fatalf("got %d overlaps %+v, want 1", len(p.Overlaps), p.Overlaps)
			}
			o := p.Overlaps[0]
			if !slices.Equal(o.EntityIDs, tt.wantIDs) || math.Abs(o.LengthMM-tt.wantLen) > 1e-9 {
				t.Errorf("overlap %+v, want ids %v length %v", o, tt.wantIDs, tt.wantLen)
			}
			open := 0
			for _, c := range p.Contours {
				if !c.Closed {
					open++
				}
			}
			if open != tt.wantOpen {
				t.Errorf("%d open contours, want %d", open, tt.wantOpen)
			}
		})
	}
}

func TestContacts(t *testing.T) {
	tests := []struct {
		name     string
		contours []Contour
		circles  []Circle
		want     []Contact
	}{
		{"separate holes", []Contour{square("plate", 0, 0, 100)}, []Circle{circ("a", 20, 20, 5), circ("b", 32, 20, 5)}, nil},
		{"tangent holes", []Contour{square("plate", 0, 0, 100)}, []Circle{circ("a", 20, 20, 5), circ("b", 30, 20, 5)},
			[]Contact{{EntityIDs: []string{"a", "b"}, Location: Point{25, 20}}}},
		{"crossing holes", []Contour{square("plate", 0, 0, 100)}, []Circle{circ("a", 20, 20, 5), circ("b", 29, 20, 5)},
			[]Contact{{EntityIDs: []string{"a", "b"}}}},
		{"hole touching outline", []Contour{square("plate", 0, 0, 100)}, []Circle{circ("a", 5, 50, 5)},
			[]Contact{{EntityIDs: []string{"plate", "a"}, Location: Point{0, 50}}}},
		{"bow tie self-intersects", []Contour{closedPoly("bow", [3]float64{0, 0, 0}, [3]float64{20, 20, 0}, [3]float64{0, 20, 0}, [3]float64{20, 0, 0})}, nil,
			[]Contact{{EntityIDs: []string{"bow"}, SameContour: true, Location: Point{10, 10}}}},
		{"rectangle corners are not contacts", []Contour{square("plate", 0, 0, 100)}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart(tt.contours, tt.circles)
			if len(p.Contacts) != len(tt.want) {
				t.Fatalf("got contacts %+v, want %d", p.Contacts, len(tt.want))
			}
			for i, w := range tt.want {
				c := p.Contacts[i]
				if !slices.Equal(c.EntityIDs, w.EntityIDs) || c.SameContour != w.SameContour {
					t.Errorf("contact %+v, want %+v", c, w)
				}
				if w.Location != (Point{}) && c.Location.Dist(w.Location) > 1e-6 {
					t.Errorf("location %v, want %v", c.Location, w.Location)
				}
			}
		})
	}
}

// TestContainsNearVertex splits one vertex of a square into two copies 2e-15 apart, straddling the probe ray, as arcs recomputing a shared joint do. Counting must not depend on which side of the ray each copy lands.
func TestContainsNearVertex(t *testing.T) {
	pieces := []monotone{
		{a: Point{-10, -10}, b: Point{10, -10}},
		{a: Point{10, -10}, b: Point{10, -1e-15}},
		{a: Point{10, 1e-15}, b: Point{10, 10}},
		{a: Point{10, 10}, b: Point{-10, 10}},
		{a: Point{-10, 10}, b: Point{-10, -10}},
	}
	idx := newBandIndex(pieces)
	if !idx.contains(Point{0, 0}) {
		t.Error("center of the square reported outside")
	}
	if idx.contains(Point{20, 0}) {
		t.Error("point right of the square reported inside")
	}
	if !idx.contains(Point{9.9, 1e-16}) {
		t.Error("point near the split vertex reported outside")
	}
}

func TestParts(t *testing.T) {
	// Two plates side by side, each with a hole, and a third plate holding an island with its own hole.
	p := NewPart(
		[]Contour{square("A", 0, 0, 40), square("B", 50, 0, 40), square("C", 100, 0, 40), square("C-cut", 105, 5, 30), square("C-island", 110, 10, 20)},
		[]Circle{circ("a-hole", 20, 20, 5), circ("b-hole", 70, 20, 5), circ("island-hole", 120, 20, 3)},
	)
	parts := p.Parts(false)
	if len(parts) != 3 {
		t.Fatalf("%d parts, want 3", len(parts))
	}
	want := [][]string{{"A", "a-hole"}, {"B", "b-hole"}, {"C", "C-cut", "C-island", "island-hole"}}
	for i, part := range parts {
		var ids []string
		for _, l := range part.Loops {
			ids = append(ids, l.EntityIDs...)
		}
		if !slices.Equal(ids, want[i]) {
			t.Errorf("part %d loops %v, want %v", i+1, ids, want[i])
		}
		if outers := part.LoopIDs(func(d int) bool { return d == 0 }); len(outers) != 1 {
			t.Errorf("part %d has %d outer boundaries", i+1, len(outers))
		}
	}
	if parts[2].Bounds.MinX != 100 || parts[2].Bounds.MaxX != 140 {
		t.Errorf("part 3 bounds %+v", parts[2].Bounds)
	}

	// An outline 0.5 mm short of closing is one unclosed part holding its hole, not a hole posing as a part; a stray line is no part at all.
	broken := NewPart([]Contour{
		square("A", 0, 0, 40),
		line(50, 0, 90, 0, "e1"), line(90, 0, 90, 40, "e2"), line(90, 40, 50, 40, "e3"), line(50, 40, 50, 0.5, "e4"),
		line(0, 100, 30, 130, "stray"),
	}, []Circle{circ("hole", 70, 20, 5)})
	parts = broken.Parts(false)
	if len(parts) != 2 || parts[0].Unclosed || !parts[1].Unclosed {
		t.Fatalf("parts %d, want a closed and an unclosed one", len(parts))
	}
	if !slices.Equal(parts[1].Outline, []string{"e1", "e2", "e3", "e4"}) || !parts[1].Owns("hole") || parts[1].Owns("stray") || parts[0].Owns("stray") {
		t.Errorf("unclosed part outline %v", parts[1].Outline)
	}
	if !parts[0].Covers(Point{20, 20}, false) || parts[0].Covers(Point{40, 20}, false) || !parts[0].Covers(Point{40, 20}, true) {
		t.Error("part 1 coverage")
	}

	// In cut-outs: the island in plate C is a part of its own, holding its hole, and plate C keeps only its window.
	parts = p.Parts(true)
	if len(parts) != 4 {
		t.Fatalf("%d parts in cut-outs mode, want 4", len(parts))
	}
	for i, w := range [][]string{{"A", "a-hole"}, {"B", "b-hole"}, {"C", "C-cut"}, {"C-island", "island-hole"}} {
		var ids []string
		for _, l := range parts[i].Loops {
			ids = append(ids, l.EntityIDs...)
		}
		if !slices.Equal(ids, w) {
			t.Errorf("cut-outs mode part %d loops %v, want %v", i+1, ids, w)
		}
	}

	// A single part, open paths and all, is returned whole.
	single := NewPart([]Contour{square("A", 0, 0, 40), line(100, 100, 110, 100, "stray")}, nil)
	if got := single.Parts(false); len(got) != 1 || got[0] != single {
		t.Errorf("single part split into %d", len(got))
	}
}
