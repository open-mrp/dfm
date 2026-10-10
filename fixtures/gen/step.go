package main

import (
	"fmt"
	"math"
	"strings"
)

// sheetModel is a strip of sheet metal W wide (along x) and T thick, lying in z = 0..T for its first flange (y = 0..A), then, when Angle is set, bent up about an axis along x at inside radius R into a second flange B long. Holes go through the first flange.
type sheetModel struct {
	W, A, T     float64
	Angle, R, B float64      // degrees; no bend when Angle is 0
	Holes       [][3]float64 // x, y, radius
	// Pockets are blind round pockets from the top face: x, y, radius, depth.
	Pockets [][4]float64
	// DX moves the solid along x, for files holding several.
	DX float64
}

// stepWriter numbers STEP entities as they are added.
type stepWriter struct {
	b strings.Builder
	n int
}

func (w *stepWriter) add(format string, args ...any) int {
	w.n++
	fmt.Fprintf(&w.b, "#%d=%s;\n", w.n, fmt.Sprintf(format, args...))
	return w.n
}

func stepNum(f float64) string {
	if math.Abs(f) < 1e-12 {
		f = 0
	}
	s := fmt.Sprintf("%.10g", f)
	if !strings.ContainsAny(s, ".e") {
		s += "."
	}
	return s
}

type v3 struct{ x, y, z float64 }

func (w *stepWriter) point(p v3) int {
	return w.add("CARTESIAN_POINT('',(%s,%s,%s))", stepNum(p.x), stepNum(p.y), stepNum(p.z))
}

func (w *stepWriter) dir(p v3) int {
	return w.add("DIRECTION('',(%s,%s,%s))", stepNum(p.x), stepNum(p.y), stepNum(p.z))
}

func (w *stepWriter) axis(o, z, x v3) int {
	return w.add("AXIS2_PLACEMENT_3D('',#%d,#%d,#%d)", w.point(o), w.dir(z), w.dir(x))
}

type stepEdge struct{ id int }

func (w *stepWriter) vertex(p v3) int { return w.add("VERTEX_POINT('',#%d)", w.point(p)) }

// line is an edge from a to b.
func (w *stepWriter) line(a, b v3) stepEdge {
	d := v3{b.x - a.x, b.y - a.y, b.z - a.z}
	l := math.Sqrt(d.x*d.x + d.y*d.y + d.z*d.z)
	vec := w.add("VECTOR('',#%d,%s)", w.dir(v3{d.x / l, d.y / l, d.z / l}), stepNum(l))
	c := w.add("LINE('',#%d,#%d)", w.point(a), vec)
	return stepEdge{w.add("EDGE_CURVE('',#%d,#%d,#%d,.T.)", w.vertex(a), w.vertex(b), c)}
}

// arc is an edge counter-clockwise about axis z through centre o, from a to b (a full circle when a is b).
func (w *stepWriter) arc(o, z, x v3, r float64, a, b v3) stepEdge {
	c := w.add("CIRCLE('',#%d,%s)", w.axis(o, z, x), stepNum(r))
	va := w.vertex(a)
	vb := va
	if a != b {
		vb = w.vertex(b)
	}
	return stepEdge{w.add("EDGE_CURVE('',#%d,#%d,#%d,.T.)", va, vb, c)}
}

// face is a face on surface s bounded by loops of edges, the first outer.
func (w *stepWriter) face(s int, sense bool, loops ...[]stepEdge) int {
	var bounds []string
	for i, l := range loops {
		var oes []string
		for _, e := range l {
			oes = append(oes, fmt.Sprintf("#%d", w.add("ORIENTED_EDGE('',*,*,#%d,.T.)", e.id)))
		}
		loop := w.add("EDGE_LOOP('',(%s))", strings.Join(oes, ","))
		kind := "FACE_BOUND"
		if i == 0 {
			kind = "FACE_OUTER_BOUND"
		}
		bounds = append(bounds, fmt.Sprintf("#%d", w.add("%s('',#%d,.T.)", kind, loop)))
	}
	return w.add("ADVANCED_FACE('',(%s),#%d,%s)", strings.Join(bounds, ","), s, map[bool]string{true: ".T.", false: ".F."}[sense])
}

func (w *stepWriter) plane(o, n, x v3) int { return w.add("PLANE('',#%d)", w.axis(o, n, x)) }

// bytes builds the STEP file of the model.
func (m sheetModel) bytes() []byte { return stepFile(m) }

// stepFile builds a STEP file holding the models, each a solid.
func stepFile(models ...sheetModel) []byte {
	w := &stepWriter{}
	var breps []string
	for _, m := range models {
		breps = append(breps, fmt.Sprintf("#%d", m.solid(w)))
	}
	mm := w.add("(LENGTH_UNIT()NAMED_UNIT(*)SI_UNIT(.MILLI.,.METRE.))")
	rad := w.add("(NAMED_UNIT(*)PLANE_ANGLE_UNIT()SI_UNIT($,.RADIAN.))")
	sr := w.add("(NAMED_UNIT(*)SI_UNIT($,.STERADIAN.)SOLID_ANGLE_UNIT())")
	ctx := w.add("(GEOMETRIC_REPRESENTATION_CONTEXT(3)GLOBAL_UNIT_ASSIGNED_CONTEXT((#%d,#%d,#%d))REPRESENTATION_CONTEXT('',''))", mm, rad, sr)
	w.add("ADVANCED_BREP_SHAPE_REPRESENTATION('',(%s),#%d)", strings.Join(breps, ","), ctx)
	return []byte("ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION(('generated sheet-metal fixture'),'2;1');\nFILE_NAME('fixture.step','2026-10-09T00:00:00',(''),(''),'dfm fixtures/gen','','');\nFILE_SCHEMA(('AUTOMOTIVE_DESIGN { 1 0 10303 214 1 1 1 1 }'));\nENDSEC;\nDATA;\n" + w.b.String() + "ENDSEC;\nEND-ISO-10303-21;\n")
}

// solid writes the model's solid and returns its entity.
func (m sheetModel) solid(w *stepWriter) int {
	X, Y, Z := v3{1, 0, 0}, v3{0, 1, 0}, v3{0, 0, 1}
	neg := func(a v3) v3 { return v3{-a.x, -a.y, -a.z} }
	at := func(x float64, yz [2]float64) v3 { return v3{x + m.DX, yz[0], yz[1]} }
	th := m.Angle * math.Pi / 180
	sin, cos := math.Sin(th), math.Cos(th)
	c := [2]float64{m.A, m.T + m.R} // bend axis
	onArc := func(r float64) [2]float64 { return [2]float64{c[0] + r*sin, c[1] - r*cos} }
	d := [2]float64{cos, sin}
	// Cross-section corners: outer (bottom) and inner (top) side.
	o1, o2, i1, i2 := [2]float64{0, 0}, [2]float64{m.A, 0}, [2]float64{0, m.T}, [2]float64{m.A, m.T}
	o3, i3 := onArc(m.R+m.T), onArc(m.R)
	o4 := [2]float64{o3[0] + m.B*d[0], o3[1] + m.B*d[1]}
	i4 := [2]float64{i3[0] + m.B*d[0], i3[1] + m.B*d[1]}
	bent := m.Angle != 0
	along := func(p [2]float64) stepEdge { return w.line(at(0, p), at(m.W, p)) }
	type side struct {
		oA, iA, endA, oArc, iArc, oB, iB, endB stepEdge
	}
	var sides [2]side
	for k, x := range []float64{0, m.W} {
		s := &sides[k]
		s.oA, s.iA, s.endA = w.line(at(x, o1), at(x, o2)), w.line(at(x, i1), at(x, i2)), w.line(at(x, o1), at(x, i1))
		if bent {
			cx := at(x, c)
			s.oArc = w.arc(cx, X, Y, m.R+m.T, at(x, o2), at(x, o3))
			s.iArc = w.arc(cx, X, Y, m.R, at(x, i2), at(x, i3))
			s.oB, s.iB, s.endB = w.line(at(x, o3), at(x, o4)), w.line(at(x, i3), at(x, i4)), w.line(at(x, o4), at(x, i4))
		} else {
			s.endB = w.line(at(x, o2), at(x, i2))
		}
	}
	lO1, lO2, lI1, lI2 := along(o1), along(o2), along(i1), along(i2)
	var top, bottom [][]stepEdge
	var holeFaces [][3]any
	for _, h := range m.Holes {
		h[0] += m.DX
		ct, cb := v3{h[0], h[1], m.T}, v3{h[0], h[1], 0}
		et := w.arc(ct, Z, X, h[2], v3{h[0] + h[2], h[1], m.T}, v3{h[0] + h[2], h[1], m.T})
		eb := w.arc(cb, Z, X, h[2], v3{h[0] + h[2], h[1], 0}, v3{h[0] + h[2], h[1], 0})
		top, bottom = append(top, []stepEdge{et}), append(bottom, []stepEdge{eb})
		holeFaces = append(holeFaces, [3]any{w.add("CYLINDRICAL_SURFACE('',#%d,%s)", w.axis(cb, Z, X), stepNum(h[2])), et, eb})
	}
	var faces []int
	for _, pk := range m.Pockets {
		x, y, r, depth := pk[0]+m.DX, pk[1], pk[2], pk[3]
		ct, cf := v3{x, y, m.T}, v3{x, y, m.T - depth}
		et := w.arc(ct, Z, X, r, v3{x + r, y, m.T}, v3{x + r, y, m.T})
		ef := w.arc(cf, Z, X, r, v3{x + r, y, m.T - depth}, v3{x + r, y, m.T - depth})
		top = append(top, []stepEdge{et})
		faces = append(faces, w.face(w.add("CYLINDRICAL_SURFACE('',#%d,%s)", w.axis(cf, Z, X), stepNum(r)), false, []stepEdge{et}, []stepEdge{ef}))
		faces = append(faces, w.face(w.plane(cf, Z, X), true, []stepEdge{ef}))
	}
	faces = append(faces, w.face(w.plane(at(0, o1), neg(Z), X), true, append([][]stepEdge{{sides[0].oA, lO2, sides[1].oA, lO1}}, bottom...)...))
	faces = append(faces, w.face(w.plane(at(0, i1), Z, X), true, append([][]stepEdge{{sides[0].iA, lI2, sides[1].iA, lI1}}, top...)...))
	faces = append(faces, w.face(w.plane(at(0, o1), neg(Y), X), true, []stepEdge{sides[0].endA, lI1, sides[1].endA, lO1}))
	for _, h := range holeFaces {
		faces = append(faces, w.face(h[0].(int), false, []stepEdge{h[1].(stepEdge)}, []stepEdge{h[2].(stepEdge)}))
	}
	if bent {
		lO3, lI3, lO4, lI4 := along(o3), along(i3), along(o4), along(i4)
		axis := w.axis(at(0, c), X, Y)
		faces = append(faces, w.face(w.add("CYLINDRICAL_SURFACE('',#%d,%s)", axis, stepNum(m.R+m.T)), true, []stepEdge{sides[0].oArc, lO3, sides[1].oArc, lO2}))
		faces = append(faces, w.face(w.add("CYLINDRICAL_SURFACE('',#%d,%s)", w.axis(at(0, c), X, Y), stepNum(m.R)), false, []stepEdge{sides[0].iArc, lI3, sides[1].iArc, lI2}))
		faces = append(faces, w.face(w.plane(at(0, o3), v3{0, sin, -cos}, X), true, []stepEdge{sides[0].oB, lO4, sides[1].oB, lO3}))
		faces = append(faces, w.face(w.plane(at(0, i3), v3{0, -sin, cos}, X), true, []stepEdge{sides[0].iB, lI4, sides[1].iB, lI3}))
		faces = append(faces, w.face(w.plane(at(0, o4), v3{0, d[0], d[1]}, X), true, []stepEdge{sides[0].endB, lI4, sides[1].endB, lO4}))
		for k, n := range []v3{neg(X), X} {
			s := sides[k]
			faces = append(faces, w.face(w.plane(at([]float64{0, m.W}[k], o1), n, Y), true, []stepEdge{s.oA, s.oArc, s.oB, s.endB, s.iB, s.iArc, s.iA, s.endA}))
		}
	} else {
		faces = append(faces, w.face(w.plane(at(0, o2), Y, X), true, []stepEdge{sides[0].endB, lI2, sides[1].endB, lO2}))
		for k, n := range []v3{neg(X), X} {
			s := sides[k]
			faces = append(faces, w.face(w.plane(at([]float64{0, m.W}[k], o1), n, Y), true, []stepEdge{s.oA, s.endB, s.iA, s.endA}))
		}
	}
	var refs []string
	for _, f := range faces {
		refs = append(refs, fmt.Sprintf("#%d", f))
	}
	shell := w.add("CLOSED_SHELL('',(%s))", strings.Join(refs, ","))
	return w.add("MANIFOLD_SOLID_BREP('part',#%d)", shell)
}
