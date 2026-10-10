package main

import (
	"fmt"
	"math"
	"strings"
)

// plateModel is a sheet-metal part as a tree of flat rectangular plates joined by bends: a base, and for each bend a child plate hanging off a span of one of its parent's edges. Unlike sheetModel it can branch, which is what a flat pattern needs to overlap itself. Products are converted before they are added so no CPU fuses them (see AGENTS.md).
type plateModel struct {
	T    float64 // thickness
	A, B float64 // the base: A along x, B along y, lying in z = 0..T
	// Bends, in order; each parent index refers to an earlier plate (0 is the base).
	Bends []plateBend
}

// plateBend hangs a plate Length long off its parent's edge (0: the +y edge, 1: +x, 2: -y, 3: -x), over the span S0..S1 along that edge measured from the edge's start, bent Angle degrees toward the parent's +z side (its inner skin) at inside radius R.
type plateBend struct {
	Parent, Edge     int
	S0, S1, Angle, R float64
	Length           float64
}

// plate is a rectangle a x b in its own frame: outer skin at z = 0, material to z = T along n.
type plate struct {
	o, u, v, n v3
	a, b       float64
}

func (p plate) at(x, y, z float64) v3 {
	return p.o.add(p.u.scale(x)).add(p.v.scale(y)).add(p.n.scale(z))
}

func (a v3) add(b v3) v3        { return v3{a.x + b.x, a.y + b.y, a.z + b.z} }
func (a v3) scale(k float64) v3 { return v3{float64(a.x * k), float64(a.y * k), float64(a.z * k)} }
func (a v3) neg() v3            { return v3{-a.x, -a.y, -a.z} }
func (a v3) cross(b v3) v3 {
	return v3{float64(a.y*b.z) - float64(a.z*b.y), float64(a.z*b.x) - float64(a.x*b.z), float64(a.x*b.y) - float64(a.y*b.x)}
}

// edgeFrame is one edge of a plate in its local 2D: start point, direction (unit) and outward normal.
type edgeFrame struct {
	p0, e, out [2]float64
	length     float64
}

func (p plate) edge(i int) edgeFrame {
	switch i {
	case 0:
		return edgeFrame{[2]float64{0, p.b}, [2]float64{1, 0}, [2]float64{0, 1}, p.a}
	case 1:
		return edgeFrame{[2]float64{p.a, p.b}, [2]float64{0, -1}, [2]float64{1, 0}, p.b}
	case 2:
		return edgeFrame{[2]float64{p.a, 0}, [2]float64{-1, 0}, [2]float64{0, -1}, p.a}
	default:
		return edgeFrame{[2]float64{0, 0}, [2]float64{0, 1}, [2]float64{-1, 0}, p.b}
	}
}

// local is the local 2D point s along the edge.
func (e edgeFrame) local(s float64) [2]float64 {
	return [2]float64{float64(e.e[0]*s) + e.p0[0], float64(e.e[1]*s) + e.p0[1]}
}

// bytes builds the STEP file of the model.
func (m plateModel) bytes() []byte {
	w := &stepWriter{}
	b := &plateBuilder{w: w, m: m}
	solid := b.build()
	mm := w.add("(LENGTH_UNIT()NAMED_UNIT(*)SI_UNIT(.MILLI.,.METRE.))")
	rad := w.add("(NAMED_UNIT(*)PLANE_ANGLE_UNIT()SI_UNIT($,.RADIAN.))")
	sr := w.add("(NAMED_UNIT(*)SI_UNIT($,.STERADIAN.)SOLID_ANGLE_UNIT())")
	ctx := w.add("(GEOMETRIC_REPRESENTATION_CONTEXT(3)GLOBAL_UNIT_ASSIGNED_CONTEXT((#%d,#%d,#%d))REPRESENTATION_CONTEXT('',''))", mm, rad, sr)
	w.add("ADVANCED_BREP_SHAPE_REPRESENTATION('',(#%d),#%d)", solid, ctx)
	return []byte("ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION(('generated sheet-metal fixture'),'2;1');\nFILE_NAME('fixture.step','2026-10-09T00:00:00',(''),(''),'dfm fixtures/gen','','');\nFILE_SCHEMA(('AUTOMOTIVE_DESIGN { 1 0 10303 214 1 1 1 1 }'));\nENDSEC;\nDATA;\n" + w.b.String() + "ENDSEC;\nEND-ISO-10303-21;\n")
}

type plateBuilder struct {
	w      *stepWriter
	m      plateModel
	plates []plate
	// spans[plate][edge] lists the bends on that edge.
	spans map[[2]int][]int
	// edge entities, created once and shared between the faces that meet on them
	segs  map[segKey]stepEdge
	verts map[vertKey]stepEdge
	faces []int
}

type segKey struct {
	plate, edge int
	s0, s1      float64
	inner       bool
}

type vertKey struct {
	plate int
	x, y  float64
}

func (b *plateBuilder) build() int {
	m := b.m
	b.plates = []plate{{o: v3{}, u: v3{1, 0, 0}, v: v3{0, 1, 0}, n: v3{0, 0, 1}, a: m.A, b: m.B}}
	b.spans = map[[2]int][]int{}
	b.segs = map[segKey]stepEdge{}
	b.verts = map[vertKey]stepEdge{}
	// Place every child plate.
	for i, bd := range m.Bends {
		p := b.plates[bd.Parent]
		e := p.edge(bd.Edge)
		th := bd.Angle * math.Pi / 180
		sin, cos := math.Sin(th), math.Cos(th)
		outW := p.u.scale(e.out[0]).add(p.v.scale(e.out[1]))
		eW := p.u.scale(e.e[0]).add(p.v.scale(e.e[1]))
		// The bend axis lies T + R above the outer skin over the edge; the child's outer skin leaves the arc at angle th.
		l0 := e.local(bd.S0)
		axis := p.at(l0[0], l0[1], m.T+bd.R)
		radial := outW.scale(sin).add(p.n.scale(-cos))
		o3 := axis.add(radial.scale(bd.R + m.T))
		child := plate{o: o3, u: eW, v: outW.scale(cos).add(p.n.scale(sin)), n: outW.scale(-sin).add(p.n.scale(cos)), a: bd.S1 - bd.S0, b: bd.Length}
		b.plates = append(b.plates, child)
		b.spans[[2]int{bd.Parent, bd.Edge}] = append(b.spans[[2]int{bd.Parent, bd.Edge}], i)
	}
	for pi := range b.plates {
		b.plateFaces(pi)
	}
	for i := range m.Bends {
		b.bendFaces(i)
	}
	var refs []string
	for _, f := range b.faces {
		refs = append(refs, fmt.Sprintf("#%d", f))
	}
	shell := b.w.add("CLOSED_SHELL('',(%s))", strings.Join(refs, ","))
	return b.w.add("MANIFOLD_SOLID_BREP('part',#%d)", shell)
}

// segments splits plate pi's edge ei at its bends: each piece is (s0, s1, bend index or -1).
func (b *plateBuilder) segments(pi, ei int) [][3]float64 {
	e := b.plates[pi].edge(ei)
	var cuts []float64
	for _, bi := range b.spans[[2]int{pi, ei}] {
		cuts = append(cuts, b.m.Bends[bi].S0, b.m.Bends[bi].S1)
	}
	cuts = append(cuts, 0, e.length)
	sortFloats(cuts)
	var out [][3]float64
	for k := 0; k+1 < len(cuts); k++ {
		s0, s1 := cuts[k], cuts[k+1]
		if s1-s0 < 1e-9 {
			continue
		}
		bend := -1.0
		for _, bi := range b.spans[[2]int{pi, ei}] {
			if b.m.Bends[bi].S0 <= s0+1e-9 && s1 <= b.m.Bends[bi].S1+1e-9 {
				bend = float64(bi)
			}
		}
		out = append(out, [3]float64{s0, s1, bend})
	}
	return out
}

func sortFloats(xs []float64) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// seg is the skin edge of plate pi along edge ei from s0 to s1, on the inner (z = T) or outer skin.
func (b *plateBuilder) seg(pi, ei int, s0, s1 float64, inner bool) stepEdge {
	k := segKey{pi, ei, s0, s1, inner}
	if e, ok := b.segs[k]; ok {
		return e
	}
	p := b.plates[pi]
	e := p.edge(ei)
	z := 0.0
	if inner {
		z = b.m.T
	}
	a, c := e.local(s0), e.local(s1)
	b.segs[k] = b.w.line(p.at(a[0], a[1], z), p.at(c[0], c[1], z))
	return b.segs[k]
}

// vert is the edge through the thickness at local point (x, y) of plate pi.
func (b *plateBuilder) vert(pi int, x, y float64) stepEdge {
	k := vertKey{pi, math.Round(x*1e6) / 1e6, math.Round(y*1e6) / 1e6}
	if e, ok := b.verts[k]; ok {
		return e
	}
	p := b.plates[pi]
	b.verts[k] = b.w.line(p.at(x, y, 0), p.at(x, y, b.m.T))
	return b.verts[k]
}

// plateFaces writes plate pi's two skins and the side faces of its free edge pieces.
func (b *plateBuilder) plateFaces(pi int) {
	p := b.plates[pi]
	w := b.w
	var outer, inner []stepEdge
	for ei := 0; ei < 4; ei++ {
		e := p.edge(ei)
		for _, sg := range b.segments(pi, ei) {
			s0, s1 := sg[0], sg[1]
			outer = append(outer, b.seg(pi, ei, s0, s1, false))
			inner = append(inner, b.seg(pi, ei, s0, s1, true))
			if sg[2] >= 0 {
				continue
			}
			// A plate's root edge (edge 2 of a child) continues into its bend; its other free pieces get side faces.
			if pi > 0 && ei == 2 {
				continue
			}
			l0, l1 := e.local(s0), e.local(s1)
			outW := p.u.scale(e.out[0]).add(p.v.scale(e.out[1]))
			eW := p.u.scale(e.e[0]).add(p.v.scale(e.e[1]))
			b.faces = append(b.faces, w.face(w.plane(p.at(l0[0], l0[1], 0), outW, eW), true, []stepEdge{b.seg(pi, ei, s0, s1, false), b.vert(pi, l1[0], l1[1]), b.seg(pi, ei, s0, s1, true), b.vert(pi, l0[0], l0[1])}))
		}
	}
	b.faces = append(b.faces, w.face(w.plane(p.at(0, 0, 0), p.n.neg(), p.u), true, outer))
	b.faces = append(b.faces, w.face(w.plane(p.at(0, 0, b.m.T), p.n, p.u), true, inner))
}

// bendFaces writes bend bi's two cylinders and two end faces.
func (b *plateBuilder) bendFaces(bi int) {
	w := b.w
	bd := b.m.Bends[bi]
	p := b.plates[bd.Parent]
	child := b.plates[bi+1]
	e := p.edge(bd.Edge)
	outW := p.u.scale(e.out[0]).add(p.v.scale(e.out[1]))
	eW := p.u.scale(e.e[0]).add(p.v.scale(e.e[1]))
	T, R := b.m.T, bd.R
	arcs := func(s float64) (outerArc, innerArc stepEdge) {
		l := e.local(s)
		axis := p.at(l[0], l[1], T+R)
		cs := s - bd.S0 // along the child's root edge
		return w.arc(axis, eW, outW, R+T, p.at(l[0], l[1], 0), child.at(cs, 0, 0)), w.arc(axis, eW, outW, R, p.at(l[0], l[1], T), child.at(cs, 0, T))
	}
	o0, i0 := arcs(bd.S0)
	o1, i1 := arcs(bd.S1)
	rootOuter, rootInner := b.seg(bi+1, 2, 0, child.a, false), b.seg(bi+1, 2, 0, child.a, true)
	l0 := e.local(bd.S0)
	axis := w.axis(p.at(l0[0], l0[1], T+R), eW, outW)
	b.faces = append(b.faces, w.face(w.add("CYLINDRICAL_SURFACE('',#%d,%s)", axis, stepNum(R+T)), true, []stepEdge{b.seg(bd.Parent, bd.Edge, bd.S0, bd.S1, false), o1, rootOuter, o0}))
	b.faces = append(b.faces, w.face(w.add("CYLINDRICAL_SURFACE('',#%d,%s)", w.axis(p.at(l0[0], l0[1], T+R), eW, outW), stepNum(R)), false, []stepEdge{b.seg(bd.Parent, bd.Edge, bd.S0, bd.S1, true), i1, rootInner, i0}))
	// End faces: planes across the bend at each end of the span, bounded by the arcs and the two edges through the thickness.
	for k, s := range []float64{bd.S0, bd.S1} {
		l := e.local(s)
		n := eW.neg()
		oa, ia := o0, i0
		cx := 0.0
		if k == 1 {
			n, oa, ia, cx = eW, o1, i1, child.a
		}
		b.faces = append(b.faces, w.face(w.plane(p.at(l[0], l[1], T+R), n, outW), true, []stepEdge{oa, b.vert(bi+1, cx, 0), ia, b.vert(bd.Parent, l[0], l[1])}))
	}
}
