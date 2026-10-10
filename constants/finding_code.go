package constants

// FindingCode identifies the kind of a finding. Values are part of the public contract and should be kept stable.
type FindingCode string

const (
	FindingCodeGeometryOpenContour          FindingCode = "geometry_open_contour"
	FindingCodeGeometryZeroLength           FindingCode = "geometry_zero_length"
	FindingCodeGeometryOverlappingSegments  FindingCode = "geometry_overlapping_segments"
	FindingCodeGeometrySelfIntersection     FindingCode = "geometry_self_intersection"
	FindingCodeGeometryContoursTouchOrCross FindingCode = "geometry_contours_touch_or_cross"
	FindingCodeFileMultipleParts            FindingCode = "file_multiple_parts"
	FindingCodeFloatingIsland               FindingCode = "floating_island"
	FindingCodeMaterialUnsupported          FindingCode = "material_unsupported"
	FindingCodeMaterialTooThick             FindingCode = "material_too_thick"
	FindingCodePartExceedsEnvelope          FindingCode = "part_exceeds_envelope"
	FindingCodeHoleTooSmall                 FindingCode = "hole_too_small"
	FindingCodeSlotTooNarrow                FindingCode = "slot_too_narrow"
	FindingCodePartTooSmall                 FindingCode = "part_too_small"
	FindingCodeWebTooThin                   FindingCode = "web_too_thin"
	FindingCodeFeaturesTooClose             FindingCode = "features_too_close"
	FindingCodeFeatureTooCloseToEdge        FindingCode = "feature_too_close_to_edge"
	FindingCodeDensePatternHeatRisk         FindingCode = "dense_pattern_heat_risk"
	FindingCodeCutoutTooNarrow              FindingCode = "cutout_too_narrow"
	FindingCodeFeatureTooThin               FindingCode = "feature_too_thin"
	FindingCodeInternalCornerTooSharp       FindingCode = "internal_corner_too_sharp"
	FindingCodeTipTooSharp                  FindingCode = "tip_too_sharp"
	FindingCodeTextNotOutlined              FindingCode = "text_not_outlined"
	FindingCodeFileNoncutGeometry           FindingCode = "file_noncut_geometry"
	FindingCodeGeometryDuplicate            FindingCode = "geometry_duplicate"
	FindingCodeBendLineNotCut               FindingCode = "bend_line_not_cut"
	FindingCodeEngravingNotCut              FindingCode = "engraving_not_cut"
	FindingCodeGeometryGapClosed            FindingCode = "geometry_gap_closed"
	FindingCodePartNestedInCutout           FindingCode = "part_nested_in_cutout"
	FindingCodePartTooShort                 FindingCode = "part_too_short"
	FindingCodePartTooSlender               FindingCode = "part_too_slender"
	FindingCodeUnitsSuspect                 FindingCode = "units_suspect"
	FindingCodeGeometryNotFlat              FindingCode = "geometry_not_flat"
	FindingCodeUnitsAssumed                 FindingCode = "units_assumed"
	FindingCodeGeometryOvercut              FindingCode = "geometry_overcut"
	FindingCodeGeometryCornerLoop           FindingCode = "geometry_corner_loop"
	FindingCodeGeometryFacetedCurve         FindingCode = "geometry_faceted_curve"
	FindingCodeHoleOperationNotAHole        FindingCode = "hole_operation_not_a_hole"
	FindingCodeHoleOperationConflict        FindingCode = "hole_operation_conflict"
	FindingCodeHoleOperationUnavailable     FindingCode = "hole_operation_unavailable"
	FindingCodeHoleResized                  FindingCode = "hole_resized"
	FindingCodeHoleOperationTooCloseToEdge  FindingCode = "hole_operation_too_close_to_edge"
	FindingCodeHoleOperationsTooClose       FindingCode = "hole_operations_too_close"
	FindingCodeCountersinkTooDeep           FindingCode = "countersink_too_deep"
	FindingCodeGeometryOutsidePage          FindingCode = "geometry_outside_page"
	FindingCodeGeometryCommonLine           FindingCode = "geometry_common_line"
	FindingCodeAnnotationNotCut             FindingCode = "annotation_not_cut"
	FindingCodeUnitsUnsupported             FindingCode = "units_unsupported"
	FindingCodePartAreaTooSmall             FindingCode = "part_area_too_small"
	FindingCodeBendLineSuspected            FindingCode = "bend_line_suspected"
	FindingCodeBorderNotCut                 FindingCode = "border_not_cut"
	FindingCodeMaterialTooThin              FindingCode = "material_too_thin"
	FindingCodeMaterialThicknessNotStocked  FindingCode = "material_thickness_not_stocked"
	FindingCodeCutoutTooSlender             FindingCode = "cutout_too_slender"
	FindingCodeBendTooLong                  FindingCode = "bend_too_long"
	FindingCodeFlangeTooShort               FindingCode = "flange_too_short"
	FindingCodeFeatureTooCloseToBend        FindingCode = "feature_too_close_to_bend"
	FindingCodeBendForceTooHigh             FindingCode = "bend_force_too_high"
	FindingCodeBendReliefMissing            FindingCode = "bend_relief_missing"
	FindingCodeBendReliefTooNarrow          FindingCode = "bend_relief_too_narrow"
	FindingCodeBendReliefTooShallow         FindingCode = "bend_relief_too_shallow"
	FindingCodeBendAngleTooLarge            FindingCode = "bend_angle_too_large"
	FindingCodeBendRadiusNotFormed          FindingCode = "bend_radius_not_formed"
	FindingCodeBendZonesOverlap             FindingCode = "bend_zones_overlap"
	FindingCodeModelThicknessMismatch       FindingCode = "model_thickness_mismatch"
	FindingCodeBendRadiusTooSmall           FindingCode = "bend_radius_too_small"
	FindingCodePartMayFallThrough           FindingCode = "part_may_fall_through"
	FindingCodeSlotFitTooTight              FindingCode = "slot_fit_too_tight"
	FindingCodeHoleOperationTooCloseToBend  FindingCode = "hole_operation_too_close_to_bend"
	FindingCodeBendToolingUnchecked         FindingCode = "bend_tooling_unchecked"
	FindingCodeBendAngleTooSmall            FindingCode = "bend_angle_too_small"
	FindingCodeFlatPatternOverlaps          FindingCode = "flat_pattern_overlaps"
	FindingCodeBendTooShort                 FindingCode = "bend_too_short"
	FindingCodeBendCornerReliefMissing      FindingCode = "bend_corner_relief_missing"
	FindingCodeBendLinesCross               FindingCode = "bend_lines_cross"
	FindingCodeNotchTooSlender              FindingCode = "notch_too_slender"
	FindingCodeBendToolCollision            FindingCode = "bend_tool_collision"
	FindingCodeBendFlangesCollide           FindingCode = "bend_flanges_collide"
)

var findingCodes = []FindingCode{
	FindingCodeGeometryOpenContour, FindingCodeGeometryZeroLength,
	FindingCodeGeometryOverlappingSegments, FindingCodeGeometrySelfIntersection,
	FindingCodeGeometryContoursTouchOrCross, FindingCodeFileMultipleParts,
	FindingCodeFloatingIsland, FindingCodeMaterialUnsupported,
	FindingCodeMaterialTooThick, FindingCodePartExceedsEnvelope,
	FindingCodeHoleTooSmall, FindingCodeSlotTooNarrow, FindingCodePartTooSmall,
	FindingCodeWebTooThin, FindingCodeFeaturesTooClose, FindingCodeFeatureTooCloseToEdge,
	FindingCodeDensePatternHeatRisk, FindingCodeCutoutTooNarrow, FindingCodeFeatureTooThin,
	FindingCodeInternalCornerTooSharp, FindingCodeTipTooSharp, FindingCodeTextNotOutlined,
	FindingCodeFileNoncutGeometry, FindingCodeGeometryDuplicate, FindingCodeBendLineNotCut,
	FindingCodeEngravingNotCut, FindingCodeGeometryGapClosed,
	FindingCodePartNestedInCutout, FindingCodePartTooShort, FindingCodePartTooSlender,
	FindingCodeUnitsSuspect, FindingCodeGeometryNotFlat, FindingCodeUnitsAssumed,
	FindingCodeGeometryOvercut, FindingCodeGeometryCornerLoop,
	FindingCodeGeometryFacetedCurve, FindingCodeHoleOperationNotAHole,
	FindingCodeHoleOperationConflict, FindingCodeHoleOperationUnavailable,
	FindingCodeHoleResized, FindingCodeHoleOperationTooCloseToEdge,
	FindingCodeHoleOperationsTooClose, FindingCodeCountersinkTooDeep,
	FindingCodeGeometryOutsidePage, FindingCodeGeometryCommonLine,
	FindingCodeAnnotationNotCut, FindingCodeUnitsUnsupported, FindingCodePartAreaTooSmall,
	FindingCodeBendLineSuspected, FindingCodeBorderNotCut,
	FindingCodeMaterialTooThin, FindingCodeMaterialThicknessNotStocked,
	FindingCodeCutoutTooSlender, FindingCodeBendTooLong, FindingCodeFlangeTooShort,
	FindingCodeFeatureTooCloseToBend, FindingCodeBendForceTooHigh, FindingCodeBendReliefMissing,
	FindingCodeBendReliefTooNarrow, FindingCodeBendReliefTooShallow,
	FindingCodeBendAngleTooLarge, FindingCodeBendRadiusNotFormed, FindingCodeBendZonesOverlap,
	FindingCodeModelThicknessMismatch, FindingCodeBendRadiusTooSmall,
	FindingCodePartMayFallThrough, FindingCodeSlotFitTooTight, FindingCodeHoleOperationTooCloseToBend,
	FindingCodeBendToolingUnchecked, FindingCodeBendAngleTooSmall,
	FindingCodeFlatPatternOverlaps, FindingCodeBendTooShort, FindingCodeBendCornerReliefMissing,
	FindingCodeBendLinesCross, FindingCodeNotchTooSlender, FindingCodeBendToolCollision,
	FindingCodeBendFlangesCollide,
}

func (c FindingCode) IsValid() bool        { return isOneOf(c, findingCodes) }
func (c FindingCode) EnumValues() []string { return enumValues(findingCodes) }

// Open marks the enum as gaining values: every new check adds a code.
func (c FindingCode) Open() bool { return true }
