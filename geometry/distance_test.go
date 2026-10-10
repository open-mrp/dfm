package geometry

import (
	"math"
	"math/rand/v2"
	"testing"
)

func lineP(x0, y0, x1, y1 float64) primitive {
	return segmentPrimitive(Segment{Start: Point{x0, y0}, End: Point{x1, y1}})
}

// arcP is a CCW arc about (cx, cy) from start to end degrees.
func arcP(cx, cy, r, start, end float64) primitive {
	s, e := start*math.Pi/180, end*math.Pi/180
	sweep := math.Mod(e-s+4*math.Pi, 2*math.Pi)
	return segmentPrimitive(Segment{
		Start: Point{cx + r*math.Cos(s), cy + r*math.Sin(s)},
		End:   Point{cx + r*math.Cos(e), cy + r*math.Sin(e)},
		Bulge: math.Tan(sweep / 4),
	})
}

func circleP(cx, cy, r float64) primitive {
	return circlePrimitive(Circle{Center: Point{cx, cy}, Radius: r})
}

func TestPrimitiveDistance(t *testing.T) {
	tests := []struct {
		name string
		p, q primitive
		want float64
	}{
		{"parallel lines", lineP(0, 0, 10, 0), lineP(0, 2, 10, 2), 2},
		{"crossing lines", lineP(0, 0, 10, 10), lineP(0, 10, 10, 0), 0},
		{"T junction", lineP(0, 0, 10, 0), lineP(5, 0, 5, 5), 0},
		{"collinear gap", lineP(0, 0, 10, 0), lineP(12, 0, 20, 0), 2},
		{"collinear overlap", lineP(0, 0, 10, 0), lineP(8, 0, 20, 0), 0},
		{"skew lines endpoint to interior", lineP(0, 0, 10, 0), lineP(5, 3, 8, 9), 3},
		{"line to circle", lineP(-10, 7, 10, 7), circleP(0, 0, 5), 2},
		{"line tangent to circle", lineP(-10, 5, 10, 5), circleP(0, 0, 5), 0},
		{"line through circle", lineP(-10, 0, 10, 0), circleP(0, 0, 5), 0},
		{"line inside circle", lineP(-1, 0, 1, 0), circleP(0, 0, 5), 4},
		{"circles apart", circleP(0, 0, 5), circleP(12, 0, 5), 2},
		{"circles tangent", circleP(0, 0, 5), circleP(10, 0, 5), 0},
		{"circle inside circle", circleP(0, 0, 10), circleP(2, 0, 3), 5},
		{"concentric circles", circleP(0, 0, 10), circleP(0, 0, 7), 3},
		// The arc is the upper half; its circle would cross the line y=-2 but the arc itself stays above, so the closest points are the arc endpoints (±5, 0).
		{"arc misses line its circle hits", arcP(0, 0, 5, 0, 180), lineP(-10, -2, 10, -2), 2},
		{"arc interior to line", arcP(0, 0, 5, 0, 180), lineP(-10, 7, 10, 7), 2},
		{"arc to arc across gap", arcP(0, 0, 5, -90, 90), arcP(12, 0, 5, 90, 270), 2},
		{"arcs whose circles cross only off-arc", arcP(0, 0, 5, 90, 270), arcP(6, 0, 5, -90, 90), 6},
		{"arc to point-like far circle", arcP(0, 0, 5, 0, 90), circleP(20, 0, 1), 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, swap := range []bool{false, true} {
				p, q := tt.p, tt.q
				if swap {
					p, q = q, p
				}
				d, pp, qp := primitiveDistance(p, q)
				if math.Abs(d-tt.want) > 1e-9 {
					t.Fatalf("swap=%v distance = %v, want %v", swap, d, tt.want)
				}
				// The reported points must realize the distance and lie on their primitives.
				if math.Abs(pp.Dist(qp)-d) > 1e-9 {
					t.Errorf("points %v %v are %v apart, distance %v", pp, qp, pp.Dist(qp), d)
				}
				if dp, _ := p.pointDist(pp); dp > 1e-9 {
					t.Errorf("closest point %v is %v off p", pp, dp)
				}
				if dq, _ := q.pointDist(qp); dq > 1e-9 {
					t.Errorf("closest point %v is %v off q", qp, dq)
				}
			}
		})
	}
}

// samples returns n+1 points along the primitive.
func (p primitive) samples(n int) []Point {
	var out []Point
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		if p.line {
			out = append(out, Point{p.a.X + t*(p.b.X-p.a.X), p.a.Y + t*(p.b.Y-p.a.Y)})
		} else {
			out = append(out, p.pointAt(p.start+t*p.sweep))
		}
	}
	return out
}

// TestPrimitiveDistanceAgainstSampling compares the exact distance with a brute-force estimate over random primitives. The exact value can never exceed the sampled one, and must be close to it.
func TestPrimitiveDistanceAgainstSampling(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	random := func() primitive {
		x, y := rng.Float64()*20, rng.Float64()*20
		switch rng.IntN(3) {
		case 0:
			return lineP(x, y, rng.Float64()*20, rng.Float64()*20)
		case 1:
			s := rng.Float64() * 360
			return arcP(x, y, 0.5+rng.Float64()*8, s, s+10+rng.Float64()*330)
		default:
			return circleP(x, y, 0.5+rng.Float64()*8)
		}
	}
	const n = 300
	cases := 1500
	if testing.Short() {
		cases = 200
	}
	for i := range cases {
		p, q := random(), random()
		exact, _, _ := primitiveDistance(p, q)
		sampled := math.Inf(1)
		for _, a := range p.samples(n) {
			for _, b := range q.samples(n) {
				sampled = math.Min(sampled, a.Dist(b))
			}
		}
		// Sampling resolution: adjacent samples are at most ~0.17 mm apart.
		if exact > sampled+1e-9 || sampled-exact > 0.1 {
			t.Fatalf("case %d: exact %v, sampled %v\np=%+v\nq=%+v", i, exact, sampled, p, q)
		}
	}
}
