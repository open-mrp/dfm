package sheet

import (
	"errors"
	"fmt"
	"math"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/solid"
)

// ErrNotSheetMetal is returned for a model that cannot be unfolded as one sheet of constant thickness.
var ErrNotSheetMetal = errors.New("not sheet metal")

// tol is how far apart (mm) two points or surfaces may be and still meet.
const tol = 1e-3

// Options are the unfolding's inputs.
type Options struct {
	// KFactor places the neutral axis, as a fraction of the thickness from the inside of each bend.
	KFactor float64
	// ClosureToleranceMM is passed to geometry.Build.
	ClosureToleranceMM float64
}

// Bend is a bend of the model, unrolled into a bend line of the flat part.
type Bend struct {
	// EntityID names the bend line: the unrolled cylinder face.
	EntityID string
	// AngleDeg is the angle from flat.
	AngleDeg float64
	// InsideRadiusMM is the bend's inside radius.
	InsideRadiusMM float64
	// Direction is up when the flange turns toward the viewer of the flat pattern.
	Direction constants.BendDirection
}

// Result is a model unfolded.
type Result struct {
	Part        *geometry.Part
	ThicknessMM float64
	Bends       []Bend
	// Placement puts the flat pattern back in the model: a flat point (x, y) of the base face (the one BasePoint lies in) is at Placement.At(x, y, 0) in the model, on the skin, and Z is the flat's viewer side.
	Placement solid.Frame
	BasePoint geometry.Point
}

// partGapMM is the space left between the flat parts of a model with several solids, laid out side by side.
const partGapMM = 10

// Flatten unfolds a model of sheet-metal parts: each solid is unfolded on its own, and the flat parts are laid out left to right in model order. Every solid must be the same thickness.
func Flatten(m *solid.Model, o Options) (*Result, error) {
	if len(m.Bodies) == 0 {
		return nil, fmt.Errorf("%w: the model has no solid", ErrNotSheetMetal)
	}
	var contours []geometry.Contour
	var circles []geometry.Circle
	var noncut []geometry.NonCut
	var bends []Bend
	t, right := 0.0, 0.0
	var first *piece
	for i, b := range m.Bodies {
		p, err := unfoldBody(b, o)
		if err != nil {
			if len(m.Bodies) > 1 {
				return nil, fmt.Errorf("solid %d of %d: %w", i+1, len(m.Bodies), err)
			}
			return nil, err
		}
		if i == 0 {
			t = p.t
		} else if math.Abs(p.t-t) > tol {
			return nil, fmt.Errorf("%w: solid %d is %.4g mm thick, solid 1 %.4g mm; one file is cut from one sheet", ErrNotSheetMetal, i+1, p.t, t)
		}
		dx := 0.0
		if i > 0 {
			dx = right + partGapMM - p.bounds.MinX
		}
		p.shift(dx)
		if i == 0 {
			first = p
		}
		right = p.bounds.MaxX + dx
		contours, circles = append(contours, p.contours...), append(circles, p.circles...)
		noncut, bends = append(noncut, p.noncut...), append(bends, p.bends...)
	}
	part := geometry.Build(contours, circles, geometry.BuildOptions{ClosureToleranceMM: o.ClosureToleranceMM})
	part.Units = constants.LengthUnitMillimeter
	part.ThicknessMM = t
	part.NonCut = noncut
	r := &Result{Part: part, ThicknessMM: t, Bends: bends}
	if first != nil {
		r.Placement = solid.Frame{Origin: first.origin, X: first.x, Y: first.y, Z: first.z}
		r.BasePoint = first.base
	}
	return r, nil
}

// piece is one solid unfolded, before it is placed.
type piece struct {
	contours []geometry.Contour
	circles  []geometry.Circle
	noncut   []geometry.NonCut
	bends    []Bend
	t        float64
	bounds   geometry.Bounds
	origin   solid.Vec // the model point flat (0, 0) of the base face is at
	x, y, z  solid.Vec
	base     geometry.Point // a flat point in the base face
}

func (p *piece) shift(dx float64) {
	p.origin = p.origin.AddScaled(p.x, -dx)
	p.base.X += dx
	if dx == 0 {
		return
	}
	mv := func(q geometry.Point) geometry.Point { return geometry.Point{X: q.X + dx, Y: q.Y} }
	for i := range p.contours {
		for j := range p.contours[i].Segments {
			sg := &p.contours[i].Segments[j]
			sg.Start, sg.End = mv(sg.Start), mv(sg.End)
		}
	}
	for i := range p.circles {
		p.circles[i].Center = mv(p.circles[i].Center)
	}
	for i := range p.noncut {
		n := &p.noncut[i]
		if n.Position != nil {
			pos := mv(*n.Position)
			n.Position = &pos
		}
		if n.Line != nil {
			line := [2]geometry.Point{mv(n.Line[0]), mv(n.Line[1])}
			n.Line = &line
		}
	}
}

// flat builds the flat part: every edge of the skin that does not join two of its faces is cut; each unrolled cylinder is a bend line.
func (u *unfolder) flat() (*piece, error) {
	var contours []geometry.Contour
	var circles []geometry.Circle
	done := map[*solid.Edge]bool{}
	for _, f := range u.order {
		m := u.maps[f]
		for _, l := range f.Loops {
			for _, e := range l.Edges {
				if done[e] || u.inner(e) {
					continue
				}
				done[e] = true
				if c, ok := m.circle(e); ok {
					circles = append(circles, c)
					continue
				}
				segs, err := m.segments(e)
				if err != nil {
					return nil, err
				}
				if len(segs) > 0 {
					contours = append(contours, geometry.Contour{Segments: segs})
				}
			}
		}
	}
	p := &piece{contours: contours, circles: circles, t: u.t, bounds: geometry.EmptyBounds()}
	for _, c := range contours {
		p.bounds.Union(c.Bounds())
	}
	for _, c := range circles {
		p.bounds.Add(geometry.Point{X: c.Center.X - c.Radius, Y: c.Center.Y})
		p.bounds.Add(geometry.Point{X: c.Center.X + c.Radius, Y: c.Center.Y})
	}
	for _, f := range u.order {
		m := u.maps[f]
		if !m.cyl || m.face != f {
			continue
		}
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, q := range u.samples(f) {
			a := q.Sub(m.O3).Dot(m.U3)
			lo, hi = min(lo, a), max(hi, a)
		}
		mid := float64((m.phiMin+m.phiMax)/2) * m.Rn
		at := func(a float64) geometry.Point {
			return p2(m.O2.X+float64(m.U2.X*a)+float64(m.V2.X*mid), m.O2.Y+float64(m.U2.Y*a)+float64(m.V2.Y*mid))
		}
		line := [2]geometry.Point{at(lo), at(hi)}
		pos := line[0]
		p.noncut = append(p.noncut, geometry.NonCut{EntityID: f.ID, Kind: geometry.NonCutBend, Position: &pos, Line: &line})
		dir := constants.BendDirectionUp
		if m.convex {
			dir = constants.BendDirectionDown
		}
		p.bends = append(p.bends, Bend{EntityID: f.ID, AngleDeg: (m.phiMax - m.phiMin) * 180 / math.Pi, InsideRadiusMM: m.innerR, Direction: dir})
	}
	if bm := u.maps[u.order[0]]; bm != nil {
		p.origin, p.x, p.y, p.z = bm.O3, bm.U3, bm.V3, bm.viewNormal()
		if qs := u.samples(u.order[0]); len(qs) > 0 {
			// The samples' mean: inside the face unless it is very concave.
			var c geometry.Point
			for _, q := range qs {
				f := bm.flat(q)
				c.X, c.Y = c.X+f.X/float64(len(qs)), c.Y+f.Y/float64(len(qs))
			}
			p.base = c
		}
	}
	return p, nil
}
