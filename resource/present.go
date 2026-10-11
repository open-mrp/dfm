package resource

import (
	"math"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
	"github.com/open-mrp/dfm/rules"
)

// PresentCheck converts a domain result to its resource: each part with every problem it has, and the problems that are in no part. Lengths are reported in millimeters.
func PresentCheck(r dfm.CheckResult) *Check {
	perPart := make([][]rules.Finding, len(r.Parts))
	var drawing []rules.Finding
	for _, f := range r.Findings {
		if len(f.Parts) == 0 {
			drawing = append(drawing, f)
		}
		for _, n := range f.Parts {
			perPart[n-1] = append(perPart[n-1], f)
		}
	}
	parts := make([]Part, 0, len(r.Parts))
	at := map[int]int{} // part number to its entry in parts
	for i, p := range r.Parts {
		in := PartInstance{Object: constants.ObjectTypeDFMPartInstance, Number: p.Number, EntityIDs: p.EntityIDs, Rotation: degrees(p.Placement.RotationDeg), Offset: point(p.Placement.Offset.X, p.Placement.Offset.Y)}
		if p.CopyOf > 0 {
			first := &parts[at[p.CopyOf]]
			first.CopyCount++
			first.Instances.Data = append(first.Instances.Data, in)
			continue
		}
		at[p.Number] = len(parts)
		parts = append(parts, Part{
			Object:    constants.ObjectTypeDFMPart,
			Number:    p.Number,
			CopyCount: 1,
			Instances: NewList([]PartInstance{in}, PageInfo{}),
			EntityIDs: p.EntityIDs,
			Sheet: &SheetPart{
				Outline:     outlineOf(p.Unclosed),
				Width:       quantity(dec(p.Width), ""),
				Height:      quantity(dec(p.Height), ""),
				Area:        quantity(areaDec(p.Area), constants.UnitSquareMillimeter),
				CutLength:   quantity(dec(p.CutLength), ""),
				PierceCount: p.Pierces,
				Holes:       holeGroups(p.Holes),
				Bends:       bends(p.Bends),
			},
			Verdict:      p.Verdict,
			ErrorCount:   p.ErrorCount,
			ReviewCount:  p.ReviewCount,
			WarningCount: p.WarningCount,
			Issues:       issues(perPart[i]),
		})
	}
	skipped := make([]SkippedCheck, 0, len(r.Skipped))
	for _, c := range r.Skipped {
		skipped = append(skipped, SkippedCheck{Object: constants.ObjectTypeDFMSkippedCheck, Code: c.Code, Limit: c.Limit})
	}
	return &Check{
		Object:        constants.ObjectTypeDFMCheck,
		Process:       r.Process,
		EngineVersion: r.EngineVersion,
		Verdict:       r.Verdict,
		ErrorCount:    r.ErrorCount,
		ReviewCount:   r.ReviewCount,
		WarningCount:  r.WarningCount,
		PartCount:     len(r.Parts),
		Parts:         NewList(parts, PageInfo{}),
		DrawingIssues: issues(drawing),
		SkippedChecks: NewList(skipped, PageInfo{}),
	}
}

// issues groups findings into issues: one per code, severity, message and limit, in the order each first occurs, each with its occurrences in order. The message is part of the key because some rules vary it by case, and an occurrence must sit under the sentence that describes it.
func issues(findings []rules.Finding) *List[Issue] {
	out := []Issue{}
	at := map[string]int{}
	for _, f := range findings {
		req := quantity(f.Required, f.Unit)
		key := string(f.Code) + "|" + string(f.Severity) + "|" + f.Message
		if req != nil {
			key += "|" + req.Value + string(req.Unit)
		}
		if f.RequiredBasis != nil {
			key += "|" + string(*f.RequiredBasis)
		}
		i, ok := at[key]
		if !ok {
			i = len(out)
			at[key] = i
			is := Issue{
				Object:        constants.ObjectTypeDFMIssue,
				Code:          f.Code,
				Severity:      f.Severity,
				Message:       f.Message,
				Required:      req,
				RequiredBasis: f.RequiredBasis,
				Occurrences:   NewList([]Occurrence{}, PageInfo{}),
			}
			if info, ok := rules.Info(f.Code); ok {
				is.Remediation = info.Remediation
			}
			out = append(out, is)
		}
		occ := Occurrence{
			Object:    constants.ObjectTypeDFMOccurrence,
			ID:        f.ID,
			EntityIDs: f.EntityIDs,
			Actual:    quantity(f.Actual, f.Unit),
			Span:      []Point{},
		}
		if occ.EntityIDs == nil {
			occ.EntityIDs = []string{}
		}
		if f.Fix != nil {
			fix := &Fix{Action: f.Fix.Action, EntityIDs: f.Fix.EntityIDs}
			if fix.EntityIDs == nil {
				fix.EntityIDs = []string{}
			}
			if f.Fix.Units != "" {
				u := f.Fix.Units
				fix.Units = &u
			}
			occ.Fix = fix
		}
		if f.Location != nil {
			occ.Location = &Point{
				X:    f.Location.X.String(),
				Y:    f.Location.Y.String(),
				Unit: constants.LengthUnitMillimeter,
			}
		}
		if f.Span != nil {
			for _, q := range f.Span {
				occ.Span = append(occ.Span, Point{X: q.X.String(), Y: q.Y.String(), Unit: constants.LengthUnitMillimeter})
			}
		}
		out[i].Occurrences.Data = append(out[i].Occurrences.Data, occ)
	}
	return NewList(out, PageInfo{})
}

// PresentPlan converts a plan result into its resource.
func PresentPlan(r dfm.PlanResult) *Plan {
	ops := make([]Operation, 0, len(r.Operations))
	for _, o := range r.Operations {
		op := Operation{Object: constants.ObjectTypeDFMOperation, Sequence: o.Sequence, Process: o.Process, Reason: o.Reason, Verdict: constants.OperationVerdictNotChecked}
		if o.Result != nil {
			op.Result = PresentCheck(*o.Result)
			op.Verdict = constants.OperationVerdict(o.Result.Verdict)
		}
		ops = append(ops, op)
	}
	return &Plan{Object: constants.ObjectTypeDFMPlan, EngineVersion: r.EngineVersion, Verdict: r.Verdict, Operations: NewList(ops, PageInfo{})}
}

// PresentCatalog presents every finding code of a process. PresentCodes lists every finding code of every process, each once, laser cutting's first.
func PresentCodes() *List[FindingCodeInfo] {
	var out []FindingCodeInfo
	seen := map[constants.FindingCode]bool{}
	for _, name := range constants.Process("").EnumValues() {
		for _, c := range PresentCatalog(constants.Process(name)).Data {
			if !seen[c.Code] {
				seen[c.Code] = true
				out = append(out, c)
			}
		}
	}
	return NewList(out, PageInfo{})
}

func PresentCatalog(p constants.Process) *List[FindingCodeInfo] {
	cat := rules.Catalog(p)
	out := make([]FindingCodeInfo, 0, len(cat))
	for _, c := range cat {
		info := FindingCodeInfo{
			Object:      constants.ObjectTypeDFMFindingCode,
			Code:        c.Code,
			Severities:  c.Severities,
			Scope:       c.Scope,
			Description: c.Description,
			Remediation: c.Remediation,
		}
		if c.Limit != "" {
			limit := c.Limit
			info.Limit = &limit
		}
		out = append(out, info)
	}
	return NewList(out, PageInfo{})
}

func bends(bs []dfm.Bend) *List[Bend] {
	out := make([]Bend, 0, len(bs))
	for _, b := range bs {
		angle, radius := b.Angle.Decimal(), b.InsideRadius
		out = append(out, Bend{
			Object:        constants.ObjectTypeDFMBend,
			EntityID:      b.EntityID,
			Length:        quantity(dec(b.Length), ""),
			Angle:         quantity(&angle, constants.UnitDegree),
			Direction:     b.Direction,
			InsideRadius:  quantity(&radius, ""),
			BendAllowance: quantity(dec(b.Allowance), ""),
			Assumed:       b.Assumed,
			Start:         point(b.Start.X, b.Start.Y),
			End:           point(b.End.X, b.End.Y),
		})
		if f := b.Forming; f != nil {
			bend := &out[len(out)-1]
			sequence, position := f.Sequence, f.PunchPosition
			bend.Sequence, bend.PunchPosition = &sequence, &position
			if f.ThroatSide != "" {
				side := f.ThroatSide
				bend.ThroatSide = &side
			}
		}
	}
	return NewList(out, PageInfo{})
}

func holeGroups(gs []dfm.HoleGroup) *List[HoleGroup] {
	out := make([]HoleGroup, 0, len(gs))
	for _, g := range gs {
		h := HoleGroup{
			Object:              constants.ObjectTypeDFMHoleGroup,
			Diameter:            quantity(dec(g.Diameter), ""),
			Count:               g.Count,
			EntityIDs:           g.EntityIDs,
			AvailableOperations: g.Available,
		}
		if h.EntityIDs == nil {
			h.EntityIDs = []string{}
		}
		if h.AvailableOperations == nil {
			h.AvailableOperations = []string{}
		}
		if g.Operation != "" {
			op := g.Operation
			h.Operation = &op
		}
		out = append(out, h)
	}
	return NewList(out, PageInfo{})
}

// degrees presents an angle rounded to a millionth of a degree.
func degrees(d float64) *Quantity {
	v := measure.Ratio(math.Round(d * 1e6)).Decimal()
	return quantity(&v, constants.UnitDegree)
}

// point presents a point in millimeters, rounded to the nanometer; real drawings are far inside the representable range.
func point(x, y float64) *Point {
	px, _ := measure.FromMM(x)
	py, _ := measure.FromMM(y)
	return &Point{X: px.String(), Y: py.String(), Unit: constants.LengthUnitMillimeter}
}

func dec(l measure.Length) *measure.Decimal {
	d := l.Decimal()
	return &d
}

func quantity(d *measure.Decimal, unit constants.Unit) *Quantity {
	if d == nil {
		return nil
	}
	if unit == "" {
		unit = constants.Unit(constants.LengthUnitMillimeter)
	}
	return &Quantity{Value: d.String(), Unit: unit}
}

func areaDec(a measure.Area) *measure.Decimal {
	d := a.Decimal()
	return &d
}

// outlineOf is the state of a part's outline.
func outlineOf(unclosed bool) constants.Outline {
	if unclosed {
		return constants.OutlineOpen
	}
	return constants.OutlineClosed
}
