package geometry

import (
	"math"
)

// Overlap is a stretch of cut path drawn more than once: two collinear lines or two co-circular arcs/circles sharing a positive length.
type Overlap struct {
	EntityIDs []string
	LengthMM  float64
	Location  Point // midpoint of the shared stretch
}

// Duplicate is a path or circle that exactly repeats other geometry. The copy was dropped; Copies are its entities (safe to delete) and Kept the geometry it repeats.
type Duplicate struct {
	Copies, Kept []string
	Location     Point
	// GapMM is how far the copy is from what it repeats: 0 for an exact copy, up to BuildOptions.NearDuplicateMM for a line drawn beside one.
	GapMM float64
}

// Overcut is a path that ran on past its start over its own first stretch; the repeat, LengthMM long, was trimmed and the path closed.
type Overcut struct {
	EntityIDs []string
	LengthMM  float64
	Location  Point
}

// CornerLoops are the small loops one path makes where it leaves a point and comes back to it before running on, corner reliefs some generators draw so the beam clears an inside corner. The loops were removed; SizeMM is the largest across and Location the first.
type CornerLoops struct {
	EntityIDs []string
	Count     int
	SizeMM    float64
	Location  Point
}

// Contact is a place where cut paths touch or cross where they should not: two distinct paths (SameContour false) or two non-adjacent pieces of one path (SameContour true, a self-intersection).
type Contact struct {
	EntityIDs   []string
	SameContour bool
	Location    Point
}

// overlapLength returns the length two primitives share and its midpoint.
func overlapLength(p, q primitive) (float64, Point, bool) {
	const tol = EndpointTolerance
	if p.line != q.line {
		return 0, Point{}, false
	}
	if p.line {
		dx, dy := p.b.X-p.a.X, p.b.Y-p.a.Y
		l := math.Hypot(dx, dy)
		if l == 0 {
			return 0, Point{}, false
		}
		ux, uy := dx/l, dy/l
		// Both endpoints of q must lie on p's supporting line.
		for _, e := range []Point{q.a, q.b} {
			if math.Abs(cross(ux, uy, e.X-p.a.X, e.Y-p.a.Y)) > tol {
				return 0, Point{}, false
			}
		}
		t0 := (q.a.X-p.a.X)*ux + (q.a.Y-p.a.Y)*uy
		t1 := (q.b.X-p.a.X)*ux + (q.b.Y-p.a.Y)*uy
		lo, hi := math.Max(0, math.Min(t0, t1)), math.Min(l, math.Max(t0, t1))
		if hi-lo <= tol {
			return 0, Point{}, false
		}
		m := (lo + hi) / 2
		return hi - lo, Point{p.a.X + m*ux, p.a.Y + m*uy}, true
	}
	if p.c.Dist(q.c) > tol || math.Abs(p.r-q.r) > tol {
		return 0, Point{}, false
	}
	a0, alen := ccwInterval(p)
	b0, blen := ccwInterval(q)
	d := math.Mod(b0-a0, 2*math.Pi)
	if d < 0 {
		d += 2 * math.Pi
	}
	// In p's frame p covers [0, alen] and q covers [d, d+blen], which may wrap past 2π onto [0, d+blen-2π].
	var shared, mid float64
	if lo, hi := d, math.Min(alen, d+blen); hi > lo {
		shared, mid = hi-lo, (lo+hi)/2
	}
	if wrap := d + blen - 2*math.Pi; wrap > 0 {
		if hi := math.Min(alen, wrap); hi > shared {
			shared, mid = hi, hi/2
		}
	}
	length := shared * p.r
	if length <= tol {
		return 0, Point{}, false
	}
	return length, p.pointAt(a0 + mid), true
}

// ccwInterval returns an arc's start angle and positive sweep.
func ccwInterval(p primitive) (float64, float64) {
	if p.sweep < 0 {
		return p.start + p.sweep, -p.sweep
	}
	return p.start, p.sweep
}

// adjacent reports whether two pieces of one contour share an endpoint by construction (consecutive segments, including last-to-first when closed).
func adjacent(a, b element) bool {
	if a.owner != b.owner || a.ownerLen == 0 {
		return false
	}
	d := a.index - b.index
	if d < 0 {
		d = -d
	}
	return d == 1 || (a.closed && d == a.ownerLen-1)
}

// findDefects reports overlaps and unwanted contacts among all cut paths. Overlaps are merged per pair of paths (total shared length, all entities involved). A pair of paths that overlaps is not also reported as a contact, and at most one contact is reported per pair of paths (or per path, for self-intersections), so one bad joint yields one finding. Contacts are only computed when wantContacts is set.
func findDefects(contours []Contour, circles []Circle, wantContacts bool) ([]Overlap, []Contact) {
	elems := elementsOf(contours, circles)
	boxes := make([]Bounds, len(elems))
	for i, e := range elems {
		boxes[i] = e.bounds
	}
	pairs := candidatePairs(boxes, EndpointTolerance)

	var overlaps []Overlap
	overlapAt := map[[2]int]int{}
	for _, pr := range pairs {
		a, b := elems[pr[0]], elems[pr[1]]
		l, at, ok := overlapLength(a.prim, b.prim)
		if !ok {
			continue
		}
		key := ownerKey(a, b)
		if i, ok := overlapAt[key]; ok {
			overlaps[i].LengthMM += l
			overlaps[i].EntityIDs = appendUnique(overlaps[i].EntityIDs, a.entityID, b.entityID)
			continue
		}
		overlapAt[key] = len(overlaps)
		overlaps = append(overlaps, Overlap{EntityIDs: appendUnique(nil, a.entityID, b.entityID), LengthMM: l, Location: at})
	}

	if !wantContacts {
		return overlaps, nil
	}
	var contacts []Contact
	seen := map[[2]int]bool{}
	for _, pr := range pairs {
		a, b := elems[pr[0]], elems[pr[1]]
		key := ownerKey(a, b)
		if _, overlapped := overlapAt[key]; overlapped || seen[key] {
			continue
		}
		if adjacent(a, b) || (a.owner == b.owner && a.ownerLen == 0) {
			continue
		}
		if d, at, _ := primitiveDistance(a.prim, b.prim); d <= EndpointTolerance {
			seen[key] = true
			contacts = append(contacts, Contact{EntityIDs: appendUnique(nil, a.entityID, b.entityID), SameContour: a.owner == b.owner, Location: at})
		}
	}
	return overlaps, contacts
}

func ownerKey(a, b element) [2]int {
	if a.owner > b.owner {
		return [2]int{b.owner, a.owner}
	}
	return [2]int{a.owner, b.owner}
}

func appendUnique(ids []string, more ...string) []string {
	for _, m := range more {
		dup := false
		for _, id := range ids {
			dup = dup || id == m
		}
		if !dup {
			ids = append(ids, m)
		}
	}
	return ids
}

// isDuplicate reports whether two single-segment pieces are the same geometry in either direction.
func isDuplicate(a, b Segment) bool {
	return (a.Start.Near(b.Start) && a.End.Near(b.End) && math.Abs(a.Bulge-b.Bulge) < 1e-9) ||
		(a.Start.Near(b.End) && a.End.Near(b.Start) && math.Abs(a.Bulge+b.Bulge) < 1e-9)
}

func sameCircle(a, b Circle) bool {
	return a.Center.Near(b.Center) && math.Abs(a.Radius-b.Radius) <= EndpointTolerance
}
