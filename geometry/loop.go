package geometry

// Loop is one closed contour or circle in the containment tree.
type Loop struct {
	// Contour and Circle index the loop's source in Part; the other is -1.
	Contour, Circle int
	Depth           int
	// Parent indexes the enclosing loop in Part.Loops, or -1.
	Parent    int
	AreaMM2   float64
	Bounds    Bounds
	EntityIDs []string

	prims      []primitive
	primBounds []Bounds
	bands      *bandIndex // prims split into y-monotone pieces by band of y, for point-in-loop tests
	along      []float64  // boundary length before each prim; the last is the total
	ccw        bool
}

// IsCutout reports whether the loop removes material (odd depth).
func (l Loop) IsCutout() bool { return l.Depth%2 == 1 }
