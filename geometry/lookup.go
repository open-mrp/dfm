package geometry

import (
	"math"
	"slices"
)

func (p *Part) rootOf(i int) int {
	for p.Loops[i].Parent >= 0 {
		i = p.Loops[i].Parent
	}
	return i
}

// Clearance is the distance from point c to the nearest cut other than the hole drawn by entities self: the part's edge, cut-outs and other holes.
func (p *Part) Clearance(c Point, self []string) float64 {
	if p.ownerIDs == nil {
		for _, k := range p.Contours {
			p.ownerIDs = append(p.ownerIDs, k.EntityIDs())
		}
		for _, k := range p.Circles {
			p.ownerIDs = append(p.ownerIDs, k.EntityIDs)
		}
	}
	mine := func(owner int) bool {
		for _, id := range p.ownerIDs[owner] {
			if slices.Contains(self, id) {
				return true
			}
		}
		return false
	}
	best := math.Inf(1)
	// A small part is scanned; a large one (thousands of cuts) is indexed by position, since building the index costs more than a few scans.
	n := len(p.Circles)
	for _, k := range p.Contours {
		n += len(k.Segments)
	}
	if n < clearanceIndexMin {
		for ci, k := range p.Contours {
			if mine(ci) {
				continue
			}
			for _, s := range k.Segments {
				if d, _ := segmentPrimitive(s).pointDist(c); d < best {
					best = d
				}
			}
		}
		for ci, k := range p.Circles {
			if mine(len(p.Contours) + ci) {
				continue
			}
			if d := math.Abs(k.Center.Dist(c) - k.Radius); d < best {
				best = d
			}
		}
		return best
	}
	if p.elems == nil {
		p.elems = newElementIndex(elementsOf(p.Contours, p.Circles), 0)
	}
	// Search a growing box around c until the nearest element found is nearer than the box's reach, so nothing outside it can be nearer.
	reach := math.Max(p.Bounds.WidthMM(), p.Bounds.HeightMM()) + 1
	for r := p.elems.grid.cell; ; r *= 2 {
		box := Bounds{MinX: c.X - r, MinY: c.Y - r, MaxX: c.X + r, MaxY: c.Y + r}
		for _, i := range p.elems.near(box) {
			e := p.elems.elems[i]
			if mine(e.owner) {
				continue
			}
			if d, _ := e.prim.pointDist(c); d < best {
				best = d
			}
		}
		if best <= r-EndpointTolerance || r > reach {
			return best
		}
	}
}

// clearanceIndexMin is the number of cut elements from which Clearance queries go through an index.
const clearanceIndexMin = 4096

// EntityPoint returns a point on a cut entity of the part, or nil when the part has no cut path from it.
func (p *Part) EntityPoint(id string) *Point {
	if p.entityAt == nil {
		p.entityAt = map[string]Point{}
		for _, c := range p.Contours {
			for _, s := range c.Segments {
				if _, ok := p.entityAt[s.EntityID]; !ok {
					p.entityAt[s.EntityID] = segmentMidpoint(s)
				}
			}
		}
		for _, c := range p.Circles {
			for _, e := range c.EntityIDs {
				if _, ok := p.entityAt[e]; !ok {
					p.entityAt[e] = Point{c.Center.X + c.Radius, c.Center.Y}
				}
			}
		}
	}
	if q, ok := p.entityAt[id]; ok {
		return &q
	}
	return nil
}

// LoopPoint returns a point that represents loop i, for pointing at it: a circle's center, a contour's bounding-box center when that lies inside it, otherwise the contour's first point.
func (p *Part) LoopPoint(i int) Point {
	l := p.Loops[i]
	if l.Circle >= 0 {
		return p.Circles[l.Circle].Center
	}
	c := Point{(l.Bounds.MinX + l.Bounds.MaxX) / 2, (l.Bounds.MinY + l.Bounds.MaxY) / 2}
	if l.containsPoint(c) {
		return c
	}
	return p.Contours[l.Contour].Segments[0].Start
}

// LoopOf returns the index of the loop drawn by the entities ids (its first entity), or -1.
func (p *Part) LoopOf(ids []string) int {
	if len(ids) == 0 {
		return -1
	}
	if p.loopOf == nil {
		p.loopOf = map[string]int{}
		for i, l := range p.Loops {
			for _, id := range l.EntityIDs {
				if _, ok := p.loopOf[id]; !ok {
					p.loopOf[id] = i
				}
			}
		}
	}
	if i, ok := p.loopOf[ids[0]]; ok {
		return i
	}
	return -1
}

// Owns reports whether the part was built from the entity with this ID.
func (p *Part) Owns(id string) bool {
	if p.owned == nil {
		p.owned = map[string]bool{}
		for _, c := range p.Contours {
			for _, s := range c.Segments {
				p.owned[s.EntityID] = true
			}
		}
		for _, c := range p.Circles {
			for _, e := range c.EntityIDs {
				p.owned[e] = true
			}
		}
	}
	return p.owned[id]
}

// Covers reports whether q lies inside the part: strictly inside its outer boundary or, for an unclosed part, within its extent. With onEdge, q may also lie on the boundary (within EndpointTolerance of its extent).
func (p *Part) Covers(q Point, onEdge bool) bool {
	if onEdge {
		b := p.Bounds
		t := EndpointTolerance
		return q.X >= b.MinX-t && q.X <= b.MaxX+t && q.Y >= b.MinY-t && q.Y <= b.MaxY+t
	}
	if p.Unclosed {
		b := p.Bounds
		return q.X > b.MinX && q.X < b.MaxX && q.Y > b.MinY && q.Y < b.MaxY
	}
	for _, l := range p.Loops {
		if l.Depth == 0 && l.containsPoint(q) {
			return true
		}
	}
	return false
}
