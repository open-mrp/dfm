package request

import (
	"fmt"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/field"
	"github.com/open-mrp/dfm/measure"
)

// BendingInput holds a press brake's limits.
type BendingInput struct {
	// Longest bend the brake forms (its usable bending length).
	MaxBendLength QuantityInput `json:"max_bend_length" validate:"required"`
	// Shortest bend the tooling holds (the narrowest punch and die segment); bends are not checked for it when omitted.
	MinBendLength field.Optional[QuantityInput] `json:"min_bend_length,omitzero"`
	// The brake's press force; each bend's air-bending force is checked against it when given.
	MaxForce field.Optional[ForceInput] `json:"max_force,omitzero"`
	// Inside radius the tooling forms, as max(min, thickness_ratio x thickness) capped at max (air bending forms about the thickness with a die opening of 8 x thickness); the stock thickness when omitted. Bends are made at this radius whatever the design gives.
	InsideRadius field.Optional[ThresholdInput] `json:"inside_radius,omitzero"`
	// Largest bend angle the tooling forms, from flat (90 for a right angle); angles are not checked when omitted.
	MaxAngle field.Optional[AngleInput] `json:"max_angle,omitzero"`
	// Smallest bend angle the brake holds repeatably, from flat (a shallow bend springs back unpredictably); not checked when omitted.
	MinAngle field.Optional[AngleInput] `json:"min_angle,omitzero"`
	// Where the neutral axis lies, as a fraction of the thickness from the inside of the bend (above 0, at most 0.5), as a decimal string; it sets each bend's allowance. 0.44 when omitted.
	KFactor field.Optional[string] `json:"k_factor,omitzero" format:"decimal"`
	// Limits of the bending checks; an omitted one is not checked.
	Limits field.Optional[BendingLimitsInput] `json:"limits,omitzero"`
	// Punch and die the bends are formed with. When given, the engine searches for a bend order in which no bend runs the part into either tool; when omitted, bends that form a channel are flagged for review.
	Tooling field.Optional[ToolingInput] `json:"tooling,omitzero"`
}

// ToolingInput is a press brake's punch and die.
type ToolingInput struct {
	// The punch (upper tool).
	Punch PunchInput `json:"punch" validate:"required"`
	// The die (lower tool).
	Die DieInput `json:"die" validate:"required"`
}

// PunchInput is a punch's cross-section and length.
type PunchInput struct {
	// Included angle of the punch's tip.
	TipAngle AngleInput `json:"tip_angle" validate:"required"`
	// Width of the punch's body.
	Width QuantityInput `json:"width" validate:"required"`
	// How far the punch reaches above its tip, up to the ram (a gooseneck punch is modeled as straight).
	Height QuantityInput `json:"height" validate:"required"`
	// Length of the punch along the bend; the part may be slid to put the bend at its start, middle or end. Omitted for segmented tooling, assembled to each bend's length.
	Length field.Optional[QuantityInput] `json:"length,omitzero"`
	// A gooseneck punch's throat: the body set back behind the tip, which lets a return flange taller than the web beside it clear the punch. Omitted for a straight punch.
	Gooseneck field.Optional[GooseneckInput] `json:"gooseneck,omitzero"`
}

// GooseneckInput is the throat of a gooseneck punch.
type GooseneckInput struct {
	// Height of the straight nose above the tip, before the throat begins.
	NoseHeight QuantityInput `json:"nose_height" validate:"required"`
	// How far the body is set back from the tip's face, on the side away from the flange being formed.
	ThroatDepth QuantityInput `json:"throat_depth" validate:"required"`
	// Height of the set-back part of the body, above the nose.
	ThroatHeight QuantityInput `json:"throat_height" validate:"required"`
}

// DieInput is a V-die's cross-section and length.
type DieInput struct {
	// Width of the V at the die's top face, as max(min, thickness_ratio x thickness) capped at max, since a shop picks the V for the stock (about 8 x thickness).
	Opening ThresholdInput `json:"opening" validate:"required"`
	// Included angle of the V; matched to each bend (180 degrees less the bend angle) when omitted.
	Angle field.Optional[AngleInput] `json:"angle,omitzero"`
	// Width of the die's top face beside the opening, on each side.
	Width QuantityInput `json:"width" validate:"required"`
	// Depth of the die below its top face.
	Height QuantityInput `json:"height" validate:"required"`
	// Length of the die along the bend, centered on it. Omitted for segmented tooling, assembled to each bend's length.
	Length field.Optional[QuantityInput] `json:"length,omitzero"`
}

func (t ToolingInput) toDomain(path string) (*capability.BrakeTooling, *apierror.Error) {
	out := &capability.BrakeTooling{}
	var err *apierror.Error
	if out.PunchTipAngle, err = t.Punch.TipAngle.toDegrees(path + ".punch.tip_angle"); err != nil {
		return nil, err
	}
	if a, ok := t.Die.Angle.Value(); ok {
		if out.DieAngle, err = a.toDegrees(path + ".die.angle"); err != nil {
			return nil, err
		}
	}
	if out.DieOpening, err = t.Die.Opening.toDomain(path + ".die.opening"); err != nil {
		return nil, err
	}
	type length struct {
		q    QuantityInput
		dst  *measure.Length
		name string
	}
	fields := []length{
		{t.Punch.Width, &out.PunchWidth, "punch.width"}, {t.Punch.Height, &out.PunchHeight, "punch.height"},
		{t.Die.Width, &out.DieWidth, "die.width"}, {t.Die.Height, &out.DieHeight, "die.height"},
	}
	if g, ok := t.Punch.Gooseneck.Value(); ok {
		fields = append(fields, length{g.NoseHeight, &out.PunchNose, "punch.gooseneck.nose_height"}, length{g.ThroatDepth, &out.PunchThroatDepth, "punch.gooseneck.throat_depth"}, length{g.ThroatHeight, &out.PunchThroatHeight, "punch.gooseneck.throat_height"})
	}
	for _, x := range fields {
		if *x.dst, err = x.q.ToDomain(path + "." + x.name); err != nil {
			return nil, err
		}
	}
	if q, ok := t.Punch.Length.Value(); ok {
		if out.PunchLength, err = q.ToDomain(path + ".punch.length"); err != nil {
			return nil, err
		}
	}
	if q, ok := t.Die.Length.Value(); ok {
		if out.DieLength, err = q.ToDomain(path + ".die.length"); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// BendingLimitsInput holds the bending checks' limits.
type BendingLimitsInput struct {
	// Shortest flange beside a bend: the material from the bend line to the part's edge, on either side (often about 3 x thickness plus the inside radius); not checked when omitted.
	FlangeLength field.Optional[BendLimitInput] `json:"flange_length,omitzero"`
	// Least distance from a bend line to a hole or cut-out, which the bend would distort (often 2 x thickness plus the inside radius); not checked when omitted.
	FeatureDistance field.Optional[BendLimitInput] `json:"feature_distance,omitzero"`
	// Narrowest bend relief: the gap a bend line ends at, measured along the line (often the thickness, at least about 0.8 mm); not checked when omitted.
	ReliefWidth field.Optional[BendLimitInput] `json:"relief_width,omitzero"`
	// Shallowest bend relief: how far the gap reaches past the bend line (often the thickness plus the inside radius); not checked when omitted.
	ReliefDepth field.Optional[BendLimitInput] `json:"relief_depth,omitzero"`
	// Tightest inside radius a design may be drawn for, as a model or a request's bends give it (often about 0.25 x thickness); not checked when omitted. offset does not apply.
	MinInsideRadius field.Optional[BendLimitInput] `json:"min_inside_radius,omitzero"`
}

// BendLimitInput is a bending limit: an error and/or a warning threshold, optionally measured from the inside radius the tooling forms.
type BendLimitInput struct {
	// Below this the bend cannot be made.
	Error field.Optional[ThresholdInput] `json:"error,omitzero"`
	// Below this the bend can be made with reduced quality.
	Warning field.Optional[ThresholdInput] `json:"warning,omitzero"`
	// What both thresholds are added to: none (the default) or inside_radius, the inside radius the tooling forms (e.g. 2 x thickness + radius), so the limit grows with the bend.
	Offset field.Optional[constants.BendLimitOffset] `json:"offset,omitzero"`
}

// MaterialBendingInput says a material can be bent, and in what thicknesses. A material without it is not bent.
type MaterialBendingInput struct {
	// Thickest stock bent in this material.
	MaxThickness QuantityInput `json:"max_thickness" validate:"required"`
	// Thinnest stock bent in this material; no lower bound when omitted.
	MinThickness field.Optional[QuantityInput] `json:"min_thickness,omitzero"`
	// Force to bend this material relative to mild steel, as a decimal string (about 1.5 for stainless, 0.55 for aluminum); 1 when omitted.
	ForceFactor field.Optional[string] `json:"force_factor,omitzero" format:"decimal"`
	// Overrides the brake's inside_radius for this material (e.g. a larger radius for alloys that crack).
	InsideRadius field.Optional[ThresholdInput] `json:"inside_radius,omitzero"`
	// Overrides the brake's max_angle for this material.
	MaxAngle field.Optional[AngleInput] `json:"max_angle,omitzero"`
	// Overrides the brake's min_angle for this material.
	MinAngle field.Optional[AngleInput] `json:"min_angle,omitzero"`
	// Overrides the brake's k_factor for this material.
	KFactor field.Optional[string] `json:"k_factor,omitzero" format:"decimal"`
	// Overrides the brake's limits of the same name for this material.
	Limits field.Optional[BendingLimitsInput] `json:"limits,omitzero"`
}

// BendAssignmentInput gives some bend lines of the design their angle, direction and inside radius. A bend line no assignment names is bent 90 degrees up at the brake's inside radius.
type BendAssignmentInput struct {
	// Bend lines, as reported in the result's bends.
	EntityIDs []string `json:"entity_ids" validate:"required" minItems:"1"`
	// Bend angle from flat (90 for a right angle, up to 180 for a flange folded back flat); 90 when omitted.
	Angle field.Optional[AngleInput] `json:"angle,omitzero"`
	// Way the flange turns: up (toward the viewer of the drawing) or down; up when omitted.
	Direction field.Optional[constants.BendDirection] `json:"direction,omitzero"`
	// Inside radius the design is drawn for; the brake's inside radius when omitted.
	InsideRadius field.Optional[QuantityInput] `json:"inside_radius,omitzero"`
}

// ForceInput is a force together with the unit it is expressed in.
type ForceInput struct {
	// Decimal value, as a string to preserve precision.
	Value string `json:"value" validate:"required" format:"decimal"`
	// Unit the value is expressed in.
	Unit constants.ForceUnit `json:"unit" validate:"required"`
}

func (b BendingInput) toDomain(path string) (*capability.Bending, *apierror.Error) {
	l, err := b.MaxBendLength.ToDomain(path + ".max_bend_length")
	if err != nil {
		return nil, err
	}
	var force measure.Ratio
	if f, ok := b.MaxForce.Value(); ok {
		r, perr := measure.ParseRatio(f.Value)
		if perr != nil || r <= 0 {
			return nil, apierror.ParameterInvalid(path+".max_force.value", "%s.max_force must be a positive decimal", path)
		}
		force = r // millionths of a kN
	}
	out := &capability.Bending{MaxBendLength: l, MaxForce: force}
	if q, ok := b.MinBendLength.Value(); ok {
		if out.MinBendLength, err = q.ToDomain(path + ".min_bend_length"); err != nil {
			return nil, err
		}
	}
	if out.InsideRadius, out.MaxAngle, out.KFactor, err = bendShape(b.InsideRadius, b.MaxAngle, b.KFactor, path); err != nil {
		return nil, err
	}
	if out.MinAngle, err = minAngle(b.MinAngle, path); err != nil {
		return nil, err
	}
	lim, _ := b.Limits.Value()
	if out.Limits, err = lim.toDomain(path + ".limits"); err != nil {
		return nil, err
	}
	if t, ok := b.Tooling.Value(); ok {
		if out.Tooling, err = t.toDomain(path + ".tooling"); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// minAngle converts a smallest angle.
func minAngle(in field.Optional[AngleInput], path string) (measure.Ratio, *apierror.Error) {
	a, ok := in.Value()
	if !ok {
		return 0, nil
	}
	return a.toDegrees(path + ".min_angle")
}

// bendShape converts an inside radius, a largest angle and a K-factor.
func bendShape(radius field.Optional[ThresholdInput], maxAngle field.Optional[AngleInput], kFactor field.Optional[string], path string) (*measure.Threshold, measure.Ratio, measure.Ratio, *apierror.Error) {
	var r *measure.Threshold
	if t, ok := radius.Value(); ok {
		conv, err := t.toDomain(path + ".inside_radius")
		if err != nil {
			return nil, 0, 0, err
		}
		r = &conv
	}
	var a measure.Ratio
	if in, ok := maxAngle.Value(); ok {
		deg, err := in.toDegrees(path + ".max_angle")
		if err != nil {
			return nil, 0, 0, err
		}
		if deg <= 0 {
			return nil, 0, 0, apierror.ParameterInvalid(path+".max_angle.value", "%s.max_angle must be greater than 0 and at most 180 degrees", path)
		}
		a = deg
	}
	var k measure.Ratio
	if s, ok := kFactor.Value(); ok {
		v, err := measure.ParseRatio(s)
		if err != nil || v <= 0 {
			return nil, 0, 0, apierror.ParameterInvalid(path+".k_factor", "%s.k_factor must be greater than 0 and at most 0.5", path)
		}
		k = v
	}
	return r, a, k, nil
}

// DecodeBendAssignments decodes a standalone list of bend assignments; params are reported under "bends" as in a full request.
func DecodeBendAssignments(data []byte) ([]capability.BendAssignment, *apierror.Error) {
	var in []BendAssignmentInput
	if err := Decode(data, &in, "bends"); err != nil {
		return nil, err
	}
	return BendAssignmentsToDomain(in)
}

// BendAssignmentsToDomain converts bend assignments.
func BendAssignmentsToDomain(in []BendAssignmentInput) ([]capability.BendAssignment, *apierror.Error) {
	var out []capability.BendAssignment
	for i, a := range in {
		path := fmt.Sprintf("bends[%d]", i)
		as := capability.BendAssignment{EntityIDs: a.EntityIDs}
		if ang, ok := a.Angle.Value(); ok {
			deg, err := ang.toDegrees(path + ".angle")
			if err != nil {
				return nil, err
			}
			if deg <= 0 {
				return nil, apierror.ParameterInvalid(path+".angle.value", "%s.angle must be greater than 0 and at most 180 degrees", path)
			}
			as.Angle = deg
		}
		as.Direction, _ = a.Direction.Value()
		if q, ok := a.InsideRadius.Value(); ok {
			r, err := q.ToDomain(path + ".inside_radius")
			if err != nil {
				return nil, err
			}
			if r <= 0 {
				return nil, apierror.ParameterInvalid(path+".inside_radius.value", "%s.inside_radius must be greater than 0", path)
			}
			as.InsideRadius = r
		}
		out = append(out, as)
	}
	if err := capability.ValidateBendAssignments(out); err != nil {
		return nil, err
	}
	return out, nil
}

func (l BendingLimitsInput) toDomain(path string) (capability.BendingLimits, *apierror.Error) {
	var out capability.BendingLimits
	for _, x := range []struct {
		name string
		in   field.Optional[BendLimitInput]
		dst  **capability.BendLimit
	}{{"flange_length", l.FlangeLength, &out.FlangeLength}, {"feature_distance", l.FeatureDistance, &out.FeatureDistance},
		{"relief_width", l.ReliefWidth, &out.ReliefWidth}, {"relief_depth", l.ReliefDepth, &out.ReliefDepth}, {"min_inside_radius", l.MinInsideRadius, &out.MinInsideRadius}} {
		in, ok := x.in.Value()
		if !ok {
			continue
		}
		lim, err := LimitInput{Error: in.Error, Warning: in.Warning}.toDomain(path + "." + x.name)
		if err != nil {
			return out, err
		}
		offset, _ := in.Offset.Value()
		*x.dst = &capability.BendLimit{Limit: lim, PlusInsideRadius: offset == constants.BendLimitOffsetInsideRadius}
	}
	return out, nil
}

func (m MaterialBendingInput) toDomain(path string) (*capability.MaterialBending, *apierror.Error) {
	thickest, err := m.MaxThickness.ToDomain(path + ".max_thickness")
	if err != nil {
		return nil, err
	}
	out := &capability.MaterialBending{MaxThickness: thickest}
	if s, ok := m.ForceFactor.Value(); ok {
		r, perr := measure.ParseRatio(s)
		if perr != nil || r <= 0 {
			return nil, apierror.ParameterInvalid(path+".force_factor", "%s.force_factor must be a positive decimal", path)
		}
		out.ForceFactor = r
	}
	if q, ok := m.MinThickness.Value(); ok {
		if out.MinThickness, err = q.ToDomain(path + ".min_thickness"); err != nil {
			return nil, err
		}
	}
	if out.MinAngle, err = minAngle(m.MinAngle, path); err != nil {
		return nil, err
	}
	if out.InsideRadius, out.MaxAngle, out.KFactor, err = bendShape(m.InsideRadius, m.MaxAngle, m.KFactor, path); err != nil {
		return nil, err
	}
	lim, _ := m.Limits.Value()
	if out.Overrides, err = lim.toDomain(path + ".limits"); err != nil {
		return nil, err
	}
	return out, nil
}
