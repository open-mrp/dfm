package design

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

func kinds(p *geometry.Part) map[string]geometry.NonCutKind {
	out := map[string]geometry.NonCutKind{}
	for _, n := range p.NonCut {
		out[n.EntityID] = n.Kind
	}
	return out
}

func ids(p *geometry.Part) map[string]bool {
	out := map[string]bool{}
	for _, c := range p.Contours {
		for _, id := range c.EntityIDs() {
			out[id] = true
		}
	}
	return out
}

func TestRepairSVG(t *testing.T) {
	// Sized in pixels: units are assumed. A solid line crosses the plate between two reliefs, and one edge is drawn twice.
	doc := `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" width="100" height="60" viewBox="0 0 100 60">
  <g transform="translate(0,0)">
    <path id="plate" d="M0 0 H100 V28 H96 V32 H100 V60 H0 V32 H4 V28 H0 Z" fill="none" stroke="#000"/>
    <path id="bend" d="M4 30 H96" fill="none" stroke="#000"/>
    <path id="copy" d="M0 0 H100" fill="none" stroke="#000"/>
  </g>
</svg>`
	d := Design{Format: constants.DesignFormatSVG, Data: []byte(doc), Adjustments: []Adjustment{
		{Action: constants.FixActionMarkBendLine, EntityIDs: []string{"bend"}},
		{Action: constants.FixActionRemove, EntityIDs: []string{"copy"}},
		{Action: constants.FixActionSetUnits, Units: constants.LengthUnitMillimeter},
	}}
	out, err := Repair(d)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(Design{Format: constants.DesignFormatSVG, Data: out})
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if p.Units != constants.LengthUnitMillimeter || p.UnitsAssumed || p.Bounds.WidthMM() != 100 {
		t.Errorf("units %q assumed %v width %v\n%s", p.Units, p.UnitsAssumed, p.Bounds.WidthMM(), out)
	}
	if kinds(p)["bend"] != geometry.NonCutBend || ids(p)["copy"] || ids(p)["bend"] {
		t.Errorf("non-cut %v, cut %v\n%s", kinds(p), ids(p), out)
	}
	if !strings.Contains(string(out), `<g id="BEND-1"><path id="bend"`) {
		t.Errorf("bend line not wrapped in place:\n%s", out)
	}
}

func TestRepairDXFWithoutHandlesOrHeader(t *testing.T) {
	// No HEADER, no handles: entities are #1, #2, ...; #2 is a polyline whose vertices go with it.
	dxf := strings.Join([]string{
		"0", "SECTION", "2", "ENTITIES",
		"0", "LINE", "8", "0", "10", "0", "20", "0", "11", "10", "21", "0",
		"0", "POLYLINE", "8", "0", "66", "1", "70", "0",
		"0", "VERTEX", "8", "0", "10", "0", "20", "5",
		"0", "VERTEX", "8", "0", "10", "10", "20", "5",
		"0", "SEQEND", "8", "0",
		"0", "LINE", "8", "0", "10", "10", "20", "0", "11", "10", "21", "10",
		"0", "LINE", "8", "0", "10", "10", "20", "10", "11", "0", "21", "10",
		"0", "LINE", "8", "0", "10", "0", "20", "10", "11", "0", "21", "0",
		"0", "ENDSEC", "0", "EOF", "",
	}, "\n")
	d := Design{Format: constants.DesignFormatDXF, Data: []byte(dxf), Units: constants.LengthUnitInch, Adjustments: []Adjustment{
		{Action: constants.FixActionRemove, EntityIDs: []string{"#2"}},
	}}
	out, err := Repair(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "POLYLINE") || strings.Contains(string(out), "VERTEX") || strings.Contains(string(out), "SEQEND") {
		t.Errorf("polyline left behind:\n%s", out)
	}
	// The units given with the request are declared in a new HEADER.
	p, err := Parse(Design{Format: constants.DesignFormatDXF, Data: out})
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if p.Units != constants.LengthUnitInch || p.Bounds.WidthMM() != 254 {
		t.Errorf("units %q width %v\n%s", p.Units, p.Bounds.WidthMM(), out)
	}
}

func TestRepairRefusesWhatItCannotEdit(t *testing.T) {
	dxf := "0\nSECTION\n2\nENTITIES\n0\nLINE\n8\n0\n10\n0\n20\n0\n11\n10\n21\n0\n0\nENDSEC\n0\nEOF\n"
	_, err := Repair(Design{Format: constants.DesignFormatDXF, Data: []byte(dxf), Units: constants.LengthUnitMillimeter, Adjustments: []Adjustment{
		{Action: constants.FixActionRemove, EntityIDs: []string{"#9"}},
	}})
	var adj *AdjustmentError
	if !errors.As(err, &adj) || adj.EntityID != "#9" {
		t.Errorf("unknown entity: %v", err)
	}
}

// TestRepairInsideBlock removes an entity from a block placed once, in the block's definition, and refuses one placed twice.
func TestRepairInsideBlock(t *testing.T) {
	file := func(inserts string) string {
		return "0\nSECTION\n2\nBLOCKS\n0\nBLOCK\n5\nB0\n2\nPLATE\n10\n0\n20\n0\n" +
			"0\nLWPOLYLINE\n5\nB1\n8\n0\n90\n4\n70\n1\n10\n0\n20\n0\n10\n100\n20\n0\n10\n100\n20\n50\n10\n0\n20\n50\n" +
			"0\nLINE\n5\nB2\n8\n0\n10\n0\n20\n0\n11\n100\n21\n0\n0\nENDBLK\n5\nB3\n0\nENDSEC\n" +
			"0\nSECTION\n2\nENTITIES\n" + inserts + "0\nENDSEC\n0\nEOF\n"
	}
	insert := func(h string, x int) string {
		return fmt.Sprintf("0\nINSERT\n5\n%s\n8\n0\n2\nPLATE\n10\n%d\n20\n0\n", h, x)
	}
	d := Design{Format: constants.DesignFormatDXF, Data: []byte(file(insert("A1", 0))), Units: constants.LengthUnitMillimeter,
		Adjustments: []Adjustment{{Action: constants.FixActionRemove, EntityIDs: []string{"A1/B2"}}}}
	out, err := Repair(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "\nB2\n") {
		t.Errorf("the line is still in the block:\n%s", out)
	}
	p, err := Parse(Design{Format: constants.DesignFormatDXF, Data: out, Units: constants.LengthUnitMillimeter})
	if err != nil || len(p.Duplicates) != 0 {
		t.Errorf("corrected file: %v, %d duplicates", err, len(p.Duplicates))
	}
	d.Data = []byte(file(insert("A1", 0) + insert("A2", 200)))
	if _, err := Repair(d); !errors.Is(err, ErrUnsupportedContent) || !strings.Contains(err.Error(), "placed 2 times") {
		t.Errorf("block placed twice: %v", err)
	}
}

// repairableDXF is a plate with reliefs on both sides and a solid line between them, with handles, a HEADER and a LAYER table, so a repair has to add a layer and a handle.
const repairableDXF = "0\nSECTION\n2\nHEADER\n9\n$INSUNITS\n70\n4\n9\n$HANDSEED\n5\nC0\n0\nENDSEC\n" +
	"0\nSECTION\n2\nTABLES\n0\nTABLE\n2\nLAYER\n5\nA1\n70\n1\n0\nLAYER\n5\nA2\n2\n0\n70\n0\n62\n7\n6\nCONTINUOUS\n0\nENDTAB\n0\nENDSEC\n" +
	"0\nSECTION\n2\nENTITIES\n" +
	"0\nLWPOLYLINE\n5\nB1\n8\n0\n90\n12\n70\n1\n10\n0\n20\n0\n10\n100\n20\n0\n10\n100\n20\n28\n10\n96\n20\n28\n10\n96\n20\n32\n10\n100\n20\n32\n10\n100\n20\n60\n10\n0\n20\n60\n10\n0\n20\n32\n10\n4\n20\n32\n10\n4\n20\n28\n10\n0\n20\n28\n" +
	"0\nLINE\n5\nB2\n8\n0\n10\n4\n20\n30\n11\n96\n21\n30\n" +
	"0\nENDSEC\n0\nEOF\n"

// TestRepairAddsBendLayer marks a line as a bend line in a file with a LAYER table and handles: the line moves to a new BEND layer whose record gets a handle above every other, and $HANDSEED moves past it.
func TestRepairAddsBendLayer(t *testing.T) {
	d := Design{Format: constants.DesignFormatDXF, Data: []byte(repairableDXF), Adjustments: []Adjustment{
		{Action: constants.FixActionMarkBendLine, EntityIDs: []string{"B2"}},
	}}
	out, err := Repair(d)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"\n  0\nLAYER\n  5\nC0\n", "\n  2\nBEND\n", "$HANDSEED\n5\nC1\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	p, err := Parse(Design{Format: constants.DesignFormatDXF, Data: out})
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if kinds(p)["B2"] != geometry.NonCutBend || ids(p)["B2"] {
		t.Errorf("non-cut %v, cut %v\n%s", kinds(p), ids(p), out)
	}
}

// TestRepairReplacesUnits sets the units of a file that declares them: the declaration is replaced, not added.
func TestRepairReplacesUnits(t *testing.T) {
	out, err := Repair(Design{Format: constants.DesignFormatDXF, Data: []byte(repairableDXF), Adjustments: []Adjustment{
		{Action: constants.FixActionSetUnits, Units: constants.LengthUnitInch},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if s := string(out); strings.Count(s, "$INSUNITS") != 1 || !strings.Contains(s, "$INSUNITS\n70\n     1\n") {
		t.Errorf("units not replaced:\n%s", out)
	}
	p, err := Parse(Design{Format: constants.DesignFormatDXF, Data: out})
	if err != nil {
		t.Fatal(err)
	}
	if p.Units != constants.LengthUnitInch || p.Bounds.WidthMM() != 2540 {
		t.Errorf("units %q width %v", p.Units, p.Bounds.WidthMM())
	}
}

// TestRepairSVGWithoutViewBox sets the units of an SVG sized in plain numbers and without a viewBox: one is added from the size, and the size gets the unit.
func TestRepairSVGWithoutViewBox(t *testing.T) {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="60"><rect x="0" y="0" width="100" height="60" fill="none" stroke="#000"/></svg>`
	out, err := Repair(Design{Format: constants.DesignFormatSVG, Data: []byte(doc), Adjustments: []Adjustment{
		{Action: constants.FixActionSetUnits, Units: constants.LengthUnitInch},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if s := string(out); !strings.Contains(s, `width="100in" height="60in" viewBox="0 0 100 60"`) {
		t.Errorf("root not rewritten:\n%s", out)
	}
	p, err := Parse(Design{Format: constants.DesignFormatSVG, Data: out})
	if err != nil {
		t.Fatal(err)
	}
	if p.Units != constants.LengthUnitInch || p.UnitsAssumed || p.Bounds.WidthMM() != 2540 {
		t.Errorf("units %q assumed %v width %v", p.Units, p.UnitsAssumed, p.Bounds.WidthMM())
	}
}

// TestRepairBinaryDXF corrects a binary DXF and writes it back as the ASCII DXF it reads as, with the same geometry.
func TestRepairBinaryDXF(t *testing.T) {
	data, err := os.ReadFile("../fixtures/valid/binary_blocks.dxf")
	if err != nil {
		t.Fatal(err)
	}
	before, err := Parse(Design{Format: constants.DesignFormatDXF, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	out, err := Repair(Design{Format: constants.DesignFormatDXF, Data: data, Adjustments: []Adjustment{
		{Action: constants.FixActionSetUnits, Units: constants.LengthUnitMillimeter},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(out), binarySentinel) || !strings.HasPrefix(string(out), "  0\nSECTION\n") {
		t.Errorf("not written as ASCII DXF:\n%.80s", out)
	}
	after, err := Parse(Design{Format: constants.DesignFormatDXF, Data: out})
	if err != nil {
		t.Fatalf("%v\n%.200s", err, out)
	}
	if after.Bounds != before.Bounds || len(after.Contours) != len(before.Contours) || len(after.Circles) != len(before.Circles) {
		t.Errorf("geometry changed: %+v %d %d vs %+v %d %d", after.Bounds, len(after.Contours), len(after.Circles), before.Bounds, len(before.Contours), len(before.Circles))
	}
}
