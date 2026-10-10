package rules

import (
	"slices"
	"strconv"
	"testing"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// testProfile is 1.5 mm mild steel cut on a 1500x3000 mm area with a 1.5 mm minimum hole and a 1.0 mm minimum slot.
func testProfile() *capability.Profile {
	return &capability.Profile{
		Process:      constants.ProcessLaserCutting,
		MaterialCode: "mild_steel",
		Thickness:    1_500_000,
		Supported:    true,
		LaserCutting: &capability.LaserCuttingProfile{
			UsableWidth:  1500 * measure.Millimeter,
			UsableHeight: 3000 * measure.Millimeter,
			KerfWidth:    150_000,
			MaxThickness: 19_050_000,
			HoleDiameter: capability.Limit{Error: &measure.Threshold{Min: 1_500_000}},
			SlotWidth:    capability.Limit{Error: &measure.Threshold{Min: 1_000_000}},
		},
	}
}

func box(w, h float64) geometry.Bounds { return geometry.Bounds{MaxX: w, MaxY: h} }

func seg(id string) geometry.Segment {
	return geometry.Segment{Start: geometry.Point{X: 0, Y: 0}, End: geometry.Point{X: 1, Y: 0}, EntityID: id}
}

type want struct {
	ids      []string
	actual   string // exact decimal, "" to skip the numeric check
	required string
}

func runRule(t *testing.T, r Rule, part *geometry.Part, wants []want) {
	t.Helper()
	got := r.Check(part, testProfile())
	if len(got) != len(wants) {
		t.Fatalf("got %d findings %+v, want %d", len(got), got, len(wants))
	}
	for i, f := range got {
		if f.Code != r.Code() || f.Severity != constants.SeverityError || f.Message == "" {
			t.Errorf("finding %d = %+v", i, f)
		}
		w := wants[i]
		if !slices.Equal(f.EntityIDs, w.ids) {
			t.Errorf("finding %d ids = %v, want %v", i, f.EntityIDs, w.ids)
		}
		if w.required != "" {
			if f.Actual == nil || f.Required == nil || f.Actual.String() != w.actual || f.Required.String() != w.required {
				t.Errorf("finding %d actual/required = %v/%v, want %v/%v", i, f.Actual, f.Required, w.actual, w.required)
			}
		}
	}
}

func TestOpenContourRule(t *testing.T) {
	at := geometry.Point{X: 1, Y: 2}
	part := &geometry.Part{
		Contours: []geometry.Contour{
			{Segments: []geometry.Segment{seg("a"), seg("b"), seg("a")}},
			{Segments: []geometry.Segment{seg("c")}, Closed: true},
			{Segments: []geometry.Segment{seg("d")}},
		},
		Openings: []geometry.Opening{{Contour: 0, GapMM: 0.25, Location: at}, {Contour: 2, GapMM: 3, Location: at}},
	}
	got := OpenContourRule{}.Check(part, testProfile())
	if len(got) != 2 {
		t.Fatalf("got %d findings %+v, want 2", len(got), got)
	}
	for i, w := range []struct {
		ids []string
		gap string
	}{{[]string{"a", "b"}, "0.25"}, {[]string{"d"}, "3"}} {
		f := got[i]
		if f.Severity != constants.SeverityError || !slices.Equal(f.EntityIDs, w.ids) || f.Actual.String() != w.gap || f.Location == nil {
			t.Errorf("finding %d = %+v, want ids %v gap %s with a location", i, f, w.ids, w.gap)
		}
	}
	if got := (OpenContourRule{}).Check(&geometry.Part{Contours: part.Contours}, testProfile()); len(got) != 0 {
		t.Errorf("no openings measured: %+v", got)
	}
}

func TestGapClosedRule(t *testing.T) {
	part := &geometry.Part{Report: geometry.Report{ClosedGaps: []geometry.Gap{{EntityIDs: []string{"a", "b"}, LengthMM: 0.02, Location: geometry.Point{X: 1, Y: 1}}}}}
	got := GapClosedRule{}.Check(part, testProfile())
	if len(got) != 1 || got[0].Severity != constants.SeverityWarning || got[0].Actual.String() != "0.02" || !slices.Equal(got[0].EntityIDs, []string{"a", "b"}) {
		t.Errorf("findings %+v", got)
	}
}

func TestEnvelopeRule(t *testing.T) {
	tests := []struct {
		name  string
		w, h  float64
		wants []want
	}{
		{"fits", 100, 50, nil},
		{"exactly bed size", 1500, 3000, nil},
		{"within float noise of bed", 1500 + 1e-9, 3000, nil},
		{"fits rotated", 3000, 1500, nil},
		{"long side too long", 1000, 3000.01, []want{{actual: "3000.01", required: "3000"}}},
		{"short side too wide", 1500.5, 2000, []want{{actual: "1500.5", required: "1500"}}},
		{"both too big reports long side", 1600, 3100, []want{{actual: "3100", required: "3000"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runRule(t, EnvelopeRule{}, &geometry.Part{Bounds: box(tt.w, tt.h)}, tt.wants)
		})
	}
}

func TestHoleRule(t *testing.T) {
	hole := func(d float64, id string) geometry.Circle {
		return geometry.Circle{Radius: d / 2, EntityIDs: []string{id}, Depth: 1}
	}
	tests := []struct {
		name    string
		circles []geometry.Circle
		wants   []want
	}{
		{"large hole passes", []geometry.Circle{hole(5, "a")}, nil},
		{"exactly minimum passes", []geometry.Circle{hole(1.5, "a")}, nil},
		{"just below minimum fails", []geometry.Circle{hole(1.499, "a")}, []want{{ids: []string{"a"}, actual: "1.499", required: "1.5"}}},
		{"small hole fails", []geometry.Circle{hole(5, "a"), hole(0.8, "b")}, []want{{ids: []string{"b"}, actual: "0.8", required: "1.5"}}},
		{"outer circle ignored", []geometry.Circle{{Radius: 0.5, EntityIDs: []string{"o"}, Depth: 0}}, nil},
		{"hole in an island is still a hole", []geometry.Circle{{Radius: 0.5, EntityIDs: []string{"h"}, Depth: 3}}, []want{{ids: []string{"h"}, actual: "1", required: "1.5"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runRule(t, HoleRule{}, &geometry.Part{Circles: tt.circles}, tt.wants)
		})
	}
}

func TestSlotRule(t *testing.T) {
	slot := func(w float64, id string) geometry.Slot {
		return geometry.Slot{Shape: geometry.SlotObround, WidthMM: w, LengthMM: 20, EntityIDs: []string{id}}
	}
	tests := []struct {
		name  string
		slots []geometry.Slot
		wants []want
	}{
		{"wide slot passes", []geometry.Slot{slot(3, "a")}, nil},
		{"exactly minimum passes", []geometry.Slot{slot(1.0, "a")}, nil},
		{"just below minimum fails", []geometry.Slot{slot(0.999, "a")}, []want{{ids: []string{"a"}, actual: "0.999", required: "1"}}},
		{"narrow slot fails", []geometry.Slot{slot(0.8, "a"), slot(2, "b")}, []want{{ids: []string{"a"}, actual: "0.8", required: "1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runRule(t, SlotRule{}, &geometry.Part{Slots: tt.slots}, tt.wants)
		})
	}
}

func TestZeroLimitDisablesCheck(t *testing.T) {
	p := testProfile()
	p.LaserCutting.HoleDiameter = capability.Limit{Error: &measure.Threshold{}}
	p.LaserCutting.SlotWidth = capability.Limit{Error: &measure.Threshold{}}
	part := &geometry.Part{
		Circles: []geometry.Circle{{Radius: 0.01, Depth: 1}},
		Slots:   []geometry.Slot{{WidthMM: 0.01}},
	}
	for _, r := range []Rule{HoleRule{}, SlotRule{}} {
		if got := r.Check(part, p); len(got) != 0 {
			t.Errorf("%s: %+v", r.Code(), got)
		}
	}
}

// TestTieredLimits uses a 2.032 mm sheet (not exactly representable in binary floating point) with hole limits error=0.5T, warning=1T.
func TestTieredLimits(t *testing.T) {
	p := testProfile()
	p.Thickness = 2_032_000
	p.LaserCutting.HoleDiameter = capability.Limit{
		Error:   &measure.Threshold{Min: 500_000, ThicknessRatio: 500_000},
		Warning: &measure.Threshold{ThicknessRatio: 1_000_000},
	}
	tests := []struct {
		diameter float64
		severity constants.Severity // "" for no finding
		required string
		basis    constants.ThresholdBasis
	}{
		{3, "", "", ""},
		{2.032, "", "", ""}, // exactly at the warning limit
		{2.031999, constants.SeverityWarning, "2.032", constants.ThresholdBasisThicknessRatio},
		{1.016, constants.SeverityWarning, "2.032", constants.ThresholdBasisThicknessRatio}, // exactly at the error limit
		{1.015999, constants.SeverityError, "1.016", constants.ThresholdBasisThicknessRatio},
		{0.4, constants.SeverityError, "1.016", constants.ThresholdBasisThicknessRatio},
	}
	for _, tt := range tests {
		part := &geometry.Part{Circles: []geometry.Circle{{Radius: tt.diameter / 2, Depth: 1, EntityIDs: []string{"h"}}}}
		got := HoleRule{}.Check(part, p)
		if tt.severity == "" {
			if len(got) != 0 {
				t.Errorf("Ø%v: unexpected %+v", tt.diameter, got)
			}
			continue
		}
		if len(got) != 1 || got[0].Severity != tt.severity || got[0].Required.String() != tt.required || *got[0].RequiredBasis != tt.basis {
			t.Errorf("Ø%v: got %+v, want %s required %s (%s)", tt.diameter, got, tt.severity, tt.required, tt.basis)
		}
	}
	// With a thin sheet the absolute floor governs.
	p.Thickness = 600_000
	got := HoleRule{}.Check(&geometry.Part{Circles: []geometry.Circle{{Radius: 0.2, Depth: 1}}}, p)
	if len(got) != 1 || got[0].Required.String() != "0.5" || *got[0].RequiredBasis != constants.ThresholdBasisMin {
		t.Errorf("thin sheet: %+v", got)
	}
}

func TestMaterialRules(t *testing.T) {
	tests := []struct {
		name      string
		supported bool
		thickness measure.Length
		codes     []constants.FindingCode
	}{
		{"supported and thin enough", true, 2 * measure.Millimeter, nil},
		{"exactly at max thickness", true, 19_050_000, nil},
		{"one nanometer too thick", true, 19_050_001, []constants.FindingCode{constants.FindingCodeMaterialTooThick}},
		{"unsupported material", false, 2 * measure.Millimeter, []constants.FindingCode{constants.FindingCodeMaterialUnsupported}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testProfile()
			p.Supported, p.Thickness = tt.supported, tt.thickness
			if !tt.supported {
				p.LaserCutting.MaxThickness = 0
			}
			var codes []constants.FindingCode
			for _, r := range []Rule{MaterialRule{}, ThicknessRule{}} {
				for _, f := range r.Check(&geometry.Part{}, p) {
					codes = append(codes, f.Code)
					if f.Code == constants.FindingCodeMaterialTooThick && (f.Actual.String() != "19.050001" || f.Required.String() != "19.05") {
						t.Errorf("too thick finding %+v", f)
					}
				}
			}
			if !slices.Equal(codes, tt.codes) {
				t.Errorf("codes %v, want %v", codes, tt.codes)
			}
		})
	}
}

func TestDefaultRulesHaveUniqueCodes(t *testing.T) {
	seen := map[constants.FindingCode]bool{}
	for _, r := range ForProcess(constants.ProcessLaserCutting) {
		if seen[r.Code()] {
			t.Errorf("duplicate code %s", r.Code())
		}
		seen[r.Code()] = true
	}
}

func TestHygieneRules(t *testing.T) {
	loc := geometry.Point{X: 1.23456789, Y: -2}
	part := &geometry.Part{
		Report: geometry.Report{
			ZeroLength: []string{"z1", "z2"},
			Text:       []geometry.Text{{EntityID: "t1", Position: &loc}},
			NonCut: []geometry.NonCut{
				{EntityID: "n1", Position: &loc},
				{EntityID: "b1", Kind: geometry.NonCutBend, Position: &loc},
				{EntityID: "e1", Kind: geometry.NonCutEngrave, Position: &loc},
			},
			Duplicates: []geometry.Duplicate{{Kept: []string{"k"}, Copies: []string{"d"}, Location: loc}},
			Overlaps:   []geometry.Overlap{{EntityIDs: []string{"a", "b"}, LengthMM: 2.5, Location: loc}},
		},
		Contacts: []geometry.Contact{
			{EntityIDs: []string{"c"}, SameContour: true, Location: loc},
			{EntityIDs: []string{"d", "e"}, Location: loc},
		},
	}
	tests := []struct {
		rule     Rule
		severity constants.Severity
		ids      [][]string
	}{
		{ZeroLengthRule{}, constants.SeverityWarning, [][]string{{"z1"}, {"z2"}}},
		{TextRule{}, constants.SeverityWarning, [][]string{{"t1"}}},
		{NonCutRule{}, constants.SeverityError, [][]string{{"n1"}}},
		{BendLineRule{}, constants.SeverityWarning, [][]string{{"b1"}}},
		{EngravingRule{}, constants.SeverityWarning, [][]string{{"e1"}}},
		{DuplicateRule{}, constants.SeverityWarning, [][]string{{"d"}}},
		{OverlapRule{}, constants.SeverityError, [][]string{{"a", "b"}}},
		{SelfIntersectionRule{}, constants.SeverityError, [][]string{{"c"}}},
		{ContactRule{}, constants.SeverityError, [][]string{{"d", "e"}}},
	}
	for _, tt := range tests {
		got := tt.rule.Check(part, testProfile())
		if len(got) != len(tt.ids) {
			t.Fatalf("%s: %d findings, want %d", tt.rule.Code(), len(got), len(tt.ids))
		}
		for i, f := range got {
			if f.Code != tt.rule.Code() || f.Severity != tt.severity || !slices.Equal(f.EntityIDs, tt.ids[i]) {
				t.Errorf("%s: finding %+v", tt.rule.Code(), f)
			}
			if tt.rule.Code() != constants.FindingCodeGeometryZeroLength && (f.Location == nil || f.Location.X.String() != "1.234568") {
				t.Errorf("%s: location %+v, want rounded to 1e-6 mm", tt.rule.Code(), f.Location)
			}
		}
	}
	if got := (OverlapRule{}).Check(part, testProfile()); got[0].Actual.String() != "2.5" {
		t.Errorf("overlap length %v", got[0].Actual)
	}
	if got := (ZeroLengthRule{}).Check(&geometry.Part{}, testProfile()); len(got) != 0 {
		t.Errorf("clean part: %+v", got)
	}
	if got := (TextRule{}).Check(&geometry.Part{Report: geometry.Report{Text: []geometry.Text{{EntityID: "t"}}}}, testProfile()); got[0].Location != nil {
		t.Errorf("unplaced text location %+v", got[0].Location)
	}
	if got := (NonCutRule{}).Check(&geometry.Part{Report: geometry.Report{NonCut: []geometry.NonCut{{EntityID: "n"}}}}, testProfile()); got[0].Location != nil {
		t.Errorf("unplaced non-cut location %+v", got[0].Location)
	}
}

func TestMultiplePartsRule(t *testing.T) {
	square := func(id string, x float64) geometry.Contour {
		pts := []geometry.Point{{X: x, Y: 0}, {X: x + 10, Y: 0}, {X: x + 10, Y: 10}, {X: x, Y: 10}}
		var segs []geometry.Segment
		for i, p := range pts {
			segs = append(segs, geometry.Segment{Start: p, End: pts[(i+1)%4], EntityID: id})
		}
		return geometry.Contour{Segments: segs, Closed: true}
	}
	stray := geometry.Contour{Segments: []geometry.Segment{{Start: geometry.Point{X: 50}, End: geometry.Point{X: 60}, EntityID: "stray"}}}
	tests := []struct {
		name  string
		paths []geometry.Contour
		want  []string // outline entity IDs in the finding; nil for no finding
	}{
		{"one part", []geometry.Contour{square("a", 0)}, nil},
		{"two parts", []geometry.Contour{square("a", 0), square("b", 20)}, []string{"a", "b"}},
		{"a stray line is not a part", []geometry.Contour{square("a", 0), stray}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MultiplePartsRule{}.Check(geometry.NewPart(tt.paths, nil), testProfile())
			if tt.want == nil && len(got) != 0 || tt.want != nil && (len(got) != 1 || !slices.Equal(got[0].EntityIDs, tt.want) || got[0].Severity != constants.SeverityWarning) {
				t.Errorf("findings %+v, want outline %v", got, tt.want)
			}
		})
	}
}

func TestTopologyRules(t *testing.T) {
	closed := func(id string, depth int) geometry.Contour {
		return geometry.Contour{Segments: []geometry.Segment{seg(id)}, Closed: true, Depth: depth}
	}
	tests := []struct {
		name    string
		part    geometry.Part
		islands [][]string
	}{
		{"single part with holes", geometry.Part{
			Contours: []geometry.Contour{closed("plate", 0), closed("slot", 1)},
			Circles:  []geometry.Circle{{EntityIDs: []string{"hole"}, Depth: 1}},
		}, nil},
		{"island and island inside an island's hole", geometry.Part{
			Contours: []geometry.Contour{closed("plate", 0), closed("window", 1)},
			Circles: []geometry.Circle{
				{EntityIDs: []string{"island"}, Radius: 10, Depth: 2},
				{EntityIDs: []string{"island-hole"}, Radius: 6, Depth: 3},
				{EntityIDs: []string{"inner-island"}, Radius: 3, Depth: 4},
				{EntityIDs: []string{"center-mark"}, Radius: 0.02, Depth: 4}, // no wider than the kerf
			},
		}, [][]string{{"island"}, {"inner-island"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			islands := FloatingIslandRule{}.Check(&tt.part, testProfile())
			if len(islands) != len(tt.islands) {
				t.Fatalf("islands: %+v, want %v", islands, tt.islands)
			}
			for i, f := range islands {
				if !slices.Equal(f.EntityIDs, tt.islands[i]) || f.Severity != constants.SeverityError {
					t.Errorf("island %+v, want %v", f, tt.islands[i])
				}
			}
		})
	}
}

func TestCatalog(t *testing.T) {
	seen := map[constants.FindingCode]bool{}
	for _, name := range constants.Process("").EnumValues() {
		process := constants.Process(name)
		cat := Catalog(process)
		inProcess := map[constants.FindingCode]CodeInfo{}
		for _, c := range cat {
			if _, dup := inProcess[c.Code]; dup {
				t.Errorf("%s: %s listed twice", process, c.Code)
			}
			inProcess[c.Code] = c
			seen[c.Code] = true
			if c.Description == "" || c.Remediation == "" || len(c.Severities) == 0 || !c.Scope.IsValid() {
				t.Errorf("%s: %s: incomplete entry %+v", process, c.Code, c)
			}
			if c.limitSet != nil && c.Limit == "" {
				t.Errorf("%s: %s: skippable without a limit", process, c.Code)
			}
		}
		// Catalog order is rule order, and every rule has the entry its scope and skipping are read from.
		var order, catOrder []constants.FindingCode
		for _, r := range ForProcess(process) {
			order = append(order, r.Code())
			if _, ok := inProcess[r.Code()]; !ok {
				t.Errorf("%s: %s has no catalog entry", process, r.Code())
			}
		}
		for _, c := range cat {
			catOrder = append(catOrder, c.Code)
		}
		if !slices.Equal(order, catOrder) {
			t.Errorf("%s: catalog order %v\nrule order %v", process, catOrder, order)
		}
	}
	for _, code := range constants.FindingCode("").EnumValues() {
		if !seen[constants.FindingCode(code)] {
			t.Errorf("%s has no catalog entry", code)
		}
	}
}

func TestSkipped(t *testing.T) {
	p := testProfile()
	p.LaserCutting.Optional = capability.OptionalLaserCuttingLimits{WebWidth: &capability.Limit{Error: &measure.Threshold{Min: 1}}}
	var got []constants.FindingCode
	for _, c := range Skipped(p) {
		got = append(got, c.Code)
	}
	want := []constants.FindingCode{
		constants.FindingCodePartTooSmall, constants.FindingCodePartTooShort, constants.FindingCodePartAreaTooSmall, constants.FindingCodePartTooSlender, constants.FindingCodePartMayFallThrough, constants.FindingCodeCutoutTooSlender, constants.FindingCodeNotchTooSlender, constants.FindingCodeSlotFitTooTight,
		constants.FindingCodeFeaturesTooClose, constants.FindingCodeFeatureTooCloseToEdge,
		constants.FindingCodeCutoutTooNarrow, constants.FindingCodeFeatureTooThin, constants.FindingCodeInternalCornerTooSharp,
		constants.FindingCodeTipTooSharp, constants.FindingCodeDensePatternHeatRisk,
	}
	if !slices.Equal(got, want) {
		t.Errorf("skipped %v, want %v", got, want)
	}
}

func TestPartSizeRules(t *testing.T) {
	// A 6 x 40 mm strip and a 30 x 30 mm plate.
	rect := func(id string, x, w, h float64) geometry.Contour {
		pts := []geometry.Point{{X: x}, {X: x + w}, {X: x + w, Y: h}, {X: x, Y: h}}
		var segs []geometry.Segment
		for i, p := range pts {
			segs = append(segs, geometry.Segment{Start: p, End: pts[(i+1)%4], EntityID: id})
		}
		return geometry.Contour{Segments: segs, Closed: true}
	}
	min := func(mm string) capability.Limit {
		return capability.Limit{Error: &measure.Threshold{Min: measure.MustFromMM(mustFloat(mm))}}
	}
	p := testProfile()
	length := min("45")
	p.LaserCutting.Optional.PartLength = &length
	p.LaserCutting.Optional.SlenderPart = []capability.SlenderTier{
		{MinLength: measure.MustFromMM(20), Width: min("5")},
		{MinLength: measure.MustFromMM(35), Width: min("8")},
	}
	for _, tt := range []struct {
		name          string
		paths         []geometry.Contour
		short, narrow []string // entity IDs of the findings
		required      string   // slender requirement for the strip
	}{
		{"strip", []geometry.Contour{rect("s", 0, 6, 40)}, []string{"s"}, []string{"s"}, "8"},
		{"plate", []geometry.Contour{rect("p", 0, 30, 30)}, []string{"p"}, nil, ""},
	} {
		part := geometry.NewPart(tt.paths, nil)
		var short, narrow []string
		for _, f := range (PartLengthRule{}).Check(part, p) {
			short = append(short, f.EntityIDs...)
		}
		for _, f := range (SlenderPartRule{}).Check(part, p) {
			narrow = append(narrow, f.EntityIDs...)
			if f.Required.String() != tt.required {
				t.Errorf("%s: required %v, want %s", tt.name, f.Required, tt.required)
			}
		}
		if !slices.Equal(short, tt.short) || !slices.Equal(narrow, tt.narrow) {
			t.Errorf("%s: too short %v, too slender %v; want %v, %v", tt.name, short, narrow, tt.short, tt.narrow)
		}
	}
}

func mustFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(err)
	}
	return f
}

func TestUnitsRule(t *testing.T) {
	plate := func(w, h float64, units constants.LengthUnit) *geometry.Part {
		pts := []geometry.Point{{}, {X: w}, {X: w, Y: h}, {Y: h}}
		var segs []geometry.Segment
		for i, p := range pts {
			segs = append(segs, geometry.Segment{Start: p, End: pts[(i+1)%4], EntityID: "o"})
		}
		part := geometry.NewPart([]geometry.Contour{{Segments: segs, Closed: true}}, nil)
		part.Units = units
		return part
	}
	p := testProfile() // 1500 x 3000 mm usable area
	mm, in := constants.LengthUnitMillimeter, constants.LengthUnitInch
	for _, tt := range []struct {
		name    string
		w, h    float64
		units   constants.LengthUnit
		suspect bool
	}{
		{"inch part read as mm", 4, 2, mm, true},          // 4 x 2 in read as 4 x 2 mm
		{"real small part", 8, 8, mm, false},              // 8 mm: small but plausible
		{"tiny even in inches", 0.05, 0.05, mm, false},    // 1.3 mm in inches: not the explanation
		{"mm part read as inches", 6000, 4000, in, true},  // a 236 x 157 mm part read as inches
		{"oversize part in mm", 1600, 3100, mm, false},    // genuinely too large
		{"too large either way", 90000, 90000, in, false}, // 3.5 m even in mm
		{"tiny part in inches", 4, 2, in, false},          // inches are not mis-read millimeters when small
		{"normal part", 100, 50, mm, false},
		{"unknown units", 4, 2, "", false},
	} {
		got := UnitsRule{}.Check(plate(tt.w, tt.h, tt.units), p)
		if (len(got) == 1) != tt.suspect {
			t.Errorf("%s: findings %+v, want suspect %v", tt.name, got, tt.suspect)
		}
	}
}
