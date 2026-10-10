package geometry

import (
	"math"
)

// MinWidth returns the smallest width of loop i over every direction: the minimum distance between two parallel lines enclosing it (its minimum Feret diameter), exact for lines, arcs and circles.
//
// Width as a function of direction is piecewise sinusoidal; its minimum lies at a breakpoint or at a critical point of one piece, so evaluating the exact width at every such candidate direction finds it.
func (p *Part) MinWidth(i int) float64 {
	w, _ := p.WidthAndLength(i)
	return w
}

// WidthAndLength returns loop i's smallest width (see MinWidth) and its length at right angles to that width: the sides of the bounding rectangle aligned with its narrowest direction, as a part 6 × 40 mm is 6 wide and 40 long whatever way it is drawn. Where several directions give the same width (a circle), the longest such length is used.
func (p *Part) WidthAndLength(i int) (width, length float64) {
	if wl, ok := p.widths[i]; ok {
		return wl[0], wl[1]
	}
	defer func() {
		if p.widths == nil {
			p.widths = map[int][2]float64{}
		}
		p.widths[i] = [2]float64{width, length}
	}()
	prims := hullPrimitives(p.Loops[i].prims, p.Loops[i].primBounds)
	width = math.Inf(1)
	for _, th := range widthCandidates(prims) {
		u := Point{math.Cos(th), math.Sin(th)}
		w := support(prims, u) + support(prims, Point{-u.X, -u.Y})
		v := Point{-u.Y, u.X}
		l := support(prims, v) + support(prims, Point{-v.X, -v.Y})
		switch {
		case w < width-1e-9:
			width, length = w, l
		case w <= width+1e-9:
			width, length = math.Min(width, w), math.Max(length, l)
		}
	}
	return width, length
}

func dot(a, b Point) float64 { return a.X*b.X + a.Y*b.Y }

// support is the largest projection of the primitives onto direction u.
func support(prims []primitive, u Point) float64 {
	best := math.Inf(-1)
	angle := math.Atan2(u.Y, u.X)
	for _, pr := range prims {
		if !pr.full {
			best = math.Max(best, math.Max(dot(pr.a, u), dot(pr.b, u)))
		}
		if !pr.line && pr.contains(angle) {
			best = math.Max(best, dot(pr.c, u)+pr.r)
		}
	}
	return best
}

// widthCandidates lists the directions where the width can be smallest. An arc's circle only bounds the width in directions the arc itself faces, so directions derived from an arc are kept only when it faces them (either way, as width is the same in opposite directions).
func widthCandidates(prims []primitive) []float64 {
	var thetas []float64
	add := func(th float64) { thetas = append(thetas, th) }
	normalOf := func(a, b Point) { add(math.Atan2(b.Y-a.Y, b.X-a.X) + math.Pi/2) }
	// within allows for rounding at the ends of the arc's range.
	within := func(a primitive, th float64) bool {
		start, sweep := a.start, a.sweep
		if sweep < 0 {
			start, sweep = start+sweep, -sweep
		}
		return angleWithin(th, start-1e-9, sweep+2e-9)
	}
	faces := func(a primitive, th float64) bool { return within(a, th) || within(a, th+math.Pi) }
	// opposed: a faces th and b faces th + π, or the other way round.
	opposed := func(a, b primitive, th float64) bool {
		return (within(a, th) && within(b, th+math.Pi)) || (within(a, th+math.Pi) && within(b, th))
	}
	// together: a and b both face th, or both face th + π.
	together := func(a, b primitive, th float64) bool {
		return (within(a, th) && within(b, th)) || (within(a, th+math.Pi) && within(b, th+math.Pi))
	}

	var points []Point
	var arcs []primitive
	for _, pr := range prims {
		points = append(points, pr.endpoints()...)
		if !pr.line {
			arcs = append(arcs, pr)
			add(0) // a lone circle: every direction is equivalent
			for _, e := range pr.endpoints() {
				add(math.Atan2(e.Y-pr.c.Y, e.X-pr.c.X))
			}
		}
	}
	hull := convexHull(points)
	for k := range hull {
		normalOf(hull[k], hull[(k+1)%len(hull)])
	}
	for ai, a := range arcs {
		for _, v := range hull {
			d := v.Dist(a.c)
			base := math.Atan2(v.Y-a.c.Y, v.X-a.c.X)
			if faces(a, base) {
				add(base) // critical point of (c - v)·u + r
			}
			if d > a.r {
				off := math.Acos(a.r / d) // tangent from v to the circle
				for _, th := range []float64{base + off, base - off} {
					if faces(a, th) {
						add(th)
					}
				}
			}
		}
		for _, b := range arcs[ai+1:] {
			d := a.c.Dist(b.c)
			if d == 0 {
				continue
			}
			base := math.Atan2(a.c.Y-b.c.Y, a.c.X-b.c.X)
			if opposed(a, b, base) {
				add(base) // critical point of (c1 - c2)·u + r1 + r2
			}
			if q := (b.r - a.r) / d; math.Abs(q) <= 1 {
				off := math.Acos(q) // outer common tangents
				for _, th := range []float64{base + off, base - off} {
					if together(a, b, th) {
						add(th)
					}
				}
			}
		}
	}
	return thetas
}

// hullPrimitives drops the primitives that cannot touch the convex hull, so cannot change any width: those whose bounding box lies strictly inside a polygon of points sampled on the primitives (which lies inside the true hull). The result is exact, and a long outline with few pieces on its hull is measured in a fraction of the time.
func hullPrimitives(prims []primitive, bounds []Bounds) []primitive {
	if len(prims) < 32 {
		return prims
	}
	var pts []Point
	for _, pr := range prims {
		pts = append(pts, pr.endpoints()...)
		if !pr.line {
			for k := range 8 {
				pts = append(pts, pr.pointAt(pr.start+pr.sweep*float64(k)/8))
			}
		}
	}
	hull := convexHull(pts)
	if len(hull) < 3 {
		return prims
	}
	var out []primitive
	for k, pr := range prims {
		b := bounds[k]
		corners := []Point{{b.MinX, b.MinY}, {b.MaxX, b.MinY}, {b.MaxX, b.MaxY}, {b.MinX, b.MaxY}}
		inside := true
		for _, c := range corners {
			if !strictlyInsideConvex(hull, c) {
				inside = false
				break
			}
		}
		if !inside {
			out = append(out, pr)
		}
	}
	return out
}
