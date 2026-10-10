package sheet

import (
	"fmt"
	"math"
	"slices"

	"github.com/open-mrp/dfm/solid"
)

// loopSamples returns points along a loop, in its direction.
func loopSamples(l solid.Loop) []solid.Vec {
	var out []solid.Vec
	for i, e := range l.Edges {
		n := 1
		if e.Curve.Kind != solid.CurveLine {
			n = 16
		}
		for j := range n {
			s := float64(j) / float64(n)
			if l.Reversed[i] {
				s = 1 - s
			}
			out = append(out, e.Point(s))
		}
	}
	return out
}

// samples returns points along every loop of the face, taken once.
func (u *unfolder) samples(f *solid.Face) []solid.Vec {
	if s, ok := u.samp[f]; ok {
		return s
	}
	var out []solid.Vec
	for _, l := range f.Loops {
		out = append(out, loopSamples(l)...)
	}
	u.samp[f] = out
	return out
}

// planeArea is the area of a flat face: the largest area one of its loops encloses (the outer loop; some files do not mark it) less the others. Of the two skins of a plate with countersinks, the side where the holes are their through-cut size has the larger area.
func planeArea(f *solid.Face) float64 {
	fr := f.Surface.Frame
	var areas []float64
	for _, l := range f.Loops {
		pts := loopSamples(l)
		a := 0.0
		for i, p := range pts {
			q := pts[(i+1)%len(pts)]
			px, py := p.Sub(fr.Origin).Dot(fr.X), p.Sub(fr.Origin).Dot(fr.Y)
			qx, qy := q.Sub(fr.Origin).Dot(fr.X), q.Sub(fr.Origin).Dot(fr.Y)
			a += float64(px*qy) - float64(qx*py)
		}
		areas = append(areas, math.Abs(a)/2)
	}
	if len(areas) == 0 {
		return 0
	}
	outer, total := slices.Max(areas), 0.0
	for _, a := range areas {
		total += a
	}
	return outer - (total - outer)
}

func (u *unfolder) largestPlane() *solid.Face {
	var best *solid.Face
	bestA := 0.0
	for _, f := range u.body.Faces {
		if f.Surface.Kind != solid.SurfacePlane {
			continue
		}
		if a := planeArea(f); a > bestA+tol {
			best, bestA = f, a
		}
	}
	return best
}

// thickness is the distance from the base face to the largest flat face facing the other way: the other skin.
func (u *unfolder) thickness(base *solid.Face) (float64, *solid.Face, error) {
	n := base.Normal(base.Surface.Frame.Origin)
	o := base.Surface.Frame.Origin
	t, bestA := 0.0, 0.0
	var other *solid.Face
	for _, f := range u.body.Faces {
		if f == base || f.Surface.Kind != solid.SurfacePlane || f.Normal(f.Surface.Frame.Origin).Dot(n) > -parallelCos {
			continue
		}
		d := o.Sub(f.Surface.Frame.Origin).Dot(n)
		if d <= tol {
			continue
		}
		if a := planeArea(f); a > bestA+tol {
			t, bestA, other = d, a, f
		}
	}
	if t == 0 {
		return 0, nil, fmt.Errorf("%w: no face lies opposite the largest flat face", ErrNotSheetMetal)
	}
	return t, other, nil
}
