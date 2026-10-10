package design

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

func parseFixture(t *testing.T, rel string) *geometry.Part {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "fixtures", rel))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(Design{Format: constants.DesignFormatDXF, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// TestFixtureGeometry asserts what the parser extracts from the corpus, independent of any capability.
func TestFixtureGeometry(t *testing.T) {
	tests := []struct {
		fixture        string
		width, height  float64
		contours, open int
		holeDiameters  []float64
		slotWidths     []float64
	}{
		{"valid/valid_rectangle.dxf", 100, 50, 1, 0, nil, nil},
		{"valid/valid_holes.dxf", 200, 100, 1, 0, []float64{1.5, 5, 10, 6}, nil},
		{"valid/valid_slots.dxf", 200, 100, 4, 0, nil, []float64{1, 6, 2}},
		{"valid/lines_and_arcs.dxf", 120, 80, 1, 0, []float64{8}, nil},
		{"valid/mirrored_arcs.dxf", 120, 80, 1, 0, nil, nil},
		{"valid/disc.dxf", 100, 100, 0, 0, []float64{10}, nil},
		{"valid/inches.dxf", 101.6, 50.8, 1, 0, []float64{1.5875}, nil},
		{"valid/with_annotations.dxf", 100, 50, 1, 0, nil, nil},
		{"valid/polyline_r12_no_handles.dxf", 50, 50, 1, 0, []float64{3}, nil},
		{"invalid/open_contour.dxf", 100, 50, 1, 1, nil, nil},
		{"invalid/open_gap.dxf", 100, 50, 1, 1, nil, nil},
		{"invalid/narrow_slot.dxf", 100, 50, 2, 0, nil, []float64{0.8}},
		{"invalid/narrow_rect_slot.dxf", 100, 50, 2, 0, nil, []float64{0.5}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			p := parseFixture(t, tt.fixture)
			if !approx(p.Bounds.WidthMM(), tt.width) || !approx(p.Bounds.HeightMM(), tt.height) {
				t.Errorf("bounds %gx%g, want %gx%g", p.Bounds.WidthMM(), p.Bounds.HeightMM(), tt.width, tt.height)
			}
			open := 0
			for _, c := range p.Contours {
				if !c.Closed {
					open++
				}
			}
			if len(p.Contours) != tt.contours || open != tt.open {
				t.Errorf("contours %d (open %d), want %d (open %d)", len(p.Contours), open, tt.contours, tt.open)
			}
			var holes []float64
			for _, h := range p.Holes() {
				holes = append(holes, h.Diameter())
			}
			if !approxSlice(holes, tt.holeDiameters) {
				t.Errorf("holes %v, want %v", holes, tt.holeDiameters)
			}
			var slots []float64
			for _, s := range p.Slots {
				slots = append(slots, s.WidthMM)
			}
			if !approxSlice(slots, tt.slotWidths) {
				t.Errorf("slots %v, want %v", slots, tt.slotWidths)
			}
		})
	}
}

func approxSlice(a, b []float64) bool {
	return slices.EqualFunc(a, b, approx)
}

func TestEntityIDs(t *testing.T) {
	// Handles are used when present.
	p := parseFixture(t, "valid/valid_holes.dxf")
	arcHole := p.Holes()[3]
	if len(arcHole.EntityIDs) != 2 {
		t.Errorf("arc-built hole ids = %v, want two handles", arcHole.EntityIDs)
	}
	// Ordinals are synthesized when the file has no handles.
	p = parseFixture(t, "valid/polyline_r12_no_handles.dxf")
	if got := p.Contours[0].EntityIDs(); !slices.Equal(got, []string{"#1"}) {
		t.Errorf("polyline ids = %v", got)
	}
	if got := p.Holes()[0].EntityIDs; !slices.Equal(got, []string{"#7"}) {
		t.Errorf("circle ids = %v", got)
	}
}

// mmHeader declares millimeter units.
const mmHeader = "9\n$INSUNITS\n70\n4\n"

// dxf builds a minimal DXF from entity bodies. An empty header declares millimeters; noHeader omits the HEADER section entirely.
func dxf(header string, entities ...string) string {
	var b strings.Builder
	if header == "" {
		header = mmHeader
	}
	if header != noHeader {
		b.WriteString("0\nSECTION\n2\nHEADER\n" + header + "0\nENDSEC\n")
	}
	b.WriteString("0\nSECTION\n2\nENTITIES\n")
	for _, e := range entities {
		b.WriteString(e)
	}
	b.WriteString("0\nENDSEC\n0\nEOF\n")
	return b.String()
}

const noHeader = "-"

const circleEnt = "0\nCIRCLE\n5\nA1\n10\n0\n20\n0\n40\n5\n"

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want error
		msg  string
	}{
		{"empty", "", ErrInvalid, "no ENTITIES section"},
		{"empty binary", "AutoCAD Binary DXF\r\n\x1a\x00", ErrInvalid, "binary"},
		{"broken binary header", "AutoCAD Binary DXF\r\n", ErrInvalid, "binary"},
		{"bad group code", "0\nSECTION\n2\nENTITIES\nX\nfoo\n", ErrInvalid, "line 5"},
		{"odd lines", "0\nSECTION\n2\n", ErrInvalid, "odd number"},
		{"no ENDSEC", "0\nSECTION\n2\nENTITIES\n" + circleEnt, ErrInvalid, "ENDSEC"},
		{"bad number", dxf("", "0\nCIRCLE\n10\nabc\n20\n0\n40\n1\n"), ErrInvalid, `"abc"`},
		{"NaN", dxf("", "0\nCIRCLE\n10\nNaN\n20\n0\n40\n1\n"), ErrInvalid, "NaN"},
		{"missing radius", dxf("", "0\nCIRCLE\n10\n0\n20\n0\n"), ErrInvalid, "group code 40"},
		{"negative radius", dxf("", "0\nCIRCLE\n10\n0\n20\n0\n40\n-1\n"), ErrInvalid, "negative radius"},
		{"only a zero-radius circle", dxf("", "0\nCIRCLE\n10\n0\n20\n0\n40\n0\n"), ErrInvalid, "no cut geometry"},
		{"only text", dxf("", "0\nTEXT\n1\nhello\n"), ErrInvalid, "no cut geometry"},
		{"spline without points", dxf("", circleEnt, "0\nSPLINE\n5\nB2\n"), ErrInvalid, "SPLINE B2"},
		{"spline fit points only", dxf("", "0\nSPLINE\n5\nB4\n71\n3\n74\n2\n11\n0\n21\n0\n11\n1\n21\n1\n"), ErrUnsupportedContent, "fit points"},
		{"spline bad knots", dxf("", "0\nSPLINE\n71\n1\n73\n2\n40\n0\n40\n1\n10\n0\n20\n0\n10\n1\n20\n0\n"), ErrInvalid, "knot count"},
		{"spline not planar", dxf("", "0\nSPLINE\n71\n1\n73\n2\n40\n0\n40\n0\n40\n1\n40\n1\n10\n0\n20\n0\n30\n0\n10\n1\n20\n0\n30\n5\n"), ErrUnsupportedContent, "XY plane"},
		{"insert of an undefined block", dxf("", "0\nINSERT\n5\nC3\n2\nBLK\n"), ErrInvalid, "INSERT C3"},
		{"ellipse without axes", dxf("", "0\nELLIPSE\n"), ErrInvalid, "ELLIPSE #1"},
		{"ellipse bad ratio", dxf("", "0\nELLIPSE\n10\n0\n20\n0\n11\n5\n21\n0\n40\n0\n"), ErrInvalid, "ratio"},
		{"polyface mesh", dxf("", "0\nPOLYLINE\n70\n64\n0\nSEQEND\n"), ErrUnsupportedContent, "mesh"},
		{"polyline without SEQEND", dxf("", "0\nPOLYLINE\n70\n1\n0\nVERTEX\n10\n0\n20\n0\n"), ErrInvalid, "SEQEND"},
		{"stray vertex", dxf("", "0\nVERTEX\n10\n0\n20\n0\n"), ErrInvalid, "outside POLYLINE"},
		{"tilted extrusion", dxf("", "0\nCIRCLE\n10\n0\n20\n0\n40\n1\n210\n1\n220\n0\n230\n0\n"), ErrUnsupportedContent, "XY plane"},
		{"unsupported units", dxf("9\n$INSUNITS\n70\n3\n", circleEnt), ErrUnsupportedContent, "$INSUNITS"},
		{"no header", dxf(noHeader, circleEnt), ErrUnitsUnknown, "supply the units"},
		{"unitless", dxf("9\n$INSUNITS\n70\n0\n", circleEnt), ErrUnitsUnknown, "$INSUNITS missing or 0"},
		{"lwpolyline count mismatch", dxf("", "0\nLWPOLYLINE\n90\n3\n70\n1\n10\n0\n20\n0\n10\n1\n20\n0\n"), ErrInvalid, "declares 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDXF([]byte(tt.data), "")
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("err %q does not mention %q", err, tt.msg)
			}
		})
	}
}

func TestParseVariants(t *testing.T) {
	square := "0\nLWPOLYLINE\n5\nP\n90\n4\n70\n1\n10\n0\n20\n0\n10\n10\n20\n0\n10\n10\n20\n10\n10\n0\n20\n10\n"
	tests := []struct {
		name  string
		data  string
		check func(*testing.T, *geometry.Part)
	}{
		{"CRLF line endings and BOM", "\xef\xbb\xbf" + strings.ReplaceAll(dxf("", square), "\n", "\r\n"), func(t *testing.T, p *geometry.Part) {
			if len(p.Contours) != 1 || !p.Contours[0].Closed {
				t.Errorf("contours = %+v", p.Contours)
			}
		}},
		{"padded group codes", strings.ReplaceAll(dxf("", circleEnt), "\n0\n", "\n  0\n"), func(t *testing.T, p *geometry.Part) {
			if len(p.Circles) != 1 {
				t.Errorf("circles = %d", len(p.Circles))
			}
		}},
		{"closed flag with plinegen bit", dxf("", strings.Replace(square, "70\n1\n", "70\n129\n", 1)), func(t *testing.T, p *geometry.Part) {
			if !p.Contours[0].Closed {
				t.Error("flag 129 should be closed")
			}
		}},
		{"implicitly closed polyline (last == first)", dxf("", "0\nLWPOLYLINE\n90\n5\n70\n0\n10\n0\n20\n0\n10\n10\n20\n0\n10\n10\n20\n10\n10\n0\n20\n10\n10\n0\n20\n0\n"), func(t *testing.T, p *geometry.Part) {
			if !p.Contours[0].Closed || len(p.Contours[0].Segments) != 4 {
				t.Errorf("contour = %+v", p.Contours[0])
			}
		}},
		{"centimeters", dxf("9\n$INSUNITS\n70\n5\n", circleEnt), func(t *testing.T, p *geometry.Part) {
			if !approx(p.Circles[0].Diameter(), 100) {
				t.Errorf("diameter = %g", p.Circles[0].Diameter())
			}
		}},
		{"paper space ignored", dxf("", circleEnt, "0\nCIRCLE\n67\n1\n10\n0\n20\n0\n40\n500\n"), func(t *testing.T, p *geometry.Part) {
			if len(p.Circles) != 1 {
				t.Errorf("circles = %d", len(p.Circles))
			}
		}},
		{"mirrored arc", dxf("", "0\nARC\n10\n-10\n20\n0\n40\n5\n50\n90\n51\n270\n230\n-1\n"), func(t *testing.T, p *geometry.Part) {
			// OCS center (-10,0) mirrors to world (10,0); the OCS left half (90°..270°) becomes the world right half, so maxX = 15.
			if !approx(p.Bounds.MaxX, 15) || !approx(p.Bounds.MinX, 10) {
				t.Errorf("bounds = %+v", p.Bounds)
			}
		}},
		{"arc with equal angles is a full circle", dxf("", "0\nARC\n10\n0\n20\n0\n40\n2\n50\n30\n51\n30\n"), func(t *testing.T, p *geometry.Part) {
			if len(p.Circles) != 1 {
				t.Errorf("circles = %d", len(p.Circles))
			}
		}},
		{"zero-length line dropped", dxf("", circleEnt, "0\nLINE\n10\n1\n20\n1\n11\n1\n21\n1\n"), func(t *testing.T, p *geometry.Part) {
			if len(p.Contours) != 0 {
				t.Errorf("contours = %d", len(p.Contours))
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseDXF([]byte(tt.data), "")
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, p)
		})
	}
}

func TestUnitsOverride(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		units    constants.LengthUnit
		diameter float64
		err      error
	}{
		{"unitless with mm override", noHeader, constants.LengthUnitMillimeter, 10, nil},
		{"unitless with inch override", "9\n$INSUNITS\n70\n0\n", constants.LengthUnitInch, 254, nil},
		{"override wins over declared mm", mmHeader, constants.LengthUnitCentimeter, 100, nil},
		{"declared inches", "9\n$INSUNITS\n70\n1\n", "", 254, nil},
		{"bad override", mmHeader, "furlong", 0, ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Parse(Design{Format: constants.DesignFormatDXF, Data: []byte(dxf(tt.header, circleEnt)), Units: tt.units})
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("err = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !approx(p.Circles[0].Diameter(), tt.diameter) {
				t.Errorf("diameter %g, want %g", p.Circles[0].Diameter(), tt.diameter)
			}
		})
	}
}

func TestZeroLengthEntitiesAreRecorded(t *testing.T) {
	data := dxf("", circleEnt,
		"0\nLINE\n5\nZ1\n10\n3\n20\n3\n11\n3\n21\n3\n",
		// Repeated middle vertex: one zero-length segment, reported once.
		"0\nLWPOLYLINE\n5\nP1\n90\n4\n70\n0\n10\n0\n20\n0\n10\n1\n20\n0\n10\n1\n20\n0\n10\n2\n20\n0\n",
		// Closed flag plus a final vertex repeating the first is the exporter's closure convention, not a zero-length segment.
		"0\nLWPOLYLINE\n5\nP2\n90\n5\n70\n1\n10\n20\n20\n20\n10\n30\n20\n20\n10\n30\n20\n30\n10\n20\n20\n30\n10\n20\n20\n20\n",
	)
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.ZeroLength, []string{"Z1", "P1"}) {
		t.Errorf("zero-length = %v, want [Z1 P1]", p.ZeroLength)
	}
}

func TestTextEntitiesAreRecorded(t *testing.T) {
	data := dxf("", circleEnt,
		"0\nTEXT\n5\nT1\n10\n2\n20\n3\n40\n1\n1\nA\n",
		// TEXT is placed in its OCS; a -Z extrusion mirrors X.
		"0\nTEXT\n5\nT2\n10\n2\n20\n3\n40\n1\n1\nB\n230\n-1\n",
		// MTEXT is placed in world coordinates whatever its extrusion.
		"0\nMTEXT\n5\nM1\n10\n4\n20\n5\n40\n1\n1\nC\n230\n-1\n",
		// Text that cannot be placed is still recorded and never fails the file.
		"0\nTEXT\n5\nT3\n40\n1\n1\nD\n",
		"0\nTEXT\n5\nT4\n10\n0\n20\n0\n1\nE\n210\n1\n230\n0\n",
	)
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id   string
		x, y float64
		ok   bool
	}{{"T1", 2, 3, true}, {"T2", -2, 3, true}, {"M1", 4, 5, true}, {"T3", 0, 0, false}, {"T4", 0, 0, false}}
	if len(p.Text) != len(want) {
		t.Fatalf("text = %+v, want %d entries", p.Text, len(want))
	}
	for i, w := range want {
		got := p.Text[i]
		if got.EntityID != w.id || (got.Position != nil) != w.ok ||
			(w.ok && (got.Position.X != w.x || got.Position.Y != w.y)) {
			t.Errorf("text %d = {%s %v}, want %+v", i, got.EntityID, got.Position, w)
		}
	}
}

// withTables inserts a TABLES section with the given records ahead of the ENTITIES section of a document built by dxf.
func withTables(doc string, records ...string) string {
	t := "0\nSECTION\n2\nTABLES\n0\nTABLE\n2\nLTYPE\n" +
		"0\nLTYPE\n2\nCONTINUOUS\n73\n0\n" +
		"0\nLTYPE\n2\nDASHED\n73\n2\n49\n0.5\n49\n-0.25\n" +
		"0\nLTYPE\n2\nSOLIDISH\n73\n1\n49\n1\n" +
		"0\nENDTAB\n0\nTABLE\n2\nLAYER\n" + strings.Join(records, "") + "0\nENDTAB\n0\nENDSEC\n"
	i := strings.Index(doc, "0\nSECTION\n2\nENTITIES\n")
	return doc[:i] + t + doc[i:]
}

func TestNonCutGeometryIsLeftOut(t *testing.T) {
	y := 0
	line := func(id, extra string) string {
		y++
		return fmt.Sprintf("0\nLINE\n5\n%s\n%s10\n1\n20\n%d\n11\n3\n21\n%[3]d\n", id, extra, y)
	}
	data := withTables(dxf("", circleEnt,
		line("C1", ""),                       // layer 0, continuous
		line("C2", "6\nBYBLOCK\n"),           // continuous outside a block
		line("C3", "8\nCUTS\n6\nSOLIDISH\n"), // a pattern without gaps is solid
		line("C4", "8\nUNLISTED\n"),          // a layer missing from the table
		line("N1", "6\ndashed\n"),            // names are case-insensitive
		line("N2", "8\nBEND\n"),              // BYLAYER, layer is dashed
		line("N3", "8\nBEND\n6\nBYLAYER\n"),
		line("N4", "8\nOFF\n"),
		line("N5", "8\nFROZEN\n"),
		line("N6", "8\nNOPLOT\n"),
		line("N7", "8\nDefpoints\n"),
		line("N8", "60\n1\n"), // invisible
		"0\nCIRCLE\n5\nN9\n8\nBEND\n10\n7\n20\n8\n40\n1\n",
		"0\nPOLYLINE\n5\nN10\n8\nOFF\n66\n1\n70\n0\n0\nVERTEX\n10\n4\n20\n4\n0\nVERTEX\n10\n5\n20\n4\n0\nSEQEND\n",
		"0\nLINE\n5\nN11\n6\nDASHED\n10\n1\n20\n1\n11\n1\n21\n1\n", // zero length
	),
		"0\nLAYER\n2\n0\n62\n7\n6\nCONTINUOUS\n",
		"0\nLAYER\n2\nCUTS\n62\n7\n",
		"0\nLAYER\n2\nBEND\n62\n1\n6\nDASHED\n",
		"0\nLAYER\n2\nOFF\n62\n-7\n",
		"0\nLAYER\n2\nFROZEN\n62\n7\n70\n1\n",
		"0\nLAYER\n2\nNOPLOT\n62\n7\n290\n0\n",
		"0\nLAYER\n2\nDEFPOINTS\n62\n7\n",
	)
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	var cut []string
	for _, c := range p.Contours {
		cut = append(cut, c.Segments[0].EntityID)
	}
	if !slices.Equal(cut, []string{"C1", "C2", "C3", "C4"}) || len(p.Circles) != 1 {
		t.Errorf("cut contours %v, %d circles; want [C1 C2 C3 C4] and A1", cut, len(p.Circles))
	}
	want := []string{"N1", "N2", "N3", "N4", "N5", "N6", "N7", "N8", "N9", "N10", "N11"}
	if len(p.NonCut) != len(want) {
		t.Fatalf("non-cut = %+v, want %v", p.NonCut, want)
	}
	for i, n := range p.NonCut {
		if n.EntityID != want[i] {
			t.Errorf("non-cut %d = %s, want %s", i, n.EntityID, want[i])
		}
	}
	if pos := p.NonCut[0].Position; pos == nil || pos.X != 1 || pos.Y != 5 {
		t.Errorf("line position %v, want its start (1,5)", pos)
	}
	if pos := p.NonCut[8].Position; pos == nil || pos.X != 7 || pos.Y != 8 {
		t.Errorf("circle position %v, want its center (7,8)", pos)
	}
	if pos := p.NonCut[10].Position; pos != nil {
		t.Errorf("zero-length position %v, want nil", pos)
	}
	if len(p.ZeroLength) != 0 {
		t.Errorf("zero-length %v: non-cut geometry must not be reported twice", p.ZeroLength)
	}
}

func TestLayerKind(t *testing.T) {
	tests := []struct {
		layer string
		kind  geometry.NonCutKind
		ok    bool
	}{
		{"BEND", geometry.NonCutBend, true},
		{"BEND_EXTENT", geometry.NonCutAnnotation, true},
		{"Bend zone", geometry.NonCutAnnotation, true},
		{"Bend lines", geometry.NonCutBend, true},
		{"bendlines-up", geometry.NonCutBend, true},
		{"FOLD", geometry.NonCutBend, true},
		{"line engrave", geometry.NonCutEngrave, true},
		{"Raster", geometry.NonCutEngrave, true},
		{"ENGRAVING", geometry.NonCutEngrave, true},
		{"etch", geometry.NonCutEngrave, true},
		{"Marking", geometry.NonCutEngrave, true},
		{"SCORE_LINES", geometry.NonCutEngrave, true},
		{"scribe", geometry.NonCutEngrave, true},
		{"0", 0, false},
		{"CUT", 0, false},
		{"OUTER_PROFILES", 0, false},
		{"UNBEND", 0, false}, // a word must start with the keyword
		{"REMARKS", 0, false},
	}
	for _, tt := range tests {
		kind, ok := layerKind(tt.layer)
		if ok != tt.ok || kind != tt.kind {
			t.Errorf("layerKind(%q) = %v, %v; want %v, %v", tt.layer, kind, ok, tt.kind, tt.ok)
		}
	}
}

func TestBendAndEngravingLayers(t *testing.T) {
	data := withTables(dxf("", circleEnt,
		// The layer name wins over a dashed line type.
		"0\nLINE\n5\nB1\n8\nBEND\n6\nDASHED\n10\n-5\n20\n0\n11\n5\n21\n0\n",
		"0\nLINE\n5\nB2\n8\nFold lines\n10\n0\n20\n-5\n11\n0\n21\n5\n",
		"0\nLWPOLYLINE\n5\nE1\n8\nline engrave\n90\n2\n70\n0\n10\n1\n20\n1\n10\n2\n20\n1\n",
	))
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id   string
		kind geometry.NonCutKind
		x, y float64
	}{{"B1", geometry.NonCutBend, -5, 0}, {"B2", geometry.NonCutBend, 0, -5}, {"E1", geometry.NonCutEngrave, 1, 1}}
	if len(p.NonCut) != len(want) {
		t.Fatalf("non-cut = %+v, want %d entries", p.NonCut, len(want))
	}
	for i, w := range want {
		n := p.NonCut[i]
		if n.EntityID != w.id || n.Kind != w.kind || n.Position == nil || n.Position.X != w.x || n.Position.Y != w.y {
			t.Errorf("non-cut %d = %+v at %v, want %+v", i, n, n.Position, w)
		}
	}
	if len(p.Contours) != 0 || len(p.Circles) != 1 {
		t.Errorf("cut geometry: %d contours, %d circles; want only the circle", len(p.Contours), len(p.Circles))
	}

}

func TestOnlyNonCutGeometry(t *testing.T) {
	data := withTables(dxf("", "0\nCIRCLE\n5\nA1\n8\nOFF\n10\n0\n20\n0\n40\n5\n"), "0\nLAYER\n2\nOFF\n62\n-7\n")
	_, err := ParseDXF([]byte(data), "")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "construction") {
		t.Fatalf("err = %v", err)
	}
}

func TestNonCutGeometryIsStillValidated(t *testing.T) {
	data := withTables(dxf("", circleEnt, "0\nLINE\n5\nN1\n6\nDASHED\n10\n1\n20\n2\n"))
	if _, err := ParseDXF([]byte(data), ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestCommentsAreIgnored(t *testing.T) {
	data := "999\nwritten by a CAD program\n" + dxf("", "999\nbetween entities\n"+circleEnt+"999\nafter a tag\n")
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Circles) != 1 {
		t.Errorf("%d circles, want 1", len(p.Circles))
	}
}

func TestUnsupportedFormat(t *testing.T) {
	if _, err := Parse(Design{Format: "step"}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v", err)
	}
}

func TestCurves(t *testing.T) {
	const square = "0\nLWPOLYLINE\n5\nP1\n90\n4\n70\n1\n10\n-50\n20\n-50\n10\n50\n20\n-50\n10\n50\n20\n50\n10\n-50\n20\n50\n"
	h := "0.70710678118654757"
	circleSpline := "0\nSPLINE\n5\nS1\n70\n4\n71\n2\n72\n12\n73\n9\n" +
		"40\n0\n40\n0\n40\n0\n40\n0.25\n40\n0.25\n40\n0.5\n40\n0.5\n40\n0.75\n40\n0.75\n40\n1\n40\n1\n40\n1\n" +
		"41\n1\n41\n" + h + "\n41\n1\n41\n" + h + "\n41\n1\n41\n" + h + "\n41\n1\n41\n" + h + "\n41\n1\n" +
		"10\n10\n20\n0\n10\n10\n20\n10\n10\n0\n20\n10\n10\n-10\n20\n10\n10\n-10\n20\n0\n10\n-10\n20\n-10\n10\n0\n20\n-10\n10\n10\n20\n-10\n10\n10\n20\n0\n"
	tests := []struct {
		name    string
		ents    []string
		circles int     // full circles recognized
		area    float64 // area of the cut-out loop, mm²
	}{
		{"ellipse", []string{square, "0\nELLIPSE\n5\nE1\n10\n0\n20\n0\n11\n20\n21\n0\n40\n0.5\n41\n0\n42\n6.283185307179586\n"}, 0, math.Pi * 20 * 10},
		{"rotated mirrored ellipse", []string{square, "0\nELLIPSE\n5\nE1\n10\n0\n20\n0\n11\n0\n21\n20\n40\n0.5\n41\n1\n42\n1\n230\n-1\n"}, 0, math.Pi * 20 * 10},
		{"round ellipse is a circle", []string{square, "0\nELLIPSE\n5\nE1\n10\n0\n20\n0\n11\n10\n21\n0\n40\n1\n"}, 1, math.Pi * 100},
		// Half an ellipse closed by a line across its major axis.
		{"elliptical arc", []string{square, "0\nELLIPSE\n5\nE1\n10\n0\n20\n0\n11\n20\n21\n0\n40\n0.5\n41\n0\n42\n3.141592653589793\n", "0\nLINE\n5\nL1\n10\n-20\n20\n0\n11\n20\n21\n0\n"}, 0, math.Pi * 20 * 10 / 2},
		{"rational spline circle", []string{square, circleSpline}, 1, math.Pi * 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part, err := ParseDXF([]byte(dxf("", tt.ents...)), "")
			if err != nil {
				t.Fatal(err)
			}
			if len(part.Loops) != 2 || part.Loops[1].Depth != 1 {
				t.Fatalf("loops %+v, want the plate and one cut-out", part.Loops)
			}
			if got := len(part.Circles); got != tt.circles {
				t.Errorf("%d circles, want %d", got, tt.circles)
			}
			if a := part.Loops[1].AreaMM2; math.Abs(a-tt.area) > 0.01 {
				t.Errorf("cut-out area %g, want %g", a, tt.area)
			}
		})
	}
}

// TestEllipseRangeIsOneTurn guards against an ellipse whose parameter range spans hundreds of turns, which once took tens of seconds to fit.
func TestEllipseRangeIsOneTurn(t *testing.T) {
	data := dxf("", "0\nELLIPSE\n10\n0\n20\n0\n11\n7\n21\n0\n40\n0.1\n42\n4075\n")
	start := time.Now()
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
	if len(p.Contours) != 1 || !p.Contours[0].Closed {
		t.Errorf("contours %+v, want one closed ellipse", p.Contours)
	}
}

// TestBlockExpansionIsBounded rejects block references that would expand to more entities than any real sheet has.
func TestBlockExpansionIsBounded(t *testing.T) {
	data := withBlocks(dxf("", circleEnt, "0\nINSERT\n5\nI1\n2\nB\n10\n0\n20\n0\n70\n300\n71\n300\n44\n1\n45\n1\n"),
		blockDef("B", 0, 0, 0, circleEnt))
	if _, err := ParseDXF([]byte(data), ""); !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("err = %v, want unsupported content", err)
	}
}

func TestOffPlane(t *testing.T) {
	data := withBlocks(dxf("",
		"0\nLWPOLYLINE\n5\nP\n90\n4\n70\n1\n10\n0\n20\n0\n10\n50\n20\n0\n10\n50\n20\n50\n10\n0\n20\n50\n",
		"0\nCIRCLE\n5\nFLAT\n10\n10\n20\n10\n30\n0\n40\n2\n",
		"0\nCIRCLE\n5\nUP\n10\n20\n20\n10\n30\n5\n40\n2\n",                              // 5 mm above
		"0\nLINE\n5\nSLOPE\n10\n5\n20\n40\n30\n0\n11\n45\n21\n40\n31\n3\n",              // rises 3 mm
		"0\nARC\n5\nMIRROR\n10\n-30\n20\n10\n30\n0\n40\n2\n50\n0\n51\n90\n230\n-1\n",    // mirrored, still at 0
		"0\nLWPOLYLINE\n5\nRAISED\n38\n2\n90\n2\n70\n0\n10\n5\n20\n30\n10\n9\n20\n30\n", // elevation 2
		"0\nINSERT\n5\nI\n2\nB\n10\n0\n20\n0\n30\n4\n",                                  // block lifted 4 mm
		"0\nINSERT\n5\nJ\n2\nB\n10\n10\n20\n20\n"),
		blockDef("B", 0, 0, 0, "0\nCIRCLE\n5\nC\n10\n35\n20\n25\n40\n1\n"))
	p, err := ParseDXF([]byte(data), "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, o := range p.OffPlane {
		got[o.EntityID] = o.DistanceMM
	}
	want := map[string]float64{"UP": 5, "SLOPE": 3, "RAISED": 2, "I/C": 4}
	if len(got) != len(want) {
		t.Errorf("off plane %v, want %v", got, want)
	}
	for id, d := range want {
		if math.Abs(got[id]-d) > 1e-9 {
			t.Errorf("%s: %g off the plane, want %g", id, got[id], d)
		}
	}
}

func TestFlat3DPolyline(t *testing.T) {
	poly := func(z2 string) string {
		return "0\nPOLYLINE\n5\nP\n66\n1\n70\n9\n" + // closed 3D polyline
			"0\nVERTEX\n10\n0\n20\n0\n30\n0\n70\n32\n0\nVERTEX\n10\n10\n20\n0\n30\n0\n70\n32\n" +
			"0\nVERTEX\n10\n10\n20\n10\n30\n" + z2 + "\n70\n32\n0\nSEQEND\n"
	}
	p, err := ParseDXF([]byte(dxf("", poly("0"))), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Contours) != 1 || !p.Contours[0].Closed || len(p.Contours[0].Segments) != 3 || len(p.OffPlane) != 0 {
		t.Errorf("flat 3D polyline: %+v, off plane %v", p.Contours, p.OffPlane)
	}
	p, err = ParseDXF([]byte(dxf("", poly("4"))), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.OffPlane) != 1 || p.OffPlane[0].DistanceMM != 4 {
		t.Errorf("raised vertex: off plane %+v, want P 4 mm off", p.OffPlane)
	}
	mesh := "0\nPOLYLINE\n5\nM\n66\n1\n70\n16\n0\nSEQEND\n"
	if _, err := ParseDXF([]byte(dxf("", circleEnt, mesh)), ""); !errors.Is(err, ErrUnsupportedContent) {
		t.Errorf("mesh: err = %v", err)
	}
}

func TestMeasurementHint(t *testing.T) {
	for header, want := range map[string]string{
		"9\n$MEASUREMENT\n70\n1\n": "metric (likely millimeters)",
		"9\n$MEASUREMENT\n70\n0\n": "imperial (likely inches)",
	} {
		_, err := ParseDXF([]byte(dxf(header, circleEnt)), "")
		if !errors.Is(err, ErrUnitsUnknown) || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err %v, want units unknown mentioning %q", header, err, want)
		}
	}
}

func TestAnnotationLayers(t *testing.T) {
	for name, want := range map[string]bool{"DIMENSION": true, "Centerline": true, "CENTRE LINES": true, "Notes": true, "TITLE": true, "Bird layer - Delete": true, "Guides": true,
		"DIMPLE": false, "CUT": false, "notch": false, "Design Template": false} {
		k, ok := layerKind(name)
		if got := ok && k == geometry.NonCutAnnotation; got != want {
			t.Errorf("%s: annotation %v, want %v", name, got, want)
		}
	}
}

// TestFillReported reports a HATCH as annotation instead of dropping it.
func TestFillReported(t *testing.T) {
	dxf := "0\nSECTION\n2\nENTITIES\n0\nLWPOLYLINE\n5\nA1\n8\n0\n90\n4\n70\n1\n10\n0\n20\n0\n10\n10\n20\n0\n10\n10\n20\n10\n10\n0\n20\n10\n0\nHATCH\n5\nA2\n8\n0\n0\nENDSEC\n0\nEOF\n"
	p, err := Parse(Design{Format: constants.DesignFormatDXF, Data: []byte(dxf), Units: constants.LengthUnitMillimeter})
	if err != nil {
		t.Fatal(err)
	}
	if kinds(p)["A2"] != geometry.NonCutAnnotation {
		t.Errorf("hatch read as %v", kinds(p))
	}
}

// TestEllipseLongMinorAxis reads an ellipse whose ratio is above 1 (its "minor" axis the longer) as the same ellipse with the axes swapped.
func TestEllipseLongMinorAxis(t *testing.T) {
	data := "0\nSECTION\n2\nENTITIES\n0\nELLIPSE\n5\nE1\n8\n0\n10\n0\n20\n0\n11\n5\n21\n0\n40\n2\n0\nENDSEC\n0\nEOF\n"
	p, err := Parse(Design{Format: constants.DesignFormatDXF, Data: []byte(data), Units: constants.LengthUnitMillimeter})
	if err != nil {
		t.Fatal(err)
	}
	b := p.Bounds
	if math.Abs(b.WidthMM()-10) > 1e-3 || math.Abs(b.HeightMM()-20) > 1e-3 {
		t.Errorf("bounds %.4f x %.4f, want 10 x 20", b.WidthMM(), b.HeightMM())
	}
}
