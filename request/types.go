package request

import (
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/field"
)

// CapabilitySchemaVersion is the only capability schema version accepted.
const CapabilitySchemaVersion = "3.0"

// QuantityInput is an amount together with the unit it is expressed in.
type QuantityInput struct {
	// Decimal value, as a string to preserve precision.
	Value string `json:"value" validate:"required" format:"decimal"`
	// Unit the value is expressed in.
	Unit constants.LengthUnit `json:"unit" validate:"required"`
}

// AreaQuantityInput is an area together with the unit it is expressed in.
type AreaQuantityInput struct {
	// Decimal value, as a string to preserve precision.
	Value string `json:"value" validate:"required" format:"decimal"`
	// Unit the value is expressed in.
	Unit constants.AreaUnit `json:"unit" validate:"required"`
}

// AngleInput is an angle together with the unit it is expressed in.
type AngleInput struct {
	// Decimal value, as a string to preserve precision.
	Value string `json:"value" validate:"required" format:"decimal"`
	// Unit the value is expressed in.
	Unit constants.AngleUnit `json:"unit" validate:"required"`
}

// CheckRequest asks whether a design can be manufactured with a capability and stock. The design file itself travels separately (e.g. as a multipart file); this carries everything else.
type CheckRequest struct {
	// How to read the design file.
	Design DesignInput `json:"design" validate:"required"`
	// The job's raw material.
	Stock StockInput `json:"stock" validate:"required"`
	// What the manufacturing setup can produce.
	Capability CapabilityInput `json:"capability" validate:"required"`
	// Operations (taps, countersinks, press-in fasteners) assigned to round holes of the design, from the capability's hole_operations. Each hole takes at most one.
	HoleOperations field.Optional[[]HoleAssignmentInput] `json:"hole_operations,omitzero"`
	// Angle, direction and inside radius of the design's bend lines, for a bending check; a bend line not listed is bent 90 degrees up at the brake's inside radius.
	Bends field.Optional[[]BendAssignmentInput] `json:"bends,omitzero"`
}

// HoleAssignmentInput assigns one operation to some of the design's round holes. The holes are resized to the operation's hole_diameter and checked at that size.
type HoleAssignmentInput struct {
	// Code of an operation in the capability's hole_operations.
	Operation string `json:"operation" validate:"required"`
	// Which holes.
	Holes HoleSelectorInput `json:"holes" validate:"required"`
	// Face the countersink opens on or the fastener is pressed in from: top (toward the viewer of the drawing) or bottom. Required for countersinks and fasteners; ignored for taps.
	Side field.Optional[constants.HoleSide] `json:"side,omitzero"`
}

// HoleSelectorInput selects round holes by their entities or by drawn diameter (exactly one of the two), optionally within one part.
type HoleSelectorInput struct {
	// Entities of the holes, as reported in the result's holes.
	EntityIDs field.Optional[[]string] `json:"entity_ids,omitzero"`
	// Every round hole drawn at this diameter, as reported in the result's holes.
	Diameter field.Optional[QuantityInput] `json:"diameter,omitzero"`
	// Only holes in this part number.
	Part field.Optional[int] `json:"part,omitzero"`
}

// DesignInput describes the design file.
type DesignInput struct {
	// Format of the design file.
	Format constants.DesignFormat `json:"format" validate:"required"`
	// Overrides the drawing units declared in the file; required for files that declare none.
	Units field.Optional[constants.LengthUnit] `json:"units,omitzero"`
	// Changes to how the drawing is read, accepted from the fix of a previous result's occurrences.
	Adjustments field.Optional[[]AdjustmentInput] `json:"adjustments,omitzero"`
}

// AdjustmentInput is an accepted fix: a change to how the drawing is read.
type AdjustmentInput struct {
	// What the fix does to how the drawing is read, as the result's fix gave it.
	Action constants.FixAction `json:"action" validate:"required"`
	// Entities it applies to, for mark_bend_line and remove.
	EntityIDs field.Optional[[]string] `json:"entity_ids,omitzero"`
	// Units to read the drawing in, for set_units.
	Units field.Optional[constants.LengthUnit] `json:"units,omitzero"`
}

// StockInput is the material and size of the job's raw material.
type StockInput struct {
	// Code of a material listed in the capability.
	MaterialCode string `json:"material_code" validate:"required"`
	// Shape the material comes in; sheet when omitted. Processes to come add forms, each with its own fields.
	Form field.Optional[constants.StockForm] `json:"form,omitzero" default:"sheet"`
	// Thickness of the sheet; required for sheet stock.
	Thickness field.Optional[QuantityInput] `json:"thickness,omitzero"`
}

// CapabilityInput describes what a manufacturing setup can produce, independent of any one job.
type CapabilityInput struct {
	// Version of the capability schema.
	SchemaVersion string `json:"schema_version" validate:"required" const:"3.0"`
	// Manufacturing process; the block of the same name is required.
	Process constants.Process `json:"process" validate:"required"`
	// Laser-cutting limits, required when process is laser_cutting.
	LaserCutting field.Optional[LaserCuttingInput] `json:"laser_cutting,omitzero"`
	// Press-brake limits, required when process is bending.
	Bending field.Optional[BendingInput] `json:"bending,omitzero"`
	// Every material the setup can process.
	Materials []MaterialInput `json:"materials" validate:"required" minItems:"1"`
	// Taps, countersinks and press-in fasteners the setup offers, which a check request can assign to holes.
	HoleOperations field.Optional[[]HoleOperationInput] `json:"hole_operations,omitzero"`
}

// HoleOperationInput is one operation offered on round holes.
type HoleOperationInput struct {
	// Identifies the operation in a check request's hole_operations (lowercase letters, digits, '_', '.', '-').
	Code string `json:"code" validate:"required"`
	// What the operation does to the hole; hardware is a press-in (self-clinching) fastener. It says which of thread, countersink and hardware apply.
	Type constants.HoleOperationType `json:"type" validate:"required"`
	// Human-readable name, e.g. "M4 x 0.7 tap".
	Name string `json:"name" validate:"required"`
	// The thread a tap cuts or a fastener carries; required for taps.
	Thread field.Optional[ThreadInput] `json:"thread,omitzero"`
	// The cone a countersink cuts; required for countersinks.
	Countersink field.Optional[CountersinkInput] `json:"countersink,omitzero"`
	// The fastener; required for hardware.
	Hardware field.Optional[HardwareInput] `json:"hardware,omitzero"`
	// Hole the operation needs (tap drill, countersink minor diameter, fastener mounting hole); assigned holes are resized to it.
	HoleDiameter QuantityInput `json:"hole_diameter" validate:"required"`
	// Sheet materials the operation is offered for, each with a thickness range.
	Materials []HoleOperationMaterialInput `json:"materials" validate:"required" minItems:"1"`
	// Least distance from the hole's center to any other cut (part edge, cut-out, other hole).
	EdgeDistance field.Optional[LimitInput] `json:"edge_distance,omitzero"`
	// Least distance between the centers of two holes with operations; the larger of the two operations' applies.
	Spacing field.Optional[LimitInput] `json:"spacing,omitzero"`
	// Least distance from the hole's edge to a bend line, which would distort the thread, countersink or fastener; checked by the plan's step for this operation when the design has bend lines. Not checked when omitted.
	BendDistance field.Optional[LimitInput] `json:"bend_distance,omitzero"`
}

// ThreadInput is a screw thread.
type ThreadInput struct {
	// Nominal (major) diameter.
	NominalDiameter QuantityInput `json:"nominal_diameter" validate:"required"`
	// Distance between threads.
	Pitch QuantityInput `json:"pitch" validate:"required"`
}

// CountersinkInput is the cone a countersink cuts.
type CountersinkInput struct {
	// Included angle of the cone, e.g. 90 (metric) or 82 (imperial) degrees.
	Angle AngleInput `json:"angle" validate:"required"`
	// Diameter of the cone at the face.
	MajorDiameter QuantityInput `json:"major_diameter" validate:"required"`
	// Deepest the cone may go, as a share of the thickness, as a decimal string (e.g. "0.6").
	MaxDepthRatio string `json:"max_depth_ratio" validate:"required" format:"decimal"`
}

// HardwareInput is a press-in fastener.
type HardwareInput struct {
	// What the fastener is, e.g. nut, stud or standoff.
	Type string `json:"type" validate:"required"`
}

// HoleOperationMaterialInput is a sheet material an operation is offered for.
type HoleOperationMaterialInput struct {
	// Code of a material listed in the capability.
	MaterialCode string `json:"material_code" validate:"required"`
	// Thinnest stock the operation is offered for.
	MinThickness QuantityInput `json:"min_thickness" validate:"required"`
	// Thickest stock it is offered for; omit for no limit.
	MaxThickness field.Optional[QuantityInput] `json:"max_thickness,omitzero"`
	// Overrides the operation's edge_distance for this material.
	EdgeDistance field.Optional[LimitInput] `json:"edge_distance,omitzero"`
}

// LaserCuttingInput holds the laser-cutting limits of a setup.
type LaserCuttingInput struct {
	// Area the head can cut in one setup (not the nominal sheet or bed size). Parts may rotate 90°.
	UsableArea AreaInput `json:"usable_area" validate:"required"`
	// Default width of material removed by the beam.
	KerfWidth QuantityInput `json:"kerf_width" validate:"required"`
	// Gaps between path ends up to this size (at most 1 mm) are closed before checking and reported as warnings. Omit it to close none.
	ContourClosureTolerance field.Optional[QuantityInput] `json:"contour_closure_tolerance,omitzero"`
	// Distance between the bed's supports (slats or pins). A part shorter than this can drop between them or tip into the beam, and is warned about; not checked when omitted.
	SupportSpacing field.Optional[QuantityInput] `json:"support_spacing,omitzero"`
	// How a drawing with more than one part is treated: rejected (one part per drawing), accepted (parts side by side are checked separately; the default), or accepted_in_cutouts (parts nested inside another part's cut-outs are too).
	MultipleParts field.Optional[constants.MultipleParts] `json:"multiple_parts,omitzero"`
	// What shapes of a color in an SVG drawing are for: cut, engrave or ignore (construction). A shape's color is its stroke, or its fill when it has no stroke. Colors not listed follow the defaults: stroked shapes are cut and filled_shapes decides the rest.
	ColorOperations field.Optional[[]ColorOperationInput] `json:"color_operations,omitzero"`
	// What shapes filled but not stroked in an SVG drawing are for: engrave (area engraving; the default), cut (along their outline) or ignore.
	FilledShapes field.Optional[constants.ShapeOperation] `json:"filled_shapes,omitzero"`
	// What SVG shapes lying wholly outside the page (the viewBox) are for: cut (the default, as services that ignore the artboard do) or ignore (as when a drawing is printed to the laser). Either way they are reported as geometry_outside_page.
	OutsidePage field.Optional[constants.OutsidePage] `json:"outside_page,omitzero"`
	// Whether parts may share a cut line (common-line cutting): rejected (the default; each part needs its own outline) or accepted (the shared line is cut once). Either way each shared line is reported as geometry_common_line.
	CommonLine field.Optional[constants.CommonLine] `json:"common_line,omitzero"`
	// Units drawings may be in; omit to accept any. A drawing in other units is units_unsupported.
	DrawingUnits field.Optional[[]constants.LengthUnit] `json:"drawing_units,omitzero"`
	// Default feature limits.
	Limits LaserCuttingLimitsInput `json:"limits" validate:"required"`
}

// AreaInput is a rectangular area.
type AreaInput struct {
	Width  QuantityInput `json:"width" validate:"required"`
	Height QuantityInput `json:"height" validate:"required"`
}

// LaserCuttingLimitsInput holds the laser-cutting feature limits.
type LaserCuttingLimitsInput struct {
	// Minimum diameter of circular holes.
	HoleDiameter LimitInput `json:"hole_diameter" validate:"required"`
	// Minimum width of recognized slots.
	SlotWidth LimitInput `json:"slot_width" validate:"required"`
	// Minimum material between two cut-outs in the same region; not checked when omitted.
	WebWidth field.Optional[LimitInput] `json:"web_width,omitzero"`
	// Minimum gap between two cut-outs, as a spacing rule; not checked when omitted.
	FeatureSpacing field.Optional[LimitInput] `json:"feature_spacing,omitzero"`
	// Minimum material between a cut-out and the edge around it; not checked when omitted.
	EdgeDistance field.Optional[LimitInput] `json:"edge_distance,omitzero"`
	// Minimum width of the part in its narrowest direction; not checked when omitted.
	PartWidth field.Optional[LimitInput] `json:"part_width,omitzero"`
	// Minimum length of the part at right angles to its narrowest width; with part_width it sets a minimum part size; not checked when omitted.
	PartLength field.Optional[LimitInput] `json:"part_length,omitzero"`
	// Minimum area of the finished part: its outline less its cut-outs; not checked when omitted.
	PartArea field.Optional[AreaLimitInput] `json:"part_area,omitzero"`
	// Minimum part width that rises with the part's length, as tiers by increasing min_length (long narrow strips warp); not checked when omitted.
	SlenderPart field.Optional[[]SlenderTierInput] `json:"slender_part,omitzero"`
	// Minimum cut-out width that rises with the cut-out's length, as tiers by increasing min_length (long narrow cut-outs warp the part); round holes are not checked; not checked when omitted.
	SlenderCutout field.Optional[[]SlenderTierInput] `json:"slender_cutout,omitzero"`
	// Minimum width of any passage within a cut-out or edge notch (slots and round holes use their own limits); not checked when omitted.
	CutoutWidth field.Optional[LimitInput] `json:"cutout_width,omitzero"`
	// Minimum width of any strip of material within one outline, such as a finger, tab or tongue; not checked when omitted.
	FeatureWidth field.Optional[LimitInput] `json:"feature_width,omitzero"`
	// Minimum radius of an inside corner of the material (a sharp corner is 0); not checked when omitted.
	InternalCornerRadius field.Optional[LimitInput] `json:"internal_corner_radius,omitzero"`
	// Minimum clearance of a slot that receives the stock, as in tab-and-slot parts: its width less the stock thickness, for slots within 25% of the thickness wide (a slot as wide as the stock has no clearance); not checked when omitted.
	SlotFitClearance field.Optional[LimitInput] `json:"slot_fit_clearance,omitzero"`
	// Warn when cut-outs remove at least this percentage of a part's area (a whole number as a decimal string, above 0 and at most 100); not checked when omitted.
	MaxRemovedAreaPercent field.Optional[string] `json:"max_removed_area_percent,omitzero" format:"decimal"`
	// Also apply max_removed_area_percent in every square window this size across each part, to find a dense patch on a large part; needs max_removed_area_percent.
	RemovedAreaWindow field.Optional[QuantityInput] `json:"removed_area_window,omitzero"`
	// Warn about corners that come to a sharper point than this angle, on the material or the cut-out side (greater than 0° and less than 180°); not checked when omitted.
	MinTipAngle field.Optional[AngleInput] `json:"min_tip_angle,omitzero"`
}

// MaterialLaserCuttingLimitsInput overrides individual default limits; each replaces the default of the same name as a whole.
type MaterialLaserCuttingLimitsInput struct {
	HoleDiameter field.Optional[LimitInput] `json:"hole_diameter,omitzero"`
	SlotWidth    field.Optional[LimitInput] `json:"slot_width,omitzero"`
	// Minimum material between two cut-outs in the same region; not checked when omitted.
	WebWidth field.Optional[LimitInput] `json:"web_width,omitzero"`
	// Minimum gap between two cut-outs, as a spacing rule; not checked when omitted.
	FeatureSpacing field.Optional[LimitInput] `json:"feature_spacing,omitzero"`
	// Minimum material between a cut-out and the edge around it; not checked when omitted.
	EdgeDistance field.Optional[LimitInput] `json:"edge_distance,omitzero"`
	// Minimum width of the part in its narrowest direction; not checked when omitted.
	PartWidth field.Optional[LimitInput] `json:"part_width,omitzero"`
	// Minimum length of the part at right angles to its narrowest width; with part_width it sets a minimum part size; not checked when omitted.
	PartLength field.Optional[LimitInput] `json:"part_length,omitzero"`
	// Minimum area of the finished part: its outline less its cut-outs; not checked when omitted.
	PartArea field.Optional[AreaLimitInput] `json:"part_area,omitzero"`
	// Minimum part width that rises with the part's length, as tiers by increasing min_length (long narrow strips warp); not checked when omitted.
	SlenderPart field.Optional[[]SlenderTierInput] `json:"slender_part,omitzero"`
	// Minimum cut-out width that rises with the cut-out's length, as tiers by increasing min_length (long narrow cut-outs warp the part); round holes are not checked; not checked when omitted.
	SlenderCutout field.Optional[[]SlenderTierInput] `json:"slender_cutout,omitzero"`
	// Minimum width of any passage within a cut-out or edge notch (slots and round holes use their own limits); not checked when omitted.
	CutoutWidth field.Optional[LimitInput] `json:"cutout_width,omitzero"`
	// Minimum width of any strip of material within one outline, such as a finger, tab or tongue; not checked when omitted.
	FeatureWidth field.Optional[LimitInput] `json:"feature_width,omitzero"`
	// Minimum radius of an inside corner of the material (a sharp corner is 0); not checked when omitted.
	InternalCornerRadius field.Optional[LimitInput] `json:"internal_corner_radius,omitzero"`
	// Minimum clearance of a slot that receives the stock, as in tab-and-slot parts: its width less the stock thickness, for slots within 25% of the thickness wide (a slot as wide as the stock has no clearance); not checked when omitted.
	SlotFitClearance field.Optional[LimitInput] `json:"slot_fit_clearance,omitzero"`
	// Warn when cut-outs remove at least this percentage of a part's area (a whole number as a decimal string, above 0 and at most 100); not checked when omitted.
	MaxRemovedAreaPercent field.Optional[string] `json:"max_removed_area_percent,omitzero" format:"decimal"`
	// Also apply max_removed_area_percent in every square window this size across each part, to find a dense patch on a large part; needs max_removed_area_percent.
	RemovedAreaWindow field.Optional[QuantityInput] `json:"removed_area_window,omitzero"`
	// Warn about corners that come to a sharper point than this angle, on the material or the cut-out side (greater than 0° and less than 180°); not checked when omitted.
	MinTipAngle field.Optional[AngleInput] `json:"min_tip_angle,omitzero"`
}

// AreaLimitInput is a minimum area with an error tier and/or a warning tier.
type AreaLimitInput struct {
	// Below this the part cannot be made.
	Error field.Optional[AreaQuantityInput] `json:"error,omitzero"`
	// Below this the part can be made with reduced quality.
	Warning field.Optional[AreaQuantityInput] `json:"warning,omitzero"`
}

// LimitInput is a minimum dimension with an error tier and/or a warning tier.
type LimitInput struct {
	// Below this the feature cannot be made.
	Error field.Optional[ThresholdInput] `json:"error,omitzero"`
	// Below this the feature can be made with reduced quality.
	Warning field.Optional[ThresholdInput] `json:"warning,omitzero"`
}

// ThresholdInput requires max(min, thickness_ratio × stock thickness, kerf_ratio × kerf width), capped at max when it is given. Omitted terms are 0; at least one of min, thickness_ratio and kerf_ratio is required.
type ThresholdInput struct {
	// Absolute minimum.
	Min field.Optional[QuantityInput] `json:"min,omitzero"`
	// Multiple of stock thickness, as a decimal string.
	ThicknessRatio field.Optional[string] `json:"thickness_ratio,omitzero" format:"decimal"`
	// Multiple of kerf width, as a decimal string.
	KerfRatio field.Optional[string] `json:"kerf_ratio,omitzero" format:"decimal"`
	// Upper cap on the requirement, e.g. the smaller of twice the thickness and 3.175 mm; at least min.
	Max field.Optional[QuantityInput] `json:"max,omitzero"`
}

// MaterialInput is one material a setup can process.
type MaterialInput struct {
	// Identifies the material in a job's stock (lowercase letters, digits, '_', '.', '-').
	Code string `json:"code" validate:"required"`
	// Laser-cutting limits for this material, required when the process is laser_cutting.
	LaserCutting field.Optional[MaterialLaserCuttingInput] `json:"laser_cutting,omitzero"`
	// Bending limits for this material, required when the process is bending.
	Bending field.Optional[MaterialBendingInput] `json:"bending,omitzero"`
}

// MaterialLaserCuttingInput holds a material's laser-cutting limits.
type MaterialLaserCuttingInput struct {
	// Thickest stock the setup can cut in this material.
	MaxThickness QuantityInput `json:"max_thickness" validate:"required"`
	// Thinnest stock the setup can cut in this material; not checked when omitted.
	MinThickness field.Optional[QuantityInput] `json:"min_thickness,omitzero"`
	// The thicknesses stocked, increasing; a job's thickness must be one of them (within 0.001 mm). Any thickness in range is accepted when omitted.
	Thicknesses field.Optional[[]QuantityInput] `json:"thicknesses,omitzero"`
	// Overrides the default kerf width.
	KerfWidth field.Optional[QuantityInput] `json:"kerf_width,omitzero"`
	// Overrides the default usable area for this material.
	UsableArea field.Optional[AreaInput] `json:"usable_area,omitzero"`
	// Overrides individual default limits.
	Limits field.Optional[MaterialLaserCuttingLimitsInput] `json:"limits,omitzero"`
	// Limits, kerf and usable area that depend on the stock thickness, like a published table. Ranges are listed by increasing max_thickness; the first that the stock fits within overrides the material's kerf, usable area and limits.
	ThicknessRanges field.Optional[[]ThicknessRangeInput] `json:"thickness_ranges,omitzero"`
}

// ThicknessRangeInput holds a material's kerf and limits for stock up to a thickness.
type ThicknessRangeInput struct {
	// Thickest stock the range applies to (inclusive); thinner stock falls in the first range that fits.
	MaxThickness QuantityInput `json:"max_thickness" validate:"required"`
	// Overrides the kerf width.
	KerfWidth field.Optional[QuantityInput] `json:"kerf_width,omitzero"`
	// Overrides the usable area, e.g. a smaller area for thick plate.
	UsableArea field.Optional[AreaInput] `json:"usable_area,omitzero"`
	// Overrides individual limits.
	Limits field.Optional[MaterialLaserCuttingLimitsInput] `json:"limits,omitzero"`
}

// ColorOperationInput maps one color to an operation.
type ColorOperationInput struct {
	// Color written #rrggbb.
	Color string `json:"color" validate:"required"`
	// What shapes of the color are for.
	Operation constants.ShapeOperation `json:"operation" validate:"required"`
}

// SlenderTierInput is a minimum width for parts at least min_length long.
type SlenderTierInput struct {
	// Parts at least this long (and shorter than the next tier's) must meet width.
	MinLength QuantityInput `json:"min_length" validate:"required"`
	// Minimum part width for them.
	Width LimitInput `json:"width" validate:"required"`
}

// PlanRequest asks for the operations that make a design (laser cutting, then bending when the drawing has bend lines), each checked against the capability for its process; carries everything but the design file.
type PlanRequest struct {
	// How to read the design file.
	Design DesignInput `json:"design" validate:"required"`
	// The job's raw material.
	Stock StockInput `json:"stock" validate:"required"`
	// The machines available, at most one per process, in any order (e.g. a laser cutter and a press brake). An operation whose process has none is not checked.
	Capabilities []CapabilityInput `json:"capabilities" validate:"required" minItems:"1"`
	// Operations assigned to round holes, for the laser-cutting step.
	HoleOperations field.Optional[[]HoleAssignmentInput] `json:"hole_operations,omitzero"`
	// Angle, direction and inside radius of the design's bend lines, for the bending step; a bend line not listed is bent 90 degrees up at the brake's inside radius.
	Bends field.Optional[[]BendAssignmentInput] `json:"bends,omitzero"`
}
