package dfm

import (
	"fmt"
	"math"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
	"github.com/open-mrp/dfm/sheet"
)

// modelBend is how a 3D model bends one of the flat part's bend lines.
type modelBend struct {
	Angle        measure.Ratio
	Direction    constants.BendDirection
	InsideRadius measure.Decimal
}

// readPart reads the design into the flat part the checks see: a drawing as drawn, or a 3D model unfolded as sheet metal, with how the model bends each bend line.
func readPart(d design.Design) (*geometry.Part, map[string]modelBend, error) {
	if d.Format != constants.DesignFormatSTEP {
		p, err := design.Parse(d)
		return p, nil, err
	}
	m, err := design.ParseModel(d)
	if err != nil {
		return nil, nil, err
	}
	k := d.KFactor
	if k == 0 {
		k = capability.DefaultKFactor
	}
	r, err := sheet.Flatten(m, sheet.Options{KFactor: float64(k) / 1e6, ClosureToleranceMM: d.ClosureTolerance.MM()})
	if err != nil {
		return nil, nil, err
	}
	bends := map[string]modelBend{}
	for _, b := range r.Bends {
		radius, err := measure.FromMM(b.InsideRadiusMM)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: bend %s has an inside radius that is not a finite number", sheet.ErrNotSheetMetal, b.EntityID)
		}
		bends[b.EntityID] = modelBend{Angle: measure.Ratio(math.Round(b.AngleDeg * 1e6)), Direction: b.Direction, InsideRadius: radius.Decimal()}
	}
	return r.Part, bends, nil
}
