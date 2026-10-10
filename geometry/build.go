package geometry

import (
	"math"
	"slices"
)

// NewPart is Build without options.
func NewPart(paths []Contour, circles []Circle) *Part {
	return Build(paths, circles, BuildOptions{})
}

// BuildOptions adjusts how Build normalizes paths.
type BuildOptions struct {
	// ClosureToleranceMM closes gaps between path ends up to this size with a straight bridge, after exact chaining. At or below EndpointTolerance nothing is bridged.
	ClosureToleranceMM float64
	// NearDuplicateMM drops a line whose ends both lie within this of the ends of an earlier straight edge, as an exact copy is dropped: the laser cuts both as one kerf (Check sets half the kerf width). 0 drops exact copies only.
	NearDuplicateMM float64
}

// Build normalizes raw cut paths into a Part. paths may be open or closed; circles are full CIRCLE primitives.
//
// The paths are cleaned first: overcuts (paths running on past their start over their own first stretch) are trimmed and closed; paths and circles that exactly repeat other geometry are reported as duplicates and dropped, so that one doubled line is not also reported as an overlap, an open contour or a hole; the remaining partial overlaps are detected; open paths are chained end-to-end into contours (in input order, so the result is deterministic), and with a closure tolerance, remaining gaps up to that size are bridged; paths that share stretches are resolved into outlines and common lines; corner loops are trimmed; and closed contours made entirely of co-circular arcs become circles.
//
// The cleaned paths are then placed: closed loops are arranged in a containment tree (see Part), cut-out contours are tested for slot shapes, paths that touch or cross are detected, and the gap keeping each open contour open is measured. A cut-out drawn as a faceted polygon with its corners on a circle is read as the round hole it stands for, and the paths are placed again with the circle in its stead.
func Build(paths []Contour, circles []Circle, opts BuildOptions) *Part {
	p := &Part{}
	p.Contours, p.Circles = clean(p, paths, circles, opts)
	place(p)
	// A round hole drawn as a polygon is read as the circle it stands for, so every check measures the hole the designer drew.
	var holes []Circle
	drop := map[int]bool{}
	for i, c := range p.Contours {
		curves := facetedCurves(c)
		p.FacetedCurves = append(p.FacetedCurves, curves...)
		if h, ok := facetedHole(c, curves); ok {
			holes = append(holes, h)
			drop[i] = true
		}
	}
	if len(holes) > 0 {
		var contours []Contour
		for i, c := range p.Contours {
			if !drop[i] {
				contours = append(contours, c)
			}
		}
		p.Contours, p.Circles = contours, append(p.Circles, holes...)
		place(p)
	}
	return p
}

// clean repairs and normalizes raw paths (see Build), recording what it repaired in p's Report, and returns the contours and circles to place.
func clean(p *Part, paths []Contour, circles []Circle, opts BuildOptions) ([]Contour, []Circle) {
	var nonEmpty []Contour
	for _, c := range paths {
		if len(c.Segments) > 0 {
			nonEmpty = append(nonEmpty, c)
		}
	}
	for i, c := range nonEmpty {
		if closed, o, ok := trimOvercut(c); ok {
			nonEmpty[i] = closed
			p.Overcuts = append(p.Overcuts, o)
		}
	}
	nonEmpty, circles, p.Duplicates = dropDuplicates(nonEmpty, circles)
	if opts.NearDuplicateMM > EndpointTolerance {
		var near []Duplicate
		nonEmpty, near = dropNearDuplicates(nonEmpty, opts.NearDuplicateMM)
		p.Duplicates = append(p.Duplicates, near...)
	}
	var stubs []Duplicate
	nonEmpty, stubs = dropStubs(nonEmpty, math.Max(EndpointTolerance, opts.ClosureToleranceMM))
	p.Duplicates = append(p.Duplicates, stubs...)
	p.Overlaps, _ = findDefects(nonEmpty, circles, false)

	var contours, open []Contour
	for _, c := range nonEmpty {
		if c.Closed {
			contours = append(contours, c)
		} else {
			open = append(open, c)
		}
	}
	chained := chain(open)
	if opts.ClosureToleranceMM > EndpointTolerance {
		chained, p.ClosedGaps = closeGaps(chained, opts.ClosureToleranceMM)
	}
	contours = append(contours, chained...)
	contours, p.CommonLines = resolveSharedEdges(contours, EndpointTolerance)
	for i, c := range contours {
		var loops CornerLoops
		if contours[i], loops = trimCornerLoops(c); loops.Count > 0 {
			p.CornerLoops = append(p.CornerLoops, loops)
		}
	}

	// Promote arc-only closed loops to circles.
	kept := contours[:0]
	for _, c := range contours {
		if circ, ok := asCircle(c); ok {
			circles = append(circles, circ)
			continue
		}
		kept = append(kept, c)
	}
	return kept, circles
}

// place arranges a part's cut paths (see Build): its bounds, the containment tree, the slots among its cut-outs, the contacts between its paths and the gap of each open contour. It runs on cleaned paths, and again whenever a path is replaced: a faceted polygon read as a circle, a hole resized for its operation.
func place(p *Part) {
	p.Bounds = EmptyBounds()
	for _, c := range p.Contours {
		p.Bounds.Union(c.Bounds())
	}
	for _, c := range p.Circles {
		p.Bounds.Union(c.Bounds())
	}
	buildContainment(p)
	p.Slots = nil
	for i, c := range p.Contours {
		if c.Closed && c.Depth%2 == 1 {
			if s, ok := asSlot(c); ok {
				s.Contour = i
				p.Slots = append(p.Slots, s)
			}
		}
	}
	_, p.Contacts = findDefects(p.Contours, p.Circles, true)
	if len(p.CommonLines) > 0 {
		// Parts meeting along a common line are reported as that.
		contacts := p.Contacts[:0]
		for _, c := range p.Contacts {
			if !onCommonLine(p.CommonLines, c.Location) {
				contacts = append(contacts, c)
			}
		}
		p.Contacts = contacts
	}
	p.Openings = openings(p.Contours)
}

// dropDuplicates removes paths that exactly repeat geometry drawn elsewhere: a single segment that repeats a segment of a longer path (whichever comes first) or an earlier single segment, a longer path whose every segment repeats an earlier one, and a circle that repeats an earlier circle. Each removal is returned as a Duplicate. Candidates are found through a grid keyed by segment midpoint / circle center.
func dropDuplicates(paths []Contour, circles []Circle) ([]Contour, []Circle, []Duplicate) {
	var dups []Duplicate
	dropped := make([]bool, len(paths))
	single := func(c Contour) bool { return !c.Closed && len(c.Segments) == 1 }

	// Longer paths first, so a single line repeating one of their edges is the copy whatever the drawing order.
	long := newPointGrid[Segment](EndpointTolerance)
	for i, c := range paths {
		if single(c) {
			continue
		}
		var ids []string
		repeat := true
		for _, s := range c.Segments {
			o, ok := long.first(segmentMidpoint(s), func(o Segment) bool { return isDuplicate(o, s) })
			if !ok {
				repeat = false
				break
			}
			ids = appendUnique(ids, o.EntityID)
		}
		if repeat {
			dropped[i] = true
			dups = append(dups, Duplicate{Copies: c.EntityIDs(), Kept: ids, Location: segmentMidpoint(c.Segments[0])})
			continue
		}
		for _, s := range c.Segments {
			long.add(segmentMidpoint(s), s)
		}
	}
	singles := newPointGrid[Segment](EndpointTolerance)
	for i, c := range paths {
		if !single(c) {
			continue
		}
		s := c.Segments[0]
		mid := segmentMidpoint(s)
		match := func(o Segment) bool { return isDuplicate(o, s) }
		o, ok := long.first(mid, match)
		if !ok {
			o, ok = singles.first(mid, match)
		}
		if ok {
			dropped[i] = true
			dups = append(dups, Duplicate{Copies: []string{s.EntityID}, Kept: []string{o.EntityID}, Location: mid})
			continue
		}
		singles.add(mid, s)
	}
	var keptPaths []Contour
	for i, c := range paths {
		if !dropped[i] {
			keptPaths = append(keptPaths, c)
		}
	}

	centers := newPointGrid[Circle](EndpointTolerance)
	var keptCircles []Circle
	for _, c := range circles {
		if o, ok := centers.first(c.Center, func(o Circle) bool { return sameCircle(o, c) }); ok {
			dups = append(dups, Duplicate{Copies: slices.Clone(c.EntityIDs), Kept: slices.Clone(o.EntityIDs), Location: c.Center})
			continue
		}
		keptCircles = append(keptCircles, c)
		centers.add(c.Center, c)
	}
	return keptPaths, keptCircles, dups
}

// chain joins open contours whose endpoints coincide. When several pieces could extend a chain, the one earliest in input order wins, which keeps the output deterministic for a given input. Endpoints are indexed in a spatial hash so chaining is linear in the number of pieces.
func chain(pieces []Contour) []Contour {
	idx := newEndpointIndex(pieces)
	used := make([]bool, len(pieces))
	var out []Contour
	for i := range pieces {
		if used[i] {
			continue
		}
		used[i] = true
		segs := append([]Segment(nil), pieces[i].Segments...)
		closed := false
		for {
			head, tail := segs[0].Start, segs[len(segs)-1].End
			if len(segs) > 1 && tail.Near(head) {
				closed = true
				break
			}
			if j, rev := idx.find(pieces, used, tail, true); j >= 0 {
				used[j] = true
				segs = append(segs, oriented(pieces[j].Segments, rev)...)
				continue
			}
			if j, rev := idx.find(pieces, used, head, false); j >= 0 {
				used[j] = true
				segs = append(oriented(pieces[j].Segments, rev), segs...)
				continue
			}
			break
		}
		out = append(out, Contour{Segments: segs, Closed: closed})
	}
	return out
}

func oriented(segs []Segment, rev bool) []Segment {
	if rev {
		return reversed(segs)
	}
	return append([]Segment(nil), segs...)
}

// endpointIndex files pieces by their endpoints in cells of EndpointTolerance, so a piece with an endpoint at a point is found among the 3 by 3 cells around it.
type endpointIndex struct{ grid pointGrid[int] }

func newEndpointIndex(pieces []Contour) endpointIndex {
	idx := endpointIndex{newPointGrid[int](EndpointTolerance)}
	for i, pc := range pieces {
		s, e := pc.Segments[0].Start, pc.Segments[len(pc.Segments)-1].End
		idx.grid.add(s, i)
		if idx.grid.key(e) != idx.grid.key(s) {
			idx.grid.add(e, i)
		}
	}
	return idx
}

// find returns the lowest-index unused piece with an endpoint at p. When appending at the tail (atTail) a piece starting at p is used as-is; when prepending at the head a piece ending at p is used as-is. rev reports whether the piece must be reversed.
func (idx endpointIndex) find(pieces []Contour, used []bool, p Point, atTail bool) (int, bool) {
	best, bestRev := -1, false
	idx.grid.around(p, func(j int) bool {
		if used[j] || (best >= 0 && j >= best) {
			return false
		}
		s, e := pieces[j].Segments[0].Start, pieces[j].Segments[len(pieces[j].Segments)-1].End
		fwd, back := s, e
		if !atTail {
			fwd, back = e, s
		}
		switch {
		case fwd.Near(p):
			best, bestRev = j, false
		case back.Near(p):
			best, bestRev = j, true
		}
		return false
	})
	return best, bestRev
}

func reversed(segs []Segment) []Segment {
	out := make([]Segment, len(segs))
	for i, s := range segs {
		out[len(segs)-1-i] = s.Reverse()
	}
	return out
}

// asCircle recognizes a closed contour made only of arcs sharing one center and radius whose sweeps total a full turn.
func asCircle(c Contour) (Circle, bool) {
	if !c.Closed || len(c.Segments) == 0 {
		return Circle{}, false
	}
	first := c.Segments[0]
	if !first.IsArc() {
		return Circle{}, false
	}
	center, radius := first.Center(), first.Radius()
	total := 0.0
	for _, s := range c.Segments {
		if !s.IsArc() || !s.Center().Near(center) || math.Abs(s.Radius()-radius) > EndpointTolerance {
			return Circle{}, false
		}
		total += s.Sweep()
	}
	if math.Abs(math.Abs(total)-2*math.Pi) > 1e-6 {
		return Circle{}, false
	}
	return Circle{Center: center, Radius: radius, EntityIDs: c.EntityIDs()}, true
}

// angleTolerance is the tolerance (radians) used for parallel/perpendicular and semicircle tests in slot recognition.
const angleTolerance = 1e-6

// asSlot recognizes the two simple slot shapes:
//
//   - obround: line, 180° arc, line, 180° arc (any rotation, any start point);
//   - rectangular: four lines meeting at right angles.
//
// Anything else (keyholes, slots with chamfered ends, slots broken into extra collinear segments, ...) is not recognized.
func asSlot(c Contour) (Slot, bool) {
	segs := c.Segments
	if len(segs) != 4 {
		return Slot{}, false
	}
	ids := c.EntityIDs()

	if allLines(segs) {
		for i := range segs {
			if !perpendicular(segs[i], segs[(i+1)%4]) {
				return Slot{}, false
			}
		}
		a, b := segs[0].Chord(), segs[1].Chord()
		return Slot{Shape: SlotRectangular, WidthMM: math.Min(a, b), LengthMM: math.Max(a, b), EntityIDs: ids}, true
	}

	// Rotate so the sequence starts with a line.
	if segs[0].IsArc() {
		segs = append(segs[1:len(segs):len(segs)], segs[0])
	}
	l1, a1, l2, a2 := segs[0], segs[1], segs[2], segs[3]
	if l1.IsArc() || l2.IsArc() || !a1.IsArc() || !a2.IsArc() {
		return Slot{}, false
	}
	if !semicircle(a1) || !semicircle(a2) || math.Signbit(a1.Bulge) != math.Signbit(a2.Bulge) {
		return Slot{}, false
	}
	if math.Abs(a1.Radius()-a2.Radius()) > EndpointTolerance || !parallel(l1, l2) ||
		math.Abs(l1.Chord()-l2.Chord()) > EndpointTolerance {
		return Slot{}, false
	}
	width := a1.Chord()
	return Slot{Shape: SlotObround, WidthMM: width, LengthMM: l1.Chord() + width, EntityIDs: ids}, true
}

func allLines(segs []Segment) bool {
	for _, s := range segs {
		if s.IsArc() {
			return false
		}
	}
	return true
}

func semicircle(s Segment) bool {
	return math.Abs(math.Abs(s.Bulge)-1) < angleTolerance
}

func direction(s Segment) (float64, float64) {
	d := s.Chord()
	return (s.End.X - s.Start.X) / d, (s.End.Y - s.Start.Y) / d
}

func perpendicular(a, b Segment) bool {
	ax, ay := direction(a)
	bx, by := direction(b)
	return math.Abs(ax*bx+ay*by) < angleTolerance
}

func parallel(a, b Segment) bool {
	ax, ay := direction(a)
	bx, by := direction(b)
	return math.Abs(ax*by-ay*bx) < angleTolerance
}

// trimOvercut recognizes an overcut: an open path that comes back to its start and then runs on over its own first stretch, a laser technique to make sure a cut closes. It returns the closed path without the repeat. The repeat must follow the path exactly: whole segments, then part of one at most.
func trimOvercut(c Contour) (Contour, Overcut, bool) {
	segs := c.Segments
	if c.Closed || len(segs) < 3 {
		return c, Overcut{}, false
	}
	start := segs[0].Start
	k := -1
	for i := 1; i < len(segs)-1; i++ {
		if segs[i].End.Near(start) {
			k = i + 1
			break
		}
	}
	if k < 0 || len(segs)-k > k {
		return c, Overcut{}, false
	}
	tail := segs[k:]
	length := 0.0
	for j, t := range tail {
		o := segs[j]
		if !t.Start.Near(o.Start) || t.IsArc() != o.IsArc() || math.Abs(t.Bulge-o.Bulge) > 1e-9 && j < len(tail)-1 {
			return c, Overcut{}, false
		}
		if j < len(tail)-1 {
			if !t.End.Near(o.End) {
				return c, Overcut{}, false
			}
		} else {
			// The last piece may stop part way along the stretch it repeats.
			if d, _ := segmentPrimitive(o).pointDist(t.End); d > EndpointTolerance {
				return c, Overcut{}, false
			}
			if t.IsArc() && (t.Bulge > 0) != (o.Bulge > 0) {
				return c, Overcut{}, false
			}
		}
		length += segmentLength(t)
	}
	closed := Contour{Segments: append([]Segment(nil), segs[:k]...), Closed: true}
	closed.Segments[k-1].End = start
	mid := segmentMidpoint(tail[0])
	return closed, Overcut{EntityIDs: c.EntityIDs(), LengthMM: length, Location: mid}, true
}

// maxCornerLoopMM is the largest corner loop recognized: about a kerf across. A larger loop would cut away material the checks cannot see.
const maxCornerLoopMM = 0.5

// trimCornerLoops removes corner loops: runs of a path that leave a point and return to it, staying within maxCornerLoopMM, while the rest of the path goes on. Such a loop touches its own path, so left in it would read as a self-intersection.
func trimCornerLoops(c Contour) (Contour, CornerLoops) {
	segs := c.Segments
	var out []Segment
	var loops CornerLoops
	for i := 0; i < len(segs); i++ {
		j, size := cornerLoopEnd(segs, i)
		if j < 0 {
			out = append(out, segs[i])
			continue
		}
		if loops.Count == 0 {
			loops = CornerLoops{EntityIDs: c.EntityIDs(), Location: segs[i].Start}
		}
		loops.Count++
		loops.SizeMM = math.Max(loops.SizeMM, size)
		if j+1 < len(segs) {
			segs[j+1].Start = segs[i].Start
		} else if len(out) > 0 {
			out[len(out)-1].End = segs[i].Start
		}
		i = j
	}
	if loops.Count == 0 {
		return c, loops
	}
	return Contour{Segments: out, Closed: c.Closed}, loops
}

// cornerLoopEnd returns the index of the last segment of a corner loop starting at segs[i], and its size, or -1. The rest of the path must be longer than the loop could be, so a small closed path is never taken for a loop of itself.
func cornerLoopEnd(segs []Segment, i int) (int, float64) {
	start := segs[i].Start
	b := EmptyBounds()
	b.Add(start)
	for j := i; j < len(segs); j++ {
		b.Union(segs[j].Bounds())
		size := math.Max(b.WidthMM(), b.HeightMM())
		if size > maxCornerLoopMM {
			return -1, 0
		}
		if j > i && segs[j].End.Near(start) && size > EndpointTolerance {
			rest := 0.0
			for k, s := range segs {
				if k < i || k > j {
					rest += segmentLength(s)
				}
			}
			if rest <= 4*maxCornerLoopMM {
				return -1, 0
			}
			return j, size
		}
	}
	return -1, 0
}

// segmentLength is a segment's length along it.
func segmentLength(s Segment) float64 {
	if !s.IsArc() {
		return s.Chord()
	}
	return s.Radius() * math.Abs(s.Sweep())
}

// dropNearDuplicates drops each single straight line whose ends both lie within tol of the ends of an earlier straight edge (of a longer path, or another line), keeping the first.
func dropNearDuplicates(paths []Contour, tol float64) ([]Contour, []Duplicate) {
	grid := newPointGrid[Segment](tol)
	add := func(s Segment) { grid.add(segmentMidpoint(s), s) }
	single := func(c Contour) bool { return !c.Closed && len(c.Segments) == 1 && !c.Segments[0].IsArc() }
	for _, c := range paths {
		if !single(c) {
			for _, s := range c.Segments {
				if !s.IsArc() {
					add(s)
				}
			}
		}
	}
	var dups []Duplicate
	var kept []Contour
	for _, c := range paths {
		if !single(c) {
			kept = append(kept, c)
			continue
		}
		s := c.Segments[0]
		mid := segmentMidpoint(s)
		var match *Segment
		gap := 0.0
		grid.around(mid, func(o Segment) bool {
			if o.EntityID == s.EntityID {
				return false
			}
			d1 := max(s.Start.Dist(o.Start), s.End.Dist(o.End))
			d2 := max(s.Start.Dist(o.End), s.End.Dist(o.Start))
			if d := min(d1, d2); d <= tol && s.Chord() > tol {
				match, gap = &o, d
				return true
			}
			return false
		})
		if match != nil {
			dups = append(dups, Duplicate{Copies: []string{s.EntityID}, Kept: []string{match.EntityID}, Location: mid, GapMM: gap})
			continue
		}
		add(s)
		kept = append(kept, c)
	}
	return kept, dups
}
