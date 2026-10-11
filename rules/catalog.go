package rules

import (
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
)

// CodeInfo describes a finding code for people and agents: what it means, how severe it can be, what it is about, which capability limit drives it and how to fix it.
type CodeInfo struct {
	Code constants.FindingCode
	// Severities the code can be reported with.
	Severities []constants.Severity
	// Scope is what the finding is about.
	Scope constants.FindingScope
	// Limit is the capability setting that drives the check, as a path from the capability's root (e.g. "laser_cutting.limits.web_width"), or "" if none.
	Limit string
	// Description says what the finding means.
	Description string
	// Remediation says how to fix the design. Values it refers to (actual, required, location) are in the finding.
	Remediation string

	// limitSet reports whether the profile sets Limit; nil when the check always runs.
	limitSet func(*capability.Profile) bool
}

var (
	errOnly     = []constants.Severity{constants.SeverityError}
	warnOnly    = []constants.Severity{constants.SeverityWarning}
	errOrWarn   = []constants.Severity{constants.SeverityError, constants.SeverityWarning}
	reviewOnly  = []constants.Severity{constants.SeverityReview}
	drawing     = constants.FindingScopeDrawing
	partScope   = constants.FindingScopePart
	optionalSet = func(get func(capability.OptionalLaserCuttingLimits) *capability.Limit) func(*capability.Profile) bool {
		return laserSet(func(p *capability.LaserCuttingProfile) bool { return get(p.Optional) != nil })
	}
	// laserSet adapts a check of the laser-cutting profile.
	laserSet = func(f func(*capability.LaserCuttingProfile) bool) func(*capability.Profile) bool {
		return func(p *capability.Profile) bool { return p.LaserCutting != nil && f(p.LaserCutting) }
	}
)

// laserCatalog lists every laser-cutting finding code in rule order.
var laserCatalog = []CodeInfo{
	{
		Code: constants.FindingCodeGeometryOpenContour, Severities: errOnly, Scope: drawing,
		Description: "A cut path's ends do not meet, so it does not enclose anything; actual is the gap.",
		Remediation: "Join the path's ends at the location given, or delete the path if it is not meant to be cut.",
	},
	{
		Code: constants.FindingCodeGeometryGapClosed, Severities: warnOnly, Scope: drawing, Limit: "laser_cutting.contour_closure_tolerance",
		Description: "A gap between path ends within the closure tolerance was bridged before checking; actual is the gap.",
		Remediation: "No change is needed to cut the part; join the ends in the design to remove the warning.",
	},
	{
		Code: constants.FindingCodeGeometryZeroLength, Severities: warnOnly, Scope: drawing,
		Description: "An entity, or a piece of a polyline, is shorter than 0.001 mm and is ignored.",
		Remediation: "Delete the entity or the repeated vertex.",
	},
	{
		Code: constants.FindingCodeTextNotOutlined, Severities: warnOnly, Scope: drawing,
		Description: "The drawing has a text entity, which is not cut.",
		Remediation: "If the text should be cut, convert it to outlines (and bridge letters with enclosed centers); otherwise delete it.",
	},
	{
		Code: constants.FindingCodeFileNoncutGeometry, Severities: errOnly, Scope: drawing,
		Description: "Geometry is drawn as a construction or hidden line (a dashed line type, or a hidden, frozen or non-plotting layer) and is left out of the part.",
		Remediation: "Delete the construction geometry, or draw it with a continuous line type on a visible layer if it should be cut.",
	},
	{
		Code: constants.FindingCodeBendLineNotCut, Severities: warnOnly, Scope: drawing,
		Description: "Geometry on a bend or fold layer, or a dashed straight line running through the material from one cut to another, is a bend line; it is not cut. Bends are checked by the bending operation (dfm plan with a press brake capability).",
		Remediation: "No change is needed to cut the flat part; check the bends with a press brake capability.",
	},
	{
		Code: constants.FindingCodeBendLineSuspected, Severities: errOnly, Scope: drawing,
		Description: "A solid straight line on a cut layer runs through the material from one cut to another, as a bend line does. It is left out of the part, so the other checks see the part without it.",
		Remediation: "If it is a bend line, put it on a layer named BEND or draw it dashed; if it is a cut, it must close a shape.",
	}, {
		Code: constants.FindingCodeBorderNotCut, Severities: warnOnly, Scope: drawing,
		Description: "A sheet border is drawn on the cut layer: two rectangles, one just inside the other, around every other path, with at least two shapes inside. It is left out of the part, so the parts inside it are checked as parts.",
		Remediation: "Delete the border from the drawing (accept the fix), or move it to a layer that is not cut.",
	},

	{
		Code: constants.FindingCodeEngravingNotCut, Severities: warnOnly, Scope: drawing,
		Description: "Geometry on an engraving, etching, marking or scoring layer is not cut through and is not checked.",
		Remediation: "No change is needed if it is meant to be engraved; move it to the cut layer if it should be cut.",
	},
	{
		Code: constants.FindingCodeAnnotationNotCut, Severities: warnOnly, Scope: drawing,
		Description: "Geometry is on a layer named for drawing annotation (dimensions, center lines, notes, a title block or construction lines), or is a DXF fill (HATCH, SOLID), which is never cut. It is left out of the part and not checked; a shape drawn only as a fill needs an outline to be cut.",
		Remediation: "No change is needed; delete annotation layers from cut files to remove the warning.",
	},
	{
		Code: constants.FindingCodeGeometryDuplicate, Severities: warnOnly, Scope: drawing,
		Description: "A path or circle is drawn exactly on top of other geometry; the copy is ignored. entity_ids are the copies only. When a copy is only a piece of a path whose rest is kept (a path that retraces part of itself), no fix is suggested: removing the path would remove the kept part too.",
		Remediation: "Delete the listed entities; they are copies of geometry that stays. Where only a piece of a path repeats, delete that piece.",
	},
	{
		Code: constants.FindingCodeGeometryOvercut, Severities: warnOnly, Scope: drawing,
		Description: "A path runs on past its start over its own first stretch (an overcut, which makes sure a cut closes); the repeat is ignored and the path closed. actual is the repeat's length.",
		Remediation: "No change is needed; end the path at its start to remove the warning.",
	},
	{
		Code: constants.FindingCodeGeometryCornerLoop, Severities: warnOnly, Scope: drawing,
		Description: "A path makes small loops where it leaves a point and comes back to it, corner reliefs some generators draw so the beam clears an inside corner; the loops are ignored, so corners are checked as drawn without them. One occurrence per path, located at its first loop; actual is the largest loop's size.",
		Remediation: "No change is needed if the reliefs are intended; otherwise remove the loops.",
	},
	{
		Code: constants.FindingCodeGeometryFacetedCurve, Severities: warnOnly, Scope: drawing,
		Description: "A curve is drawn as many short straight segments (at least 8, each turning 0.25° to 15° the same way), the sign of an export through a mesh format or a file converter. It is cut as drawn, a polygon, and slower than a true arc. actual is the number of segments.",
		Remediation: "Export the curve from the original design as arcs, circles or splines rather than polylines.",
	},
	{
		Code: constants.FindingCodeGeometryOutsidePage, Severities: warnOnly, Scope: drawing, Limit: "laser_cutting.outside_page",
		Description: "A shape lies wholly outside the SVG's page, such as a preview or note drawn beside it. It is cut unless the capability's outside_page is ignore, when it is left out. One occurrence lists every such shape.",
		Remediation: "Move the shape onto the page if it is a part, or delete it if it is not.",
	},
	{
		Code: constants.FindingCodeGeometryCommonLine, Severities: errOrWarn, Scope: drawing, Limit: "laser_cutting.common_line",
		Description: "Two parts share a cut line (common-line cutting): one line separates them, so neither has its own outline there. Each part is still checked on its own. An error unless the capability's common_line is accepted. actual is the shared length.",
		Remediation: "Give each part its own outline with at least the kerf between them, or use a capability that accepts common-line cutting.",
	},
	{
		Code: constants.FindingCodeGeometryOverlappingSegments, Severities: errOnly, Scope: drawing,
		Description: "Paths partly overlap, so a stretch would be cut twice; actual is the shared length. Between two parts this is a shared (common) cut line.",
		Remediation: "Trim the overlapping paths so each stretch is drawn once; separate parts that share an edge by at least the kerf.",
	},
	{
		Code: constants.FindingCodeGeometrySelfIntersection, Severities: errOnly, Scope: drawing,
		Description: "A cut path touches or crosses itself, so what is part and what is scrap is ambiguous.",
		Remediation: "Redraw the path at the location given so it does not touch or cross itself.",
	},
	{
		Code: constants.FindingCodeGeometryContoursTouchOrCross, Severities: errOnly, Scope: drawing,
		Description: "Two separate cut paths touch or cross.",
		Remediation: "Move the paths apart at the location given, or merge them into one outline.",
	}, {
		Code: constants.FindingCodeFlatPatternOverlaps, Severities: errOnly, Scope: drawing,
		Description: "A flat pattern unfolded from a STEP model overlaps itself: flanges that lie over each other or over the base once unfolded (their outlines cross, touch or run along each other) cannot be cut from one sheet. It replaces the drawing checks for crossing, touching and retraced paths on unfolded models.",
		Remediation: "Shorten or narrow the flanges that overlap when unfolded, add a relief or gap between them, or make the part from more than one piece.",
	},

	{
		Code: constants.FindingCodeGeometryNotFlat, Severities: errOnly, Scope: drawing,
		Description: "A cut entity is not at the drawing's height: above or below the plane most entities share, or sloping across it; actual is how far off it reaches. A flat part's drawing must lie in one plane.",
		Remediation: "Export the drawing as a flat 2D view with every entity at the same height (Z = 0).",
	},
	{
		Code: constants.FindingCodeFileMultipleParts, Severities: errOrWarn, Scope: drawing, Limit: "laser_cutting.multiple_parts",
		Description: "The drawing has more than one part. With multiple_parts rejected it fails; otherwise each part is checked separately.",
		Remediation: "Upload each part as its own drawing, or use a capability that accepts several parts per drawing.",
	},
	{
		Code: constants.FindingCodePartNestedInCutout, Severities: warnOnly, Scope: drawing, Limit: "laser_cutting.multiple_parts",
		Description: "A part lies inside another part's cut-out and is cut free as a separate part.",
		Remediation: "No change is needed if the part is meant to be nested there; if it should stay attached, bridge it to the surrounding part.",
	},
	{
		Code: constants.FindingCodeFloatingIsland, Severities: errOnly, Scope: partScope,
		Description: "Material is enclosed by a cut-out and would fall out loose, like the center of a letter O. An island no wider than the kerf burns away and is not reported.",
		Remediation: "Bridge the island to the surrounding material, remove it, or upload it as a separate part.",
	},
	{
		Code: constants.FindingCodeMaterialUnsupported, Severities: errOnly, Scope: drawing,
		Description: "The stock's material is not one the capability can cut.",
		Remediation: "Choose a material the capability lists.",
	},
	{
		Code: constants.FindingCodeMaterialTooThick, Severities: errOnly, Scope: drawing, Limit: "materials[].laser_cutting.max_thickness",
		Description: "The stock is thicker than the material's maximum thickness.",
		Remediation: "Choose stock no thicker than the material's maximum (required), or a capability that cuts thicker stock.",
	},
	{
		Code: constants.FindingCodeMaterialTooThin, Severities: errOnly, Scope: drawing, Limit: "materials[].laser_cutting.min_thickness",
		Description: "The stock is thinner than the material's minimum thickness.",
		Remediation: "Choose stock at least the material's minimum (required), or a capability that cuts thinner stock.",
	},
	{
		Code: constants.FindingCodeMaterialThicknessNotStocked, Severities: errOnly, Scope: drawing, Limit: "materials[].laser_cutting.thicknesses",
		Description: "The stock's thickness is not one the shop stocks for the material (within 0.001 mm).",
		Remediation: "Choose one of the thicknesses the capability lists for the material.",
	}, {
		Code: constants.FindingCodeModelThicknessMismatch, Severities: errOnly, Scope: drawing,
		Description: "A 3D model's sheet (actual) differs from the stock thickness (required) by more than max(0.05 mm, 3%): the part would be cut from sheet of another thickness than it was designed in, and its bends and fits would change.",
		Remediation: "Choose stock of the model's thickness, or change the model's sheet thickness to the stock's.",
	},

	{
		Code: constants.FindingCodeUnitsSuspect, Severities: warnOnly, Scope: drawing,
		Description: "The drawing's size is implausible in the units it was read in (under 6 mm across, or larger than the usable area) but plausible in inches or millimeters instead; actual is its largest dimension as read.",
		Remediation: "Confirm the drawing's units; if they are wrong, check it again with the right units set (the design's units override the file's).",
	},
	{
		Code: constants.FindingCodeUnitsUnsupported, Severities: errOnly, Scope: drawing, Limit: "laser_cutting.drawing_units",
		Description: "The drawing is in units the capability does not accept (for example centimeters, which some services reject because exports default to them by mistake).",
		Remediation: "Save the drawing in one of the accepted units, at full scale.",
	},
	{
		Code: constants.FindingCodeUnitsAssumed, Severities: warnOnly, Scope: drawing,
		Description: "Part of the drawing's size had to be assumed: its sizes are in pixels, read at 72 per inch for Illustrator files, 90 for Inkscape before 0.92 and otherwise the standard 96.",
		Remediation: "Confirm the part's size; if it is wrong, check it again with the design's units set, or save the drawing with its size in millimeters or inches.",
	},
	{
		Code: constants.FindingCodePartExceedsEnvelope, Severities: errOnly, Scope: partScope, Limit: "laser_cutting.usable_area",
		Description: "The part's bounding box does not fit the usable cut area, even rotated 90°.",
		Remediation: "Make the part smaller than the usable area, or split it into pieces.",
	},
	{
		Code: constants.FindingCodePartTooSmall, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.part_width",
		Description: "The part's narrowest width is below the minimum; small parts can tip or fall through the machine bed.",
		Remediation: "Make the part at least the required width, or attach it to a larger part with tabs.",
		limitSet:    optionalSet(func(o capability.OptionalLaserCuttingLimits) *capability.Limit { return o.PartWidth }),
	},
	{
		Code: constants.FindingCodePartTooShort, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.part_length",
		Description: "The part's length (at right angles to its narrowest width) is below the minimum part size.",
		Remediation: "Make the part at least the required length, or attach it to a larger part with tabs.",
		limitSet:    optionalSet(func(o capability.OptionalLaserCuttingLimits) *capability.Limit { return o.PartLength }),
	},
	{
		Code: constants.FindingCodePartAreaTooSmall, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.part_area",
		Description: "The finished part's area (its outline less its cut-outs) is below the minimum, even if its width and length pass.",
		Remediation: "Make the part larger or remove less material from it, or attach it to a larger part with tabs.",
		limitSet:    laserSet(func(p *capability.LaserCuttingProfile) bool { return p.Optional.PartArea != nil }),
	},
	{
		Code: constants.FindingCodePartTooSlender, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.slender_part",
		Description: "The part is too narrow for its length: long narrow strips warp from the heat of cutting.",
		Remediation: "Widen the part to at least the required width, or shorten it.",
		limitSet:    laserSet(func(p *capability.LaserCuttingProfile) bool { return len(p.Optional.SlenderPart) > 0 }),
	},
	{
		Code: constants.FindingCodePartMayFallThrough, Severities: warnOnly, Scope: partScope, Limit: "laser_cutting.support_spacing",
		Description: "A part's length (at right angles to its narrowest width) is less than the distance between the bed's supports: when it is cut free it can drop between them or tip into the beam. Shops tab such parts in or cut them on a finer grid.",
		Remediation: "Accept that the shop may tab the part (leaving small marks), make it longer than required, or nest it in a larger part's scrap.",
		limitSet:    laserSet(func(p *capability.LaserCuttingProfile) bool { return p.SupportSpacing > 0 }),
	},
	{
		Code: constants.FindingCodeCutoutTooSlender, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.slender_cutout",
		Description: "A cut-out is too narrow for its length: long narrow cut-outs let the part warp from the heat of cutting. Round holes are not checked.",
		Remediation: "Widen the cut-out to at least the required width, shorten it, or break it up with bridges of material.",
		limitSet:    laserSet(func(p *capability.LaserCuttingProfile) bool { return len(p.Optional.SlenderCutout) > 0 }),
	}, {
		Code: constants.FindingCodeNotchTooSlender, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.slender_cutout",
		Description: "An edge notch (a bay of the part's outline, measured inside its convex hull) is narrower than the slender_cutout tier for its length requires: a long narrow notch leaves long narrow fingers either side, which warp in the heat of the cut, as long narrow cut-outs do.",
		Remediation: "Widen the notch to at least the required width, shorten it, or accept a risk of warping.",
		limitSet:    laserSet(func(p *capability.LaserCuttingProfile) bool { return len(p.Optional.SlenderCutout) > 0 }),
	},

	{
		Code: constants.FindingCodeHoleTooSmall, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.hole_diameter",
		Description: "A round hole's diameter is below the minimum for the stock.",
		Remediation: "Enlarge the hole to at least the required diameter, or remove it and drill it after cutting.",
	},
	{
		Code: constants.FindingCodeHoleOperationNotAHole, Severities: errOnly, Scope: drawing,
		Description: "A hole_operations assignment selects no round hole: an entity that is not part of one, or a diameter (actual) no round hole is drawn at.",
		Remediation: "Select holes by the entity_ids or diameter listed in the part's holes.",
	},
	{
		Code: constants.FindingCodeHoleOperationConflict, Severities: errOnly, Scope: drawing,
		Description: "A hole is selected by more than one hole_operations assignment; it is given none of them.",
		Remediation: "Assign each hole one operation.",
	},
	{
		Code: constants.FindingCodeHoleOperationUnavailable, Severities: errOnly, Scope: partScope,
		Description: "The operation assigned to a hole is not offered for the stock's material and thickness (actual).",
		Remediation: "Choose an operation listed in the hole group's available_operations, or change the material or thickness.",
	},
	{
		Code: constants.FindingCodeHoleResized, Severities: warnOnly, Scope: partScope,
		Description: "A hole was drawn at actual and is resized to the operation's hole size (required); every check sees the new size.",
		Remediation: "No change is needed; draw the hole at the required diameter to remove the warning.",
	},
	{
		Code: constants.FindingCodeHoleOperationTooCloseToEdge, Severities: errOrWarn, Scope: partScope, Limit: "hole_operations[].edge_distance",
		Description: "The distance from a tapped, countersunk or fastener hole's center to the nearest other cut (actual) is below the operation's edge_distance.",
		Remediation: "Move the hole away from the edge or the cut-out at the location given, or choose a smaller operation.",
	},
	{
		Code: constants.FindingCodeHoleOperationsTooClose, Severities: errOrWarn, Scope: partScope, Limit: "hole_operations[].spacing",
		Description: "Two holes with operations are closer, center to center (actual), than the larger of their operations' spacing.",
		Remediation: "Move the holes apart.",
	},
	{
		Code: constants.FindingCodeCountersinkTooDeep, Severities: errOnly, Scope: partScope, Limit: "hole_operations[].countersink.max_depth_ratio",
		Description: "The countersink's cone, from the hole to its major diameter, is deeper (actual) than the allowed share of the thickness (required), so it would leave a knife edge or cut through.",
		Remediation: "Use thicker stock or a smaller countersink, or a fastener instead.",
	},
	{
		Code: constants.FindingCodeSlotTooNarrow, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.slot_width",
		Description: "A slot's width is below the minimum for the stock.",
		Remediation: "Widen the slot to at least the required width.",
	},
	{
		Code: constants.FindingCodeSlotFitTooTight, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.slot_fit_clearance",
		Description: "A slot that receives the stock (one within 25% of the thickness wide, as in tab-and-slot parts) is wider than the stock by less than the minimum clearance: actual is its width less the thickness, negative when it is narrower. Sheet varies in thickness and the kerf varies, so a slot drawn at the thickness may not fit.",
		Remediation: "Widen the slot to the stock thickness plus the required clearance.",
		limitSet:    optionalSet(func(o capability.OptionalLaserCuttingLimits) *capability.Limit { return o.SlotFitClearance }),
	},
	{
		Code: constants.FindingCodeWebTooThin, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.web_width",
		Description: "The material between two cut-outs is thinner than the minimum and may warp or burn away.",
		Remediation: "Move the cut-outs apart, or shrink them, until the material between them is at least the required width.",
		limitSet:    optionalSet(func(o capability.OptionalLaserCuttingLimits) *capability.Limit { return o.WebWidth }),
	},
	{
		Code: constants.FindingCodeFeaturesTooClose, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.feature_spacing",
		Description: "Two cut-outs are closer together than the minimum spacing.",
		Remediation: "Move the cut-outs apart to at least the required spacing.",
		limitSet:    optionalSet(func(o capability.OptionalLaserCuttingLimits) *capability.Limit { return o.FeatureSpacing }),
	},
	{
		Code: constants.FindingCodeFeatureTooCloseToEdge, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.edge_distance",
		Description: "The material between a cut-out and the edge around it is thinner than the minimum.",
		Remediation: "Move the cut-out away from the edge, or shrink it, until at least the required distance remains.",
		limitSet:    optionalSet(func(o capability.OptionalLaserCuttingLimits) *capability.Limit { return o.EdgeDistance }),
	},
	{
		Code: constants.FindingCodeCutoutTooNarrow, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.cutout_width",
		Description: "A cut-out or edge notch narrows below the minimum width somewhere along it.",
		Remediation: "Widen the cut-out at the location given to at least the required width.",
		limitSet:    optionalSet(func(o capability.OptionalLaserCuttingLimits) *capability.Limit { return o.CutoutWidth }),
	},
	{
		Code: constants.FindingCodeFeatureTooThin, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.feature_width",
		Description: "A strip of material within one outline (a finger, tab or tongue) is thinner than the minimum.",
		Remediation: "Widen the strip at the location given to at least the required width.",
		limitSet:    optionalSet(func(o capability.OptionalLaserCuttingLimits) *capability.Limit { return o.FeatureWidth }),
	},
	{
		Code: constants.FindingCodeInternalCornerTooSharp, Severities: errOrWarn, Scope: partScope, Limit: "laser_cutting.limits.internal_corner_radius",
		Description: "An inside corner of the material is sharper than the minimum radius.",
		Remediation: "Round the corner to at least the required radius.",
		limitSet:    optionalSet(func(o capability.OptionalLaserCuttingLimits) *capability.Limit { return o.InternalCornerRadius }),
	},
	{
		Code: constants.FindingCodeTipTooSharp, Severities: warnOnly, Scope: partScope, Limit: "laser_cutting.limits.min_tip_angle",
		Description: "An outline comes to a point sharper than the minimum angle; the tip may burn away.",
		Remediation: "Widen the angle at the location given, or blunt the tip.",
		limitSet:    laserSet(func(p *capability.LaserCuttingProfile) bool { return p.Optional.MinTipAngle != nil }),
	},
	{
		Code: constants.FindingCodeDensePatternHeatRisk, Severities: warnOnly, Scope: partScope, Limit: "laser_cutting.limits.max_removed_area_percent",
		Description: "Cut-outs remove a large share of the part's area, which concentrates heat and can warp it. With removed_area_window set, the same share is also checked in every square window that size across the part, and the densest window is reported with its center.",
		Remediation: "Reduce the cut-out area, or accept a risk of warping.",
		limitSet:    laserSet(func(p *capability.LaserCuttingProfile) bool { return p.Optional.MaxRemovedAreaPercent != nil }),
	},
}

// bendingCatalog lists the bending codes in rule order.
var bendingCatalog = []CodeInfo{
	{
		Code: constants.FindingCodeMaterialUnsupported, Severities: errOnly, Scope: drawing,
		Description: "The stock's material is not one the press brake bends.",
		Remediation: "Choose a material the capability lists for bending.",
	},
	{
		Code: constants.FindingCodeMaterialTooThick, Severities: errOnly, Scope: drawing, Limit: "materials[].bending.max_thickness",
		Description: "The stock is thicker than the press brake bends in this material.",
		Remediation: "Choose stock no thicker than the material's maximum (required), or a press brake that bends thicker stock.",
	},
	{
		Code: constants.FindingCodeMaterialTooThin, Severities: errOnly, Scope: drawing, Limit: "materials[].bending.min_thickness",
		Description: "The stock is thinner than the press brake bends in this material.",
		Remediation: "Choose stock at least the material's minimum (required).",
	},
	{
		Code: constants.FindingCodeBendTooLong, Severities: errOnly, Scope: drawing, Limit: "bending.max_bend_length",
		Description: "A bend line is longer than the press brake's usable bending length.",
		Remediation: "Shorten the bend, split the part, or choose a longer press brake.",
	},
	{
		Code: constants.FindingCodeBendTooShort, Severities: errOnly, Scope: drawing, Limit: "bending.min_bend_length",
		Description: "A bend line is shorter than the shortest bend the tooling holds (its narrowest punch and die segment): the part cannot be held square in the die.",
		Remediation: "Lengthen the bend to at least the required length, or choose a press brake with shorter tooling segments.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.MinBendLength > 0 },
	},
	{
		Code: constants.FindingCodeBendForceTooHigh, Severities: errOnly, Scope: drawing, Limit: "bending.max_force",
		Description: "A bend needs more air-bending force than the press brake has: 60 t² L / V tonnes with a die opening V of 8 t (7.5 t L tonnes for mild steel; t in mm, L in m), times the material's force_factor. actual is the force needed, in kN.",
		Remediation: "Shorten the bend, use thinner stock, or choose a press brake with more force.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.MaxForce > 0 },
	},
	{
		Code: constants.FindingCodeBendAngleTooLarge, Severities: errOnly, Scope: drawing, Limit: "bending.max_angle",
		Description: "A bend's angle (from flat; 90 when the request's bends do not give it) is larger than the tooling forms. actual and required are in degrees.",
		Remediation: "Open the bend to at most the required angle, or choose a press brake with tooling for sharper bends.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.MaxAngle > 0 },
	},
	{
		Code: constants.FindingCodeBendAngleTooSmall, Severities: errOnly, Scope: drawing, Limit: "bending.min_angle",
		Description: "A bend's angle (from flat) is smaller than the brake holds repeatably: a shallow air bend springs back by an amount that varies more than the bend. actual and required are in degrees.",
		Remediation: "Bend at least the required angle, or leave the part flat.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.MinAngle > 0 },
	},
	{
		Code: constants.FindingCodeBendRadiusNotFormed, Severities: warnOnly, Scope: drawing, Limit: "bending.inside_radius",
		Description: "A bend's inside radius, as the request's bends give it, differs by more than 0.01 mm from the radius the tooling forms (required). The bend is made at the tooling's radius, so a flat pattern worked out for the design's radius bends to slightly different dimensions.",
		Remediation: "Redo the flat pattern for the required inside radius, or choose a press brake whose tooling forms the design's radius.",
	},
	{
		Code: constants.FindingCodeBendRadiusTooSmall, Severities: errOrWarn, Scope: drawing, Limit: "bending.limits.min_inside_radius",
		Description: "A bend's inside radius, as a model or the request's bends give it, is below the minimum: a bend drawn much tighter than tooling forms has a flat pattern no press brake makes, and tight bends crack. Assumed radii are the tooling's and are not checked.",
		Remediation: "Draw the bend at least at the required inside radius (the tooling's radius is best), and redo the flat pattern.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.MinInsideRadius != nil },
	},
	{
		Code: constants.FindingCodeFlangeTooShort, Severities: errOrWarn, Scope: drawing, Limit: "bending.limits.flange_length",
		Description: "The shorter flange beside a bend (from the bend line to the part's edge or the next bend line, whichever is nearer, measured along the middle 80% of the bend) is below the minimum: it cannot reach across the die. Between two bends (the base of a U channel) it names both.",
		Remediation: "Lengthen the flange to at least the required length, or remove the bend.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.FlangeLength != nil },
	},
	{
		Code: constants.FindingCodeBendZonesOverlap, Severities: errOnly, Scope: drawing,
		Description: "Two neighboring bends are closer together than their bend zones are wide: each bend takes up its bend allowance of flat material (angle x (inside radius + k_factor x thickness), at the radius the tooling forms), centered on its line, and the two would overlap. actual is the distance between the lines, required half of each allowance added together.",
		Remediation: "Move the bends apart by at least the required distance, or bend through smaller angles.",
	},
	{
		Code: constants.FindingCodeBendToolingUnchecked, Severities: reviewOnly, Scope: drawing,
		Description: "Two parallel bends turning the same way overlap along their length: a channel. Forming the second may bring the first flange into the punch or the ram, which depends on the tooling and the bend order, and these are not modeled. A person must check it. Not reported when the capability gives the press brake's tooling: bend_tool_collision simulates the bends instead. Directions the request does not give are assumed (up), so a drawing's channel is flagged unless its bends are given turning opposite ways.",
		Remediation: "Have the shop confirm the channel can be formed with its tooling (a gooseneck punch, or a deeper die), or give the bends' directions if they turn opposite ways.",
	},
	{
		Code: constants.FindingCodeBendToolCollision, Severities: []constants.Severity{constants.SeverityError, constants.SeverityReview}, Scope: drawing, Limit: "bending.tooling",
		Description: "No bend order forms the part without running it into the punch, the die or the ram above the punch. Each bend is formed from flat to its angle in 5 degree steps, with the bends before it in the order at their angles and the ones after it flat; the punch stands on the bend's bisector and may sit at the bend's start, middle or end, and the die lies under the flange that stays put. A gooseneck punch's nose joins its set-back body by a neck sloping back at 45 degrees, and the punch may face either way round. The holder and ram are taken to stand over the punch's shank, as wide as it, for the length of the machine. The bend's own two flanges count against the punch at its final angle: a punch that does not fit between them cannot form the bend. Every order is tried; the bends reported are those no order could form at the furthest any order got. A review finding means the search stopped before trying every order. For a part that clears, each bend's sequence, punch_position and throat_side give the order found and how each bend was formed.",
		Remediation: "Shorten or move the flange that is hit, open the bend angles, or choose tooling that clears (a narrower or taller punch, a gooseneck punch, a narrower die). For a review finding, have the shop confirm a bend order.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.Tooling != nil },
	},
	{
		Code: constants.FindingCodeBendFlangesCollide, Severities: errOnly, Scope: drawing,
		Description: "Two flanges run into each other once every bend is formed: with the bends at their angles and the radius the tooling forms, a flange's sheet lies inside another's (lips that overlap at a box corner, a wall drawn past the base's corner into the next wall, a return deeper than the wall it hangs from). The part is folded as bend_tool_collision folds it and the flanges are compared to the resolution of its samples, about a millimeter on a typical part, so a shallower overlap can go unseen. A flange and the one it is bent from meet only at their bend and are not compared; only the finished part is checked, not whether a flange sweeps through another while a bend forms. Located at the flat material that collides, with the span to the material it meets.",
		Remediation: "Shorten the flanges that meet, miter or notch them where they cross, or change a bend's angle or direction so they clear.",
	},
	{
		Code: constants.FindingCodeFeatureTooCloseToBend, Severities: errOrWarn, Scope: drawing, Limit: "bending.limits.feature_distance",
		Description: "A hole or cut-out is closer to a bend line than the minimum (0 when the bend crosses it): the bend stretches the material near it and distorts it.",
		Remediation: "Move the feature away from the bend to at least the required distance, or add relief cuts.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.FeatureDistance != nil },
	},
	{
		Code: constants.FindingCodeBendReliefMissing, Severities: errOnly, Scope: drawing,
		Description: "A bend line ends where the material beside it stays flat: at an inside corner, running along an edge, or inside the material. The bend tears the material there. A bend that ends on an edge crossing it (the part's side, a relief slot or a relief hole) is free.",
		Remediation: "Add a bend relief at the end of the bend (a slot or hole at least the stock thickness wide reaching past the bend line), or end the bend on the part's edge.",
	},
	{
		Code: constants.FindingCodeBendCornerReliefMissing, Severities: errOnly, Scope: drawing,
		Description: "Two bend lines meet at a corner where the material stays whole (a box or tray corner with no corner relief): the two flanges pull the corner both ways and tear it.",
		Remediation: "Cut a corner relief where the bends meet (a round or square hole, or a notch, reaching past both bend lines), or end the bends on the part's edge.",
	},
	{
		Code: constants.FindingCodeBendLinesCross, Severities: errOnly, Scope: drawing,
		Description: "Two bend lines cross each other away from their ends: forming one bend folds the other line out of the die, so the part cannot be bent as drawn.",
		Remediation: "Split one bend at the other with a relief cut, or redesign so the bends do not cross.",
	},
	{
		Code: constants.FindingCodeBendReliefTooNarrow, Severities: errOrWarn, Scope: drawing, Limit: "bending.limits.relief_width",
		Description: "The relief a bend line ends at (the gap along the line from its end until material starts again, when that is within max(5 mm, 5 t)) is narrower than the minimum.",
		Remediation: "Widen the relief to at least the required width.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.ReliefWidth != nil },
	},
	{
		Code: constants.FindingCodeBendReliefTooShallow, Severities: errOrWarn, Scope: drawing, Limit: "bending.limits.relief_depth",
		Description: "The relief a bend line ends at reaches less than the minimum past the bend line, measured across the line from the middle of the gap to its bottom (the shallower side when both sides are closed).",
		Remediation: "Extend the relief past the bend line to at least the required depth.",
		limitSet:    func(p *capability.Profile) bool { return p.Bending != nil && p.Bending.ReliefDepth != nil },
	},
}

// holeOpBendInfo is the catalog entry of a hole operation step's check.
func holeOpBendInfo(typ constants.HoleOperationType) []CodeInfo {
	return []CodeInfo{{
		Code: constants.FindingCodeHoleOperationTooCloseToBend, Severities: errOrWarn, Scope: drawing, Limit: "hole_operations[].bend_distance",
		Description: "A hole whose operation is this step's (a countersink, a tapped thread or a press-in fastener) is closer to a bend line, from the hole's edge, than the operation's bend_distance: the bend distorts the hole and what is in it.",
		Remediation: "Move the hole away from the bend to at least the required distance, or drop its operation.",
		limitSet: func(p *capability.Profile) bool {
			for _, op := range p.HoleOperations {
				if op.Type == typ && op.BendDistance != nil {
					return true
				}
			}
			return false
		},
	}}
}

// Catalog returns every finding code of a process in rule order.
func Catalog(p constants.Process) []CodeInfo {
	switch p {
	case constants.ProcessLaserCutting:
		return laserCatalog
	case constants.ProcessBending:
		return bendingCatalog
	case constants.ProcessCountersinking:
		return countersinkingCatalog
	case constants.ProcessTapping:
		return tappingCatalog
	case constants.ProcessHardwareInsertion:
		return hardwareCatalog
	}
	return nil
}

var (
	countersinkingCatalog = holeOpBendInfo(constants.HoleOperationCountersink)
	tappingCatalog        = holeOpBendInfo(constants.HoleOperationTap)
	hardwareCatalog       = holeOpBendInfo(constants.HoleOperationHardware)
)

// catalogIndex is every catalog by process and code, for the lookups the runner makes on each rule.
var catalogIndex = func() map[constants.Process]map[constants.FindingCode]CodeInfo {
	out := map[constants.Process]map[constants.FindingCode]CodeInfo{}
	for _, p := range catalogOrder {
		byCode := map[constants.FindingCode]CodeInfo{}
		for _, c := range Catalog(p) {
			byCode[c.Code] = c
		}
		out[p] = byCode
	}
	return out
}()

// catalogOrder is the processes in the order Info searches their catalogs: laser cutting's entry first for a code several report.
var catalogOrder = []constants.Process{constants.ProcessLaserCutting, constants.ProcessBending, constants.ProcessCountersinking, constants.ProcessTapping, constants.ProcessHardwareInsertion}

// Info returns the catalog entry for a code, laser cutting's first for a code several processes report.
func Info(code constants.FindingCode) (CodeInfo, bool) {
	for _, p := range catalogOrder {
		if c, ok := catalogIndex[p][code]; ok {
			return c, true
		}
	}
	return CodeInfo{}, false
}

// Skipped returns the checks that did not run because the profile does not set their limit, in rule order.
func Skipped(p *capability.Profile) []CodeInfo {
	var out []CodeInfo
	for _, c := range Catalog(p.Process) {
		if c.limitSet != nil && !c.limitSet(p) {
			out = append(out, c)
		}
	}
	return out
}
