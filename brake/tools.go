package brake

import "math"

// poly is a convex polygon in a tool's cross-section: (lateral, depth), with its bounding box and winding.
type poly struct {
	pts    [][2]float64
	lo, hi [2]float64
	area   float64 // signed: the winding decides which side of an edge is inside
}

func newPoly(pts ...[2]float64) poly {
	p := poly{pts: pts, lo: pts[0], hi: pts[0]}
	for i, q := range pts {
		p.lo = [2]float64{min(p.lo[0], q[0]), min(p.lo[1], q[1])}
		p.hi = [2]float64{max(p.hi[0], q[0]), max(p.hi[1], q[1])}
		b := pts[(i+1)%len(pts)]
		p.area += float64(q[0]*b[1]) - float64(b[0]*q[1])
	}
	return p
}

// shape is a union of convex polygons.
type shape []poly

// near reports whether point (y, x) lies inside the shape or within margin of it.
func (s shape) near(y, x, margin float64) bool {
	for i := range s {
		p := &s[i]
		if y < p.lo[0]-margin || y > p.hi[0]+margin || x < p.lo[1]-margin || x > p.hi[1]+margin {
			continue
		}
		if p.dist(y, x) <= margin {
			return true
		}
	}
	return false
}

// dist is the distance from (y, x) to the convex polygon: 0 inside.
func (p *poly) dist(y, x float64) float64 {
	inside := true
	best := math.Inf(1)
	n := len(p.pts)
	area := p.area
	for i := range p.pts {
		a, b := p.pts[i], p.pts[(i+1)%n]
		ey, ex := b[0]-a[0], b[1]-a[1]
		cr := float64(ey*(x-a[1])) - float64(ex*(y-a[0]))
		if cr*area < 0 {
			inside = false
		}
		l2 := float64(ey*ey) + float64(ex*ex)
		t := 0.0
		if l2 > 0 {
			t = math.Max(0, math.Min(1, (float64((y-a[0])*ey)+float64((x-a[1])*ex))/l2))
		}
		best = math.Min(best, math.Hypot(y-(a[0]+float64(t*ey)), x-(a[1]+float64(t*ex))))
	}
	if inside {
		return 0
	}
	return best
}

// punch is the punch's cross-section, measured from its tip (in the inner arc's midpoint): depth x toward the ram, lateral y. A gooseneck's body is set back behind the tip from the nose up, away from the +y side, or from the -y side when mirrored. The nose and the body are joined by a neck that slopes back at 45 degrees from the nose's top until it reaches the body, as a real gooseneck's does: the throat opens above it, and a flange that curls into the throat at that angle clears it, while the material between the nose and the body is there to be hit.
func (tool Tooling) punch(mirrored bool) shape {
	tip := tool.PunchTipAngle * math.Pi / 360
	hw := tool.PunchWidth / 2
	tipDepth := hw / math.Tan(tip)
	nose := tool.PunchHeight
	if tool.PunchThroatDepth > 0 {
		nose = math.Min(tool.PunchHeight, math.Max(tipDepth, tool.PunchNose))
	}
	s := shape{
		newPoly([2]float64{0, 0}, [2]float64{hw, tipDepth}, [2]float64{-hw, tipDepth}),
		newPoly([2]float64{-hw, tipDepth}, [2]float64{hw, tipDepth}, [2]float64{hw, nose}, [2]float64{-hw, nose}),
	}
	if tool.PunchThroatDepth > 0 && nose < tool.PunchHeight {
		// The neck: the nose's section carried back at 45 degrees by the throat's depth, then the body at that offset up to the ram.
		d := tool.PunchThroatDepth
		neckTop := math.Min(tool.PunchHeight, nose+d)
		back := neckTop - nose
		s = append(s, newPoly([2]float64{-hw, nose}, [2]float64{hw, nose}, [2]float64{hw - back, neckTop}, [2]float64{-hw - back, neckTop}))
		if neckTop < tool.PunchHeight {
			s = append(s, newPoly([2]float64{-hw - d, neckTop}, [2]float64{hw - d, neckTop}, [2]float64{hw - d, tool.PunchHeight}, [2]float64{-hw - d, tool.PunchHeight}))
		}
	}
	return mirror(s, mirrored)
}

// ramReach is how far above the punch the ram's column is checked: past any flange.
const ramReach = 10000

// ram is the column above the punch's shank: the holder that clamps the punch and the ram that drives it are at least as wide as the shank and run the length of the machine, so a flange that rises above the punch over its shank runs into them.
func (tool Tooling) ram(mirrored bool) shape {
	hw := tool.PunchWidth / 2
	d := 0.0
	if tool.PunchThroatDepth > 0 {
		tipDepth := hw / math.Tan(tool.PunchTipAngle*math.Pi/360)
		nose := math.Min(tool.PunchHeight, math.Max(tipDepth, tool.PunchNose))
		d = math.Min(tool.PunchThroatDepth, tool.PunchHeight-nose)
	}
	h := tool.PunchHeight
	return mirror(shape{newPoly([2]float64{-hw - d, h}, [2]float64{hw - d, h}, [2]float64{hw - d, h + ramReach}, [2]float64{-hw - d, h + ramReach})}, mirrored)
}

// mirror reflects a cross-section across its depth axis when mirrored, keeping each polygon's winding.
func mirror(s shape, mirrored bool) shape {
	if !mirrored {
		return s
	}
	for i := range s {
		pts := make([][2]float64, len(s[i].pts))
		for j, p := range s[i].pts {
			pts[len(pts)-1-j] = [2]float64{-p[0], p[1]}
		}
		s[i] = newPoly(pts...)
	}
	return s
}

// die is the die's cross-section under a flange lying flat on it: lateral y from the bend line, depth x up from the die's top face.
func (tool Tooling) die() shape {
	hd := tool.DieOpening / 2
	vd := hd / math.Tan(tool.DieAngle*math.Pi/360) // depth of the V
	w, bottom := hd+tool.DieWidth, -tool.DieHeight
	return shape{
		newPoly([2]float64{-w, 0}, [2]float64{-hd, 0}, [2]float64{-hd, bottom}, [2]float64{-w, bottom}),
		newPoly([2]float64{hd, 0}, [2]float64{w, 0}, [2]float64{w, bottom}, [2]float64{hd, bottom}),
		newPoly([2]float64{-hd, 0}, [2]float64{0, -vd}, [2]float64{0, bottom}, [2]float64{-hd, bottom}),
		newPoly([2]float64{0, -vd}, [2]float64{hd, 0}, [2]float64{hd, bottom}, [2]float64{0, bottom}),
	}
}
