package geometry

import "math"

// primitive is a line, arc or full circle in a form convenient for exact distance computation. Arcs sweep from start by sweep radians (positive = counter-clockwise); full circles have sweep 2π.
type primitive struct {
	line         bool
	a, b         Point // line endpoints, or arc endpoints
	c            Point // arc/circle center
	r            float64
	start, sweep float64
	full         bool // full circle: no endpoints
}

func segmentPrimitive(s Segment) primitive {
	if !s.IsArc() {
		return primitive{line: true, a: s.Start, b: s.End}
	}
	c := s.Center()
	return primitive{
		a: s.Start, b: s.End, c: c, r: s.Radius(),
		start: math.Atan2(s.Start.Y-c.Y, s.Start.X-c.X), sweep: s.Sweep(),
	}
}

func circlePrimitive(c Circle) primitive {
	return primitive{c: c.Center, r: c.Radius, sweep: 2 * math.Pi, full: true}
}

// contains reports whether the direction angle a lies on the arc.
func (p primitive) contains(a float64) bool {
	return p.full || angleWithin(a, p.start, p.sweep)
}

// endpoints returns the primitive's endpoints (none for a full circle).
func (p primitive) endpoints() []Point {
	if p.full {
		return nil
	}
	return []Point{p.a, p.b}
}

// pointAt returns the point on the circle at angle a.
func (p primitive) pointAt(a float64) Point {
	return Point{p.c.X + p.r*math.Cos(a), p.c.Y + p.r*math.Sin(a)}
}

// pointDist returns the distance from q to the primitive and the closest point.
func (p primitive) pointDist(q Point) (float64, Point) {
	if p.line {
		return pointSegment(q, p.a, p.b)
	}
	d := q.Dist(p.c)
	if d > 0 {
		a := math.Atan2(q.Y-p.c.Y, q.X-p.c.X)
		if p.contains(a) {
			on := p.pointAt(a)
			return math.Abs(d - p.r), on
		}
	} else if p.full {
		return p.r, p.pointAt(0)
	}
	// Closest point is an endpoint.
	da, db := q.Dist(p.a), q.Dist(p.b)
	if da <= db {
		return da, p.a
	}
	return db, p.b
}

func pointSegment(q, a, b Point) (float64, Point) {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := dx*dx + dy*dy
	t := 0.0
	if l2 > 0 {
		t = math.Max(0, math.Min(1, ((q.X-a.X)*dx+(q.Y-a.Y)*dy)/l2))
	}
	f := Point{a.X + t*dx, a.Y + t*dy}
	return q.Dist(f), f
}

// primitiveDistance returns the exact shortest distance between two primitives and a closest point on each. Intersecting primitives have distance 0.
func primitiveDistance(p, q primitive) (d float64, pp, qp Point) {
	if pt, ok := intersection(p, q); ok {
		return 0, pt, pt
	}
	d = math.Inf(1)
	try := func(dist float64, a, b Point) {
		if dist < d {
			d, pp, qp = dist, a, b
		}
	}
	// Without an intersection, the minimum is attained at an endpoint of one primitive or at an interior critical point.
	if !p.full {
		for _, e := range [2]Point{p.a, p.b} {
			dist, on := q.pointDist(e)
			try(dist, e, on)
		}
	}
	if !q.full {
		for _, e := range [2]Point{q.a, q.b} {
			dist, on := p.pointDist(e)
			try(dist, on, e)
		}
	}
	var buf [2]Point
	for _, cand := range appendCriticalPoints(buf[:0], p, q) {
		dist, on := q.pointDist(cand)
		try(dist, cand, on)
	}
	for _, cand := range appendCriticalPoints(buf[:0], q, p) {
		dist, on := p.pointDist(cand)
		try(dist, on, cand)
	}
	return d, pp, qp
}

// criticalPoints returns interior points of curved primitive p that can be closest to q: the points of p's circle on the line through p's center and the nearest point of q's supporting geometry.
func appendCriticalPoints(out []Point, p, q primitive) []Point {
	if p.line {
		return out
	}
	var t Point
	if q.line {
		// Foot of the perpendicular from p's center onto q's line.
		_, t = pointSegment(p.c, q.a, q.b)
	} else {
		t = q.c
	}
	if t.Dist(p.c) == 0 {
		// Every direction is equivalent; endpoints cover arcs, so only a full circle needs a representative point.
		if p.full {
			out = append(out, p.pointAt(0))
		}
		return out
	}
	a := math.Atan2(t.Y-p.c.Y, t.X-p.c.X)
	for _, ang := range [2]float64{a, a + math.Pi} {
		if p.contains(ang) {
			out = append(out, p.pointAt(ang))
		}
	}
	return out
}

// intersection returns a point where two primitives meet, if any.
func intersection(p, q primitive) (Point, bool) {
	switch {
	case p.line && q.line:
		return segmentIntersection(p.a, p.b, q.a, q.b)
	case p.line:
		return lineCircleIntersection(p, q)
	case q.line:
		return lineCircleIntersection(q, p)
	default:
		return circleCircleIntersection(p, q)
	}
}

func cross(ax, ay, bx, by float64) float64 { return ax*by - ay*bx }

func segmentIntersection(a, b, c, d Point) (Point, bool) {
	rx, ry := b.X-a.X, b.Y-a.Y
	sx, sy := d.X-c.X, d.Y-c.Y
	den := cross(rx, ry, sx, sy)
	qpx, qpy := c.X-a.X, c.Y-a.Y
	if den == 0 {
		// Parallel: report overlap of collinear segments via an endpoint.
		if cross(qpx, qpy, rx, ry) != 0 {
			return Point{}, false
		}
		for _, e := range []Point{c, d} {
			if dist, _ := pointSegment(e, a, b); dist == 0 {
				return e, true
			}
		}
		for _, e := range []Point{a, b} {
			if dist, _ := pointSegment(e, c, d); dist == 0 {
				return e, true
			}
		}
		return Point{}, false
	}
	t := cross(qpx, qpy, sx, sy) / den
	u := cross(qpx, qpy, rx, ry) / den
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return Point{}, false
	}
	return Point{a.X + t*rx, a.Y + t*ry}, true
}

func lineCircleIntersection(l, c primitive) (Point, bool) {
	dx, dy := l.b.X-l.a.X, l.b.Y-l.a.Y
	fx, fy := l.a.X-c.c.X, l.a.Y-c.c.Y
	A := dx*dx + dy*dy
	B := 2 * (fx*dx + fy*dy)
	C := fx*fx + fy*fy - c.r*c.r
	disc := B*B - 4*A*C
	if A == 0 || disc < 0 {
		return Point{}, false
	}
	sq := math.Sqrt(disc)
	for _, t := range []float64{(-B - sq) / (2 * A), (-B + sq) / (2 * A)} {
		if t < 0 || t > 1 {
			continue
		}
		pt := Point{l.a.X + t*dx, l.a.Y + t*dy}
		if c.contains(math.Atan2(pt.Y-c.c.Y, pt.X-c.c.X)) {
			return pt, true
		}
	}
	return Point{}, false
}

func circleCircleIntersection(p, q primitive) (Point, bool) {
	d := p.c.Dist(q.c)
	if d == 0 {
		// Concentric: they meet only if radii are equal and the arcs share an angle, which an endpoint test detects.
		if p.r != q.r {
			return Point{}, false
		}
		for _, e := range p.endpoints() {
			if q.contains(math.Atan2(e.Y-q.c.Y, e.X-q.c.X)) {
				return e, true
			}
		}
		for _, e := range q.endpoints() {
			if p.contains(math.Atan2(e.Y-p.c.Y, e.X-p.c.X)) {
				return e, true
			}
		}
		if p.full && q.full {
			return p.pointAt(0), true
		}
		return Point{}, false
	}
	if d > p.r+q.r || d < math.Abs(p.r-q.r) {
		return Point{}, false
	}
	a := (p.r*p.r - q.r*q.r + d*d) / (2 * d)
	h := math.Sqrt(math.Max(0, p.r*p.r-a*a))
	ux, uy := (q.c.X-p.c.X)/d, (q.c.Y-p.c.Y)/d
	mx, my := p.c.X+a*ux, p.c.Y+a*uy
	for _, s := range []float64{1, -1} {
		pt := Point{mx - s*h*uy, my + s*h*ux}
		if p.contains(math.Atan2(pt.Y-p.c.Y, pt.X-p.c.X)) && q.contains(math.Atan2(pt.Y-q.c.Y, pt.X-q.c.X)) {
			return pt, true
		}
	}
	return Point{}, false
}
