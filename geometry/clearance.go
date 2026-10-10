package geometry

import (
	"math"
	"sort"
)

// Clearance is the shortest distance between the boundaries of two loops.
type Clearance struct {
	// A and B index Part.Loops.
	A, B       int
	DistanceMM float64
	// PointA and PointB are the closest points on A and B.
	PointA, PointB Point
}

// Midpoint is the center of the gap between the loops.
func (c Clearance) Midpoint() Point {
	return Point{(c.PointA.X + c.PointB.X) / 2, (c.PointA.Y + c.PointB.Y) / 2}
}

func boundsGap(a, b Bounds) float64 {
	dx := math.Max(0, math.Max(a.MinX-b.MaxX, b.MinX-a.MaxX))
	dy := math.Max(0, math.Max(a.MinY-b.MaxY, b.MinY-a.MaxY))
	return math.Hypot(dx, dy)
}

// LoopClearance returns the exact boundary distance between loops a and b when it is at most radius; ok is false when the loops are farther apart.
func (p *Part) LoopClearance(a, b int, radius float64) (Clearance, bool) {
	la, lb := p.Loops[a], p.Loops[b]
	best := Clearance{A: a, B: b, DistanceMM: math.Inf(1)}
	// A large loop (an outline with thousands of pieces) is queried through a grid of its primitives rather than piece by piece.
	var grid *boxGrid
	if len(lb.prims) >= primGridMin {
		grid = p.primGrid(b)
	}
	for i, pa := range la.prims {
		n := len(lb.prims)
		var cands []int32
		if grid != nil {
			cands = grid.within(la.primBounds[i], math.Min(radius, best.DistanceMM))
			n = len(cands)
		}
		for k := 0; k < n; k++ {
			j := k
			if grid != nil {
				j = int(cands[k])
			}
			if boundsGap(la.primBounds[i], lb.primBounds[j]) > math.Min(radius, best.DistanceMM) {
				continue
			}
			d, ptA, ptB := primitiveDistance(pa, lb.prims[j])
			d = math.Max(d, curveDistance(la, i, lb, j))
			if d < best.DistanceMM {
				best.DistanceMM, best.PointA, best.PointB = d, ptA, ptB
			}
		}
	}
	return best, best.DistanceMM <= radius
}

// primGridMin is the loop size from which clearance queries go through a grid.
const primGridMin = 64

// primGrid returns loop i's primitives filed by the cells their bounds cover, built on first use.
func (p *Part) primGrid(i int) *boxGrid {
	if g, ok := p.primGrids[i]; ok {
		return g
	}
	l := p.Loops[i]
	n := len(l.prims)
	cell := math.Max(l.Bounds.WidthMM(), l.Bounds.HeightMM()) / math.Sqrt(float64(n))
	if !(cell > 0) {
		cell = 1
	}
	g := newBoxGrid(cell, Point{l.Bounds.MinX, l.Bounds.MinY}, n)
	for j, b := range l.primBounds {
		g.add(j, b, 0)
	}
	if p.primGrids == nil {
		p.primGrids = map[int]*boxGrid{}
	}
	p.primGrids[i] = g
	return g
}

// clearanceCache holds SiblingClearances for the largest radius asked so far; a smaller radius takes the subset within it.
type clearanceCache struct {
	radius float64
	out    []Clearance
}

// SiblingClearances returns the clearance of every pair of cut-outs that lie in the same material region (share a parent) and are at most radius apart, ordered by loop index.
func (p *Part) SiblingClearances(radius float64) []Clearance {
	if c := p.siblings; c != nil && radius <= c.radius {
		out := make([]Clearance, 0, len(c.out))
		for _, cl := range c.out {
			if cl.DistanceMM <= radius {
				out = append(out, cl)
			}
		}
		return out
	}
	out := p.siblingClearances(radius)
	p.siblings = &clearanceCache{radius: radius, out: out}
	return out
}

func (p *Part) siblingClearances(radius float64) []Clearance {
	var cutouts []int
	for i, l := range p.Loops {
		if l.IsCutout() {
			cutouts = append(cutouts, i)
		}
	}
	sort.SliceStable(cutouts, func(a, b int) bool { return p.Loops[cutouts[a]].Bounds.MinX < p.Loops[cutouts[b]].Bounds.MinX })
	var out []Clearance
	for x, i := range cutouts {
		bi := p.Loops[i].Bounds
		for _, j := range cutouts[x+1:] {
			bj := p.Loops[j].Bounds
			if bj.MinX > bi.MaxX+radius {
				break
			}
			if p.Loops[i].Parent != p.Loops[j].Parent || boundsGap(bi, bj) > radius {
				continue
			}
			a, b := min(i, j), max(i, j)
			if c, ok := p.LoopClearance(a, b, radius); ok {
				out = append(out, c)
			}
		}
	}
	sort.Slice(out, func(x, y int) bool {
		if out[x].A != out[y].A {
			return out[x].A < out[y].A
		}
		return out[x].B < out[y].B
	})
	return out
}
