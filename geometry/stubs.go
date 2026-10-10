package geometry

// maxIndexSamples bounds the points a segment index holds.
const maxIndexSamples = 4e6

// dropStubs removes open paths that lie wholly on other cut paths (within tol), so they add no geometry: a short line traced over a polyline's edge, or a leftover piece of a longer line. Such a path is kept when both its ends meet other paths' ends and an open path covers it, since it may then be the piece that closes an outline. Each removal is returned as a Duplicate.
func dropStubs(paths []Contour, tol float64) ([]Contour, []Duplicate) {
	open := false
	for _, c := range paths {
		open = open || !c.Closed
	}
	if !open {
		return paths, nil
	}
	idx := newSegIndex(paths, tol)
	ends := map[[2]int64]int{} // path ends by cell, counted
	for _, c := range paths {
		if !c.Closed && len(c.Segments) > 0 {
			ends[cellOf(c.Segments[0].Start)]++
			ends[cellOf(c.Segments[len(c.Segments)-1].End)]++
		}
	}
	dropped := make([]bool, len(paths))
	// coveredBy returns the paths q lies on, other than path self and dropped ones.
	coveredBy := func(q Point, self int) []int {
		var out []int
		for _, r := range idx.near(q) {
			if r.path == self || dropped[r.path] {
				continue
			}
			if d, _ := segmentPrimitive(paths[r.path].Segments[r.seg]).pointDist(q); d <= tol {
				out = append(out, r.path)
			}
		}
		return out
	}
	var dups []Duplicate
	for i, c := range paths {
		if c.Closed || len(c.Segments) == 0 {
			continue
		}
		covers := map[int]bool{}
		onOthers := true
		for _, s := range c.Segments {
			p := segmentPrimitive(s)
			for k := 0; k <= 4 && onOthers; k++ {
				var q Point
				if p.line {
					t := float64(k) / 4
					q = Point{s.Start.X + t*(s.End.X-s.Start.X), s.Start.Y + t*(s.End.Y-s.Start.Y)}
				} else {
					q = p.pointAt(p.start + p.sweep*float64(k)/4)
				}
				on := coveredBy(q, i)
				if len(on) == 0 {
					onOthers = false
				}
				for _, j := range on {
					covers[j] = true
				}
			}
			if !onOthers {
				break
			}
		}
		if !onOthers {
			continue
		}
		openCover := false
		for j := range covers {
			if !paths[j].Closed {
				openCover = true
			}
		}
		joined := 0
		for _, q := range []Point{c.Segments[0].Start, c.Segments[len(c.Segments)-1].End} {
			if ends[cellOf(q)] > 1 {
				joined++
			}
		}
		if openCover && joined == 2 {
			continue
		}
		dropped[i] = true
		var kept []string
		seen := map[string]bool{}
		for j := range paths {
			if covers[j] {
				for _, id := range paths[j].EntityIDs() {
					if !seen[id] {
						seen[id] = true
						kept = append(kept, id)
					}
				}
			}
		}
		dups = append(dups, Duplicate{Copies: c.EntityIDs(), Kept: kept, Location: segmentMidpoint(c.Segments[0])})
	}
	var out []Contour
	for i, c := range paths {
		if !dropped[i] {
			out = append(out, c)
		}
	}
	return out, dups
}
