package dfm_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/feedback"
	"github.com/open-mrp/dfm/resource"
)

// TestFeedbackFixtures checks every feedback fixture: a directory under fixtures/feedback holding a design (design.dxf or design.svg), its check request (request.json) and a partial expectation (expect.json), made from a report that a result was wrong. A fixed problem cannot come back.
func TestFeedbackFixtures(t *testing.T) {
	dirs, err := filepath.Glob("fixtures/feedback/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			designs, _ := filepath.Glob(filepath.Join(dir, "design.*"))
			if len(designs) != 1 {
				t.Fatalf("want one design file, found %v", designs)
			}
			design, err := os.ReadFile(designs[0])
			if err != nil {
				t.Fatal(err)
			}
			reqJSON, err := os.ReadFile(filepath.Join(dir, "request.json"))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(dir, "expect.json"))
			if err != nil {
				t.Fatal(err)
			}
			exp, aerr := feedback.DecodeExpectation(raw)
			if aerr != nil {
				t.Fatal(aerr)
			}
			req, aerr := dfm.RequestFromJSON(design, reqJSON)
			if aerr != nil {
				t.Fatal(aerr)
			}
			res, err := dfm.Check(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range feedback.Evaluate(exp, *resource.PresentCheck(res)) {
				t.Errorf("%s (report: %s)", p, exp.Report)
			}
		})
	}
}
