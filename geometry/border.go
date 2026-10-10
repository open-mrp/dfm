package geometry

import "math"

// A sheet border's band (between its two rectangles) is at most maxBorderBand of the frame's shorter side, and it holds at least minBorderContents separate shapes: a frame part with one plate nested in its window is a part, not a border.
const (
	maxBorderBand     = 0.1
	minBorderContents = 2
)

// FindBorder returns the entities of a sheet border drawn on the cut layer: two axis-aligned rectangles, one just inside the other, around every other path of the drawing, with nothing in the band between them and at least minBorderContents shapes inside. at is the outer rectangle's corner.
func (p *Part) FindBorder() (ids []string, at Point, ok bool) {
	outer, inner := -1, -1
	for i, l := range p.Loops {
		if l.Depth == 0 {
			if outer >= 0 {
				return nil, Point{}, false
			}
			outer = i
		}
	}
	if outer < 0 || len(p.Contours) == 0 {
		return nil, Point{}, false
	}
	children := 0
	for i, l := range p.Loops {
		if l.Parent == outer {
			children++
			inner = i
		}
	}
	if children > 1 {
		return p.singleBorder(outer)
	}
	contents := 0
	for _, l := range p.Loops {
		if inner >= 0 && l.Parent == inner {
			contents++
		}
	}
	if inner < 0 || contents < minBorderContents || !p.isRectangle(outer) || !p.isRectangle(inner) {
		return nil, Point{}, false
	}
	// Every open path lies inside the frame too.
	o, n := p.Loops[outer].Bounds, p.Loops[inner].Bounds
	for _, c := range p.Contours {
		if !c.Closed && !boundsContain(n, c.Bounds()) {
			return nil, Point{}, false
		}
	}
	band := math.Max(math.Max(n.MinX-o.MinX, o.MaxX-n.MaxX), math.Max(n.MinY-o.MinY, o.MaxY-n.MaxY))
	if band > maxBorderBand*math.Min(o.WidthMM(), o.HeightMM()) {
		return nil, Point{}, false
	}
	ids = append(append(ids, p.Loops[outer].EntityIDs...), p.Loops[inner].EntityIDs...)
	return ids, Point{o.MinX, o.MinY}, true
}

// singleBorder reports whether the one outline, outer, is a sheet border drawn as a single rectangle around nested parts: an axis-aligned rectangle holding at least minBorderContents cut-outs that each hold islands (the parts, with their holes), nothing nested deeper, and every open path inside. A plate's cut-outs do not all hold loose islands; a frame with plates nested in its windows has their holes nested deeper.
func (p *Part) singleBorder(outer int) (ids []string, at Point, ok bool) {
	if !p.isRectangle(outer) {
		return nil, Point{}, false
	}
	holds := map[int]int{}
	contents := 0
	for _, l := range p.Loops {
		switch l.Depth {
		case 1:
			contents++
		case 2:
			holds[l.Parent]++
		case 3:
			return nil, Point{}, false
		}
	}
	for i, l := range p.Loops {
		if l.Depth == 1 && holds[i] == 0 {
			return nil, Point{}, false
		}
	}
	if contents < minBorderContents {
		return nil, Point{}, false
	}
	o := p.Loops[outer].Bounds
	for _, c := range p.Contours {
		if !c.Closed && !boundsContain(o, c.Bounds()) {
			return nil, Point{}, false
		}
	}
	return p.Loops[outer].EntityIDs, Point{o.MinX, o.MinY}, true
}

// isRectangle reports whether loop i is a contour of straight pieces that make an axis-aligned rectangle (collinear pieces count as one side).
func (p *Part) isRectangle(i int) bool {
	l := p.Loops[i]
	if l.Contour < 0 {
		return false
	}
	b := l.Bounds
	for _, s := range p.Contours[l.Contour].Segments {
		if s.IsArc() {
			return false
		}
		// Every piece runs along one of the bounding box's sides.
		onX := func(q Point, x float64) bool { return math.Abs(q.X-x) <= EndpointTolerance }
		onY := func(q Point, y float64) bool { return math.Abs(q.Y-y) <= EndpointTolerance }
		side := (onX(s.Start, b.MinX) && onX(s.End, b.MinX)) || (onX(s.Start, b.MaxX) && onX(s.End, b.MaxX)) ||
			(onY(s.Start, b.MinY) && onY(s.End, b.MinY)) || (onY(s.Start, b.MaxY) && onY(s.End, b.MaxY))
		if !side {
			return false
		}
	}
	return b.WidthMM() > 0 && b.HeightMM() > 0
}
