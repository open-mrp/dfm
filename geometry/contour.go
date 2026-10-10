package geometry

// Contour is a chain of connected segments forming one cut path.
type Contour struct {
	Segments []Segment
	Closed   bool
	// Depth is the nesting depth of a closed contour (see Part); -1 for open contours.
	Depth int
}

// EntityIDs returns the distinct source entity IDs of the contour's segments in traversal order.
func (c Contour) EntityIDs() []string {
	var ids []string
	seen := map[string]bool{}
	for _, s := range c.Segments {
		if !seen[s.EntityID] {
			seen[s.EntityID] = true
			ids = append(ids, s.EntityID)
		}
	}
	return ids
}

// Bounds returns the contour's bounding box.
func (c Contour) Bounds() Bounds {
	b := EmptyBounds()
	for _, s := range c.Segments {
		b.Union(s.Bounds())
	}
	return b
}
