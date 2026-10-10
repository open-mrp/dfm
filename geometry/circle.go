package geometry

// Circle is a full circular cut path.
type Circle struct {
	Center    Point
	Radius    float64
	EntityIDs []string
	// Depth is the nesting depth (see Part).
	Depth int
}

// Diameter returns 2·Radius.
func (c Circle) Diameter() float64 { return 2 * c.Radius }

// Bounds returns the circle's bounding box.
func (c Circle) Bounds() Bounds {
	return Bounds{MinX: c.Center.X - c.Radius, MinY: c.Center.Y - c.Radius, MaxX: c.Center.X + c.Radius, MaxY: c.Center.Y + c.Radius}
}
