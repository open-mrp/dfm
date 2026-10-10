package sheet

import (
	"fmt"
	"math"
	"slices"

	"github.com/open-mrp/dfm/solid"
)

// meshTriangles is how many triangular flat faces make a model look converted from a mesh.
const meshTriangles = 50

// unfoldBody unfolds one solid.
func unfoldBody(b *solid.Body, o Options) (*piece, error) {
	u := &unfolder{body: b, k: o.KFactor, faces: map[*solid.Edge][]*solid.Face{}, maps: map[*solid.Face]*mapping{}, samp: map[*solid.Face][]solid.Vec{}}
	triangles := 0
	for _, f := range u.body.Faces {
		for _, l := range f.Loops {
			for _, e := range l.Edges {
				if !slices.Contains(u.faces[e], f) {
					u.faces[e] = append(u.faces[e], f)
				}
			}
		}
		if f.Surface.Kind == solid.SurfacePlane && len(f.Loops) == 1 && len(f.Loops[0].Edges) == 3 {
			triangles++
		}
	}
	if triangles >= meshTriangles && 2*triangles >= len(u.body.Faces) {
		return nil, fmt.Errorf("%w: %d of the model's %d faces are triangles: it was converted from a mesh, which has no true curves or bends; export the part from the CAD model it was designed in", ErrNotSheetMetal, triangles, len(u.body.Faces))
	}
	base := u.largestPlane()
	if base == nil {
		return nil, fmt.Errorf("%w: the model has no flat face", ErrNotSheetMetal)
	}
	t, other, err := u.thickness(base)
	if err != nil {
		return nil, err
	}
	u.t = t
	if err := u.unroll(base); err != nil {
		return nil, err
	}
	// The other skin, unrolled the same way, shows whether any of the sheet was out of reach of both.
	v := &unfolder{body: u.body, k: u.k, t: t, faces: u.faces, maps: map[*solid.Face]*mapping{}, samp: u.samp}
	if err := v.unroll(other); err != nil {
		return nil, err
	}
	for _, skin := range []*unfolder{u, v} {
		if f, g := skin.continued(u.maps, v.maps); f != nil {
			return nil, fmt.Errorf("%w: face %s (%s) continues the sheet from face %s but is not a cylindrical bend; formed features and free-form bends cannot be unfolded", ErrNotSheetMetal, g.ID, g.Surface.Name, f.ID)
		}
	}
	if f := u.unreached(u.maps, v.maps); f != nil {
		return nil, fmt.Errorf("%w: face %s is not reached across a cylindrical bend from the rest of the sheet; a bend modeled sharp or as a free-form surface cannot be unfolded", ErrNotSheetMetal, f.ID)
	}
	if e := u.climbing(u.maps, v.maps); e != nil {
		return nil, fmt.Errorf("%w: edge %s twists out of every plane, as a modeled thread does; model a tapped hole as its plain hole and assign the tap", ErrNotSheetMetal, e.ID)
	}
	if g, d := u.inside(u.maps, v.maps); g != nil {
		return nil, fmt.Errorf("%w: face %s lies inside the sheet, %.4g mm from its face: the thickness varies there (a pocket, counterbore or step), which a part cut from flat sheet cannot have", ErrNotSheetMetal, g.ID, d)
	}
	return u.flat()
}

type unfolder struct {
	body  *solid.Body
	k, t  float64
	faces map[*solid.Edge][]*solid.Face // the faces each edge bounds
	maps  map[*solid.Face]*mapping      // the skin's faces, unrolled
	order []*solid.Face                 // the skin's faces, in unrolling order
	samp  map[*solid.Face][]solid.Vec   // sample points of each face, taken once (shared by both skins)
}

// Direction tests: cosines this close to 1 are parallel, and to 0 perpendicular, for face normals and axes the file computed in double precision; a line along an axis is aligned within a looser bound, as lines meet cylinders through rounding.
const (
	parallelCos      = 1 - 1e-9
	alignedCos       = 1 - 1e-6
	perpendicularCos = 1e-6
)

// unroll places the skin's faces, from the base face outward across tangent edges.
func (u *unfolder) unroll(base *solid.Face) error {
	fr := base.Surface.Frame
	n := base.Normal(fr.Origin)
	m := &mapping{face: base, O3: fr.Origin, U3: fr.X, V3: n.Cross(fr.X), U2: p2(1, 0), V2: p2(0, 1)}
	u.maps[base], u.order = m, []*solid.Face{base}
	for i := 0; i < len(u.order); i++ {
		f := u.order[i]
		from := u.maps[f]
		for _, l := range f.Loops {
			for _, e := range l.Edges {
				for _, g := range u.faces[e] {
					if g == f || u.maps[g] != nil {
						continue
					}
					next, err := u.across(from, g, e)
					if err != nil {
						return err
					}
					if next != nil {
						u.maps[g] = next
						u.order = append(u.order, g)
					}
				}
			}
		}
	}
	return nil
}

// across places face g from the placed face of from across their shared edge e, or returns nil when g is not on the same skin (the edge is not a smooth join).
func (u *unfolder) across(from *mapping, g *solid.Face, e *solid.Edge) (*mapping, error) {
	f := from.face
	switch {
	case f.Surface.Kind == solid.SurfacePlane && g.Surface.Kind == solid.SurfacePlane:
		if coplanar(f, g) {
			m := *from
			m.face = g
			return &m, nil
		}
	case f.Surface.Kind == solid.SurfacePlane && g.Surface.Kind == solid.SurfaceCylinder:
		if tangentLine(f, g, e) {
			return u.bendFrom(from, g, e)
		}
	case f.Surface.Kind == solid.SurfaceCylinder && g.Surface.Kind == solid.SurfacePlane:
		if tangentLine(g, f, e) {
			return u.planeFrom(from, g, e), nil
		}
	case f.Surface.Kind == solid.SurfaceCylinder && g.Surface.Kind == solid.SurfaceCylinder:
		if coaxial(f, g) && f.SameSense == g.SameSense {
			m := *from
			m.face = g
			m.phiMin, m.phiMax = u.phiRange(&m, g)
			return &m, nil
		}
	}
	return nil, nil
}

func coplanar(f, g *solid.Face) bool {
	nf, ng := f.Normal(f.Surface.Frame.Origin), g.Normal(g.Surface.Frame.Origin)
	return nf.Dot(ng) > parallelCos && math.Abs(g.Surface.Frame.Origin.Sub(f.Surface.Frame.Origin).Dot(nf)) < tol
}

func coaxial(f, g *solid.Face) bool {
	a, b := f.Surface.Frame, g.Surface.Frame
	d := b.Origin.Sub(a.Origin)
	off := d.AddScaled(a.Z, -d.Dot(a.Z)).Len()
	return math.Abs(a.Z.Dot(b.Z)) > parallelCos && off < tol && math.Abs(f.Surface.Radius-g.Surface.Radius) < tol
}

// tangentLine reports whether edge e joins flat face p smoothly to cylinder c: a line along the axis, where the plane touches the cylinder.
func tangentLine(p, c *solid.Face, e *solid.Edge) bool {
	if e.Curve.Kind != solid.CurveLine {
		return false
	}
	ax := c.Surface.Frame
	dir := e.End.Sub(e.Start).Unit()
	if math.Abs(dir.Dot(ax.Z)) < alignedCos {
		return false
	}
	n := p.Surface.Frame.Z
	if math.Abs(n.Dot(ax.Z)) > perpendicularCos {
		return false
	}
	dist := math.Abs(ax.Origin.Sub(p.Surface.Frame.Origin).Dot(n))
	return math.Abs(dist-c.Surface.Radius) < tol
}

// bendFrom places cylinder c from the flat face of from, across line e.
func (u *unfolder) bendFrom(from *mapping, c *solid.Face, e *solid.Edge) (*mapping, error) {
	ax := c.Surface.Frame
	l0 := e.Start
	d := l0.Sub(ax.Origin)
	r0 := d.AddScaled(ax.Z, -d.Dot(ax.Z)).Unit()
	t0 := ax.Z.Cross(r0)
	if s := u.side(c, ax, t0, e); s < 0 {
		t0 = t0.Scale(-1)
	}
	m := &mapping{face: c, cyl: true, O3: ax.Origin, U3: ax.Z, R0: r0, T0: t0}
	m.U2, m.V2 = from.linear(ax.Z), from.linear(t0)
	at := from.flat(l0)
	axial := d.Dot(ax.Z)
	m.O2 = p2(at.X-float64(m.U2.X*axial), at.Y-float64(m.U2.Y*axial))
	r := c.Surface.Radius
	m.convex = c.Normal(l0).Dot(r0) > 0
	m.innerR = r
	if m.convex {
		m.innerR = r - u.t
	}
	if m.innerR < -tol {
		return nil, fmt.Errorf("%w: bend %s is tighter than the thickness", ErrNotSheetMetal, c.ID)
	}
	m.innerR = max(m.innerR, 0)
	m.Rn = m.innerR + float64(u.k*u.t)
	m.phiMin, m.phiMax = u.phiRange(m, c)
	return m, nil
}

// side is positive when the cylinder face lies toward t0 from the line: judged at the middle of an edge that leaves the line (a bend's end arc).
func (u *unfolder) side(c *solid.Face, ax solid.Frame, t0 solid.Vec, line *solid.Edge) float64 {
	best := 0.0
	for _, l := range c.Loops {
		for _, e := range l.Edges {
			if e == line {
				continue
			}
			q := e.Point(0.5)
			d := q.Sub(ax.Origin)
			rq := d.AddScaled(ax.Z, -d.Dot(ax.Z)).Unit()
			s := rq.Dot(t0)
			if e.Curve.Kind != solid.CurveLine && math.Abs(s) > perpendicularCos {
				return s
			}
			if math.Abs(s) > math.Abs(best) {
				best = s
			}
		}
	}
	return best
}

// phiRange is the angle the cylinder face spans in m.
func (u *unfolder) phiRange(m *mapping, c *solid.Face) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, q := range u.samples(c) {
		a := m.phi(q)
		lo, hi = min(lo, a), max(hi, a)
	}
	return lo, hi
}

// planeFrom places flat face p from the cylinder of from, across line e.
func (u *unfolder) planeFrom(from *mapping, p *solid.Face, e *solid.Edge) *mapping {
	l0 := e.Start
	a := from.phi(l0)
	tan := from.R0.Scale(-math.Sin(a)).Add(from.T0.Scale(math.Cos(a)))
	sign := 1.0
	if a < (from.phiMin+from.phiMax)/2 {
		sign = -1
	}
	return &mapping{face: p, O3: l0, U3: from.U3, V3: tan.Scale(sign), O2: from.flat(l0), U2: from.U2, V2: p2(from.V2.X*sign, from.V2.Y*sign)}
}

// inner reports whether e joins two faces of the skin.
func (u *unfolder) inner(e *solid.Edge) bool {
	n := 0
	for _, g := range u.faces[e] {
		if u.maps[g] != nil {
			n++
		}
	}
	return n > 1
}
