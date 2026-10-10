package design

import (
	"errors"
	"maps"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

// withBlocks inserts a BLOCKS section holding the given block definitions ahead of the ENTITIES section of a document built by dxf.
func withBlocks(doc string, blocks ...string) string {
	i := strings.Index(doc, "0\nSECTION\n2\nENTITIES\n")
	return doc[:i] + "0\nSECTION\n2\nBLOCKS\n" + strings.Join(blocks, "") + "0\nENDSEC\n" + doc[i:]
}

// blockDef is a block named name with base point (bx, by) holding entities.
func blockDef(name string, bx, by float64, flags int, entities ...string) string {
	return "0\nBLOCK\n2\n" + name + "\n70\n" + itoa(flags) + "\n10\n" + ftoa(bx) + "\n20\n" + ftoa(by) + "\n" +
		strings.Join(entities, "") + "0\nENDBLK\n"
}

func itoa(i int) string     { return strconv.Itoa(i) }
func ftoa(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

func near(a, b geometry.Point) bool { return math.Hypot(a.X-b.X, a.Y-b.Y) < 1e-9 }

func TestBlockPlacement(t *testing.T) {
	// A line and a hole drawn about base point (5, 5), placed at (100, 50), turned 90° and doubled.
	data := withBlocks(dxf("", "0\nINSERT\n5\nI1\n2\nPART\n10\n100\n20\n50\n41\n2\n42\n2\n50\n90\n"),
		blockDef("PART", 5, 5, 0,
			"0\nLINE\n5\nL1\n10\n5\n20\n5\n11\n15\n21\n5\n",
			"0\nCIRCLE\n5\nC1\n10\n10\n20\n10\n40\n1\n"))
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Circles) != 1 || p.Circles[0].EntityIDs[0] != "I1/C1" || !near(p.Circles[0].Center, geometry.Point{X: 90, Y: 60}) || math.Abs(p.Circles[0].Radius-2) > 1e-9 {
		t.Errorf("circle %+v, want I1/C1 at (90, 60) radius 2", p.Circles)
	}
	if len(p.Contours) != 1 {
		t.Fatalf("contours %+v", p.Contours)
	}
	s := p.Contours[0].Segments[0]
	if s.EntityID != "I1/L1" || !near(s.Start, geometry.Point{X: 100, Y: 50}) || !near(s.End, geometry.Point{X: 100, Y: 70}) {
		t.Errorf("line %+v, want I1/L1 from (100, 50) to (100, 70)", s)
	}
}

func TestBlockMirrorReversesArcs(t *testing.T) {
	// A quarter arc from (5, 0) to (0, 5), counter-clockwise; mirrored in X it runs from (-5, 0) to (0, 5), clockwise.
	data := withBlocks(dxf("", "0\nINSERT\n5\nI1\n2\nQ\n10\n0\n20\n0\n41\n-1\n"),
		blockDef("Q", 0, 0, 0, "0\nARC\n5\nA1\n10\n0\n20\n0\n40\n5\n50\n0\n51\n90\n"))
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	s := p.Contours[0].Segments[0]
	if !near(s.Start, geometry.Point{X: -5}) || !near(s.End, geometry.Point{Y: 5}) || s.Bulge >= 0 {
		t.Errorf("mirrored arc %+v, want (-5, 0) to (0, 5) clockwise", s)
	}
	if mid := s.Bounds(); mid.MaxX > 1e-9 {
		t.Errorf("mirrored arc reaches x = %g, want it left of the axis", mid.MaxX)
	}
}

func TestBlockArraysAndNesting(t *testing.T) {
	data := withBlocks(dxf("",
		// 2 columns 20 apart, 2 rows 30 apart.
		"0\nINSERT\n5\nI1\n2\nHOLE\n10\n0\n20\n0\n70\n2\n71\n2\n44\n20\n45\n30\n",
		// A block holding a reference to another, both moved.
		"0\nINSERT\n5\nI2\n2\nPAIR\n10\n100\n20\n0\n"),
		blockDef("HOLE", 0, 0, 0, "0\nCIRCLE\n5\nC1\n10\n0\n20\n0\n40\n1\n"),
		blockDef("PAIR", 0, 0, 0, "0\nINSERT\n5\nN1\n2\nHOLE\n10\n0\n20\n10\n"))
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]geometry.Point{}
	for _, c := range p.Circles {
		got[c.EntityIDs[0]] = c.Center
	}
	want := map[string]geometry.Point{
		"I1[0,0]/C1": {}, "I1[0,1]/C1": {X: 20}, "I1[1,0]/C1": {Y: 30}, "I1[1,1]/C1": {X: 20, Y: 30},
		"I2/N1/C1": {X: 100, Y: 10},
	}
	if len(got) != len(want) {
		t.Fatalf("circles %v, want %v", got, want)
	}
	for id, at := range want {
		if c, ok := got[id]; !ok || !near(c, at) {
			t.Errorf("%s at %v, want %v", id, c, at)
		}
	}
}

func TestBlockLayerInheritance(t *testing.T) {
	line := func(id, extra string, y int) string {
		return "0\nLINE\n5\n" + id + "\n" + extra + "10\n0\n20\n" + itoa(y) + "\n11\n10\n21\n" + itoa(y) + "\n"
	}
	doc := dxf("", circleEnt,
		"0\nINSERT\n5\nB\n8\nBEND\n2\nK\n10\n0\n20\n0\n",      // layer-0 contents become bend lines
		"0\nINSERT\n5\nE\n8\nENGRAVE\n2\nOWN\n10\n0\n20\n0\n", // contents on their own layer stay cut
		"0\nINSERT\n5\nD\n6\nDASHED\n2\nBYB\n10\n0\n20\n0\n",  // BYBLOCK takes the reference's line type
		"0\nINSERT\n5\nH\n8\nOFF\n2\nOWN2\n10\n0\n20\n0\n",    // a hidden reference hides everything
	)
	data := withTables(withBlocks(doc,
		blockDef("K", 0, 0, 0, line("L", "8\n0\n", 1)),
		blockDef("OWN", 0, 0, 0, line("L", "8\nCUT\n", 2)),
		blockDef("BYB", 0, 0, 0, line("L", "6\nBYBLOCK\n", 3)),
		blockDef("OWN2", 0, 0, 0, line("L", "8\nCUT\n", 4))),
		"0\nLAYER\n2\nOFF\n62\n-7\n")
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]geometry.NonCutKind{}
	for _, n := range p.NonCut {
		kinds[n.EntityID] = n.Kind
	}
	want := map[string]geometry.NonCutKind{"B/L": geometry.NonCutBend, "D/L": geometry.NonCutConstruction, "H/L": geometry.NonCutConstruction}
	if len(kinds) != len(want) {
		t.Errorf("non-cut %v, want %v", kinds, want)
	}
	for id, k := range want {
		if kinds[id] != k {
			t.Errorf("%s: kind %v, want %v", id, kinds[id], k)
		}
	}
	if len(p.Contours) != 1 || p.Contours[0].Segments[0].EntityID != "E/L" {
		t.Errorf("cut contours %+v, want only E/L", p.Contours)
	}
}

func TestBlockAttributesAndStretch(t *testing.T) {
	data := withBlocks(dxf("",
		// Attributes follow a reference up to SEQEND.
		"0\nINSERT\n5\nI1\n66\n1\n2\nHOLE\n10\n0\n20\n0\n",
		"0\nATTRIB\n5\nT1\n1\nPART-7\n", "0\nSEQEND\n5\nS1\n",
		// Stretched three times along X: the hole becomes an ellipse.
		"0\nINSERT\n5\nI2\n2\nHOLE\n10\n50\n20\n0\n41\n3\n42\n1\n"),
		blockDef("HOLE", 0, 0, 0, "0\nCIRCLE\n5\nC1\n10\n0\n20\n0\n40\n2\n"))
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Circles) != 1 || p.Circles[0].EntityIDs[0] != "I1/C1" {
		t.Errorf("circles %+v", p.Circles)
	}
	if len(p.Contours) != 1 || !p.Contours[0].Closed {
		t.Fatalf("stretched hole %+v, want one closed contour", p.Contours)
	}
	b := p.Contours[0].Bounds()
	if math.Abs(b.WidthMM()-12) > 1e-3 || math.Abs(b.HeightMM()-4) > 1e-3 {
		t.Errorf("stretched hole is %gx%g, want 12x4", b.WidthMM(), b.HeightMM())
	}
}

func TestBlockErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want error
	}{
		{"undefined block", withBlocks(dxf("", circleEnt, "0\nINSERT\n5\nI1\n2\nNOPE\n10\n0\n20\n0\n")), ErrInvalid},
		{"external reference", withBlocks(dxf("", circleEnt, "0\nINSERT\n5\nI1\n2\nX\n10\n0\n20\n0\n"), blockDef("X", 0, 0, 4)), ErrUnsupportedContent},
		{"circular reference", withBlocks(dxf("", circleEnt, "0\nINSERT\n5\nI1\n2\nA\n10\n0\n20\n0\n"),
			blockDef("A", 0, 0, 0, "0\nINSERT\n5\nN\n2\nA\n10\n1\n20\n0\n")), ErrInvalid},
		{"zero scale", withBlocks(dxf("", circleEnt, "0\nINSERT\n5\nI1\n2\nB\n10\n0\n20\n0\n41\n0\n"), blockDef("B", 0, 0, 0, circleEnt)), ErrInvalid},
		{"unterminated block", strings.Replace(withBlocks(dxf("", circleEnt), blockDef("B", 0, 0, 0)), "0\nENDBLK\n", "", 1), ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseDXF([]byte(tt.data), ""); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
	// Unsupported entities inside a block are named after their reference.
	data := withBlocks(dxf("", circleEnt, "0\nINSERT\n5\nI1\n2\nB\n10\n0\n20\n0\n"), blockDef("B", 0, 0, 0, "0\n3DFACE\n5\nF\n"))
	if _, err := ParseDXF([]byte(data), ""); !errors.Is(err, ErrUnsupportedContent) || !strings.Contains(err.Error(), "I1/F") {
		t.Errorf("err = %v, want unsupported I1/F", err)
	}
}

func TestACIColors(t *testing.T) {
	for i, want := range map[int]string{1: "#ff0000", 5: "#0000ff", 7: aciForeground, 8: "", 10: "#ff0000",
		11: "#ff7f7f", 12: "#cc0000", 13: "#cc6666", 19: "#4c2626", 30: "#ff7f00", 40: "#ffbf00", 21: "#ff9f7f",
		150: "#007fff", 250: ""} {
		if got := aciColor(i); got != want {
			t.Errorf("ACI %d = %q, want %q", i, got, want)
		}
	}
}

func TestDXFColorOperations(t *testing.T) {
	line := func(id, extra string, y int) string {
		return "0\nLINE\n5\n" + id + "\n" + extra + "10\n0\n20\n" + itoa(y) + "\n11\n10\n21\n" + itoa(y) + "\n"
	}
	doc := dxf("", circleEnt,
		line("R", "62\n1\n", 1),                          // red by index
		line("T", "420\n255\n", 2),                       // blue by true color
		line("L", "8\nREDLAYER\n", 3),                    // red through its layer
		line("K", "62\n7\n", 4),                          // the foreground color matches black
		line("G", "62\n3\n", 5),                          // green: no operation, cut
		"0\nINSERT\n5\nI\n62\n1\n2\nBYB\n10\n0\n20\n0\n", // BYBLOCK takes the reference's red
	)
	data := withTables(withBlocks(doc, blockDef("BYB", 0, 0, 0, line("L", "62\n0\n", 6))), "0\nLAYER\n2\nREDLAYER\n62\n1\n")
	ops := Operations{ByColor: map[string]constants.ShapeOperation{
		"#ff0000": constants.ShapeOperationEngrave, "#0000ff": constants.ShapeOperationIgnore, "#000000": constants.ShapeOperationEngrave,
	}}
	p, err := parseDXF([]byte(data), "", ops, nil, geometry.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]geometry.NonCutKind{}
	for _, n := range p.NonCut {
		kinds[n.EntityID] = n.Kind
	}
	want := map[string]geometry.NonCutKind{"R": geometry.NonCutEngrave, "T": geometry.NonCutIgnored, "L": geometry.NonCutEngrave, "K": geometry.NonCutEngrave, "I/L": geometry.NonCutEngrave}
	if !maps.Equal(kinds, want) {
		t.Errorf("non-cut %v, want %v", kinds, want)
	}
	if len(p.Contours) != 1 || p.Contours[0].Segments[0].EntityID != "G" {
		t.Errorf("cut contours %+v, want only G", p.Contours)
	}
	// Without operations, colors change nothing.
	if p, err := ParseDXF([]byte(data), ""); err != nil || len(p.NonCut) != 0 {
		t.Errorf("without operations: non-cut %+v, err %v", p.NonCut, err)
	}
}
