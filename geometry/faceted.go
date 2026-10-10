package geometry

import "math"

// FacetedCurve is a run of straight segments that turn a little, the same way, at every joint: a curve drawn as a polygon, as exports through mesh formats and file converters write them. Location is the run's middle.
type FacetedCurve struct {
	EntityIDs []string
	Segments  int
	Location  Point
}

// A faceted curve has at least minFacetedSegments segments, each turning from the last by more than minFacetTurnDeg (less is a straight line drawn in pieces) and at most maxFacetTurnDeg (24 or more sides to a circle; more is a deliberate polygon or corner).
const (
	minFacetedSegments = 8
	minFacetTurnDeg    = 0.25
	maxFacetTurnDeg    = 15
)

// facetedCurves finds the faceted curves of a contour, each the longest run it can be.
func facetedCurves(c Contour) []FacetedCurve {
	segs := c.Segments
	n := len(segs)
	if n < minFacetedSegments {
		return nil
	}
	// turns[j] is the direction of the turn from segment j to the next: +1 or -1 when it is a facet joint, otherwise 0.
	m := n - 1
	if c.Closed {
		m = n
	}
	turns := make([]int, m)
	all := true
	for j := range m {
		turns[j] = facetTurn(segs[j], segs[(j+1)%n])
		all = all && turns[j] != 0 && turns[j] == turns[0]
	}
	if c.Closed && all {
		return []FacetedCurve{facetedRun(segs, 0, n)}
	}
	// Start just after a break, so no run wraps past the start uncounted.
	start := 0
	if c.Closed {
		for j := range m {
			if turns[j] != turns[(j+m-1)%m] {
				start = j
				break
			}
		}
	}
	var out []FacetedCurve
	for k := 0; k < m; {
		j := (start + k) % m
		if turns[j] == 0 {
			k++
			continue
		}
		g := 1
		for k+g < m && turns[(start+k+g)%m] == turns[j] {
			g++
		}
		if g+1 >= minFacetedSegments {
			out = append(out, facetedRun(segs, j, g+1))
		}
		k += g
	}
	return out
}

// facetTurn returns the direction of the turn between two consecutive straight segments if it is in the facet range, otherwise 0.
func facetTurn(a, b Segment) int {
	if a.IsArc() || b.IsArc() || a.Chord() == 0 || b.Chord() == 0 {
		return 0
	}
	ax, ay := a.End.X-a.Start.X, a.End.Y-a.Start.Y
	bx, by := b.End.X-b.Start.X, b.End.Y-b.Start.Y
	turn := math.Atan2(ax*by-ay*bx, ax*bx+ay*by) * 180 / math.Pi
	switch {
	case math.Abs(turn) <= minFacetTurnDeg || math.Abs(turn) > maxFacetTurnDeg:
		return 0
	case turn > 0:
		return 1
	}
	return -1
}

// facetedRun describes count segments from first, wrapping round.
func facetedRun(segs []Segment, first, count int) FacetedCurve {
	run := Contour{Segments: make([]Segment, count)}
	for i := range count {
		run.Segments[i] = segs[(first+i)%len(segs)]
	}
	return FacetedCurve{EntityIDs: run.EntityIDs(), Segments: count, Location: segmentMidpoint(run.Segments[count/2])}
}

// maxFacetedHoleOutOfRound is how far, as a share of the radius, the corners of a faceted cut-out may stray from a circle for it to be read as a round hole.
const maxFacetedHoleOutOfRound = 0.01

// facetedHole reads a closed cut-out drawn wholly as one faceted curve, with its corners on a circle, as the round hole it approximates: the circle through its corners (their mean distance from its center).
func facetedHole(c Contour, curves []FacetedCurve) (Circle, bool) {
	if !c.Closed || c.Depth%2 != 1 || len(curves) != 1 || curves[0].Segments != len(c.Segments) {
		return Circle{}, false
	}
	var center Point
	for _, s := range c.Segments {
		center.X += s.Start.X
		center.Y += s.Start.Y
	}
	n := float64(len(c.Segments))
	center = Point{center.X / n, center.Y / n}
	lo, hi, sum := math.Inf(1), 0.0, 0.0
	for _, s := range c.Segments {
		d := s.Start.Dist(center)
		lo, hi, sum = math.Min(lo, d), math.Max(hi, d), sum+d
	}
	if hi == 0 || (hi-lo)/hi > maxFacetedHoleOutOfRound {
		return Circle{}, false
	}
	return Circle{Center: center, Radius: sum / n, EntityIDs: c.EntityIDs(), Depth: c.Depth}, true
}

// curvePrim returns piece k of the loop as the curve it stands for when it is a facet (a straight piece about as long as its neighbors, between two that turn the same small way): the arc through its ends turning as they do. Other pieces are returned as they are. Measuring against these allows for how far a polygon strays from the curve drawn.
func (l Loop) curvePrim(k int) (primitive, bool) {
	n := len(l.prims)
	pr := l.prims[k]
	if n < minFacetedSegments {
		return pr, false
	}
	prev, next := l.prims[(k-1+n)%n], l.prims[(k+1)%n]
	if !pr.line || !prev.line || !next.line {
		return pr, false
	}
	t1, t2 := lineTurn(prev, pr), lineTurn(pr, next)
	lo, hi := minFacetTurnDeg*math.Pi/180, maxFacetTurnDeg*math.Pi/180
	if t1*t2 <= 0 || math.Abs(t1) <= lo || math.Abs(t1) > hi || math.Abs(t2) <= lo || math.Abs(t2) > hi {
		return pr, false
	}
	// Facets of one curve are much the same length; a long straight side meeting a curve at a slight angle is not one.
	chord := pr.a.Dist(pr.b)
	for _, nb := range []primitive{prev, next} {
		if m := nb.a.Dist(nb.b); chord == 0 || chord > 2*m || m > 2*chord {
			return pr, false
		}
	}
	theta := (math.Abs(t1) + math.Abs(t2)) / 2
	return segmentPrimitive(Segment{Start: pr.a, End: pr.b, Bulge: math.Copysign(math.Tan(theta/4), t1)}), true
}

// lineTurn is the signed turn, in radians, from line a to line b.
func lineTurn(a, b primitive) float64 {
	ax, ay := a.b.X-a.a.X, a.b.Y-a.a.Y
	bx, by := b.b.X-b.a.X, b.b.Y-b.a.Y
	return math.Atan2(ax*by-ay*bx, ax*bx+ay*by)
}

// curveDistance is the distance between pieces i of la and j of lb as if facets were the curves they stand for, or 0 when neither is a facet.
func curveDistance(la Loop, i int, lb Loop, j int) float64 {
	pa, fa := la.curvePrim(i)
	pb, fb := lb.curvePrim(j)
	if !fa && !fb {
		return 0
	}
	d, _, _ := primitiveDistance(pa, pb)
	return d
}
