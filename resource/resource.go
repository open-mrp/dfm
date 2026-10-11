// Package resource defines the JSON response resources and presents domain results as them. Fields are never omitted: absent values are null.
package resource

import (
	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/constants"
)

// Quantity is a measured amount: a value together with its unit. It is a value shape, part of its parent, so it has no object; formatting it is the client's job.
type Quantity struct {
	// Exact decimal value, as a string to preserve precision.
	Value string `json:"value" validate:"required" format:"decimal"`
	// Unit the value is expressed in.
	Unit constants.Unit `json:"unit" validate:"required"`
}

// Point is a location in the drawing: a value shape, part of its parent, with no object.
type Point struct {
	// X coordinate, as a decimal string.
	X string `json:"x" validate:"required" format:"decimal"`
	// Y coordinate, as a decimal string.
	Y string `json:"y" validate:"required" format:"decimal"`
	// Unit of both coordinates.
	Unit constants.LengthUnit `json:"unit" validate:"required"`
}

// PageInfo describes the position of a list page: the URLs of its neighbors, null where there is none.
type PageInfo struct {
	// Relative URL that fetches the next page of results.
	NextPageURL *string `json:"next_page_url"`
	// Relative URL that fetches the previous page of results.
	PreviousPageURL *string `json:"previous_page_url"`
}

// List is an array of resources.
type List[T any] struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=list" const:"list"`
	// Position of this page; empty for embedded lists.
	PageInfo PageInfo `json:"page_info" validate:"required"`
	// The resources.
	Data []T `json:"data" validate:"required"`
}

// NewList wraps items in a list; data is never null.
func NewList[T any](items []T, page PageInfo) *List[T] {
	if items == nil {
		items = []T{}
	}
	return &List[T]{Object: constants.ObjectTypeList, PageInfo: page, Data: items}
}

// Issue is one kind of problem (a finding code at one severity) with every place it occurs. What is the same for every occurrence is stated once.
type Issue struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_issue" const:"dfm_issue"`
	// What was found.
	Code constants.FindingCode `json:"code" validate:"required"`
	// Whether it prevents manufacture (error), needs a person's judgment before the part is made (review) or only reduces quality (warning).
	Severity constants.Severity `json:"severity" validate:"required"`
	// A short sentence for the problem; some codes vary it by case (what a bend ends at, which tool a bend runs into). Values are in the occurrences.
	Message string `json:"message" validate:"required"`
	// How to fix the design, fixed per code; the values it refers to (actual, required, location) are in this issue and its occurrences.
	Remediation string `json:"remediation" validate:"required"`
	// Limit that was not met, exactly as computed from the capability; null for issues without a limit.
	Required *Quantity `json:"required"`
	// Term of the limit that set required.
	RequiredBasis *constants.ThresholdBasis `json:"required_basis"`
	// Every place the problem occurs.
	Occurrences *List[Occurrence] `json:"occurrences" validate:"required"`
}

// Occurrence is one place a problem occurs.
type Occurrence struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_occurrence" const:"dfm_occurrence"`
	// Identifies the occurrence. Derived from its content, so it is the same whenever the same design is checked, including on a newer engine that reports it unchanged; an occurrence between two parts (such as a shared edge) is listed under both with the same ID.
	ID string `json:"id" validate:"required"`
	// Source entities involved, e.g. DXF handles (or "#n" for the n-th entity of a file without handles; "insert/entity" inside a block reference).
	EntityIDs []string `json:"entity_ids"`
	// Measured value, rounded to the nearest nanometer; null where nothing is measured.
	Actual *Quantity `json:"actual"`
	// Representative point of the problem in the drawing.
	Location *Point `json:"location"`
	// The two ends of a measured width or gap (a web, a spacing, a narrow passage or strip), for findings that measure one; empty otherwise.
	Span []Point `json:"span" validate:"required"`
	// A change to how the drawing is read that resolves this occurrence, for the customer to accept: send it back as one of design.adjustments. Null when there is none.
	Fix *Fix `json:"fix"`
}

// Fix is a suggested change to how the drawing is read: a section of its occurrence, with no object. Accept it by sending its action, entity_ids and units back as one of the check request's design.adjustments.
type Fix struct {
	// mark_bend_line (read the entities as bend lines, not cut), remove (leave the entities out of the drawing) or set_units (read the drawing in units).
	Action constants.FixAction `json:"action" validate:"required"`
	// Entities the fix applies to; empty for set_units.
	EntityIDs []string `json:"entity_ids"`
	// Units to read the drawing in, for set_units; null otherwise.
	Units *constants.LengthUnit `json:"units"`
}

// Part is one part of the drawing: an outer boundary and everything inside it.
type Part struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_part" const:"dfm_part"`
	// 1-based number, in drawing order, of the part's first instance.
	Number int `json:"number" validate:"required"`
	// How many identical copies of the part the drawing holds: the same cut shape, moved or rotated but not mirrored, with the same problems.
	CopyCount int `json:"copy_count" validate:"required"`
	// Every copy, the first being this part as described. Issues are located on the first copy; a copy's placement maps a location onto it.
	Instances *List[PartInstance] `json:"instances" validate:"required"`
	// Entities of the part's outer boundary (the open paths, when it is not closed); empty when the drawing has none.
	EntityIDs []string `json:"entity_ids"`
	// The part as a cut sheet: its outline, size, cut length, holes and bends. Null for a process whose parts are not sheets.
	Sheet *SheetPart `json:"sheet"`
	// manufacturable, needs_review (review findings and no errors: send the part to a person) or not_manufacturable (an error), from the findings about this part; findings about the drawing as a whole are not counted.
	Verdict constants.Verdict `json:"verdict" validate:"required"`
	// Number of error occurrences in this part's issues, for one copy.
	ErrorCount int `json:"error_count"`
	// Number of review occurrences: problems a person must judge before the part is made.
	ReviewCount int `json:"review_count"`
	// Number of warning occurrences in this part's issues, for one copy.
	WarningCount int `json:"warning_count"`
	// Every problem with this part, in check order. A problem between this part and another (such as a shared edge) is listed under both.
	Issues *List[Issue] `json:"issues" validate:"required"`
}

// SheetPart is a part as a shape cut from sheet, what the laser-cutting and bending processes see: a section of its part, with no object.
type SheetPart struct {
	// Whether the outline closes; the part's own checks (holes, widths, ...) run once it does.
	Outline constants.Outline `json:"outline" validate:"required"`
	// Width of the part's bounding box in the drawing.
	Width *Quantity `json:"width" validate:"required"`
	// Height of the part's bounding box in the drawing.
	Height *Quantity `json:"height" validate:"required"`
	// Area of the part: its outline less its cut-outs (0 when the outline is not closed).
	Area *Quantity `json:"area" validate:"required"`
	// Length cut to make one copy: every path and circle, measured along arcs.
	CutLength *Quantity `json:"cut_length" validate:"required"`
	// Times the beam pierces the sheet to cut one copy: once per path and circle.
	PierceCount int `json:"pierce_count"`
	// The part's round holes (circles, and polygons with their corners on a circle), grouped by drawn diameter and assigned operation, with the operations each group can take; for one copy.
	Holes *List[HoleGroup] `json:"holes" validate:"required"`
	// The part's bend lines as the bending check bent them, in drawing order; empty for other processes.
	Bends *List[Bend] `json:"bends" validate:"required"`
}

// Bend is a bend line as the bending check bent it.
type Bend struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_bend" const:"dfm_bend"`
	// Entity of the bend line; a request's bends name it.
	EntityID string `json:"entity_id" validate:"required"`
	// Length of the bend line.
	Length *Quantity `json:"length" validate:"required"`
	// Bend angle from flat.
	Angle *Quantity `json:"angle" validate:"required"`
	// Way the flange turns: up (toward the viewer of the drawing) or down.
	Direction constants.BendDirection `json:"direction" validate:"required"`
	// Inside radius the design is drawn for.
	InsideRadius *Quantity `json:"inside_radius" validate:"required"`
	// Length of flat material the bend takes up at the radius the tooling forms: angle x (radius + k_factor x thickness). The bend zone is this wide, centered on the bend line.
	BendAllowance *Quantity `json:"bend_allowance" validate:"required"`
	// What the request did not give and was assumed: angle (90), direction (up), inside_radius (the tooling's).
	Assumed []string `json:"assumed"`
	// Where the bend line starts in the drawing; throat_side and punch_position are measured from it toward end.
	Start *Point `json:"start" validate:"required"`
	// Where the bend line ends in the drawing.
	End *Point `json:"end" validate:"required"`
	// Position of this bend, from 1, in the order the bending check found that forms every bend clear of the press brake's tooling: the bends before it are formed when it is, the ones after it are flat. Null when the capability gives no tooling or no order was found.
	Sequence *int `json:"sequence"`
	// Where along the bend line the punch stood when it formed the bend in that order: start (the punch's end at the line's start), middle (centered) or end. Null when sequence is null.
	PunchPosition *constants.PunchPosition `json:"punch_position"`
	// For a gooseneck punch, the side of the bend line, looking from start to end with the drawing seen from its viewer, that the punch's throat opened toward when it formed the bend. Null for a straight punch, or when sequence is null.
	ThroatSide *constants.LineSide `json:"throat_side"`
}

// HoleGroup is a part's round holes of one drawn diameter and operation.
type HoleGroup struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_hole_group" const:"dfm_hole_group"`
	// Diameter the holes are drawn at; a hole_operations selector can use it.
	Diameter *Quantity `json:"diameter" validate:"required"`
	// Number of holes.
	Count int `json:"count" validate:"required"`
	// Entities of the holes.
	EntityIDs []string `json:"entity_ids"`
	// Code of the hole operation assigned to them, or null.
	Operation *string `json:"operation"`
	// Codes of the capability's hole operations offered for the stock whose edge_distance every hole of the group meets.
	AvailableOperations []string `json:"available_operations"`
}

// PartInstance is one copy of a part in the drawing.
type PartInstance struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_part_instance" const:"dfm_part_instance"`
	// 1-based part number, in drawing order; findings about this copy carry it.
	Number int `json:"number" validate:"required"`
	// Entities of this copy's outer boundary.
	EntityIDs []string `json:"entity_ids"`
	// Counterclockwise rotation from the first copy, in [0, 360) degrees: a point p of the first copy is at rotation(p) + offset in this one, rotating about the drawing's origin.
	Rotation *Quantity `json:"rotation" validate:"required"`
	// Offset from the first copy, after the rotation.
	Offset *Point `json:"offset" validate:"required"`
}

// SkippedCheck is a check that did not run because the capability does not set its limit.
type SkippedCheck struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_skipped_check" const:"dfm_skipped_check"`
	// Finding code the check would report.
	Code constants.FindingCode `json:"code" validate:"required"`
	// Capability setting that is not set, as a path from the capability's root.
	Limit string `json:"limit" validate:"required"`
}

// FindingCodeInfo describes a finding code: what it means, how severe it can be and how to fix it.
type FindingCodeInfo struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_finding_code" const:"dfm_finding_code"`
	// The finding code.
	Code constants.FindingCode `json:"code" validate:"required"`
	// Severities the code can be reported with.
	Severities []constants.Severity `json:"severities" validate:"required"`
	// What findings with this code are about: the drawing as a whole (attributed to the parts they fall in) or one part.
	Scope constants.FindingScope `json:"scope" validate:"required"`
	// Capability setting that drives the check, as a path from the capability's root, or null if none.
	Limit *string `json:"limit"`
	// What the finding means.
	Description string `json:"description" validate:"required"`
	// How to fix the design.
	Remediation string `json:"remediation" validate:"required"`
}

// Check is the result of checking a design against a capability and stock.
type Check struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_check" const:"dfm_check"`
	// Process the design was checked for.
	Process constants.Process `json:"process" validate:"required"`
	// Version of the engine that produced the result; results can change between versions.
	EngineVersion string `json:"engine_version" validate:"required"`
	// manufacturable, needs_review (review findings and no errors: send the design to a person rather than rejecting it) or not_manufacturable (an error).
	Verdict constants.Verdict `json:"verdict" validate:"required"`
	// Number of error occurrences in the whole result, each counted once, including those on every copy of a part.
	ErrorCount int `json:"error_count"`
	// Number of review occurrences: problems a person must judge before the part is made.
	ReviewCount int `json:"review_count"`
	// Number of warning occurrences in the whole result, each counted once, including those on every copy of a part.
	WarningCount int `json:"warning_count"`
	// Number of parts in the drawing, counting every copy.
	PartCount int `json:"part_count"`
	// Distinct parts of the drawing, one per outer boundary with identical copies listed together, each checked separately, each with every problem it has.
	Parts *List[Part] `json:"parts" validate:"required"`
	// Problems that are in no part: the stock, the drawing as a whole, or paths outside every part.
	DrawingIssues *List[Issue] `json:"drawing_issues" validate:"required"`
	// Checks that did not run because the capability does not set their limit: a manufacturable result covers only the checks that ran.
	SkippedChecks *List[SkippedCheck] `json:"skipped_checks" validate:"required"`
}

// Plan is the operations that make a design, in the order they must happen, each checked against the capability for its process.
type Plan struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_plan" const:"dfm_plan"`
	// Version of the engine that produced the result.
	EngineVersion string `json:"engine_version" validate:"required"`
	// not_manufacturable when an operation is, incomplete when an operation was not checked (no capability for its process), needs_review when review findings keep any from passing (send the design to a person), manufacturable when every operation is.
	Verdict constants.PlanVerdict `json:"verdict" validate:"required"`
	// The operations, in order.
	Operations *List[Operation] `json:"operations" validate:"required"`
}

// Operation is one step of a plan.
type Operation struct {
	// Resource type identifier.
	Object constants.ObjectType `json:"object" validate:"required,enum=dfm_operation" const:"dfm_operation"`
	// 1-based place in the plan.
	Sequence int `json:"sequence" validate:"required"`
	// The step's process.
	Process constants.Process `json:"process" validate:"required"`
	// Why the design needs the step.
	Reason string `json:"reason" validate:"required"`
	// The step's verdict: not_checked when no capability for its process was given, otherwise its result's.
	Verdict constants.OperationVerdict `json:"verdict" validate:"required"`
	// The step's check; null when it was not checked.
	Result *Check `json:"result"`
}

// ErrorResponse is the body of every error response.
type ErrorResponse struct {
	Error *apierror.Error `json:"error"`
}
