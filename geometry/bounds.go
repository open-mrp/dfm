package geometry

import (
	"math"
)

// Bounds is an axis-aligned bounding box.
type Bounds struct {
	MinX, MinY, MaxX, MaxY float64
}

// EmptyBounds returns a bounds value that contains nothing; adding any point to it yields that point's bounds.
func EmptyBounds() Bounds {
	return Bounds{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
}

// IsEmpty reports whether no points have been added.
func (b Bounds) IsEmpty() bool { return b.MinX > b.MaxX }

// Add grows b to include p.
func (b *Bounds) Add(p Point) {
	b.MinX = math.Min(b.MinX, p.X)
	b.MinY = math.Min(b.MinY, p.Y)
	b.MaxX = math.Max(b.MaxX, p.X)
	b.MaxY = math.Max(b.MaxY, p.Y)
}

// Union grows b to include o.
func (b *Bounds) Union(o Bounds) {
	if o.IsEmpty() {
		return
	}
	b.Add(Point{X: o.MinX, Y: o.MinY})
	b.Add(Point{X: o.MaxX, Y: o.MaxY})
}

// WidthMM is the extent along X.
func (b Bounds) WidthMM() float64 {
	if b.IsEmpty() {
		return 0
	}
	return b.MaxX - b.MinX
}

// HeightMM is the extent along Y.
func (b Bounds) HeightMM() float64 {
	if b.IsEmpty() {
		return 0
	}
	return b.MaxY - b.MinY
}

// Area is the bounding-box area.
func (b Bounds) Area() float64 { return b.WidthMM() * b.HeightMM() }
