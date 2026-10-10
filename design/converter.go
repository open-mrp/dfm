package design

import (
	"math"

	"github.com/open-mrp/dfm/geometry"
)

// The converter is the format-neutral core of the readers: the DXF and SVG readers resolve each format's entities to lines, arcs and circles in drawing coordinates and hand them to it, and it places them in world millimeters and accumulates the part's geometry and the report that goes with it.

// affine maps drawing coordinates (before unit scaling) into the world: x' = a·x + b·y + tx, y' = c·x + d·y + ty. Model-space entities use the identity; entities inside a block reference use the reference's placement.
type affine struct{ a, b, c, d, tx, ty float64 }

var identity = affine{a: 1, d: 1}

// Every product is rounded with float64(...), which stops the compiler fusing it with the addition (it fuses on arm64, not on amd64), so a drawing gives the same coordinates, and the same result, on every architecture.
func (m affine) apply(x, y float64) (float64, float64) {
	return float64(m.a*x) + float64(m.b*y) + m.tx, float64(m.c*x) + float64(m.d*y) + m.ty
}

func (m affine) linear(x, y float64) (float64, float64) {
	return float64(m.a*x) + float64(m.b*y), float64(m.c*x) + float64(m.d*y)
}

// then returns the transform that applies n first, then m.
func (m affine) then(n affine) affine {
	return affine{
		a: float64(m.a*n.a) + float64(m.b*n.c), b: float64(m.a*n.b) + float64(m.b*n.d),
		c: float64(m.c*n.a) + float64(m.d*n.c), d: float64(m.c*n.b) + float64(m.d*n.d),
		tx: float64(m.a*n.tx) + float64(m.b*n.ty) + m.tx, ty: float64(m.c*n.tx) + float64(m.d*n.ty) + m.ty,
	}
}

func (m affine) det() float64 { return m.a*m.d - m.b*m.c }

// similar reports whether m keeps shapes (rotation, uniform scale, mirror), so circles stay circles; scale is then its scale factor.
func (m affine) similar() (scale float64, ok bool) {
	sx, sy := math.Hypot(m.a, m.c), math.Hypot(m.b, m.d)
	dot := m.a*m.b + m.c*m.d
	return sx, math.Abs(sx-sy) <= 1e-9*max(sx, sy) && math.Abs(dot) <= 1e-9*sx*sy
}

func translate(x, y float64) affine { return affine{a: 1, d: 1, tx: x, ty: y} }

func scaling(x, y float64) affine { return affine{a: x, d: y} }

func rotation(rad float64) affine {
	s, c := math.Sin(rad), math.Cos(rad)
	return affine{a: c, b: -s, c: s, d: c}
}

// converter accumulates normalized geometry from the entities a reader resolves.
type converter struct {
	scale      float64
	paths      []geometry.Contour
	circles    []geometry.Circle
	zeroLength []string                  // entities with a zero-length segment, in file order
	zeroAt     map[string]geometry.Point // where, when known
	texts      []geometry.Text
	nonCut     []geometry.NonCut
	// xf places the entities being converted: the identity in model space, a block reference's placement inside it.
	xf affine
	// expanded and expandedSegments count the entities produced by block references so far, and the segments they converted to.
	expanded, expandedSegments int
	// zOff and zScale place heights the way xf places X and Y; heights records each cut entity's lowest and highest Z, in millimeters.
	zOff, zScale float64
	heights      []height
}

// height is the range of Z an entity spans.
type height struct {
	id       string
	low, top float64
}

// noteZ records that entity id lies at height z, given in its object coordinate system (ocsZ) or in world coordinates.
func (c *converter) noteZ(id string, o ocs, ocsZ bool, zs ...float64) {
	h := height{id: id, low: math.Inf(1), top: math.Inf(-1)}
	for _, z := range zs {
		if ocsZ && o.mirror {
			z = -z
		}
		z = (c.zOff + float64(c.zScale*z)) * c.scale
		h.low, h.top = math.Min(h.low, z), math.Max(h.top, z)
	}
	if n := len(c.heights); n > 0 && c.heights[n-1].id == id {
		c.heights[n-1].low = math.Min(c.heights[n-1].low, h.low)
		c.heights[n-1].top = math.Max(c.heights[n-1].top, h.top)
		return
	}
	c.heights = append(c.heights, h)
}

// offPlane returns the entities that are not on the drawing's plane: the height most entities share (the lowest, on a tie). An entity is off it when any part of it is further than geometry.EndpointTolerance away.
func offPlane(heights []height) []geometry.OffPlane {
	count := map[float64]int{}
	for _, h := range heights {
		count[math.Round(h.low*1e6)/1e6]++
	}
	plane, best := 0.0, -1
	for z, n := range count {
		if n > best || n == best && z < plane {
			plane, best = z, n
		}
	}
	var out []geometry.OffPlane
	for _, h := range heights {
		d := math.Max(math.Abs(h.low-plane), math.Abs(h.top-plane))
		if d > geometry.EndpointTolerance {
			out = append(out, geometry.OffPlane{EntityID: h.id, DistanceMM: d})
		}
	}
	return out
}

// first returns the first point of the converted geometry, or nil if there is none.
func (c *converter) first() *geometry.Point {
	switch {
	case len(c.paths) > 0:
		p := c.paths[0].Segments[0].Start
		return &p
	case len(c.circles) > 0:
		p := c.circles[0].Center
		return &p
	}
	return nil
}

func (c *converter) noteZeroLength(id string, at ...geometry.Point) {
	if len(at) > 0 {
		if c.zeroAt == nil {
			c.zeroAt = map[string]geometry.Point{}
		}
		if _, ok := c.zeroAt[id]; !ok {
			c.zeroAt[id] = at[0]
		}
	}
	if n := len(c.zeroLength); n == 0 || c.zeroLength[n-1] != id {
		c.zeroLength = append(c.zeroLength, id)
	}
}

// wpt maps a point in drawing coordinates (world, or a block's own) to world millimeters.
func (c *converter) wpt(x, y float64) geometry.Point {
	x, y = c.xf.apply(x, y)
	return geometry.Point{X: x * c.scale, Y: y * c.scale}
}

// vec maps a direction in drawing coordinates to world millimeters.
func (c *converter) vec(x, y float64) (float64, float64) {
	x, y = c.xf.linear(x, y)
	return x * c.scale, y * c.scale
}

// seg makes a segment from world points, with the bulge turned over when the entity was drawn mirrored (its own coordinate system, or the placement it is drawn with, flips it) and ok false when the piece is too short to keep.
func (c *converter) seg(mirrored bool, start, end geometry.Point, bulge float64, id string) (geometry.Segment, bool) {
	if mirrored != (c.xf.det() < 0) {
		bulge = -bulge
	}
	s := geometry.Segment{Start: start, End: end, Bulge: bulge, EntityID: id}
	// A piece shorter than the endpoint tolerance cannot be told apart from a point: both its ends join the same neighbor. An arc of half a turn or more is long whatever its chord.
	return s, start.Dist(end) > geometry.EndpointTolerance || math.Abs(bulge) >= 1
}

// addCurve adds fitted segments as one contour, closed when its ends meet.
func (c *converter) addCurve(segs []geometry.Segment) {
	if len(segs) == 0 {
		return
	}
	closed := len(segs) > 1 && segs[0].Start.Near(segs[len(segs)-1].End)
	if closed {
		segs[len(segs)-1].End = segs[0].Start
	}
	c.paths = append(c.paths, geometry.Contour{Segments: segs, Closed: closed})
}
