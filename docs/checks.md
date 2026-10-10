# Checks

Every finding code, by process, with its severity and when it fails. `dfm codes` gives each code's meaning and fix.

Severities: `error` (cannot be made), `review` (a person must judge; the verdict is `needs_review`), `warning` (can be made, perhaps at lower quality). A feature exactly at a limit passes, and comparisons are exact. A check whose limit the capability does not set is skipped and listed in `skipped_checks`.

## Laser cutting

| Code                               | Severity        | Fails when                                                                                                                                          |
| ---------------------------------- | --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `geometry_open_contour`            | error           | A cut path's ends do not meet. `actual` is the gap to the nearest loose end.                                                                        |
| `geometry_gap_closed`              | warning         | A gap within `contour_closure_tolerance` was bridged.                                                                                               |
| `geometry_zero_length`             | warning         | An entity or polyline piece is shorter than 0.001 mm; it is ignored.                                                                                |
| `geometry_duplicate`               | warning         | A path or circle lies exactly on other geometry, or a straight line lies within half the kerf of another edge. The copy is ignored.                 |
| `geometry_common_line`             | error / warning | Two parts share a cut line. A warning when `common_line` is `accepted`.                                                                             |
| `geometry_outside_page`            | warning         | SVG shapes lie wholly outside the `viewBox`. Cut unless `outside_page` is `ignore`.                                                                 |
| `geometry_overcut`                 | warning         | A path runs on past its start over its first stretch; the repeat is ignored.                                                                        |
| `geometry_corner_loop`             | warning         | A path makes small corner-relief loops (at most 0.5 mm across); they are ignored.                                                                   |
| `geometry_faceted_curve`           | warning         | A curve is drawn as at least 8 short segments turning the same way. Widths are measured to the curve they stand for.                                |
| `geometry_overlapping_segments`    | error           | Paths partly overlap. `actual` is the shared length.                                                                                                |
| `geometry_not_flat`                | error           | A cut entity is off the drawing's plane.                                                                                                            |
| `geometry_self_intersection`       | error           | A cut path touches or crosses itself.                                                                                                               |
| `geometry_contours_touch_or_cross` | error           | Two cut paths touch or cross (within 0.001 mm).                                                                                                     |
| `flat_pattern_overlaps`            | error           | A STEP model's flat pattern overlaps itself. Replaces the three checks above on unfolded models.                                                    |
| `file_multiple_parts`              | error / warning | The drawing has more than one part. An error when `multiple_parts` is `rejected`.                                                                   |
| `part_nested_in_cutout`            | warning         | With `multiple_parts: accepted_in_cutouts`, a part lies in another part's cut-out.                                                                  |
| `floating_island`                  | error           | Material inside a cut-out would fall out loose. Islands no wider than the kerf are not reported.                                                    |
| `material_unsupported`             | error           | The stock's material is not in the capability.                                                                                                      |
| `material_too_thick`               | error           | The stock is thicker than the material's `max_thickness`.                                                                                           |
| `material_too_thin`                | error           | The stock is thinner than the material's `min_thickness`.                                                                                           |
| `material_thickness_not_stocked`   | error           | The stock's thickness is not one of the material's `thicknesses` (within 0.001 mm).                                                                 |
| `model_thickness_mismatch`         | error           | A STEP model's sheet differs from the stock by more than max(0.05 mm, 3%).                                                                          |
| `units_suspect`                    | warning         | The drawing's size is implausible in its units but plausible in mm or inches instead.                                                               |
| `units_unsupported`                | error           | The drawing's units are not in `drawing_units`.                                                                                                     |
| `units_assumed`                    | warning         | The size had to be assumed, as for an SVG sized in pixels.                                                                                          |
| `part_exceeds_envelope`            | error           | The part does not fit the usable area, even rotated 90°.                                                                                            |
| `hole_too_small`                   | error / warning | A round hole is below `hole_diameter`. Holes with an operation are not checked.                                                                     |
| `hole_operation_not_a_hole`        | error           | A `hole_operations` assignment selects no round hole.                                                                                               |
| `hole_operation_conflict`          | error           | A hole is selected by more than one assignment; it gets none.                                                                                       |
| `hole_operation_unavailable`       | error           | The operation is not offered for the stock's material and thickness.                                                                                |
| `hole_resized`                     | warning         | A hole is resized to its operation's `hole_diameter`; every check sees the new size.                                                                |
| `hole_operation_too_close_to_edge` | error / warning | An operated hole's center is closer than the operation's `edge_distance` to another cut.                                                            |
| `hole_operations_too_close`        | error / warning | Two operated holes are closer than the larger of their `spacing`.                                                                                   |
| `countersink_too_deep`             | error           | A countersink is deeper than `max_depth_ratio` of the thickness.                                                                                    |
| `slot_too_narrow`                  | error / warning | A slot is below `slot_width`.                                                                                                                       |
| `slot_fit_too_tight`               | error / warning | A slot about the stock's thickness wide is wider than the stock by less than `slot_fit_clearance`.                                                  |
| `part_too_short`                   | error / warning | The part's length, across its narrowest width, is below `part_length`.                                                                              |
| `part_may_fall_through`            | warning         | The part is shorter than `support_spacing`.                                                                                                         |
| `part_area_too_small`              | error / warning | The part's area (outline less cut-outs) is below `part_area`.                                                                                       |
| `part_too_slender`                 | error / warning | The part's narrowest width is below the `slender_part` tier for its length.                                                                         |
| `cutout_too_slender`               | error / warning | A cut-out's narrowest width is below the `slender_cutout` tier for its length. Round holes are not checked.                                         |
| `notch_too_slender`                | error / warning | An edge notch is narrower than the `slender_cutout` tier for its length.                                                                            |
| `part_too_small`                   | error / warning | The part's narrowest width in any direction is below `part_width`.                                                                                  |
| `web_too_thin`                     | error / warning | The material between two cut-outs is below `web_width`.                                                                                             |
| `features_too_close`               | error / warning | The gap between two cut-outs is below `feature_spacing`.                                                                                            |
| `feature_too_close_to_edge`        | error / warning | The material between a cut-out and the edge is below `edge_distance`.                                                                               |
| `cutout_too_narrow`                | error / warning | The narrowest passage within a cut-out or notch is below `cutout_width`.                                                                            |
| `feature_too_thin`                 | error / warning | The narrowest strip within one outline (a finger, tab or tongue) is below `feature_width`. Tips that narrow to a point are left to `tip_too_sharp`. |
| `internal_corner_too_sharp`        | error / warning | An inside corner's radius is below `internal_corner_radius`.                                                                                        |
| `tip_too_sharp`                    | warning         | An outline comes to a point sharper than `min_tip_angle`.                                                                                           |
| `file_noncut_geometry`             | error           | Geometry is a construction or hidden line ([layers](designs.md#parts-entities-and-layers)); it is left out.                                         |
| `bend_line_not_cut`                | warning         | Geometry is on a bend layer, or is a dashed line across the material. Left out, and checked by the plan's bending step.                             |
| `border_not_cut`                   | warning         | A sheet border is drawn on the cut layer. It is left out; its fix removes it.                                                                       |
| `bend_line_suspected`              | error           | A solid line on a cut layer crosses the material like a bend line. Put it on a `BEND` layer or draw it dashed.                                      |
| `annotation_not_cut`               | warning         | Geometry is on an annotation layer, or is a DXF fill. Left out and not checked.                                                                     |
| `engraving_not_cut`                | warning         | Geometry is on an engraving layer. Left out and not checked.                                                                                        |
| `text_not_outlined`                | warning         | The drawing has `TEXT` or `MTEXT`, which is not cut.                                                                                                |
| `dense_pattern_heat_risk`          | warning         | Cut-outs remove at least `max_removed_area_percent` of a part, or of any `removed_area_window`.                                                     |

Each cut-out reports only its narrowest gap. Gaps of 0.001 mm or less are `geometry_contours_touch_or_cross` instead.

## Bending

Run by a plan's bending step against a press brake capability.

| Code                                       | Severity        | Fails when                                                                                           |
| ------------------------------------------ | --------------- | ---------------------------------------------------------------------------------------------------- |
| `material_unsupported`                     | error           | The press brake does not bend the stock's material.                                                  |
| `material_too_thick` / `material_too_thin` | error           | The stock is outside the material's bending thicknesses.                                             |
| `bend_too_long`                            | error           | A bend line is longer than `max_bend_length`.                                                        |
| `bend_too_short`                           | error           | A bend line is shorter than `min_bend_length`.                                                       |
| `bend_force_too_high`                      | error           | A bend needs more than `max_force` (60 t² L / V with V = 8 t, times `force_factor`).                 |
| `bend_angle_too_large`                     | error           | A bend's angle is larger than `max_angle`.                                                           |
| `bend_angle_too_small`                     | error           | A bend's angle is smaller than `min_angle`.                                                          |
| `bend_radius_not_formed`                   | warning         | A bend's given inside radius is more than 0.01 mm from the radius the tooling forms.                 |
| `bend_radius_too_small`                    | error / warning | A bend's given inside radius is below `min_inside_radius`.                                           |
| `bend_zones_overlap`                       | error           | Two bends are closer than half their bend allowances added together.                                 |
| `flange_too_short`                         | error / warning | The shorter flange beside a bend is below `flange_length`.                                           |
| `bend_tooling_unchecked`                   | review          | No `tooling` is given and two parallel bends turning the same way form a channel.                    |
| `bend_tool_collision`                      | error, review   | No bend order forms the part without hitting the punch or die. Review when the search stopped early. |
| `bend_flanges_collide`                     | error           | Two flanges overlap once every bend is formed.                                                       |
| `feature_too_close_to_bend`                | error / warning | A hole or cut-out is closer to a bend line than `feature_distance`.                                  |
| `bend_relief_missing`                      | error           | A bend line ends where the material beside it stays flat.                                            |
| `bend_corner_relief_missing`               | error           | Two bend lines meet where the material stays whole.                                                  |
| `bend_lines_cross`                         | error           | Two bend lines cross away from their ends.                                                           |
| `bend_relief_too_narrow`                   | error / warning | A bend's relief is narrower than `relief_width`.                                                     |
| `bend_relief_too_shallow`                  | error / warning | A bend's relief reaches past the line less than `relief_depth`.                                      |

## Hole operations

Run by a plan's `countersinking`, `tapping` and `hardware_insertion` steps for the holes assigned an operation of that kind.

| Code                               | Severity        | Fails when                                                                   |
| ---------------------------------- | --------------- | ---------------------------------------------------------------------------- |
| `hole_operation_too_close_to_bend` | error / warning | A hole's edge is closer to a bend line than its operation's `bend_distance`. |
