package rules

import (
	"math"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// MaterialRule fails a job whose material is not listed in the capability. The remaining checks still run, using the capability's defaults.
type MaterialRule struct{}

func (MaterialRule) Code() constants.FindingCode { return constants.FindingCodeMaterialUnsupported }

func (MaterialRule) Check(_ *geometry.Part, p *capability.Profile) []Finding {
	if p.Supported {
		return nil
	}
	return []Finding{{
		Code:     constants.FindingCodeMaterialUnsupported,
		Severity: constants.SeverityError,
		Message:  "Material is not supported by the capability",
	}}
}

// ThicknessRule fails laser-cutting stock thicker than the material's maximum. Stock exactly at the maximum passes.
type ThicknessRule struct{}

func (ThicknessRule) Code() constants.FindingCode { return constants.FindingCodeMaterialTooThick }

func (ThicknessRule) Check(_ *geometry.Part, p *capability.Profile) []Finding {
	_, upper := thicknessRange(p)
	if !p.Supported || p.Thickness <= upper {
		return nil
	}
	return []Finding{{
		Code:     constants.FindingCodeMaterialTooThick,
		Severity: constants.SeverityError,
		Message:  "Stock thickness exceeds the maximum for this material",
		Actual:   dec(p.Thickness),
		Required: dec(upper),
	}}
}

// thicknessRange is the material's thickness range for the profile's process.
func thicknessRange(p *capability.Profile) (lower, upper measure.Length) {
	switch {
	case p.LaserCutting != nil:
		return p.LaserCutting.MinThickness, p.LaserCutting.MaxThickness
	case p.Bending != nil:
		return p.Bending.MinThickness, p.Bending.MaxThickness
	}
	return 0, 0
}

// stockedTolerance is how close a job's thickness must be to a stocked one: the same gauge written in inches or millimeters differs by rounding.
const stockedTolerance = measure.Length(1_000) // 0.001 mm

// MaterialTooThinRule fails laser-cutting stock thinner than the material's minimum. Stock exactly at the minimum passes.
type MaterialTooThinRule struct{}

func (MaterialTooThinRule) Code() constants.FindingCode { return constants.FindingCodeMaterialTooThin }

func (MaterialTooThinRule) Check(_ *geometry.Part, p *capability.Profile) []Finding {
	lower, _ := thicknessRange(p)
	if !p.Supported || p.Thickness >= lower {
		return nil
	}
	return []Finding{{
		Code:     constants.FindingCodeMaterialTooThin,
		Severity: constants.SeverityError,
		Message:  "Stock thickness is below the minimum for this material",
		Actual:   dec(p.Thickness),
		Required: dec(lower),
	}}
}

// ThicknessStockedRule fails a thickness that is not one the material lists as stocked (within stockedTolerance). Thicknesses outside the material's range are left to the thickness rules.
type ThicknessStockedRule struct{}

func (ThicknessStockedRule) Code() constants.FindingCode {
	return constants.FindingCodeMaterialThicknessNotStocked
}

func (ThicknessStockedRule) Check(_ *geometry.Part, p *capability.Profile) []Finding {
	lc := p.LaserCutting
	if !p.Supported || len(lc.Thicknesses) == 0 || p.Thickness < lc.MinThickness || p.Thickness > lc.MaxThickness {
		return nil
	}
	for _, t := range lc.Thicknesses {
		if d := p.Thickness - t; d >= -stockedTolerance && d <= stockedTolerance {
			return nil
		}
	}
	return []Finding{{
		Code:     constants.FindingCodeMaterialThicknessNotStocked,
		Severity: constants.SeverityError,
		Message:  "Stock thickness is not one stocked for this material",
		Actual:   dec(p.Thickness),
	}}
}

// ModelThicknessRule fails a 3D model whose sheet is not the stock's thickness, by more than max(0.05 mm, 3%): the part would be cut from sheet of another thickness than it was designed in.
type ModelThicknessRule struct{}

func (ModelThicknessRule) Code() constants.FindingCode {
	return constants.FindingCodeModelThicknessMismatch
}

func (ModelThicknessRule) Check(part *geometry.Part, p *capability.Profile) []Finding {
	if part.ThicknessMM == 0 {
		return nil
	}
	stock := p.Thickness.MM()
	if math.Abs(part.ThicknessMM-stock) <= max(0.05, 0.03*stock) {
		return nil
	}
	model, err := measure.FromMM(part.ThicknessMM)
	if err != nil {
		return nil
	}
	return []Finding{{
		Code:     constants.FindingCodeModelThicknessMismatch,
		Severity: constants.SeverityError,
		Message:  "The model's sheet is not the stock's thickness",
		Actual:   dec(model),
		Required: dec(p.Thickness),
	}}
}
