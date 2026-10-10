package capability

import (
	"testing"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/measure"
)

func mm(v float64) measure.Length { return measure.MustFromMM(v) }

func limit(minMM float64) *Limit { return &Limit{Error: &measure.Threshold{Min: mm(minMM)}} }

// laser is a valid laser-cutting capability with one material and one tap.
func laser() Capability {
	return Capability{
		Process: constants.ProcessLaserCutting,
		LaserCutting: &LaserCutting{
			UsableWidth: mm(1500), UsableHeight: mm(3000), KerfWidth: mm(0.15),
			Limits: LaserCuttingLimits{HoleDiameter: *limit(1.5), SlotWidth: *limit(1)},
		},
		Materials: []Material{{Code: "mild_steel", LaserCutting: &MaterialLaserCutting{MaxThickness: mm(20), Thicknesses: []measure.Length{mm(1.5), mm(3)}}}},
		HoleOperations: []HoleOperation{{
			Code: "tap_m4", Type: constants.HoleOperationTap, Name: "M4", Thread: &Thread{NominalDiameter: mm(4), Pitch: mm(0.7)}, HoleDiameter: mm(3.3),
			Materials: []HoleOperationMaterial{{MaterialCode: "mild_steel", MinThickness: mm(1)}},
		}},
	}
}

// brake is a valid bending capability with one material.
func brake() Capability {
	return Capability{
		Process:   constants.ProcessBending,
		Bending:   &Bending{MaxBendLength: mm(3000), InsideRadius: &measure.Threshold{ThicknessRatio: 1_000_000}},
		Materials: []Material{{Code: "mild_steel", Bending: &MaterialBending{MaxThickness: mm(6)}}},
	}
}

// TestValidateParams checks that each kind of problem is reported under the parameter that holds it, in the wire's dotted path.
func TestValidateParams(t *testing.T) {
	tests := []struct {
		name  string
		mut   func(c *Capability)
		param string
	}{
		{"unknown process", func(c *Capability) { c.Process = "welding" }, "capability.process"},
		{"hole operation process", func(c *Capability) { c.Process = constants.ProcessTapping }, "capability.process"},
		{"no laser block", func(c *Capability) { c.LaserCutting = nil }, "capability.laser_cutting"},
		{"zero width", func(c *Capability) { c.LaserCutting.UsableWidth = 0 }, "capability.laser_cutting.usable_area.width"},
		{"negative kerf", func(c *Capability) { c.LaserCutting.KerfWidth = -1 }, "capability.laser_cutting.kerf_width"},
		{"closure too wide", func(c *Capability) { t := mm(2); c.LaserCutting.ContourClosureTolerance = &t }, "capability.laser_cutting.contour_closure_tolerance"},
		{"bad multiple parts", func(c *Capability) { c.LaserCutting.MultipleParts = "sometimes" }, "capability.laser_cutting.multiple_parts"},
		{"bad color", func(c *Capability) {
			c.LaserCutting.ColorOperations = []ColorOperation{{Color: "red", Operation: constants.ShapeOperationCut}}
		}, "capability.laser_cutting.color_operations[0].color"},
		{"color twice", func(c *Capability) {
			c.LaserCutting.ColorOperations = []ColorOperation{{Color: "#ff0000", Operation: constants.ShapeOperationCut}, {Color: "#FF0000", Operation: constants.ShapeOperationEngrave}}
		}, "capability.laser_cutting.color_operations[1].color"},
		{"bad unit", func(c *Capability) { c.LaserCutting.DrawingUnits = []constants.LengthUnit{"furlong"} }, "capability.laser_cutting.drawing_units[0]"},
		{"empty limit", func(c *Capability) { c.LaserCutting.Limits.HoleDiameter = Limit{} }, "capability.laser_cutting.limits.hole_diameter"},
		{"negative term", func(c *Capability) { c.LaserCutting.Limits.SlotWidth.Error.Min = -1 }, "capability.laser_cutting.limits.slot_width.error"},
		{"optional limit empty", func(c *Capability) { c.LaserCutting.Limits.Optional.WebWidth = &Limit{} }, "capability.laser_cutting.limits.web_width"},
		{"two bad optional limits report the first", func(c *Capability) {
			c.LaserCutting.Limits.Optional.FeatureWidth, c.LaserCutting.Limits.Optional.EdgeDistance = &Limit{}, &Limit{}
		}, "capability.laser_cutting.limits.edge_distance"},
		{"both tiers bad report error first", func(c *Capability) {
			c.LaserCutting.Limits.Optional.WebWidth = &Limit{Error: &measure.Threshold{Min: -1}, Warning: &measure.Threshold{Min: -1}}
		}, "capability.laser_cutting.limits.web_width.error"},
		{"part area empty", func(c *Capability) { c.LaserCutting.Limits.Optional.PartArea = &AreaLimit{} }, "capability.laser_cutting.limits.part_area"},
		{"part area zero", func(c *Capability) {
			z := measure.Area(0)
			c.LaserCutting.Limits.Optional.PartArea = &AreaLimit{Error: &z}
		}, "capability.laser_cutting.limits.part_area.error"},
		{"tiers out of order", func(c *Capability) {
			c.LaserCutting.Limits.Optional.SlenderPart = []SlenderTier{{MinLength: mm(100), Width: *limit(5)}, {MinLength: mm(50), Width: *limit(10)}}
		}, "capability.laser_cutting.limits.slender_part[1].min_length"},
		{"window without percent", func(c *Capability) { w := mm(60); c.LaserCutting.Limits.Optional.RemovedAreaWindow = &w }, "capability.laser_cutting.limits.removed_area_window"},
		{"percent over 100", func(c *Capability) {
			r := measure.Ratio(200_000_000)
			c.LaserCutting.Limits.Optional.MaxRemovedAreaPercent = &r
		}, "capability.laser_cutting.limits.max_removed_area_percent"},
		{"no materials", func(c *Capability) { c.Materials = nil; c.HoleOperations = nil }, "capability.materials"},
		{"bad material code", func(c *Capability) { c.Materials[0].Code = "Mild Steel" }, "capability.materials[0].code"},
		{"material twice", func(c *Capability) { c.Materials = append(c.Materials, c.Materials[0]) }, "capability.materials[1].code"},
		{"no material block", func(c *Capability) { c.Materials[0].LaserCutting = nil }, "capability.materials[0].laser_cutting"},
		{"min over max", func(c *Capability) { c.Materials[0].LaserCutting.MinThickness = mm(30) }, "capability.materials[0].laser_cutting.min_thickness"},
		{"thickness out of range", func(c *Capability) { c.Materials[0].LaserCutting.Thicknesses = []measure.Length{mm(25)} }, "capability.materials[0].laser_cutting.thicknesses[0]"},
		{"thicknesses out of order", func(c *Capability) { c.Materials[0].LaserCutting.Thicknesses = []measure.Length{mm(3), mm(1.5)} }, "capability.materials[0].laser_cutting.thicknesses[1]"},
		{"range out of order", func(c *Capability) {
			c.Materials[0].LaserCutting.ThicknessRanges = []ThicknessRange{{MaxThickness: mm(6)}, {MaxThickness: mm(3)}}
		}, "capability.materials[0].laser_cutting.thickness_ranges[1].max_thickness"},
		{"range area zero", func(c *Capability) {
			c.Materials[0].LaserCutting.ThicknessRanges = []ThicknessRange{{MaxThickness: mm(6), UsableArea: &UsableArea{Width: mm(1000)}}}
		}, "capability.materials[0].laser_cutting.thickness_ranges[0].usable_area.height"},
		{"hole op code", func(c *Capability) { c.HoleOperations[0].Code = "M4!" }, "capability.hole_operations[0].code"},
		{"hole op type", func(c *Capability) { c.HoleOperations[0].Type = "rivet" }, "capability.hole_operations[0].type"},
		{"tap without thread", func(c *Capability) { c.HoleOperations[0].Thread = nil }, "capability.hole_operations[0].thread"},
		{"tap with countersink", func(c *Capability) { c.HoleOperations[0].Countersink = &Countersink{} }, "capability.hole_operations[0].countersink"},
		{"hole op unknown material", func(c *Capability) { c.HoleOperations[0].Materials[0].MaterialCode = "brass" }, "capability.hole_operations[0].materials[0].material_code"},
		{"hole op max under min", func(c *Capability) { z := measure.Length(0); c.HoleOperations[0].Materials[0].MaxThickness = &z }, "capability.hole_operations[0].materials[0].max_thickness"},
	}
	for _, tt := range tests {
		c := laser()
		tt.mut(&c)
		err := Validate(c)
		if err == nil {
			t.Errorf("%s: accepted", tt.name)
		} else if err.Param != tt.param {
			t.Errorf("%s: param %q, want %q (%s)", tt.name, err.Param, tt.param, err.Message)
		}
	}
	if err := Validate(laser()); err != nil {
		t.Errorf("valid laser capability rejected: %v", err)
	}

	bending := []struct {
		name  string
		mut   func(c *Capability)
		param string
	}{
		{"no bending block", func(c *Capability) { c.Bending = nil }, "capability.bending"},
		{"zero bend length", func(c *Capability) { c.Bending.MaxBendLength = 0 }, "capability.bending.max_bend_length"},
		{"min bend over max", func(c *Capability) { c.Bending.MinBendLength = mm(4000) }, "capability.bending.min_bend_length"},
		{"negative force", func(c *Capability) { c.Bending.MaxForce = -1 }, "capability.bending.max_force"},
		{"min angle over max", func(c *Capability) { c.Bending.MinAngle, c.Bending.MaxAngle = 120_000_000, 90_000_000 }, "capability.bending.min_angle"},
		{"k factor over half", func(c *Capability) { c.Bending.KFactor = 600_000 }, "capability.bending.k_factor"},
		{"radius with kerf term", func(c *Capability) { c.Bending.InsideRadius = &measure.Threshold{KerfRatio: 1} }, "capability.bending.inside_radius"},
		{"tooling tip angle", func(c *Capability) { c.Bending.Tooling = &BrakeTooling{} }, "capability.bending.tooling.punch.tip_angle"},
		{"tooling die opening", func(c *Capability) {
			c.Bending.Tooling = &BrakeTooling{PunchTipAngle: 88_000_000, PunchWidth: mm(12), PunchHeight: mm(120), DieWidth: mm(30), DieHeight: mm(60)}
		}, "capability.bending.tooling.die.opening"},
		{"gooseneck half given", func(c *Capability) {
			c.Bending.Tooling = &BrakeTooling{PunchTipAngle: 88_000_000, PunchWidth: mm(12), PunchHeight: mm(120), DieWidth: mm(30), DieHeight: mm(60), DieOpening: measure.Threshold{ThicknessRatio: 8_000_000}, PunchThroatDepth: mm(20)}
		}, "capability.bending.tooling.punch.gooseneck"},
		{"empty flange limit", func(c *Capability) { c.Bending.Limits.FlangeLength = &BendLimit{} }, "capability.bending.limits.flange_length"},
		{"no material block", func(c *Capability) { c.Materials[0].Bending = nil }, "capability.materials[0].bending"},
		{"material negative force factor", func(c *Capability) { c.Materials[0].Bending.ForceFactor = -1 }, "capability.materials[0].bending.force_factor"},
		{"material bad k factor", func(c *Capability) { c.Materials[0].Bending.KFactor = 900_000 }, "capability.materials[0].bending.k_factor"},
		{"material empty relief limit", func(c *Capability) { c.Materials[0].Bending.Overrides.ReliefWidth = &BendLimit{} }, "capability.materials[0].bending.limits.relief_width"},
	}
	for _, tt := range bending {
		c := brake()
		tt.mut(&c)
		err := Validate(c)
		if err == nil {
			t.Errorf("%s: accepted", tt.name)
		} else if err.Param != tt.param {
			t.Errorf("%s: param %q, want %q (%s)", tt.name, err.Param, tt.param, err.Message)
		}
	}
	if err := Validate(brake()); err != nil {
		t.Errorf("valid bending capability rejected: %v", err)
	}
}

// TestValidateAssignments checks hole and bend assignments against the catalog and on their own.
func TestValidateAssignments(t *testing.T) {
	c := laser()
	for _, tt := range []struct {
		name  string
		a     HoleAssignment
		param string
	}{
		{"unknown operation", HoleAssignment{Operation: "tap_m5", Diameter: mm(4.2)}, "hole_operations[0].operation"},
		{"neither entities nor diameter", HoleAssignment{Operation: "tap_m4"}, "hole_operations[0].holes"},
		{"both entities and diameter", HoleAssignment{Operation: "tap_m4", EntityIDs: []string{"a"}, Diameter: mm(3.3)}, "hole_operations[0].holes"},
		{"negative part", HoleAssignment{Operation: "tap_m4", Diameter: mm(3.3), Part: -1}, "hole_operations[0].holes.part"},
		{"bad side", HoleAssignment{Operation: "tap_m4", Diameter: mm(3.3), Side: "left"}, "hole_operations[0].side"},
	} {
		err := ValidateHoleAssignments(c, []HoleAssignment{tt.a})
		if err == nil || err.Param != tt.param {
			t.Errorf("%s: %v, want param %q", tt.name, err, tt.param)
		}
	}
	if err := ValidateHoleAssignments(c, []HoleAssignment{{Operation: "tap_m4", Diameter: mm(3.3)}}); err != nil {
		t.Errorf("valid assignment rejected: %v", err)
	}
	for _, tt := range []struct {
		name  string
		as    []BendAssignment
		param string
	}{
		{"no entities", []BendAssignment{{Angle: 90_000_000}}, "bends[0].entity_ids"},
		{"entity twice", []BendAssignment{{EntityIDs: []string{"a"}}, {EntityIDs: []string{"a"}}}, "bends[1].entity_ids[0]"},
		{"angle over 180", []BendAssignment{{EntityIDs: []string{"a"}, Angle: 200_000_000}}, "bends[0].angle"},
		{"bad direction", []BendAssignment{{EntityIDs: []string{"a"}, Direction: "sideways"}}, "bends[0].direction"},
		{"negative radius", []BendAssignment{{EntityIDs: []string{"a"}, InsideRadius: -1}}, "bends[0].inside_radius"},
	} {
		err := ValidateBendAssignments(tt.as)
		if err == nil || err.Param != tt.param {
			t.Errorf("%s: %v, want param %q", tt.name, err, tt.param)
		}
	}
}
