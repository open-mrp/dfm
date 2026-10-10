package geometry

import (
	"math"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
)

// BendLine is a straight bend line of the drawing, with how it is bent.
type BendLine struct {
	EntityID string
	A, B     Point
	Spec     BendSpec
}

// BendSpec is how a bend line is bent (set by the bending check; zero otherwise).
type BendSpec struct {
	// Angle is from flat, in millionths of a degree.
	Angle     measure.Ratio
	Direction constants.BendDirection
	// InsideRadius is the radius the design is drawn for, exact in millimeters.
	InsideRadius measure.Decimal
	// AngleGiven, DirectionGiven and RadiusGiven are false where the default was assumed.
	AngleGiven, DirectionGiven, RadiusGiven bool
	// AllowanceMM is the length of flat material the bend consumes, at the radius the tooling forms: the bend zone, centered on the line.
	AllowanceMM float64
}

// Length is the bend's length.
func (b BendLine) Length() float64 { return b.A.Dist(b.B) }

// Midpoint is the middle of the bend line.
func (b BendLine) Midpoint() Point { return Point{X: (b.A.X + b.B.X) / 2, Y: (b.A.Y + b.B.Y) / 2} }

// BendLines returns the drawing's straight bend lines, in drawing order.
func (p *Part) BendLines() []BendLine {
	var out []BendLine
	for _, n := range p.NonCut {
		if n.Kind == NonCutBend && n.Line != nil {
			out = append(out, BendLine{EntityID: n.EntityID, A: n.Line[0], B: n.Line[1], Spec: p.BendSpecs[n.EntityID]})
		}
	}
	return out
}

// outlineAround returns the outer loop whose inside holds q, or -1.
func (p *Part) outlineAround(q Point) int {
	for i, l := range p.Loops {
		if l.Depth == 0 && l.containsPoint(q) {
			return i
		}
	}
	return -1
}

// Flange is the shortest flange on one side of a bend: the distance from a point on the bend line, along its normal, to the part's outline or the next bend line.
type Flange struct {
	LengthMM float64
	From, To Point
	// ToBend is the entity ID of the bend line the flange ends at, empty when it ends at the outline.
	ToBend string
}

// Flanges measures the flanges on both sides of bend line b: from points along the middle 80% of the line (its ends are where reliefs are), along its normal, to the outline of the part it lies in (curves as polygons with points at most 10° apart) or to another bend line, whichever is nearer: material between two bends is a flange of both. ok is false when the line does not lie in a part.
func (p *Part) Flanges(b BendLine) (left, right Flange, ok bool) {
	length := b.Length()
	mid := Point{(b.A.X + b.B.X) / 2, (b.A.Y + b.B.Y) / 2}
	root := p.outlineAround(mid)
	if root < 0 || length == 0 {
		return Flange{}, Flange{}, false
	}
	poly := loopPolygon(p.Loops[root])
	ux, uy := (b.B.X-b.A.X)/length, (b.B.Y-b.A.Y)/length
	reach := math.Hypot(p.Bounds.WidthMM(), p.Bounds.HeightMM()) + 1
	var others []BendLine
	for _, o := range p.BendLines() {
		if o.EntityID != b.EntityID && p.outlineAround(Point{(o.A.X + o.B.X) / 2, (o.A.Y + o.B.Y) / 2}) == root {
			others = append(others, o)
		}
	}
	left, right = Flange{LengthMM: math.Inf(1)}, Flange{LengthMM: math.Inf(1)}
	const samples = 40
	for i := 0; i <= samples; i++ {
		t := length * (0.1 + 0.8*float64(i)/samples)
		q := Point{b.A.X + float64(t*ux), b.A.Y + float64(t*uy)}
		for side, f := range []*Flange{&left, &right} {
			nx, ny := -uy, ux
			if side == 1 {
				nx, ny = uy, -ux
			}
			far := Point{q.X + float64(reach*nx), q.Y + float64(reach*ny)}
			if d, hit, found := nearestHit(poly, q, far); found && d < f.LengthMM {
				*f = Flange{LengthMM: d, From: q, To: hit}
			}
			for _, o := range others {
				if hit, found := segmentIntersection(q, far, o.A, o.B); found {
					if d := q.Dist(hit); d > EndpointTolerance && d < f.LengthMM {
						*f = Flange{LengthMM: d, From: q, To: hit, ToBend: o.EntityID}
					}
				}
			}
		}
	}
	return left, right, !math.IsInf(left.LengthMM, 1) && !math.IsInf(right.LengthMM, 1)
}

// nearestHit returns the nearest point where segment q-far crosses the polygon, beyond EndpointTolerance.
func nearestHit(poly []Point, q, far Point) (float64, Point, bool) {
	best, at, found := math.Inf(1), Point{}, false
	for k := range poly {
		hit, ok := segmentIntersection(q, far, poly[k], poly[(k+1)%len(poly)])
		if !ok {
			continue
		}
		if d := q.Dist(hit); d > EndpointTolerance && d < best {
			best, at, found = d, hit, true
		}
	}
	return best, at, found
}

// BendFeature is a hole or cut-out near a bend line.
type BendFeature struct {
	// Loop indexes Part.Loops.
	Loop       int
	DistanceMM float64
	// OnBend and OnFeature are the closest points.
	OnBend, OnFeature Point
}

// BendFeatures returns the holes and cut-outs of the part bend line b lies in whose edge comes within radius of the line, with the exact distance. A cut-out the line crosses is at distance 0.
func (p *Part) BendFeatures(b BendLine, radius float64) []BendFeature {
	mid := Point{(b.A.X + b.B.X) / 2, (b.A.Y + b.B.Y) / 2}
	root := p.outlineAround(mid)
	if root < 0 {
		return nil
	}
	line := primitive{line: true, a: b.A, b: b.B}
	var out []BendFeature
	for i, l := range p.Loops {
		if !l.IsCutout() || p.rootOf(i) != root {
			continue
		}
		best := BendFeature{Loop: i, DistanceMM: math.Inf(1)}
		for _, pr := range l.prims {
			if d, pa, pb := primitiveDistance(line, pr); d < best.DistanceMM {
				best.DistanceMM, best.OnBend, best.OnFeature = d, pa, pb
			}
		}
		if best.DistanceMM <= EndpointTolerance && !l.containsPoint(mid) && endsOn(b, l) {
			continue // the relief the bend ends on, sized by the relief checks
		}
		if l.containsPoint(mid) || crossesLoop(line, l) {
			best.DistanceMM = 0
		}
		if best.DistanceMM <= radius {
			out = append(out, best)
		}
	}
	return out
}

// endsOn reports whether an end of the bend line touches the loop, and the
// line does not run on through it.
func endsOn(b BendLine, l Loop) bool {
	near := func(q Point) bool {
		for _, pr := range l.prims {
			if d, _, _ := primitiveDistance(primitive{line: true, a: q, b: q}, pr); d <= EndpointTolerance {
				return true
			}
		}
		return l.containsPoint(q)
	}
	length := b.Length()
	if length == 0 {
		return false
	}
	ux, uy := (b.B.X-b.A.X)/length, (b.B.Y-b.A.Y)/length
	step := min(1.0, length/4)
	inner := func(q Point, sign float64) Point {
		return Point{q.X + float64(sign*step*ux), q.Y + float64(sign*step*uy)}
	}
	return near(b.A) && !l.containsPoint(inner(b.A, 1)) || near(b.B) && !l.containsPoint(inner(b.B, -1))
}

// crossesLoop reports whether the line meets the loop's boundary.
func crossesLoop(line primitive, l Loop) bool {
	for _, pr := range l.prims {
		if d, _, _ := primitiveDistance(line, pr); d <= EndpointTolerance {
			return true
		}
	}
	return false
}

// BendEndKind says how a bend line ends.
type BendEndKind int

const (
	// BendEndFree ends on an edge that crosses the bend (the part's side, a relief slot or a relief hole): the material beside it is free to bend.
	BendEndFree BendEndKind = iota
	// BendEndInsideCorner ends at an inside corner of the material, where the bent flange meets material that stays flat: it tears.
	BendEndInsideCorner
	// BendEndAlongEdge ends running along an edge, so the bend continues into material that stays flat: it tears.
	BendEndAlongEdge
	// BendEndInMaterial stops inside the material.
	BendEndInMaterial
)

// parallelBendAngle is how far, in radians, an edge may turn from the bend and still run along it.
const parallelBendAngle = 10 * math.Pi / 180

// BendEnd is one end of a bend line.
type BendEnd struct {
	At   Point
	Kind BendEndKind
}

// BendEnds classifies both ends of bend line b.
func (p *Part) BendEnds(b BendLine) [2]BendEnd {
	length := b.Length()
	if length == 0 {
		return [2]BendEnd{{b.A, BendEndFree}, {b.B, BendEndFree}}
	}
	u := Point{(b.B.X - b.A.X) / length, (b.B.Y - b.A.Y) / length}
	return [2]BendEnd{{b.A, p.bendEnd(b.A, u)}, {b.B, p.bendEnd(b.B, u)}}
}

func (p *Part) bendEnd(e, u Point) BendEndKind {
	parallel := func(t Point) bool {
		return math.Abs(cross(t.X, t.Y, u.X, u.Y)) <= math.Sin(parallelBendAngle)
	}
	for _, l := range p.Loops {
		for k, pr := range l.prims {
			if d, _ := pr.pointDist(e); d > EndpointTolerance {
				continue
			}
			n := len(l.prims)
			var in, out Point
			switch {
			case pr.full:
				// A relief hole: its edge at e crosses the bend unless the bend runs along it.
				phi := math.Atan2(e.Y-pr.c.Y, e.X-pr.c.X)
				if parallel(tangentAt(pr, phi)) {
					return BendEndAlongEdge
				}
				return BendEndFree
			case e.Near(pr.a):
				in, out = endTangent(l.prims[(k-1+n)%n]), startTangent(pr)
			case e.Near(pr.b):
				in, out = endTangent(pr), startTangent(l.prims[(k+1)%n])
			default:
				phi := 0.0
				if !pr.line {
					phi = math.Atan2(e.Y-pr.c.Y, e.X-pr.c.X)
				}
				if parallel(tangentAt(pr, phi)) {
					return BendEndAlongEdge
				}
				return BendEndFree
			}
			// At a vertex: a corner where the outline turns. Material is left of travel on a counter-clockwise outline; a turn away from the material is an inside corner.
			turn := cross(in.X, in.Y, out.X, out.Y)
			if math.Abs(turn) <= math.Sin(parallelBendAngle) && dot(in, out) > 0 {
				if parallel(in) {
					return BendEndAlongEdge
				}
				return BendEndFree
			}
			materialLeft := l.ccw == (l.Depth%2 == 0)
			if (turn < 0) == materialLeft {
				// An inside corner tears when an edge runs on along the bend (a tab off a plate's edge); the tip of a V notch, both sides crossing the bend, is a relief.
				if parallel(in) || parallel(out) {
					return BendEndInsideCorner
				}
				return BendEndFree
			}
			if parallel(in) || parallel(out) {
				return BendEndAlongEdge
			}
			return BendEndFree
		}
	}
	return BendEndInMaterial
}

// BendRelief is the gap a bend line ends at: a slot, notch or hole the material on both sides of the bend line stops at.
type BendRelief struct {
	// End is 0 for the line's start, 1 for its end.
	End int
	// WidthMM is the gap along the bend line, from the end (Width[0]) to where the material starts again (Width[1]).
	WidthMM float64
	Width   [2]Point
	// DepthMM is how far the gap reaches past the bend line, measured across it from the middle of the gap (Depth[0]) to its bottom (Depth[1]): the shallower side when both sides are closed.
	DepthMM float64
	Depth   [2]Point
}

// BendReliefs returns the reliefs bend line b ends at: at each free end (see BendEnds), the gap along the line until material starts again, when it does within maxWidth. An end on the part's side, with nothing beyond it, has no relief, nor has one whose gap opens to the outside on both sides of the line (an edge notch the whole bend runs out through).
func (p *Part) BendReliefs(b BendLine, maxWidth float64) []BendRelief {
	length := b.Length()
	if length == 0 {
		return nil
	}
	u := Point{(b.B.X - b.A.X) / length, (b.B.Y - b.A.Y) / length}
	n := Point{-u.Y, u.X}
	var out []BendRelief
	for i, e := range p.BendEnds(b) {
		if e.Kind != BendEndFree {
			continue
		}
		dir := u
		if i == 0 {
			dir = Point{-u.X, -u.Y}
		}
		w, ok := p.castRay(e.At, dir, maxWidth)
		if !ok {
			continue
		}
		far := Point{e.At.X + float64(w*dir.X), e.At.Y + float64(w*dir.Y)}
		m := Point{(e.At.X + far.X) / 2, (e.At.Y + far.Y) / 2}
		r := BendRelief{End: i, WidthMM: w, Width: [2]Point{e.At, far}, DepthMM: math.Inf(1), Depth: [2]Point{m, m}}
		for _, s := range []Point{n, {-n.X, -n.Y}} {
			if d, ok := p.castRay(m, s, math.Inf(1)); ok && d < r.DepthMM {
				r.DepthMM, r.Depth[1] = d, Point{m.X + float64(d*s.X), m.Y + float64(d*s.Y)}
			}
		}
		if math.IsInf(r.DepthMM, 1) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// castRay returns the distance from q along unit direction d to the nearest cut it crosses, beyond EndpointTolerance and within reach. Edges running along the ray are not crossings.
func (p *Part) castRay(q, d Point, reach float64) (float64, bool) {
	best := math.Inf(1)
	try := func(t float64) {
		if t > EndpointTolerance && t <= reach && t < best {
			best = t
		}
	}
	for _, l := range p.Loops {
		for _, pr := range l.prims {
			if pr.line {
				sx, sy := pr.b.X-pr.a.X, pr.b.Y-pr.a.Y
				den := cross(d.X, d.Y, sx, sy)
				if den == 0 {
					continue
				}
				qx, qy := pr.a.X-q.X, pr.a.Y-q.Y
				t, s := cross(qx, qy, sx, sy)/den, cross(qx, qy, d.X, d.Y)/den
				if s >= 0 && s <= 1 {
					try(t)
				}
				continue
			}
			fx, fy := q.X-pr.c.X, q.Y-pr.c.Y
			bh := float64(fx*d.X) + float64(fy*d.Y)
			c := float64(fx*fx) + float64(fy*fy) - float64(pr.r*pr.r)
			disc := float64(bh*bh) - c
			if disc < 0 {
				continue
			}
			sq := math.Sqrt(disc)
			for _, t := range []float64{-bh - sq, -bh + sq} {
				at := Point{q.X + float64(t*d.X), q.Y + float64(t*d.Y)}
				if pr.full || pr.contains(math.Atan2(at.Y-pr.c.Y, at.X-pr.c.X)) {
					try(t)
				}
			}
		}
	}
	return best, !math.IsInf(best, 1)
}

// SnapBendEnds moves an end of a bend line that stops inside the material to the edge it points at, when that is at most gap away: drawings stop bend lines short of the edge they bend to (fold lines from some CAD stop 1 mm before a relief notch).
func (p *Part) SnapBendEnds(gap float64) {
	var polys [][]Point
	for _, l := range p.Loops {
		polys = append(polys, loopPolygon(l))
	}
	touches := func(q Point) bool {
		for _, l := range p.Loops {
			for _, pr := range l.prims {
				if d, _ := pr.pointDist(q); d <= EndpointTolerance {
					return true
				}
			}
		}
		return false
	}
	snap := func(e, dir Point) Point {
		if touches(e) || !p.Covers(e, false) {
			return e
		}
		far := Point{e.X + float64(dir.X*gap), e.Y + float64(dir.Y*gap)}
		best, at := math.Inf(1), e
		for _, poly := range polys {
			if d, hit, ok := nearestHit(poly, e, far); ok && d < best {
				best, at = d, hit
			}
		}
		return at
	}
	for i := range p.NonCut {
		n := &p.NonCut[i]
		if n.Kind != NonCutBend || n.Line == nil {
			continue
		}
		a, b := n.Line[0], n.Line[1]
		l := a.Dist(b)
		if l == 0 {
			continue
		}
		u := Point{(b.X - a.X) / l, (b.Y - a.Y) / l}
		n.Line = &[2]Point{snap(a, Point{-u.X, -u.Y}), snap(b, u)}
	}
}
