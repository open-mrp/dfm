// Package brake simulates forming a flat part on a press brake: bending its bend lines one after another, in a sequence, and checking at each step whether the part runs into the punch or the die. It also folds the finished part and finds the flanges that run into each other (Collisions).
//
// The flat part is sampled on a grid and split into flanges along its bend lines (a tree, rooted at the largest flange). A state gives every bend an angle; each flange is placed by the rigid motions of the bends between it and the root, and the samples inside a bend's zone are wrapped around its arc. While a bend is formed, from flat to its angle in steps, a sample of the part inside the punch or the die is a collision, except the bend's own arc and, for the punch, the two flanges the bend joins: the tools form those. The bends formed earlier in the sequence are at their angles; the later ones are flat.
//
// The punch stands on the bisector of the bend so far, its tip in the inner arc. The die lies under the flange nearer the root, which rests on it, centered on the bend's middle. The punch may sit at three places along the bend line (its start, middle and end), as an operator slides the part along the tooling; the bend collides only when every place does, and with the punch or the die when that tool is hit at every place.
package brake

import (
	"errors"
	"math"
	"slices"

	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/solid"
)

// Tooling is the punch and die cross-section and length, in millimeters and degrees.
type Tooling struct {
	PunchTipAngle float64 // included angle of the punch tip
	PunchWidth    float64 // width of the punch body
	PunchHeight   float64 // height of the punch from its tip
	PunchLength   float64 // length of the punch along the bend; 0: segmented, as long as the bend
	// A gooseneck punch: above the nose, the body is set back by the throat's depth for its height. 0 for a straight punch.
	PunchNose, PunchThroatDepth, PunchThroatHeight float64
	DieOpening                                     float64 // width of the V opening at the die's top face
	DieAngle                                       float64 // included angle of the V; 0 matches each bend
	DieWidth                                       float64 // width of the die beside the opening, each side
	DieHeight                                      float64 // depth of the die below its top face
	DieLength                                      float64 // length of the die along the bend; 0: segmented, as long as the bend
}

// Bend is a bend line of the flat part and how it is formed.
type Bend struct {
	A, B         geometry.Point
	Angle        float64 // degrees from flat
	InsideRadius float64
	Up           bool // toward the viewer of the flat part
}

// Result is what forming one bend runs into, at the best place along it.
type Result struct {
	Punch, Die bool
	// Simulated is false when the bend line does not divide the part into two flanges, so there is nothing to form.
	Simulated bool
}

// Collides reports whether the bend runs into either tool.
func (r Result) Collides() bool { return r.Punch || r.Die }

// ErrEmpty is returned for a part with no material to sample.
var ErrEmpty = errors.New("brake: nothing to sample")

// stepDeg is the angle between the states a bend is checked at.
const stepDeg = 5

// minDieAngle is the sharpest V a die matched to its bend has (a bend folded flat is pressed in a sharp die, then flattened).
const minDieAngle = 10

// toolClearance is the share of the thickness a sample may come within a tool's face, less than half the sheet, before it is a collision: the sheet touching the tool is forming, not a collision.
const toolClearance = 0.05

// maxSequenceBends is the most bends a sequence is searched for: sets of formed bends are kept as bits of one word. No real part is near it.
const maxSequenceBends = 64

// Model is a flat part prepared for simulation. Its methods write into buffers it keeps between calls, so a Model is for one goroutine at a time; the rules build one per check.
type Model struct {
	bends        []Bend
	t, k         float64
	spacing      float64
	pts          []geometry.Point
	region       []int            // region of each sample
	parentBend   []int            // the bend between each region and its parent region; -1 for the root
	parentRegion []int            // each region's parent region; -1 for the root
	order        []int            // regions, root first
	side         []float64        // for each bend, the side its child lies on (+1 or -1 along its normal)
	child        []int            // the child region of each bend, or -1 when it divides nothing
	kids         [][]int          // the bends to each region's children
	axes         []bendAxes       // each bend's frame in the flat
	adj          map[int][][2]int // region -> (neighbor region, bend between)
	near         map[[2]int]int   // (region, bend) -> a sample of the region beside the bend line
	nreg         int
	// buffers state reuses between calls
	posBuf   []solid.Vec
	inArcBuf []bool
	frameBuf []frame
	arcBuf   []arc
	// moving lists, per bend, the samples its angle places: its child's subtree and the parent samples within its arc. Built on first use.
	moving [][]int32
}

type bendAxes struct {
	o, a, n solid.Vec // a point on the line, along it, and its normal toward the child
	length  float64
}

// segmentsCross reports whether segments p-q and a-b cross. A segment ending exactly on the other may count either way; a sample lying exactly on a bend line then joins one flange or neither, which at the sample spacing changes nothing.
func segmentsCross(p, q, a, b geometry.Point) bool {
	side := func(o, s, e geometry.Point) float64 {
		return float64((e.X-o.X)*(s.Y-o.Y)) - float64((e.Y-o.Y)*(s.X-o.X))
	}
	d1, d2 := side(a, b, p), side(a, b, q)
	d3, d4 := side(p, q, a), side(p, q, b)
	return (d1 > 0) != (d2 > 0) && (d3 > 0) != (d4 > 0)
}

// New samples the flat part every spacing mm and builds its flanges.
func New(part *geometry.Part, bends []Bend, thickness, kFactor, spacing float64) (*Model, error) {
	m := &Model{bends: bends, t: thickness, k: kFactor, spacing: spacing}
	var polys [][]geometry.Point
	for i := range part.Loops {
		polys = append(polys, part.LoopPolygon(i))
	}
	b := part.Bounds
	if b.IsEmpty() || len(polys) == 0 {
		return nil, ErrEmpty
	}
	cols := int(math.Ceil(b.WidthMM()/spacing)) + 1
	rows := int(math.Ceil(b.HeightMM()/spacing)) + 1
	index := make([]int, rows*cols)
	for i := range index {
		index[i] = -1
	}
	// crossings of the outline along each row and each column of samples
	rowCuts := make([][]float64, rows)
	colCuts := make([][]float64, cols)
	for _, poly := range polys {
		for k := range poly {
			p, q := poly[k], poly[(k+1)%len(poly)]
			for j := range rows {
				if y := b.MinY + float64((float64(j)+0.5)*spacing); (p.Y > y) != (q.Y > y) {
					rowCuts[j] = append(rowCuts[j], p.X+float64((y-p.Y)*(q.X-p.X))/(q.Y-p.Y))
				}
			}
			for i := range cols {
				if x := b.MinX + float64((float64(i)+0.5)*spacing); (p.X > x) != (q.X > x) {
					colCuts[i] = append(colCuts[i], p.Y+float64((x-p.X)*(q.Y-p.Y))/(q.X-p.X))
				}
			}
		}
	}
	for j := range rows {
		slices.Sort(rowCuts[j])
	}
	for i := range cols {
		slices.Sort(colCuts[i])
	}
	// cut reports whether the outline crosses between lo and hi.
	cut := func(cuts []float64, lo, hi float64) bool {
		k, _ := slices.BinarySearch(cuts, lo)
		return k < len(cuts) && cuts[k] <= hi
	}
	for j := range rows {
		y := b.MinY + float64((float64(j)+0.5)*spacing)
		for i := range cols {
			x := b.MinX + float64((float64(i)+0.5)*spacing)
			if k, _ := slices.BinarySearch(rowCuts[j], x); k%2 == 1 {
				index[j*cols+i] = len(m.pts)
				m.pts = append(m.pts, geometry.Point{X: x, Y: y})
			}
		}
	}
	if len(m.pts) == 0 {
		return nil, ErrEmpty
	}
	// Flanges: samples joined unless a bend line runs between them.
	uf := make([]int, len(m.pts))
	for i := range uf {
		uf[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for uf[i] != i {
			uf[i] = uf[uf[i]]
			i = uf[i]
		}
		return i
	}
	type link struct{ a, b, bend int }
	var crossings []link
	join := func(p, q int) {
		for bi, bd := range bends {
			if segmentsCross(m.pts[p], m.pts[q], bd.A, bd.B) {
				crossings = append(crossings, link{p, q, bi})
				return
			}
		}
		if rp, rq := find(p), find(q); rp != rq {
			uf[rp] = rq
		}
	}
	for j := 0; j < rows; j++ {
		for i := 0; i < cols; i++ {
			p := index[j*cols+i]
			if p < 0 {
				continue
			}
			// Neighbors join only through material: not across a gap in the outline narrower than the spacing.
			if i+1 < cols && index[j*cols+i+1] >= 0 && !cut(rowCuts[j], m.pts[p].X, m.pts[index[j*cols+i+1]].X) {
				join(p, index[j*cols+i+1])
			}
			if j+1 < rows && index[(j+1)*cols+i] >= 0 && !cut(colCuts[i], m.pts[p].Y, m.pts[index[(j+1)*cols+i]].Y) {
				join(p, index[(j+1)*cols+i])
			}
		}
	}
	ids := map[int]int{}
	m.region = make([]int, len(m.pts))
	var size []int
	for i := range m.pts {
		r := find(i)
		id, ok := ids[r]
		if !ok {
			id = len(size)
			ids[r] = id
			size = append(size, 0)
		}
		m.region[i] = id
		size[id]++
	}
	root := 0
	for r, s := range size {
		if s > size[root] {
			root = r
		}
	}
	m.nreg = len(size)
	// Which flanges each bend divides.
	m.adj = map[int][][2]int{}
	seen := map[[3]int]bool{}
	// near holds, for a region and a bend, a sample of the region right beside the bend line: which side of the line the region lies on.
	m.near = map[[2]int]int{}
	for _, c := range crossings {
		ra, rb := m.region[c.a], m.region[c.b]
		if ra == rb {
			continue
		}
		if _, ok := m.near[[2]int{ra, c.bend}]; !ok {
			m.near[[2]int{ra, c.bend}] = c.a
		}
		if _, ok := m.near[[2]int{rb, c.bend}]; !ok {
			m.near[[2]int{rb, c.bend}] = c.b
		}
		if seen[[3]int{ra, rb, c.bend}] {
			continue
		}
		seen[[3]int{ra, rb, c.bend}], seen[[3]int{rb, ra, c.bend}] = true, true
		m.adj[ra] = append(m.adj[ra], [2]int{rb, c.bend})
		m.adj[rb] = append(m.adj[rb], [2]int{ra, c.bend})
	}
	m.tree(root)
	return m, nil
}

// tree roots the flanges at region root: each bend joins a flange to its parent, and its child lies on the side of the line toward its samples.
func (m *Model) tree(root int) {
	m.child = make([]int, len(m.bends))
	m.side = make([]float64, len(m.bends))
	m.axes = make([]bendAxes, len(m.bends))
	for bi := range m.bends {
		m.child[bi] = -1
		bd := m.bends[bi]
		l := bd.A.Dist(bd.B)
		a := solid.Vec{X: (bd.B.X - bd.A.X) / l, Y: (bd.B.Y - bd.A.Y) / l, Z: 0}
		m.axes[bi] = bendAxes{o: solid.Vec{X: bd.A.X, Y: bd.A.Y, Z: 0}, a: a, n: solid.Vec{X: -a.Y, Y: a.X, Z: 0}, length: l}
	}
	m.kids = make([][]int, m.nreg)
	m.parentBend = make([]int, m.nreg)
	m.parentRegion = make([]int, m.nreg)
	for i := range m.parentBend {
		m.parentBend[i], m.parentRegion[i] = -1, -1
	}
	visited := map[int]bool{root: true}
	m.order = []int{root}
	for q := 0; q < len(m.order); q++ {
		r := m.order[q]
		for _, e := range m.adj[r] {
			nb, bi := e[0], e[1]
			if visited[nb] || m.child[bi] >= 0 {
				continue
			}
			visited[nb] = true
			m.order = append(m.order, nb)
			m.parentBend[nb], m.parentRegion[nb], m.child[bi] = bi, r, nb
			m.kids[r] = append(m.kids[r], bi)
			p := m.pts[m.near[[2]int{nb, bi}]]
			if ax := m.axes[bi]; (solid.Vec{X: p.X, Y: p.Y, Z: 0}).Sub(ax.o).Dot(ax.n) < 0 {
				m.axes[bi].n = ax.n.Scale(-1)
			}
		}
	}
}

// Folded returns the samples' positions with every bend at its angle and the flange holding the sample nearest held still, for checking the folding against a model.
func (m *Model) Folded(held geometry.Point) [][3]float64 {
	angles := make([]float64, len(m.bends))
	for i, b := range m.bends {
		angles[i] = b.Angle * math.Pi / 180
	}
	pos, frames, _ := m.state(angles, -1)
	best, bd := 0, math.Inf(1)
	for i, q := range m.pts {
		if d := q.Dist(held); d < bd {
			best, bd = i, d
		}
	}
	f := frames[m.region[best]]
	out := make([][3]float64, len(pos))
	for i, q := range pos {
		v := f.invert(q)
		out[i] = [3]float64{v.X, v.Y, v.Z}
	}
	return out
}

// frame is a rigid motion: x ↦ r·x + t.
type frame struct {
	r [3][3]float64
	t solid.Vec
}

var identity = frame{r: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}

func (f frame) apply(v solid.Vec) solid.Vec {
	return solid.Vec{
		X: float64(f.r[0][0]*v.X) + float64(f.r[0][1]*v.Y) + float64(f.r[0][2]*v.Z) + f.t.X,
		Y: float64(f.r[1][0]*v.X) + float64(f.r[1][1]*v.Y) + float64(f.r[1][2]*v.Z) + f.t.Y,
		Z: float64(f.r[2][0]*v.X) + float64(f.r[2][1]*v.Y) + float64(f.r[2][2]*v.Z) + f.t.Z,
	}
}

func (f frame) rotate(v solid.Vec) solid.Vec { return f.apply(v).Sub(f.t) }

// invert takes a point back through the motion.
func (f frame) invert(v solid.Vec) solid.Vec {
	d := v.Sub(f.t)
	return solid.Vec{
		X: float64(f.r[0][0]*d.X) + float64(f.r[1][0]*d.Y) + float64(f.r[2][0]*d.Z),
		Y: float64(f.r[0][1]*d.X) + float64(f.r[1][1]*d.Y) + float64(f.r[2][1]*d.Z),
		Z: float64(f.r[0][2]*d.X) + float64(f.r[1][2]*d.Y) + float64(f.r[2][2]*d.Z),
	}
}

// then is f after g: x ↦ f(g(x)).
func (f frame) then(g frame) frame {
	var out frame
	for i := range 3 {
		for j := range 3 {
			out.r[i][j] = float64(f.r[i][0]*g.r[0][j]) + float64(f.r[i][1]*g.r[1][j]) + float64(f.r[i][2]*g.r[2][j])
		}
	}
	out.t = f.apply(g.t)
	return out
}

// rotation about the unit axis k through point c by angle th.
func rotation(k, c solid.Vec, th float64) frame {
	s, co := math.Sin(th), math.Cos(th)
	v := 1 - co
	r := [3][3]float64{
		{co + float64(k.X*k.X*v), float64(k.X*k.Y*v) - float64(k.Z*s), float64(k.X*k.Z*v) + float64(k.Y*s)},
		{float64(k.Y*k.X*v) + float64(k.Z*s), co + float64(k.Y*k.Y*v), float64(k.Y*k.Z*v) - float64(k.X*s)},
		{float64(k.Z*k.X*v) - float64(k.Y*s), float64(k.Z*k.Y*v) + float64(k.X*s), co + float64(k.Z*k.Z*v)},
	}
	f := frame{r: r}
	f.t = c.Sub(f.rotate(c))
	return f
}

// zAxis is the flat part's normal, toward its viewer.
var zAxis = solid.Vec{Z: 1}

// geometry of bend bi at angle phi (radians), in its parent's flat frame.
type arc struct {
	ax                bendAxes
	sigma, rm, rn, ri float64
	phi, h            float64   // h: half the flat the arc takes up
	start, c          solid.Vec // where the arc starts on the line's normal, its center
}

func (m *Model) arc(bi int, phi float64) arc {
	bd := m.bends[bi]
	sigma := 1.0
	if !bd.Up {
		sigma = -1
	}
	ax := m.axes[bi]
	ri := bd.InsideRadius
	rm, rn := ri+m.t/2, ri+float64(m.k*m.t)
	// The arc takes up rn·phi of flat, half on each side of the line.
	h := rn * phi / 2
	start := ax.o.Sub(ax.n.Scale(h))
	return arc{ax: ax, sigma: sigma, rm: rm, rn: rn, ri: ri, phi: phi, h: h, start: start, c: start.Add(zAxis.Scale(sigma * rm))}
}

// hinge is the rigid motion of bend bi's child beyond its arc.
func (a arc) hinge() frame {
	shift := frame{r: identity.r, t: a.ax.n.Scale(-2 * a.h)}
	k := a.ax.n.Cross(zAxis).Scale(a.sigma)
	return rotation(k, a.c, a.phi).then(shift)
}

// normal is the sheet's normal at the arc point u mm past the bend line, in the parent's flat frame: the flat's normal turned by the arc so far.
func (a arc) normal(u float64) solid.Vec {
	al := (u + a.h) / a.rn
	return zAxis.Scale(math.Cos(al)).Sub(a.ax.n.Scale(a.sigma * math.Sin(al)))
}

// onArc places a flat point u mm past the bend line (toward the child; within the arc, |u| ≤ h).
func (a arc) onArc(p solid.Vec, u float64) solid.Vec {
	s := u + a.h
	al := s / a.rn
	base := p.Sub(a.ax.n.Scale(s))
	return base.Add(a.ax.n.Scale(a.rm * math.Sin(al))).Add(zAxis.Scale(a.sigma * a.rm * (1 - math.Cos(al))))
}

// state places every sample with the bends at the given angles (radians); inArc marks the samples inside bend cur's arc.
func (m *Model) state(angles []float64, cur int) (pos []solid.Vec, frames []frame, inArc []bool) {
	return m.place(angles, cur, nil)
}

// movingSamples lists the samples bend bi's angle places.
func (m *Model) movingSamples(bi int) []int32 {
	if m.moving == nil {
		m.moving = make([][]int32, len(m.bends))
	}
	if m.moving[bi] != nil {
		return m.moving[bi]
	}
	below := map[int]bool{}
	if c := m.child[bi]; c >= 0 {
		below[c] = true
		for _, r := range m.order { // parents come before children
			if up := m.parentRegion[r]; up >= 0 && below[up] {
				below[r] = true
			}
		}
	}
	a := m.arc(bi, m.bends[bi].Angle*math.Pi/180)
	parentReg := -1
	if m.child[bi] >= 0 {
		parentReg = m.parentRegion[m.child[bi]]
	}
	out := []int32{}
	for i, p := range m.pts {
		r := m.region[i]
		v := solid.Vec{X: p.X, Y: p.Y, Z: 0}
		if below[r] || r == parentReg && v.Sub(a.ax.o).Dot(a.ax.n) >= -a.h && m.within(bi, v) {
			out = append(out, int32(i))
		}
	}
	m.moving[bi] = out
	return out
}

// place is state over the samples in only (every sample when nil), keeping the others as the buffers hold them.
func (m *Model) place(angles []float64, cur int, only []int32) (pos []solid.Vec, frames []frame, inArc []bool) {
	if m.posBuf == nil {
		m.posBuf, m.inArcBuf = make([]solid.Vec, len(m.pts)), make([]bool, len(m.pts))
		m.frameBuf, m.arcBuf = make([]frame, m.nreg), make([]arc, len(m.bends))
	}
	frames = m.frameBuf
	// A region the tree does not reach (material the sampling cut off from the rest, such as a web narrower than the spacing) stays flat where it is.
	for r := range frames {
		frames[r] = identity
	}
	for _, r := range m.order {
		if bi := m.parentBend[r]; bi >= 0 {
			frames[r] = frames[m.parentRegion[r]].then(m.arc(bi, angles[bi]).hinge())
		}
	}
	pos, inArc, arcs := m.posBuf, m.inArcBuf, m.arcBuf
	for bi := range m.bends {
		arcs[bi] = m.arc(bi, angles[bi])
	}
	n := len(m.pts)
	if only != nil {
		n = len(only)
	}
	for k := 0; k < n; k++ {
		i := k
		if only != nil {
			i = int(only[k])
		}
		p := m.pts[i]
		r := m.region[i]
		v := solid.Vec{X: p.X, Y: p.Y, Z: 0}
		placed := false
		inArc[i] = false
		// Within the half of the arc on its own side of the line: a bend to its parent, or one to a child.
		if bi := m.parentBend[r]; bi >= 0 {
			a := arcs[bi]
			if u := v.Sub(a.ax.o).Dot(a.ax.n); u <= a.h {
				pos[i], inArc[i], placed = frames[m.parentRegion[r]].apply(a.onArc(v, u)), bi == cur, true
			}
		}
		for _, bi := range m.kids[r] {
			if placed {
				break
			}
			a := arcs[bi]
			if u := v.Sub(a.ax.o).Dot(a.ax.n); u >= -a.h && m.within(bi, v) {
				pos[i], inArc[i], placed = frames[r].apply(a.onArc(v, u)), bi == cur, true
			}
		}
		if !placed {
			pos[i] = frames[r].apply(v)
		}
	}
	return pos, frames, inArc
}

// Simulate forms the bends in the order seq and reports, for each, what forming it runs into.
func (m *Model) Simulate(seq []int, tool Tooling) []Result {
	angles := make([]float64, len(m.bends))
	out := make([]Result, len(seq))
	for si, bi := range seq {
		out[si] = m.form(bi, angles, tool)
		angles[bi] = m.bends[bi].Angle * math.Pi / 180
	}
	return out
}

// form bends bi from flat to its angle, with the other bends at angles.
func (m *Model) form(bi int, angles []float64, tool Tooling) Result {
	if m.child[bi] < 0 {
		return Result{}
	}
	if tool.DieAngle <= 0 {
		tool.DieAngle = max(180-m.bends[bi].Angle, minDieAngle)
	}
	die := tool.die()
	// A gooseneck punch may face either way; the bend collides only when it does both ways.
	ways := []shape{tool.punch(false)}
	if tool.PunchThroatDepth > 0 {
		ways = append(ways, tool.punch(true))
	}
	var first Result
	for w, punch := range ways {
		places := m.formWith(bi, angles, tool, punch, die)
		// The bend collides only if no place clears it. The punch collides when it does at every place, the die likewise; when the places collide with different tools, the middle place says which.
		best := Result{Simulated: true}
		for _, p := range places {
			if !p.Collides() {
				return best
			}
		}
		best.Punch = places[0].Punch && places[1].Punch && places[2].Punch
		best.Die = places[0].Die && places[1].Die && places[2].Die
		if !best.Punch && !best.Die {
			best.Punch, best.Die = places[1].Punch, places[1].Die
		}
		if w == 0 {
			first = best
		}
	}
	return first
}

// formWith forms bend bi from flat to its angle with one punch, reporting what each place along the bend ran into.
func (m *Model) formWith(bi int, angles []float64, tool Tooling, punch, die shape) [3]Result {
	final := m.bends[bi].Angle
	// Place the whole part once with this bend flat; each step then moves only what the bend's angle places.
	angles[bi] = 0
	m.place(angles, bi, nil)
	moving := m.movingSamples(bi)
	var places [3]Result
	for step := 0.0; ; step += stepDeg {
		ang := math.Min(step, final)
		angles[bi] = ang * math.Pi / 180
		m.place(angles, bi, moving)
		m.check(bi, angles, tool, punch, die, places[:])
		if ang >= final || places[0].Collides() && places[1].Collides() && places[2].Collides() {
			break
		}
	}
	angles[bi] = 0
	return places
}

// Plan is the outcome of searching for a bend sequence.
type Plan struct {
	// Order is a sequence forming every bend clear of the tooling, when Found.
	Order []int
	Found bool
	// Exhausted is true when every sequence was tried (none clears, when not Found); false when the search ran out of its budget first.
	Exhausted bool
	// Blocked, when not Found, are the bends that could not be formed at the furthest the search got, and what each ran into; Formed are the bends formed by then.
	Blocked map[int]Result
	Formed  []int
}

// Sequence searches for an order forming every bend clear of the tooling, forming at most budget bends in all; a part with more than maxSequenceBends bends is not searched (the plan is neither Found nor Exhausted). Whether a bend clears depends only on which bends are formed before it, so the search is over those sets. A bend that runs into the tooling with some bends formed is taken to do so with more formed too (forming a bend only lifts more of the part out of the flat, toward the tools), so a set from which some bend collides is a dead end.
func (m *Model) Sequence(tool Tooling, budget int) Plan {
	n := len(m.bends)
	if n > maxSequenceBends {
		return Plan{}
	}
	failed := map[uint64]bool{}
	angles := make([]float64, n)
	var plan Plan
	var path []int
	spent := 0
	var search func(done uint64) bool
	search = func(done uint64) bool {
		if len(path) == n {
			return true
		}
		if failed[done] {
			return false
		}
		var next []int
		blocked := map[int]Result{}
		for bi := range n {
			if done&(1<<bi) != 0 {
				continue
			}
			if spent >= budget {
				return false
			}
			spent++
			if r := m.form(bi, angles, tool); r.Collides() {
				blocked[bi] = r
			} else {
				next = append(next, bi)
			}
		}
		if len(blocked) > 0 {
			if len(path) >= len(plan.Formed) {
				plan.Formed, plan.Blocked = slices.Clone(path), blocked
			}
			failed[done] = true
			return false
		}
		for _, bi := range next {
			angles[bi] = m.bends[bi].Angle * math.Pi / 180
			path = append(path, bi)
			if search(done | 1<<bi) {
				return true
			}
			path = path[:len(path)-1]
			angles[bi] = 0
			if spent >= budget {
				return false
			}
		}
		failed[done] = true
		return false
	}
	if search(0) {
		return Plan{Order: path, Found: true, Exhausted: true}
	}
	plan.Exhausted = spent < budget
	return plan
}

// check tests one state of bend bi against the tooling at each place, recording what each place runs into.
func (m *Model) check(bi int, angles []float64, tool Tooling, punch, die shape, places []Result) {
	pos, frames, inArc := m.posBuf, m.frameBuf, m.inArcBuf
	a := m.arc(bi, angles[bi])
	g := frames[m.parentRegion[m.child[bi]]]
	half := a.phi / 2
	// The punch: on the bisector, measured from the inner arc's midpoint.
	radial := a.ax.n.Scale(math.Sin(half)).Add(zAxis.Scale(-a.sigma * math.Cos(half)))
	apex := g.apply(a.c.Add(radial.Scale(a.ri)))
	d := g.rotate(radial.Scale(-1))
	along := g.rotate(a.ax.a)
	lat := along.Cross(d)
	start := g.apply(a.ax.o).Sub(apex).Dot(along)
	length, pl := a.ax.length, tool.PunchLength
	if pl <= 0 {
		pl = length
	}
	if tool.DieLength <= 0 {
		tool.DieLength = length
	}
	places3 := [3][2]float64{
		{start, start + pl},
		{float64(start+length/2) - pl/2, float64(start+length/2) + pl/2},
		{float64(start+length) - pl, start + length},
	}
	// The die: under the parent flange, centered on the bend's middle.
	dieLo, dieHi := float64(length/2)-tool.DieLength/2, float64(length/2)+tool.DieLength/2
	parentReg, childReg := m.parentRegion[m.child[bi]], m.child[bi]
	// A sample is the sheet's mid-surface: it touches a tool within half the thickness, less a small clearance.
	margin := float64(m.t/2) - float64(toolClearance*m.t)
	for i, q := range pos {
		if inArc[i] {
			continue
		}
		l := g.invert(q).Sub(a.ax.o)
		if c := l.Dot(a.ax.a); c >= dieLo-margin && c <= dieHi+margin &&
			die.near(l.Dot(a.ax.n), float64(l.Dot(zAxis)*a.sigma)+m.t/2, margin) {
			for k := range places {
				places[k].Die = true
			}
		}
		if m.region[i] == childReg || m.region[i] == parentReg {
			continue
		}
		rel := q.Sub(apex)
		if !punch.near(rel.Dot(lat), rel.Dot(d), margin) {
			continue
		}
		c := rel.Dot(along)
		for k, e := range places3 {
			if c >= e[0]-margin && c <= e[1]+margin {
				places[k].Punch = true
			}
		}
	}
}

// inTree reports whether region r is folded with the part: the root or a flange reached from it through a bend. The rest is material the sampling cut off from the part, which stays flat.
func (m *Model) inTree(r int) bool { return r == m.order[0] || m.parentBend[r] >= 0 }

// within reports whether flat point v lies beside bend bi's line, along it.
func (m *Model) within(bi int, v solid.Vec) bool {
	ax := m.axes[bi]
	c := v.Sub(ax.o).Dot(ax.a)
	return c >= 0 && c <= ax.length
}
