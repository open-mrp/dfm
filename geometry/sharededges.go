package geometry

import (
	"math"
	"sort"
)

// CommonLine is a cut line two parts share (common-line cutting): one cut separates them. LengthMM is its length, Location its middle.
type CommonLine struct {
	EntityIDs []string
	LengthMM  float64
	Location  Point
	// Segments are the shared stretches, for matching contacts along them.
	Segments []Segment
}

// resolveSharedEdges reads drawings whose parts share cut lines: where an open path ends part way along another path, or three or more path ends meet, the paths involved are split into a network of lines and every region the network encloses becomes a closed outline. A line between two such regions is a common line. Lines that enclose nothing (dangling ends and bridges between outlines) stay open paths. Contours without such junctions are returned unchanged.
func resolveSharedEdges(contours []Contour, tol float64) ([]Contour, []CommonLine) {
	var openIdx []int
	for i, c := range contours {
		if !c.Closed && len(c.Segments) > 0 {
			openIdx = append(openIdx, i)
		}
	}
	if len(openIdx) == 0 {
		return contours, nil
	}
	idx := newSegIndex(contours, tol)
	// A line drawn across a closed outline (a bend line, a stray) divides no parts: only paths outside every closed outline take part.
	var outlines []*bandIndex
	for _, c := range contours {
		if c.Closed {
			var m []monotone
			for _, s := range c.Segments {
				m = append(m, yMonotonePieces(s)...)
			}
			outlines = append(outlines, newBandIndex(m))
		}
	}
	inside := map[int]bool{}
	for _, i := range openIdx {
		q := segmentMidpoint(contours[i].Segments[len(contours[i].Segments)/2])
		for _, o := range outlines {
			if o.contains(q) {
				inside[i] = true
				break
			}
		}
	}
	// splits[i][j] are the points where path ends land inside segment j of contour i.
	splits := map[int]map[int][]Point{}
	// touched are the contours an open path's end meets at a corner (a vertex that is not their own end).
	touched := map[int]bool{}
	ends := map[[2]int64]int{}
	for _, i := range openIdx {
		if inside[i] {
			continue
		}
		c := contours[i]
		for k, e := range []Point{c.Segments[0].Start, c.Segments[len(c.Segments)-1].End} {
			ends[snapKey(e, tol)]++
			for _, r := range idx.near(e) {
				s := contours[r.path].Segments[r.seg]
				if e.Dist(s.Start) <= tol || e.Dist(s.End) <= tol {
					o := contours[r.path]
					ownEnd := r.path == i && ((k == 0 && r.seg == 0 && e.Dist(s.Start) <= tol) || (k == 1 && r.seg == len(o.Segments)-1 && e.Dist(s.End) <= tol))
					isEnd := !o.Closed && ((r.seg == 0 && e.Dist(s.Start) <= tol) || (r.seg == len(o.Segments)-1 && e.Dist(s.End) <= tol))
					if !ownEnd && !isEnd && !inside[r.path] {
						touched[r.path] = true
					}
					continue
				}
				if d, _ := segmentPrimitive(s).pointDist(e); d <= tol {
					if splits[r.path] == nil {
						splits[r.path] = map[int][]Point{}
					}
					splits[r.path][r.seg] = append(splits[r.path][r.seg], e)
				}
			}
		}
	}
	junction := len(splits) > 0 || len(touched) > 0
	for _, n := range ends {
		junction = junction || n >= 3
	}
	if !junction {
		return contours, nil
	}

	involved := map[int]bool{}
	for _, i := range openIdx {
		involved[i] = !inside[i]
	}
	for i := range splits {
		involved[i] = true
	}
	for i := range touched {
		involved[i] = true
	}
	// Split every involved segment wherever an involved path's end lies along it, so a stretch two paths both draw becomes one edge.
	for i := range contours {
		if !involved[i] {
			continue
		}
		for _, s := range contours[i].Segments {
			for _, e := range []Point{s.Start, s.End} {
				for _, r := range idx.near(e) {
					if !involved[r.path] {
						continue
					}
					t := contours[r.path].Segments[r.seg]
					if e.Dist(t.Start) <= tol || e.Dist(t.End) <= tol {
						continue
					}
					if d, _ := segmentPrimitive(t).pointDist(e); d <= tol {
						if splits[r.path] == nil {
							splits[r.path] = map[int][]Point{}
						}
						splits[r.path][r.seg] = append(splits[r.path][r.seg], e)
					}
				}
			}
		}
	}
	g := newPlanarGraph(tol)
	for i := range contours {
		if !involved[i] {
			continue
		}
		for j, s := range contours[i].Segments {
			for _, piece := range splitSegment(s, splits[i][j]) {
				g.addEdge(piece)
			}
		}
	}
	faces, leftover, shared := g.faces()
	var out []Contour
	for i, c := range contours {
		if !involved[i] {
			out = append(out, c)
		}
	}
	for _, f := range faces {
		out = append(out, Contour{Segments: mergePieces(f), Closed: true})
	}
	var pieces []Contour
	for _, s := range leftover {
		pieces = append(pieces, Contour{Segments: []Segment{s}})
	}
	out = append(out, chain(pieces)...)

	// One common line per entity drawn along shared edges.
	var lines []CommonLine
	at := map[string]int{}
	for _, s := range shared {
		i, ok := at[s.EntityID]
		if !ok {
			i = len(lines)
			at[s.EntityID] = i
			lines = append(lines, CommonLine{EntityIDs: []string{s.EntityID}, Location: segmentMidpoint(s)})
		}
		lines[i].LengthMM += segmentLength(s)
		lines[i].Segments = append(lines[i].Segments, s)
	}
	return out, lines
}

// splitSegment cuts s at the given points on it, in order along it.
func splitSegment(s Segment, at []Point) []Segment {
	if len(at) == 0 {
		return []Segment{s}
	}
	p := segmentPrimitive(s)
	type cut struct {
		t float64
		q Point
	}
	var cuts []cut
	for _, q := range at {
		cuts = append(cuts, cut{p.along(q), q})
	}
	sort.Slice(cuts, func(a, b int) bool { return cuts[a].t < cuts[b].t })
	var out []Segment
	start, from := s.Start, 0.0
	for _, c := range append(cuts, cut{p.length(), s.End}) {
		if c.t-from <= 0 || start.Near(c.q) {
			continue
		}
		piece := Segment{Start: start, End: c.q, EntityID: s.EntityID}
		if !p.line {
			piece.Bulge = math.Tan((c.t - from) / p.r / 4)
			if p.sweep < 0 {
				piece.Bulge = -piece.Bulge
			}
		}
		out = append(out, piece)
		start, from = c.q, c.t
	}
	return out
}

// mergePieces joins consecutive pieces of one entity that continue one another (collinear lines, arcs of one circle), undoing splits that turned out not to be junctions of a closed outline.
func mergePieces(segs []Segment) []Segment {
	var out []Segment
	for _, s := range segs {
		if n := len(out); n > 0 {
			last := out[n-1]
			if last.EntityID == s.EntityID && continues(last, s) {
				merged := Segment{Start: last.Start, End: s.End, EntityID: s.EntityID}
				if last.IsArc() {
					sweep := last.Sweep() + s.Sweep()
					merged.Bulge = math.Tan(sweep / 4)
				}
				if math.Abs(merged.Sweep()) < 2*math.Pi-1e-9 && !merged.Start.Near(merged.End) {
					out[n-1] = merged
					continue
				}
			}
		}
		out = append(out, s)
	}
	return out
}

// continues reports whether b carries on a without a corner: two lines in one direction, or two arcs of one circle turning the same way.
func continues(a, b Segment) bool {
	if a.IsArc() != b.IsArc() {
		return false
	}
	if !a.IsArc() {
		ax, ay := direction(a)
		bx, by := direction(b)
		return math.Abs(ax*by-ay*bx) < 1e-9 && ax*bx+ay*by > 0
	}
	return (a.Bulge > 0) == (b.Bulge > 0) && a.Center().Near(b.Center()) && math.Abs(a.Radius()-b.Radius()) <= EndpointTolerance
}

// tangentTie is how close, in radians, two pieces' directions where they leave a point must be to count as one direction.
const tangentTie = 1e-3

// planarGraph is a network of segments joined at shared end points.
type planarGraph struct {
	tol    float64
	nodes  []Point
	at     pointGrid[int] // nodes by position
	edges  []graphEdge
	byEnds map[[2]int][]int // edges by their nodes, lower first
}

type graphEdge struct {
	u, v int
	seg  Segment // from node u to node v
}

func newPlanarGraph(tol float64) *planarGraph {
	return &planarGraph{tol: tol, at: newPointGrid[int](snapCell(tol)), byEnds: map[[2]int][]int{}}
}

// snapCell is the cell size that files points within tol of each other in the same or neighboring cells.
func snapCell(tol float64) float64 { return math.Max(tol, EndpointTolerance) * 2 }

func snapKey(p Point, tol float64) [2]int64 { return cellKey(p, snapCell(tol)) }

// node returns the node within tol of p, adding one when there is none.
func (g *planarGraph) node(p Point) int {
	if n, ok := g.at.first(p, func(n int) bool { return g.nodes[n].Dist(p) <= g.tol }); ok {
		return n
	}
	g.nodes = append(g.nodes, p)
	g.at.add(p, len(g.nodes)-1)
	return len(g.nodes) - 1
}

func (g *planarGraph) addEdge(s Segment) {
	u, v := g.node(s.Start), g.node(s.End)
	if u == v {
		return
	}
	// A stretch drawn twice (reported as an overlap) is one edge.
	for _, e := range g.byEnds[[2]int{min(u, v), max(u, v)}] {
		o := g.edges[e].seg
		if e := g.edges[e]; e.u != u {
			o = o.Reverse()
		}
		if math.Abs(o.Bulge-s.Bulge) <= 1e-9 {
			return
		}
	}
	key := [2]int{min(u, v), max(u, v)}
	g.byEnds[key] = append(g.byEnds[key], len(g.edges))
	g.edges = append(g.edges, graphEdge{u, v, s})
}

// faces returns the boundary of every bounded region of the network, the edges that bound none (left over as open paths), and the edges that bound two regions (shared).
func (g *planarGraph) faces() (faces [][]Segment, leftover, shared []Segment) {
	alive := g.cyclicEdges()
	// Half-edge 2e runs u→v along edge e, 2e+1 runs back.
	out := make([][]int, len(g.nodes))
	angle := make([]float64, 2*len(g.edges))
	for e, ed := range g.edges {
		if !alive[e] {
			leftover = append(leftover, ed.seg)
			continue
		}
		fwd, back := startTangent(segmentPrimitive(ed.seg)), startTangent(segmentPrimitive(ed.seg.Reverse()))
		angle[2*e] = math.Atan2(fwd.Y, fwd.X)
		angle[2*e+1] = math.Atan2(back.Y, back.X)
		out[ed.u] = append(out[ed.u], 2*e)
		out[ed.v] = append(out[ed.v], 2*e+1)
	}
	pos := make([]int, 2*len(g.edges)) // index of each half-edge in its node's list
	for n := range out {
		sort.SliceStable(out[n], func(a, b int) bool {
			ha, hb := out[n][a], out[n][b]
			if math.Abs(wrapAngle(angle[ha]-angle[hb])) > tangentTie {
				return angle[ha] < angle[hb]
			}
			// Leaving in one direction (a fillet tangent to a line, to rounding in the file), a piece curving left lies counterclockwise of a straight one, one curving right clockwise.
			return curvature(g, ha) < curvature(g, hb)
		})
		for k, h := range out[n] {
			pos[h] = k
		}
	}
	head := func(h int) int {
		if h%2 == 0 {
			return g.edges[h/2].v
		}
		return g.edges[h/2].u
	}
	seg := func(h int) Segment {
		if h%2 == 0 {
			return g.edges[h/2].seg
		}
		return g.edges[h/2].seg.Reverse()
	}
	faceOf := make([]int, 2*len(g.edges))
	for i := range faceOf {
		faceOf[i] = -1
	}
	var bounded []bool
	for h := range faceOf {
		if !alive[h/2] || faceOf[h] >= 0 {
			continue
		}
		id := len(bounded)
		var loop []Segment
		for cur, steps := h, 0; faceOf[cur] < 0 && steps <= 2*len(g.edges); steps++ {
			faceOf[cur] = id
			loop = append(loop, seg(cur))
			// Next: at the head, the half-edge just clockwise of the way back.
			n, back := head(cur), cur^1
			list := out[n]
			cur = list[(pos[back]-1+len(list))%len(list)]
		}
		ok := signedArea(Contour{Segments: loop}) > EndpointTolerance*EndpointTolerance
		bounded = append(bounded, ok)
		if ok {
			faces = append(faces, loop)
		}
	}
	for e := range g.edges {
		if !alive[e] {
			continue
		}
		in := 0
		for _, h := range []int{2 * e, 2*e + 1} {
			if faceOf[h] >= 0 && bounded[faceOf[h]] {
				in++
			}
		}
		switch in {
		case 2:
			shared = append(shared, g.edges[e].seg)
		case 0:
			// Bounds no region: never lose it, keep it as an open path.
			leftover = append(leftover, g.edges[e].seg)
		}
	}
	return faces, leftover, shared
}

// cyclicEdges marks the edges that lie on a cycle: not dangling and not a bridge between two parts of the network.
func (g *planarGraph) cyclicEdges() []bool {
	adj := make([][]int, len(g.nodes)) // edge indexes
	for e, ed := range g.edges {
		adj[ed.u] = append(adj[ed.u], e)
		adj[ed.v] = append(adj[ed.v], e)
	}
	alive := make([]bool, len(g.edges))
	for e := range alive {
		alive[e] = true
	}
	disc := make([]int, len(g.nodes))
	low := make([]int, len(g.nodes))
	for i := range disc {
		disc[i] = -1
	}
	t := 0
	type frame struct{ n, via, next int }
	for root := range g.nodes {
		if disc[root] >= 0 {
			continue
		}
		disc[root], low[root] = t, t
		t++
		stack := []frame{{root, -1, 0}}
		for len(stack) > 0 {
			f := &stack[len(stack)-1]
			if f.next < len(adj[f.n]) {
				e := adj[f.n][f.next]
				f.next++
				if e == f.via {
					continue
				}
				m := g.edges[e].u
				if m == f.n {
					m = g.edges[e].v
				}
				if disc[m] < 0 {
					disc[m], low[m] = t, t
					t++
					stack = append(stack, frame{m, e, 0})
				} else {
					low[f.n] = min(low[f.n], disc[m])
				}
				continue
			}
			stack = stack[:len(stack)-1]
			if len(stack) > 0 {
				p := stack[len(stack)-1].n
				low[p] = min(low[p], low[f.n])
				if low[f.n] > disc[p] {
					alive[f.via] = false // a bridge
				}
			}
		}
	}
	return alive
}

// segIndex finds segments near a point: each is filed under the cells of points sampled along it.
type segIndex struct {
	grid *boxGrid
	refs []segRef
}

type segRef struct{ path, seg int }

func newSegIndex(paths []Contour, tol float64) *segIndex {
	b := EmptyBounds()
	total, n := 0.0, 0
	for _, c := range paths {
		b.Union(c.Bounds())
		n += len(c.Segments)
		for _, s := range c.Segments {
			total += segmentLength(s)
		}
	}
	cell := math.Max(1, math.Max(b.WidthMM(), b.HeightMM())/1024)
	cell = math.Max(cell, 4*tol)
	cell = math.Max(cell, 2*total/maxIndexSamples)
	ix := &segIndex{grid: newBoxGrid(cell, Point{}, n)}
	for i, c := range paths {
		for j, s := range c.Segments {
			p := segmentPrimitive(s)
			ref := len(ix.refs)
			ix.refs = append(ix.refs, segRef{i, j})
			n := int(math.Ceil(2*segmentLength(s)/cell)) + 1
			last := [2]int{math.MinInt, 0}
			for k := 0; k <= n; k++ {
				t := float64(k) / float64(n)
				var q Point
				if p.line {
					q = Point{s.Start.X + t*(s.End.X-s.Start.X), s.Start.Y + t*(s.End.Y-s.Start.Y)}
				} else {
					q = p.pointAt(p.start + p.sweep*t)
				}
				if k := ix.grid.cellAt(q); k != last {
					ix.grid.addCell(ref, k)
					last = k
				}
			}
		}
	}
	return ix
}

// near returns the segments filed around q, each once.
func (ix *segIndex) near(q Point) []segRef {
	var out []segRef
	for _, r := range ix.grid.around(ix.grid.cellAt(q)) {
		out = append(out, ix.refs[r])
	}
	return out
}

// onCommonLine reports whether q lies on one of the lines.
func onCommonLine(lines []CommonLine, q Point) bool {
	for _, l := range lines {
		for _, s := range l.Segments {
			if d, _ := segmentPrimitive(s).pointDist(q); d <= EndpointTolerance {
				return true
			}
		}
	}
	return false
}

// curvature is the signed curvature of half-edge h where it starts: positive when it turns left, 0 for a line.
func curvature(g *planarGraph, h int) float64 {
	s := g.edges[h/2].seg
	if h%2 == 1 {
		s = s.Reverse()
	}
	if !s.IsArc() {
		return 0
	}
	return math.Copysign(1/s.Radius(), s.Bulge)
}
