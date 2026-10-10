package capability

import (
	"fmt"
	"strings"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
)

// HoleOperation is one operation a setup offers on round holes: a tap, a countersink or a press-in (self-clinching) fastener. Holes it is assigned to are resized to HoleDiameter.
type HoleOperation struct {
	// Code identifies the operation in a job's hole assignments.
	Code string
	Type constants.HoleOperationType
	Name string
	// Thread is set for taps, and for fasteners that carry a thread.
	Thread *Thread
	// Countersink is set for countersinks.
	Countersink *Countersink
	// Hardware is set for fasteners.
	Hardware *Hardware
	// HoleDiameter is the hole the operation needs (tap drill, countersink minor diameter, fastener mounting hole).
	HoleDiameter measure.Length
	// Materials lists the sheet materials the operation is offered for, each with its thickness range.
	Materials []HoleOperationMaterial
	// EdgeDistance is the least distance from the hole's center to any other cut; Spacing the least distance between the centers of two holes with operations that set it. Either may be nil.
	EdgeDistance *Limit
	Spacing      *Limit
	// BendDistance is the least distance from the hole's edge to a bend line, which would distort the thread, countersink or fastener; nil when not checked.
	BendDistance *Limit
}

// Thread is a screw thread.
type Thread struct {
	NominalDiameter measure.Length
	Pitch           measure.Length
}

// Countersink is the cone a countersink cuts.
type Countersink struct {
	// AngleDeg is the cone's included angle, in millionths of a degree.
	AngleDeg measure.Ratio
	// MajorDiameter is the cone's diameter at the face.
	MajorDiameter measure.Length
	// MaxDepthRatio is the deepest the cone may go, as a share of the thickness (millionths).
	MaxDepthRatio measure.Ratio
}

// Hardware is a press-in fastener.
type Hardware struct {
	// Type is what the fastener is, e.g. "nut", "stud" or "standoff".
	Type string
}

// HoleOperationMaterial is a sheet material an operation is offered for.
type HoleOperationMaterial struct {
	MaterialCode string
	MinThickness measure.Length
	// MaxThickness is nil when there is no upper bound.
	MaxThickness *measure.Length
	// EdgeDistance overrides the operation's for this material.
	EdgeDistance *Limit
}

// HoleAssignment assigns an operation to holes of a job's design: those whose entities are listed, or every round hole drawn at Diameter, within part number Part when it is not 0.
type HoleAssignment struct {
	Operation string
	EntityIDs []string
	Diameter  measure.Length
	Part      int
	// Side is required for countersinks and fasteners.
	Side constants.HoleSide
}

// ResolvedHoleOperation is an operation resolved for one job's stock.
type ResolvedHoleOperation struct {
	*HoleOperation
	// Available is false when the operation is not offered for the stock's material and thickness.
	Available bool
	// EdgeDistance is the material's override, or the operation's.
	EdgeDistance *Limit
}

func validateHoleOperations(c Capability) *apierror.Error {
	materials := map[string]bool{}
	for _, m := range c.Materials {
		materials[m.Code] = true
	}
	seen := map[string]bool{}
	for i, op := range c.HoleOperations {
		path := fmt.Sprintf("%s.hole_operations[%d]", root, i)
		if !materialCode.MatchString(op.Code) {
			return apierror.ParameterInvalid(path+".code", "code %q must be 1-64 lowercase letters, digits, '_', '.' or '-'", op.Code)
		}
		if seen[op.Code] {
			return apierror.ParameterInvalid(path+".code", "code %q is listed more than once", op.Code)
		}
		seen[op.Code] = true
		if !op.Type.IsValid() {
			return apierror.ParameterInvalid(path+".type", "%s.type must be one of %s", path, strings.Join(op.Type.EnumValues(), ", "))
		}
		needs := map[constants.HoleOperationType]struct {
			name string
			set  bool
		}{
			constants.HoleOperationTap:         {"thread", op.Thread != nil},
			constants.HoleOperationCountersink: {"countersink", op.Countersink != nil},
			constants.HoleOperationHardware:    {"hardware", op.Hardware != nil},
		}
		if n := needs[op.Type]; !n.set {
			return apierror.ParameterMissing(path + "." + n.name)
		}
		if op.Type != constants.HoleOperationCountersink && op.Countersink != nil {
			return apierror.ParameterInvalid(path+".countersink", "countersink is only allowed when type is countersink")
		}
		if op.Type != constants.HoleOperationHardware && op.Hardware != nil {
			return apierror.ParameterInvalid(path+".hardware", "hardware is only allowed when type is hardware")
		}
		if err := positive(op.HoleDiameter, path+".hole_diameter"); err != nil {
			return err
		}
		if t := op.Thread; t != nil {
			if err := positive(t.NominalDiameter, path+".thread.nominal_diameter"); err != nil {
				return err
			}
			if err := positive(t.Pitch, path+".thread.pitch"); err != nil {
				return err
			}
		}
		if cs := op.Countersink; cs != nil {
			if cs.AngleDeg <= 0 || cs.AngleDeg >= 180*measure.Ratio(1e6) {
				return apierror.ParameterInvalid(path+".countersink.angle", "%s.countersink.angle must be between 0 and 180 degrees", path)
			}
			if cs.MajorDiameter <= op.HoleDiameter {
				return apierror.ParameterInvalid(path+".countersink.major_diameter", "%s.countersink.major_diameter must be larger than hole_diameter", path)
			}
			if cs.MaxDepthRatio <= 0 || cs.MaxDepthRatio > measure.Ratio(1e6) {
				return apierror.ParameterInvalid(path+".countersink.max_depth_ratio", "%s.countersink.max_depth_ratio must be greater than 0 and at most 1", path)
			}
		}
		if len(op.Materials) == 0 {
			return apierror.ParameterInvalid(path+".materials", "%s.materials must list at least one material", path)
		}
		listed := map[string]bool{}
		for j, m := range op.Materials {
			mp := fmt.Sprintf("%s.materials[%d]", path, j)
			if !materials[m.MaterialCode] {
				return apierror.ParameterInvalid(mp+".material_code", "material %q is not in the capability's materials", m.MaterialCode)
			}
			if listed[m.MaterialCode] {
				return apierror.ParameterInvalid(mp+".material_code", "material %q is listed more than once", m.MaterialCode)
			}
			listed[m.MaterialCode] = true
			if err := nonNegative(m.MinThickness, mp+".min_thickness"); err != nil {
				return err
			}
			if m.MaxThickness != nil && *m.MaxThickness < m.MinThickness {
				return apierror.ParameterInvalid(mp+".max_thickness", "%s.max_thickness must be at least min_thickness", mp)
			}
			if err := validateLimit(m.EdgeDistance, mp+".edge_distance"); err != nil {
				return err
			}
		}
		if err := validateLimit(op.EdgeDistance, path+".edge_distance"); err != nil {
			return err
		}
		if err := validateLimit(op.BendDistance, path+".bend_distance"); err != nil {
			return err
		}
		if err := validateLimit(op.Spacing, path+".spacing"); err != nil {
			return err
		}
	}
	return nil
}

// ValidateHoleAssignments checks a job's hole assignments against the capability's catalog. Param paths are under "hole_operations".
func ValidateHoleAssignments(c Capability, as []HoleAssignment) *apierror.Error {
	ops := map[string]*HoleOperation{}
	for i := range c.HoleOperations {
		ops[c.HoleOperations[i].Code] = &c.HoleOperations[i]
	}
	for i, a := range as {
		path := fmt.Sprintf("hole_operations[%d]", i)
		op, ok := ops[a.Operation]
		if !ok {
			return apierror.ParameterInvalid(path+".operation", "operation %q is not in the capability's hole_operations", a.Operation)
		}
		if (len(a.EntityIDs) > 0) == (a.Diameter > 0) {
			return apierror.ParameterInvalid(path+".holes", "%s.holes needs exactly one of entity_ids or diameter", path)
		}
		if a.Diameter < 0 {
			return apierror.ParameterInvalid(path+".holes.diameter", "%s.holes.diameter must be greater than 0", path)
		}
		if a.Part < 0 {
			return apierror.ParameterInvalid(path+".holes.part", "%s.holes.part must be a part number, 1 or more", path)
		}
		if a.Side != "" && !a.Side.IsValid() {
			return apierror.ParameterInvalid(path+".side", "%s.side must be one of %s", path, strings.Join(a.Side.EnumValues(), ", "))
		}
		if a.Side == "" && op.Type != constants.HoleOperationTap {
			return apierror.ParameterInvalid(path+".side", "%s.side is required for a %s (top or bottom)", path, op.Type)
		}
	}
	return nil
}

// resolveHoleOperations resolves the catalog for the stock.
func resolveHoleOperations(ops []HoleOperation, s Stock) map[string]*ResolvedHoleOperation {
	if len(ops) == 0 {
		return nil
	}
	out := map[string]*ResolvedHoleOperation{}
	for i := range ops {
		op := &ops[i]
		r := &ResolvedHoleOperation{HoleOperation: op, EdgeDistance: op.EdgeDistance}
		for _, m := range op.Materials {
			if m.MaterialCode != s.MaterialCode {
				continue
			}
			r.Available = s.Thickness >= m.MinThickness && (m.MaxThickness == nil || s.Thickness <= *m.MaxThickness)
			if m.EdgeDistance != nil {
				r.EdgeDistance = m.EdgeDistance
			}
		}
		out[op.Code] = r
	}
	return out
}
