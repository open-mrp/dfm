package geometry

import (
	"math"
	"slices"
)

// Placement maps one part onto another: a point p of the first is at R(RotationDeg)·p + Offset in the second, rotating about the origin.
type Placement struct {
	RotationDeg float64
	Offset      Point
}

// Apply places p.
func (pl Placement) Apply(p Point) Point {
	s, c := math.Sincos(pl.RotationDeg * math.Pi / 180)
	return Point{c*p.X - s*p.Y + pl.Offset.X, s*p.X + c*p.Y + pl.Offset.Y}
}

// maxIdenticalCandidates bounds the rotations tried for one pair of parts (one per point at the reference distance: a gear tries one per tooth).
const maxIdenticalCandidates = 720

// SameShape reports whether b is a copy of a, moved and rotated but not mirrored, with every cut path and circle matching within EndpointTolerance, and returns how a is placed onto b. Paths must be split into the same pieces in both.
func SameShape(a, b *Part) (Placement, bool) {
	if a.shape == nil {
		a.shape = shapeOf(a)
	}
	if b.shape == nil {
		b.shape = shapeOf(b)
	}
	fa, fb := a.shape, b.shape
	if !fa.comparable(fb) {
		return Placement{}, false
	}
	// Turn a's farthest point from its center onto each of b's points as far out, and check the rest.
	ref := fa.points[fa.farthest]
	refAngle := math.Atan2(ref.Y-fa.center.Y, ref.X-fa.center.X)
	r := ref.Dist(fa.center)
	tried := 0
	for _, q := range fb.points {
		if math.Abs(q.Dist(fb.center)-r) > EndpointTolerance {
			continue
		}
		if tried++; tried > maxIdenticalCandidates {
			break
		}
		turn := math.Atan2(q.Y-fb.center.Y, q.X-fb.center.X) - refAngle
		if r <= EndpointTolerance {
			turn = 0 // every point is at the center: nothing to turn by
		}
		pl := Placement{RotationDeg: turn * 180 / math.Pi}
		c := pl.Apply(fa.center)
		pl.Offset = Point{fb.center.X - c.X, fb.center.Y - c.Y}
		if fb.matches(fa, pl) {
			pl.RotationDeg = normalizeDeg(pl.RotationDeg)
			return pl, true
		}
	}
	return Placement{}, false
}

// normalizeDeg maps an angle to [0, 360), with angles within a millionth of a degree of a whole one snapped to it.
func normalizeDeg(d float64) float64 {
	d = math.Mod(d, 360)
	if d < 0 {
		d += 360
	}
	if r := math.Round(d); math.Abs(d-r) < 1e-6 {
		d = math.Mod(r, 360)
	}
	return d
}

// shape is what SameShape compares: the part's cut pieces, the points they are drawn through and their center, and sorted sizes that differ between parts that cannot be the same.
type shape struct {
	segs     []Segment
	circles  []Circle
	points   []Point        // segment starts and ends, then circle centers
	center   Point          // the mean of points
	farthest int            // index of the point farthest from center
	sizes    []float64      // piece lengths, then circle radii, each group sorted
	grid     pointGrid[int] // segs and circles (offset by len(segs)) by their midpoint
}

func shapeOf(p *Part) *shape {
	s := &shape{}
	for _, c := range p.Contours {
		s.segs = append(s.segs, c.Segments...)
	}
	s.circles = p.Circles
	for _, g := range s.segs {
		s.points = append(s.points, g.Start, g.End)
	}
	for _, c := range s.circles {
		s.points = append(s.points, c.Center)
	}
	if len(s.points) == 0 {
		return s
	}
	for _, q := range s.points {
		s.center.X += q.X
		s.center.Y += q.Y
	}
	n := float64(len(s.points))
	s.center = Point{s.center.X / n, s.center.Y / n}
	for i, q := range s.points {
		if q.Dist(s.center) > s.points[s.farthest].Dist(s.center) {
			s.farthest = i
		}
	}
	for _, g := range s.segs {
		s.sizes = append(s.sizes, segmentLength(g))
	}
	slices.Sort(s.sizes)
	for _, c := range s.circles {
		s.sizes = append(s.sizes, c.Radius)
	}
	slices.Sort(s.sizes[len(s.segs):])
	s.grid = newPointGrid[int](EndpointTolerance)
	for i, g := range s.segs {
		s.grid.add(segmentMidpoint(g), i)
	}
	for i, c := range s.circles {
		s.grid.add(c.Center, len(s.segs)+i)
	}
	return s
}

// comparable reports whether s and o have as many pieces of each kind, of the same sizes, so they may be the same shape.
func (s *shape) comparable(o *shape) bool {
	if len(s.points) == 0 || len(s.segs) != len(o.segs) || len(s.circles) != len(o.circles) {
		return false
	}
	for i, v := range s.sizes {
		if math.Abs(v-o.sizes[i]) > EndpointTolerance {
			return false
		}
	}
	return true
}

// matches reports whether every piece of a, placed by pl, is a piece of s; both have as many pieces (they are comparable), and each piece is used once.
func (s *shape) matches(a *shape, pl Placement) bool {
	used := make([]bool, len(s.segs)+len(s.circles))
	find := func(at Point, ok func(int) bool) bool {
		i, found := s.grid.first(at, func(i int) bool { return !used[i] && ok(i) })
		if found {
			used[i] = true
		}
		return found
	}
	for _, g := range a.segs {
		start, end := pl.Apply(g.Start), pl.Apply(g.End)
		mid := pl.Apply(segmentMidpoint(g))
		if !find(mid, func(i int) bool {
			if i >= len(s.segs) {
				return false
			}
			h := s.segs[i]
			switch {
			case h.Start.Near(start) && h.End.Near(end):
				return math.Abs(h.Bulge-g.Bulge) <= 1e-6
			case h.Start.Near(end) && h.End.Near(start):
				return math.Abs(h.Bulge+g.Bulge) <= 1e-6
			}
			return false
		}) {
			return false
		}
	}
	for _, c := range a.circles {
		center := pl.Apply(c.Center)
		if !find(center, func(i int) bool {
			if i < len(s.segs) {
				return false
			}
			h := s.circles[i-len(s.segs)]
			return h.Center.Near(center) && math.Abs(h.Radius-c.Radius) <= EndpointTolerance
		}) {
			return false
		}
	}
	return true
}
