package geometry

import (
	"math"
	"sort"
)

// elementIndex files elements by the cells their bounds cover, so a short segment is only tested against elements near it.
type elementIndex struct {
	elems []element
	grid  *boxGrid
}

// newElementIndex builds the index with cells about radius across (a neck is at most radius long), but no smaller than 1/200 of the drawing, so a large circle does not fill thousands of cells.
func newElementIndex(elems []element, radius float64) *elementIndex {
	all := EmptyBounds()
	for _, e := range elems {
		all.Add(Point{e.bounds.MinX, e.bounds.MinY})
		all.Add(Point{e.bounds.MaxX, e.bounds.MaxY})
	}
	cell := math.Max(radius, math.Max(all.WidthMM(), all.HeightMM())/200)
	if cell <= 0 || math.IsInf(cell, 0) || math.IsNaN(cell) {
		cell = 1
	}
	idx := &elementIndex{elems: elems, grid: newBoxGrid(cell, Point{all.MinX, all.MinY}, len(elems))}
	for i, e := range elems {
		idx.grid.add(i, e.bounds, EndpointTolerance)
	}
	return idx
}

// near returns the elements whose bounds come within EndpointTolerance of b, each once.
func (idx *elementIndex) near(b Bounds) []int32 {
	var out []int32
	for _, i := range idx.grid.within(b, EndpointTolerance) {
		if boundsGap(b, idx.elems[i].bounds) <= EndpointTolerance {
			out = append(out, i)
		}
	}
	return out
}

// element is one primitive with its owning path, for pairwise tests.
type element struct {
	prim     primitive
	bounds   Bounds
	entityID string
	owner    int // index into the owner list (contours then circles)
	index    int // position within its contour
	ownerLen int // number of segments in the owner contour (0 for circles)
	closed   bool
}

func elementsOf(contours []Contour, circles []Circle) []element {
	var out []element
	for ci, c := range contours {
		for si, s := range c.Segments {
			out = append(out, element{
				prim: segmentPrimitive(s), bounds: s.Bounds(), entityID: s.EntityID,
				owner: ci, index: si, ownerLen: len(c.Segments), closed: c.Closed,
			})
		}
	}
	for i, c := range circles {
		id := ""
		if len(c.EntityIDs) > 0 {
			id = c.EntityIDs[0]
		}
		out = append(out, element{prim: circlePrimitive(c), bounds: c.Bounds(), entityID: id, owner: len(contours) + i})
	}
	return out
}

// candidatePairs returns index pairs whose bounds, grown by pad, overlap. It sorts by MinX and sweeps, which is near-linear for typical drawings.
func candidatePairs(boxes []Bounds, pad float64) [][2]int {
	order := make([]int, len(boxes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return boxes[order[a]].MinX < boxes[order[b]].MinX })
	var pairs [][2]int
	for x, i := range order {
		bi := boxes[i]
		for _, j := range order[x+1:] {
			bj := boxes[j]
			if bj.MinX > bi.MaxX+pad {
				break
			}
			if bj.MinY <= bi.MaxY+pad && bj.MaxY >= bi.MinY-pad {
				a, b := i, j
				if a > b {
					a, b = b, a
				}
				pairs = append(pairs, [2]int{a, b})
			}
		}
	}
	sort.Slice(pairs, func(a, b int) bool {
		if pairs[a][0] != pairs[b][0] {
			return pairs[a][0] < pairs[b][0]
		}
		return pairs[a][1] < pairs[b][1]
	})
	return pairs
}
