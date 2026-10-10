package design

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/open-mrp/dfm/geometry"
)

// maxBlockDepth bounds how deeply block references may nest; deeper (or circular) nesting is an invalid file.
const maxBlockDepth = 16

// maxBlockInstances and maxBlockSegments bound what block references may expand to (entities, and the segments they convert to), so a small file of nested arrays or stretched curves cannot make a check take minutes. Real sheets stay far below both.
const (
	maxBlockInstances = 10_000
	maxBlockSegments  = 50_000
)

// inherited is what entities inside a block reference take from it: entities on layer 0 take its layer, BYBLOCK line types take its line type, and a hidden reference hides everything in it.
type inherited struct {
	layer, linetype string
	// color is the reference's color, which BYBLOCK entities take.
	color    string
	hidden   bool
	idPrefix string
}

// effective returns e as it is drawn inside a block reference: its handle prefixed with the reference's, and its layer and line type inherited.
func (in inherited) effective(e rawEntity) rawEntity {
	if in.idPrefix == "" {
		return e
	}
	out := rawEntity{kind: e.kind, line: e.line, ordinal: e.ordinal}
	id := in.idPrefix + e.id()
	hasLayer := false
	for _, t := range e.tags {
		switch t.code {
		case 5:
			continue // replaced below
		case 8:
			hasLayer = true
			if t.value == "0" && in.layer != "" {
				t.value = in.layer
			}
		case 6:
			if strings.EqualFold(t.value, "BYBLOCK") {
				t.value = in.linetype
			}
		case 62:
			if strings.TrimSpace(t.value) == "0" && in.color != "" {
				// BYBLOCK: the reference's color, kept as a true color.
				out.tags = append(out.tags, tag{code: colorTag, value: in.color, line: t.line})
				continue
			}
		}
		out.tags = append(out.tags, t)
	}
	if !hasLayer && in.layer != "" {
		out.tags = append(out.tags, tag{code: 8, value: in.layer, line: e.line})
	}
	out.tags = append(out.tags, tag{code: 5, value: id, line: e.line})
	return out
}

// walk converts entities, expanding block references. Entities that are not through-cuts are validated, then recorded in c.nonCut instead.
func (c *converter) walk(f *dxfFile, entities []rawEntity, in inherited, depth int, unsupported *[]string) error {
	for i := 0; i < len(entities); i++ {
		e := in.effective(entities[i])
		if space, err := e.int(67, 0); err != nil {
			return err
		} else if space != 0 {
			continue // paper space
		}
		if e.kind == "INSERT" {
			n, err := c.insert(f, e, entities[i+1:], in, depth, unsupported)
			i += n
			if err != nil {
				return err
			}
			continue
		}
		kind, nonCut, err := f.nonCut(e)
		if err != nil {
			return err
		}
		if in.hidden && cutEntities[e.kind] {
			kind, nonCut = geometry.NonCutConstruction, true
		}
		if !nonCut {
			paths, circles := len(c.paths), len(c.circles)
			var n int
			n, *unsupported, err = c.convert(e, entities[i+1:], *unsupported)
			i += n
			if err != nil {
				return err
			}
			if depth > 0 {
				c.expandedSegments += len(c.circles) - circles
				for _, p := range c.paths[paths:] {
					c.expandedSegments += len(p.Segments)
				}
				if c.expandedSegments > maxBlockSegments {
					return fmt.Errorf("%w: block references expand to more than %d segments", ErrUnsupportedContent, maxBlockSegments)
				}
			}
			continue
		}
		// Convert non-cut geometry on its own, so it is still validated and can be located, then leave it out of the part.
		scratch := converter{scale: c.scale, xf: c.xf, zOff: c.zOff, zScale: c.zScale}
		var n int
		n, *unsupported, err = scratch.convert(e, entities[i+1:], *unsupported)
		i += n
		if err != nil {
			return err
		}
		nc := geometry.NonCut{EntityID: e.id(), Kind: kind, Position: scratch.first()}
		lineCandidate := kind == geometry.NonCutBend || kind == geometry.NonCutConstruction && !in.hidden && f.onlyDashed(e)
		if lineCandidate && len(scratch.paths) == 1 {
			if a, b, ok := geometry.StraightEnds(scratch.paths[0]); ok {
				nc.Line = &[2]geometry.Point{a, b}
			}
		}
		c.nonCut = append(c.nonCut, nc)
	}
	return nil
}

// insert expands a block reference (INSERT): every column and row of it, with its position, rotation, scale and extrusion, on top of the transform the reference itself is drawn with. rest is the entities that follow it; n is how many of them it owns (attributes up to SEQEND).
func (c *converter) insert(f *dxfFile, e rawEntity, rest []rawEntity, in inherited, depth int, unsupported *[]string) (n int, err error) {
	if follows, err := e.int(66, 0); err != nil {
		return 0, err
	} else if follows != 0 {
		for n < len(rest) && rest[n].kind == "ATTRIB" {
			n++
		}
		if n < len(rest) && rest[n].kind == "SEQEND" {
			n++
		}
	}
	name := e.str(2)
	b, ok := f.blocks[strings.ToUpper(name)]
	switch {
	case !ok:
		return n, e.errorf(ErrInvalid, "refers to undefined block %q", truncate(name))
	case b.external:
		return n, e.errorf(ErrUnsupportedContent, "refers to external reference %q, whose geometry is in another file", truncate(name))
	case depth >= maxBlockDepth:
		return n, e.errorf(ErrInvalid, "nests blocks more than %d deep (circular reference?)", maxBlockDepth)
	}
	o, err := entityOCS(e)
	if err != nil {
		return n, err
	}
	v, err := floats(e, 10, 20)
	if err != nil {
		return n, err
	}
	var p [7]float64 // scale x, y; rotation; columns, rows; column, row spacing
	for i, d := range []struct {
		code int
		def  float64
	}{{41, 1}, {42, 1}, {50, 0}, {70, 1}, {71, 1}, {44, 0}, {45, 0}} {
		if p[i], err = e.float(d.code, d.def); err != nil {
			return n, err
		}
	}
	sx, sy, rot := p[0], p[1], p[2]*math.Pi/180
	if p[3] > maxBlockInstances || p[4] > maxBlockInstances {
		return n, e.errorf(ErrUnsupportedContent, "has an array of more than %d copies", maxBlockInstances)
	}
	cols, rows := max(1, int(p[3])), max(1, int(p[4]))
	if sx == 0 || sy == 0 {
		return n, e.errorf(ErrInvalid, "has a zero scale")
	}
	// Each copy counts, even of an empty block: placing one still costs.
	if c.expanded += cols * rows * max(1, len(b.entities)); c.expanded > maxBlockInstances {
		return n, fmt.Errorf("%w: block references expand to more than %d entities", ErrUnsupportedContent, maxBlockInstances)
	}
	ocsMap := identity
	if o.mirror {
		ocsMap = scaling(-1, 1)
	}
	layer := e.layer()
	hidden := false
	if inv, err := e.int(60, 0); err != nil {
		return n, err
	} else if inv != 0 || f.layers[strings.ToUpper(layer)].hidden {
		hidden = true
	}
	insZ, err := e.float(30, 0)
	if err != nil {
		return n, err
	}
	if o.mirror {
		insZ = -insZ
	}
	sz, err := e.float(43, 1)
	if err != nil {
		return n, err
	}
	parent, parentZOff, parentZScale := c.xf, c.zOff, c.zScale
	defer func() { c.xf, c.zOff, c.zScale = parent, parentZOff, parentZScale }()
	c.zOff, c.zScale = parentZOff+float64(parentZScale*insZ), parentZScale*sz
	for row := range rows {
		for col := range cols {
			place := ocsMap.then(translate(v[0], v[1])).then(rotation(rot)).
				then(translate(float64(col)*p[5], float64(row)*p[6])).
				then(scaling(sx, sy)).then(translate(-b.baseX, -b.baseY))
			c.xf = parent.then(place)
			prefix := e.id()
			if cols*rows > 1 {
				prefix += "[" + strconv.Itoa(row) + "," + strconv.Itoa(col) + "]"
			}
			child := inherited{layer: layer, linetype: e.str(6), color: f.entityColor(e), hidden: in.hidden || hidden, idPrefix: prefix + "/"}
			if err := c.walk(f, b.entities, child, depth+1, unsupported); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}
