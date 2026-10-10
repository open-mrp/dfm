// Package rules contains the DFM rule engine and the individual rules.
//
// Rules operate only on geometry.Part and a resolved capability.Profile; they never see source file formats or wire types.
package rules

import (
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// Finding is one DFM result. Message is a short sentence for the code, which some rules vary by case (what a bend ends at, which tool a bend runs into); values are in the numeric fields, all exact and in millimeters.
type Finding struct {
	// ID identifies the finding within its result, derived from its content (set by dfm.Check).
	ID        string
	Code      constants.FindingCode
	Severity  constants.Severity
	Message   string
	EntityIDs []string
	// Actual is the measured value; measurements from geometry are rounded to the nearest nanometer.
	Actual *measure.Decimal
	// Required is the limit that was not met, exactly as computed.
	Required *measure.Decimal
	// RequiredBasis is set for limit findings.
	RequiredBasis *constants.ThresholdBasis
	// Unit of Actual and Required; empty means millimeters.
	Unit     constants.Unit
	Location *Location
	// Span is the two ends of a measured width or gap, for findings that measure one.
	Span *[2]Location
	// Fix is a change to how the drawing is read that resolves the finding, for the customer to accept, or nil.
	Fix *Fix
	// Parts are the 1-based numbers of the parts the finding is about: one usually, several when the problem lies between parts (a shared edge), none for a finding about the drawing as a whole (set by dfm.Check).
	Parts []int
}

// Fix is a suggested change to how the drawing is read. A request accepts it as a design adjustment with the same fields.
type Fix struct {
	Action    constants.FixAction
	EntityIDs []string
	Units     constants.LengthUnit
}

// span returns the two ends of a measured width or gap.
func span(a, b geometry.Point) *[2]Location {
	return &[2]Location{*at(a), *at(b)}
}

// Location is a point in the drawing, rounded to the nearest nanometer.
type Location struct {
	X, Y measure.Length
}

// Rule is a single DFM check.
type Rule interface {
	// Code returns the finding code this rule emits.
	Code() constants.FindingCode
	// Check returns findings for part under the job's resolved profile. It must be deterministic and must not mutate its inputs. It may assume the profile is for the rule's process (ForProcess chose it), that the rule is Enabled for the profile (the limit it checks is set), and that the part's hole assignments were validated against the capability.
	Check(part *geometry.Part, p *capability.Profile) []Finding
}

// ChecksDrawing reports whether r checks the drawing as a whole (its hygiene, the stock) rather than each part, as its catalog entry's Scope says.
func ChecksDrawing(r Rule) bool {
	info, ok := Info(r.Code())
	return ok && info.Scope == constants.FindingScopeDrawing
}

// Enabled reports whether r's check runs under the profile: a check driven by a capability limit the profile does not set is skipped, and Skipped lists it. Check is only called on an enabled rule.
func Enabled(r Rule, p *capability.Profile) bool {
	info, ok := catalogIndex[p.Process][r.Code()]
	return !ok || info.limitSet == nil || info.limitSet(p)
}

// geometryRules apply to every 2D cutting process.
func geometryRules() []Rule {
	return []Rule{
		OpenContourRule{},
		GapClosedRule{},
		ZeroLengthRule{},
		TextRule{},
		NonCutRule{},
		BendLineRule{},
		SuspectedBendRule{},
		BorderRule{},
		EngravingRule{},
		AnnotationRule{},
		DuplicateRule{},
		OvercutRule{},
		CornerLoopRule{},
		FacetedCurveRule{},
		OutsidePageRule{},
		CommonLineRule{},
		OverlapRule{},
		SelfIntersectionRule{},
		ContactRule{},
		FlatOverlapRule{},
		NotFlatRule{},
		MultiplePartsRule{},
		NestedPartRule{},
		FloatingIslandRule{},
		MaterialRule{},
	}
}

// ForProcess returns the rules for a process in their fixed execution order: drawing hygiene, part topology, material, then dimensional limits. Findings are reported in this order.
func ForProcess(p constants.Process) []Rule {
	switch p {
	case constants.ProcessLaserCutting:
		return append(geometryRules(),
			ThicknessRule{},
			MaterialTooThinRule{},
			ThicknessStockedRule{},
			ModelThicknessRule{},
			UnitsRule{},
			UnitsUnsupportedRule{},
			UnitsAssumedRule{},
			EnvelopeRule{},
			PartWidthRule{},
			PartLengthRule{},
			PartAreaRule{},
			SlenderPartRule{},
			PartFallRule{},
			SlenderCutoutRule{},
			NotchSlenderRule{},
			HoleRule{},
			HoleOpNotAHoleRule{},
			HoleOpConflictRule{},
			HoleOpUnavailableRule{},
			HoleResizedRule{},
			HoleOpEdgeRule{},
			HoleOpSpacingRule{},
			CountersinkDepthRule{},
			SlotRule{},
			SlotFitRule{},
			WebRule{},
			FeatureSpacingRule{},
			EdgeDistanceRule{},
			CutoutWidthRule{},
			FeatureWidthRule{},
			CornerRule{},
			TipRule{},
			DensityRule{},
		)
	case constants.ProcessBending:
		return []Rule{
			MaterialRule{},
			ThicknessRule{},
			MaterialTooThinRule{},
			BendLengthRule{},
			BendShortRule{},
			BendForceRule{},
			BendAngleRule{},
			BendAngleMinRule{},
			BendRadiusRule{},
			BendRadiusMinRule{},
			FlangeRule{},
			BendZonesRule{},
			BendToolingRule{},
			BendCollisionRule{},
			FlangeCollisionRule{},
			BendFeatureRule{},
			BendReliefRule{},
			BendCornerRule{},
			BendCrossRule{},
			BendReliefWidthRule{},
			BendReliefDepthRule{},
		}
	case constants.ProcessCountersinking:
		return []Rule{HoleOpBendRule{Type: constants.HoleOperationCountersink}}
	case constants.ProcessTapping:
		return []Rule{HoleOpBendRule{Type: constants.HoleOperationTap}}
	case constants.ProcessHardwareInsertion:
		return []Rule{HoleOpBendRule{Type: constants.HoleOperationHardware}}
	default:
		return nil
	}
}

// at converts a geometry point to a Location, or nil if it is beyond the representable range (only possible for pathological input).
func at(p geometry.Point) *Location {
	x, errX := measure.FromMM(p.X)
	y, errY := measure.FromMM(p.Y)
	if errX != nil || errY != nil {
		return nil
	}
	return &Location{X: x, Y: y}
}

// mm converts a measured value to an exact decimal, rounded to 1 nm, or nil if it is beyond the representable range.
func mm(v float64) *measure.Decimal {
	l, err := measure.FromMM(v)
	if err != nil {
		return nil
	}
	return dec(l)
}

func dec(l measure.Length) *measure.Decimal {
	d := l.Decimal()
	return &d
}

// checkLimit compares a measured value against a two-tier limit. It returns the severity and requirement of the first tier violated (error before warning), or ok=false when the value meets both.
func checkLimit(lim capability.Limit, thickness, kerf, actual measure.Length) (constants.Severity, measure.Requirement, bool) {
	if lim.Error != nil {
		if r := lim.Error.Required(thickness, kerf); r.Below(actual) {
			return constants.SeverityError, r, true
		}
	}
	if lim.Warning != nil {
		if r := lim.Warning.Required(thickness, kerf); r.Below(actual) {
			return constants.SeverityWarning, r, true
		}
	}
	return "", measure.Requirement{}, false
}

// Locate returns f with a location when it has none and names entities: the point representing the loop they draw, or else a point on the first of them that is in the part.
func Locate(f Finding, part *geometry.Part) Finding {
	if f.Location == nil {
		if i := part.LoopOf(f.EntityIDs); i >= 0 {
			f.Location = at(part.LoopPoint(i))
		} else if q := firstEntityPoint(part, f.EntityIDs); q != nil {
			f.Location = at(*q)
		}
	}
	return f
}

func firstEntityPoint(part *geometry.Part, ids []string) *geometry.Point {
	for _, id := range ids {
		if q := part.EntityPoint(id); q != nil {
			return q
		}
	}
	return nil
}

func limitFinding(code constants.FindingCode, msg string, sev constants.Severity, req measure.Requirement, actual measure.Length, ids []string) Finding {
	r, basis := req.Value, req.Basis
	return Finding{
		Code:          code,
		Severity:      sev,
		Message:       msg,
		EntityIDs:     ids,
		Actual:        dec(actual),
		Required:      &r,
		RequiredBasis: &basis,
	}
}

// searchRadius bounds the distances worth measuring for a limit: the larger of its tier requirements, plus a margin for floating-point noise.
func searchRadius(lim capability.Limit, thickness, kerf measure.Length) float64 {
	r := 0.0
	for _, t := range []*measure.Threshold{lim.Error, lim.Warning} {
		if t != nil {
			r = max(r, t.Required(thickness, kerf).Value.Float64())
		}
	}
	return r + 1e-6
}

// contactLength is the gap below which paths are reported as touching by the contact rule rather than as a thin web or close features.
var contactLength = measure.MustFromMM(geometry.EndpointTolerance)
