package dfm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/capabilities"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/internal/golden"
	"github.com/open-mrp/dfm/request"
	"github.com/open-mrp/dfm/resource"
)

// TestPlanFixtures checks every plan fixture: a design in fixtures/plan with a <name>.plan.json sidecar giving the stock and the capabilities (paths, or preset:<id>), against its .expected.json golden file (a dfm_plan resource; run with -update to regenerate).
func TestPlanFixtures(t *testing.T) {
	planSchema := loadSchema(t, "plan")
	for _, path := range globDesigns(t, "fixtures/plan/*") {
		t.Run(filepath.Base(base(path)), func(t *testing.T) {
			raw, err := os.ReadFile(base(path) + ".plan.json")
			if err != nil {
				t.Fatal(err)
			}
			var side struct {
				Stock        json.RawMessage `json:"stock"`
				Capabilities []string        `json:"capabilities"`
				Bends        json.RawMessage `json:"bends"`
				HoleOps      json.RawMessage `json:"hole_operations"`
			}
			if err := json.Unmarshal(raw, &side); err != nil {
				t.Fatal(err)
			}
			var stockIn request.StockInput
			if aerr := request.Decode(side.Stock, &stockIn, "stock"); aerr != nil {
				t.Fatal(aerr)
			}
			stock, aerr := stockIn.ToDomain()
			if aerr != nil {
				t.Fatal(aerr)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			req := dfm.PlanRequest{Design: design.Design{Format: formatOf(path), Data: data}, Stock: stock}
			if side.HoleOps != nil {
				if req.HoleOperations, aerr = request.DecodeHoleAssignments(side.HoleOps); aerr != nil {
					t.Fatal(aerr)
				}
			}
			if side.Bends != nil {
				if req.Bends, aerr = request.DecodeBendAssignments(side.Bends); aerr != nil {
					t.Fatal(aerr)
				}
			}
			for _, c := range side.Capabilities {
				req.Capabilities = append(req.Capabilities, planCapability(t, c))
			}
			res, err := dfm.CheckPlan(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			out := encode(t, resource.PresentPlan(res))
			if err := planSchema.Validate(out); err != nil {
				t.Errorf("schema: %v", err)
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
				t.Fatalf("missing golden file (run go test . -run TestPlanFixtures -update): %v", err)
			}
			if !bytes.Equal(out, want) {
				t.Errorf("result differs from %s\n--- got\n%s--- want\n%s", goldenFile, out, want)
			}
		})
	}
}

func planCapability(t *testing.T, ref string) capability.Capability {
	t.Helper()
	var data []byte
	if id, ok := strings.CutPrefix(ref, "preset:"); ok {
		var found bool
		if data, found = capabilities.Get(id); !found {
			t.Fatalf("no preset %s", id)
		}
	} else {
		var err error
		if data, err = os.ReadFile(ref); err != nil {
			t.Fatal(err)
		}
	}
	c, aerr := request.DecodeCapability(data)
	if aerr != nil {
		t.Fatal(aerr)
	}
	return c
}

// TestPlanRequests checks the plan's inputs: capabilities in any order give the same plan, and two for one process are refused.
func TestPlanRequests(t *testing.T) {
	data, _ := os.ReadFile("fixtures/plan/bent_bracket.dxf")
	laser, brake := planCapability(t, "examples/laser.json"), planCapability(t, "preset:trumpf-trubend-3100")
	stock := capability.Stock{MaterialCode: "mild_steel", Thickness: 1_500_000}
	run := func(caps ...capability.Capability) (dfm.PlanResult, error) {
		return dfm.CheckPlan(context.Background(), dfm.PlanRequest{Design: design.Design{Format: formatOf("x.dxf"), Data: data}, Stock: stock, Capabilities: caps})
	}
	a, err1 := run(laser, brake)
	b, err2 := run(brake, laser)
	if err1 != nil || err2 != nil || !bytes.Equal(encode(t, resource.PresentPlan(a)), encode(t, resource.PresentPlan(b))) {
		t.Errorf("capability order changes the plan: %v %v", err1, err2)
	}
	if _, err := run(laser, laser); err == nil || !strings.Contains(err.Error(), "capabilities[1].process") {
		t.Errorf("two laser capabilities: %v", err)
	}
	// A bend assignment must name bend lines; 2F is the outline.
	_, err := dfm.CheckPlan(context.Background(), dfm.PlanRequest{Design: design.Design{Format: formatOf("x.dxf"), Data: data}, Stock: stock, Capabilities: []capability.Capability{laser, brake},
		Bends: []capability.BendAssignment{{EntityIDs: []string{"30", "2F"}, Angle: 45_000_000}}})
	if err == nil || !strings.Contains(err.Error(), "bends[0].entity_ids[1]") || !strings.Contains(err.Error(), "not a bend line") {
		t.Errorf("bend on the outline: %v", err)
	}
}

// TestPlanFromJSON builds a plan request from its JSON document and runs it, and reports a bad capability under its index.
func TestPlanFromJSON(t *testing.T) {
	data, _ := os.ReadFile("fixtures/plan/bent_bracket.dxf")
	capJSON, _ := os.ReadFile("examples/laser.json")
	doc := `{"design": {"format": "dxf"}, "stock": {"material_code": "mild_steel", "thickness": {"value": "1.5", "unit": "mm"}}, "capabilities": [` + string(capJSON) + `]}`
	req, aerr := dfm.PlanFromJSON(data, []byte(doc))
	if aerr != nil {
		t.Fatal(aerr)
	}
	res, err := dfm.CheckPlan(context.Background(), req)
	if err != nil || len(res.Operations) != 2 || res.Verdict != constants.PlanVerdictIncomplete {
		t.Errorf("plan: %v, %d operations, verdict %s", err, len(res.Operations), res.Verdict)
	}
	_, aerr = dfm.PlanFromJSON(data, []byte(strings.Replace(doc, `"mild_steel"`, `"Mild Steel"`, 2)))
	if aerr == nil || aerr.Param != "capabilities[0].materials[0].code" {
		t.Errorf("bad material code: %v", aerr)
	}
}
