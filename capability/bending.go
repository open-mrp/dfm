package capability

import (
	"fmt"
	"strings"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
)

// DefaultKFactor is the K-factor of a capability that gives none: where the neutral axis lies, as a fraction of the thickness from the inside of the bend, in millionths.
const DefaultKFactor measure.Ratio = 440_000

// DefaultBendAngle is the angle of a bend the request does not give, in millionths of a degree.
const DefaultBendAngle measure.Ratio = 90_000_000

// Bending holds a press brake's limits.
type Bending struct {
	// MaxBendLength is the longest bend the brake forms; MinBendLength the shortest its tooling holds (0 when not checked).
	MaxBendLength, MinBendLength measure.Length
	// MaxForce is the brake's press force in millionths of a kilonewton; 0 means the force is not checked.
	MaxForce measure.Ratio
	// InsideRadius is the inside radius the brake's tooling forms; nil means the stock thickness.
	InsideRadius *measure.Threshold
	// MaxAngle is the largest bend angle the tooling forms (from flat, in millionths of a degree); 0 means angles are not checked.
	MaxAngle measure.Ratio
	// MinAngle is the smallest bend angle the brake holds repeatably (a shallow bend springs back unpredictably); 0 means not checked.
	MinAngle measure.Ratio
	// KFactor places the neutral axis, in millionths of the thickness from the inside of the bend; 0 means DefaultKFactor.
	KFactor measure.Ratio
	Limits  BendingLimits
	// Tooling is the punch and die bends are formed with; nil means the bend sequence is not simulated.
	Tooling *BrakeTooling
}

// BrakeTooling is a press brake's punch and die, as cross-sections and lengths. Angles are in millionths of a degree.
type BrakeTooling struct {
	// PunchNose, PunchThroatDepth and PunchThroatHeight describe a gooseneck punch: above the nose (the straight part over the tip) a neck slopes back at 45 degrees to the body, set back by the throat's depth, away from the flange being formed, so a return flange taller than the web it stands beside clears it. All 0 for a straight punch.
	PunchNose, PunchThroatDepth, PunchThroatHeight measure.Length
	// PunchTipAngle is the included angle of the punch's tip; PunchWidth the width of its body; PunchHeight how far it reaches above its tip (to the ram); PunchLength its length along the bend, 0 for segmented tooling assembled to each bend's length.
	PunchTipAngle                        measure.Ratio
	PunchWidth, PunchHeight, PunchLength measure.Length
	// DieOpening is the width of the V at the die's top face; DieAngle the V's included angle (0: matched to each bend, 180° less its angle); DieWidth the die's top face beside the opening, each side; DieHeight its depth; DieLength its length along the bend, 0 for segmented tooling.
	DieOpening                     measure.Threshold
	DieAngle                       measure.Ratio
	DieWidth, DieHeight, DieLength measure.Length
}

// BendLimit is a bending limit: a Limit whose requirement has the inside radius the tooling forms added to it when PlusInsideRadius is set (e.g. 2 t + R), since a larger radius widens the bend.
type BendLimit struct {
	Limit
	PlusInsideRadius bool
}

// BendingLimits are the bending checks' limits; a nil limit is not checked.
type BendingLimits struct {
	// FlangeLength is the shortest flange beside a bend: the material from the bend line to the part's edge, on either side.
	FlangeLength *BendLimit
	// FeatureDistance is the least distance from a bend line to a hole or cut-out, which the bend would distort.
	FeatureDistance *BendLimit
	// ReliefWidth is the narrowest bend relief: the gap a bend line ends at, along the line.
	ReliefWidth *BendLimit
	// ReliefDepth is the shallowest bend relief: how far the gap reaches past the bend line.
	ReliefDepth *BendLimit
	// MinInsideRadius is the tightest inside radius a design may be drawn for: a bend drawn much tighter than tooling forms has a flat pattern no brake makes.
	MinInsideRadius *BendLimit
}

// merge overrides each limit that o sets.
func (l BendingLimits) merge(o BendingLimits) BendingLimits {
	if o.FlangeLength != nil {
		l.FlangeLength = o.FlangeLength
	}
	if o.FeatureDistance != nil {
		l.FeatureDistance = o.FeatureDistance
	}
	if o.ReliefWidth != nil {
		l.ReliefWidth = o.ReliefWidth
	}
	if o.ReliefDepth != nil {
		l.ReliefDepth = o.ReliefDepth
	}
	if o.MinInsideRadius != nil {
		l.MinInsideRadius = o.MinInsideRadius
	}
	return l
}

// MaterialBending says a material can be bent, and in what thicknesses.
type MaterialBending struct {
	// MaxThickness is the thickest stock the brake bends in this material.
	MaxThickness measure.Length
	// MinThickness is the thinnest; 0 means no lower bound.
	MinThickness measure.Length
	// ForceFactor scales the force to bend this material relative to mild steel (e.g. 1.5 for stainless, 0.55 for aluminum), in millionths; 0 means 1.
	ForceFactor measure.Ratio
	// InsideRadius, MinAngle, MaxAngle and KFactor, when set, replace the brake's.
	InsideRadius *measure.Threshold
	MinAngle     measure.Ratio
	MaxAngle     measure.Ratio
	KFactor      measure.Ratio
	// Overrides replace the brake's limits of the same name.
	Overrides BendingLimits
}

// BendingProfile is a press brake resolved for one job's stock.
type BendingProfile struct {
	MaxBendLength, MinBendLength measure.Length
	MinThickness, MaxThickness   measure.Length
	MaxForce, ForceFactor        measure.Ratio
	// InsideRadius is the radius the tooling forms in this stock, exact in millimeters.
	InsideRadius measure.Decimal
	// MinAngle and MaxAngle are 0 when not checked.
	MinAngle, MaxAngle measure.Ratio
	// KFactor is never 0.
	KFactor measure.Ratio
	// Tooling is nil when the bend sequence is not simulated.
	Tooling *ResolvedTooling
	BendingLimits
}

// ResolvedTooling is a press brake's punch and die for one job's stock: the die opening resolved from its threshold.
type ResolvedTooling struct {
	BrakeTooling
	// DieOpeningMM is the width of the V for this stock, exact in millimeters, resolved from BrakeTooling.DieOpening.
	DieOpeningMM measure.Decimal
}

func resolveBending(b *Bending, m *Material, thickness measure.Length) *BendingProfile {
	p := &BendingProfile{MaxBendLength: b.MaxBendLength, MinBendLength: b.MinBendLength, MaxForce: b.MaxForce, ForceFactor: 1_000_000, MinAngle: b.MinAngle, MaxAngle: b.MaxAngle, KFactor: b.KFactor, BendingLimits: b.Limits}
	if b.Tooling != nil {
		p.Tooling = &ResolvedTooling{BrakeTooling: *b.Tooling, DieOpeningMM: b.Tooling.DieOpening.Required(thickness, 0).Value}
	}
	if p.KFactor == 0 {
		p.KFactor = DefaultKFactor
	}
	radius := b.InsideRadius
	if m != nil && m.Bending != nil {
		p.MinThickness, p.MaxThickness = m.Bending.MinThickness, m.Bending.MaxThickness
		if m.Bending.ForceFactor > 0 {
			p.ForceFactor = m.Bending.ForceFactor
		}
		if m.Bending.InsideRadius != nil {
			radius = m.Bending.InsideRadius
		}
		if m.Bending.MaxAngle > 0 {
			p.MaxAngle = m.Bending.MaxAngle
		}
		if m.Bending.MinAngle > 0 {
			p.MinAngle = m.Bending.MinAngle
		}
		if m.Bending.KFactor > 0 {
			p.KFactor = m.Bending.KFactor
		}
		p.BendingLimits = p.BendingLimits.merge(m.Bending.Overrides)
	}
	p.InsideRadius = thickness.Decimal()
	if radius != nil {
		p.InsideRadius = radius.Required(thickness, 0).Value
	}
	return p
}

func validateBending(b *Bending) *apierror.Error {
	path := root + ".bending"
	if b == nil {
		return apierror.ParameterMissing(path)
	}
	if err := positive(b.MaxBendLength, path+".max_bend_length"); err != nil {
		return err
	}
	if b.MinBendLength < 0 || b.MinBendLength > b.MaxBendLength {
		return apierror.ParameterInvalid(path+".min_bend_length", "%s.min_bend_length must be at least 0 and at most max_bend_length", path)
	}
	if b.MaxForce < 0 {
		return apierror.ParameterInvalid(path+".max_force", "%s.max_force must not be negative; 0 means the force is not checked", path)
	}
	if err := validateMinAngle(b.MinAngle, b.MaxAngle, path); err != nil {
		return err
	}
	if err := validateBendShape(b.InsideRadius, b.MaxAngle, b.KFactor, path); err != nil {
		return err
	}
	if err := validateTooling(b.Tooling, path+".tooling"); err != nil {
		return err
	}
	return validateBendingLimits(b.Limits, path+".limits")
}

// validateTooling checks a punch and die: positive sizes and angles below 180 degrees.
func validateTooling(t *BrakeTooling, path string) *apierror.Error {
	if t == nil {
		return nil
	}
	if t.PunchTipAngle <= 0 || t.PunchTipAngle >= 180_000_000 {
		return apierror.ParameterInvalid(path+".punch.tip_angle", "%s.punch.tip_angle must be greater than 0 and below 180 degrees", path)
	}
	if t.DieAngle < 0 || t.DieAngle >= 180_000_000 {
		return apierror.ParameterInvalid(path+".die.angle", "%s.die.angle must be at least 0 and below 180 degrees; 0 matches each bend", path)
	}
	for _, x := range []struct {
		l    measure.Length
		name string
	}{{t.PunchWidth, "punch.width"}, {t.PunchHeight, "punch.height"}, {t.DieWidth, "die.width"}, {t.DieHeight, "die.height"}} {
		if err := positive(x.l, path+"."+x.name); err != nil {
			return err
		}
	}
	if o := t.DieOpening; o.Min < 0 || o.ThicknessRatio < 0 || o.KerfRatio != 0 || o.Min == 0 && o.ThicknessRatio == 0 {
		return apierror.ParameterInvalid(path+".die.opening", "%s.die.opening needs a positive min or thickness_ratio, and no kerf_ratio", path)
	}
	if (t.PunchThroatDepth > 0) != (t.PunchThroatHeight > 0) {
		return apierror.ParameterInvalid(path+".punch.gooseneck", "%s.punch.gooseneck needs both throat_depth and throat_height", path)
	}
	for _, x := range []struct {
		l    measure.Length
		name string
	}{{t.PunchLength, "punch.length"}, {t.DieLength, "die.length"}, {t.PunchNose, "punch.gooseneck.nose_height"}, {t.PunchThroatDepth, "punch.gooseneck.throat_depth"}, {t.PunchThroatHeight, "punch.gooseneck.throat_height"}} {
		if err := nonNegative(x.l, path+"."+x.name); err != nil {
			return err
		}
	}
	return nil
}

func validateBendingLimits(l BendingLimits, path string) *apierror.Error {
	for _, x := range []struct {
		lim  *BendLimit
		name string
	}{{l.FlangeLength, "flange_length"}, {l.FeatureDistance, "feature_distance"}, {l.ReliefWidth, "relief_width"}, {l.ReliefDepth, "relief_depth"}, {l.MinInsideRadius, "min_inside_radius"}} {
		if x.lim == nil {
			continue
		}
		if err := validateLimit(&x.lim.Limit, path+"."+x.name); err != nil {
			return err
		}
	}
	return nil
}

func validateMaterialBending(m *MaterialBending, path string) *apierror.Error {
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
	if m.ForceFactor < 0 {
		return apierror.ParameterInvalid(path+".force_factor", "%s.force_factor must not be negative; 0 means 1", path)
	}
	if err := validateMinAngle(m.MinAngle, m.MaxAngle, path); err != nil {
		return err
	}
	if err := validateBendShape(m.InsideRadius, m.MaxAngle, m.KFactor, path); err != nil {
		return err
	}
	return validateBendingLimits(m.Overrides, path+".limits")
}

// validateMinAngle checks a smallest angle against the largest.
func validateMinAngle(minAngle, maxAngle measure.Ratio, path string) *apierror.Error {
	if minAngle < 0 || minAngle >= 180_000_000 || maxAngle > 0 && minAngle > maxAngle {
		return apierror.ParameterInvalid(path+".min_angle", "%s.min_angle must be at least 0, below 180 degrees and at most max_angle", path)
	}
	return nil
}

// validateBendShape checks an inside radius, a largest angle and a K-factor.
func validateBendShape(radius *measure.Threshold, maxAngle, kFactor measure.Ratio, path string) *apierror.Error {
	if kFactor < 0 || kFactor > 500_000 {
		return apierror.ParameterInvalid(path+".k_factor", "%s.k_factor must be at least 0 and at most 0.5; 0 means the default", path)
	}
	if r := radius; r != nil && (r.Min < 0 || r.ThicknessRatio < 0 || r.KerfRatio != 0 || r.Min == 0 && r.ThicknessRatio == 0) {
		return apierror.ParameterInvalid(path+".inside_radius", "%s.inside_radius needs a positive min or thickness_ratio, and no kerf_ratio", path)
	}
	if maxAngle < 0 || maxAngle > 180_000_000 {
		return apierror.ParameterInvalid(path+".max_angle", "%s.max_angle must be at least 0 and at most 180 degrees; 0 means angles are not checked", path)
	}
	return nil
}

// BendAssignment gives some bend lines of a design their angle, direction and inside radius. Zero fields are not given: the bend takes the default (DefaultBendAngle, up, the brake's inside radius).
type BendAssignment struct {
	EntityIDs []string
	// Angle is from flat, in millionths of a degree.
	Angle        measure.Ratio
	Direction    constants.BendDirection
	InsideRadius measure.Length
}

// ValidateBendAssignments checks the assignments on their own, before the design is read: every assignment names entities, each entity once, with an angle above 0 and at most 180 degrees and a positive radius.
func ValidateBendAssignments(as []BendAssignment) *apierror.Error {
	seen := map[string]bool{}
	for i, a := range as {
		path := fmt.Sprintf("bends[%d]", i)
		if len(a.EntityIDs) == 0 {
			return apierror.ParameterInvalid(path+".entity_ids", "%s.entity_ids must name at least one bend line", path)
		}
		for j, id := range a.EntityIDs {
			if seen[id] {
				return apierror.ParameterInvalid(fmt.Sprintf("%s.entity_ids[%d]", path, j), "bend line %q is listed more than once", id)
			}
			seen[id] = true
		}
		if a.Angle < 0 || a.Angle > 180_000_000 {
			return apierror.ParameterInvalid(path+".angle", "%s.angle must be at least 0 and at most 180 degrees; 0 is not given", path)
		}
		if a.Direction != "" && !a.Direction.IsValid() {
			return apierror.ParameterInvalid(path+".direction", "%s.direction must be one of %s", path, strings.Join(a.Direction.EnumValues(), ", "))
		}
		if a.InsideRadius < 0 {
			return apierror.ParameterInvalid(path+".inside_radius", "%s.inside_radius must not be negative; 0 is not given", path)
		}
	}
	return nil
}
