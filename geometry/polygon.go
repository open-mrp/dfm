package geometry

import (
	"math"
	"sort"
)

// loopPolygon returns the loop's boundary as a polygon, curves sampled at most 10° apart.
func loopPolygon(l Loop) []Point {
	var out []Point
	for _, pr := range l.prims {
		if pr.full {
			for k := 0; k < 36; k++ {
				a := float64(k) * math.Pi / 18
				out = append(out, Point{pr.c.X + float64(pr.r*math.Cos(a)), pr.c.Y + float64(pr.r*math.Sin(a))})
			}
			continue
		}
		out = pr.appendPointsBetween(out, pr.a, pr.b)
	}
	return out
}

// LoopPolygon returns loop i as a polygon (curves as chords at most 10° apart).
func (p *Part) LoopPolygon(i int) []Point { return loopPolygon(p.Loops[i]) }

// clippedArea is the area of polygon poly inside window w (Sutherland- Hodgman clipping, which suits any simple polygon against a convex window).
func clippedArea(poly []Point, w Bounds) float64 {
	inside := []func(Point) bool{
		func(q Point) bool { return q.X >= w.MinX },
		func(q Point) bool { return q.X <= w.MaxX },
		func(q Point) bool { return q.Y >= w.MinY },
		func(q Point) bool { return q.Y <= w.MaxY },
	}
	cut := []func(a, b Point) Point{
		func(a, b Point) Point { return lerpX(a, b, w.MinX) },
		func(a, b Point) Point { return lerpX(a, b, w.MaxX) },
		func(a, b Point) Point { return lerpY(a, b, w.MinY) },
		func(a, b Point) Point { return lerpY(a, b, w.MaxY) },
	}
	out := poly
	for e := range inside {
		in := out
		out = nil
		for k, q := range in {
			prev := in[(k+len(in)-1)%len(in)]
			switch qi, pi := inside[e](q), inside[e](prev); {
			case qi && pi:
				out = append(out, q)
			case qi:
				out = append(out, cut[e](prev, q), q)
			case pi:
				out = append(out, cut[e](prev, q))
			}
		}
		if len(out) == 0 {
			return 0
		}
	}
	area := 0.0
	for k, q := range out {
		r := out[(k+1)%len(out)]
		area += float64(q.X*r.Y) - float64(r.X*q.Y)
	}
	return math.Abs(area) / 2
}

func lerpX(a, b Point, x float64) Point {
	t := (x - a.X) / (b.X - a.X)
	return Point{x, a.Y + float64(t*(b.Y-a.Y))}
}

func lerpY(a, b Point, y float64) Point {
	t := (y - a.Y) / (b.Y - a.Y)
	return Point{a.X + float64(t*(b.X-a.X)), y}
}

// convexHull returns the hull of points in counter-clockwise order.
func convexHull(points []Point) []Point {
	pts := append([]Point(nil), points...)
	sort.Slice(pts, func(i, j int) bool {
		if pts[i].X != pts[j].X {
			return pts[i].X < pts[j].X
		}
		return pts[i].Y < pts[j].Y
	})
	if len(pts) < 3 {
		return pts
	}
	turn := func(o, a, b Point) float64 { return cross(a.X-o.X, a.Y-o.Y, b.X-o.X, b.Y-o.Y) }
	var hull []Point
	for pass := 0; pass < 2; pass++ {
		start := len(hull)
		for _, p := range pts {
			for len(hull) >= start+2 && turn(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
				hull = hull[:len(hull)-1]
			}
			hull = append(hull, p)
		}
		hull = hull[:len(hull)-1]
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
	}
	return hull
}

// strictlyInsideConvex reports whether q lies inside the counter-clockwise convex polygon, by more than a nanometer from every edge.
func strictlyInsideConvex(poly []Point, q Point) bool {
	for i, a := range poly {
		b := poly[(i+1)%len(poly)]
		edge := math.Hypot(b.X-a.X, b.Y-a.Y)
		if (b.X-a.X)*(q.Y-a.Y)-(b.Y-a.Y)*(q.X-a.X) <= 1e-6*edge {
			return false
		}
	}
	return true
}

// polygonWidth is a polygon's minimum Feret diameter and its extent at right angles to that, from the edges of its convex hull (rotating calipers).
func polygonWidth(pts []Point) (width, length float64) {
	hull := convexHull(pts)
	width = math.Inf(1)
	for k := range hull {
		a, b := hull[k], hull[(k+1)%len(hull)]
		ex, ey := b.X-a.X, b.Y-a.Y
		l := math.Hypot(ex, ey)
		if l == 0 {
			continue
		}
		ux, uy := ex/l, ey/l
		lo, hi, near, far := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, q := range hull {
			along := float64((q.X-a.X)*ux) + float64((q.Y-a.Y)*uy)
			across := float64((q.X-a.X)*-uy) + float64((q.Y-a.Y)*ux)
			lo, hi = math.Min(lo, along), math.Max(hi, along)
			near, far = math.Min(near, across), math.Max(far, across)
		}
		if w := far - near; w < width {
			width, length = w, hi-lo
		}
	}
	return width, length
}

// pointSegmentDist is the distance from q to segment a-b.
func pointSegmentDist(q, a, b Point) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := float64(dx*dx) + float64(dy*dy)
	if l2 == 0 {
		return q.Dist(a)
	}
	t := math.Max(0, math.Min(1, (float64((q.X-a.X)*dx)+float64((q.Y-a.Y)*dy))/l2))
	return q.Dist(Point{a.X + float64(t*dx), a.Y + float64(t*dy)})
}

// linePointDist is the distance from q to the line through a and b.
func linePointDist(q, a, b Point) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return q.Dist(a)
	}
	return math.Abs(float64((q.X-a.X)*dy)-float64((q.Y-a.Y)*dx)) / l
}
