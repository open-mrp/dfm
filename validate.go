package dfm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/sheet"
)

func validateDesign(d design.Design) *apierror.Error {
	if !d.Format.IsValid() {
		return apierror.New(constants.ErrorCodeDesignFormatUnsupported, "design.format", "design format %q is not one of %s", d.Format, strings.Join(d.Format.EnumValues(), ", "))
	}
	if d.Units != "" && !d.Units.IsValid() {
		return apierror.ParameterInvalid("design.units", "unit %q is not one of %s", d.Units, strings.Join(d.Units.EnumValues(), ", "))
	}
	return validateAdjustments(d)
}

// validateAdjustments checks that each adjustment says what it needs: entities for mark_bend_line and remove, units for set_units (once, and not against design.units), and no entity in two adjustments.
func validateAdjustments(d design.Design) *apierror.Error {
	owner := map[string]int{}
	units := -1
	for i, a := range d.Adjustments {
		path := fmt.Sprintf("design.adjustments[%d]", i)
		switch a.Action {
		case constants.FixActionMarkBendLine, constants.FixActionRemove:
			if len(a.EntityIDs) == 0 {
				return apierror.ParameterInvalid(path+".entity_ids", "%s.entity_ids is required for %s", path, a.Action)
			}
			if a.Units != "" {
				return apierror.ParameterInvalid(path+".units", "%s.units is only for set_units", path)
			}
		case constants.FixActionSetUnits:
			if !a.Units.IsValid() {
				return apierror.ParameterInvalid(path+".units", "%s.units must be one of %s", path, strings.Join(a.Units.EnumValues(), ", "))
			}
			if len(a.EntityIDs) > 0 {
				return apierror.ParameterInvalid(path+".entity_ids", "%s.entity_ids is not used by set_units", path)
			}
			if units >= 0 {
				return apierror.ParameterInvalid(path, "%s sets the units again; design.adjustments[%d] already does", path, units)
			}
			if d.Units != "" && d.Units != a.Units {
				return apierror.ParameterInvalid(path+".units", "%s.units contradicts design.units", path)
			}
			units = i
		default:
			return apierror.ParameterInvalid(path+".action", "%s.action %q is not one of %s", path, a.Action, strings.Join(a.Action.EnumValues(), ", "))
		}
		for j, id := range a.EntityIDs {
			if k, dup := owner[id]; dup {
				return apierror.ParameterInvalid(fmt.Sprintf("%s.entity_ids[%d]", path, j), "entity %q is already in design.adjustments[%d]", id, k)
			}
			owner[id] = i
		}
	}
	return nil
}

// designError maps parser failures to API errors; the message is the parser's, which names the offending line or entity.
func designError(err error) *apierror.Error {
	var adj *design.AdjustmentError
	switch {
	case errors.As(err, &adj):
		return apierror.ParameterInvalid(fmt.Sprintf("design.adjustments[%d].entity_ids", adj.Index), "design.adjustments[%d]: %s", adj.Index, adj)
	case errors.Is(err, design.ErrUnitsUnknown):
		e := apierror.New(constants.ErrorCodeDesignUnitsUnknown, "design.units", "%s", err)
		e.Internal = err
		return e
	case errors.Is(err, design.ErrUnsupportedFormat):
		return apierror.New(constants.ErrorCodeDesignFormatUnsupported, "design.format", "%s", err)
	case errors.Is(err, design.ErrUnsupportedContent), errors.Is(err, sheet.ErrNotSheetMetal):
		return apierror.New(constants.ErrorCodeDesignContentUnsupported, "design", "%s", err)
	case errors.Is(err, design.ErrInvalid):
		return apierror.New(constants.ErrorCodeDesignInvalid, "design", "%s", err)
	default:
		return apierror.Internal(err)
	}
}

// AsError converts any error from Check into an *apierror.Error.
func AsError(err error) *apierror.Error {
	var e *apierror.Error
	if errors.As(err, &e) {
		return e
	}
	return apierror.Internal(err)
}
