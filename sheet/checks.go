package sheet

import (
	"math"
	"slices"

	"github.com/open-mrp/dfm/solid"
)

// reached reports whether one of the skins placed the face.
func reached(f *solid.Face, skins ...map[*solid.Face]*mapping) bool {
	for _, s := range skins {
		if s[f] != nil {
			return true
		}
	}
	return false
}

// climbing returns an edge between two faces on neither skin that does not lie in a plane: wall edges are lines, or flat curves where walls meet (holes, outlines, whichever way a flange faces), so a curve that twists out of every plane is a thread's helix or a sloped feature.
func (u *unfolder) climbing(skins ...map[*solid.Face]*mapping) *solid.Edge {
	onSkin := func(f *solid.Face) bool { return reached(f, skins...) }
	for _, f := range u.body.Faces {
		for _, l := range f.Loops {
			for _, e := range l.Edges {
				if e.Curve.Kind == solid.CurveLine || e.Curve.Kind == solid.CurveCircle || slices.ContainsFunc(u.faces[e], onSkin) {
					continue
				}
				if !planar(e) {
					return e
				}
			}
		}
	}
	return nil
}

// planar reports whether the edge lies in one plane, within tol.
func planar(e *solid.Edge) bool {
	const k = 16
	pts := make([]solid.Vec, k+1)
	for i := range pts {
		pts[i] = e.Point(float64(i) / k)
	}
	a := pts[0]
	var n solid.Vec
	for i := 1; i <= k && n.Len() < 1e-9; i++ {
		for j := i + 1; j <= k; j++ {
			if c := pts[i].Sub(a).Cross(pts[j].Sub(a)); c.Len() > 1e-6 {
				n = c.Unit()
				break
			}
		}
	}
	if n.Len() == 0 {
		return true // straight
	}
	for _, q := range pts {
		if math.Abs(q.Sub(a).Dot(n)) > tol {
			return false
		}
	}
	return true
}

// inside returns a flat face on neither skin that lies parallel to the skin and part way through the sheet, with its distance from the skin.
func (u *unfolder) inside(skins ...map[*solid.Face]*mapping) (*solid.Face, float64) {
	for _, g := range u.body.Faces {
		if g.Surface.Kind != solid.SurfacePlane || reached(g, skins...) {
			continue
		}
		for _, f := range u.order {
			if f.Surface.Kind != solid.SurfacePlane || math.Abs(f.Surface.Frame.Z.Dot(g.Surface.Frame.Z)) < parallelCos {
				continue
			}
			d := math.Abs(g.Surface.Frame.Origin.Sub(f.Surface.Frame.Origin).Dot(f.Surface.Frame.Z))
			if d > tol && d < u.t-tol && u.overlap(f, g) {
				return g, d
			}
		}
	}
	return nil, 0
}

// unreached returns a flat face that neither skin reached although another lies opposite it at the sheet's thickness: a flange beyond a bend that is not a cylinder (sharp, or a free-form surface).
func (u *unfolder) unreached(skins ...map[*solid.Face]*mapping) *solid.Face {
	for _, f := range u.body.Faces {
		if f.Surface.Kind != solid.SurfacePlane || reached(f, skins...) {
			continue
		}
		n := f.Normal(f.Surface.Frame.Origin)
		for _, g := range u.body.Faces {
			if g == f || g.Surface.Kind != solid.SurfacePlane || reached(g, skins...) || g.Normal(g.Surface.Frame.Origin).Dot(n) > -parallelCos {
				continue
			}
			if d := f.Surface.Frame.Origin.Sub(g.Surface.Frame.Origin).Dot(n); math.Abs(d-u.t) < tol && u.overlap(f, g) {
				return f
			}
		}
	}
	return nil
}

// continued returns a flat face of the skin and a face beside it, on neither skin, that leaves it at less than 45°: the sheet goes on there (a formed feature or a bend that is not a cylinder), where a wall would leave it at about 90°.
func (u *unfolder) continued(skins ...map[*solid.Face]*mapping) (*solid.Face, *solid.Face) {
	for _, f := range u.order {
		if f.Surface.Kind != solid.SurfacePlane {
			continue
		}
		n := f.Normal(f.Surface.Frame.Origin)
		for _, l := range f.Loops {
			for _, e := range l.Edges {
				for _, g := range u.faces[e] {
					if g == f || reached(g, skins...) {
						continue
					}
					if v, ok := u.leaving(g, e, f); ok && math.Abs(v.Dot(n)) < continuesCos {
						return f, g
					}
				}
			}
		}
	}
	return nil, nil
}

// continuesCos is the cosine of 45°: a face leaving a flat face at less than that angle continues the sheet, where a wall leaves it at about 90°.
var continuesCos = math.Sqrt2 / 2

// leaving is the direction face g leaves edge e of flat face f in: toward the nearest of its points off f's plane, across the edge.
func (u *unfolder) leaving(g *solid.Face, e *solid.Edge, f *solid.Face) (solid.Vec, bool) {
	o, n := f.Surface.Frame.Origin, f.Surface.Frame.Z
	a, b := e.Start, e.End
	if e.Closed() || a.Near(b, tol) {
		return solid.Vec{}, false
	}
	mid := e.Point(0.5)
	dir := b.Sub(a).Unit()
	if e.Curve.Kind != solid.CurveLine {
		dir = e.Point(0.501).Sub(e.Point(0.499)).Unit()
	}
	best, found := math.Inf(1), solid.Vec{}
	for _, q := range u.samples(g) {
		if math.Abs(q.Sub(o).Dot(n)) < tol {
			continue
		}
		d := q.Sub(mid)
		across := d.AddScaled(dir, -d.Dot(dir))
		if l := across.Len(); l > tol && d.Len() < best {
			best, found = d.Len(), across.Unit()
		}
	}
	return found, best < math.Inf(1)
}

// overlap reports whether two parallel flat faces overlap, seen along their normal (by their bounding boxes).
func (u *unfolder) overlap(f, g *solid.Face) bool {
	fr := f.Surface.Frame
	box := func(h *solid.Face) [4]float64 {
		b := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		for _, q := range u.samples(h) {
			x, y := q.Sub(fr.Origin).Dot(fr.X), q.Sub(fr.Origin).Dot(fr.Y)
			b = [4]float64{min(b[0], x), min(b[1], y), max(b[2], x), max(b[3], y)}
		}
		return b
	}
	a, b := box(f), box(g)
	return min(a[2], b[2])-max(a[0], b[0]) > tol && min(a[3], b[3])-max(a[1], b[1]) > tol
}
