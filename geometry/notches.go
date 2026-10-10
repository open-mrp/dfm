package geometry

// Notch is a bay of an outline: a stretch of it that dips inside the outline's convex hull, closed across its mouth by the hull. WidthMM is its narrowest width (its minimum Feret diameter) and LengthMM its length at right angles to that.
type Notch struct {
	WidthMM, LengthMM float64
	// Mouth is the hull edge across its opening; Deepest the point of the bay furthest from it.
	Mouth   [2]Point
	Deepest Point
}

// Notches returns the bays of outer loop i deeper than EndpointTolerance, in outline order. Curves are polygons (10°).
func (p *Part) Notches(i int) []Notch {
	poly := loopPolygon(p.Loops[i])
	n := len(poly)
	if n < 4 {
		return nil
	}
	onHull := map[Point]bool{}
	for _, q := range convexHull(poly) {
		onHull[q] = true
	}
	var idx []int
	for k, q := range poly {
		if onHull[q] {
			idx = append(idx, k)
		}
	}
	var out []Notch
	for j, a := range idx {
		b := idx[(j+1)%len(idx)]
		if (b-a+n)%n <= 1 {
			continue
		}
		bay := []Point{poly[a]}
		for k := (a + 1) % n; k != b; k = (k + 1) % n {
			bay = append(bay, poly[k])
		}
		bay = append(bay, poly[b])
		// Stretches of outline along the hull either side are not the notch: it starts where the outline leaves the mouth's line.
		online := func(q Point) bool { return linePointDist(q, poly[a], poly[b]) <= EndpointTolerance }
		lo, hi := 0, len(bay)-1
		for lo+1 < hi && online(bay[lo+1]) {
			lo++
		}
		for hi-1 > lo && online(bay[hi-1]) {
			hi--
		}
		bay = bay[lo : hi+1]
		mouth := [2]Point{bay[0], bay[len(bay)-1]}
		depth, deepest := 0.0, mouth[0]
		for _, q := range bay {
			if d := pointSegmentDist(q, mouth[0], mouth[1]); d > depth {
				depth, deepest = d, q
			}
		}
		if depth <= EndpointTolerance {
			continue
		}
		w, l := polygonWidth(bay)
		out = append(out, Notch{WidthMM: w, LengthMM: l, Mouth: mouth, Deepest: deepest})
	}
	return out
}
