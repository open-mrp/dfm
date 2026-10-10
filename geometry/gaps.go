package geometry

import (
	"math"
	"sort"
)

// Gap is a break between path ends that was closed with a straight bridge because it was within the closure tolerance.
type Gap struct {
	EntityIDs []string
	LengthMM  float64
	Location  Point // midpoint of the bridge
}

// Opening is an open contour and the gap that keeps it open: the shortest distance from either of its ends to the nearest other loose end (its own other end included). Location is the midpoint of that gap.
type Opening struct {
	Contour  int
	GapMM    float64
	Location Point
}

// maxNearestEnds bounds the quadratic nearest-end search. Beyond it each open contour reports the distance between its own ends.
const maxNearestEnds = 4000

// end is one end of an open contour.
type end struct {
	contour int
	tail    bool
}

func (e end) point(cs []Contour) Point {
	segs := cs[e.contour].Segments
	if e.tail {
		return segs[len(segs)-1].End
	}
	return segs[0].Start
}

func (e end) entityID(cs []Contour) string {
	segs := cs[e.contour].Segments
	if e.tail {
		return segs[len(segs)-1].EntityID
	}
	return segs[0].EntityID
}

// closeGaps joins open contours whose ends are within tol of each other, the closest pair first (ties broken by contour order, so the result is deterministic), bridging each gap with a straight segment. A contour's own two ends are joined only when it has more than one segment or is an arc, so a short stray line is never turned into a sliver loop.
func closeGaps(cs []Contour, tol float64) ([]Contour, []Gap) {
	var gaps []Gap
	for {
		a, b, d, ok := closestEnds(cs, tol)
		if !ok {
			return cs, gaps
		}
		gaps = append(gaps, Gap{
			EntityIDs: appendUnique(nil, a.entityID(cs), b.entityID(cs)),
			LengthMM:  d,
			Location:  midpoint(a.point(cs), b.point(cs)),
		})
		if a.contour == b.contour {
			c := &cs[a.contour]
			last := c.Segments[len(c.Segments)-1]
			c.Segments = append(c.Segments, Segment{Start: last.End, End: c.Segments[0].Start, EntityID: last.EntityID})
			c.Closed = true
			continue
		}
		// Orient so a is the first contour's tail and b the second's head.
		first, second := cs[a.contour].Segments, cs[b.contour].Segments
		if !a.tail {
			first = reversed(first)
		}
		if b.tail {
			second = reversed(second)
		}
		tail := first[len(first)-1]
		segs := append(append(append([]Segment(nil), first...),
			Segment{Start: tail.End, End: second[0].Start, EntityID: tail.EntityID}), second...)
		joined := Contour{Segments: segs, Closed: segs[len(segs)-1].End.Near(segs[0].Start)}
		lo, hi := min(a.contour, b.contour), max(a.contour, b.contour)
		cs[lo] = joined
		cs = append(cs[:hi], cs[hi+1:]...)
	}
}

// closestEnds returns the closest pair of open ends no farther apart than tol, if any.
func closestEnds(cs []Contour, tol float64) (end, end, float64, bool) {
	grid := newPointGrid[end](tol)
	var ends []end
	for i, c := range cs {
		if c.Closed || len(c.Segments) == 0 {
			continue
		}
		for _, e := range []end{{i, false}, {i, true}} {
			ends = append(ends, e)
			grid.add(e.point(cs), e)
		}
	}
	var best [2]end
	bestD, found := math.Inf(1), false
	for _, a := range ends {
		p := a.point(cs)
		grid.around(p, func(b end) bool {
			if !before(a, b) || !joinable(cs, a, b) {
				return false
			}
			d := p.Dist(b.point(cs))
			if d <= tol && d < bestD {
				best, bestD, found = [2]end{a, b}, d, true
			}
			return false
		})
	}
	return best[0], best[1], bestD, found
}

// before orders ends by contour, head before tail, so each pair is considered once and the earliest pair wins a tie.
func before(a, b end) bool {
	if a.contour != b.contour {
		return a.contour < b.contour
	}
	return !a.tail && b.tail
}

func joinable(cs []Contour, a, b end) bool {
	if a.contour != b.contour {
		return true
	}
	segs := cs[a.contour].Segments
	return len(segs) > 1 || segs[0].IsArc()
}

// openings measures the gap of every open contour.
func openings(cs []Contour) []Opening {
	var ends []end
	for i, c := range cs {
		if !c.Closed {
			ends = append(ends, end{i, false}, end{i, true})
		}
	}
	var out []Opening
	for i, c := range cs {
		if c.Closed {
			continue
		}
		head, tail := end{i, false}, end{i, true}
		bestD, bestAt := head.point(cs).Dist(tail.point(cs)), midpoint(head.point(cs), tail.point(cs))
		if len(ends) <= maxNearestEnds {
			for _, own := range []end{head, tail} {
				p := own.point(cs)
				for _, o := range ends {
					if o.contour == i {
						continue
					}
					if d := p.Dist(o.point(cs)); d < bestD {
						bestD, bestAt = d, midpoint(p, o.point(cs))
					}
				}
			}
		}
		out = append(out, Opening{Contour: i, GapMM: bestD, Location: bestAt})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Contour < out[b].Contour })
	return out
}

func midpoint(a, b Point) Point {
	return Point{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
}
