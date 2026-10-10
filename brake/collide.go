package brake

import (
	"math"

	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/solid"
)

// Collision is two flanges of the finished part whose sheets run into each other.
type Collision struct {
	// Bends are the bends that join each flange to its parent, the flange whose sample was found inside the other first; the root flange has no bend and is left out.
	Bends []int
	// A and B are where the sheets meet, in the flat: the sample of the first flange that lies inside the second's sheet, and the sample of the second it lies over.
	A, B geometry.Point
}

// Collisions folds every bend to its angle and reports each pair of flanges whose sheets run into each other once the part is finished: a sample of one lies inside the other's sheet, within half the thickness (less the clearance) of one of its samples along the sheet's normal there and within half the sample spacing across it, so an overlap shallower than the spacing can go unseen. A flange and its parent are joined by their bend and meet nowhere else, so they are not compared; material the sampling cut off from the part (a web narrower than the spacing) is not folded and is left out. Only the finished part is checked; a flange that sweeps through another while a bend forms depends on the bend order, which the tooling search chooses.
func (m *Model) Collisions() []Collision {
	pos, nrm := m.fold()
	inside := float64(m.t/2) - float64(toolClearance*m.t)
	across := m.spacing / 2
	reach := math.Hypot(inside, across)
	// Each flange's plane and box: a sample can only lie inside a flange whose plane it is within reach of (the flange's samples leave its plane only on its arcs, by less than their middle radius) and whose box it is in. Most samples are near no other flange and are never looked up.
	type flange struct {
		o, n   solid.Vec
		slack  float64
		lo, hi solid.Vec
	}
	fl := make([]flange, m.nreg)
	for r := range fl {
		fl[r].lo, fl[r].hi = solid.Vec{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}, solid.Vec{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
		fl[r].n = m.frameBuf[r].rotate(zAxis)
		fl[r].o = m.frameBuf[r].t
		for _, bi := range m.kids[r] {
			fl[r].slack = max(fl[r].slack, m.arcBuf[bi].rm)
		}
		if bi := m.parentBend[r]; bi >= 0 {
			fl[r].slack = max(fl[r].slack, m.arcBuf[bi].rm)
		}
	}
	for i, p := range pos {
		f := &fl[m.region[i]]
		f.lo = solid.Vec{X: min(f.lo.X, p.X), Y: min(f.lo.Y, p.Y), Z: min(f.lo.Z, p.Z)}
		f.hi = solid.Vec{X: max(f.hi.X, p.X), Y: max(f.hi.Y, p.Y), Z: max(f.hi.Z, p.Z)}
	}
	joined := func(r, s int) bool { return m.parentRegion[r] == s || m.parentRegion[s] == r }
	// Samples are hashed into cells as large as the reach, chained per cell, so a probe looks only at its own cell and the neighbors.
	key := func(p solid.Vec) [3]int {
		return [3]int{int(math.Floor(p.X / reach)), int(math.Floor(p.Y / reach)), int(math.Floor(p.Z / reach))}
	}
	head := make(map[[3]int]int32, len(pos))
	next := make([]int32, len(pos))
	for i, p := range pos {
		k := key(p)
		if h, ok := head[k]; ok {
			next[i] = h
		} else {
			next[i] = -1
		}
		head[k] = int32(i)
	}
	seen := map[[2]int]bool{}
	var out []Collision
	for i, p := range pos {
		ri := m.region[i]
		if !m.inTree(ri) {
			continue
		}
		near := false
		for r := range fl {
			if r == ri || joined(ri, r) || !m.inTree(r) {
				continue
			}
			f := &fl[r]
			if p.X < f.lo.X-reach || p.X > f.hi.X+reach || p.Y < f.lo.Y-reach || p.Y > f.hi.Y+reach || p.Z < f.lo.Z-reach || p.Z > f.hi.Z+reach {
				continue
			}
			if math.Abs(p.Sub(f.o).Dot(f.n)) <= reach+f.slack {
				near = true
				break
			}
		}
		if !near {
			continue
		}
		k := key(p)
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for dz := -1; dz <= 1; dz++ {
					h, ok := head[[3]int{k[0] + dx, k[1] + dy, k[2] + dz}]
					if !ok {
						continue
					}
					for j := int(h); j >= 0; j = int(next[j]) {
						rj := m.region[j]
						if rj == ri || joined(ri, rj) || !m.inTree(rj) || seen[[2]int{ri, rj}] {
							continue
						}
						d := p.Sub(pos[j])
						along := d.Dot(nrm[j])
						if math.Abs(along) > inside || d.Sub(nrm[j].Scale(along)).Len() > across {
							continue
						}
						seen[[2]int{ri, rj}], seen[[2]int{rj, ri}] = true, true
						var bends []int
						for _, r := range []int{ri, rj} {
							if bi := m.parentBend[r]; bi >= 0 {
								bends = append(bends, bi)
							}
						}
						out = append(out, Collision{Bends: bends, A: m.pts[i], B: m.pts[j]})
					}
				}
			}
		}
	}
	return out
}

// fold places every sample with the bends at their angles and gives each the normal of the sheet there: the flange's, turned by the arc for a sample inside one. The frames and arcs it used stay in the Model's buffers.
func (m *Model) fold() (pos, nrm []solid.Vec) {
	angles := make([]float64, len(m.bends))
	for i, b := range m.bends {
		angles[i] = b.Angle * math.Pi / 180
	}
	pos, frames, _ := m.state(angles, -1)
	arcs := m.arcBuf
	nrm = make([]solid.Vec, len(pos))
	for i, p := range m.pts {
		r := m.region[i]
		v := solid.Vec{X: p.X, Y: p.Y, Z: 0}
		placed := false
		if bi := m.parentBend[r]; bi >= 0 {
			a := arcs[bi]
			if u := v.Sub(a.ax.o).Dot(a.ax.n); u <= a.h {
				nrm[i], placed = frames[m.parentRegion[r]].rotate(a.normal(u)), true
			}
		}
		for _, bi := range m.kids[r] {
			if placed {
				break
			}
			a := arcs[bi]
			if u := v.Sub(a.ax.o).Dot(a.ax.n); u >= -a.h && m.within(bi, v) {
				nrm[i], placed = frames[r].rotate(a.normal(u)), true
			}
		}
		if !placed {
			nrm[i] = frames[r].rotate(zAxis)
		}
	}
	return pos, nrm
}
