package geometry

import (
	"math"
)

// NeckKind says what a neck is made of.
type NeckKind int

const (
	// NeckMaterial is a narrow strip of material: a thin finger, tab or tongue.
	NeckMaterial NeckKind = iota
	// NeckVoid is a narrow passage of empty space: a tight cut-out or edge notch.
	NeckVoid
)

// Neck is the narrowest place between two facing sides of one loop.
type Neck struct {
	// Loop indexes Part.Loops.
	Loop           int
	Kind           NeckKind
	DistanceMM     float64
	PointA, PointB Point
}

// Midpoint is the center of the neck.
func (n Neck) Midpoint() Point {
	return Point{(n.PointA.X + n.PointB.X) / 2, (n.PointA.Y + n.PointB.Y) / 2}
}

// facingTolerance is how far from a side's normal the line across a neck may lean. Sides meeting at a fillet or chamfer lean 45° or more and are not necks.
const facingTolerance = 30 * math.Pi / 180

// A neck's two points must be joined, the short way round the loop, by at least neckPathRatio times their distance (a semicircular end gives π/2, a fillet or rounded tip less) and at least minNeckPath, below any kerf.
const (
	neckPathRatio = 1.5
	minNeckPath   = 0.05
)

// pathBetween is the length of boundary between point pa on piece i and point pb on piece j (i < j), the shorter way round the loop.
func (l Loop) pathBetween(i int, pa Point, j int, pb Point) float64 {
	total := l.along[len(l.prims)]
	forward := (l.along[j] + l.prims[j].along(pb)) - (l.along[i] + l.prims[i].along(pa))
	return math.Min(forward, total-forward)
}

// A pocket fills less than taperFill of the rectangle its depth and the neck's width make when it narrows to a point: about half for straight sides, less for sides curving in. A passage that keeps its width fills about all of it, and 0.785 for a pocket that is only a round end.
const taperFill = 0.6

// tapers reports whether the boundary beyond the neck from pa (on piece i) to pb (on piece j), the short way round, is a pocket that narrows to a point: at least as deep as the neck is wide, and filling less than taperFill of the rectangle of its depth and the neck's width.
func (l Loop) tapers(i int, pa Point, j int, pb Point, d float64, buf []Point) (bool, []Point) {
	pts := l.shortPath(i, pa, j, pb, buf[:0])
	ux, uy := (pb.X-pa.X)/d, (pb.Y-pa.Y)/d
	depth, area := 0.0, 0.0
	for k, q := range pts {
		depth = math.Max(depth, math.Abs(cross(ux, uy, q.X-pa.X, q.Y-pa.Y)))
		r := pts[(k+1)%len(pts)]
		area += float64(q.X*r.Y) - float64(r.X*q.Y)
	}
	area = math.Abs(area) / 2
	return depth >= d && area < taperFill*d*depth, pts
}

// shortPath returns points along the loop's boundary from pa on piece i to pb on piece j (i < j), the shorter way round, with arcs sampled at most 10° apart.
func (l Loop) shortPath(i int, pa Point, j int, pb Point, pts []Point) []Point {
	n := len(l.prims)
	total := l.along[n]
	forward := (l.along[j] + l.prims[j].along(pb)) - (l.along[i] + l.prims[i].along(pa))
	from, fromPt, to, toPt := i, pa, j, pb
	if forward > total-forward {
		from, fromPt, to, toPt = j, pb, i, pa
	}
	pts = append(pts, fromPt)
	for k := from; ; k = (k + 1) % n {
		pr := l.prims[k]
		start, end := pr.a, pr.b
		if k == from {
			start = fromPt
		}
		if k == to {
			end = toPt
		}
		pts = pr.appendPointsBetween(pts, start, end)
		if k == to {
			break
		}
	}
	return pts
}

// pointsBetween returns points of the piece after from up to and including to, both on it, in its direction: to alone for a line, points at most 10° apart for an arc.
func (pr primitive) appendPointsBetween(out []Point, from, to Point) []Point {
	if pr.line {
		return append(out, to)
	}
	t0, t1 := pr.along(from)/pr.r, pr.along(to)/pr.r
	steps := int(math.Ceil(math.Abs(t1-t0) / (10 * math.Pi / 180)))
	for s := 1; s < steps; s++ {
		t := t0 + (t1-t0)*float64(s)/float64(steps)
		if pr.sweep < 0 {
			t = -t
		}
		out = append(out, Point{pr.c.X + float64(pr.r*math.Cos(pr.start+t)), pr.c.Y + float64(pr.r*math.Sin(pr.start+t))})
	}
	return append(out, to)
}

func (pr primitive) length() float64 {
	if pr.line {
		return pr.a.Dist(pr.b)
	}
	return pr.r * math.Abs(pr.sweep)
}

// along is how far q lies along the piece from its start.
func (pr primitive) along(q Point) float64 {
	if pr.line {
		return pr.a.Dist(q)
	}
	t := math.Atan2(q.Y-pr.c.Y, q.X-pr.c.X) - pr.start
	if pr.sweep < 0 {
		t = -t
	}
	t = math.Mod(t, 2*math.Pi)
	if t < 0 {
		t += 2 * math.Pi
	}
	if sw := math.Abs(pr.sweep); t > sw {
		// Off the arc by rounding: snap to the nearer end.
		if t > (sw+2*math.Pi)/2 {
			t = 0
		} else {
			t = sw
		}
	}
	return pr.r * t
}

// smoothFacingTolerance is how far the line across a neck may lean from the normals at a joint of a faceted curve.
const smoothFacingTolerance = math.Pi / 180

// Necks returns, for every loop and kind, its narrowest neck of at most radius. A neck joins closest points of two non-adjacent pieces of one loop's boundary that face each other across empty space or material, with no other boundary in between. Gaps of EndpointTolerance or less are contacts, not necks.
func (p *Part) Necks(radius float64) []Neck {
	// The narrowest neck of a loop within a larger radius is the same as within a smaller one when it is that close, so one search at the largest radius asked serves every rule.
	if c := p.necks; c != nil && radius <= c.radius {
		out := make([]Neck, 0, len(c.necks))
		for _, n := range c.necks {
			if n.DistanceMM <= radius {
				out = append(out, n)
			}
		}
		return out
	}
	out := p.findNecks(radius)
	p.necks = &neckCache{radius: radius, necks: out}
	return out
}

// neckCache holds the necks found within radius.
type neckCache struct {
	radius float64
	necks  []Neck
}

func (p *Part) findNecks(radius float64) []Neck {
	all := newElementIndex(elementsOf(p.Contours, p.Circles), radius)
	best := map[[2]int]Neck{}
	var pathBuf []Point
	for li, l := range p.Loops {
		n := len(l.prims)
		if n < 3 {
			continue
		}
		for _, pr := range candidatePairs(l.primBounds, radius) {
			i, j := pr[0], pr[1]
			if j-i == 1 || (i == 0 && j == n-1) {
				continue
			}
			d, pa, pb := primitiveDistance(l.prims[i], l.prims[j])
			if a, b, ok := parallelMidpair(l.prims[i], l.prims[j]); ok {
				pa, pb = a, b
			}
			if d > radius || d <= EndpointTolerance {
				continue
			}
			d = math.Max(d, curveDistance(l, i, l, j))
			// Points joined by a short stretch of boundary are not across a strip: a chord across a rounded tip or a fillet, or a jog smaller than any kerf.
			if path := l.pathBetween(i, pa, j, pb); path < minNeckPath || path < neckPathRatio*d {
				continue
			}
			// A passage or strip that narrows to nothing beyond the neck is a taper ending in a point, not a neck: the tip check covers it.
			var taper bool
			if taper, pathBuf = l.tapers(i, pa, j, pb, d, pathBuf); taper {
				continue
			}
			mid := Point{(pa.X + pb.X) / 2, (pa.Y + pb.Y) / 2}
			inward := l.containsPoint(mid)
			dir := Point{(pb.X - pa.X) / d, (pb.Y - pa.Y) / d}
			if !l.facing(i, pa, dir, inward) || !l.facing(j, pb, Point{-dir.X, -dir.Y}, inward) {
				continue
			}
			if crossesAny(all, pa, pb) {
				continue
			}
			kind := NeckVoid
			if inward == (l.Depth%2 == 0) {
				kind = NeckMaterial
			}
			key := [2]int{li, int(kind)}
			if cur, ok := best[key]; !ok || d < cur.DistanceMM {
				best[key] = Neck{Loop: li, Kind: kind, DistanceMM: d, PointA: pa, PointB: pb}
			}
		}
	}
	out := make([]Neck, 0, len(best))
	for li := range p.Loops {
		for _, kind := range []NeckKind{NeckMaterial, NeckVoid} {
			if nk, ok := best[[2]int{li, int(kind)}]; ok {
				out = append(out, nk)
			}
		}
	}
	return out
}

// parallelMidpair measures parallel lines across the middle of their overlap. Every point of the overlap is equally close, and an endpoint pair would put the neck on the boundary itself.
func parallelMidpair(p, q primitive) (Point, Point, bool) {
	if !p.line || !q.line {
		return Point{}, Point{}, false
	}
	l := p.a.Dist(p.b)
	u := Point{(p.b.X - p.a.X) / l, (p.b.Y - p.a.Y) / l}
	qd := q.a.Dist(q.b)
	if math.Abs(cross(u.X, u.Y, (q.b.X-q.a.X)/qd, (q.b.Y-q.a.Y)/qd)) > 1e-9 {
		return Point{}, Point{}, false
	}
	t0, t1 := dot(Point{q.a.X - p.a.X, q.a.Y - p.a.Y}, u), dot(Point{q.b.X - p.a.X, q.b.Y - p.a.Y}, u)
	lo, hi := math.Max(0, math.Min(t0, t1)), math.Min(l, math.Max(t0, t1))
	if hi-lo <= EndpointTolerance {
		return Point{}, Point{}, false
	}
	m := (lo + hi) / 2
	pa := Point{p.a.X + m*u.X, p.a.Y + m*u.Y}
	_, pb := pointSegment(pa, q.a, q.b)
	return pa, pb, true
}

// tangentAt returns the unit direction of travel along pr at angle phi (arcs) or anywhere (lines).
func tangentAt(pr primitive, phi float64) Point {
	if pr.line {
		d := pr.a.Dist(pr.b)
		return Point{(pr.b.X - pr.a.X) / d, (pr.b.Y - pr.a.Y) / d}
	}
	if pr.sweep >= 0 {
		return Point{-math.Sin(phi), math.Cos(phi)}
	}
	return Point{math.Sin(phi), -math.Cos(phi)}
}

func startTangent(pr primitive) Point { return tangentAt(pr, pr.start) }

func endTangent(pr primitive) Point { return tangentAt(pr, pr.start+pr.sweep) }

// sideNormal turns a tangent into the normal pointing into the loop (inward) or out of it.
func (l Loop) sideNormal(t Point, inward bool) Point {
	out := Point{t.Y, -t.X} // right of travel: outward for a counter-clockwise loop
	if !l.ccw {
		out = Point{-out.X, -out.Y}
	}
	if inward {
		return Point{-out.X, -out.Y}
	}
	return out
}

// facing reports whether dir leaves boundary point q of piece i squarely into the chosen side: within facingTolerance of the side normal, or inside the cone of normals at a vertex.
func (l Loop) facing(i int, q Point, dir Point, inward bool) bool {
	pr := l.prims[i]
	n := len(l.prims)
	var normals []Point
	switch {
	case q.Near(pr.a):
		prev := l.prims[(i-1+n)%n]
		normals = []Point{l.sideNormal(endTangent(prev), inward), l.sideNormal(startTangent(pr), inward)}
	case q.Near(pr.b):
		next := l.prims[(i+1)%n]
		normals = []Point{l.sideNormal(endTangent(pr), inward), l.sideNormal(startTangent(next), inward)}
	default:
		phi := 0.0
		if !pr.line {
			phi = math.Atan2(q.Y-pr.c.Y, q.X-pr.c.X)
		}
		normals = []Point{l.sideNormal(tangentAt(pr, phi), inward)}
	}
	th := math.Atan2(dir.Y, dir.X)
	if len(normals) == 2 && facetJoint(l.prims, i, q) {
		// A joint along a faceted curve: a line across that leans off the curve's normal is not the narrowest place, which lies further along the curve (or nowhere, across a curved end).
		a1 := math.Atan2(normals[0].Y, normals[0].X)
		span := wrapAngle(math.Atan2(normals[1].Y, normals[1].X) - a1)
		off := wrapAngle(th - a1)
		return math.Abs(off-span/2) <= math.Abs(span)/2+smoothFacingTolerance
	}
	for _, nm := range normals {
		if math.Abs(wrapAngle(th-math.Atan2(nm.Y, nm.X))) <= facingTolerance {
			return true
		}
	}
	if len(normals) == 2 {
		a1 := math.Atan2(normals[0].Y, normals[0].X)
		span := wrapAngle(math.Atan2(normals[1].Y, normals[1].X) - a1)
		off := wrapAngle(th - a1)
		return off*span >= 0 && math.Abs(off) <= math.Abs(span)
	}
	return false
}

// facetJoint reports whether boundary point q of piece i is a joint between two straight pieces that bend only slightly, as along a curve drawn as a polygon.
func facetJoint(prims []primitive, i int, q Point) bool {
	n := len(prims)
	a, b := prims[(i-1+n)%n], prims[i]
	if q.Near(prims[i].b) {
		a, b = prims[i], prims[(i+1)%n]
	} else if !q.Near(prims[i].a) {
		return false
	}
	if !a.line || !b.line {
		return false
	}
	ta, tb := endTangent(a), startTangent(b)
	turn := math.Abs(math.Atan2(ta.X*tb.Y-ta.Y*tb.X, ta.X*tb.X+ta.Y*tb.Y))
	return turn <= maxFacetTurnDeg*math.Pi/180
}

// wrapAngle maps an angle to (-π, π].
func wrapAngle(a float64) float64 {
	a = math.Mod(a, 2*math.Pi)
	if a > math.Pi {
		a -= 2 * math.Pi
	} else if a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// containsPoint reports whether q lies strictly inside the loop.
func (l Loop) containsPoint(q Point) bool { return l.bands.contains(q) }

// monotonePieces splits primitives into pieces monotone in y, for ray casting.
func monotonePieces(prims []primitive) []monotone {
	var pieces []monotone
	for _, pr := range prims {
		pieces = append(pieces, yMonotonePrimitive(pr)...)
	}
	return pieces
}

// crossesAny reports whether the segment a-b meets any element away from its own endpoints.
func crossesAny(idx *elementIndex, a, b Point) bool {
	seg := EmptyBounds()
	seg.Add(a)
	seg.Add(b)
	for _, i := range idx.near(seg) {
		if segmentCrosses(a, b, idx.elems[i].prim) {
			return true
		}
	}
	return false
}

// segmentCrosses reports whether a-b meets pr at a point that is not within EndpointTolerance of a or b.
func segmentCrosses(a, b Point, pr primitive) bool {
	away := func(q Point) bool { return !q.Near(a) && !q.Near(b) }
	if pr.line {
		q, ok := segmentIntersection(a, b, pr.a, pr.b)
		if !ok {
			return false
		}
		if away(q) {
			return true
		}
		// Collinear overlap: any shared stretch beyond the endpoints counts.
		for _, e := range []Point{pr.a, pr.b} {
			if d, _ := pointSegment(e, a, b); d <= EndpointTolerance && away(e) {
				return true
			}
		}
		return false
	}
	dx, dy := b.X-a.X, b.Y-a.Y
	fx, fy := a.X-pr.c.X, a.Y-pr.c.Y
	A := dx*dx + dy*dy
	B := 2 * (fx*dx + fy*dy)
	C := fx*fx + fy*fy - pr.r*pr.r
	disc := B*B - 4*A*C
	if A == 0 || disc < 0 {
		return false
	}
	sq := math.Sqrt(disc)
	for _, t := range []float64{(-B - sq) / (2 * A), (-B + sq) / (2 * A)} {
		if t < 0 || t > 1 {
			continue
		}
		q := Point{a.X + t*dx, a.Y + t*dy}
		if away(q) && pr.contains(math.Atan2(q.Y-pr.c.Y, q.X-pr.c.X)) {
			return true
		}
	}
	return false
}
