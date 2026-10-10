package feedback

import (
	"encoding/json"
	"github.com/open-mrp/dfm/request"
	"os"
	"strings"
	"testing"

	"github.com/open-mrp/dfm/resource"
)

func storedResult(t *testing.T) resource.Check {
	t.Helper()
	raw, err := os.ReadFile("testdata/text-plate-0.24.0.json")
	if err != nil {
		t.Fatal(err)
	}
	var r resource.Check
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// The text-plate fixture's expectation fails on the 0.24.0 result it came from, so it would catch the false positive coming back.
func TestEvaluateCatchesTheReportedProblem(t *testing.T) {
	raw, err := os.ReadFile("../fixtures/feedback/text-plate-v-spike/expect.json")
	if err != nil {
		t.Fatal(err)
	}
	exp, aerr := DecodeExpectation(raw)
	if aerr != nil {
		t.Fatal(aerr)
	}
	if problems := Evaluate(exp, storedResult(t)); len(problems) != 1 || !strings.Contains(problems[0], "unexpected") {
		t.Errorf("problems on the 0.24.0 result: %v", problems)
	}
}

func TestEvaluateMatching(t *testing.T) {
	r := storedResult(t)
	for _, tt := range []struct {
		name, expect string
		ok           bool
	}{
		{"code and entities", `{"report":"r","note":"n","present":[{"code":"feature_too_thin","entity_ids":["140"]}]}`, true},
		{"severity", `{"report":"r","note":"n","present":[{"code":"feature_too_thin","severity":"warning"}]}`, false},
		{"actual, exactly", `{"report":"r","note":"n","present":[{"code":"feature_too_thin","actual":{"value":"0.308604","unit":"mm"}}]}`, true},
		{"actual in other units", `{"report":"r","note":"n","present":[{"code":"feature_too_thin","actual":{"value":"0.0308604","unit":"cm"}}]}`, true},
		{"wrong actual", `{"report":"r","note":"n","present":[{"code":"feature_too_thin","actual":{"value":"0.3","unit":"mm"}}]}`, false},
		{"location within 0.01 mm", `{"report":"r","note":"n","present":[{"code":"feature_too_thin","location":{"x":"45.14","y":"17.64"}}]}`, true},
		{"location too far", `{"report":"r","note":"n","present":[{"code":"feature_too_thin","location":{"x":"45.2","y":"17.64"}}]}`, false},
		{"verdict", `{"report":"r","note":"n","verdict":"manufacturable"}`, false},
		{"absent code", `{"report":"r","note":"n","absent":[{"code":"hole_too_small"}]}`, true},
	} {
		exp, aerr := DecodeExpectation([]byte(tt.expect))
		if aerr != nil {
			t.Fatalf("%s: %v", tt.name, aerr)
		}
		if got := len(Evaluate(exp, r)) == 0; got != tt.ok {
			t.Errorf("%s: holds = %v, want %v (%v)", tt.name, got, tt.ok, Evaluate(exp, r))
		}
	}
}

func TestExpectationAndCompare(t *testing.T) {
	r := storedResult(t)
	if _, err := Expectation("x", "n", []string{"fnd_nope"}, nil, nil, r); err == nil {
		t.Error("a report naming an occurrence not in the result made a fixture")
	}
	occ := Occurrences(r)
	if len(occ) == 0 {
		t.Fatal("the stored result has no occurrences")
	}
	e, err := Expectation("x", "n", []string{occ[0].ID}, nil, []request.ExpectedFinding{{Code: "hole_too_small"}}, r)
	if err != nil {
		t.Fatal(err)
	}
	absent, _ := e.Absent.Value()
	present, _ := e.Present.Value()
	if e.Report != "x" || e.Note != "n" || len(absent) != 1 || absent[0].Code != occ[0].Code || len(present) != 1 || present[0].Code != "hole_too_small" {
		t.Errorf("expectation %+v", e)
	}
	if occ[0].Actual != nil {
		e, err = Expectation("x", "n", nil, []Correction{{ID: occ[0].ID, Value: "1.25"}}, nil, r)
		p, _ := e.Present.Value()
		if a, ok := p[0].Actual.Value(); err != nil || !ok || a.Value != "1.25" || string(a.Unit) != string(occ[0].Actual.Unit) {
			t.Errorf("corrected %+v %v", e, err)
		}
	}
	if d := Compare(r, r); d.Changed() || d.Unchanged != len(Occurrences(r)) {
		t.Errorf("a result compared with itself: %+v", d)
	}
}
