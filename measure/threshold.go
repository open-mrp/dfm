package measure

import (
	"github.com/open-mrp/dfm/constants"
)

// Threshold is a minimum dimension defined as the largest of an absolute floor, a multiple of stock thickness and a multiple of kerf width, capped at Max when it is set (non-zero): e.g. the smaller of 2t and 3.175 mm.
type Threshold struct {
	Min            Length
	ThicknessRatio Ratio
	KerfRatio      Ratio
	Max            Length
}

// Requirement is a resolved threshold: the exact minimum in millimeters and which term set it.
type Requirement struct {
	Value Decimal
	Basis constants.ThresholdBasis
}

// Required evaluates the threshold for a stock thickness and kerf width. When terms tie, the earlier one (min, thickness_ratio, kerf_ratio) is reported as the basis.
func (t Threshold) Required(thickness, kerf Length) Requirement {
	req := Requirement{Value: t.Min.Decimal(), Basis: constants.ThresholdBasisMin}
	if v := t.ThicknessRatio.Times(thickness); v.Cmp(req.Value) > 0 {
		req = Requirement{Value: v, Basis: constants.ThresholdBasisThicknessRatio}
	}
	if v := t.KerfRatio.Times(kerf); v.Cmp(req.Value) > 0 {
		req = Requirement{Value: v, Basis: constants.ThresholdBasisKerfRatio}
	}
	if t.Max > 0 {
		if m := t.Max.Decimal(); m.Cmp(req.Value) < 0 {
			req = Requirement{Value: m, Basis: constants.ThresholdBasisMax}
		}
	}
	return req
}

// Below reports whether actual is strictly less than the requirement. A value exactly at the requirement passes.
func (r Requirement) Below(actual Length) bool { return actual.Decimal().Cmp(r.Value) < 0 }
