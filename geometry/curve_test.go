package geometry

import (
	"errors"
	"math"
	"math/rand/v2"
	"testing"
)

func ellipse(a, b float64) Curve {
	return func(t float64) (Point, Point) {
		return Point{a * math.Cos(t), b * math.Sin(t)}, Point{-a * math.Sin(t), b * math.Cos(t)}
	}
}

func TestFitArcs(t *testing.T) {
	tests := []struct {
		name   string
		a, b   float64
		t0, t1 float64
	}{
		{"circle", 10, 10, 0, 2 * math.Pi},
		{"ellipse", 20, 10, 0, 2 * math.Pi},
		{"flat ellipse", 50, 1, 0, 2 * math.Pi},
		{"elliptical arc", 20, 10, 0.3, 2.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segs, err := FitArcs(ellipse(tt.a, tt.b), tt.t0, tt.t1, "e")
			if err != nil || len(segs) == 0 {
				t.Fatalf("no segments: %v", err)
			}
			p0, _ := ellipse(tt.a, tt.b)(tt.t0)
			p1, _ := ellipse(tt.a, tt.b)(tt.t1)
			if !segs[0].Start.Near(p0) || !segs[len(segs)-1].End.Near(p1) {
				t.Errorf("ends %v %v, want %v %v", segs[0].Start, segs[len(segs)-1].End, p0, p1)
			}
			for i, s := range segs {
				if i > 0 && s.Start != segs[i-1].End {
					t.Fatalf("segment %d does not start where %d ends", i, i-1)
				}
				// Every point of the fit is close to the ellipse: its implicit value scales with distance, so bound it by the tolerance.
				pr := segmentPrimitive(s)
				for k := 0; k <= 8; k++ {
					q := s.Start
					if pr.line {
						f := float64(k) / 8
						q = Point{s.Start.X + f*(s.End.X-s.Start.X), s.Start.Y + f*(s.End.Y-s.Start.Y)}
					} else {
						q = pr.pointAt(pr.start + pr.sweep*float64(k)/8)
					}
					if d := ellipseDistance(tt.a, tt.b, q); d > 1.01*CurveTolerance {
						t.Fatalf("segment %d strays %g mm from the curve", i, d)
					}
				}
				if i > 0 {
					t1, t2 := endTangent(segmentPrimitive(segs[i-1])), startTangent(pr)
					if turn := math.Atan2(cross(t1.X, t1.Y, t2.X, t2.Y), dot(t1, t2)); math.Abs(turn) > 1e-6 {
						t.Fatalf("corner of %g rad between segments %d and %d", turn, i-1, i)
					}
				}
			}
			t.Logf("%d arcs", len(segs))
		})
	}
}

// ellipseDistance finds the distance from q to the ellipse by dense search.
func ellipseDistance(a, b float64, q Point) float64 {
	best, bt := math.Inf(1), 0.0
	for i := 0; i < 3600; i++ {
		th := 2 * math.Pi * float64(i) / 3600
		if d := q.Dist(Point{a * math.Cos(th), b * math.Sin(th)}); d < best {
			best, bt = d, th
		}
	}
	lo, hi := bt-2*math.Pi/3600, bt+2*math.Pi/3600
	for range 60 {
		m1, m2 := lo+(hi-lo)/3, hi-(hi-lo)/3
		if q.Dist(Point{a * math.Cos(m1), b * math.Sin(m1)}) < q.Dist(Point{a * math.Cos(m2), b * math.Sin(m2)}) {
			hi = m2
		} else {
			lo = m1
		}
	}
	return q.Dist(Point{a * math.Cos(lo), b * math.Sin(lo)})
}

func TestFitArcsStraightLine(t *testing.T) {
	line := func(t float64) (Point, Point) { return Point{t, 2 * t}, Point{1, 2} }
	segs, _ := FitArcs(line, 0, 10, "l")
	for _, s := range segs {
		if math.Abs(s.Bulge) > 1e-9 {
			t.Fatalf("straight curve fitted with bulge %g", s.Bulge)
		}
	}
}

func TestNURBSCircle(t *testing.T) {
	// The standard rational quadratic circle of radius 10: nine control points on a square, corner weights √2/2.
	h := math.Sqrt2 / 2
	n := NURBS{
		Degree:  2,
		Knots:   []float64{0, 0, 0, 0.25, 0.25, 0.5, 0.5, 0.75, 0.75, 1, 1, 1},
		Points:  []Point{{10, 0}, {10, 10}, {0, 10}, {-10, 10}, {-10, 0}, {-10, -10}, {0, -10}, {10, -10}, {10, 0}},
		Weights: []float64{1, h, 1, h, 1, h, 1, h, 1},
	}
	if err := n.Validate(); err != nil {
		t.Fatal(err)
	}
	segs, _ := n.Segments("c")
	if len(segs) != 8 {
		t.Errorf("%d segments, want 8 (one exact arc pair per quarter)", len(segs))
	}
	for _, s := range segs {
		if r := s.Radius(); math.Abs(r-10) > 1e-9 {
			t.Fatalf("radius %g, want 10", r)
		}
	}
	if c, ok := asCircle(Contour{Segments: segs, Closed: true}); !ok || math.Abs(c.Radius-10) > 1e-9 {
		t.Errorf("not recognized as a circle: %+v %v", c, ok)
	}
}

func TestNURBSPolyline(t *testing.T) {
	n := NURBS{Degree: 1, Knots: []float64{0, 0, 1, 2, 3, 3}, Points: []Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}}}
	segs, _ := n.Segments("p")
	want := []Segment{{Point{0, 0}, Point{10, 0}, 0, "p"}, {Point{10, 0}, Point{10, 10}, 0, "p"}, {Point{10, 10}, Point{0, 10}, 0, "p"}}
	if len(segs) != len(want) {
		t.Fatalf("segments %+v, want %+v", segs, want)
	}
	for i := range want {
		if !segs[i].Start.Near(want[i].Start) || !segs[i].End.Near(want[i].End) || segs[i].Bulge != 0 {
			t.Errorf("segment %d = %+v, want %+v", i, segs[i], want[i])
		}
	}
}

func TestNURBSValidate(t *testing.T) {
	ok := NURBS{Degree: 1, Knots: []float64{0, 0, 1, 1}, Points: []Point{{0, 0}, {1, 0}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, n := range map[string]NURBS{
		"degree 0":       {Degree: 0, Knots: []float64{0, 1}, Points: []Point{{0, 0}}},
		"degree 31":      {Degree: 31, Knots: make([]float64, 64), Points: make([]Point, 32)},
		"knot count":     {Degree: 1, Knots: []float64{0, 1, 1}, Points: []Point{{0, 0}, {1, 0}}},
		"knot order":     {Degree: 1, Knots: []float64{0, 1, 0.5, 1}, Points: []Point{{0, 0}, {1, 0}}},
		"zero weight":    {Degree: 1, Knots: []float64{0, 0, 1, 1}, Points: []Point{{0, 0}, {1, 0}}, Weights: []float64{1, 0}},
		"empty range":    {Degree: 1, Knots: []float64{0, 0, 0, 0}, Points: []Point{{0, 0}, {1, 0}}},
		"weights length": {Degree: 1, Knots: []float64{0, 0, 1, 1}, Points: []Point{{0, 0}, {1, 0}}, Weights: []float64{1}},
	} {
		if n.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestFitArcsTooComplex(t *testing.T) {
	// A curve wiggling far faster than the fitter may subdivide.
	wild := func(t float64) (Point, Point) {
		return Point{t, math.Sin(1e9 * t)}, Point{1, 1e9 * math.Cos(1e9*t)}
	}
	if _, err := FitArcs(wild, 0, 1, "w"); !errors.Is(err, ErrCurveTooComplex) {
		t.Fatalf("err = %v, want ErrCurveTooComplex", err)
	}
}

// TestMinWidthOfFittedCurves checks the width of many-arc outlines against a dense scan of directions: the scan can only overestimate the minimum, so MinWidth must not exceed it.
func TestMinWidthOfFittedCurves(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range 40 {
		a, b := 5+rng.Float64()*100, 1+rng.Float64()*50
		rot, cx, cy := rng.Float64()*math.Pi, rng.Float64()*100, rng.Float64()*100
		c, s := math.Cos(rot), math.Sin(rot)
		curve := func(t float64) (Point, Point) {
			x, y, dx, dy := a*math.Cos(t), b*math.Sin(t), -a*math.Sin(t), b*math.Cos(t)
			return Point{cx + x*c - y*s, cy + x*s + y*c}, Point{dx*c - dy*s, dx*s + dy*c}
		}
		var segs []Segment
		for q := range 4 {
			piece, err := FitArcs(curve, float64(q)*math.Pi/2, float64(q+1)*math.Pi/2, "e")
			if err != nil {
				t.Fatal(err)
			}
			if len(segs) > 0 {
				piece[0].Start = segs[len(segs)-1].End
			}
			segs = append(segs, piece...)
		}
		segs[len(segs)-1].End = segs[0].Start
		p := NewPart([]Contour{{Segments: segs, Closed: true}}, nil)
		got := p.MinWidth(0)
		prims := p.Loops[0].prims
		width := func(th float64) float64 {
			u := Point{math.Cos(th), math.Sin(th)}
			return support(prims, u) + support(prims, Point{-u.X, -u.Y})
		}
		scan := math.Inf(1)
		for k := range 7200 {
			scan = math.Min(scan, width(math.Pi*float64(k)/7200))
		}
		if got > scan+1e-9 {
			t.Errorf("shape %d (%d arcs): MinWidth %g exceeds scanned %g", i, len(segs), got, scan)
		}
		if want := 2 * math.Min(a, b); math.Abs(got-want) > 2*CurveTolerance {
			t.Errorf("shape %d: MinWidth %g, want %g within the fit tolerance", i, got, want)
		}
	}
}

// TestFitArcsStraightPiece fits a quarter of a flat ellipse, whose fit has a straight piece: it must still follow the curve. A near-zero bulge once made that piece an arc too large to measure against, and a fit 0.55 mm off passed as within tolerance.
func TestFitArcsStraightPiece(t *testing.T) {
	curve := func(t float64) (Point, Point) {
		return Point{60 + 6*math.Cos(t), 45 + 2*math.Sin(t)}, Point{-6 * math.Sin(t), 2 * math.Cos(t)}
	}
	segs, err := FitArcs(curve, 0, math.Pi/2, "e")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 200; i++ {
		q, _ := curve(math.Pi / 2 * float64(i) / 200)
		best := math.Inf(1)
		for _, s := range segs {
			d, _ := segmentPrimitive(s).pointDist(q)
			best = math.Min(best, d)
		}
		if best > CurveTolerance+1e-9 {
			t.Fatalf("curve point %v is %.4f mm from the fit", q, best)
		}
	}
	for _, s := range segs {
		if s.IsArc() && s.Radius() > 1e6 {
			t.Errorf("arc of radius %g mm should be a line", s.Radius())
		}
	}
}
