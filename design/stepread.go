package design

import (
	"fmt"
	"math"
	"slices"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/solid"
)

// ParseModel reads a 3D design (STEP) into its solids, in millimeters.
func ParseModel(d Design) (*solid.Model, error) {
	if d.Format != constants.DesignFormatSTEP {
		return nil, fmt.Errorf("%w: %q is not a 3D format", ErrUnsupportedFormat, d.Format)
	}
	if err := tooLarge(d); err != nil {
		return nil, err
	}
	f, err := parseStepFile(d.Data)
	if err != nil {
		return nil, err
	}
	r := &stepReader{f: f, edges: map[int]*solid.Edge{}}
	return r.model(d.Units)
}

// maxStepFaces bounds the faces of one solid: real sheet parts have hundreds, and the unfolding compares faces pairwise.
const maxStepFaces = 5_000

type stepReader struct {
	f     *stepFile
	scale float64 // millimeters per file unit
	edges map[int]*solid.Edge
}

// stepError is a problem with the file's content, naming the entity.
func stepError(id int, format string, args ...any) error {
	return fmt.Errorf("%w: STEP entity #%d: %s", ErrInvalid, id, fmt.Sprintf(format, args...))
}

func (r *stepReader) model(units constants.LengthUnit) (*solid.Model, error) {
	var bodies []int
	for _, id := range r.f.order {
		e := r.f.entities[id]
		if e.is("MANIFOLD_SOLID_BREP") || e.is("BREP_WITH_VOIDS") {
			bodies = append(bodies, id)
		}
	}
	if len(bodies) == 0 {
		for _, id := range r.f.order {
			if n := r.f.entities[id].name(); n == "SHELL_BASED_SURFACE_MODEL" || n == "MANIFOLD_SURFACE_SHAPE_REPRESENTATION" || n == "GEOMETRIC_CURVE_SET" {
				return nil, fmt.Errorf("%w: the STEP file has surfaces but no solid; export the part as a solid body", ErrUnsupportedContent)
			}
		}
		for _, id := range r.f.order {
			if r.f.entities[id].is("NEXT_ASSEMBLY_USAGE_OCCURRENCE") {
				return nil, fmt.Errorf("%w: the STEP file is an assembly whose parts are in other files; export the part, or the assembly as one file", ErrUnsupportedContent)
			}
		}
		return nil, fmt.Errorf("%w: the STEP file has no solid body", ErrInvalid)
	}
	scale, err := r.lengthScale(bodies[0], units)
	if err != nil {
		return nil, err
	}
	r.scale = scale
	m := &solid.Model{}
	for _, id := range bodies {
		b, err := r.body(id)
		if err != nil {
			return nil, err
		}
		m.Bodies = append(m.Bodies, b)
	}
	return m, nil
}

// lengthScale finds the length unit of the representation holding the body (or else the file's only length unit). A units override wins.
func (r *stepReader) lengthScale(body int, override constants.LengthUnit) (float64, error) {
	if override != "" {
		mm, ok := unitMM[override]
		if !ok {
			return 0, fmt.Errorf("%w: units %q", ErrUnsupportedContent, override)
		}
		return mm, nil
	}
	var candidates []int
	for _, id := range r.f.order {
		e := r.f.entities[id]
		p, ok := e.part(representationOf(e))
		if !ok || len(p.params) < 3 || !refersTo(p.param(1), body) {
			continue
		}
		if ctx, ok := r.f.entities[p.param(2).ref]; ok {
			if g, ok := ctx.part("GLOBAL_UNIT_ASSIGNED_CONTEXT"); ok && len(g.params) > 0 {
				for _, u := range g.param(0).list {
					candidates = append(candidates, u.ref)
				}
			}
		}
	}
	if len(candidates) == 0 {
		for _, id := range r.f.order {
			if r.f.entities[id].is("LENGTH_UNIT") {
				candidates = append(candidates, id)
			}
		}
	}
	for _, id := range candidates {
		if e, ok := r.f.entities[id]; ok && e.is("LENGTH_UNIT") {
			if mm, ok := r.unitScale(e, 0); ok {
				return mm, nil
			}
		}
	}
	return 0, fmt.Errorf("%w: the STEP file does not declare a length unit", ErrUnitsUnknown)
}

func representationOf(e *stepEntity) string {
	for _, p := range e.parts {
		switch p.name {
		case "ADVANCED_BREP_SHAPE_REPRESENTATION", "MANIFOLD_SURFACE_SHAPE_REPRESENTATION", "SHAPE_REPRESENTATION", "FACETED_BREP_SHAPE_REPRESENTATION":
			return p.name
		}
	}
	return ""
}

func refersTo(v stepValue, id int) bool {
	return v.kind == stepList && slices.ContainsFunc(v.list, func(x stepValue) bool { return x.kind == stepRef && x.ref == id })
}

var siPrefixes = map[string]float64{"": 1000, "MILLI": 1, "CENTI": 10, "DECI": 100, "MICRO": 0.001, "KILO": 1e6}

var unitMM = map[constants.LengthUnit]float64{
	constants.LengthUnitMicrometer: 0.001, constants.LengthUnitMillimeter: 1, constants.LengthUnitCentimeter: 10,
	constants.LengthUnitMeter: 1000, constants.LengthUnitInch: 25.4, constants.LengthUnitFoot: 304.8,
}

// unitScale is millimeters per unit for an SI or conversion-based length unit.
func (r *stepReader) unitScale(e *stepEntity, depth int) (float64, bool) {
	if depth > 4 {
		return 0, false
	}
	if si, ok := e.part("SI_UNIT"); ok && len(si.params) == 2 && si.param(1).str == "METRE" {
		mm, ok := siPrefixes[si.param(0).str]
		return mm, ok
	}
	if cb, ok := e.part("CONVERSION_BASED_UNIT"); ok && len(cb.params) == 2 {
		if m, ok := r.f.entities[cb.param(1).ref]; ok {
			if mwu, ok := m.part("LENGTH_MEASURE_WITH_UNIT"); ok && len(mwu.params) == 2 {
				factor := mwu.param(0)
				if factor.kind == stepTyped && len(factor.list) == 1 {
					factor = factor.list[0]
				}
				if base, ok := r.f.entities[mwu.param(1).ref]; ok && factor.kind == stepNumber {
					if mm, ok := r.unitScale(base, depth+1); ok {
						return float64(factor.num * mm), true
					}
				}
			}
		}
		switch cb.param(0).str {
		case "INCH", "inch":
			return 25.4, true
		case "FOOT", "foot":
			return 304.8, true
		}
	}
	return 0, false
}

func (r *stepReader) entity(id int, names ...string) (*stepEntity, stepPart, error) {
	e, ok := r.f.entities[id]
	if !ok {
		return nil, stepPart{}, stepError(id, "missing")
	}
	for _, n := range names {
		if p, ok := e.part(n); ok {
			return e, p, nil
		}
	}
	return nil, stepPart{}, stepError(id, "expected %v", names)
}

func (r *stepReader) body(id int) (*solid.Body, error) {
	_, p, err := r.entity(id, "MANIFOLD_SOLID_BREP", "BREP_WITH_VOIDS")
	if err != nil {
		return nil, err
	}
	if len(p.params) < 2 {
		return nil, stepError(id, "too few parameters")
	}
	b := &solid.Body{ID: fmt.Sprintf("#%d", id)}
	shells := []stepValue{p.param(1)}
	if len(p.params) > 2 && p.param(2).kind == stepList {
		shells = append(shells, p.param(2).list...)
	}
	for _, s := range shells {
		_, sp, err := r.entity(s.ref, "CLOSED_SHELL", "ORIENTED_CLOSED_SHELL", "OPEN_SHELL")
		if err != nil {
			return nil, err
		}
		faces := sp.param(len(sp.params) - 1)
		if len(b.Faces)+len(faces.list) > maxStepFaces {
			return nil, fmt.Errorf("%w: solid %s has more than %d faces; a sheet-metal part has far fewer (export the part, not a mesh or an assembly)", ErrUnsupportedContent, b.ID, maxStepFaces)
		}
		for _, fv := range faces.list {
			f, err := r.face(fv.ref)
			if err != nil {
				return nil, err
			}
			b.Faces = append(b.Faces, f)
		}
	}
	return b, nil
}

func (r *stepReader) face(id int) (*solid.Face, error) {
	_, p, err := r.entity(id, "ADVANCED_FACE", "FACE_SURFACE")
	if err != nil {
		return nil, err
	}
	if len(p.params) < 4 {
		return nil, stepError(id, "too few parameters")
	}
	surf, err := r.surface(p.param(2).ref)
	if err != nil {
		return nil, err
	}
	f := &solid.Face{ID: fmt.Sprintf("#%d", id), Surface: surf, SameSense: p.param(3).str != "F"}
	for _, bv := range p.param(1).list {
		be, bp, err := r.entity(bv.ref, "FACE_OUTER_BOUND", "FACE_BOUND")
		if err != nil {
			return nil, err
		}
		_, lp, err := r.entity(bp.param(1).ref, "EDGE_LOOP", "VERTEX_LOOP", "POLY_LOOP")
		if err != nil {
			return nil, err
		}
		if _, isEdges := r.f.entities[bp.param(1).ref].part("EDGE_LOOP"); !isEdges {
			continue // a point or polygon loop bounds nothing to cut
		}
		loop := solid.Loop{Outer: be.is("FACE_OUTER_BOUND")}
		flip := bp.param(2).str == "F"
		for _, ov := range lp.param(len(lp.params) - 1).list {
			_, op, err := r.entity(ov.ref, "ORIENTED_EDGE")
			if err != nil {
				return nil, err
			}
			e, err := r.edge(op.param(3).ref)
			if err != nil {
				return nil, err
			}
			loop.Edges = append(loop.Edges, e)
			loop.Reversed = append(loop.Reversed, (op.param(4).str == "F") != flip)
		}
		if flip {
			slices.Reverse(loop.Edges)
			slices.Reverse(loop.Reversed)
		}
		f.Loops = append(f.Loops, loop)
	}
	return f, nil
}

func (r *stepReader) edge(id int) (*solid.Edge, error) {
	if e, ok := r.edges[id]; ok {
		return e, nil
	}
	_, p, err := r.entity(id, "EDGE_CURVE")
	if err != nil {
		return nil, err
	}
	if len(p.params) < 5 {
		return nil, stepError(id, "too few parameters")
	}
	start, err := r.vertex(p.param(1).ref)
	if err != nil {
		return nil, err
	}
	end, err := r.vertex(p.param(2).ref)
	if err != nil {
		return nil, err
	}
	c, err := r.curve(p.param(3).ref)
	if err != nil {
		return nil, err
	}
	e := &solid.Edge{ID: fmt.Sprintf("#%d", id), Start: start, End: end, Curve: c, SameSense: p.param(4).str != "F"}
	e.Prepare()
	r.edges[id] = e
	return e, nil
}

func (r *stepReader) vertex(id int) (solid.Vec, error) {
	_, p, err := r.entity(id, "VERTEX_POINT")
	if err != nil {
		return solid.Vec{}, err
	}
	return r.point(p.param(1).ref)
}

func (r *stepReader) point(id int) (solid.Vec, error) {
	_, p, err := r.entity(id, "CARTESIAN_POINT")
	if err != nil {
		return solid.Vec{}, err
	}
	v, err := r.triple(id, p.param(1))
	return v.Scale(r.scale), err
}

func (r *stepReader) direction(id int) (solid.Vec, error) {
	_, p, err := r.entity(id, "DIRECTION")
	if err != nil {
		return solid.Vec{}, err
	}
	v, err := r.triple(id, p.param(1))
	if err == nil && v.Len() == 0 {
		return v, stepError(id, "zero direction")
	}
	return v.Unit(), err
}

func (r *stepReader) triple(id int, v stepValue) (solid.Vec, error) {
	if v.kind != stepList || len(v.list) < 2 || len(v.list) > 3 {
		return solid.Vec{}, stepError(id, "expected 2 or 3 coordinates")
	}
	var c [3]float64
	for i, x := range v.list {
		if x.kind != stepNumber {
			return solid.Vec{}, stepError(id, "coordinate is not a number")
		}
		c[i] = x.num
	}
	return solid.Vec{X: c[0], Y: c[1], Z: c[2]}, nil
}

// placement reads an AXIS2_PLACEMENT_3D into a right-handed frame.
func (r *stepReader) placement(id int) (solid.Frame, error) {
	_, p, err := r.entity(id, "AXIS2_PLACEMENT_3D")
	if err != nil {
		return solid.Frame{}, err
	}
	o, err := r.point(p.param(1).ref)
	if err != nil {
		return solid.Frame{}, err
	}
	z, x := solid.Vec{Z: 1}, solid.Vec{X: 1}
	if len(p.params) > 2 && p.param(2).kind == stepRef {
		if z, err = r.direction(p.param(2).ref); err != nil {
			return solid.Frame{}, err
		}
	}
	if len(p.params) > 3 && p.param(3).kind == stepRef {
		if x, err = r.direction(p.param(3).ref); err != nil {
			return solid.Frame{}, err
		}
	}
	x = x.AddScaled(z, -x.Dot(z))
	if x.Len() < 1e-9 {
		x = solid.Vec{Y: 1}.AddScaled(z, -z.Y)
		if x.Len() < 1e-9 {
			x = solid.Vec{X: 1}.AddScaled(z, -z.X)
		}
	}
	x = x.Unit()
	return solid.Frame{Origin: o, X: x, Y: z.Cross(x), Z: z}, nil
}

func (r *stepReader) number(id int, v stepValue) (float64, error) {
	if v.kind == stepTyped && len(v.list) == 1 {
		v = v.list[0]
	}
	if v.kind != stepNumber {
		return 0, stepError(id, "expected a number")
	}
	return v.num, nil
}

// surfaceKinds are the surfaces read as the shapes they are; any other is SurfaceOther.
var surfaceKinds = map[string]solid.SurfaceKind{"PLANE": solid.SurfacePlane, "CYLINDRICAL_SURFACE": solid.SurfaceCylinder, "CONICAL_SURFACE": solid.SurfaceCone, "SPHERICAL_SURFACE": solid.SurfaceSphere, "TOROIDAL_SURFACE": solid.SurfaceTorus}

func (r *stepReader) surface(id int) (solid.Surface, error) {
	e, ok := r.f.entities[id]
	if !ok {
		return solid.Surface{}, stepError(id, "missing")
	}
	name := e.name()
	s := solid.Surface{Kind: solid.SurfaceOther, Name: name}
	if name == "" {
		s.Name = e.parts[0].name
	}
	k, ok := surfaceKinds[name]
	if !ok {
		return s, nil
	}
	p := e.parts[0]
	f, err := r.placement(p.param(1).ref)
	if err != nil {
		return s, err
	}
	s.Kind, s.Frame = k, f
	if len(p.params) > 2 {
		v, err := r.number(id, p.param(2))
		if err != nil {
			return s, err
		}
		s.Radius = float64(v * r.scale)
	}
	if k == solid.SurfaceTorus && len(p.params) > 3 {
		v, err := r.number(id, p.param(3))
		if err != nil {
			return s, err
		}
		s.Minor = float64(v * r.scale)
	}
	return s, nil
}

func (r *stepReader) curve(id int) (solid.Curve, error) {
	e, ok := r.f.entities[id]
	if !ok {
		return solid.Curve{}, stepError(id, "missing")
	}
	switch e.name() {
	case "SURFACE_CURVE", "SEAM_CURVE", "INTERSECTION_CURVE":
		return r.curve(e.parts[0].param(1).ref)
	case "LINE":
		p := e.parts[0]
		o, err := r.point(p.param(1).ref)
		if err != nil {
			return solid.Curve{}, err
		}
		_, vp, err := r.entity(p.param(2).ref, "VECTOR")
		if err != nil {
			return solid.Curve{}, err
		}
		d, err := r.direction(vp.param(1).ref)
		if err != nil {
			return solid.Curve{}, err
		}
		return solid.Curve{Kind: solid.CurveLine, Frame: solid.Frame{Origin: o, Z: d}, Name: "LINE"}, nil
	case "CIRCLE", "ELLIPSE":
		p := e.parts[0]
		f, err := r.placement(p.param(1).ref)
		if err != nil {
			return solid.Curve{}, err
		}
		c := solid.Curve{Kind: solid.CurveCircle, Frame: f, Name: e.name()}
		rad, err := r.number(id, p.param(2))
		if err != nil {
			return c, err
		}
		c.Radius = float64(rad * r.scale)
		if e.name() == "ELLIPSE" {
			minor, err := r.number(id, p.param(3))
			if err != nil {
				return c, err
			}
			c.Kind, c.Minor = solid.CurveEllipse, float64(minor*r.scale)
		}
		return c, nil
	}
	if e.is("B_SPLINE_CURVE_WITH_KNOTS") {
		return r.bspline(e)
	}
	return solid.Curve{Kind: solid.CurveOther, Name: e.parts[0].name}, nil
}

// bspline reads a B-spline curve, simple or complex (rational).
func (r *stepReader) bspline(e *stepEntity) (solid.Curve, error) {
	var degree, pts, mults, knots, weights stepValue
	if p, ok := e.part("B_SPLINE_CURVE"); ok {
		degree, pts = p.param(0), p.param(1)
		k, _ := e.part("B_SPLINE_CURVE_WITH_KNOTS")
		mults, knots = k.param(0), k.param(1)
		if w, ok := e.part("RATIONAL_B_SPLINE_CURVE"); ok {
			weights = w.param(0)
		}
	} else {
		p, _ := e.part("B_SPLINE_CURVE_WITH_KNOTS")
		if len(p.params) < 8 {
			return solid.Curve{}, stepError(e.id, "too few parameters")
		}
		degree, pts, mults, knots = p.param(1), p.param(2), p.param(6), p.param(7)
	}
	s := &solid.BSpline{Degree: int(degree.num)}
	for _, v := range pts.list {
		q, err := r.point(v.ref)
		if err != nil {
			return solid.Curve{}, err
		}
		s.Points = append(s.Points, q)
	}
	if len(mults.list) != len(knots.list) {
		return solid.Curve{}, stepError(e.id, "knot multiplicities do not match knots")
	}
	total := 0
	for _, m := range mults.list {
		if m.num < 1 || m.num > float64(len(s.Points)+s.Degree+1) {
			return solid.Curve{}, stepError(e.id, "inconsistent B-spline")
		}
		total += int(m.num)
	}
	if s.Degree < 1 || s.Degree > 32 || total != len(s.Points)+s.Degree+1 {
		return solid.Curve{}, stepError(e.id, "inconsistent B-spline")
	}
	for i, k := range knots.list {
		for range int(mults.list[i].num) {
			s.Knots = append(s.Knots, k.num)
		}
	}
	for _, w := range weights.list {
		s.Weights = append(s.Weights, w.num)
	}
	if len(s.Points) <= s.Degree || len(s.Knots) != len(s.Points)+s.Degree+1 || s.Weights != nil && len(s.Weights) != len(s.Points) {
		return solid.Curve{}, stepError(e.id, "inconsistent B-spline")
	}
	for _, w := range s.Weights {
		if !(w > 0) || math.IsInf(w, 0) {
			return solid.Curve{}, stepError(e.id, "B-spline weight is not positive")
		}
	}
	return solid.Curve{Kind: solid.CurveBSpline, BSpline: s, Name: "B_SPLINE_CURVE"}, nil
}
