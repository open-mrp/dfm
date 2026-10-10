package geometry

import (
	"slices"
	"testing"
)

func TestFindBorder(t *testing.T) {
	rect := func(id string, x, y, w, h float64) Contour {
		return closedPoly(id, [3]float64{x, y, 0}, [3]float64{x + w, y, 0}, [3]float64{x + w, y + h, 0}, [3]float64{x, y + h, 0})
	}
	parts := []Contour{square("a", 20, 20, 30), square("b", 100, 20, 30)}
	for _, tt := range []struct {
		name     string
		contours []Contour
		want     []string
	}{
		{"double frame around two parts", append([]Contour{rect("o", 0, 0, 200, 100), rect("i", 5, 5, 190, 90)}, parts...), []string{"o", "i"}},
		{"band too wide", append([]Contour{rect("o", 0, 0, 200, 100), rect("i", 15, 15, 170, 70)}, parts...), nil},
		{"one part inside: a frame part", []Contour{rect("o", 0, 0, 200, 100), rect("i", 5, 5, 190, 90), square("a", 20, 20, 30)}, nil},
		{"a hole in the band", append([]Contour{rect("o", 0, 0, 200, 100), rect("i", 5, 5, 190, 90), square("h", 1, 1, 2)}, parts...), nil},
		{"inner not a rectangle", append([]Contour{rect("o", 0, 0, 200, 100),
			closedPoly("i", [3]float64{5, 5, 0}, [3]float64{195, 5, 0}, [3]float64{195, 95, 0}, [3]float64{100, 80, 0}, [3]float64{5, 95, 0})}, parts...), nil},
		{"drawn as lines", append([]Contour{
			line(0, 0, 200, 0, "o1"), line(200, 0, 200, 100, "o2"), line(200, 100, 0, 100, "o3"), line(0, 100, 0, 0, "o4"),
			rect("i", 5, 5, 190, 90)}, parts...), []string{"o1", "o2", "o3", "o4", "i"}},
	} {
		p := NewPart(tt.contours, nil)
		ids, _, ok := p.FindBorder()
		slices.Sort(ids)
		want := slices.Clone(tt.want)
		slices.Sort(want)
		if ok != (tt.want != nil) || !slices.Equal(ids, want) {
			t.Errorf("%s: border %v %v, want %v", tt.name, ids, ok, tt.want)
		}
	}
}
