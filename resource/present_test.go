package resource

import (
	"testing"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
	"github.com/open-mrp/dfm/rules"
)

// TestQuantity checks the value and unit each kind of measurement presents as.
func TestQuantity(t *testing.T) {
	ratio := func(s string) *measure.Decimal { r, _ := measure.ParseRatio(s); d := r.Decimal(); return &d }
	mm := constants.Unit(constants.LengthUnitMillimeter)
	for _, tt := range []struct {
		d     *measure.Decimal
		unit  constants.Unit
		value string
		want  constants.Unit
	}{
		{dec(measure.MustFromMM(0.8)), "", "0.8", mm},
		{dec(measure.MustFromMM(25.4)), constants.UnitInch, "25.4", constants.UnitInch},
		{areaDec(6_250_000), constants.UnitSquareMillimeter, "6.25", constants.UnitSquareMillimeter},
		{ratio("55.25"), constants.UnitPercent, "55.25", constants.UnitPercent},
		{ratio("90"), constants.UnitDegree, "90", constants.UnitDegree},
		{ratio("36"), constants.UnitSegment, "36", constants.UnitSegment},
	} {
		q := quantity(tt.d, tt.unit)
		if q.Value != tt.value || q.Unit != tt.want {
			t.Errorf("%s %s: %+v", tt.value, tt.unit, q)
		}
	}
	if quantity(nil, "") != nil {
		t.Error("nil decimal presents as a quantity")
	}
}

// TestIssuesGroupByMessage checks that occurrences with the same code but a different message are separate issues, each under its own sentence, and that occurrences with the same message are grouped.
func TestIssuesGroupByMessage(t *testing.T) {
	f := func(msg, id string) rules.Finding {
		return rules.Finding{Code: constants.FindingCodeGeometryDuplicate, Severity: constants.SeverityWarning, Message: msg, EntityIDs: []string{id}}
	}
	got := issues([]rules.Finding{f("copy", "a"), f("beside", "b"), f("copy", "c")})
	if len(got.Data) != 2 || got.Data[0].Message != "copy" || len(got.Data[0].Occurrences.Data) != 2 || got.Data[1].Message != "beside" || len(got.Data[1].Occurrences.Data) != 1 {
		t.Errorf("issues %+v", got.Data)
	}
	if got.Data[0].Remediation == "" {
		t.Error("remediation not filled from the catalog")
	}
}
