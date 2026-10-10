package dfm_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/brake"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
	"github.com/open-mrp/dfm/sheet"
	"github.com/open-mrp/dfm/solid"
)

// bendsLabels are the labels of a part of the bend dataset this test uses.
type bendsLabels struct {
	TotalBends     int     `json:"total_bends"`
	ToolCollision  int     `json:"y_tool_collision"`
	UnfoldOverlap  int     `json:"y_unfolding_collision"`
	Thickness      float64 `json:"sheet_thickness"`
	MinBendRadius  float64 `json:"min_bend_radius"`
	MaxBendRadius  float64 `json:"max_bend_radius"`
	MinBendAngle   float64 `json:"min_bend_angle"`
	MaxBendAngle   float64 `json:"max_bend_angle"`
	FlatArea       float64 `json:"bbox_area_unfolded"` // in 1000 mm² (labeled cm²)
	RoundedFlanges int     `json:"num_rounded_flanges"`
}

// bendsStep is one bend of a part's sequence in the bend dataset (its _sequence.json, after the header): the order the dataset formed the bends in, and what each ran into.
type bendsStep struct {
	Angle  float64 `json:"bend_angle"`
	Radius float64 `json:"bend_radius"`
	Width  float64 `json:"edge_width"`
	Punch  bool    `json:"collision_punch"`
	Die    bool    `json:"collision_die"`
}

// bendsTooling is the dataset's punch and die (its paper: a 90° punch tip, a 10 mm punch, 300 mm long, and a 40 mm die opening). The rest was fitted to the labels: a punch reaching to the ram (1000 mm), a die 10 mm wide beside the opening, 100 mm deep and 300 mm long, its V matched to each bend, lying under the flange that stays put.
var bendsTooling = capability.BrakeTooling{
	PunchTipAngle: 90_000_000, PunchWidth: measure.MustFromMM(10), PunchHeight: measure.MustFromMM(1000), PunchLength: measure.MustFromMM(300),
	DieOpening: measure.Threshold{Min: measure.MustFromMM(40)}, DieWidth: measure.MustFromMM(10), DieHeight: measure.MustFromMM(100), DieLength: measure.MustFromMM(300),
}

// bendsPart is what the engine made of one part.
type bendsPart struct {
	id     string
	labels bendsLabels
	read   string // "" when unfolded, else the refusal
	bends  int
	thick  float64
	minR   float64
	maxR   float64
	minA   float64
	maxA   float64
	area   float64 // flat bounding box, 1000 mm²
	plan   constants.PlanVerdict
	codes  []string
	// sim is the simulation's outcome in the dataset's bend order: -1 when not simulated, else 1 when a bend ran into the tooling.
	sim   int
	bendC bendConfusion
	// foldBad is the fraction of the model's edge points that lie farther than half the thickness plus 5 mm (the sample spacing) from every sample of the part folded by the simulation: 0 when the folding agrees with the model.
	foldBad float64
	// tool is the severity of the engine's bend_tool_collision, or "".
	tool constants.Severity
}

// bendConfusion counts bends by label (first index) and simulated outcome (second): any collision, the punch's and the die's.
type bendConfusion struct{ any, punch, die [2][2]int }

// TestExternalBends reads, unfolds and plans every part of the bend dataset (the source of kind "bends" in the private corpus's outside data, fixtures/private/external; see docs/private-corpus.md), compares what the engine reads with the dataset's labels and the engine's findings with its tool collision and unfolding overlap labels, and compares the summary with the dataset's record under external/records. The whole dataset takes about twenty minutes here, so without DFM_BENDS_FULL it runs every fortieth part and only logs the summary; with it, and -update, it rewrites the record. Set DFM_BENDS_DIR to run on another directory of parts (such as the dataset repository's example files).
func TestExternalBends(t *testing.T) {
	root := os.Getenv("DFM_BENDS_DIR")
	full := os.Getenv("DFM_BENDS_FULL") != ""
	local := root == ""
	record := local && full
	var recordPath string
	if local {
		id := externalSource(t, "bends")
		root = filepath.Join(externalDir, id)
		recordPath = filepath.Join(externalDir, "records", id+".json")
		if _, err := os.Stat(filepath.Join(root, ".fetched")); err != nil {
			t.Skip("not fetched; scripts/private-corpus fetch-external " + id)
		}
	}
	var labelFiles []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, "_labels.json") {
			labelFiles = append(labelFiles, p)
		}
		return nil
	})
	slices.Sort(labelFiles)
	if len(labelFiles) == 0 {
		t.Fatal("no parts found")
	}
	if local && !full {
		var sample []string
		for i := 0; i < len(labelFiles); i += 40 {
			sample = append(sample, labelFiles[i])
		}
		labelFiles = sample
	}
	laser := planCapability(t, "fixtures/capabilities/capability-metal.json")
	brake := planCapability(t, "fixtures/capabilities/capability-press-brake.json")
	tooled := *brake.Bending
	tooled.Tooling = &bendsTooling
	brake.Bending = &tooled
	parts := make([]bendsPart, len(labelFiles))
	// Progress is logged every 5% with the agreement so far, so a long run shows it is getting somewhere. finished holds the parts done in any order; parts keeps the dataset's order for the record.
	var mu sync.Mutex
	var finished []bendsPart
	start := time.Now()
	step := max(len(labelFiles)/20, 1)
	var wg sync.WaitGroup
	work := make(chan int)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				p := bendsCheck(t, labelFiles[i], root, laser, brake)
				parts[i] = p
				mu.Lock()
				finished = append(finished, p)
				if n := len(finished); n%step == 0 && n < len(labelFiles) {
					t.Logf("%s", bendsProgress(finished, len(labelFiles), time.Since(start)))
				}
				mu.Unlock()
			}
		}()
	}
	for i := range labelFiles {
		work <- i
	}
	close(work)
	wg.Wait()
	sum := bendsSummarize(parts)
	out, _ := json.MarshalIndent(sum, "", "  ")
	t.Logf("%s", out)
	if record {
		compareRecord(t, recordPath, sum)
	}
}

func bendsCheck(t *testing.T, labelFile, root string, laser, brake capability.Capability) bendsPart {
	rel, _ := filepath.Rel(root, strings.TrimSuffix(labelFile, "_labels.json"))
	p := bendsPart{id: filepath.ToSlash(rel)}
	raw, err := os.ReadFile(labelFile)
	if err != nil || json.Unmarshal(raw, &p.labels) != nil {
		p.read = "labels unreadable"
		return p
	}
	data, err := os.ReadFile(strings.TrimSuffix(labelFile, "_labels.json") + ".stp")
	if err != nil {
		p.read = "model missing"
		return p
	}
	d := design.Design{Format: constants.DesignFormatSTEP, Data: data}
	m, err := design.ParseModel(d)
	var r *sheet.Result
	if err == nil {
		r, err = sheet.Flatten(m, sheet.Options{KFactor: 0.44})
	}
	if err != nil {
		p.read = refusal(err.Error())
		return p
	}
	p.bends, p.thick = len(r.Bends), r.ThicknessMM
	p.minR, p.maxR, p.minA, p.maxA = math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, b := range r.Bends {
		p.minR, p.maxR = min(p.minR, b.InsideRadiusMM), max(p.maxR, b.InsideRadiusMM)
		p.minA, p.maxA = min(p.minA, b.AngleDeg), max(p.maxA, b.AngleDeg)
	}
	p.area = r.Part.Bounds.WidthMM() * r.Part.Bounds.HeightMM() / 1000
	p.sim = -1
	if steps, ok := bendsSequence(strings.TrimSuffix(labelFile, "_labels.json") + "_sequence.json"); ok {
		p.sim, p.bendC, p.foldBad = bendsSimulate(m, r, steps)
	}
	thick, _ := measure.FromMM(math.Round(r.ThicknessMM*1000) / 1000)
	res, err := dfm.CheckPlan(context.Background(), dfm.PlanRequest{Design: d, Stock: capability.Stock{MaterialCode: "mild_steel", Thickness: thick}, Capabilities: []capability.Capability{laser, brake}})
	var e *apierror.Error
	switch {
	case errors.As(err, &e):
		p.plan = constants.PlanVerdict("error " + string(e.Code))
		if e.Code == constants.ErrorCodeInternalError {
			t.Errorf("%s: %s", p.id, e.Message)
		}
		return p
	case err != nil:
		t.Errorf("%s: %v", p.id, err)
		return p
	}
	p.plan = res.Verdict
	seen := map[string]bool{}
	for _, op := range res.Operations {
		if op.Result == nil {
			continue
		}
		for _, f := range op.Result.Findings {
			if f.Code == constants.FindingCodeBendToolCollision {
				p.tool = f.Severity
			}
			if f.Severity != constants.SeverityWarning && !seen[string(f.Code)] {
				seen[string(f.Code)] = true
				p.codes = append(p.codes, string(f.Code))
			}
		}
	}
	slices.Sort(p.codes)
	return p
}

// bendsSequence reads a part's bend sequence.
func bendsSequence(path string) ([]bendsStep, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var seq []json.RawMessage
	if json.Unmarshal(raw, &seq) != nil || len(seq) < 2 {
		return nil, false
	}
	steps := make([]bendsStep, len(seq)-1)
	for i, r := range seq[1:] {
		if json.Unmarshal(r, &steps[i]) != nil {
			return nil, false
		}
	}
	return steps, true
}

// bendsSimulate forms the unfolded part's bends in the dataset's order on its tooling. The sequence names bends by the faces the dataset built them on, which the model does not keep, so each step is matched to the unused bend closest in angle, radius and length.
func bendsSimulate(model *solid.Model, r *sheet.Result, steps []bendsStep) (int, bendConfusion, float64) {
	var conf bendConfusion
	lines := map[string][2]geometry.Point{}
	for _, n := range r.Part.NonCut {
		if n.Kind == geometry.NonCutBend && n.Line != nil {
			lines[n.EntityID] = *n.Line
		}
	}
	var bends []brake.Bend
	for _, b := range r.Bends {
		ln := lines[b.EntityID]
		bends = append(bends, brake.Bend{A: ln[0], B: ln[1], Angle: b.AngleDeg, InsideRadius: b.InsideRadiusMM, Up: b.Direction == constants.BendDirectionUp})
	}
	if len(bends) != len(steps) {
		return -1, conf, 0
	}
	used := make([]bool, len(bends))
	var order []int
	for _, st := range steps {
		best, cost := -1, math.Inf(1)
		for i, b := range bends {
			if used[i] {
				continue
			}
			c := math.Abs(b.Angle-st.Angle) + 10*math.Abs(b.InsideRadius-st.Radius) + 0.05*math.Abs(b.A.Dist(b.B)-st.Width)
			if c < cost {
				best, cost = i, c
			}
		}
		used[best] = true
		order = append(order, best)
	}
	m, err := brake.New(r.Part, bends, r.ThicknessMM, 0.44, 4)
	if err != nil {
		return -1, conf, 0
	}
	// The folded part, placed back in the model.
	var folded [][3]float64
	for _, q := range m.Folded(r.BasePoint) {
		w := r.Placement.At(q[0], q[1], q[2])
		folded = append(folded, [3]float64{w.X, w.Y, w.Z})
	}
	foldBad := foldDisagreement(model, folded, r.ThicknessMM/2+5)
	t := bendsTooling
	tool := brake.Tooling{PunchTipAngle: 90, PunchWidth: t.PunchWidth.MM(), PunchHeight: t.PunchHeight.MM(), PunchLength: t.PunchLength.MM(), DieOpening: t.DieOpening.Min.MM(), DieWidth: t.DieWidth.MM(), DieHeight: t.DieHeight.MM(), DieLength: t.DieLength.MM()}
	b2i := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	ours := 0
	for i, res := range m.Simulate(order, tool) {
		st := steps[i]
		conf.any[b2i(st.Punch || st.Die)][b2i(res.Collides())]++
		conf.punch[b2i(st.Punch)][b2i(res.Punch)]++
		conf.die[b2i(st.Die)][b2i(res.Die)]++
		if res.Collides() {
			ours = 1
		}
	}
	return ours, conf, foldBad
}

// foldDisagreement is the fraction of the model's edge points farther than tol from every folded sample.
func foldDisagreement(m *solid.Model, folded [][3]float64, tol float64) float64 {
	grid := map[[3]int][][3]float64{}
	key := func(p [3]float64) [3]int {
		return [3]int{int(math.Floor(p[0] / tol)), int(math.Floor(p[1] / tol)), int(math.Floor(p[2] / tol))}
	}
	for _, p := range folded {
		grid[key(p)] = append(grid[key(p)], p)
	}
	total, bad := 0, 0
	for _, b := range m.Bodies {
		for _, fc := range b.Faces {
			for _, l := range fc.Loops {
				for _, e := range l.Edges {
					for _, u := range []float64{0.1, 0.5, 0.9} {
						q := e.Point(u)
						p := [3]float64{q.X, q.Y, q.Z}
						total++
						k := key(p)
						near := false
						for dx := -1; dx <= 1 && !near; dx++ {
							for dy := -1; dy <= 1 && !near; dy++ {
								for dz := -1; dz <= 1 && !near; dz++ {
									for _, s := range grid[[3]int{k[0] + dx, k[1] + dy, k[2] + dz}] {
										if math.Hypot(math.Hypot(s[0]-p[0], s[1]-p[1]), s[2]-p[2]) <= tol {
											near = true
											break
										}
									}
								}
							}
						}
						if !near {
							bad++
						}
					}
				}
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(bad) / float64(total)
}

var refusalIDs = regexp.MustCompile(`#\d+|\d+(\.\d+)? mm|solid \d+ of \d+: `)

// refusal shortens a refusal to its kind.
func refusal(msg string) string {
	msg = refusalIDs.ReplaceAllString(msg, "")
	if i := strings.Index(msg, ";"); i > 0 {
		msg = msg[:i]
	}
	return strings.TrimSpace(msg)
}

// bendsProgress is one line of progress while the dataset runs: how far, how long, and the agreement so far.
func bendsProgress(done []bendsPart, total int, elapsed time.Duration) string {
	s := bendsSummarize(done)
	left := time.Duration(float64(elapsed) / float64(len(done)) * float64(total-len(done)))
	if left >= time.Minute {
		left = left.Round(time.Minute)
	} else {
		left = left.Round(time.Second)
	}
	agree := func(k string) any {
		if a, ok := s.Simulated[k].(map[string]any)["agreement"]; ok {
			return a
		}
		return "-"
	}
	return fmt.Sprintf("%d of %d parts in %s, about %s left: %d unfolded; tool collisions agree on %v of parts and %v of bends; %v folded parts differ from the model", len(done), total, elapsed.Round(time.Second), left, s.Unfolded, agree("parts"), agree("bends"), s.Folding["parts_differing_from_model"])
}

// bendsSummary is the record kept in the private corpus: rates, not parts.
type bendsSummary struct {
	Parts     int                 `json:"parts"`
	Unfolded  int                 `json:"unfolded"`
	Refused   map[string]int      `json:"refused"`
	Agreement map[string]any      `json:"read_against_labels"`
	ToolLabel map[string]int      `json:"tool_collision_label_vs_engine"`
	Simulated map[string]any      `json:"tool_collision_in_dataset_order"`
	Folding   map[string]any      `json:"folding_against_model"`
	FlatLabel map[string]int      `json:"unfolding_overlap_label_vs_flat_pattern_overlaps"`
	Verdicts  map[string]int      `json:"plan_verdicts"`
	Codes     map[string]int      `json:"parts_by_blocking_code"`
	Examples  map[string][]string `json:"examples"`
}

func bendsSummarize(parts []bendsPart) bendsSummary {
	s := bendsSummary{Parts: len(parts), Refused: map[string]int{}, ToolLabel: map[string]int{}, Simulated: map[string]any{}, FlatLabel: map[string]int{}, Verdicts: map[string]int{}, Codes: map[string]int{}, Examples: map[string][]string{}}
	example := func(kind, id string) {
		if len(s.Examples[kind]) < 10 {
			s.Examples[kind] = append(s.Examples[kind], id)
		}
	}
	var bendsSame, thickSame, radiusSame, angleSame int
	var partConf [2][2]int
	var bc bendConfusion
	var folded, foldedBadly int
	var areaErr []float64
	for _, p := range parts {
		if p.read != "" {
			s.Refused[p.read]++
			example("refused: "+p.read, p.id)
			continue
		}
		s.Unfolded++
		l := p.labels
		if p.bends == l.TotalBends {
			bendsSame++
		} else {
			example("bend count differs", p.id)
		}
		if math.Abs(p.thick-l.Thickness) <= 0.001 {
			thickSame++
		} else {
			example("thickness differs", p.id)
		}
		if p.bends > 0 && math.Abs(p.minR-l.MinBendRadius) <= 0.01 && math.Abs(p.maxR-l.MaxBendRadius) <= 0.01 {
			radiusSame++
		} else {
			example("radius differs", p.id)
		}
		if p.bends > 0 && math.Abs(p.minA-l.MinBendAngle) <= 0.01 && math.Abs(p.maxA-l.MaxBendAngle) <= 0.01 {
			angleSame++
		} else {
			example("angle differs", p.id)
		}
		if l.FlatArea > 0 {
			areaErr = append(areaErr, math.Abs(p.area-l.FlatArea)/l.FlatArea)
		}
		engine := "clear"
		switch {
		case p.tool == constants.SeverityError:
			engine = "collides"
		case p.tool == constants.SeverityReview:
			engine = "search stopped"
		}
		key := fmt.Sprintf("label %d, engine %s", l.ToolCollision, engine)
		s.ToolLabel[key]++
		if p.sim >= 0 {
			folded++
			if p.foldBad > 0.05 {
				foldedBadly++
				example("folded part differs from the model", p.id)
			}
			partConf[l.ToolCollision][p.sim]++
			for i := range 2 {
				for j := range 2 {
					bc.any[i][j] += p.bendC.any[i][j]
					bc.punch[i][j] += p.bendC.punch[i][j]
					bc.die[i][j] += p.bendC.die[i][j]
				}
			}
			if l.ToolCollision != p.sim {
				example(fmt.Sprintf("simulated %d, label %d", p.sim, l.ToolCollision), p.id)
			}
		}
		example("tool "+key, p.id)
		overlap := slices.Contains(p.codes, string(constants.FindingCodeFlatPatternOverlaps))
		key = fmt.Sprintf("label %d, found %v", l.UnfoldOverlap, overlap)
		s.FlatLabel[key]++
		example("unfolding "+key, p.id)
		s.Verdicts[string(p.plan)]++
		for _, c := range p.codes {
			s.Codes[c]++
		}
	}
	sort.Float64s(areaErr)
	pct := func(q float64) string {
		if len(areaErr) == 0 {
			return "-"
		}
		return fmt.Sprintf("%.4f%%", 100*areaErr[int(q*float64(len(areaErr)-1))])
	}
	rate := func(n int) string {
		if s.Unfolded == 0 {
			return "-"
		}
		return fmt.Sprintf("%d of %d", n, s.Unfolded)
	}
	s.Agreement = map[string]any{
		"bend_count":      rate(bendsSame),
		"thickness":       rate(thickSame),
		"bend_radius":     rate(radiusSame),
		"bend_angle":      rate(angleSame),
		"flat_area_error": map[string]string{"median": pct(0.5), "p95": pct(0.95), "max": pct(1)},
	}
	conf := func(c [2][2]int) map[string]any {
		n := c[0][0] + c[0][1] + c[1][0] + c[1][1]
		out := map[string]any{"label 0, simulated 0": c[0][0], "label 0, simulated 1": c[0][1], "label 1, simulated 0": c[1][0], "label 1, simulated 1": c[1][1]}
		if n > 0 {
			out["agreement"] = fmt.Sprintf("%.1f%%", 100*float64(c[0][0]+c[1][1])/float64(n))
		}
		return out
	}
	s.Simulated = map[string]any{"parts": conf(partConf), "bends": conf(bc.any), "bends_punch": conf(bc.punch), "bends_die": conf(bc.die)}
	s.Folding = map[string]any{"parts_folded": folded, "parts_differing_from_model": foldedBadly}
	return s
}
