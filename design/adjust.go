package design

import (
	"slices"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

// adjust applies the adjustments that name entities: a removed entity is left out of the drawing, and one marked as a bend line is left out of the part as a bend line. Each raw path and circle is one entity. It fails with an *AdjustmentError when an adjustment names an entity the drawing does not have.
func (c *converter) adjust(adj []Adjustment) error {
	action := map[string]constants.FixAction{}
	for _, a := range adj {
		for _, id := range a.EntityIDs {
			action[id] = a.Action
		}
	}
	if len(action) == 0 {
		return nil
	}
	seen := map[string]bool{}
	bent := map[string]bool{}
	// take reports whether the entity is adjusted away from cutting; path is its geometry when it is a path.
	take := func(id string, at geometry.Point, path *geometry.Contour) bool {
		switch action[id] {
		case constants.FixActionRemove:
			seen[id] = true
			return true
		case constants.FixActionMarkBendLine:
			seen[id] = true
			if !bent[id] {
				bent[id] = true
				p := at
				nc := geometry.NonCut{EntityID: id, Kind: geometry.NonCutBend, Position: &p}
				if path != nil {
					if a, b, ok := geometry.StraightEnds(*path); ok {
						nc.Line = &[2]geometry.Point{a, b}
					}
				}
				c.nonCut = append(c.nonCut, nc)
			}
			return true
		}
		return false
	}
	nonCut := c.nonCut[:0:0]
	for _, n := range c.nonCut {
		switch action[n.EntityID] {
		case constants.FixActionRemove:
			seen[n.EntityID] = true
			continue
		case constants.FixActionMarkBendLine:
			seen[n.EntityID], bent[n.EntityID] = true, true
			n.Kind = geometry.NonCutBend
		}
		nonCut = append(nonCut, n)
	}
	c.nonCut = nonCut
	paths := c.paths[:0:0]
	for _, p := range c.paths {
		if !take(p.Segments[0].EntityID, p.Segments[0].Start, &p) {
			paths = append(paths, p)
		}
	}
	c.paths = paths
	circles := c.circles[:0:0]
	for _, k := range c.circles {
		if len(k.EntityIDs) == 0 || !take(k.EntityIDs[0], geometry.Point{X: k.Center.X + k.Radius, Y: k.Center.Y}, nil) {
			circles = append(circles, k)
		}
	}
	c.circles = circles
	texts := c.texts[:0:0]
	for _, t := range c.texts {
		if action[t.EntityID] == constants.FixActionRemove {
			seen[t.EntityID] = true
			continue
		}
		texts = append(texts, t)
	}
	c.texts = texts
	heights := c.heights[:0:0]
	for _, h := range c.heights {
		if action[h.id] != constants.FixActionRemove {
			heights = append(heights, h)
		}
	}
	c.heights = heights
	for i, a := range adj {
		for _, id := range a.EntityIDs {
			if !seen[id] {
				return &AdjustmentError{Index: i, EntityID: id}
			}
		}
	}
	return nil
}

// buildWithBendLines builds the part, then reads straight lines that run through the material from one cut to another as bend lines: a dashed one (the way quote services ask bend lines to be drawn) becomes a bend line, and a solid one on a cut layer is left out of the part as a suspected bend line, with any copies drawn on top of it, which the returned part's NonCut lists (the copies with it, not as duplicates).
func buildWithBendLines(paths []geometry.Contour, circles []geometry.Circle, opts geometry.BuildOptions, nonCut []geometry.NonCut) *geometry.Part {
	part := geometry.Build(paths, circles, opts)
	// A sheet border drawn around everything is left out first, so the parts inside it are parts rather than islands in one big frame.
	border, borderAt, framed := part.FindBorder()
	if framed {
		paths = withoutEntities(paths, border)
		part = geometry.Build(paths, circles, opts)
	}
	defer func() {
		if framed {
			part.Border, part.BorderAt = border, &borderAt
		}
	}()
	for i, n := range nonCut {
		if n.Kind == geometry.NonCutConstruction && n.Line != nil && part.SpansMaterial(n.Line[0], n.Line[1], nil, geometry.EndpointTolerance) {
			nonCut[i].Kind = geometry.NonCutBend
		}
	}
	var suspected []geometry.NonCut
	drop := map[string]bool{}
	for _, c := range part.Contours {
		ids := c.EntityIDs()
		if len(ids) != 1 {
			continue
		}
		a, b, ok := geometry.StraightEnds(c)
		if ok && part.SpansMaterial(a, b, ids, geometry.EndpointTolerance) {
			p := a
			suspected = append(suspected, geometry.NonCut{EntityID: ids[0], Kind: geometry.NonCutSuspectedBend, Position: &p})
			drop[ids[0]] = true
		}
	}
	if len(suspected) == 0 {
		return part
	}
	// Copies of a dropped line go with it, listed with it rather than as duplicates, so its fix covers them.
	for _, d := range part.Duplicates {
		for i := range suspected {
			if slices.Contains(d.Kept, suspected[i].EntityID) {
				suspected[i].Copies = append(suspected[i].Copies, d.Copies...)
				for _, id := range d.Copies {
					drop[id] = true
				}
			}
		}
	}
	var kept []geometry.Contour
	for _, c := range paths {
		if !slices.ContainsFunc(c.EntityIDs(), func(id string) bool { return drop[id] }) {
			kept = append(kept, c)
		}
	}
	part = geometry.Build(kept, circles, opts)
	part.NonCut = suspected
	return part
}

// withoutEntities returns the paths not drawn by any of ids.
func withoutEntities(paths []geometry.Contour, ids []string) []geometry.Contour {
	var kept []geometry.Contour
	for _, c := range paths {
		if !slices.ContainsFunc(c.EntityIDs(), func(id string) bool { return slices.Contains(ids, id) }) {
			kept = append(kept, c)
		}
	}
	return kept
}
