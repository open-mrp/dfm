package geometry

import (
	"math"
)

// RemovedFraction returns, for the outer loop i, the share of its area that cut-outs remove (islands left inside cut-outs count as material).
func (p *Part) RemovedFraction(i int) float64 {
	if p.Loops[i].AreaMM2 == 0 {
		return 0
	}
	return p.removedAreaMM2(i) / p.Loops[i].AreaMM2
}

// NetAreaMM2 returns, for the outer loop i, the finished part's area: the outline less its cut-outs, with islands left inside cut-outs counted as material.
func (p *Part) NetAreaMM2(i int) float64 {
	return p.Loops[i].AreaMM2 - p.removedAreaMM2(i)
}

// removedAreaMM2 returns the area that cut-outs remove from outer loop i, less the islands left inside them.
func (p *Part) removedAreaMM2(i int) float64 {
	removed := 0.0
	for j, l := range p.Loops {
		if j == i || p.rootOf(j) != i {
			continue
		}
		if l.IsCutout() {
			removed += l.AreaMM2
		} else {
			removed -= l.AreaMM2
		}
	}
	return removed
}

// CutLengthMM is the length the beam travels cutting the part: every path and circle, measured along arcs.
func (p *Part) CutLengthMM() float64 {
	total := 0.0
	for _, c := range p.Contours {
		for _, s := range c.Segments {
			if s.IsArc() {
				total += float64(s.Radius() * math.Abs(s.Sweep()))
			} else {
				total += s.Chord()
			}
		}
	}
	for _, c := range p.Circles {
		total += float64(2 * math.Pi * c.Radius)
	}
	return total
}

// Pierces is how many times the beam pierces the sheet to cut the part: once for each path and circle.
func (p *Part) Pierces() int { return len(p.Contours) + len(p.Circles) }
