package geometry

import (
	"math"
	"slices"
)

// cellKey is the cell of p in a grid of square cells cell wide from the origin.
func cellKey(p Point, cell float64) [2]int64 {
	return [2]int64{int64(math.Floor(p.X / cell)), int64(math.Floor(p.Y / cell))}
}

// cellOf is the cell of p in a grid of EndpointTolerance cells, the grid coincident points are found in.
func cellOf(p Point) [2]int64 { return cellKey(p, EndpointTolerance) }

// pointGrid buckets values by the cell of a point, cells cell wide, so the values near a point are those in the 3 by 3 cells around its own: every value within cell of it, and none farther than two cells.
type pointGrid[T any] struct {
	cell  float64
	cells map[[2]int64][]T
}

func newPointGrid[T any](cell float64) pointGrid[T] {
	return pointGrid[T]{cell: cell, cells: map[[2]int64][]T{}}
}

func (g pointGrid[T]) key(p Point) [2]int64 { return cellKey(p, g.cell) }

// add files v under the cell of p.
func (g pointGrid[T]) add(p Point, v T) {
	k := g.key(p)
	g.cells[k] = append(g.cells[k], v)
}

// around calls visit with every value in the 3 by 3 cells around p, cell by cell in the order the values were added, until visit returns true, and reports whether one did.
func (g pointGrid[T]) around(p Point, visit func(T) bool) bool {
	k := g.key(p)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for _, v := range g.cells[[2]int64{k[0] + dx, k[1] + dy}] {
				if visit(v) {
					return true
				}
			}
		}
	}
	return false
}

// first returns the first value around p that match accepts.
func (g pointGrid[T]) first(p Point, match func(T) bool) (T, bool) {
	var found T
	ok := g.around(p, func(v T) bool {
		if match(v) {
			found = v
			return true
		}
		return false
	})
	return found, ok
}

// boxGrid buckets items, numbered from 0, by the cells their bounds cover, cells cell wide from origin, so a query for a box visits each item in the cells it touches once.
type boxGrid struct {
	cell   float64
	origin Point
	cells  map[[2]int][]int32
	stamp  []int // per item, the last query that returned it
	query  int
}

// newBoxGrid makes a grid for n items.
func newBoxGrid(cell float64, origin Point, n int) *boxGrid {
	return &boxGrid{cell: cell, origin: origin, cells: map[[2]int][]int32{}, stamp: make([]int, n)}
}

// cellAt is the cell holding p.
func (g *boxGrid) cellAt(p Point) [2]int {
	return [2]int{int(math.Floor((p.X - g.origin.X) / g.cell)), int(math.Floor((p.Y - g.origin.Y) / g.cell))}
}

// span is the range of cells b covers once grown by grow on every side.
func (g *boxGrid) span(b Bounds, grow float64) (x0, y0, x1, y1 int) {
	lo, hi := g.cellAt(Point{b.MinX - grow, b.MinY - grow}), g.cellAt(Point{b.MaxX + grow, b.MaxY + grow})
	return lo[0], lo[1], hi[0], hi[1]
}

// add files item i under every cell b covers, grown by grow.
func (g *boxGrid) add(i int, b Bounds, grow float64) {
	x0, y0, x1, y1 := g.span(b, grow)
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			g.addCell(i, [2]int{x, y})
		}
	}
}

// addCell files item i under one cell.
func (g *boxGrid) addCell(i int, k [2]int) { g.cells[k] = append(g.cells[k], int32(i)) }

// within returns the items filed under the cells b covers, grown by grow, each once and in item order, so a caller visits them as a plain scan would.
func (g *boxGrid) within(b Bounds, grow float64) []int32 {
	x0, y0, x1, y1 := g.span(b, grow)
	return g.collect(x0, y0, x1, y1)
}

// around returns the items filed under the 3 by 3 cells around k, each once and in item order.
func (g *boxGrid) around(k [2]int) []int32 {
	return g.collect(k[0]-1, k[1]-1, k[0]+1, k[1]+1)
}

func (g *boxGrid) collect(x0, y0, x1, y1 int) []int32 {
	g.query++
	var out []int32
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			for _, i := range g.cells[[2]int{x, y}] {
				if g.stamp[i] != g.query {
					g.stamp[i] = g.query
					out = append(out, i)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}
