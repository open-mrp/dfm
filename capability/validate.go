package capability

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
)

// Param paths below match the JSON request (see package request), so a library caller and an HTTP caller see the same param for the same problem.
const root = "capability"

var materialCode = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

// Validate checks a capability and returns the first problem found.
func Validate(c Capability) *apierror.Error {
	if !c.Process.IsValid() {
		return apierror.ParameterInvalid(root+".process", "process %q is not one of %s", c.Process, strings.Join(c.Process.EnumValues(), ", "))
	}
	if !c.Process.HasCapability() {
		return apierror.ParameterInvalid(root+".process", "process %q has no capability of its own: a laser_cutting capability's hole_operations offers it", c.Process)
	}
	switch c.Process {
	case constants.ProcessLaserCutting:
		if err := validateLaserCutting(c.LaserCutting); err != nil {
			return err
		}
	case constants.ProcessBending:
		if err := validateBending(c.Bending); err != nil {
			return err
		}
	}
	if len(c.Materials) == 0 {
		return apierror.ParameterInvalid(root+".materials", "materials must list at least one material")
	}
	seen := map[string]bool{}
	for i, m := range c.Materials {
		path := fmt.Sprintf("%s.materials[%d]", root, i)
		if !materialCode.MatchString(m.Code) {
			return apierror.ParameterInvalid(path+".code", "code %q must be 1-64 lowercase letters, digits, '_', '.' or '-'", m.Code)
		}
		if seen[m.Code] {
			return apierror.ParameterInvalid(path+".code", "code %q is listed more than once", m.Code)
		}
		seen[m.Code] = true
		switch c.Process {
		case constants.ProcessLaserCutting:
			if err := validateMaterialLaserCutting(m.LaserCutting, path+".laser_cutting"); err != nil {
				return err
			}
		case constants.ProcessBending:
			if err := validateMaterialBending(m.Bending, path+".bending"); err != nil {
				return err
			}
		}
	}
	return validateHoleOperations(c)
}

func validateLaserCutting(l *LaserCutting) *apierror.Error {
	path := root + ".laser_cutting"
	if l == nil {
		return apierror.ParameterMissing(path)
	}
	if err := positive(l.UsableWidth, path+".usable_area.width"); err != nil {
		return err
	}
	if err := positive(l.UsableHeight, path+".usable_area.height"); err != nil {
		return err
	}
	if err := nonNegative(l.KerfWidth, path+".kerf_width"); err != nil {
		return err
	}
	if t := l.ContourClosureTolerance; t != nil {
		param := path + ".contour_closure_tolerance"
		if err := positive(*t, param); err != nil {
			return err
		}
		// A larger tolerance would join ends that are meant to be apart.
		if *t > maxContourClosureTolerance {
			return apierror.ParameterInvalid(param, "%s must be at most 1 mm", param)
		}
	}
	if err := nonNegative(l.SupportSpacing, path+".support_spacing"); err != nil {
		return err
	}
	if m := l.MultipleParts; m != "" && !m.IsValid() {
		return apierror.ParameterInvalid(path+".multiple_parts", "%s.multiple_parts must be one of %s", path, strings.Join(m.EnumValues(), ", "))
	}
	seen := map[string]bool{}
	for i, c := range l.ColorOperations {
		p := fmt.Sprintf("%s.color_operations[%d]", path, i)
		if !hexColor.MatchString(c.Color) {
			return apierror.ParameterInvalid(p+".color", "%s.color must be a color written #rrggbb", p)
		}
		if seen[strings.ToLower(c.Color)] {
			return apierror.ParameterInvalid(p+".color", "%s.color %s is listed twice", p, c.Color)
		}
		seen[strings.ToLower(c.Color)] = true
		if !c.Operation.IsValid() {
			return apierror.ParameterInvalid(p+".operation", "%s.operation must be one of %s", p, strings.Join(c.Operation.EnumValues(), ", "))
		}
	}
	for i, u := range l.DrawingUnits {
		if !u.IsValid() {
			return apierror.ParameterInvalid(fmt.Sprintf("%s.drawing_units[%d]", path, i), "unit %q is not one of %s", u, strings.Join(u.EnumValues(), ", "))
		}
	}
	if c := l.CommonLine; c != "" && !c.IsValid() {
		return apierror.ParameterInvalid(path+".common_line", "%s.common_line must be one of %s", path, strings.Join(c.EnumValues(), ", "))
	}
	if o := l.OutsidePage; o != "" && !o.IsValid() {
		return apierror.ParameterInvalid(path+".outside_page", "%s.outside_page must be one of %s", path, strings.Join(o.EnumValues(), ", "))
	}
	if o := l.FilledShapes; o != "" && !o.IsValid() {
		return apierror.ParameterInvalid(path+".filled_shapes", "%s.filled_shapes must be one of %s", path, strings.Join(o.EnumValues(), ", "))
	}
	if err := validateLimit(&l.Limits.HoleDiameter, path+".limits.hole_diameter"); err != nil {
		return err
	}
	if err := validateLimit(&l.Limits.SlotWidth, path+".limits.slot_width"); err != nil {
		return err
	}
	return validateOptional(l.Limits.Optional, path+".limits")
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// maxContourClosureTolerance bounds the gap a capability may close.
const maxContourClosureTolerance = measure.Millimeter

func validateOptional(o OptionalLaserCuttingLimits, path string) *apierror.Error {
	// In field order, so the first problem reported is the same every time.
	for _, x := range []struct {
		name string
		lim  *Limit
	}{
		{"web_width", o.WebWidth}, {"feature_spacing", o.FeatureSpacing}, {"edge_distance", o.EdgeDistance},
		{"part_width", o.PartWidth}, {"part_length", o.PartLength}, {"cutout_width", o.CutoutWidth},
		{"feature_width", o.FeatureWidth}, {"internal_corner_radius", o.InternalCornerRadius}, {"slot_fit_clearance", o.SlotFitClearance},
	} {
		if err := validateLimit(x.lim, path+"."+x.name); err != nil {
			return err
		}
	}
	if a := o.PartArea; a != nil {
		if a.Error == nil && a.Warning == nil {
			return apierror.ParameterInvalid(path+".part_area", "%s.part_area needs an error and/or a warning threshold", path)
		}
		for _, tier := range []struct {
			name string
			v    *measure.Area
		}{{"error", a.Error}, {"warning", a.Warning}} {
			if tier.v != nil && *tier.v <= 0 {
				return apierror.ParameterInvalid(path+".part_area."+tier.name, "%s.part_area.%s must be greater than 0", path, tier.name)
			}
		}
	}
	for _, set := range []struct {
		name  string
		tiers []SlenderTier
	}{{"slender_part", o.SlenderPart}, {"slender_cutout", o.SlenderCutout}} {
		name, tiers := set.name, set.tiers
		for i, tier := range tiers {
			tp := fmt.Sprintf("%s.%s[%d]", path, name, i)
			if err := positive(tier.MinLength, tp+".min_length"); err != nil {
				return err
			}
			if i > 0 && tier.MinLength <= tiers[i-1].MinLength {
				return apierror.ParameterInvalid(tp+".min_length", "%s.min_length must be greater than the previous tier's: list tiers by increasing length", tp)
			}
			w := tier.Width
			if err := validateLimit(&w, tp+".width"); err != nil {
				return err
			}
		}
	}
	if w := o.RemovedAreaWindow; w != nil {
		if err := positive(*w, path+".removed_area_window"); err != nil {
			return err
		}
		if o.MaxRemovedAreaPercent == nil {
			return apierror.ParameterInvalid(path+".removed_area_window", "%s.removed_area_window needs max_removed_area_percent", path)
		}
	}
	if r := o.MaxRemovedAreaPercent; r != nil && (*r <= 0 || *r > 100_000_000) {
		return apierror.ParameterInvalid(path+".max_removed_area_percent", "max_removed_area_percent must be greater than 0 and at most 100")
	}
	if a := o.MinTipAngle; a != nil && (*a <= 0 || *a >= 180_000_000) {
		return apierror.ParameterInvalid(path+".min_tip_angle", "min_tip_angle must be greater than 0° and less than 180°")
	}
	return nil
}

func validateMaterialLaserCutting(m *MaterialLaserCutting, path string) *apierror.Error {
	if m == nil {
		return apierror.ParameterMissing(path)
	}
	if err := positive(m.MaxThickness, path+".max_thickness"); err != nil {
		return err
	}
	if err := nonNegative(m.MinThickness, path+".min_thickness"); err != nil {
		return err
	}
	if m.MinThickness > m.MaxThickness {
		return apierror.ParameterInvalid(path+".min_thickness", "%s.min_thickness must be at most max_thickness", path)
	}
	for i, t := range m.Thicknesses {
		tp := fmt.Sprintf("%s.thicknesses[%d]", path, i)
		if t < m.MinThickness || t > m.MaxThickness || t <= 0 {
			return apierror.ParameterInvalid(tp, "%s must be greater than 0 and between min_thickness and max_thickness", tp)
		}
		if i > 0 && t <= m.Thicknesses[i-1] {
			return apierror.ParameterInvalid(tp, "%s must be greater than the previous thickness: list them increasing", tp)
		}
	}
	if err := validateOverrides(m.KerfWidth, m.UsableArea, m.Overrides, path); err != nil {
		return err
	}
	for i, r := range m.ThicknessRanges {
		rp := fmt.Sprintf("%s.thickness_ranges[%d]", path, i)
		if err := positive(r.MaxThickness, rp+".max_thickness"); err != nil {
			return err
		}
		if i > 0 && r.MaxThickness <= m.ThicknessRanges[i-1].MaxThickness {
			return apierror.ParameterInvalid(rp+".max_thickness", "%s.max_thickness must be greater than the previous range's: list ranges by increasing thickness", rp)
		}
		if err := validateOverrides(r.KerfWidth, r.UsableArea, r.Overrides, rp); err != nil {
			return err
		}
	}
	return nil
}

func validateOverrides(kerf *measure.Length, area *UsableArea, o LimitOverrides, path string) *apierror.Error {
	if kerf != nil {
		if err := nonNegative(*kerf, path+".kerf_width"); err != nil {
			return err
		}
	}
	if area != nil {
		if err := positive(area.Width, path+".usable_area.width"); err != nil {
			return err
		}
		if err := positive(area.Height, path+".usable_area.height"); err != nil {
			return err
		}
	}
	if err := validateLimit(o.HoleDiameter, path+".limits.hole_diameter"); err != nil {
		return err
	}
	if err := validateLimit(o.SlotWidth, path+".limits.slot_width"); err != nil {
		return err
	}
	return validateOptional(o.Optional, path+".limits")
}

// validateLimit accepts a nil limit (an absent override).
func validateLimit(l *Limit, path string) *apierror.Error {
	if l == nil {
		return nil
	}
	if l.Error == nil && l.Warning == nil {
		return apierror.ParameterInvalid(path, "%s needs an error and/or a warning threshold", path)
	}
	for _, tier := range []struct {
		name string
		t    *measure.Threshold
	}{{"error", l.Error}, {"warning", l.Warning}} {
		if t := tier.t; t != nil && (t.Min < 0 || t.ThicknessRatio < 0 || t.KerfRatio < 0) {
			return apierror.ParameterInvalid(path+"."+tier.name, "threshold terms must be >= 0")
		}
	}
	return nil
}

func positive(v measure.Length, param string) *apierror.Error {
	if v <= 0 {
		return apierror.ParameterInvalid(param, "%s must be greater than 0", param)
	}
	return nil
}

func nonNegative(v measure.Length, param string) *apierror.Error {
	if v < 0 {
		return apierror.ParameterInvalid(param, "%s must not be negative", param)
	}
	return nil
}

// Resolve combines a validated capability with a job's stock. An unknown material is not an error: the profile is marked unsupported (reported as a finding) and uses the capability's defaults so the geometry is still checked.
func Resolve(c Capability, s Stock) (*Profile, *apierror.Error) {
	if s.MaterialCode == "" {
		return nil, apierror.ParameterMissing("stock.material_code")
	}
	if s.Thickness <= 0 {
		return nil, apierror.ParameterInvalid("stock.thickness", "stock.thickness must be greater than 0")
	}
	p := &Profile{Process: c.Process, MaterialCode: s.MaterialCode, Thickness: s.Thickness}
	var material *Material
	for i := range c.Materials {
		if c.Materials[i].Code == s.MaterialCode {
			material = &c.Materials[i]
			break
		}
	}
	p.Supported = material != nil
	switch c.Process {
	case constants.ProcessLaserCutting:
		p.LaserCutting = resolveLaserCutting(c.LaserCutting, material, s.Thickness)
	case constants.ProcessBending:
		p.Bending = resolveBending(c.Bending, material, s.Thickness)
	}
	p.HoleOperations = resolveHoleOperations(c.HoleOperations, s)
	return p, nil
}

func resolveLaserCutting(l *LaserCutting, m *Material, thickness measure.Length) *LaserCuttingProfile {
	p := &LaserCuttingProfile{
		UsableWidth:  l.UsableWidth,
		UsableHeight: l.UsableHeight,
		KerfWidth:    l.KerfWidth,
		HoleDiameter: l.Limits.HoleDiameter,
		SlotWidth:    l.Limits.SlotWidth,
		Optional:     l.Limits.Optional,
	}
	if l.ContourClosureTolerance != nil {
		p.ContourClosureTolerance = *l.ContourClosureTolerance
	}
	p.SupportSpacing = l.SupportSpacing
	p.MultipleParts = l.MultipleParts
	if p.MultipleParts == "" {
		p.MultipleParts = constants.MultiplePartsAccepted
	}
	p.DrawingUnits = l.DrawingUnits
	p.CommonLine = l.CommonLine
	if p.CommonLine == "" {
		p.CommonLine = constants.CommonLineRejected
	}
	p.OutsidePage = l.OutsidePage
	if p.OutsidePage == "" {
		p.OutsidePage = constants.OutsidePageCut
	}
	p.FilledShapes = l.FilledShapes
	if p.FilledShapes == "" {
		p.FilledShapes = constants.ShapeOperationEngrave
	}
	if len(l.ColorOperations) > 0 {
		p.ColorOperations = map[string]constants.ShapeOperation{}
		for _, c := range l.ColorOperations {
			p.ColorOperations[strings.ToLower(c.Color)] = c.Operation
		}
	}
	if m == nil || m.LaserCutting == nil {
		return p
	}
	ml := m.LaserCutting
	p.MaxThickness = ml.MaxThickness
	p.MinThickness, p.Thicknesses = ml.MinThickness, ml.Thicknesses
	p.apply(ml.KerfWidth, ml.UsableArea, ml.Overrides)
	for _, r := range ml.ThicknessRanges {
		if thickness <= r.MaxThickness {
			p.apply(r.KerfWidth, r.UsableArea, r.Overrides)
			break
		}
	}
	return p
}

// apply replaces the kerf, the usable area and the limits that are set.
func (p *LaserCuttingProfile) apply(kerf *measure.Length, area *UsableArea, o LimitOverrides) {
	if kerf != nil {
		p.KerfWidth = *kerf
	}
	if area != nil {
		p.UsableWidth, p.UsableHeight = area.Width, area.Height
	}
	if o.HoleDiameter != nil {
		p.HoleDiameter = *o.HoleDiameter
	}
	if o.SlotWidth != nil {
		p.SlotWidth = *o.SlotWidth
	}
	p.Optional = p.Optional.merge(o.Optional)
}
