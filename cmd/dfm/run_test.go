package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/internal/golden"
	"github.com/open-mrp/dfm/resource"
)

const laser = "../../examples/laser.json"

func fixture(rel string) string { return filepath.Join("../../fixtures", rel) }

func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	err := Run(context.Background(), args, &out, &errOut)
	var exit ExitCode
	switch {
	case err == nil:
		code = 0
	case errors.As(err, &exit):
		code = int(exit)
	default:
		code = -1
	}
	return code, out.String(), errOut.String()
}

// stock appends the flags every check needs.
func stock(args ...string) []string {
	return append(args, "--capability", laser, "--material", "mild_steel", "--thickness", "1.5mm")
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"manufacturable", stock("check", fixture("valid/valid_rectangle.dxf")), 0},
		{"flags before path", append(stock("check"), "--json", fixture("valid/valid_rectangle.dxf")), 0},
		{"thickness with space and inches", []string{"check", fixture("valid/valid_rectangle.dxf"), "--capability", laser, "--material", "mild_steel", "--thickness", "0.0625 in"}, 0},
		{"not manufacturable", stock("check", fixture("invalid/small_hole.dxf")), 1},
		{"unsupported material", []string{"check", fixture("valid/valid_rectangle.dxf"), "--capability", laser, "--material", "titanium", "--thickness", "1.5mm"}, 1},
		{"malformed design", stock("check", fixture("malformed/not_a_dxf.dxf")), 2},
		{"unitless without --units", stock("check", fixture("malformed/unitless.dxf")), 2},
		{"unitless with --units", append(stock("check", fixture("malformed/unitless.dxf")), "--units", "mm"), 0},
		{"--units scales the drawing", append(stock("check", fixture("valid/valid_rectangle.dxf")), "--units", "m"), 1},
		{"thickness without unit", []string{"check", fixture("valid/valid_rectangle.dxf"), "--capability", laser, "--material", "mild_steel", "--thickness", "1.5"}, 2},
		{"missing --material", []string{"check", fixture("valid/valid_rectangle.dxf"), "--capability", laser, "--thickness", "1.5mm"}, 2},
		{"missing design file", stock("check", "nope.dxf"), 2},
		{"two design files", stock("check", "a.dxf", "b.dxf"), 2},
		{"non-dxf extension", stock("check", laser), 2},
		{"unknown flag", []string{"check", "--bogus"}, 2},
		{"unknown command", []string{"lint"}, 2},
		{"no args", nil, 2},
		{"help", []string{"--help"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, out, errOut := runCLI(tt.args...); code != tt.code {
				t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, tt.code, out, errOut)
			}
		})
	}
}

// TestJSONMatchesLibrary checks that --json output is byte-identical to the golden files produced by the library (see TestFixtures in package dfm).
func TestJSONMatchesLibrary(t *testing.T) {
	for _, name := range []string{"valid/valid_rectangle", "invalid/small_hole", "invalid/multiple_violations", "malformed/spline_fit_points_only"} {
		t.Run(name, func(t *testing.T) {
			_, out, errOut := runCLI(append(stock("check", fixture(name+".dxf")), "--json")...)
			if errOut != "" {
				t.Errorf("stderr not empty in --json mode: %s", errOut)
			}
			want, err := os.ReadFile(fixture(name + ".expected.json"))
			if err != nil {
				t.Fatal(err)
			}
			// The golden file is indented for review; output is compact.
			var compact bytes.Buffer
			if err := json.Compact(&compact, golden.Unmask(want, dfm.EngineVersion)); err != nil {
				t.Fatal(err)
			}
			if out != compact.String()+"\n" {
				t.Errorf("stdout:\n%s\nwant:\n%s", out, compact.String())
			}
		})
	}
}

func TestJSONErrors(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		code  constants.ErrorCode
		param string
	}{
		{"bad capability", []string{"check", fixture("valid/valid_rectangle.dxf"), "--capability", fixture("valid/valid_rectangle.dxf"), "--material", "mild_steel", "--thickness", "1.5mm", "--json"}, constants.ErrorCodeInvalidFormat, "capability"},
		{"usage", []string{"check", "--json"}, constants.ErrorCodeParameterInvalid, ""},
		{"bad flag", []string{"check", "--json", "--nope"}, constants.ErrorCodeParameterInvalid, ""},
		{"unitless", append(stock("check", fixture("malformed/unitless.dxf")), "--json"), constants.ErrorCodeDesignUnitsUnknown, "design.units"},
		{"sub-nanometer thickness", []string{"check", fixture("valid/valid_rectangle.dxf"), "--capability", laser, "--material", "mild_steel", "--thickness", "1.0000001mm", "--json"}, constants.ErrorCodeParameterInvalid, "stock.thickness"},
		{"zero thickness", []string{"check", fixture("valid/valid_rectangle.dxf"), "--capability", laser, "--material", "mild_steel", "--thickness", "0mm", "--json"}, constants.ErrorCodeParameterInvalid, "stock.thickness"},
		{"unknown unit", []string{"check", fixture("valid/valid_rectangle.dxf"), "--capability", laser, "--material", "mild_steel", "--thickness", "2furlong", "--json"}, constants.ErrorCodeParameterInvalid, "stock.thickness"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, _ := runCLI(tt.args...)
			if code != 2 {
				t.Fatalf("exit %d", code)
			}
			var resp resource.ErrorResponse
			if err := json.Unmarshal([]byte(out), &resp); err != nil || resp.Error == nil || resp.Error.Code != tt.code || resp.Error.Param != tt.param {
				t.Fatalf("stdout %q: %v", out, err)
			}
		})
	}
}

func TestHumanOutput(t *testing.T) {
	_, out, _ := runCLI(stock("check", fixture("invalid/small_hole.dxf"))...)
	for _, s := range []string{"FAIL", "hole_too_small", "required 1.5 mm from min", "Part 1 (100 mm x 50 mm): not manufacturable"} {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q:\n%s", s, out)
		}
	}
	if strings.Contains(out, " parts, ") {
		t.Errorf("single-part output counts parts:\n%s", out)
	}
	for _, s := range []string{"(1 error, 0 warnings)", "Fix: Enlarge the hole", "Not checked (limit not set in the capability): part_too_short, part_area_too_small, part_too_slender, part_may_fall_through, cutout_too_slender, notch_too_slender, slot_fit_too_tight, internal_corner_too_sharp", "- 0.8 mm at (25, 25) [30]"} {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q:\n%s", s, out)
		}
	}

	_, out, _ = runCLI(stock("check", fixture("invalid/nested_sheet.dxf"))...)
	for _, s := range []string{"3 parts, 1 not manufacturable", "Part 2 (40 mm x 40 mm): not manufacturable\n  error   hole_too_small", "Drawing:\n  warning file_multiple_parts"} {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q:\n%s", s, out)
		}
	}
}

func TestCodesAndVersion(t *testing.T) {
	code, out, _ := runCLI("codes")
	if code != 0 || !strings.Contains(out, "hole_too_small (error/warning; part)") || !strings.Contains(out, "Limit: laser_cutting.limits.hole_diameter") {
		t.Errorf("codes exit %d:\n%s", code, out)
	}
	code, out, _ = runCLI("codes", "--json")
	var cat struct {
		Object string `json:"object"`
		Data   []struct {
			Code        string `json:"code"`
			Remediation string `json:"remediation"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &cat); err != nil || code != 0 || cat.Object != "list" || len(cat.Data) != len(constants.FindingCode("").EnumValues()) {
		t.Errorf("codes --json exit %d, %d codes, err %v", code, len(cat.Data), err)
	}
	if code, out, _ := runCLI("version"); code != 0 || strings.TrimSpace(out) != dfm.EngineVersion {
		t.Errorf("version exit %d: %q", code, out)
	}
}

func TestFix(t *testing.T) {
	dir := t.TempDir()
	adj := filepath.Join(dir, "adjustments.json")
	if err := os.WriteFile(adj, []byte(`[{ "action": "mark_bend_line", "entity_ids": ["32"] }]`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "fixed.dxf")
	if code, _, errOut := runCLI(append(stock("fix", fixture("invalid/solid_bend_line.dxf")), "--adjustments", adj, "--output", out)...); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	// The corrected file passes on its own.
	if code, stdout, _ := runCLI(stock("check", out)...); code != 0 || !strings.Contains(stdout, "bend_line_not_cut") {
		t.Errorf("checking the corrected file: exit %d\n%s", code, stdout)
	}
	if code, _, _ := runCLI(append(stock("fix", fixture("invalid/solid_bend_line.dxf")), "--adjustments", adj)...); code != 2 {
		t.Errorf("fix without --output: exit %d, want 2", code)
	}
}

func TestPreviewCommand(t *testing.T) {
	out := filepath.Join(t.TempDir(), "preview.svg")
	if code, _, errOut := runCLI(append(stock("preview", fixture("invalid/small_hole.dxf")), "--output", out)...); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	data, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(data), `class="finding error"`) {
		t.Errorf("preview: %v\n%s", err, data)
	}
}

func TestPresetCapability(t *testing.T) {
	if code, out, errOut := runCLI("check", fixture("valid/valid_rectangle.dxf"), "--capability", "preset:co2-100w-1300x900", "--material", "plywood", "--thickness", "6mm"); code != 0 {
		t.Errorf("preset check: exit %d\n%s%s", code, out, errOut)
	}
	if code, out, _ := runCLI("capabilities"); code != 0 || !strings.Contains(out, "preset:fiber-3kw-3015") {
		t.Errorf("capabilities: exit %d\n%s", code, out)
	}
	if code, _, _ := runCLI("check", fixture("valid/valid_rectangle.dxf"), "--capability", "preset:nope", "--material", "plywood", "--thickness", "6mm"); code != 2 {
		t.Errorf("unknown preset: exit %d, want 2", code)
	}
}

func TestPlanCommand(t *testing.T) {
	design := fixture("plan/bent_bracket.dxf")
	code, out, errOut := runCLI("plan", design, "--capability", laser, "--capability", "preset:trumpf-trubend-3100", "--material", "mild_steel", "--thickness", "1.5mm")
	if code != 0 || !strings.Contains(out, "1. laser_cutting") || !strings.Contains(out, "2. bending") {
		t.Errorf("plan: exit %d\n%s%s", code, out, errOut)
	}
	code, out, _ = runCLI("plan", design, "--capability", laser, "--material", "mild_steel", "--thickness", "1.5mm")
	if code != 1 || !strings.Contains(out, "not checked: no bending capability given") {
		t.Errorf("plan without a press brake: exit %d\n%s", code, out)
	}
	if code, out, _ := runCLI("plan", design, "--capability", laser, "--capability", "preset:trumpf-trubend-3100", "--material", "mild_steel", "--thickness", "1.5mm", "--json"); code != 0 || !strings.Contains(out, `"object":"dfm_plan"`) {
		t.Errorf("plan --json: exit %d\n%s", code, out)
	}
}

// TestReplayAndFeedbackFixture stores a check as the platform would (its design, request and result), replays it on this engine (nothing changed: exit 0), and turns what a report said about it into a feedback fixture.
func TestReplayAndFeedbackFixture(t *testing.T) {
	dir := t.TempDir()
	designPath := fixture("feedback/text-plate-v-spike/design.dxf")
	reqPath := fixture("feedback/text-plate-v-spike/request.json")
	designData, _ := os.ReadFile(designPath)
	reqJSON, _ := os.ReadFile(reqPath)
	req, aerr := dfm.RequestFromJSON(designData, reqJSON)
	if aerr != nil {
		t.Fatal(aerr)
	}
	res, err := dfm.Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := json.Marshal(resource.PresentCheck(res))
	resPath := filepath.Join(dir, "result.json")
	if err := os.WriteFile(resPath, stored, 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI("replay", designPath, "--request", reqPath, "--result", resPath)
	if code != 0 || !strings.Contains(out, "0 changed") {
		t.Errorf("replay: exit %d\n%s%s", code, out, errOut)
	}
	code, out, _ = runCLI("replay", designPath, "--request", reqPath, "--result", resPath, "--json")
	if code != 0 || !strings.Contains(out, `"changes":[]`) {
		t.Errorf("replay --json: exit %d\n%s", code, out)
	}
	fixtures := filepath.Join(dir, "fixtures")
	code, out, errOut = runCLI("feedback-fixture", designPath, "--request", reqPath, "--result", resPath, "--present", `{"code": "hole_too_small", "entity_ids": ["99"]}`, "--note", "a 1 mm hole is drawn at 99", "--name", "small-hole", "--dir", fixtures)
	if code != 0 {
		t.Fatalf("feedback-fixture: exit %d\n%s%s", code, out, errOut)
	}
	written := filepath.Join(fixtures, "small-hole")
	for _, name := range []string{"design.dxf", "request.json", "expect.json"} {
		if _, err := os.Stat(filepath.Join(written, name)); err != nil {
			t.Errorf("%s not written: %v", name, err)
		}
	}
	expJSON, _ := os.ReadFile(filepath.Join(written, "expect.json"))
	if !strings.Contains(string(expJSON), `"hole_too_small"`) || !strings.Contains(string(expJSON), `"present"`) {
		t.Errorf("expect.json:\n%s", expJSON)
	}
	if code, _, _ = runCLI("feedback-fixture", designPath, "--request", reqPath, "--result", resPath, "--present", `{"code": "hole_too_small", "entity_ids": ["99"]}`, "--note", "a 1 mm hole is drawn at 99", "--name", "small-hole", "--dir", fixtures); code != 2 {
		t.Errorf("writing over an existing fixture: exit %d", code)
	}
}
