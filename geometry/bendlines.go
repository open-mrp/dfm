package geometry

import "math"

// minEndLoopWidth is the narrowest loop a bend line may end on: the part's outline, a relief or a cut-out, not a sliver drawn as a doubled line.
const minEndLoopWidth = 0.1

// SpansMaterial reports whether the straight line from a to b runs through the part's material from one cut to another, as a bend line does: both ends lie on closed cuts at least minEndLoopWidth wide (within tol), the
// line runs into material from each end, and it never leaves the part,
// though it may cross cut-outs (a bend through a window). Paths drawn by entities self are left out, so an open path can be tested against the rest of the part.
func (p *Part) SpansMaterial(a, b Point, self []string, tol float64) bool {
	length := a.Dist(b)
	// Points this far from the ends are clear of the cuts the ends sit on.
	margin := max(100*tol, 0.1)
	if length <= 4*margin {
		return false
	}
	if p.Clearance(a, self) > tol || p.Clearance(b, self) > tol || !p.endsOnLoop(a, tol) || !p.endsOnLoop(b, tol) {
		return false
	}
	ux, uy := (b.X-a.X)/length, (b.Y-a.Y)/length
	at := func(t float64) Point { return Point{a.X + float64(t*ux), a.Y + float64(t*uy)} }
	// The line is checked at points at most 0.5 mm apart (at most 2000).
	n := int(math.Min(2000, math.Ceil((length-2*margin)/0.5)))
	for i := 0; i <= n; i++ {
		q := at(margin + (length-2*margin)*float64(i)/float64(n))
		enclosing, inPart := 0, false
		for _, l := range p.Loops {
			if l.containsPoint(q) {
				enclosing++
				inPart = inPart || l.Depth == 0
			}
		}
		if !inPart {
			return false
		}
		// Material is inside an odd number of loops: an outline, not one of its cut-outs.
		if (i == 0 || i == n) && enclosing%2 == 0 {
			return false
		}
	}
	return true
}

// endsOnLoop reports whether q lies on a loop (within tol) that is at least minEndLoopWidth wide, taking twice its area over its perimeter as its width.
func (p *Part) endsOnLoop(q Point, tol float64) bool {
	for _, l := range p.Loops {
		perimeter := l.along[len(l.along)-1]
		if perimeter == 0 || 2*l.AreaMM2/perimeter < minEndLoopWidth {
			continue
		}
		for _, pr := range l.prims {
			if d, _ := pr.pointDist(q); d <= tol {
				return true
			}
		}
	}
	return false
}

// StraightEnds returns the ends of c when it is one straight line: every segment straight and on the line through its first start and last end.
func StraightEnds(c Contour) (a, b Point, ok bool) {
	if c.Closed || len(c.Segments) == 0 {
		return Point{}, Point{}, false
	}
	a, b = c.Segments[0].Start, c.Segments[len(c.Segments)-1].End
	length := a.Dist(b)
	if length == 0 {
		return Point{}, Point{}, false
	}
	for _, s := range c.Segments {
		if s.IsArc() {
			return Point{}, Point{}, false
		}
		for _, q := range []Point{s.Start, s.End} {
			if d := math.Abs(cross(b.X-a.X, b.Y-a.Y, q.X-a.X, q.Y-a.Y)) / length; d > EndpointTolerance {
				return Point{}, Point{}, false
			}
		}
	}
	return a, b, true
}
