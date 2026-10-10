package geometry

import (
	"math"
)

// EndpointTolerance is the maximum distance (mm) between two endpoints for them to be considered coincident when chaining segments into contours.
const EndpointTolerance = 1e-3

// Segment is a straight line (Bulge == 0) or circular arc between Start and End. Bulge follows the DXF convention: tan(sweep/4), positive for counter-clockwise arcs. A single segment cannot represent a full circle; full circles are modeled as Circle.
type Segment struct {
	Start, End Point
	Bulge      float64
	EntityID   string
}

// IsArc reports whether the segment is an arc.
func (s Segment) IsArc() bool { return s.Bulge != 0 }

// Reverse returns the same segment traversed in the opposite direction.
func (s Segment) Reverse() Segment {
	return Segment{Start: s.End, End: s.Start, Bulge: -s.Bulge, EntityID: s.EntityID}
}

// Sweep returns the signed included angle of an arc segment in radians (positive = counter-clockwise). It is zero for lines.
func (s Segment) Sweep() float64 {
	return 4 * math.Atan(s.Bulge)
}

// Chord returns the straight-line distance from Start to End.
func (s Segment) Chord() float64 { return s.Start.Dist(s.End) }

// Radius returns the arc radius, or +Inf for a line.
func (s Segment) Radius() float64 {
	if s.Bulge == 0 {
		return math.Inf(1)
	}
	half := s.Chord() / 2
	return math.Abs(half * (1 + float64(s.Bulge*s.Bulge)) / (2 * s.Bulge))
}

// Center returns the arc center. It is only meaningful for arcs.
func (s Segment) Center() Point {
	mx, my := (s.Start.X+s.End.X)/2, (s.Start.Y+s.End.Y)/2
	dx, dy := s.End.X-s.Start.X, s.End.Y-s.Start.Y
	// Signed distance from chord midpoint to center along the chord's left normal, derived from bulge b: d = chord/2 * (1 - b²) / (2b).
	k := (1 - float64(s.Bulge*s.Bulge)) / (4 * s.Bulge)
	return Point{X: mx - float64(dy*k), Y: my + float64(dx*k)}
}

// Bounds returns the axis-aligned bounding box of the segment, including arc extremes that lie between the endpoints.
func (s Segment) Bounds() Bounds {
	b := EmptyBounds()
	b.Add(s.Start)
	b.Add(s.End)
	if !s.IsArc() {
		return b
	}
	c, r := s.Center(), s.Radius()
	start := math.Atan2(s.Start.Y-c.Y, s.Start.X-c.X)
	sweep := s.Sweep()
	for k := range 4 {
		axis := float64(k) * math.Pi / 2
		if angleWithin(axis, start, sweep) {
			b.Add(Point{X: c.X + r*math.Cos(axis), Y: c.Y + r*math.Sin(axis)})
		}
	}
	return b
}

// angleWithin reports whether angle a lies on the arc that starts at start and sweeps by sweep radians (either direction).
func angleWithin(a, start, sweep float64) bool {
	if sweep < 0 {
		start, sweep = start+sweep, -sweep
	}
	d := math.Mod(a-start, 2*math.Pi)
	if d < 0 {
		d += 2 * math.Pi
	}
	return d <= sweep
}
