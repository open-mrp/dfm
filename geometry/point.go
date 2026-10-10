package geometry

import (
	"math"
)

// Point is a 2D point in millimeters.
type Point struct {
	X, Y float64
}

// Dist returns the Euclidean distance between p and q.
func (p Point) Dist(q Point) float64 {
	return math.Hypot(q.X-p.X, q.Y-p.Y)
}

// Near reports whether p and q coincide within EndpointTolerance.
func (p Point) Near(q Point) bool {
	return p.Dist(q) <= EndpointTolerance
}
