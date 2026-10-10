package geometry

import "testing"

// BenchmarkChainManyLines chains 40,000 loose LINE segments (10,000 squares), typical of an exploded drawing.
func BenchmarkChainManyLines(b *testing.B) {
	var paths []Contour
	for i := range 10000 {
		x, y := float64(i%100)*20, float64(i/100)*20
		paths = append(paths,
			line(x, y, x+10, y, "a"), line(x+10, y+10, x, y+10, "c"),
			line(x+10, y, x+10, y+10, "b"), line(x, y+10, x, y, "d"))
	}
	b.ResetTimer()
	for range b.N {
		p := NewPart(paths, nil)
		if len(p.Contours) != 10000 || !p.Contours[0].Closed {
			b.Fatal("bad chaining")
		}
	}
}
