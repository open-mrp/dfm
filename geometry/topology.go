package geometry

import (
	"math"
	"sort"
)

func segmentMidpoint(s Segment) Point {
	if !s.IsArc() {
		return Point{(s.Start.X + s.End.X) / 2, (s.Start.Y + s.End.Y) / 2}
	}
	p := segmentPrimitive(s)
	return p.pointAt(p.start + p.sweep/2)
}

// bandIndex answers point-in-loop tests: the loop's y-monotone pieces, bucketed by the band of y they cover, so a test visits the pieces at the point's height instead of all of them. The bands overlap by the largest nudge contains makes, so a nudged point finds the same pieces.
type bandIndex struct {
	pieces []monotone
	bounds Bounds
	y0, h  float64
	bands  [][]int32
}

// bandOverlap covers the nudges contains makes and its grazing tolerance.
const bandOverlap = 1e-6

func newBandIndex(pieces []monotone) *bandIndex {
	b := EmptyBounds()
	for _, m := range pieces {
		b.Add(m.a)
		b.Add(m.b)
		if m.r != 0 {
			// An arc piece bulges past its ends, up to its radius.
			b.Add(Point{m.c.X - m.r, m.c.Y - m.r})
			b.Add(Point{m.c.X + m.r, m.c.Y + m.r})
		}
	}
	idx := &bandIndex{pieces: pieces, bounds: b}
	n := max(1, min(len(pieces)/4, 4096))
	h := (b.MaxY - b.MinY) / float64(n)
	if !(h > 0) {
		n, h = 1, 1
	}
	idx.y0, idx.h = b.MinY, h
	idx.bands = make([][]int32, n)
	for i, m := range pieces {
		lo, hi := min(m.a.Y, m.b.Y)-bandOverlap, max(m.a.Y, m.b.Y)+bandOverlap
		for k := idx.band(lo); k <= idx.band(hi); k++ {
			idx.bands[k] = append(idx.bands[k], int32(i))
		}
	}
	return idx
}

// band is the band holding y, clamped to the index.
func (idx *bandIndex) band(y float64) int {
	k := int(math.Floor((y - idx.y0) / idx.h))
	return max(0, min(k, len(idx.bands)-1))
}

// contains reports whether q is inside the closed boundary, by counting crossings of a ray from q towards +X over the pieces at q's height, after a bounds test (a point outside the loop's bounds is outside the loop). A vertex computed twice (once per adjoining arc) can land a hair either side of the ray and be counted twice or not at all, so a ray that grazes a vertex is moved off it first. The move is far below EndpointTolerance, and loops closer than that are reported as touching, so it never changes which loop q is in.
func (idx *bandIndex) contains(q Point) bool {
	b := idx.bounds
	if q.X < b.MinX-bandOverlap || q.X > b.MaxX+bandOverlap || q.Y < b.MinY-bandOverlap || q.Y > b.MaxY+bandOverlap {
		return false
	}
	const graze = 1e-9
	for _, nudge := range nudges {
		p := Point{q.X, q.Y + nudge}
		clear := true
		band := idx.bands[idx.band(p.Y)]
		for _, i := range band {
			m := idx.pieces[i]
			for _, e := range []Point{m.a, m.b} {
				if e.X > p.X-graze && math.Abs(e.Y-p.Y) <= graze {
					clear = false
				}
			}
		}
		if !clear {
			continue
		}
		inside := false
		for _, i := range band {
			if crossesRay(idx.pieces[i], p) {
				inside = !inside
			}
		}
		return inside
	}
	return false
}

// nudges are the offsets contains tries when a ray grazes an endpoint.
var nudges = []float64{0, 1.3e-7, -2.9e-7, 4.1e-7, -5.3e-7, 6.7e-7}

// monotone is a piece of a segment along which Y changes monotonically.
type monotone struct {
	a, b Point
	// For arc pieces: center, radius and which side of the center the piece lies on (+1 right, -1 left). Lines have r == 0.
	c    Point
	r    float64
	side float64
}

// yMonotonePieces splits an arc at its topmost/bottommost points.
func yMonotonePieces(s Segment) []monotone {
	return yMonotonePrimitive(segmentPrimitive(s))
}

func yMonotonePrimitive(p primitive) []monotone {
	if p.line {
		return []monotone{{a: p.a, b: p.b}}
	}
	start, sweep := p.start, p.sweep
	if sweep < 0 {
		start, sweep = start+sweep, -sweep
	}
	cuts := []float64{start}
	for _, ext := range []float64{math.Pi / 2, 3 * math.Pi / 2} {
		d := math.Mod(ext-start, 2*math.Pi)
		if d < 0 {
			d += 2 * math.Pi
		}
		if d > 0 && d < sweep {
			cuts = append(cuts, start+d)
		}
	}
	sort.Float64s(cuts)
	cuts = append(cuts, start+sweep)
	var out []monotone
	for i := 0; i+1 < len(cuts); i++ {
		mid := (cuts[i] + cuts[i+1]) / 2
		side := 1.0
		if math.Cos(mid) < 0 {
			side = -1
		}
		out = append(out, monotone{a: p.pointAt(cuts[i]), b: p.pointAt(cuts[i+1]), c: p.c, r: p.r, side: side})
	}
	return out
}

func crossesRay(m monotone, q Point) bool {
	if (m.a.Y > q.Y) == (m.b.Y > q.Y) {
		return false
	}
	var x float64
	if m.r == 0 {
		x = m.a.X + (q.Y-m.a.Y)*(m.b.X-m.a.X)/(m.b.Y-m.a.Y)
	} else {
		dy := q.Y - m.c.Y
		x = m.c.X + m.side*math.Sqrt(math.Max(0, m.r*m.r-dy*dy))
	}
	return x > q.X
}

// contourArea returns the absolute area enclosed by a closed contour: the polygon through its vertices plus the circular segment of each arc.
func contourArea(c Contour) float64 { return math.Abs(signedArea(c)) }

// signedArea is positive for a counter-clockwise contour.
func signedArea(c Contour) float64 {
	sum := 0.0
	for _, s := range c.Segments {
		sum += (float64(s.Start.X*s.End.Y) - float64(s.End.X*s.Start.Y)) / 2
		if s.IsArc() {
			th := s.Sweep()
			r := s.Radius()
			seg := float64(r*r) / 2 * (math.Abs(th) - math.Sin(math.Abs(th)))
			sum += math.Copysign(seg, th)
		}
	}
	return sum
}

// buildContainment assigns every closed contour and circle its nesting depth (0 for an outer material boundary, 1 for a cut-out inside it, 2 for a material island inside that cut-out, and so on; -1 for open contours) and records the tree in p.Loops.
func buildContainment(p *Part) {
	var loops []Loop
	for i := range p.Contours {
		if !p.Contours[i].Closed {
			p.Contours[i].Depth = -1
			continue
		}
		loops = append(loops, newLoop(p, i, -1))
	}
	for i := range p.Circles {
		loops = append(loops, newLoop(p, -1, i))
	}
	// Larger loops first, so every potential parent is placed before its children. The sort is stable to keep equal-area loops in input order.
	order := make([]int, len(loops))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return loops[order[a]].AreaMM2 > loops[order[b]].AreaMM2 })
	for oi, li := range order {
		l := &loops[li]
		l.Parent = -1
		probe := l.probe(p)
		// Walking back from the loop's place in the order, the first loop around it is the smallest, so its parent.
		for oj := oi - 1; oj >= 0; oj-- {
			cand := &loops[order[oj]]
			if !boundsContain(cand.Bounds, l.Bounds) || !cand.containsPoint(probe) {
				continue
			}
			l.Parent = order[oj]
			break
		}
		if l.Parent < 0 {
			l.Depth = 0
		} else {
			l.Depth = loops[l.Parent].Depth + 1
		}
		if l.Contour >= 0 {
			p.Contours[l.Contour].Depth = l.Depth
		} else {
			p.Circles[l.Circle].Depth = l.Depth
		}
	}
	p.Loops = loops
}

// newLoop makes the loop of contour ci or circle ci of p (the other -1), with everything the containment and distance queries need, before its place in the tree is known.
func newLoop(p *Part, ci, circle int) Loop {
	l := Loop{Contour: ci, Circle: circle}
	if ci >= 0 {
		c := p.Contours[ci]
		l.AreaMM2, l.Bounds, l.EntityIDs = contourArea(c), c.Bounds(), c.EntityIDs()
		l.ccw = signedArea(c) > 0
		for _, s := range c.Segments {
			l.prims = append(l.prims, segmentPrimitive(s))
			l.primBounds = append(l.primBounds, s.Bounds())
		}
	} else {
		c := p.Circles[circle]
		l.AreaMM2, l.Bounds, l.EntityIDs = math.Pi*c.Radius*c.Radius, c.Bounds(), c.EntityIDs
		l.prims = []primitive{circlePrimitive(c)}
		l.primBounds = []Bounds{c.Bounds()}
	}
	l.bands = newBandIndex(monotonePieces(l.prims))
	l.along = make([]float64, len(l.prims)+1)
	for k, pr := range l.prims {
		l.along[k+1] = l.along[k] + pr.length()
	}
	return l
}

// probe returns a point on the loop's boundary away from its vertices, used to test whether the loop lies inside another. Loops that touch or cross are reported separately, so for valid input any boundary point gives the same answer.
func (l Loop) probe(p *Part) Point {
	if l.Circle >= 0 {
		c := p.Circles[l.Circle]
		return Point{c.Center.X + c.Radius, c.Center.Y}
	}
	return segmentMidpoint(p.Contours[l.Contour].Segments[0])
}

func boundsContain(outer, inner Bounds) bool {
	const eps = EndpointTolerance
	return inner.MinX >= outer.MinX-eps && inner.MinY >= outer.MinY-eps &&
		inner.MaxX <= outer.MaxX+eps && inner.MaxY <= outer.MaxY+eps
}

// maxOutlineGapRatio is how open a loose path may be and still be read as a part's outline: the gaps keeping it open may add up to at most this share of its size (bounding-box diagonal). A stray line, whose gap is its whole length, never qualifies.
const maxOutlineGapRatio = 0.1

// Parts splits the drawing into parts. Each outer boundary (depth-0 loop) is a part, with the loops nested inside it and any open paths that lie inside it. A group of open paths outside every boundary that nearly closes (see maxOutlineGapRatio) is a part too, marked Unclosed, holding the boundaries inside its extent: they are its cut-outs, not parts of their own. Closed parts come first in Loops order, then unclosed ones in contour order. A drawing with at most one part is returned whole, so nothing is lost from it.
//
// With inCutouts, material inside a cut-out (an even-depth loop below the outer boundaries, which would otherwise be a loose island) is a part of its own too, nested in the cut-out (part-in-part).
func (p *Part) Parts(inCutouts bool) []*Part {
	if p.parts == nil {
		p.parts = map[bool][]*Part{}
	}
	if _, ok := p.parts[inCutouts]; !ok {
		p.parts[inCutouts] = p.split(inCutouts)
	}
	return p.parts[inCutouts]
}

func (p *Part) split(inCutouts bool) []*Part {
	var roots []int
	isRoot := map[int]bool{}
	for i, l := range p.Loops {
		if l.Depth == 0 || inCutouts && l.Depth%2 == 0 {
			roots = append(roots, i)
			isRoot[i] = true
		}
	}
	// nearestRoot is the part a loop belongs to: itself if it is a root, else its closest enclosing root.
	nearestRoot := func(i int) int {
		for !isRoot[i] {
			i = p.Loops[i].Parent
		}
		return i
	}
	// Open paths inside a boundary belong to that boundary's part; the rest are grouped by touching extents and may form outlines.
	insideRoot := map[int]int{}
	var outside []int
	for i, c := range p.Contours {
		if c.Closed {
			continue
		}
		probe := segmentMidpoint(c.Segments[0])
		owner := -1
		for _, r := range roots {
			if p.Loops[r].containsPoint(probe) && (owner < 0 || p.Loops[r].AreaMM2 < p.Loops[owner].AreaMM2) {
				owner = r // the innermost part containing it
			}
		}
		if owner >= 0 {
			insideRoot[i] = owner
		} else {
			outside = append(outside, i)
		}
	}
	gapOf := map[int]float64{}
	for _, o := range p.Openings {
		gapOf[o.Contour] = o.GapMM
	}
	type outline struct {
		contours []int
		bounds   Bounds
	}
	var outlines []outline
	for _, g := range groupByExtent(p.Contours, outside) {
		b, gaps := EmptyBounds(), 0.0
		for _, c := range g {
			b.Union(p.Contours[c].Bounds())
			gaps += gapOf[c]
		}
		diag := math.Hypot(b.WidthMM(), b.HeightMM())
		if b.Area() > 0 && gaps <= maxOutlineGapRatio*diag {
			outlines = append(outlines, outline{contours: g, bounds: b})
		}
	}
	// A boundary inside an outline's extent is one of its cut-outs.
	absorbedBy := map[int]int{}
	for _, r := range roots {
		if p.Loops[r].Depth > 0 {
			continue // nested parts stay with their own part
		}
		best := -1
		for k, o := range outlines {
			if boundsContain(o.bounds, p.Loops[r].Bounds) && (best < 0 || o.bounds.Area() < outlines[best].bounds.Area()) {
				best = k
			}
		}
		if best >= 0 {
			absorbedBy[r] = best
		}
	}
	var closedRoots []int
	for _, r := range roots {
		if _, ok := absorbedBy[r]; !ok {
			closedRoots = append(closedRoots, r)
		}
	}

	if len(closedRoots)+len(outlines) < 2 {
		switch {
		case len(closedRoots) == 1:
			p.Outline = p.Loops[closedRoots[0]].EntityIDs
		case len(outlines) == 1:
			p.Outline = contourIDs(p.Contours, outlines[0].contours)
			p.Unclosed = true
		}
		return []*Part{p}
	}

	n := len(closedRoots) + len(outlines)
	contours := make([][]Contour, n)
	circles := make([][]Circle, n)
	partOfRoot := map[int]int{}
	for k, r := range closedRoots {
		partOfRoot[r] = k
	}
	for r, o := range absorbedBy {
		partOfRoot[r] = len(closedRoots) + o
	}
	for i, l := range p.Loops {
		k := partOfRoot[nearestRoot(i)]
		if l.Contour >= 0 {
			contours[k] = append(contours[k], p.Contours[l.Contour])
		} else {
			circles[k] = append(circles[k], p.Circles[l.Circle])
		}
	}
	for i := range p.Contours {
		if r, ok := insideRoot[i]; ok {
			k := partOfRoot[r]
			contours[k] = append(contours[k], p.Contours[i])
		}
	}
	for k, o := range outlines {
		for _, c := range o.contours {
			contours[len(closedRoots)+k] = append(contours[len(closedRoots)+k], p.Contours[c])
		}
	}
	parts := make([]*Part, n)
	for k := range parts {
		parts[k] = Build(contours[k], circles[k], BuildOptions{})
		for _, op := range p.HoleOps {
			if parts[k].Owns(op.EntityIDs[0]) {
				parts[k].HoleOps = append(parts[k].HoleOps, op)
			}
		}
		if k < len(closedRoots) {
			parts[k].Outline = p.Loops[closedRoots[k]].EntityIDs
		} else {
			parts[k].Outline = contourIDs(p.Contours, outlines[k-len(closedRoots)].contours)
			parts[k].Unclosed = true
		}
	}
	return parts
}

// groupByExtent groups the listed contours whose bounding boxes touch, directly or through others, each group in contour order.
func groupByExtent(cs []Contour, idx []int) [][]int {
	parent := make([]int, len(idx))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	bounds := make([]Bounds, len(idx))
	for i, c := range idx {
		bounds[i] = cs[c].Bounds()
	}
	for _, pr := range candidatePairs(bounds, EndpointTolerance) {
		if a, b := find(pr[0]), find(pr[1]); a != b {
			parent[max(a, b)] = min(a, b)
		}
	}
	byRoot := map[int][]int{}
	var order []int
	for i, c := range idx {
		r := find(i)
		if _, ok := byRoot[r]; !ok {
			order = append(order, r)
		}
		byRoot[r] = append(byRoot[r], c)
	}
	out := make([][]int, 0, len(order))
	for _, r := range order {
		out = append(out, byRoot[r])
	}
	return out
}

func contourIDs(cs []Contour, idx []int) []string {
	var ids []string
	for _, i := range idx {
		ids = appendUnique(ids, cs[i].EntityIDs()...)
	}
	return ids
}
