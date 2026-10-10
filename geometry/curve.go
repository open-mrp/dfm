package geometry

import (
	"errors"
	"math"
)

// CurveTolerance is the furthest (mm) a fitted arc may stray from the smooth curve it replaces.
const CurveTolerance = 0.5e-3

// maxFitDepth bounds how often a curve piece is halved while fitting, and maxFitPieces how many pieces one curve may be fitted with.
const (
	maxFitDepth  = 24
	maxFitPieces = 1 << 14
)

// ErrCurveTooComplex is returned when a curve cannot be fitted within CurveTolerance using maxFitPieces pieces.
var ErrCurveTooComplex = errors.New("curve too complex to approximate within tolerance")

// Curve evaluates a smooth parametric curve: its point and derivative at t.
type Curve func(t float64) (p, d Point)

// FitArcs approximates the curve between t0 and t1 with tangent-continuous circular arcs (pairs of arcs meeting with a common tangent), halving the range until every pair is within CurveTolerance of the curve. Because neighboring arcs share tangents, the result has no corners the curve does not have, and a tight bend keeps its radius.
func FitArcs(c Curve, t0, t1 float64, id string) ([]Segment, error) {
	var out []Segment
	budget := maxFitPieces
	var fit func(a, b float64, depth int)
	fit = func(a, b float64, depth int) {
		if budget <= 0 {
			return
		}
		budget--
		// A stretch that stays on its chord is a line (a cubic whose control points lie on it, as PDF imports write every edge).
		pa, _ := c(a)
		pb, _ := c(b)
		if line := []Segment{{Start: pa, End: pb, EntityID: id}}; !pa.Near(pb) && withinTolerance(c, a, b, line) {
			out = append(out, line[0])
			return
		}
		segs, ok := biarc(c, a, b, id)
		if (!ok || !gentle(segs) || !withinTolerance(c, a, b, segs)) && depth < maxFitDepth {
			m := (a + b) / 2
			fit(a, m, depth+1)
			fit(m, b, depth+1)
			return
		}
		if !ok {
			pa, _ := c(a)
			pb, _ := c(b)
			segs = []Segment{{Start: pa, End: pb, EntityID: id}}
		}
		for _, s := range segs {
			if s.Chord() > 0 {
				out = append(out, s)
			}
		}
	}
	fit(t0, t1, 0)
	if budget <= 0 {
		return nil, ErrCurveTooComplex
	}
	return out, nil
}

// tangent returns the unit direction of travel at t, falling back to a short chord where the derivative vanishes.
func tangent(c Curve, t, toward float64) (Point, bool) {
	p, d := c(t)
	if l := math.Hypot(d.X, d.Y); l > 1e-12 {
		return Point{d.X / l, d.Y / l}, true
	}
	q, _ := c(t + (toward-t)*1e-6)
	v := Point{q.X - p.X, q.Y - p.Y}
	if toward < t {
		v = Point{-v.X, -v.Y}
	}
	l := math.Hypot(v.X, v.Y)
	if l == 0 {
		return Point{}, false
	}
	return Point{v.X / l, v.Y / l}, true
}

// biarc joins c(a) and c(b) with two arcs that match the curve's tangents at both ends and each other's at the joint (equal tangent lengths).
func biarc(c Curve, a, b float64, id string) ([]Segment, bool) {
	p0, _ := c(a)
	p1, _ := c(b)
	t0, ok0 := tangent(c, a, b)
	t1, ok1 := tangent(c, b, a)
	v := Point{p1.X - p0.X, p1.Y - p0.Y}
	if !ok0 || !ok1 || math.Hypot(v.X, v.Y) < 1e-12 {
		return nil, false
	}
	if l := math.Hypot(v.X, v.Y); dot(t0, v) > 0 && dot(t1, v) > 0 &&
		math.Abs(cross(t0.X, t0.Y, v.X, v.Y)) <= 1e-12*l && math.Abs(cross(t1.X, t1.Y, v.X, v.Y)) <= 1e-12*l {
		return []Segment{{Start: p0, End: p1, EntityID: id}}, true // straight
	}
	// Control points p0 + d·t0 and p1 − d·t1 lie 2d apart: (t·t − 4)d² − 2(v·t)d + v·v = 0 with t = t0 + t1.
	t := Point{t0.X + t1.X, t0.Y + t1.Y}
	qa, vt, vv := dot(t, t)-4, dot(v, t), dot(v, v)
	var d float64
	if math.Abs(qa) < 1e-12 {
		if vt <= 0 {
			return nil, false
		}
		d = vv / (2 * vt)
	} else {
		d = (vt - math.Sqrt(vt*vt-qa*vv)) / qa
	}
	if !(d > 0) || math.IsInf(d, 0) {
		return nil, false
	}
	j := Point{(p0.X + d*t0.X + p1.X - d*t1.X) / 2, (p0.Y + d*t0.Y + p1.Y - d*t1.Y) / 2}
	return []Segment{
		{Start: p0, End: j, Bulge: bulgeFromStart(p0, j, t0), EntityID: id},
		{Start: j, End: p1, Bulge: bulgeFromEnd(j, p1, t1), EntityID: id},
	}, true
}

// bulgeFromStart is the bulge of the arc from a to b leaving a along t.
func bulgeFromStart(a, b, t Point) float64 {
	ch := Point{b.X - a.X, b.Y - a.Y}
	return straighten(math.Tan(math.Atan2(cross(t.X, t.Y, ch.X, ch.Y), dot(t, ch))/2), ch)
}

// bulgeFromEnd is the bulge of the arc from a to b arriving at b along t.
func bulgeFromEnd(a, b, t Point) float64 {
	ch := Point{b.X - a.X, b.Y - a.Y}
	return straighten(math.Tan(math.Atan2(cross(ch.X, ch.Y, t.X, t.Y), dot(ch, t))/2), ch)
}

// straighten returns 0 for a bulge whose arc departs from its chord by less than a nanometer. Such an arc has a radius beyond the range distances can be computed in (a bulge of 1e-17 is a radius of 1e17 mm), which once let a fit that missed the curve pass as within tolerance.
func straighten(bulge float64, chord Point) float64 {
	if sagitta := math.Abs(bulge) * math.Hypot(chord.X, chord.Y) / 2; sagitta < 1e-6 {
		return 0
	}
	return bulge
}

// gentle reports whether each arc turns at most a quarter turn, so a fitted pair cannot loop away from the curve between samples.
func gentle(segs []Segment) bool {
	for _, s := range segs {
		if math.Abs(s.Bulge) > math.Tan(math.Pi/8) {
			return false
		}
	}
	return true
}

// withinTolerance samples the curve between a and b and checks every sample lies within CurveTolerance of the fitted segments.
func withinTolerance(c Curve, a, b float64, segs []Segment) bool {
	const samples = 16
	var buf [2]primitive
	prims := buf[:0]
	for _, s := range segs {
		prims = append(prims, segmentPrimitive(s))
	}
	for i := 1; i < samples; i++ {
		q, _ := c(a + (b-a)*float64(i)/samples)
		best := math.Inf(1)
		for _, pr := range prims {
			d, _ := pr.pointDist(q)
			best = math.Min(best, d)
		}
		if best > CurveTolerance {
			return false
		}
	}
	return true
}
