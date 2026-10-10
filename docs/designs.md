# Reading designs

How DXF, SVG and STEP files become parts, and where reading stops. A reader never drops geometry silently: anything it cannot read is an error, not a pass.

## Parts, entities and layers

- **Parts.** Closed loops form a containment tree: an outer boundary, its cut-outs, and islands inside those. Each outer boundary is a part. A drawing with several is split, reported once as `file_multiple_parts`, and each part is checked on its own; drawing hygiene and the stock are checked once and attributed to the parts involved. Open paths that nearly close are read as a part's broken outline (`outline: "open"`).
- **DXF entities.** `LINE`, `ARC`, `CIRCLE`, `LWPOLYLINE`, `POLYLINE`, `ELLIPSE` and `SPLINE` are cut. `TEXT` and `MTEXT` are `text_not_outlined`. Other annotation (dimensions, hatches, points, leaders) and paper space are ignored. Anything else is `design_content_unsupported`.
- **Blocks.** `INSERT` references are expanded with position, rotation, scale, mirroring, arrays and nesting. Entity IDs are prefixed with the reference's (`1C2/3A`). Undefined or circular blocks are `design_invalid`; external references, or more than 10,000 copies or 50,000 segments, are `design_content_unsupported`.
- **SVG.** Stroked shapes are cut; fill-only shapes are engraving; hidden shapes are left out. All path commands, basic shapes, transforms, `<use>`, style sheets and inherited styles are read. Size comes from `width`, `height` and `viewBox`; pixel sizes are read at the writing program's resolution and reported as `units_assumed`. No color convention is assumed: a capability sets one with `color_operations` and `filled_shapes`.
- **Curves.** Ellipses and splines are replaced by tangent arcs within 0.0005 mm. Fit-point-only splines are `design_content_unsupported`.
- **Layers.** Geometry that is not cut is left out of the part. A layer name word starting with `BEND` or `FOLD` is a bend line; `ENGRAV`, `ETCH`, `MARK`, `RASTER`, `SCORE` or `SCRIBE` is engraving; dimension, center-line, note, title or construction words are annotation. Otherwise an invisible, dashed, hidden or `Defpoints` entity is `file_noncut_geometry`, and every other layer is cut. A straight line across the material from cut to cut is a bend line: `bend_line_not_cut` when dashed, `bend_line_suspected` when solid.
- **Duplicates.** Geometry drawn exactly on top of other geometry is ignored as `geometry_duplicate`; partial overlaps are errors.
- **Units** come from `$INSUNITS`. A file without them is `design_units_unknown` unless the request gives `design.units`, which always overrides the file.
- **Planar only.** Every cut entity must lie on one plane (`geometry_not_flat`); extrusion `(0,0,-1)` is handled.
- **Features.** Holes are full circles. Slots are obrounds and rectangles; keyholes and other narrow shapes are not recognized as slots. Narrow places within one outline are `cutout_too_narrow` and `feature_too_thin`, each outline reporting only its narrowest.
- **Chaining.** Loose segments join greedily in input order. Pieces under 0.001 mm are dropped, then gaps up to `contour_closure_tolerance` are bridged.
- **Envelope** allows 0° or 90° rotation only.

## DXF

The reader (`design/dxfread.go`) is a small, strict group-code reader. Every structural problem is reported with a line number. Binary DXF is decoded to the same group codes, so everything after decoding is shared. It is fuzz-tested.

## STEP

A STEP file (`"format": "step"`) is read into a boundary model (package `solid`) and unfolded into a flat part by package `sheet`:

- The thickness is the distance between the largest flat face and the largest face opposite it.
- That skin is unrolled across each bend at the neutral radius, inside radius + K × thickness, with the press brake's `k_factor` (0.44 by default).
- Every unshared edge is cut; each bend becomes a bend line with the model's angle, radius and direction.
- A solid that is not one sheet (formed features, sharp bends, pockets, meshes) is `design_content_unsupported`. Several solids are laid out side by side as separate parts and must share a thickness.
- `model_thickness_mismatch` fails a model whose sheet differs from the stock. `dfm fix` cannot write into a STEP file; `dfm preview` draws the flat part.

## Limits

A design file over 64 MB, a drawing over 250,000 cut segments or a STEP solid over 5,000 faces is `design_content_unsupported`. A malformed file is `design_invalid`, never a crash: any panic becomes an `internal_error`.
