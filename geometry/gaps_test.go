package geometry

import (
	"math"
	"slices"
	"testing"
)

// gappedSquare is a 10 mm square of four lines whose last corner misses by gap.
func gappedSquare(gap float64) []Contour {
	return []Contour{
		line(0, 0, 10, 0, "a"),
		line(10, 0, 10, 10, "b"),
		line(10, 10, 0, 10, "c"),
		line(0, 10, 0, gap, "d"),
	}
}

func TestCloseGaps(t *testing.T) {
	tests := []struct {
		name       string
		paths      []Contour
		tol        float64
		wantGaps   []float64
		wantIDs    [][]string
		wantClosed int
		wantOpen   int
	}{
		{"gap within tolerance is bridged", gappedSquare(0.02), 0.05, []float64{0.02}, [][]string{{"a", "d"}}, 1, 0},
		{"gap beyond tolerance stays open", gappedSquare(0.08), 0.05, nil, nil, 0, 1},
		{"no tolerance bridges nothing", gappedSquare(0.02), 0, nil, nil, 0, 1},
		{
			"pieces chained into one path are closed",
			[]Contour{line(0, 0, 10, 0, "a"), line(10.03, 0, 10, 10, "b"), line(10, 10, 0, 0, "c")},
			0.05, []float64{0.03}, [][]string{{"b", "a"}}, 1, 0,
		},
		{
			"separate open paths are joined",
			[]Contour{line(0, 0, 10, 0, "a"), line(10.04, 0, 20, 0, "b")},
			0.05, []float64{0.04}, [][]string{{"a", "b"}}, 0, 1,
		},
		{"a short stray line is not looped", []Contour{line(0, 0, 0.03, 0, "s")}, 0.05, nil, nil, 0, 1},
		{
			// A circle drawn as one arc that stops 0.01 mm short of closing.
			"a near-full arc is closed",
			[]Contour{arc(5, 0, 5*math.Cos(2*math.Pi-0.002), 5*math.Sin(2*math.Pi-0.002), math.Tan((2*math.Pi-0.002)/4), "r")},
			0.05, []float64{0.01}, [][]string{{"r"}}, 1, 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Build(tt.paths, nil, BuildOptions{ClosureToleranceMM: tt.tol})
			if len(p.ClosedGaps) != len(tt.wantGaps) {
				t.Fatalf("closed gaps %+v, want %v", p.ClosedGaps, tt.wantGaps)
			}
			for i, g := range p.ClosedGaps {
				if math.Abs(g.LengthMM-tt.wantGaps[i]) > 1e-6 || !slices.Equal(g.EntityIDs, tt.wantIDs[i]) {
					t.Errorf("gap %d = %+v, want %v %v", i, g, tt.wantGaps[i], tt.wantIDs[i])
				}
			}
			closed, open := 0, 0
			for _, c := range p.Contours {
				if c.Closed {
					closed++
				} else {
					open++
				}
			}
			closed += len(p.Circles)
			if closed != tt.wantClosed || open != tt.wantOpen {
				t.Errorf("%d closed, %d open; want %d, %d", closed, open, tt.wantClosed, tt.wantOpen)
			}
		})
	}
}

func TestOpenings(t *testing.T) {
	// The gap is measured to the nearest loose end, here the other line's.
	p := NewPart([]Contour{line(0, 0, 10, 0, "a"), line(10.5, 0, 20, 5, "b")}, nil)
	if len(p.Openings) != 2 {
		t.Fatalf("openings %+v, want 2", p.Openings)
	}
	for _, o := range p.Openings {
		if math.Abs(o.GapMM-0.5) > 1e-9 || o.Location != (Point{10.25, 0}) {
			t.Errorf("opening %+v, want a 0.5 mm gap at (10.25, 0)", o)
		}
	}

	// A lone open path measures the distance between its own ends.
	p = NewPart(gappedSquare(0.3), nil)
	if len(p.Openings) != 1 || math.Abs(p.Openings[0].GapMM-0.3) > 1e-9 {
		t.Errorf("openings %+v, want one 0.3 mm gap", p.Openings)
	}

	if p := NewPart(gappedSquare(0), nil); len(p.Openings) != 0 {
		t.Errorf("closed square has openings %+v", p.Openings)
	}
}

func TestOvercut(t *testing.T) {
	// A 10 mm square drawn as one open path from (0,0), back to (0,0), then on over its first edge.
	path := func(tail ...Segment) Contour {
		segs := []Segment{
			{Start: Point{0, 0}, End: Point{10, 0}, EntityID: "p"},
			{Start: Point{10, 0}, End: Point{10, 10}, EntityID: "p"},
			{Start: Point{10, 10}, End: Point{0, 10}, EntityID: "p"},
			{Start: Point{0, 10}, End: Point{0, 0}, EntityID: "p"},
		}
		return Contour{Segments: append(segs, tail...)}
	}
	tests := []struct {
		name   string
		c      Contour
		overMM float64 // 0 for no overcut
	}{
		{"whole first edge again", path(Segment{Start: Point{0, 0}, End: Point{10, 0}, EntityID: "p"}), 10},
		{"part way along it", path(Segment{Start: Point{0, 0}, End: Point{3, 0}, EntityID: "p"}), 3},
		{"two edges again", path(Segment{Start: Point{0, 0}, End: Point{10, 0}, EntityID: "p"}, Segment{Start: Point{10, 0}, End: Point{10, 4}, EntityID: "p"}), 14},
		{"leaving the path", path(Segment{Start: Point{0, 0}, End: Point{3, 3}, EntityID: "p"}), 0},
		{"no tail", path(), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart([]Contour{tt.c}, nil)
			if tt.overMM == 0 {
				if len(p.Overcuts) != 0 {
					t.Errorf("overcuts %+v, want none", p.Overcuts)
				}
				return
			}
			if len(p.Overcuts) != 1 || math.Abs(p.Overcuts[0].LengthMM-tt.overMM) > 1e-9 {
				t.Fatalf("overcuts %+v, want one of %g mm", p.Overcuts, tt.overMM)
			}
			if len(p.Contours) != 1 || !p.Contours[0].Closed || len(p.Contours[0].Segments) != 4 || len(p.Overlaps) != 0 || len(p.Openings) != 0 {
				t.Errorf("contours %+v overlaps %+v openings %+v, want one closed square", p.Contours, p.Overlaps, p.Openings)
			}
		})
	}
}

func TestCornerLoops(t *testing.T) {
	// A closed polyline through pts; loop adds a 0.2 mm triangle loop that leaves and returns to the point at that index before the path goes on.
	path := func(loopAt int, size float64, pts ...Point) Contour {
		var segs []Segment
		for i := range pts {
			a, b := pts[i], pts[(i+1)%len(pts)]
			if i == loopAt {
				segs = append(segs,
					Segment{Start: a, End: Point{a.X + size, a.Y}, EntityID: "p"},
					Segment{Start: Point{a.X + size, a.Y}, End: Point{a.X, a.Y - size}, EntityID: "p"},
					Segment{Start: Point{a.X, a.Y - size}, End: a, EntityID: "p"})
			}
			segs = append(segs, Segment{Start: a, End: b, EntityID: "p"})
		}
		return Contour{Segments: segs, Closed: true}
	}
	square := []Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	tiny := []Point{{0, 0}, {0.2, 0}, {0.2, 0.2}, {0, 0.2}}
	tests := []struct {
		name  string
		c     Contour
		loops int
	}{
		{"loop at a corner", path(1, 0.2, square...), 1},
		{"loop at the start", path(0, 0.2, square...), 1},
		{"too big to be a relief", path(1, 2, square...), 0},
		{"no loop", path(-1, 0, square...), 0},
		{"a tiny closed path is not a loop of itself", path(-1, 0, tiny...), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart([]Contour{tt.c}, nil)
			got := 0
			if len(p.CornerLoops) > 0 {
				got = p.CornerLoops[0].Count
			}
			if got != tt.loops {
				t.Fatalf("loops %d, want %d", got, tt.loops)
			}
			if tt.loops > 0 && (len(p.Contours) != 1 || len(p.Contours[0].Segments) != 4 || len(p.Contacts) != 0) {
				t.Errorf("contours %+v, contacts %+v; want the plain square", p.Contours, p.Contacts)
			}
		})
	}
}

func TestFacetedCurves(t *testing.T) {
	// arc is n straight segments along a circle of radius r from angle 0 by turn degrees in total.
	arc := func(r, turn float64, n int) [][3]float64 {
		var out [][3]float64
		for i := 0; i <= n; i++ {
			a := turn * math.Pi / 180 * float64(i) / float64(n)
			out = append(out, [3]float64{r * math.Cos(a), r * math.Sin(a), 0})
		}
		return out
	}
	open := func(id string, p [][3]float64) Contour {
		c := closedPoly(id, p...)
		c.Closed = false
		c.Segments = c.Segments[:len(c.Segments)-1]
		return c
	}
	circle := closedPoly("c", arc(5, 360, 36)[:36]...)
	tests := []struct {
		name string
		c    Contour
		want []int // segments in each faceted curve
	}{
		{"36-sided circle", circle, []int{36}},
		{"quarter circle in 10 segments", open("q", arc(5, 90, 10)), []int{10}},
		{"7 segments are too few", open("q", arc(5, 90, 7)), nil},
		{"turns over 15° are corners", open("q", arc(5, 180, 10)), nil},
		{"straight line in pieces", open("q", arc(1000, 0.5, 10)), nil},
		{"zigzag", open("z", pts(0, 0, 1, 0.05, 2, 0, 3, 0.05, 4, 0, 5, 0.05, 6, 0, 7, 0.05, 8, 0, 9, 0.05)), nil},
		{"square", closedPoly("s", pts(0, 0, 10, 0, 10, 10, 0, 10)...), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			for _, f := range facetedCurves(tt.c) {
				got = append(got, f.Segments)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("faceted curves %v, want %v", got, tt.want)
			}
		})
	}
	// A 36-sided cut-out is read as the circle through its corners, and stays reported as a faceted curve.
	p := NewPart([]Contour{closedPoly("plate", pts(-20, -20, 20, -20, 20, 20, -20, 20)...), circle}, nil)
	holes := p.Holes()
	if len(holes) != 1 || math.Abs(holes[0].Diameter()-10) > 1e-9 || len(p.Contours) != 1 || len(p.FacetedCurves) != 1 {
		t.Errorf("holes %+v, contours %d, faceted %d; want one 10 mm hole, the plate and one faceted curve", holes, len(p.Contours), len(p.FacetedCurves))
	}
}

func TestSameShape(t *testing.T) {
	// An L bracket with a hole, placed by pl, mirrored or with its hole moved.
	bracket := func(pl Placement, mirror bool, holeX float64) *Part {
		at := func(x, y float64) [3]float64 {
			if mirror {
				x = -x
			}
			q := pl.Apply(Point{x, y})
			return [3]float64{q.X, q.Y, 0}
		}
		var vs [][3]float64
		for _, p := range [][2]float64{{0, 0}, {60, 0}, {60, 15}, {15, 15}, {15, 40}, {0, 40}} {
			vs = append(vs, at(p[0], p[1]))
		}
		h := at(holeX, 7.5)
		return NewPart([]Contour{closedPoly("b", vs...)}, []Circle{circ("h", h[0], h[1], 2)})
	}
	first := bracket(Placement{}, false, 7.5)
	tests := []struct {
		name string
		p    *Part
		want *Placement
	}{
		{"moved", bracket(Placement{Offset: Point{100, 5}}, false, 7.5), &Placement{Offset: Point{100, 5}}},
		{"turned 30°", bracket(Placement{RotationDeg: 30, Offset: Point{-20, 40}}, false, 7.5), &Placement{RotationDeg: 30, Offset: Point{-20, 40}}},
		{"turned 270°", bracket(Placement{RotationDeg: -90}, false, 7.5), &Placement{RotationDeg: 270}},
		{"mirrored", bracket(Placement{}, true, 7.5), nil},
		{"hole moved", bracket(Placement{}, false, 8), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl, ok := SameShape(first, tt.p)
			if ok != (tt.want != nil) {
				t.Fatalf("same = %v, want %v", ok, tt.want != nil)
			}
			if ok && (math.Abs(pl.RotationDeg-tt.want.RotationDeg) > 1e-9 || !pl.Offset.Near(tt.want.Offset)) {
				t.Errorf("placement %+v, want %+v", pl, *tt.want)
			}
		})
	}
}

func TestStubs(t *testing.T) {
	square := closedPoly("sq", pts(0, 0, 10, 0, 10, 10, 0, 10)...)
	tests := []struct {
		name  string
		paths []Contour
		drop  []string // entities dropped as duplicates
	}{
		{"line on a polyline's edge", []Contour{square, line(2, 0, 6, 0, "stub")}, []string{"stub"}},
		{"line 0.0005 mm off the edge", []Contour{square, line(2, 0.0005, 6, 0.0005, "stub")}, []string{"stub"}},
		{"line partly off the edge", []Contour{square, line(8, 0, 12, 0, "long")}, nil},
		// The outline is four lines; a stray longer line covers the bottom one, which still closes the outline and stays.
		{"closing piece covered by a stray line", []Contour{
			line(0, 0, 10, 0, "b"), line(10, 0, 10, 10, "r"), line(10, 10, 0, 10, "t"), line(0, 10, 0, 0, "l"),
			line(-5, 0, 15, 0, "stray"),
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart(tt.paths, nil)
			var got []string
			for _, d := range p.Duplicates {
				got = append(got, d.Copies...)
			}
			if !slices.Equal(got, tt.drop) {
				t.Errorf("dropped %v, want %v", got, tt.drop)
			}
		})
	}
}

func TestFacetedClearance(t *testing.T) {
	// A plate whose corner is rounded (radius 10 about (10, 10)) with 18 facets, and a hole of radius 1 at (7, 7): 10 - 1 - √18 from the arc.
	var pts3 [][3]float64
	pts3 = append(pts3, [3]float64{40, 0, 0}, [3]float64{40, 40, 0}, [3]float64{0, 40, 0})
	for i := 0; i <= 18; i++ {
		a := math.Pi + math.Pi/2*float64(i)/18
		pts3 = append(pts3, [3]float64{10 + 10*math.Cos(a), 10 + 10*math.Sin(a), 0})
	}
	p := NewPart([]Contour{closedPoly("plate", pts3...)}, []Circle{circ("h", 7, 7, 1)})
	var hole int
	for i, l := range p.Loops {
		if l.Circle >= 0 {
			hole = i
		}
	}
	c, ok := p.LoopClearance(hole, p.Loops[hole].Parent, 10)
	want := 10 - 1 - math.Sqrt(18)
	if !ok || math.Abs(c.DistanceMM-want) > 1e-3 {
		t.Errorf("clearance %v, want %.4f (to the arc, not its facets)", c.DistanceMM, want)
	}
}

func TestSharedEdges(t *testing.T) {
	path := func(id string, xy ...float64) Contour {
		var c Contour
		for i := 0; i+3 < len(xy); i += 2 {
			c.Segments = append(c.Segments, Segment{Start: Point{xy[i], xy[i+1]}, End: Point{xy[i+2], xy[i+3]}, EntityID: id})
		}
		return c
	}
	square := closedPoly("sq", pts(0, 0, 10, 0, 10, 10, 0, 10)...)
	tests := []struct {
		name          string
		paths         []Contour
		closed, lines int // outlines and common lines expected
		open          int
	}{
		{"panel added on a square's edge", []Contour{square, path("u", 10, 2, 20, 2, 20, 8, 10, 8)}, 2, 1, 0},
		{"panels added at a square's corners", []Contour{square, path("u", 10, 0, 20, 0, 20, 10, 10, 10), path("v", 20, 0, 30, 0, 30, 10, 20, 10)}, 3, 2, 0},
		// Once the outer edges close into one outline, lines across it are read as lines drawn across a part, not as dividers: the two cannot be told apart.
		{"closed strip with lines across", []Contour{path("a", 10, 0, 0, 0, 0, 10, 10, 10), path("b", 10, 0, 30, 0, 30, 10, 10, 10), path("c", 10, 0, 10, 10), path("d", 20, 0, 20, 10)}, 1, 0, 2},
		// A line drawn across a closed part divides nothing.
		{"line across a closed part", []Contour{square, path("bend", 5, 0, 5, 10)}, 1, 0, 1},
		{"stray line off a corner", []Contour{square, path("stray", 10, 10, 15, 15)}, 1, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPart(tt.paths, nil)
			closed, open := 0, 0
			for _, c := range p.Contours {
				if c.Closed {
					closed++
				} else {
					open++
				}
			}
			if closed != tt.closed || open != tt.open || len(p.CommonLines) != tt.lines {
				t.Errorf("closed %d, open %d, common lines %d; want %d, %d, %d", closed, open, len(p.CommonLines), tt.closed, tt.open, tt.lines)
			}
			if len(p.Contacts) != 0 && tt.lines > 0 {
				t.Errorf("contacts along common lines: %+v", p.Contacts)
			}
		})
	}
}
