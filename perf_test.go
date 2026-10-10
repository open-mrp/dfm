package dfm_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/measure"
	"github.com/open-mrp/dfm/request"
)

func dfmLength(nm int64) measure.Length { return measure.Length(nm) }

// perfCases are checks that guard speed: a typical part, a drawing with many narrow places, a grid of hundreds of holes, a nested sheet of 100 parts whose curves are dense polylines (100,000 segments, like the largest real sheet seen), a bracket, a frame of curves and a bent model; all generated (fixtures/perf), so the benchmarks need nothing outside the repository. Budgets are generous (CI machines are slow and shared), so only a real regression, such as a search that grows with the square of the drawing, fails them. BenchmarkCheck measures the same cases; testdata/bench/baseline.txt holds a recorded run to compare with (see docs/development.md, Running tests).
var perfCases = []struct {
	name, path, capability, material string
	thickness                        int64
	budget                           time.Duration
}{
	{"typical", "fixtures/invalid/bracket.dxf", "examples/laser.json", "mild_steel", 1_500_000, time.Second},
	{"many_narrow_places", "fixtures/perf/narrow_places.dxf", "fixtures/capabilities/capability-nonmetal.json", "plywood", 3_000_000, 3 * time.Second},
	{"hole_grid", "fixtures/exported/fusion_hole_grid_400.dxf", "examples/laser.json", "mild_steel", 1_500_000, 2 * time.Second},
	{"nested_sheet", "fixtures/perf/nested_sheet.svg", "fixtures/capabilities/capability-nonmetal.json", "acrylic", 3_000_000, 10 * time.Second},
	{"bracket", "fixtures/perf/bracket.dxf", "fixtures/capabilities/capability-metal.json", "aluminum_6061", 3_175_000, 2 * time.Second},
	{"frame", "fixtures/perf/frame.svg", "fixtures/capabilities/capability-nonmetal.json", "acrylic", 5_000_000, 2 * time.Second},
	{"bent_step_model", "fixtures/perf/bent.step", "fixtures/capabilities/capability-metal.json", "aluminum_5052", 1_600_000, 3 * time.Second},
}

// planCases plan a bent STEP model: unfolding, the laser check and the bending check, without and with tooling to simulate.
var planCases = []struct {
	name    string
	tooling bool
	budget  time.Duration
}{
	{"step_bent", false, 3 * time.Second},
	{"step_bent_tooling", true, 6 * time.Second},
}

func planPerfRequest(tb testing.TB, tooling bool) dfm.PlanRequest {
	tb.Helper()
	read := func(path string) capability.Capability {
		data, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		c, aerr := request.DecodeCapability(data)
		if aerr != nil {
			tb.Fatal(aerr)
		}
		return c
	}
	metal, brake := read("fixtures/capabilities/capability-metal.json"), read("fixtures/capabilities/capability-press-brake.json")
	// The corpus brake gives its tooling; the plain case takes it away.
	bending := *brake.Bending
	bending.Tooling = nil
	brake.Bending = &bending
	if tooling {
		bending := *brake.Bending
		bending.Tooling = &capability.BrakeTooling{
			PunchTipAngle: 86_000_000, PunchWidth: measure.MustFromMM(12), PunchHeight: measure.MustFromMM(100), PunchLength: measure.MustFromMM(500),
			DieOpening: measure.Threshold{Min: measure.MustFromMM(12)}, DieWidth: measure.MustFromMM(15), DieHeight: measure.MustFromMM(60), DieLength: measure.MustFromMM(500),
		}
		brake.Bending = &bending
	}
	data, err := os.ReadFile("fixtures/perf/bent.step")
	if err != nil {
		tb.Fatal(err)
	}
	return dfm.PlanRequest{
		Design:       design.Design{Format: constants.DesignFormatSTEP, Data: data},
		Stock:        capability.Stock{MaterialCode: "aluminum_5052", Thickness: dfmLength(1_600_000)},
		Capabilities: []capability.Capability{metal, brake},
	}
}

func perfRequest(tb testing.TB, path, capPath, material string, thickness int64) dfm.CheckRequest {
	tb.Helper()
	capData, err := os.ReadFile(capPath)
	if err != nil {
		tb.Fatal(err)
	}
	c, aerr := request.DecodeCapability(capData)
	if aerr != nil {
		tb.Fatal(aerr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	return dfm.CheckRequest{
		Design:     design.Design{Format: formatOf(path), Data: data},
		Capability: c,
		Stock:      capability.Stock{MaterialCode: material, Thickness: dfmLength(thickness)},
	}
}

func TestCheckTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	for _, pc := range perfCases {
		req := perfRequest(t, pc.path, pc.capability, pc.material, pc.thickness)
		start := time.Now()
		if _, err := dfm.Check(context.Background(), req); err != nil {
			t.Fatalf("%s: %v", pc.name, err)
		}
		if d := time.Since(start); d > pc.budget {
			t.Errorf("%s (%s) took %v, budget %v", pc.name, pc.path, d, pc.budget)
		}
	}
	for _, pc := range planCases {
		req := planPerfRequest(t, pc.tooling)
		start := time.Now()
		if _, err := dfm.CheckPlan(context.Background(), req); err != nil {
			t.Fatalf("%s: %v", pc.name, err)
		}
		if d := time.Since(start); d > pc.budget {
			t.Errorf("%s took %v, budget %v", pc.name, d, pc.budget)
		}
	}
}

func BenchmarkPlan(b *testing.B) {
	for _, pc := range planCases {
		req := planPerfRequest(b, pc.tooling)
		b.Run(pc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := dfm.CheckPlan(context.Background(), req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCheck(b *testing.B) {
	for _, pc := range perfCases {
		req := perfRequest(b, pc.path, pc.capability, pc.material, pc.thickness)
		b.Run(pc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := dfm.Check(context.Background(), req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
