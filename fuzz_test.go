package dfm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/request"
	"github.com/open-mrp/dfm/resource"
)

// FuzzCheck runs arbitrary DXF data through the whole pipeline (parse, normalize, every rule, exact conversions) and checks that it never panics and only fails with a structured *dfm.Error.
func FuzzCheck(f *testing.F) {
	paths, _ := filepath.Glob("fixtures/*/*.dxf")
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil && len(data) < 4096 {
			f.Add(data)
		}
	}
	// Extreme but in-range values: huge coordinates and bulges.
	f.Add([]byte("0\nSECTION\n2\nHEADER\n9\n$INSUNITS\n70\n6\n0\nENDSEC\n0\nSECTION\n2\nENTITIES\n" +
		"0\nLWPOLYLINE\n90\n2\n70\n1\n10\n-1e9\n20\n0\n42\n1e9\n10\n1e9\n20\n0\n42\n1e9\n" +
		"0\nCIRCLE\n10\n1e9\n20\n1e9\n40\n1e9\n0\nENDSEC\n0\nEOF\n"))
	data, err := os.ReadFile("examples/laser.json")
	if err != nil {
		f.Fatal(err)
	}
	c, aerr := request.DecodeCapability(data)
	if aerr != nil {
		f.Fatal(aerr)
	}
	f.Fuzz(func(t *testing.T, dxf []byte) {
		res, err := dfm.Check(context.Background(), dfm.CheckRequest{
			Design:     design.Design{Format: constants.DesignFormatDXF, Data: dxf},
			Capability: c,
			Stock:      capability.Stock{MaterialCode: "mild_steel", Thickness: 1_500_000},
		})
		var e *apierror.Error
		if err != nil && (!errors.As(err, &e) || e.Code == constants.ErrorCodeInternalError) {
			t.Fatalf("unstructured error: %v", err)
		}
		if err == nil {
			_ = resource.PresentCheck(res)
		}
	})
}
