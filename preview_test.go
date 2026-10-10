package dfm_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/constants"
)

// TestPreview checks the SVG preview is well-formed, the same every time, and marks every located finding with its severity and code.
func TestPreview(t *testing.T) {
	req := perfRequest(t, "fixtures/invalid/bracket.dxf", "examples/laser.json", "mild_steel", 1_500_000)
	svg, err := dfm.Preview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := dfm.Preview(context.Background(), req)
	if !bytes.Equal(svg, again) {
		t.Error("the preview differs between runs")
	}
	dec := xml.NewDecoder(bytes.NewReader(svg))
	for {
		if _, err := dec.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("not well-formed: %v", err)
		}
	}
	res, err := dfm.Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(svg)
	for _, f := range res.Findings {
		if f.Location == nil {
			continue
		}
		class := `class="finding warning"`
		if f.Severity == constants.SeverityError {
			class = `class="finding error"`
		}
		if !strings.Contains(doc, class) || !strings.Contains(doc, "<title>"+string(f.Code)+": ") {
			t.Errorf("no %s marker for %s", f.Severity, f.Code)
		}
	}
}
