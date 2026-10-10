package sheet

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/solid"
)

func flatten(t *testing.T, path string, k float64) *Result {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("corpus file not present")
	}
	m, err := design.ParseModel(design.Design{Format: constants.DesignFormatSTEP, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	r, err := Flatten(m, Options{KFactor: k})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// corpusFile is the path of a case's drawing or model in the private corpus (docs/private-corpus.md), which the tests skip without.
func corpusFile(id, ext string) string { return "../fixtures/private/" + id + "/" + id + ext }

// TestFlatMatchesDrawing unfolds corpus models whose flat DXF was saved beside them (the drawing is its own case), with the K-factor their CAD used, and compares the flat part's size and holes with the drawing's.
func TestFlatMatchesDrawing(t *testing.T) {
	for _, tt := range []struct {
		step, dxf string
		k         float64
	}{
		{"p0069", "p0068", 0.44},
		{"p0096", "p0095", 0.44},
		{"p0094", "p0093", 0.44},
		// The CAD these four were drawn in unfolds with K = 0.5.
		{"p0199", "p0198", 0.5},
		{"p0191", "p0190", 0.5},
		{"p0189", "p0188", 0.5},
		{"p0195", "p0194", 0.5},
	} {
		t.Run(tt.step, func(t *testing.T) {
			r := flatten(t, corpusFile(tt.step, ".step"), tt.k)
			data, err := os.ReadFile(corpusFile(tt.dxf, ".dxf"))
			if err != nil {
				t.Skip("corpus file not present")
			}
			p, err := design.Parse(design.Design{Format: constants.DesignFormatDXF, Data: data, Units: constants.LengthUnitMillimeter})
			if err != nil {
				t.Fatal(err)
			}
			got := []float64{r.Part.Bounds.WidthMM(), r.Part.Bounds.HeightMM()}
			want := []float64{p.Bounds.WidthMM(), p.Bounds.HeightMM()}
			if math.Abs(min(got[0], got[1])-min(want[0], want[1])) > 0.02 || math.Abs(max(got[0], got[1])-max(want[0], want[1])) > 0.02 {
				t.Errorf("flat %.3f x %.3f, drawing %.3f x %.3f", got[0], got[1], want[0], want[1])
			}
			if len(r.Part.Holes()) != len(p.Holes()) {
				t.Errorf("%d holes, drawing has %d", len(r.Part.Holes()), len(p.Holes()))
			}
		})
	}
}

// TestBendsRead checks the bends read from a corpus model (a 3 mm bracket with two 90° bends) against what its geometry shows.
func TestBendsRead(t *testing.T) {
	r := flatten(t, corpusFile("p0189", ".step"), 0.44)
	if r.ThicknessMM != 3 || len(r.Bends) != 2 {
		t.Fatalf("thickness %v, %d bends", r.ThicknessMM, len(r.Bends))
	}
	for _, b := range r.Bends {
		if math.Abs(b.AngleDeg-90) > 1e-6 || math.Abs(b.InsideRadiusMM-0.2) > 1e-6 {
			t.Errorf("bend %+v, want 90° at 0.2 mm", b)
		}
	}
}

// TestMeshRefused refuses a model made of triangles, as meshes converted to STEP are.
func TestMeshRefused(t *testing.T) {
	b := &solid.Body{}
	for i := range 60 {
		x := float64(i)
		a, c, d := solid.Vec{X: x}, solid.Vec{X: x + 1}, solid.Vec{X: x, Y: 1}
		e := func(p, q solid.Vec) *solid.Edge {
			return &solid.Edge{Start: p, End: q, Curve: solid.Curve{Kind: solid.CurveLine}}
		}
		b.Faces = append(b.Faces, &solid.Face{
			ID:        fmt.Sprint(i),
			Surface:   solid.Surface{Kind: solid.SurfacePlane, Frame: solid.Frame{X: solid.Vec{X: 1}, Y: solid.Vec{Y: 1}, Z: solid.Vec{Z: 1}}},
			SameSense: true,
			Loops:     []solid.Loop{{Edges: []*solid.Edge{e(a, c), e(c, d), e(d, a)}, Reversed: make([]bool, 3), Outer: true}},
		})
	}
	_, err := Flatten(&solid.Model{Bodies: []*solid.Body{b}}, Options{KFactor: 0.44})
	if !errors.Is(err, ErrNotSheetMetal) || !strings.Contains(err.Error(), "mesh") {
		t.Fatalf("err = %v, want a mesh refusal", err)
	}
}

// FuzzFlatten checks that reading and unfolding any STEP file never panics.
func FuzzFlatten(f *testing.F) {
	paths, _ := filepath.Glob("../fixtures/*/*.step")
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := design.ParseModel(design.Design{Format: constants.DesignFormatSTEP, Data: data})
		if err == nil {
			_, _ = Flatten(m, Options{KFactor: 0.44})
		}
	})
}

// TestPlanarEdges tells a thread's helix from the flat curves walls meet on.
func TestPlanarEdges(t *testing.T) {
	spline := func(pts ...solid.Vec) *solid.Edge {
		knots := []float64{0, 0, 0}
		for i := 1; i < len(pts)-2; i++ {
			knots = append(knots, float64(i))
		}
		end := float64(len(pts) - 2)
		knots = append(knots, end, end, end)
		e := &solid.Edge{Start: pts[0], End: pts[len(pts)-1], SameSense: true, Curve: solid.Curve{Kind: solid.CurveBSpline, BSpline: &solid.BSpline{Degree: 2, Points: pts, Knots: knots}}}
		e.Prepare()
		return e
	}
	helix := spline(solid.Vec{X: 3}, solid.Vec{X: 3, Y: 3, Z: 0.25}, solid.Vec{Y: 3, Z: 0.5}, solid.Vec{X: -3, Y: 3, Z: 0.75}, solid.Vec{X: -3, Z: 1})
	flat := spline(solid.Vec{X: 3, Z: 2}, solid.Vec{X: 3, Y: 3, Z: 2}, solid.Vec{Y: 3, Z: 2}, solid.Vec{X: -3, Y: 3, Z: 2}, solid.Vec{X: -3, Z: 2})
	tilted := spline(solid.Vec{X: 3}, solid.Vec{X: 3, Y: 3, Z: 3}, solid.Vec{Y: 3, Z: 3}, solid.Vec{X: -3, Y: 3, Z: 3}, solid.Vec{X: -3})
	if planar(helix) || !planar(flat) {
		t.Errorf("helix planar %v, flat planar %v", planar(helix), planar(flat))
	}
	if !planar(tilted) {
		t.Errorf("a flat curve in a tilted plane is planar")
	}
}
