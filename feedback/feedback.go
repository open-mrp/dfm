// Package feedback turns a report that a check result was wrong into an improvement: it replays a stored check on the current engine, builds a feedback fixture's expectation from what the report said, and checks feedback fixtures, whose partial expectations keep a fixed problem from coming back.
//
// It works on results in their JSON form (resource.Check), as the platform stores them. The report itself (who said what, on what evidence) is the platform's resource; this package only needs what it says about the result.
package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/field"
	"github.com/open-mrp/dfm/request"
	"github.com/open-mrp/dfm/resource"
)

// Occurrence is one occurrence of a finding in a result, with its issue's code and severity.
type Occurrence struct {
	Code     constants.FindingCode `json:"code"`
	Severity constants.Severity    `json:"severity"`
	resource.Occurrence
}

// Occurrences lists every occurrence in a result, drawing issues first, then each part's, once per occurrence ID.
func Occurrences(r resource.Check) []Occurrence {
	var out []Occurrence
	seen := map[string]bool{}
	add := func(issues *resource.List[resource.Issue]) {
		if issues == nil {
			return
		}
		for _, is := range issues.Data {
			if is.Occurrences == nil {
				continue
			}
			for _, o := range is.Occurrences.Data {
				if !seen[o.ID] {
					seen[o.ID] = true
					out = append(out, Occurrence{Code: is.Code, Severity: is.Severity, Occurrence: o})
				}
			}
		}
	}
	add(r.DrawingIssues)
	if r.Parts != nil {
		for _, p := range r.Parts.Data {
			add(p.Issues)
		}
	}
	return out
}

// DecodeExpectation decodes a feedback fixture's expect.json.
func DecodeExpectation(data []byte) (request.FeedbackExpectation, *apierror.Error) {
	var e request.FeedbackExpectation
	err := request.Decode(data, &e, "expectation")
	return e, err
}

// Change is an occurrence that differs between a stored result and a replay: added, removed, or changed in severity or measurement.
type Change struct {
	Change string      `json:"change"` // added, removed or changed
	Stored *Occurrence `json:"stored"`
	Now    *Occurrence `json:"now"`
}

// Diff compares a stored result with the current engine's result for the same check, occurrence by occurrence (occurrences are matched by their IDs, which are derived from their content).
type Diff struct {
	StoredEngineVersion string            `json:"stored_engine_version"`
	EngineVersion       string            `json:"engine_version"`
	StoredVerdict       constants.Verdict `json:"stored_verdict"`
	Verdict             constants.Verdict `json:"verdict"`
	Unchanged           int               `json:"unchanged"`
	Changes             []Change          `json:"changes"`
}

// Changed reports whether the result differs at all.
func (d Diff) Changed() bool {
	return len(d.Changes) > 0 || d.Verdict != d.StoredVerdict
}

// Replay re-runs a stored check on the current engine and compares the result with the stored one.
func Replay(ctx context.Context, req dfm.CheckRequest, stored resource.Check) (Diff, error) {
	res, err := dfm.Check(ctx, req)
	if err != nil {
		return Diff{}, err
	}
	now := resource.PresentCheck(res)
	return Compare(stored, *now), nil
}

// Compare compares two results occurrence by occurrence.
func Compare(stored, now resource.Check) Diff {
	d := Diff{
		StoredEngineVersion: stored.EngineVersion, EngineVersion: now.EngineVersion,
		StoredVerdict: stored.Verdict, Verdict: now.Verdict,
		Changes: []Change{},
	}
	before, after := Occurrences(stored), Occurrences(now)
	byID := map[string]*Occurrence{}
	for i := range after {
		byID[after[i].ID] = &after[i]
	}
	for i := range before {
		o := &before[i]
		n, ok := byID[o.ID]
		switch {
		case !ok:
			d.Changes = append(d.Changes, Change{Change: "removed", Stored: o})
		case n.Severity != o.Severity || quantity(n.Actual) != quantity(o.Actual):
			d.Changes = append(d.Changes, Change{Change: "changed", Stored: o, Now: n})
		default:
			d.Unchanged++
		}
	}
	inBefore := map[string]bool{}
	for _, o := range before {
		inBefore[o.ID] = true
	}
	for i := range after {
		if !inBefore[after[i].ID] {
			d.Changes = append(d.Changes, Change{Change: "added", Now: &after[i]})
		}
	}
	return d
}

func quantity(q *resource.Quantity) string {
	if q == nil {
		return ""
	}
	return q.Value + " " + string(q.Unit)
}

// Evaluate checks a result against a fixture's expectation and returns what does not hold, empty when everything does.
func Evaluate(e request.FeedbackExpectation, r resource.Check) []string {
	var problems []string
	if v, ok := e.Verdict.Value(); ok && v != r.Verdict {
		problems = append(problems, fmt.Sprintf("verdict is %s, want %s", r.Verdict, v))
	}
	occ := Occurrences(r)
	present, _ := e.Present.Value()
	for _, f := range present {
		if !slices.ContainsFunc(occ, func(o Occurrence) bool { return matches(f, o) }) {
			problems = append(problems, "missing "+describe(f))
		}
	}
	absent, _ := e.Absent.Value()
	for _, f := range absent {
		for _, o := range occ {
			if matches(f, o) {
				problems = append(problems, fmt.Sprintf("unexpected %s (occurrence %s)", describe(f), o.ID))
			}
		}
	}
	return problems
}

// matches reports whether occurrence o fits every field f gives.
func matches(f request.ExpectedFinding, o Occurrence) bool {
	if o.Code != f.Code {
		return false
	}
	if s, ok := f.Severity.Value(); ok && s != o.Severity {
		return false
	}
	if ids, ok := f.EntityIDs.Value(); ok {
		for _, id := range ids {
			if !slices.Contains(o.EntityIDs, id) {
				return false
			}
		}
	}
	if p, ok := f.Location.Value(); ok {
		if o.Location == nil {
			return false
		}
		px, _ := strconv.ParseFloat(p.X, 64)
		py, _ := strconv.ParseFloat(p.Y, 64)
		ox, _ := strconv.ParseFloat(o.Location.X, 64)
		oy, _ := strconv.ParseFloat(o.Location.Y, 64)
		if math.Hypot(px-ox, py-oy) > 0.01 {
			return false
		}
	}
	if q, ok := f.Actual.Value(); ok {
		if o.Actual == nil {
			return false
		}
		want, err1 := q.ToDomain("actual")
		got, err2 := request.QuantityInput{Value: o.Actual.Value, Unit: constants.LengthUnit(o.Actual.Unit)}.ToDomain("actual")
		if err1 != nil || err2 != nil || want != got {
			return false
		}
	}
	return true
}

func describe(f request.ExpectedFinding) string {
	b, _ := json.Marshal(f)
	return string(b)
}

// Correction says an occurrence of the stored result was reported with the wrong measurement, and what the actual value is, in the unit the occurrence reports.
type Correction struct {
	ID    string
	Value string
}

// Expectation builds a fixture expectation from what a report said about a stored result: occurrences that must be absent (false positives), occurrences that must be present with a corrected actual (wrong measurements), and findings that must be present (false negatives). An occurrence is found in the stored result by its ID, and described by its code and entities, or its location when it names no entity.
func Expectation(report, note string, absent []string, corrected []Correction, present []request.ExpectedFinding, stored resource.Check) (request.FeedbackExpectation, error) {
	e := request.FeedbackExpectation{Report: report, Note: note}
	find := func(id string) (Occurrence, error) {
		for _, o := range Occurrences(stored) {
			if o.ID == id {
				return o, nil
			}
		}
		return Occurrence{}, fmt.Errorf("occurrence %q is not in the stored result", id)
	}
	var out []request.ExpectedFinding
	for _, id := range absent {
		o, err := find(id)
		if err != nil {
			return e, err
		}
		out = append(out, expectedFrom(o))
	}
	if out != nil {
		e.Absent = field.Some(out)
	}
	out = nil
	for _, c := range corrected {
		o, err := find(c.ID)
		if err != nil {
			return e, err
		}
		if o.Actual == nil || !constants.LengthUnit(o.Actual.Unit).IsValid() {
			return e, fmt.Errorf("occurrence %q does not measure a length, so its actual cannot be corrected", c.ID)
		}
		f := expectedFrom(o)
		f.Actual = field.Some(request.QuantityInput{Value: c.Value, Unit: constants.LengthUnit(o.Actual.Unit)})
		out = append(out, f)
	}
	out = append(out, present...)
	if out != nil {
		e.Present = field.Some(out)
	}
	return e, nil
}

// expectedFrom describes an occurrence by its code and entities, or its location when it names no entity.
func expectedFrom(o Occurrence) request.ExpectedFinding {
	f := request.ExpectedFinding{Code: o.Code}
	if len(o.EntityIDs) > 0 {
		f.EntityIDs = field.Some(o.EntityIDs)
	} else if o.Location != nil {
		f.Location = field.Some(request.PointInput{X: o.Location.X, Y: o.Location.Y})
	}
	return f
}
