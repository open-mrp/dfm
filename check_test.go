package dfm_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/internal/golden"
	"github.com/open-mrp/dfm/internal/schemacheck"
	"github.com/open-mrp/dfm/measure"
	"github.com/open-mrp/dfm/request"
	"github.com/open-mrp/dfm/resource"
	"github.com/open-mrp/dfm/rules"
	"github.com/open-mrp/dfm/schema"
)

var update = flag.Bool("update", false, "rewrite fixture *.expected.json golden files")

func laserCapability(t *testing.T) capability.Capability {
	t.Helper()
	data, err := os.ReadFile("examples/laser.json")
	if err != nil {
		t.Fatal(err)
	}
	c, aerr := request.DecodeCapability(data)
	if aerr != nil {
		t.Fatal(aerr)
	}
	return c
}

// corpusStock is the stock every fixture is checked with unless it has a <name>.stock.json sidecar.
var corpusStock = capability.Stock{MaterialCode: "mild_steel", Thickness: 1_500_000}

// designFiles are the design file extensions fixtures use.
var designFiles = []string{".dxf", ".svg", ".step"}

// pendingFiles are design formats the real corpus collects before the engine reads them; their drawings are listed but not checked yet.
var pendingFiles = []string{".stp", ".dwg", ".ai", ".eps", ".pdf"}

// formatOf returns a fixture's design format from its extension.
func formatOf(path string) constants.DesignFormat {
	return constants.DesignFormat(strings.TrimPrefix(filepath.Ext(path), "."))
}

// base returns a fixture's path without its design file extension.
func base(path string) string { return strings.TrimSuffix(path, filepath.Ext(path)) }

// globDesigns returns the design files matching pattern (without an extension) in every design format.
func globDesigns(t *testing.T, pattern string) []string {
	t.Helper()
	var out []string
	for _, ext := range designFiles {
		m, err := filepath.Glob(pattern + ext)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m...)
	}
	slices.Sort(out)
	return out
}

func checkFile(t *testing.T, path string, c capability.Capability, stock capability.Stock) (dfm.CheckResult, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return dfm.Check(context.Background(), dfm.CheckRequest{
		Design:         design.Design{Format: formatOf(path), Data: data},
		Capability:     c,
		Stock:          stock,
		HoleOperations: fixtureHoleOperations(t, path),
	})
}

// acceptAllFixes accepts every fix res suggests and checks the design again: the fixes must be valid together, no fix may be suggested again for an entity already adjusted, and there must be no more errors.
func acceptAllFixes(t *testing.T, req dfm.CheckRequest, res dfm.CheckResult) {
	t.Helper()
	seen := map[string]bool{}
	adjusted := map[string]bool{}
	errs := func(r dfm.CheckResult) int {
		n := 0
		for _, f := range r.Findings {
			if f.Severity == constants.SeverityError {
				n++
			}
		}
		return n
	}
	var adj []design.Adjustment
	for _, f := range res.Findings {
		if f.Fix == nil {
			continue
		}
		key := fmt.Sprint(f.Fix.Action, f.Fix.EntityIDs, f.Fix.Units)
		if seen[key] {
			continue
		}
		seen[key] = true
		adj = append(adj, design.Adjustment{Action: f.Fix.Action, EntityIDs: f.Fix.EntityIDs, Units: f.Fix.Units})
		for _, id := range f.Fix.EntityIDs {
			adjusted[id] = true
		}
	}
	if len(adj) == 0 {
		return
	}
	req.Design.Adjustments = adj
	again, err := dfm.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("accepting every fix: %v", err)
	}
	for _, f := range again.Findings {
		if f.Fix == nil {
			continue
		}
		if f.Fix.Action == constants.FixActionSetUnits && req.Design.Adjustments != nil {
			for _, a := range adj {
				if a.Action == constants.FixActionSetUnits {
					t.Errorf("set_units suggested again after it was accepted")
				}
			}
		}
		for _, id := range f.Fix.EntityIDs {
			if adjusted[id] {
				t.Errorf("%s fix for %s suggested again after it was accepted", f.Code, id)
			}
		}
	}
	if a, b := errs(res), errs(again); b > a {
		t.Errorf("accepting every fix raised the errors from %d to %d", a, b)
	}

	// The corrected file must say the same as the adjustments: checked on its own, it gives the same findings (entity IDs aside, as #n IDs move when an entity is deleted).
	corrected, err := dfm.Repair(context.Background(), req)
	var e *apierror.Error
	if errors.As(err, &e) && e.Code == constants.ErrorCodeDesignContentUnsupported {
		return // binary DXF, or an entity in a block reference
	}
	if err != nil {
		t.Fatalf("writing the corrected file: %v", err)
	}
	plain := req
	plain.Design.Data, plain.Design.Units, plain.Design.Adjustments = corrected, "", nil
	fromFile, err := dfm.Check(context.Background(), plain)
	if err != nil {
		t.Fatalf("checking the corrected file: %v", err)
	}
	if a, b := findingSummary(again), findingSummary(fromFile); !slices.Equal(a, b) {
		t.Errorf("the corrected file reads differently from the adjustments:\nadjusted: %v\nfile:     %v", a, b)
	}
}

// findingSummary lists a result's findings without entity or finding IDs, sorted.
func findingSummary(r dfm.CheckResult) []string {
	var out []string
	for _, f := range r.Findings {
		s := fmt.Sprintf("%s|%s|%v|%v", f.Code, f.Severity, f.Actual, f.Required)
		if f.Location != nil {
			s += fmt.Sprintf("|%d,%d", f.Location.X, f.Location.Y)
		}
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

// fixtureHoleOperations returns the hole assignments from the fixture's optional <name>.hole_operations.json sidecar.
func fixtureHoleOperations(t *testing.T, path string) []capability.HoleAssignment {
	t.Helper()
	data, err := os.ReadFile(base(path) + ".hole_operations.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	as, aerr := request.DecodeHoleAssignments(data)
	if aerr != nil {
		t.Fatalf("hole operations sidecar: %v", aerr)
	}
	return as
}

func fixtureStock(t *testing.T, dxfPath string) capability.Stock {
	t.Helper()
	data, err := os.ReadFile(base(dxfPath) + ".stock.json")
	if errors.Is(err, os.ErrNotExist) {
		return corpusStock
	}
	if err != nil {
		t.Fatal(err)
	}
	var in request.StockInput
	if aerr := request.Decode(data, &in, "stock"); aerr != nil {
		t.Fatalf("stock sidecar: %v", aerr)
	}
	s, aerr := in.ToDomain()
	if aerr != nil {
		t.Fatalf("stock sidecar: %v", aerr)
	}
	return s
}

// fixtureCapability returns the capability from the fixture's optional <name>.capability.json sidecar, or def.
func fixtureCapability(t *testing.T, dxfPath string, def capability.Capability) capability.Capability {
	t.Helper()
	data, err := os.ReadFile(base(dxfPath) + ".capability.json")
	if errors.Is(err, os.ErrNotExist) {
		return def
	}
	if err != nil {
		t.Fatal(err)
	}
	c, aerr := request.DecodeCapability(data)
	if aerr != nil {
		t.Fatalf("capability sidecar: %v", aerr)
	}
	return c
}

func loadSchema(t *testing.T, name string) *schemacheck.Schema {
	t.Helper()
	data, ok := schema.Get(name)
	if !ok {
		t.Fatalf("no schema %s", name)
	}
	s, err := schemacheck.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestFixtures runs every fixture against examples/laser.json and compares the JSON response (a dfm_check resource or an error envelope) with the fixture's .expected.json golden file. Run with -update to regenerate.
//
//   - fixtures/valid:     must be manufacturable
//   - fixtures/invalid:   must be not manufacturable
//   - fixtures/malformed: must return an error
//   - fixtures/exported:  drawings exported from CAD software; the golden
//     file records the expected verdict or error
func TestFixtures(t *testing.T) {
	c := laserCapability(t)
	checkSchema, errorSchema := loadSchema(t, "check"), loadSchema(t, "error")
	// fixtures/plan holds plans, checked by TestPlanFixtures; fixtures/perf holds benchmark drawings, which have no golden file.
	paths := slices.DeleteFunc(globDesigns(t, "fixtures/*/*"), func(p string) bool {
		g := filepath.Base(filepath.Dir(p))
		return g == "plan" || g == "perf"
	})
	if len(paths) == 0 {
		t.Fatal("no fixtures found")
	}
	seen := map[constants.FindingCode]bool{}
	for _, path := range paths {
		group := filepath.Base(filepath.Dir(path))
		t.Run(group+"/"+filepath.Base(base(path)), func(t *testing.T) {
			res, err := checkFile(t, path, fixtureCapability(t, path, c), fixtureStock(t, path))
			var out []byte
			var e *apierror.Error
			if group == "exported" && errors.As(err, &e) {
				group = "malformed"
			}
			switch group {
			case "valid", "invalid", "exported":
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if want := group == "valid"; group != "exported" && res.Manufacturable() != want {
					t.Errorf("manufacturable = %v, want %v; findings: %+v", res.Manufacturable(), want, res.Findings)
				}
				for _, f := range res.Findings {
					seen[f.Code] = true
				}
				data, _ := os.ReadFile(path)
				acceptAllFixes(t, dfm.CheckRequest{Design: design.Design{Format: formatOf(path), Data: data}, Capability: fixtureCapability(t, path, c), Stock: fixtureStock(t, path), HoleOperations: fixtureHoleOperations(t, path)}, res)
				out = encode(t, resource.PresentCheck(res))
				if err := checkSchema.Validate(out); err != nil {
					t.Errorf("schema: %v", err)
				}
			case "malformed":
				if !errors.As(err, &e) {
					t.Fatalf("want *apierror.Error, got result %+v, err %v", res, err)
				}
				out = encode(t, resource.ErrorResponse{Error: e})
				if err := errorSchema.Validate(out); err != nil {
					t.Errorf("schema: %v", err)
				}
			default:
				t.Fatalf("unknown fixture group %q", group)
			}
			out = golden.Mask(out, dfm.EngineVersion)
			goldenFile := base(path) + ".expected.json"
			if *update {
				if err := os.WriteFile(goldenFile, out, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("missing golden file (run go test . -run TestFixtures -update): %v", err)
			}
			if !bytes.Equal(out, want) {
				t.Errorf("result differs from %s\n--- got\n%s--- want\n%s", goldenFile, out, want)
			}
		})
	}
	for _, r := range rules.ForProcess(constants.ProcessLaserCutting) {
		if !seen[r.Code()] {
			t.Errorf("no fixture produces %s", r.Code())
		}
	}
}

// TestPartAttribution checks that drawing problems on a nested sheet are reported against the parts they are in.
func TestPartAttribution(t *testing.T) {
	path := "fixtures/invalid/nested_sheet_problems.dxf"
	res, err := checkFile(t, path, laserCapability(t), corpusStock)
	if err != nil {
		t.Fatal(err)
	}
	want := map[constants.FindingCode][]int{
		constants.FindingCodeGeometryDuplicate:           {1},
		constants.FindingCodeGeometryOverlappingSegments: {2, 3},
		constants.FindingCodeGeometryOpenContour:         {4},
		constants.FindingCodeTextNotOutlined:             nil,
		constants.FindingCodeFileMultipleParts:           nil,
	}
	for _, f := range res.Findings {
		w, ok := want[f.Code]
		if !ok {
			t.Errorf("unexpected finding %s", f.Code)
			continue
		}
		if !slices.Equal(f.Parts, w) {
			t.Errorf("%s: parts %v, want %v", f.Code, f.Parts, w)
		}
		delete(want, f.Code)
	}
	if len(want) > 0 {
		t.Errorf("missing findings %v", want)
	}
	var ok []bool
	for _, p := range res.Parts {
		ok = append(ok, p.Verdict == constants.VerdictManufacturable)
	}
	if !slices.Equal(ok, []bool{true, false, false, false}) || !res.Parts[3].Unclosed {
		t.Errorf("parts %+v", res.Parts)
	}
}

// TestResultSummary checks the parts of a result agents rely on beyond the findings: stable unique IDs, counts and the engine version.
func TestResultSummary(t *testing.T) {
	path := "fixtures/invalid/nested_sheet_problems.dxf"
	first, err := checkFile(t, path, laserCapability(t), corpusStock)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := checkFile(t, path, laserCapability(t), corpusStock)
	ids := map[string]bool{}
	errs, warns := 0, 0
	partErrs := map[int]int{}
	for i, f := range first.Findings {
		if f.ID == "" || ids[f.ID] || f.ID != again.Findings[i].ID {
			t.Errorf("finding %d id %q is empty, repeated or unstable (%q)", i, f.ID, again.Findings[i].ID)
		}
		ids[f.ID] = true
		if f.Severity == constants.SeverityError {
			errs++
			for _, n := range f.Parts {
				partErrs[n]++
			}
		} else {
			warns++
		}
	}
	if first.ErrorCount != errs || first.WarningCount != warns {
		t.Errorf("counts %d/%d, want %d/%d", first.ErrorCount, first.WarningCount, errs, warns)
	}
	for _, p := range first.Parts {
		if p.ErrorCount != partErrs[p.Number] {
			t.Errorf("part %d error count %d, want %d", p.Number, p.ErrorCount, partErrs[p.Number])
		}
	}
	if first.EngineVersion != dfm.EngineVersion {
		t.Errorf("engine version %q", first.EngineVersion)
	}
}

func minOnly(l measure.Length) capability.Limit {
	return capability.Limit{Error: &measure.Threshold{Min: l}}
}

// TestBoundaries re-checks valid fixtures with limits tightened just past the features they contain: exactly at a limit passes, 1 µm past it fails.
func TestBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		stock   capability.Stock
		modify  func(*capability.LaserCutting)
		want    constants.FindingCode // "" for no error finding
	}{
		{"hole at limit passes", "valid_holes", corpusStock, func(l *capability.LaserCutting) { l.Limits.HoleDiameter = minOnly(1_500_000) }, ""},
		{"hole limit raised fails", "valid_holes", corpusStock, func(l *capability.LaserCutting) { l.Limits.HoleDiameter = minOnly(1_501_000) }, constants.FindingCodeHoleTooSmall},
		{"slot at limit passes", "valid_slots", corpusStock, func(l *capability.LaserCutting) { l.Limits.SlotWidth = minOnly(1_000_000) }, ""},
		{"slot limit raised fails", "valid_slots", corpusStock, func(l *capability.LaserCutting) { l.Limits.SlotWidth = minOnly(1_001_000) }, constants.FindingCodeSlotTooNarrow},
		{"area exactly fits", "envelope_boundary", corpusStock, func(*capability.LaserCutting) {}, ""},
		{"area one micron short", "envelope_boundary", corpusStock, func(l *capability.LaserCutting) { l.UsableHeight = 2_999_999_000 }, constants.FindingCodePartExceedsEnvelope},
		{"rotation allowed", "rotated_fit", corpusStock, func(*capability.LaserCutting) {}, ""},
		{"outer circle is not a hole", "disc", corpusStock, func(l *capability.LaserCutting) { l.Limits.HoleDiameter = minOnly(101_000_000) }, constants.FindingCodeHoleTooSmall},
		{"inner circle of disc is a hole", "disc", corpusStock, func(l *capability.LaserCutting) { l.Limits.HoleDiameter = minOnly(10_001_000) }, constants.FindingCodeHoleTooSmall},
		{"inch drawing converted", "inches", corpusStock, func(l *capability.LaserCutting) { l.Limits.HoleDiameter = minOnly(1_587_500) }, ""},
		{"inch drawing converted fail", "inches", corpusStock, func(l *capability.LaserCutting) { l.Limits.HoleDiameter = minOnly(1_588_000) }, constants.FindingCodeHoleTooSmall},
		{"no-handle R12 hole detected", "polyline_r12_no_handles", corpusStock, func(l *capability.LaserCutting) { l.Limits.HoleDiameter = minOnly(3_100_000) }, constants.FindingCodeHoleTooSmall},
		// aluminum_5052 overrides the hole limit to max(1.5 mm, 1 × thickness).
		{"thickness ratio at limit passes", "valid_holes", capability.Stock{MaterialCode: "aluminum_5052", Thickness: 1_500_000}, func(*capability.LaserCutting) {}, ""},
		{"thickness ratio governs", "valid_holes", capability.Stock{MaterialCode: "aluminum_5052", Thickness: 1_501_000}, func(*capability.LaserCutting) {}, constants.FindingCodeHoleTooSmall},
		{"stock at max thickness passes", "valid_rectangle", capability.Stock{MaterialCode: "mild_steel", Thickness: 19_050_000}, func(*capability.LaserCutting) {}, ""},
		{"stock over max thickness fails", "valid_rectangle", capability.Stock{MaterialCode: "mild_steel", Thickness: 19_050_001}, func(*capability.LaserCutting) {}, constants.FindingCodeMaterialTooThick},
		{"unknown material fails", "valid_rectangle", capability.Stock{MaterialCode: "titanium", Thickness: 1_000_000}, func(*capability.LaserCutting) {}, constants.FindingCodeMaterialUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := laserCapability(t)
			lc := *c.LaserCutting
			tt.modify(&lc)
			c.LaserCutting = &lc
			res, err := checkFile(t, filepath.Join("fixtures/valid", tt.fixture+".dxf"), c, tt.stock)
			if err != nil {
				t.Fatal(err)
			}
			var codes []constants.FindingCode
			for _, f := range res.Findings {
				if f.Severity == constants.SeverityError {
					codes = append(codes, f.Code)
				}
			}
			if tt.want == "" && len(codes) != 0 {
				t.Errorf("want no errors, got %v", codes)
			}
			if tt.want != "" && (len(codes) != 1 || codes[0] != tt.want) {
				t.Errorf("want exactly [%s], got %v", tt.want, codes)
			}
			if res.Manufacturable() != (tt.want == "") {
				t.Errorf("manufacturable = %v", res.Manufacturable())
			}
		})
	}
}

func TestCheckErrors(t *testing.T) {
	good := laserCapability(t)
	rect, err := os.ReadFile("fixtures/valid/valid_rectangle.dxf")
	if err != nil {
		t.Fatal(err)
	}
	dxf := design.Design{Format: constants.DesignFormatDXF, Data: rect}
	tests := []struct {
		name  string
		req   dfm.CheckRequest
		code  constants.ErrorCode
		param string
	}{
		{"unsupported format", dfm.CheckRequest{Design: design.Design{Format: "iges", Data: rect}, Capability: good, Stock: corpusStock}, constants.ErrorCodeDesignFormatUnsupported, "design.format"},
		{"not a STEP file", dfm.CheckRequest{Design: design.Design{Format: constants.DesignFormatSTEP, Data: rect}, Capability: good, Stock: corpusStock}, constants.ErrorCodeDesignInvalid, "design"},
		{"bad units override", dfm.CheckRequest{Design: design.Design{Format: constants.DesignFormatDXF, Data: rect, Units: "furlong"}, Capability: good, Stock: corpusStock}, constants.ErrorCodeParameterInvalid, "design.units"},
		{"unknown process", dfm.CheckRequest{Design: dxf, Capability: capability.Capability{Process: "cnc_milling"}, Stock: corpusStock}, constants.ErrorCodeParameterInvalid, "capability.process"},
		{"missing laser block", dfm.CheckRequest{Design: dxf, Capability: capability.Capability{Process: constants.ProcessLaserCutting, Materials: good.Materials}, Stock: corpusStock}, constants.ErrorCodeParameterMissing, "capability.laser_cutting"},
		{"no material", dfm.CheckRequest{Design: dxf, Capability: good, Stock: capability.Stock{Thickness: 1}}, constants.ErrorCodeParameterMissing, "stock.material_code"},
		{"no thickness", dfm.CheckRequest{Design: dxf, Capability: good, Stock: capability.Stock{MaterialCode: "mild_steel"}}, constants.ErrorCodeParameterInvalid, "stock.thickness"},
		{"empty design", dfm.CheckRequest{Design: design.Design{Format: constants.DesignFormatDXF}, Capability: good, Stock: corpusStock}, constants.ErrorCodeDesignInvalid, "design"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dfm.Check(context.Background(), tt.req)
			e := dfm.AsError(err)
			if err == nil || e.Code != tt.code || e.Param != tt.param {
				t.Fatalf("got %v, want %s param %q", err, tt.code, tt.param)
			}
		})
	}
}

func TestCheckCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data, _ := os.ReadFile("fixtures/valid/valid_rectangle.dxf")
	_, err := dfm.Check(ctx, dfm.CheckRequest{Design: design.Design{Format: constants.DesignFormatDXF, Data: data}, Capability: laserCapability(t), Stock: corpusStock})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestResourceShape(t *testing.T) {
	res, err := checkFile(t, "fixtures/valid/valid_rectangle.dxf", laserCapability(t), corpusStock)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(encode(t, resource.PresentCheck(res)), &m); err != nil {
		t.Fatal(err)
	}
	// Empty lists are present and empty, never null: the drawing's issues, and the single part's.
	drawing := m["drawing_issues"].(map[string]any)
	parts := m["parts"].(map[string]any)["data"].([]any)
	issues := parts[0].(map[string]any)["issues"].(map[string]any)
	if m["object"] != "dfm_check" || drawing["object"] != "list" || len(drawing["data"].([]any)) != 0 ||
		len(parts) != 1 || issues["object"] != "list" || len(issues["data"].([]any)) != 0 {
		t.Fatalf("unexpected shape: %v", m)
	}
}

// corpusRates measures the engine on the corpus (mcp-requirements F5): for known-good drawings, how many each error code fails, by why (our stricter limit, a recorded defect, or unexplained: a false positive to chase); for known-bad drawings, which ones pass (false negatives). It is kept in rates.json, so a change shows in review.
type corpusRates struct {
	Good       int                                 `json:"good_checked"`
	GoodPass   int                                 `json:"good_manufacturable"`
	GoodReview int                                 `json:"good_needs_review"`
	GoodByCode map[constants.FindingCode]*codeRate `json:"good_failing_by_code"`
	Bad        int                                 `json:"bad_checked"`
	BadMissed  []string                            `json:"bad_manufacturable"`
}

type codeRate struct {
	StricterLimit int `json:"stricter_limit"`
	Defect        int `json:"defect"`
	AssumedInput  int `json:"assumed_input"`
	Unexplained   int `json:"unexplained"`
}

// checkRates compares the rates with the file, or writes it with -update.
func checkRates(t *testing.T, path string, r corpusRates) {
	slices.Sort(r.BadMissed)
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, '\n')
	if *update {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing %s (run go test . -run TestRealCorpus -update): %v", path, err)
	}
	if !bytes.Equal(out, want) {
		t.Errorf("corpus rates changed; review and run go test . -run TestRealCorpus -update\n--- now\n%s--- recorded\n%s", out, want)
	}
}

// corpusManifest is the manifest.json of the corpus of real drawings (fixtures/private, fetched by scripts/private-corpus; published open-hardware parts and proprietary ones alike).
type corpusManifest struct {
	Fixtures []struct {
		File       string          `json:"file"`
		SHA256     string          `json:"sha256"`
		Status     string          `json:"status"`
		Capability string          `json:"capability"`
		Stock      json.RawMessage `json:"stock"`
		Units      *string         `json:"units"`
		// KnownErrors are error codes the drawing is expected to get: where the corpus capability is stricter than the shop that made it (reason stricter_limit), a real defect the shop tolerated or fixed by hand (reason defect), or an input the drawing does not carry and the test assumes, such as a DXF's bend directions (reason assumed_input).
		KnownErrors []struct {
			Code   constants.FindingCode `json:"code"`
			Reason string                `json:"reason"`
			Note   string                `json:"note"`
		} `json:"known_errors"`
	} `json:"fixtures"`
	Sources []struct {
		Project     string `json:"project"`
		LicenseFile string `json:"license_file"`
	} `json:"sources"`
}

// TestPrivateCorpus checks every real drawing of the private corpus (published open-hardware parts and proprietary ones alike, under opaque case IDs; see docs/private-corpus.md) with its own stock, capability and units, and compares the result with its .expected.json golden file (run with -update to regenerate). A golden file records what the engine does today, not what it should do; the scorecard logged with -v shows how many known-good drawings pass. It runs when the corpus has been fetched. Its output never contains a drawing's content: failures name the case and the finding codes whose counts changed, nothing else.
func TestPrivateCorpus(t *testing.T) {
	if _, err := os.Stat("fixtures/private/manifest.json"); errors.Is(err, os.ErrNotExist) {
		t.Skip("private corpus not fetched; see docs/private-corpus.md")
	}
	m := runCorpus(t, "fixtures/private", true)
	for _, s := range m.Sources {
		if _, err := os.Stat(filepath.Join("fixtures/private", s.LicenseFile)); err != nil {
			t.Errorf("source %s: %v", s.Project, err)
		}
	}
}

// runCorpus checks every drawing listed in dir/manifest.json against its golden file. Capabilities are looked up in dir, then in fixtures/capabilities. With private set, every file must match its manifest hash and failures are redacted.
func runCorpus(t *testing.T, dir string, private bool) corpusManifest {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m corpusManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("%s/manifest.json: %v", dir, err)
	}
	listed := map[string]bool{}
	for _, f := range m.Fixtures {
		listed[f.File] = true
	}
	files := globDesigns(t, filepath.Join(dir, "*", "*"))
	for _, ext := range pendingFiles {
		m, err := filepath.Glob(filepath.Join(dir, "*", "*"+ext))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	for _, p := range files {
		if rel, _ := filepath.Rel(dir, p); !listed[filepath.ToSlash(rel)] {
			t.Errorf("%s is not in the manifest", p)
		}
	}

	checkSchema, errorSchema, planSchema := loadSchema(t, "check"), loadSchema(t, "error"), loadSchema(t, "plan")
	caps := map[string]capability.Capability{}
	// A drawing that is not manufacturable fails only on known errors, or for reasons not yet explained (the false positives to chase).
	type tally struct{ pass, review, limits, defects, assumed, unexplained, rejected, pending int }
	score := map[string]*tally{}
	rates := corpusRates{GoodByCode: map[constants.FindingCode]*codeRate{}, BadMissed: []string{}}
	var unexplained []string
	for _, f := range m.Fixtures {
		path := filepath.Join(dir, filepath.FromSlash(f.File))
		t.Run(base(f.File), func(t *testing.T) {
			if !slices.Contains(designFiles, filepath.Ext(path)) {
				if _, err := os.Stat(path); err != nil {
					t.Fatal(err)
				}
				s := score[f.Status]
				if s == nil {
					s = &tally{}
					score[f.Status] = s
				}
				s.pending++
				t.Skipf("%s files are collected but not read yet", filepath.Ext(path))
			}
			c, ok := caps[f.Capability]
			if !ok {
				c = corpusCapability(t, dir, f.Capability)
				caps[f.Capability] = c
			}
			var in request.StockInput
			if aerr := request.Decode(f.Stock, &in, "stock"); aerr != nil {
				t.Fatal(aerr)
			}
			stock, aerr := in.ToDomain()
			if aerr != nil {
				t.Fatal(aerr)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if f.SHA256 != "" {
				if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != f.SHA256 {
					t.Fatal("drawing does not match its manifest hash; fetch the corpus again")
				}
			}
			d := design.Design{Format: formatOf(path), Data: raw}
			if f.Units != nil {
				d.Units = constants.LengthUnit(*f.Units)
			}
			req := dfm.CheckRequest{Design: d, Capability: c, Stock: stock, HoleOperations: fixtureHoleOperations(t, path)}
			res, err := dfm.Check(context.Background(), req)
			if err == nil {
				acceptAllFixes(t, req, res)
			}
			// Metal parts are planned with a press brake too, so bent parts are checked for bending.
			var plan *dfm.PlanResult
			if err == nil && strings.HasPrefix(f.Capability, "capability-metal") {
				brake, ok := caps[pressBrake]
				if !ok {
					brake = corpusCapability(t, dir, pressBrake)
					caps[pressBrake] = brake
				}
				p, perr := dfm.CheckPlan(context.Background(), dfm.PlanRequest{Design: d, Stock: stock, Capabilities: []capability.Capability{c, brake}, HoleOperations: req.HoleOperations})
				if perr != nil {
					t.Fatal(redact(private, perr))
				}
				plan = &p
			}
			manufacturable := err == nil && res.Manufacturable() && (plan == nil || plan.Manufacturable())
			// A design needing review has no errors: a person decides, so it is neither a pass nor a false failure.
			review := err == nil && (plan != nil && plan.Verdict == constants.PlanVerdictNeedsReview || plan == nil && res.Verdict == constants.VerdictNeedsReview)

			s := score[f.Status]
			if s == nil {
				s = &tally{}
				score[f.Status] = s
			}
			var out []byte
			var e *apierror.Error
			switch {
			case errors.As(err, &e):
				s.rejected++
				out = encode(t, resource.ErrorResponse{Error: e})
				if err := errorSchema.Validate(out); err != nil {
					t.Errorf("error response does not match its schema: %v", redact(private, err))
				}
			case err != nil:
				t.Fatal(redact(private, err))
			default:
				errs := map[constants.FindingCode]bool{}
				findings := res.Findings
				if plan != nil {
					findings = nil
					for _, op := range plan.Operations {
						if op.Result != nil {
							findings = append(findings, op.Result.Findings...)
						}
					}
				}
				for _, fd := range findings {
					if fd.Severity == constants.SeverityError {
						errs[fd.Code] = true
					}
				}
				known, defect, assumed := map[constants.FindingCode]bool{}, false, false
				reason := map[constants.FindingCode]string{}
				for _, k := range f.KnownErrors {
					reason[k.Code] = k.Reason
					if !k.Code.IsValid() || (k.Reason != "stricter_limit" && k.Reason != "defect" && k.Reason != "assumed_input") || k.Note == "" {
						t.Errorf("known error %+v needs a finding code, a reason (stricter_limit, defect or assumed_input) and a note", k)
					}
					if !errs[k.Code] {
						t.Errorf("known error %s is no longer reported; update the manifest", k.Code)
					}
					known[k.Code] = true
					defect = defect || k.Reason == "defect"
					assumed = assumed || k.Reason == "assumed_input"
				}
				explained := true
				for c := range errs {
					explained = explained && known[c]
				}
				switch f.Status {
				case "good":
					rates.Good++
					if manufacturable {
						rates.GoodPass++
					}
					if review {
						rates.GoodReview++
					}
					for c := range errs {
						r := rates.GoodByCode[c]
						if r == nil {
							r = &codeRate{}
							rates.GoodByCode[c] = r
						}
						switch reason[c] {
						case "stricter_limit":
							r.StricterLimit++
						case "defect":
							r.Defect++
						case "assumed_input":
							r.AssumedInput++
						default:
							r.Unexplained++
						}
					}
				case "bad":
					rates.Bad++
					if manufacturable {
						rates.BadMissed = append(rates.BadMissed, f.File)
					}
				}
				switch {
				case manufacturable:
					s.pass++
				case review:
					s.review++
				case !explained:
					s.unexplained++
					if f.Status == "good" {
						unexplained = append(unexplained, f.File)
					}
				case defect:
					s.defects++
				case assumed:
					s.assumed++
				default:
					s.limits++
				}
				if plan != nil {
					out = encode(t, resource.PresentPlan(*plan))
					if err := planSchema.Validate(out); err != nil {
						t.Errorf("plan does not match its schema: %v", redact(private, err))
					}
				} else {
					out = encode(t, resource.PresentCheck(res))
					if err := checkSchema.Validate(out); err != nil {
						t.Errorf("result does not match its schema: %v", redact(private, err))
					}
				}
			}
			out = golden.Mask(out, dfm.EngineVersion)
			goldenFile := base(path) + ".expected.json"
			if filepath.Ext(path) == ".step" {
				// A model's flat DXF is often saved beside it, under the same name.
				goldenFile = path + ".expected.json"
			}
			if *update {
				if err := os.WriteFile(goldenFile, out, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("missing golden file (run go test . -run %s -update): %v", t.Name()[:strings.IndexByte(t.Name(), '/')], redact(private, err))
			}
			if bytes.Equal(out, want) {
				return
			}
			if private {
				t.Errorf("result differs from its golden file; finding counts (code/severity: golden -> now): %s", countDiff(want, out))
				return
			}
			t.Errorf("result differs from %s\n--- got\n%s--- want\n%s", goldenFile, out, want)
		})
	}
	t.Run("rates", func(t *testing.T) { checkRates(t, filepath.Join(dir, "rates.json"), rates) })
	for _, status := range []string{"good", "bad", "unknown"} {
		if s := score[status]; s != nil {
			t.Logf("%s drawings: %d manufacturable, %d needing review; not manufacturable: %d only on stricter limits, %d on known defects, %d on assumed inputs, %d unexplained; %d rejected as input errors; %d in formats not read yet",
				status, s.pass, s.review, s.limits, s.defects, s.assumed, s.unexplained, s.rejected, s.pending)
		}
	}
	if len(unexplained) > 0 {
		t.Logf("good drawings failing for unexplained reasons: %s", strings.Join(unexplained, ", "))
	}
	return m
}

// pressBrake is the corpus capability metal parts are bent with.
const pressBrake = "capability-press-brake.json"

// corpusCapability loads a capability named in a corpus manifest from the corpus directory, or else from fixtures/capabilities.
func corpusCapability(t *testing.T, dir, name string) capability.Capability {
	t.Helper()
	if name != filepath.Base(name) {
		t.Fatalf("capability %q must be a file name", name)
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		raw, err = os.ReadFile(filepath.Join("fixtures/capabilities", name))
	}
	if err != nil {
		t.Fatal(err)
	}
	c, aerr := request.DecodeCapability(raw)
	if aerr != nil {
		t.Fatal(aerr)
	}
	return c
}

// redact hides an error's text for private drawings: messages can quote file content, entity handles or measurements.
func redact(private bool, err error) any {
	if !private {
		return err
	}
	return "(details hidden for private drawings)"
}

// countDiff summarizes how two results differ without revealing them: the response kind (result or error code) and, per finding code and severity, how many findings each has.
func countDiff(want, got []byte) string {
	summary := func(b []byte) (string, map[string]int) {
		type issue struct {
			Code        string `json:"code"`
			Severity    string `json:"severity"`
			Occurrences struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"occurrences"`
		}
		type issues struct {
			Data []issue `json:"data"`
		}
		var v struct {
			Verdict string `json:"verdict"`
			Error   *struct {
				Code string `json:"code"`
			} `json:"error"`
			Parts struct {
				Data []struct {
					Issues issues `json:"issues"`
				} `json:"data"`
			} `json:"parts"`
			DrawingIssues issues `json:"drawing_issues"`
		}
		if json.Unmarshal(b, &v) != nil {
			return "unreadable", nil
		}
		if v.Error != nil {
			return "error " + v.Error.Code, nil
		}
		// An occurrence between two parts is listed under both; count it once.
		counts, seen := map[string]int{}, map[string]bool{}
		all := v.DrawingIssues.Data
		for _, p := range v.Parts.Data {
			all = append(all, p.Issues.Data...)
		}
		for _, is := range all {
			for _, o := range is.Occurrences.Data {
				if !seen[o.ID] {
					seen[o.ID] = true
					counts[is.Code+"/"+is.Severity]++
				}
			}
		}
		return v.Verdict, counts
	}
	wantKind, wantCounts := summary(want)
	gotKind, gotCounts := summary(got)
	keys := map[string]bool{}
	for k := range wantCounts {
		keys[k] = true
	}
	for k := range gotCounts {
		keys[k] = true
	}
	var parts []string
	if wantKind != gotKind {
		parts = append(parts, wantKind+" -> "+gotKind)
	}
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		if wantCounts[k] != gotCounts[k] {
			parts = append(parts, fmt.Sprintf("%s: %d -> %d", k, wantCounts[k], gotCounts[k]))
		}
	}
	if len(parts) == 0 {
		return "same findings; values or locations changed"
	}
	return strings.Join(parts, ", ")
}

// TestAdjustments checks that accepted fixes change how the drawing is read and that invalid ones are refused with the parameter at fault.
func TestAdjustments(t *testing.T) {
	c := laserCapability(t)
	run := func(path string, adj ...design.Adjustment) (dfm.CheckResult, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return dfm.Check(context.Background(), dfm.CheckRequest{
			Design:     design.Design{Format: formatOf(path), Data: data, Adjustments: adj},
			Capability: fixtureCapability(t, path, c), Stock: corpusStock,
		})
	}
	codes := func(r dfm.CheckResult) map[constants.FindingCode]int {
		out := map[constants.FindingCode]int{}
		for _, f := range r.Findings {
			out[f.Code]++
		}
		return out
	}
	// Each fix, accepted, resolves its finding.
	for _, tt := range []struct {
		path string
		code constants.FindingCode
		then constants.FindingCode
	}{
		{"fixtures/invalid/solid_bend_line.dxf", constants.FindingCodeBendLineSuspected, constants.FindingCodeBendLineNotCut},
		{"fixtures/invalid/center_lines.dxf", constants.FindingCodeFileNoncutGeometry, ""},
		{"fixtures/valid/duplicate_line.dxf", constants.FindingCodeGeometryDuplicate, ""},
		{"fixtures/invalid/inch_drawing_declared_mm.dxf", constants.FindingCodeUnitsSuspect, ""},
	} {
		res, err := run(tt.path)
		if err != nil {
			t.Fatal(err)
		}
		var adj []design.Adjustment
		for _, f := range res.Findings {
			if f.Code == tt.code {
				if f.Fix == nil {
					t.Fatalf("%s: %s has no fix", tt.path, tt.code)
				}
				adj = append(adj, design.Adjustment{Action: f.Fix.Action, EntityIDs: f.Fix.EntityIDs, Units: f.Fix.Units})
			}
		}
		if len(adj) == 0 {
			t.Fatalf("%s: no %s", tt.path, tt.code)
		}
		again, err := run(tt.path, adj...)
		if err != nil {
			t.Fatalf("%s: %v", tt.path, err)
		}
		if got := codes(again); got[tt.code] != 0 || (tt.then != "" && got[tt.then] == 0) {
			t.Errorf("%s: after accepting its fixes, findings %v", tt.path, got)
		}
	}

	bend := func(ids ...string) design.Adjustment {
		return design.Adjustment{Action: constants.FixActionMarkBendLine, EntityIDs: ids}
	}
	units := func(u constants.LengthUnit) design.Adjustment {
		return design.Adjustment{Action: constants.FixActionSetUnits, Units: u}
	}
	for _, tt := range []struct {
		name  string
		adj   []design.Adjustment
		param string
	}{
		{"no entities", []design.Adjustment{bend()}, "design.adjustments[0].entity_ids"},
		{"units on an entity fix", []design.Adjustment{{Action: constants.FixActionRemove, EntityIDs: []string{"32"}, Units: "mm"}}, "design.adjustments[0].units"},
		{"set_units without units", []design.Adjustment{{Action: constants.FixActionSetUnits}}, "design.adjustments[0].units"},
		{"set_units with entities", []design.Adjustment{{Action: constants.FixActionSetUnits, Units: "in", EntityIDs: []string{"32"}}}, "design.adjustments[0].entity_ids"},
		{"set_units twice", []design.Adjustment{units("in"), units("in")}, "design.adjustments[1]"},
		{"entity twice", []design.Adjustment{bend("32"), {Action: constants.FixActionRemove, EntityIDs: []string{"32"}}}, "design.adjustments[1].entity_ids[0]"},
		{"unknown action", []design.Adjustment{{Action: "explode", EntityIDs: []string{"32"}}}, "design.adjustments[0].action"},
		{"unknown entity", []design.Adjustment{bend("32"), bend("nope")}, "design.adjustments[1].entity_ids"},
	} {
		_, err := run("fixtures/invalid/solid_bend_line.dxf", tt.adj...)
		var e *apierror.Error
		if !errors.As(err, &e) || e.Param != tt.param {
			t.Errorf("%s: err = %v, want param %s", tt.name, err, tt.param)
		}
	}
}
