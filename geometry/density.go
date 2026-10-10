package geometry

import (
	"math"
)

// DensestWindow slides a square window size wide across the part whose outer loop is root, a quarter of its size at a time, and returns the largest share of the part's area inside a window that cut-outs remove (islands left in cut-outs count as material), with that window's center. Windows less than half covered by the part are skipped, so a part smaller than the window has none. Curves are measured as polygons with points at most 10° apart.
func (p *Part) DensestWindow(root int, size float64) (fraction float64, center Point, ok bool) {
	b := p.Loops[root].Bounds
	type member struct {
		poly []Point
		sign float64
		b    Bounds
	}
	// The outline first, then everything inside it.
	tree := []member{{loopPolygon(p.Loops[root]), 1, b}}
	for i, l := range p.Loops {
		if i == root || p.rootOf(i) != root {
			continue
		}
		sign := 1.0
		if (l.Depth-p.Loops[root].Depth)%2 == 1 {
			sign = -1
		}
		tree = append(tree, member{loopPolygon(l), sign, l.Bounds})
	}
	step := size / 4
	best := -1.0
	for y := b.MinY - size/2; y <= b.MaxY-size/2+1e-9; y += step {
		for x := b.MinX - size/2; x <= b.MaxX-size/2+1e-9; x += step {
			w := Bounds{MinX: x, MinY: y, MaxX: x + size, MaxY: y + size}
			outline := clippedArea(tree[0].poly, w)
			if outline < size*size/2 {
				continue
			}
			material := outline
			for _, m := range tree[1:] {
				if m.b.MaxX <= w.MinX || m.b.MinX >= w.MaxX || m.b.MaxY <= w.MinY || m.b.MinY >= w.MaxY {
					continue
				}
				material += m.sign * clippedArea(m.poly, w)
			}
			if f := (outline - material) / outline; f > best {
				best, center = f, Point{x + size/2, y + size/2}
			}
		}
	}
	if best < 0 {
		return 0, Point{}, false
	}
	return math.Max(0, best), center, true
}
