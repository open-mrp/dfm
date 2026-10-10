package request

import (
	"fmt"
	"strings"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/field"
	"github.com/open-mrp/dfm/measure"
)

// DecodeCheckRequest decodes a full check request (all but the design file). The schema version is checked by Decode, from the field's const tag.
func DecodeCheckRequest(data []byte) (CheckRequest, *apierror.Error) {
	var r CheckRequest
	err := Decode(data, &r, "")
	return r, err
}

// DecodePlanRequest decodes a plan request.
func DecodePlanRequest(data []byte) (PlanRequest, *apierror.Error) {
	var r PlanRequest
	err := Decode(data, &r, "")
	return r, err
}

// DecodeCapability decodes a standalone capability document; params are reported under "capability" as in a full request.
func DecodeCapability(data []byte) (capability.Capability, *apierror.Error) {
	var c CapabilityInput
	if err := Decode(data, &c, "capability"); err != nil {
		return capability.Capability{}, err
	}
	return c.ToDomain("capability")
}

// ToDomain converts the length, checking it is a whole number of nanometers.
func (q QuantityInput) ToDomain(path string) (measure.Length, *apierror.Error) {
	l, err := measure.ParseLength(q.Value, q.Unit)
	if err != nil {
		return 0, apierror.ParameterInvalid(path+".value", "%s: %v", path, err)
	}
	return l, nil
}

// toDegrees converts the angle to millionths of a degree.
func (a AngleInput) toDegrees(path string) (measure.Ratio, *apierror.Error) {
	r, err := measure.ParseRatio(a.Value)
	if err != nil {
		return 0, apierror.ParameterInvalid(path+".value", "%s: %v", path, err)
	}
	return r, nil
}

// ToDomain converts the design options; data is the design file.
func (d DesignInput) ToDomain(data []byte) design.Design {
	units, _ := d.Units.Value()
	adj, _ := d.Adjustments.Value()
	return design.Design{Format: d.Format, Data: data, Units: units, Adjustments: AdjustmentsToDomain(adj)}
}

// AdjustmentsToDomain converts accepted fixes; Check validates them.
func AdjustmentsToDomain(in []AdjustmentInput) []design.Adjustment {
	var out []design.Adjustment
	for _, a := range in {
		ids, _ := a.EntityIDs.Value()
		units, _ := a.Units.Value()
		out = append(out, design.Adjustment{Action: a.Action, EntityIDs: ids, Units: units})
	}
	return out
}

// DecodeAdjustments decodes a JSON array of accepted fixes, as the CLI reads them from a file.
func DecodeAdjustments(data []byte) ([]design.Adjustment, *apierror.Error) {
	var in []AdjustmentInput
	if err := Decode(data, &in, "design.adjustments"); err != nil {
		return nil, err
	}
	return AdjustmentsToDomain(in), nil
}

// ToDomain converts the stock.
func (s StockInput) ToDomain() (capability.Stock, *apierror.Error) {
	form, ok := s.Form.Value()
	if !ok {
		form = constants.StockFormSheet
	}
	out := capability.Stock{MaterialCode: s.MaterialCode, Form: form}
	q, given := s.Thickness.Value()
	if !given {
		return capability.Stock{}, apierror.ParameterMissing("stock.thickness")
	}
	t, err := q.ToDomain("stock.thickness")
	if err != nil {
		return capability.Stock{}, err
	}
	out.Thickness = t
	return out, nil
}

// ToDomain converts and validates the capability. Its schema version was checked when it was decoded.
func (c CapabilityInput) ToDomain(path string) (capability.Capability, *apierror.Error) {
	out := capability.Capability{Process: c.Process}
	lc, hasLC := c.LaserCutting.Value()
	switch {
	case c.Process == constants.ProcessLaserCutting && !hasLC:
		return capability.Capability{}, apierror.ParameterMissing(path + ".laser_cutting")
	case c.Process != constants.ProcessLaserCutting && hasLC:
		return capability.Capability{}, apierror.ParameterInvalid(path+".laser_cutting", "laser_cutting is only allowed when process is laser_cutting")
	case hasLC:
		l, err := lc.toDomain(path + ".laser_cutting")
		if err != nil {
			return capability.Capability{}, err
		}
		out.LaserCutting = l
	}
	bd, hasBD := c.Bending.Value()
	switch {
	case c.Process == constants.ProcessBending && !hasBD:
		return capability.Capability{}, apierror.ParameterMissing(path + ".bending")
	case c.Process != constants.ProcessBending && hasBD:
		return capability.Capability{}, apierror.ParameterInvalid(path+".bending", "bending is only allowed when process is bending")
	case hasBD:
		b, err := bd.toDomain(path + ".bending")
		if err != nil {
			return capability.Capability{}, err
		}
		out.Bending = b
	}
	for i, m := range c.Materials {
		mp := fmt.Sprintf("%s.materials[%d]", path, i)
		mat := capability.Material{Code: m.Code}
		if mlc, ok := m.LaserCutting.Value(); ok {
			if c.Process != constants.ProcessLaserCutting {
				return capability.Capability{}, apierror.ParameterInvalid(mp+".laser_cutting", "laser_cutting is only allowed when process is laser_cutting")
			}
			conv, err := mlc.toDomain(mp + ".laser_cutting")
			if err != nil {
				return capability.Capability{}, err
			}
			mat.LaserCutting = conv
		}
		if mb, ok := m.Bending.Value(); ok {
			if c.Process != constants.ProcessBending {
				return capability.Capability{}, apierror.ParameterInvalid(mp+".bending", "bending is only allowed when process is bending")
			}
			conv, err := mb.toDomain(mp + ".bending")
			if err != nil {
				return capability.Capability{}, err
			}
			mat.Bending = conv
		}
		out.Materials = append(out.Materials, mat)
	}
	ops, _ := c.HoleOperations.Value()
	for i, h := range ops {
		op, err := h.toDomain(fmt.Sprintf("%s.hole_operations[%d]", path, i))
		if err != nil {
			return capability.Capability{}, err
		}
		out.HoleOperations = append(out.HoleOperations, op)
	}
	if err := capability.Validate(out); err != nil {
		// Validate reports under "capability"; a plan's capabilities are under capabilities[i].
		if rest, ok := strings.CutPrefix(err.Param, "capability"); ok {
			err.Param = path + rest
		}
		return capability.Capability{}, err
	}
	return out, nil
}

func (l LaserCuttingInput) toDomain(path string) (*capability.LaserCutting, *apierror.Error) {
	w, err := l.UsableArea.Width.ToDomain(path + ".usable_area.width")
	if err != nil {
		return nil, err
	}
	h, err := l.UsableArea.Height.ToDomain(path + ".usable_area.height")
	if err != nil {
		return nil, err
	}
	k, err := l.KerfWidth.ToDomain(path + ".kerf_width")
	if err != nil {
		return nil, err
	}
	hole, err := l.Limits.HoleDiameter.toDomain(path + ".limits.hole_diameter")
	if err != nil {
		return nil, err
	}
	slot, err := l.Limits.SlotWidth.toDomain(path + ".limits.slot_width")
	if err != nil {
		return nil, err
	}
	lim := l.Limits
	opt, err := optionalLimits(path+".limits",
		optionalInputs{web: lim.WebWidth, spacing: lim.FeatureSpacing, edge: lim.EdgeDistance, partWidth: lim.PartWidth, partLength: lim.PartLength, cutoutWidth: lim.CutoutWidth, featureWidth: lim.FeatureWidth, cornerRadius: lim.InternalCornerRadius, slotFit: lim.SlotFitClearance},
		extraInputs{area: lim.PartArea, slender: lim.SlenderPart, slenderCut: lim.SlenderCutout, percent: lim.MaxRemovedAreaPercent, window: lim.RemovedAreaWindow, tipAngle: lim.MinTipAngle})
	if err != nil {
		return nil, err
	}
	out := &capability.LaserCutting{
		UsableWidth: w, UsableHeight: h, KerfWidth: k,
		Limits: capability.LaserCuttingLimits{HoleDiameter: hole, SlotWidth: slot, Optional: opt},
	}
	if m, ok := l.MultipleParts.Value(); ok {
		out.MultipleParts = m
	}
	if f, ok := l.FilledShapes.Value(); ok {
		out.FilledShapes = f
	}
	if o, ok := l.OutsidePage.Value(); ok {
		out.OutsidePage = o
	}
	if c, ok := l.CommonLine.Value(); ok {
		out.CommonLine = c
	}
	out.DrawingUnits, _ = l.DrawingUnits.Value()
	colors, _ := l.ColorOperations.Value()
	for _, c := range colors {
		out.ColorOperations = append(out.ColorOperations, capability.ColorOperation{Color: c.Color, Operation: c.Operation})
	}
	if q, ok := l.ContourClosureTolerance.Value(); ok {
		t, err := q.ToDomain(path + ".contour_closure_tolerance")
		if err != nil {
			return nil, err
		}
		out.ContourClosureTolerance = &t
	}
	if q, ok := l.SupportSpacing.Value(); ok {
		if out.SupportSpacing, err = q.ToDomain(path + ".support_spacing"); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (m MaterialLaserCuttingInput) toDomain(path string) (*capability.MaterialLaserCutting, *apierror.Error) {
	thickest, err := m.MaxThickness.ToDomain(path + ".max_thickness")
	if err != nil {
		return nil, err
	}
	out := &capability.MaterialLaserCutting{MaxThickness: thickest}
	if q, ok := m.MinThickness.Value(); ok {
		if out.MinThickness, err = q.ToDomain(path + ".min_thickness"); err != nil {
			return nil, err
		}
	}
	stocked, _ := m.Thicknesses.Value()
	for i, q := range stocked {
		t, err := q.ToDomain(fmt.Sprintf("%s.thicknesses[%d]", path, i))
		if err != nil {
			return nil, err
		}
		out.Thicknesses = append(out.Thicknesses, t)
	}
	if out.KerfWidth, err = optionalLength(m.KerfWidth, path+".kerf_width"); err != nil {
		return nil, err
	}
	if out.UsableArea, err = optionalUsableArea(m.UsableArea, path+".usable_area"); err != nil {
		return nil, err
	}
	if out.Overrides, err = materialLimits(m.Limits, path+".limits"); err != nil {
		return nil, err
	}
	ranges, _ := m.ThicknessRanges.Value()
	for i, r := range ranges {
		rp := fmt.Sprintf("%s.thickness_ranges[%d]", path, i)
		var tr capability.ThicknessRange
		if tr.MaxThickness, err = r.MaxThickness.ToDomain(rp + ".max_thickness"); err != nil {
			return nil, err
		}
		if tr.KerfWidth, err = optionalLength(r.KerfWidth, rp+".kerf_width"); err != nil {
			return nil, err
		}
		if tr.UsableArea, err = optionalUsableArea(r.UsableArea, rp+".usable_area"); err != nil {
			return nil, err
		}
		if tr.Overrides, err = materialLimits(r.Limits, rp+".limits"); err != nil {
			return nil, err
		}
		out.ThicknessRanges = append(out.ThicknessRanges, tr)
	}
	return out, nil
}

func optionalUsableArea(a field.Optional[AreaInput], path string) (*capability.UsableArea, *apierror.Error) {
	v, ok := a.Value()
	if !ok {
		return nil, nil
	}
	w, err := v.Width.ToDomain(path + ".width")
	if err != nil {
		return nil, err
	}
	h, err := v.Height.ToDomain(path + ".height")
	if err != nil {
		return nil, err
	}
	return &capability.UsableArea{Width: w, Height: h}, nil
}

func optionalLength(q field.Optional[QuantityInput], path string) (*measure.Length, *apierror.Error) {
	v, ok := q.Value()
	if !ok {
		return nil, nil
	}
	l, err := v.ToDomain(path)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// materialLimits converts the limits a material or thickness range overrides.
func materialLimits(in field.Optional[MaterialLaserCuttingLimitsInput], path string) (capability.LimitOverrides, *apierror.Error) {
	var out capability.LimitOverrides
	limits, ok := in.Value()
	if !ok {
		return out, nil
	}
	if l, ok := limits.HoleDiameter.Value(); ok {
		conv, err := l.toDomain(path + ".hole_diameter")
		if err != nil {
			return out, err
		}
		out.HoleDiameter = &conv
	}
	if l, ok := limits.SlotWidth.Value(); ok {
		conv, err := l.toDomain(path + ".slot_width")
		if err != nil {
			return out, err
		}
		out.SlotWidth = &conv
	}
	opt, err := optionalLimits(path,
		optionalInputs{web: limits.WebWidth, spacing: limits.FeatureSpacing, edge: limits.EdgeDistance, partWidth: limits.PartWidth, partLength: limits.PartLength, cutoutWidth: limits.CutoutWidth, featureWidth: limits.FeatureWidth, cornerRadius: limits.InternalCornerRadius, slotFit: limits.SlotFitClearance},
		extraInputs{area: limits.PartArea, slender: limits.SlenderPart, slenderCut: limits.SlenderCutout, percent: limits.MaxRemovedAreaPercent, window: limits.RemovedAreaWindow, tipAngle: limits.MinTipAngle})
	if err != nil {
		return out, err
	}
	out.Optional = opt
	return out, nil
}

// optionalInputs lists the optional tiered limits in declaration order.
type optionalInputs struct {
	web, spacing, edge, partWidth, partLength, cutoutWidth, featureWidth, cornerRadius, slotFit field.Optional[LimitInput]
}

// extraInputs lists the optional limits that are not plain tiered lengths.
type extraInputs struct {
	area                field.Optional[AreaLimitInput]
	slender, slenderCut field.Optional[[]SlenderTierInput]
	percent             field.Optional[string]
	window              field.Optional[QuantityInput]
	tipAngle            field.Optional[AngleInput]
}

func optionalLimits(path string, in optionalInputs, x extraInputs) (capability.OptionalLaserCuttingLimits, *apierror.Error) {
	area, percent, tipAngle := x.area, x.percent, x.tipAngle
	var out capability.OptionalLaserCuttingLimits
	for _, l := range []struct {
		name string
		in   field.Optional[LimitInput]
		dst  **capability.Limit
	}{
		{"web_width", in.web, &out.WebWidth},
		{"feature_spacing", in.spacing, &out.FeatureSpacing},
		{"edge_distance", in.edge, &out.EdgeDistance},
		{"part_width", in.partWidth, &out.PartWidth},
		{"part_length", in.partLength, &out.PartLength},
		{"cutout_width", in.cutoutWidth, &out.CutoutWidth},
		{"feature_width", in.featureWidth, &out.FeatureWidth},
		{"internal_corner_radius", in.cornerRadius, &out.InternalCornerRadius},
		{"slot_fit_clearance", in.slotFit, &out.SlotFitClearance},
	} {
		in, ok := l.in.Value()
		if !ok {
			continue
		}
		conv, err := in.toDomain(path + "." + l.name)
		if err != nil {
			return out, err
		}
		*l.dst = &conv
	}
	if a, ok := area.Value(); ok {
		conv, err := a.toDomain(path + ".part_area")
		if err != nil {
			return out, err
		}
		out.PartArea = &conv
	}
	for _, set := range []struct {
		name string
		in   field.Optional[[]SlenderTierInput]
		dst  *[]capability.SlenderTier
	}{{"slender_part", x.slender, &out.SlenderPart}, {"slender_cutout", x.slenderCut, &out.SlenderCutout}} {
		tiers, _ := set.in.Value()
		for i, tier := range tiers {
			tp := fmt.Sprintf("%s.%s[%d]", path, set.name, i)
			l, err := tier.MinLength.ToDomain(tp + ".min_length")
			if err != nil {
				return out, err
			}
			w, err := tier.Width.toDomain(tp + ".width")
			if err != nil {
				return out, err
			}
			*set.dst = append(*set.dst, capability.SlenderTier{MinLength: l, Width: w})
		}
	}
	if q, ok := x.window.Value(); ok {
		w, err := q.ToDomain(path + ".removed_area_window")
		if err != nil {
			return out, err
		}
		out.RemovedAreaWindow = &w
	}
	if f, ok := percent.Value(); ok {
		r, err := measure.ParseRatio(f)
		if err != nil {
			return out, apierror.ParameterInvalid(path+".max_removed_area_percent", "%s.max_removed_area_percent: %v", path, err)
		}
		out.MaxRemovedAreaPercent = &r
	}
	if a, ok := tipAngle.Value(); ok {
		deg, err := a.toDegrees(path + ".min_tip_angle")
		if err != nil {
			return out, err
		}
		out.MinTipAngle = &deg
	}
	return out, nil
}

func (l AreaLimitInput) toDomain(path string) (capability.AreaLimit, *apierror.Error) {
	var out capability.AreaLimit
	for _, t := range []struct {
		name string
		in   field.Optional[AreaQuantityInput]
		dst  **measure.Area
	}{{"error", l.Error, &out.Error}, {"warning", l.Warning, &out.Warning}} {
		a, ok := t.in.Value()
		if !ok {
			continue
		}
		v, err := measure.ParseArea(a.Value, a.Unit)
		if err != nil {
			return capability.AreaLimit{}, apierror.ParameterInvalid(path+"."+t.name+".value", "%s.%s: %v", path, t.name, err)
		}
		*t.dst = &v
	}
	return out, nil
}

func (l LimitInput) toDomain(path string) (capability.Limit, *apierror.Error) {
	var out capability.Limit
	if t, ok := l.Error.Value(); ok {
		conv, err := t.toDomain(path + ".error")
		if err != nil {
			return capability.Limit{}, err
		}
		out.Error = &conv
	}
	if t, ok := l.Warning.Value(); ok {
		conv, err := t.toDomain(path + ".warning")
		if err != nil {
			return capability.Limit{}, err
		}
		out.Warning = &conv
	}
	return out, nil
}

func (t ThresholdInput) toDomain(path string) (measure.Threshold, *apierror.Error) {
	if t.Min.IsZero() && t.ThicknessRatio.IsZero() && t.KerfRatio.IsZero() {
		return measure.Threshold{}, apierror.ParameterInvalid(path, "%s needs at least one of min, thickness_ratio, kerf_ratio", path)
	}
	var out measure.Threshold
	if q, ok := t.Min.Value(); ok {
		l, err := q.ToDomain(path + ".min")
		if err != nil {
			return measure.Threshold{}, err
		}
		out.Min = l
	}
	if q, ok := t.Max.Value(); ok {
		l, err := q.ToDomain(path + ".max")
		if err != nil {
			return measure.Threshold{}, err
		}
		if l <= 0 || l < out.Min {
			return measure.Threshold{}, apierror.ParameterInvalid(path+".max", "%s.max must be greater than 0 and at least min", path)
		}
		out.Max = l
	}
	for _, r := range []struct {
		name string
		v    *string
		dst  *measure.Ratio
	}{
		{"thickness_ratio", t.ThicknessRatio.Ptr(), &out.ThicknessRatio},
		{"kerf_ratio", t.KerfRatio.Ptr(), &out.KerfRatio},
	} {
		if r.v == nil {
			continue
		}
		v, err := measure.ParseRatio(*r.v)
		if err != nil {
			return measure.Threshold{}, apierror.ParameterInvalid(path+"."+r.name, "%s.%s: %v", path, r.name, err)
		}
		*r.dst = v
	}
	return out, nil
}

// DecodeHoleAssignments decodes a standalone list of hole assignments; params are reported under "hole_operations" as in a full request.
func DecodeHoleAssignments(data []byte) ([]capability.HoleAssignment, *apierror.Error) {
	var in []HoleAssignmentInput
	if err := Decode(data, &in, "hole_operations"); err != nil {
		return nil, err
	}
	return HoleAssignmentsToDomain(in)
}

// HoleAssignmentsToDomain converts hole assignments.
func HoleAssignmentsToDomain(in []HoleAssignmentInput) ([]capability.HoleAssignment, *apierror.Error) {
	var out []capability.HoleAssignment
	for i, a := range in {
		path := fmt.Sprintf("hole_operations[%d]", i)
		as := capability.HoleAssignment{Operation: a.Operation}
		as.EntityIDs, _ = a.Holes.EntityIDs.Value()
		if q, ok := a.Holes.Diameter.Value(); ok {
			d, err := q.ToDomain(path + ".holes.diameter")
			if err != nil {
				return nil, err
			}
			if d <= 0 {
				return nil, apierror.ParameterInvalid(path+".holes.diameter", "%s.holes.diameter must be greater than 0", path)
			}
			as.Diameter = d
		}
		if n, ok := a.Holes.Part.Value(); ok {
			if n < 1 {
				return nil, apierror.ParameterInvalid(path+".holes.part", "%s.holes.part must be a part number, 1 or more", path)
			}
			as.Part = n
		}
		as.Side, _ = a.Side.Value()
		out = append(out, as)
	}
	return out, nil
}

func (h HoleOperationInput) toDomain(path string) (capability.HoleOperation, *apierror.Error) {
	out := capability.HoleOperation{Code: h.Code, Type: h.Type, Name: h.Name}
	var err *apierror.Error
	if out.HoleDiameter, err = h.HoleDiameter.ToDomain(path + ".hole_diameter"); err != nil {
		return out, err
	}
	if t, ok := h.Thread.Value(); ok {
		var th capability.Thread
		if th.NominalDiameter, err = t.NominalDiameter.ToDomain(path + ".thread.nominal_diameter"); err != nil {
			return out, err
		}
		if th.Pitch, err = t.Pitch.ToDomain(path + ".thread.pitch"); err != nil {
			return out, err
		}
		out.Thread = &th
	}
	if c, ok := h.Countersink.Value(); ok {
		var cs capability.Countersink
		if cs.AngleDeg, err = c.Angle.toDegrees(path + ".countersink.angle"); err != nil {
			return out, err
		}
		if cs.MajorDiameter, err = c.MajorDiameter.ToDomain(path + ".countersink.major_diameter"); err != nil {
			return out, err
		}
		r, perr := measure.ParseRatio(c.MaxDepthRatio)
		if perr != nil {
			return out, apierror.ParameterInvalid(path+".countersink.max_depth_ratio", "%s.countersink.max_depth_ratio: %v", path, perr)
		}
		cs.MaxDepthRatio = r
		out.Countersink = &cs
	}
	if hw, ok := h.Hardware.Value(); ok {
		out.Hardware = &capability.Hardware{Type: hw.Type}
	}
	for j, m := range h.Materials {
		mp := fmt.Sprintf("%s.materials[%d]", path, j)
		mat := capability.HoleOperationMaterial{MaterialCode: m.MaterialCode}
		if mat.MinThickness, err = m.MinThickness.ToDomain(mp + ".min_thickness"); err != nil {
			return out, err
		}
		if q, ok := m.MaxThickness.Value(); ok {
			l, err := q.ToDomain(mp + ".max_thickness")
			if err != nil {
				return out, err
			}
			mat.MaxThickness = &l
		}
		if l, ok := m.EdgeDistance.Value(); ok {
			lim, err := l.toDomain(mp + ".edge_distance")
			if err != nil {
				return out, err
			}
			mat.EdgeDistance = &lim
		}
		out.Materials = append(out.Materials, mat)
	}
	for _, lim := range []struct {
		in  field.Optional[LimitInput]
		dst **capability.Limit
		p   string
	}{{h.EdgeDistance, &out.EdgeDistance, ".edge_distance"}, {h.Spacing, &out.Spacing, ".spacing"}, {h.BendDistance, &out.BendDistance, ".bend_distance"}} {
		if l, ok := lim.in.Value(); ok {
			conv, err := l.toDomain(path + lim.p)
			if err != nil {
				return out, err
			}
			*lim.dst = &conv
		}
	}
	return out, nil
}
