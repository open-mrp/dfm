package geometry

import (
	"math"
	"slices"
	"testing"
)

const eps = 1e-9

func near(a, b float64) bool { return math.Abs(a-b) < eps }

func line(x0, y0, x1, y1 float64, id string) Contour {
	return Contour{Segments: []Segment{{Start: Point{x0, y0}, End: Point{x1, y1}, EntityID: id}}}
}

func arc(x0, y0, x1, y1, bulge float64, id string) Contour {
	return Contour{Segments: []Segment{{Start: Point{x0, y0}, End: Point{x1, y1}, Bulge: bulge, EntityID: id}}}
}

func TestSegmentArcMath(t *testing.T) {
	// Quarter circle, radius 10 about the origin, CCW from (10,0) to (0,10).
	q := Segment{Start: Point{10, 0}, End: Point{0, 10}, Bulge: math.Tan(math.Pi / 8)}
	if c := q.Center(); !near(c.X, 0) || !near(c.Y, 0) {
		t.Errorf("center = %+v", c)
	}
	if !near(q.Radius(), 10) || !near(q.Sweep(), math.Pi/2) {
		t.Errorf("radius %v sweep %v", q.Radius(), q.Sweep())
	}
	// The same chord traversed clockwise is the 270° arc through the other three quadrants, so its bounds span the full circle.
	cw := Segment{Start: Point{10, 0}, End: Point{0, 10}, Bulge: -math.Tan(3 * math.Pi / 8)}
	b := cw.Bounds()
	if !near(b.MinX, -10) || !near(b.MinY, -10) || !near(b.MaxX, 10) || !near(b.MaxY, 10) {
		t.Errorf("cw bounds = %+v", b)
	}
	// A semicircle above the X axis reaches y = r.
	semi := Segment{Start: Point{-5, 0}, End: Point{5, 0}, Bulge: -1}
	if b := semi.Bounds(); !near(b.MaxY, 5) || !near(b.MinY, 0) {
		t.Errorf("semicircle bounds = %+v", b)
	}
	if r := semi.Reverse(); r.Bulge != 1 || r.Start != semi.End {
		t.Errorf("reverse = %+v", r)
	}
}

func TestChaining(t *testing.T) {
	tests := []struct {
		name       string
		paths      []Contour
		wantClosed []bool
		wantIDs    [][]string
	}{
		{
			name: "out of order and reversed lines close",
			paths: []Contour{
				line(10, 10, 0, 10, "c"),
				line(0, 0, 10, 0, "a"),
				line(0, 0, 0, 10, "d"), // reversed relative to traversal
				line(10, 0, 10, 10, "b"),
			},
			wantClosed: []bool{true},
			wantIDs:    [][]string{{"c", "d", "a", "b"}},
		},
		{
			name: "gap larger than tolerance stays open",
			paths: []Contour{
				line(0, 0, 10, 0, "a"),
				line(10, 0, 10, 10, "b"),
				line(10, 10, 0, 10, "c"),
				line(0, 10, 0, 0.01, "d"),
			},
			wantClosed: []bool{false},
			wantIDs:    [][]string{{"a", "b", "c", "d"}},
		},
		{
			name: "gap within tolerance closes",
			paths: []Contour{
				line(0, 0, 10, 0, "a"),
				line(10, 0, 10, 10, "b"),
				line(10, 10, 0, 10, "c"),
				line(0, 10, 0, 0.0005, "d"),
			},
			wantClosed: []bool{true},
			wantIDs:    [][]string{{"a", "b", "c", "d"}},
		},
		{
			name:       "disjoint pieces stay separate",
			paths:      []Contour{line(0, 0, 1, 0, "a"), line(5, 5, 6, 5, "b")},
			wantClosed: []bool{false, false},
			wantIDs:    [][]string{{"a"}, {"b"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart(tt.paths, nil)
			if len(p.Contours) != len(tt.wantClosed) {
				t.Fatalf("got %d contours, want %d", len(p.Contours), len(tt.wantClosed))
			}
			for i, c := range p.Contours {
				if c.Closed != tt.wantClosed[i] {
					t.Errorf("contour %d closed = %v", i, c.Closed)
				}
				if got := c.EntityIDs(); !slices.Equal(got, tt.wantIDs[i]) {
					t.Errorf("contour %d ids = %v, want %v", i, got, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestArcsPromotedToCircle(t *testing.T) {
	p := NewPart([]Contour{
		line(-20, -20, 20, -20, "o1"), line(20, -20, 20, 20, "o2"),
		line(20, 20, -20, 20, "o3"), line(-20, 20, -20, -20, "o4"),
		arc(1, 0, -1, 0, 1, "h1"),
		arc(-1, 0, 1, 0, 1, "h2"),
	}, nil)
	holes := p.Holes()
	if len(holes) != 1 || !near(holes[0].Diameter(), 2) || !slices.Equal(holes[0].EntityIDs, []string{"h1", "h2"}) {
		t.Fatalf("holes = %+v", holes)
	}
	if len(p.Contours) != 1 {
		t.Fatalf("contours = %d, want 1 (the outline)", len(p.Contours))
	}
}

func TestOuterCircleIsNotAHole(t *testing.T) {
	p := NewPart(nil, []Circle{
		{Center: Point{0, 0}, Radius: 2, EntityIDs: []string{"hole"}},
		{Center: Point{0, 0}, Radius: 50, EntityIDs: []string{"outer"}},
	})
	holes := p.Holes()
	if len(holes) != 1 || holes[0].EntityIDs[0] != "hole" {
		t.Fatalf("holes = %+v", holes)
	}
	if !near(p.Bounds.WidthMM(), 100) || !near(p.Bounds.HeightMM(), 100) {
		t.Fatalf("bounds = %+v", p.Bounds)
	}
}

func closedPoly(id string, pts ...[3]float64) Contour {
	var c Contour
	c.Closed = true
	for i, p := range pts {
		n := pts[(i+1)%len(pts)]
		c.Segments = append(c.Segments, Segment{Start: Point{p[0], p[1]}, End: Point{n[0], n[1]}, Bulge: p[2], EntityID: id})
	}
	return c
}

func rotate(c Contour, deg float64) Contour {
	a := deg * math.Pi / 180
	rot := func(p Point) Point {
		return Point{p.X*math.Cos(a) - p.Y*math.Sin(a), p.X*math.Sin(a) + p.Y*math.Cos(a)}
	}
	out := Contour{Closed: c.Closed}
	for _, s := range c.Segments {
		out.Segments = append(out.Segments, Segment{Start: rot(s.Start), End: rot(s.End), Bulge: s.Bulge, EntityID: s.EntityID})
	}
	return out
}

func TestSlotRecognition(t *testing.T) {
	outer := closedPoly("outer", [3]float64{-100, -100, 0}, [3]float64{100, -100, 0}, [3]float64{100, 100, 0}, [3]float64{-100, 100, 0})
	obround := closedPoly("s", [3]float64{-5, -1, 0}, [3]float64{5, -1, 1}, [3]float64{5, 1, 0}, [3]float64{-5, 1, 1})
	tests := []struct {
		name      string
		contour   Contour
		wantShape SlotShape
		wantWidth float64
		wantLen   float64
	}{
		{"obround", obround, SlotObround, 2, 12},
		{"obround starting with arc", Contour{Closed: true, Segments: append(obround.Segments[1:4:4], obround.Segments[0])}, SlotObround, 2, 12},
		{"obround rotated 37°", rotate(obround, 37), SlotObround, 2, 12},
		{"rectangle", closedPoly("r", [3]float64{0, 0, 0}, [3]float64{30, 0, 0}, [3]float64{30, 0.5, 0}, [3]float64{0, 0.5, 0}), SlotRectangular, 0.5, 30},
		{"rectangle rotated 45°", rotate(closedPoly("r", [3]float64{0, 0, 0}, [3]float64{3, 0, 0}, [3]float64{3, 10, 0}, [3]float64{0, 10, 0}), 45), SlotRectangular, 3, 10},
		{"trapezoid is not a slot", closedPoly("t", [3]float64{0, 0, 0}, [3]float64{10, 0, 0}, [3]float64{8, 2, 0}, [3]float64{2, 2, 0}), "", 0, 0},
		{"triangle is not a slot", closedPoly("t", [3]float64{0, 0, 0}, [3]float64{10, 0, 0}, [3]float64{5, 2, 0}), "", 0, 0},
		{"unequal end radii is not a slot", closedPoly("u", [3]float64{-5, -1, 0}, [3]float64{5, -1, 0.5}, [3]float64{5, 1, 0}, [3]float64{-5, 1, 1}), "", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart([]Contour{outer, tt.contour}, nil)
			if tt.wantShape == "" {
				if len(p.Slots) != 0 {
					t.Fatalf("unexpected slot %+v", p.Slots)
				}
				return
			}
			if len(p.Slots) != 1 {
				t.Fatalf("got %d slots", len(p.Slots))
			}
			s := p.Slots[0]
			if s.Shape != tt.wantShape || math.Abs(s.WidthMM-tt.wantWidth) > 1e-9 || math.Abs(s.LengthMM-tt.wantLen) > 1e-9 {
				t.Errorf("slot = %+v", s)
			}
		})
	}
}

func TestOuterRectangleIsNotASlot(t *testing.T) {
	p := NewPart([]Contour{closedPoly("strip", [3]float64{0, 0, 0}, [3]float64{100, 0, 0}, [3]float64{100, 0.5, 0}, [3]float64{0, 0.5, 0})}, nil)
	if len(p.Slots) != 0 {
		t.Fatalf("outer profile reported as slot: %+v", p.Slots)
	}
}

func TestWidthAndLength(t *testing.T) {
	// A 6 x 40 strip turned 30°, a disc and a square.
	strip := closedPoly("s", [3]float64{0, 0, 0}, [3]float64{40 * math.Cos(math.Pi/6), 40 * math.Sin(math.Pi/6), 0},
		[3]float64{40*math.Cos(math.Pi/6) - 6*math.Sin(math.Pi/6), 40*math.Sin(math.Pi/6) + 6*math.Cos(math.Pi/6), 0},
		[3]float64{-6 * math.Sin(math.Pi/6), 6 * math.Cos(math.Pi/6), 0})
	for _, tt := range []struct {
		name          string
		p             *Part
		width, length float64
	}{
		{"turned strip", NewPart([]Contour{strip}, nil), 6, 40},
		{"disc", NewPart(nil, []Circle{circ("d", 0, 0, 5)}), 10, 10},
		{"square", NewPart([]Contour{square("q", 0, 0, 10)}, nil), 10, 10},
	} {
		w, l := tt.p.WidthAndLength(0)
		if math.Abs(w-tt.width) > 1e-6 || math.Abs(l-tt.length) > 1e-6 {
			t.Errorf("%s: %g x %g, want %g x %g", tt.name, w, l, tt.width, tt.length)
		}
	}
}
