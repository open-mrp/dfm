package design

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
)

// ignoredEntities are annotation or construction entities that are never cut.
var ignoredEntities = map[string]bool{
	"DIMENSION": true, "ARC_DIMENSION": true, "LARGE_RADIAL_DIMENSION": true, "LEADER": true,
	"OLE2FRAME": true, "OLEFRAME": true,
	"MLEADER": true, "MULTILEADER": true, "HATCH": true, "POINT": true,
	"VIEWPORT": true, "ATTDEF": true, "ATTRIB": true, "IMAGE": true,
	"WIPEOUT": true, "TOLERANCE": true, "XLINE": true, "RAY": true,
}

// unitScale maps $INSUNITS values to millimeters per drawing unit. 0 means unitless and is deliberately absent.
var unitScale = map[int]float64{
	1:  25.4,    // inches
	2:  304.8,   // feet
	4:  1,       // millimeters
	5:  10,      // centimeters
	6:  1000,    // meters
	8:  2.54e-5, // microinches
	9:  0.0254,  // mils
	13: 0.001,   // microns
}

// ParseDXF parses ASCII DXF data into a normalized part.
//
// Supported cut entities: LINE, ARC, CIRCLE, LWPOLYLINE, 2D POLYLINE, ELLIPSE and SPLINE. Ellipses and splines are replaced by arcs within geometry.CurveTolerance. TEXT and MTEXT are not cut but are recorded in Part.Text so the check can report them. Cut entities that are not through-cuts (construction, bend and engraving geometry; see nonCut) are validated, then recorded in Part.NonCut instead of being cut. Other annotation entities (DIMENSION, HATCH, ...) and paper-space entities are ignored. Any other entity type causes ErrUnsupportedContent so that geometry is never silently dropped.
//
// units overrides the file's $INSUNITS declaration; pass "" to use the file. A file without a declaration (missing or 0) requires an override.
func ParseDXF(data []byte, units constants.LengthUnit) (*geometry.Part, error) {
	return parseDXF(data, units, Operations{}, nil, geometry.BuildOptions{})
}

func parseDXF(data []byte, units constants.LengthUnit, ops Operations, adj []Adjustment, opts geometry.BuildOptions) (*geometry.Part, error) {
	f, err := readDXF(data)
	if err != nil {
		return nil, err
	}
	f.ops = ops
	scale, assumed, err := resolveScale(f, units)
	if err != nil {
		return nil, err
	}

	c := converter{scale: scale, xf: identity, zScale: 1}
	var unsupported []string
	if err := c.walk(f, f.entities, inherited{}, 0, &unsupported); err != nil {
		return nil, err
	}
	if len(unsupported) > 0 {
		sort.Strings(unsupported)
		return nil, fmt.Errorf("%w: unsupported entities: %s", ErrUnsupportedContent, strings.Join(unsupported, ", "))
	}
	if err := c.adjust(adj); err != nil {
		return nil, err
	}
	if len(c.paths) == 0 && len(c.circles) == 0 {
		if len(c.nonCut) > 0 {
			return nil, fmt.Errorf("%w: no cut geometry found in ENTITIES section; all %d entities are construction, bend or engraving geometry", ErrInvalid, len(c.nonCut))
		}
		return nil, fmt.Errorf("%w: no cut geometry found in ENTITIES section", ErrInvalid)
	}
	if err := tooManySegments(c.paths, c.circles); err != nil {
		return nil, err
	}
	part := buildWithBendLines(c.paths, c.circles, opts, c.nonCut)
	part.Units = drawingUnits(f, units)
	part.UnitsAssumed = assumed
	part.OffPlane = offPlane(c.heights)
	part.ZeroLength, part.ZeroLengthAt = c.zeroLength, c.zeroAt
	part.Text = c.texts
	part.NonCut = append(c.nonCut, part.NonCut...)
	return part, nil
}

// cutEntities are the entity types converted to cut geometry.
var cutEntities = map[string]bool{
	"LINE": true, "ARC": true, "CIRCLE": true, "LWPOLYLINE": true,
	"POLYLINE": true, "ELLIPSE": true, "SPLINE": true,
}

// nonCut reports whether a cut entity is not a through-cut, and why. Layer names come first: geometry on a bend or fold layer is a bend line, and on an engraving, etching, marking or scoring layer it is engraving. Otherwise the entity is construction geometry when it is invisible, has a line type with gaps (directly or through its layer), or is on a layer that is off, frozen, not plotted or Defpoints.
func (f *dxfFile) nonCut(e rawEntity) (geometry.NonCutKind, bool, error) {
	if !cutEntities[e.kind] {
		return 0, false, nil
	}
	name := e.layer()
	if kind, ok := layerKind(name); ok {
		return kind, true, nil
	}
	if invisible, err := e.int(60, 0); err != nil {
		return 0, false, err
	} else if invisible != 0 {
		return geometry.NonCutConstruction, true, nil
	}
	l := f.layers[strings.ToUpper(name)]
	if l.hidden {
		return geometry.NonCutConstruction, true, nil
	}
	if f.dashed(e) {
		return geometry.NonCutConstruction, true, nil
	}
	// Colors mean nothing unless the capability says what they are for.
	if op, ok := f.ops.colorOperation(f.entityColor(e)); ok {
		switch op {
		case opEngrave:
			return geometry.NonCutEngrave, true, nil
		case opConstruction:
			return geometry.NonCutIgnored, true, nil
		}
	}
	return 0, false, nil
}

// dashed reports whether e is drawn with a line type that has gaps, set on the entity or, by BYLAYER, on its layer.
func (f *dxfFile) dashed(e rawEntity) bool {
	name := e.layer()
	lt := strings.ToUpper(e.str(6))
	switch lt {
	case "", "BYLAYER":
		lt = f.layers[strings.ToUpper(name)].linetype
	case "BYBLOCK":
		lt = "" // outside a block, BYBLOCK draws continuous
	}
	return f.linetypes[lt]
}

// onlyDashed reports whether e is construction geometry only because its
// line type has gaps: it is visible and on a visible layer.
func (f *dxfFile) onlyDashed(e rawEntity) bool {
	name := e.layer()
	invisible, err := e.int(60, 0)
	return err == nil && invisible == 0 && !f.layers[strings.ToUpper(name)].hidden && f.dashed(e)
}

// layerKind classifies a layer by the words in its name (split at anything that is not a letter or digit, ignoring case): a word starting with BEND or FOLD marks bend lines (BEND, "Bend lines", FOLD), unless another word says the lines bound the bend zone (BEND_EXTENT: annotation), one starting with ENGRAV, ETCH, MARK, RASTER, SCORE or SCRIBE marks engraving, and a dimension, center-line, note, title, construction, guide or "delete" word marks annotation (not "template": service templates put the design in a layer inside their template layer).
func layerKind(name string) (geometry.NonCutKind, bool) {
	words := strings.FieldsFunc(strings.ToUpper(name), func(r rune) bool {
		return !('A' <= r && r <= 'Z' || '0' <= r && r <= '9')
	})
	for _, w := range words {
		if strings.HasPrefix(w, "EXTENT") || w == "BOUNDARY" || w == "ZONE" || w == "REGION" {
			for _, v := range words {
				if strings.HasPrefix(v, "BEND") || strings.HasPrefix(v, "FOLD") {
					return geometry.NonCutAnnotation, true
				}
			}
		}
	}
	for _, w := range words {
		for _, p := range []string{"BEND", "FOLD"} {
			if strings.HasPrefix(w, p) {
				return geometry.NonCutBend, true
			}
		}
		for _, p := range []string{"ENGRAV", "ETCH", "MARK", "RASTER", "SCORE", "SCRIBE"} {
			if strings.HasPrefix(w, p) {
				return geometry.NonCutEngrave, true
			}
		}
		if annotationWords[w] || strings.HasPrefix(w, "CENTERLINE") || strings.HasPrefix(w, "CENTRELINE") {
			return geometry.NonCutAnnotation, true
		}
	}
	return 0, false
}

// annotationWords are whole words that name a layer of drawing annotation (whole words, so DIMPLE is not DIM).
var annotationWords = map[string]bool{
	"DIM": true, "DIMS": true, "DIMENSION": true, "DIMENSIONS": true, "CENTER": true, "CENTRE": true,
	"ANNOTATION": true, "ANNOTATIONS": true, "NOTE": true, "NOTES": true, "TITLE": true, "TITLEBLOCK": true,
	"CONSTRUCTION": true, "GUIDE": true, "GUIDES": true, "DELETE": true,
}

// convert adds entity e to c. rest is the entities that follow e; n is how many of them e consumed (a POLYLINE's vertices). Unsupported entity types are appended to unsupported rather than failing at once, so the error can list them all.
func (c *converter) convert(e rawEntity, rest []rawEntity, unsupported []string) (n int, _ []string, err error) {
	switch e.kind {
	case "LINE":
		err = c.line(e)
	case "ARC":
		err = c.arc(e)
	case "CIRCLE":
		err = c.circle(e)
	case "LWPOLYLINE":
		err = c.lwpolyline(e)
	case "ELLIPSE":
		err = c.ellipse(e)
	case "SPLINE":
		err = c.spline(e)
	case "POLYLINE":
		n, err = c.polyline(e, rest)
	case "TEXT", "MTEXT":
		c.text(e)
	case "VERTEX", "SEQEND":
		err = fmt.Errorf("%w: line %d: %s outside POLYLINE", ErrInvalid, e.line, e.kind)
	case "HATCH", "SOLID":
		// A fill is never cut, but a logo drawn only as a fill would be lost: report it rather than drop it silently.
		c.nonCut = append(c.nonCut, geometry.NonCut{EntityID: e.id(), Kind: geometry.NonCutAnnotation})
	default:
		if !ignoredEntities[e.kind] {
			unsupported = append(unsupported, fmt.Sprintf("%s %s (line %d)", e.kind, e.id(), e.line))
		}
	}
	return n, unsupported, err
}

// resolveScale returns millimeters per drawing unit. Units are never guessed: a file without $INSUNITS needs them supplied, and its $MEASUREMENT setting (imperial or metric) is only quoted as a hint.
func resolveScale(f *dxfFile, override constants.LengthUnit) (scale float64, assumed bool, _ error) {
	if override != "" {
		if !override.IsValid() {
			return 0, false, fmt.Errorf("%w: unknown units override %q", ErrInvalid, override)
		}
		return float64(override.Nanometers()) / 1e6, false, nil
	}
	if !f.hasInsUnits || f.insUnits == 0 {
		hint := ""
		switch f.measurement {
		case 0:
			hint = "; its $MEASUREMENT says imperial (likely inches)"
		case 1:
			hint = "; its $MEASUREMENT says metric (likely millimeters)"
		}
		return 0, false, fmt.Errorf("%w: the file does not declare its units ($INSUNITS missing or 0)%s; supply the units explicitly", ErrUnitsUnknown, hint)
	}
	s, ok := unitScale[f.insUnits]
	if !ok {
		return 0, false, fmt.Errorf("%w: unsupported $INSUNITS value %d", ErrUnsupportedContent, f.insUnits)
	}
	return s, false, nil
}

// insUnits maps the $INSUNITS values that name a unit the engine knows.
var insUnits = map[int]constants.LengthUnit{
	1: constants.LengthUnitInch, 2: constants.LengthUnitFoot, 4: constants.LengthUnitMillimeter,
	5: constants.LengthUnitCentimeter, 6: constants.LengthUnitMeter, 13: constants.LengthUnitMicrometer,
}

// drawingUnits is the unit the drawing was read in: the override, else the file's declaration ("" for one without a named unit, such as mils).
func drawingUnits(f *dxfFile, override constants.LengthUnit) constants.LengthUnit {
	if override != "" {
		return override
	}
	return insUnits[f.insUnits]
}

// text records a TEXT or MTEXT entity, which is never cut. Its position is best effort: annotation that cannot be placed is still reported, and never makes the file unreadable.
func (c *converter) text(e rawEntity) {
	t := geometry.Text{EntityID: e.id()}
	o := ocs{} // MTEXT is positioned in world coordinates
	if e.kind == "TEXT" {
		var err error
		if o, err = entityOCS(e); err != nil {
			c.texts = append(c.texts, t)
			return
		}
	}
	if v, err := floats(e, 10, 20); err == nil {
		p := c.pt(o, v[0], v[1])
		t.Position = &p
	}
	c.texts = append(c.texts, t)
}

// ocs describes the transform from an entity's object coordinate system to world XY. Only extrusion (0,0,±1) is supported; (0,0,-1) mirrors X, which also reverses arc direction.
type ocs struct{ mirror bool }

func entityOCS(e rawEntity) (ocs, error) {
	x, err := e.float(210, 0)
	if err != nil {
		return ocs{}, err
	}
	y, err := e.float(220, 0)
	if err != nil {
		return ocs{}, err
	}
	z, err := e.float(230, 1)
	if err != nil {
		return ocs{}, err
	}
	const eps = 1e-9
	if math.Abs(x) > eps || math.Abs(y) > eps || math.Abs(math.Abs(z)-1) > 1e-6 {
		return ocs{}, e.errorf(ErrUnsupportedContent, "is not in the XY plane (extrusion %g,%g,%g)", x, y, z)
	}
	return ocs{mirror: z < 0}, nil
}

// pt maps a point in an entity's object coordinate system to world millimeters.
func (c *converter) pt(o ocs, x, y float64) geometry.Point {
	if o.mirror {
		x = -x
	}
	return c.wpt(x, y)
}

// floats reads the listed group codes, all of which are required.
func floats(e rawEntity, codes ...int) ([]float64, error) {
	out := make([]float64, len(codes))
	for i, code := range codes {
		if !e.has(code) {
			return nil, e.errorf(ErrInvalid, "missing group code %d", code)
		}
		v, err := e.float(code, 0)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (c *converter) line(e rawEntity) error {
	v, err := floats(e, 10, 20, 11, 21)
	if err != nil {
		return err
	}
	z0, err := e.float(30, 0)
	if err != nil {
		return err
	}
	z1, err := e.float(31, 0)
	if err != nil {
		return err
	}
	c.noteZ(e.id(), ocs{}, false, z0, z1)
	// LINE coordinates are in world coordinates, not OCS.
	s, ok := c.seg(false, c.pt(ocs{}, v[0], v[1]), c.pt(ocs{}, v[2], v[3]), 0, e.id())
	if !ok {
		c.noteZeroLength(e.id(), c.pt(ocs{}, v[0], v[1]))
		return nil
	}
	c.paths = append(c.paths, geometry.Contour{Segments: []geometry.Segment{s}})
	return nil
}

func (c *converter) circle(e rawEntity) error {
	o, err := entityOCS(e)
	if err != nil {
		return err
	}
	v, err := floats(e, 10, 20, 40)
	if err != nil {
		return err
	}
	z, err := e.float(30, 0)
	if err != nil {
		return err
	}
	c.noteZ(e.id(), o, true, z)
	if v[2] < 0 {
		return e.errorf(ErrInvalid, "has negative radius %g", v[2])
	}
	if v[2] == 0 {
		c.noteZeroLength(e.id(), c.pt(o, v[0], v[1])) // a point: nothing to cut
		return nil
	}
	if k, ok := c.xf.similar(); ok {
		c.circles = append(c.circles, geometry.Circle{Center: c.pt(o, v[0], v[1]), Radius: v[2] * k * c.scale, EntityIDs: []string{e.id()}})
		return nil
	}
	return c.stretchedArc(e, o, v[0], v[1], v[2], 0, 2*math.Pi)
}

func (c *converter) arc(e rawEntity) error {
	o, err := entityOCS(e)
	if err != nil {
		return err
	}
	v, err := floats(e, 10, 20, 40, 50, 51)
	if err != nil {
		return err
	}
	z, err := e.float(30, 0)
	if err != nil {
		return err
	}
	c.noteZ(e.id(), o, true, z)
	cx, cy, r := v[0], v[1], v[2]
	if r < 0 {
		return e.errorf(ErrInvalid, "has negative radius %g", r)
	}
	if r == 0 {
		c.noteZeroLength(e.id(), c.pt(o, cx, cy)) // a point: nothing to cut
		return nil
	}
	start, end := v[3]*math.Pi/180, v[4]*math.Pi/180
	sweep := math.Mod(end-start, 2*math.Pi)
	if sweep < 0 {
		sweep += 2 * math.Pi
	}
	k, similar := c.xf.similar()
	if !similar {
		if sweep < 1e-12 {
			sweep = 2 * math.Pi
		}
		return c.stretchedArc(e, o, cx, cy, r, start, start+sweep)
	}
	if sweep < 1e-12 {
		// Equal start and end angles: treat as a full circle.
		c.circles = append(c.circles, geometry.Circle{Center: c.pt(o, cx, cy), Radius: r * k * c.scale, EntityIDs: []string{e.id()}})
		return nil
	}
	p0 := c.pt(o, cx+float64(r*math.Cos(start)), cy+float64(r*math.Sin(start)))
	p1 := c.pt(o, cx+float64(r*math.Cos(end)), cy+float64(r*math.Sin(end)))
	s, ok := c.seg(o.mirror, p0, p1, math.Tan(sweep/4), e.id())
	if !ok {
		c.noteZeroLength(e.id())
		return nil
	}
	c.paths = append(c.paths, geometry.Contour{Segments: []geometry.Segment{s}})
	return nil
}

type vertex struct {
	x, y, bulge float64
}

// addPolyline converts a vertex list into a contour. A polyline is closed if its closed flag is set or its first and last vertices coincide. A final vertex repeating the first is an exporter convention for closure, not a zero-length segment, so it is absorbed in either case.
func (c *converter) addPolyline(o ocs, id string, vs []vertex, closedFlag bool) {
	if len(vs) < 2 {
		if len(vs) == 1 {
			c.noteZeroLength(id, c.pt(o, vs[0].x, vs[0].y))
		} else {
			c.noteZeroLength(id)
		}
		return
	}
	n := len(vs)
	if n > 2 && c.pt(o, vs[0].x, vs[0].y).Near(c.pt(o, vs[n-1].x, vs[n-1].y)) {
		closedFlag = true
		n-- // last vertex duplicates the first
	}
	var segs []geometry.Segment
	last := n - 1
	if closedFlag {
		last = n
	}
	// A piece too short to keep is dropped by merging its end vertex into the next piece, so the contour stays continuous.
	from := c.pt(o, vs[0].x, vs[0].y)
	for i := 0; i < last; i++ {
		b := vs[(i+1)%n]
		s, ok := c.seg(o.mirror, from, c.pt(o, b.x, b.y), vs[i].bulge, id)
		if !ok {
			c.noteZeroLength(id)
			continue
		}
		segs = append(segs, s)
		from = s.End
	}
	if len(segs) > 0 {
		c.paths = append(c.paths, geometry.Contour{Segments: segs, Closed: closedFlag})
	}
}

func (c *converter) lwpolyline(e rawEntity) error {
	o, err := entityOCS(e)
	if err != nil {
		return err
	}
	z, err := e.float(38, 0) // elevation
	if err != nil {
		return err
	}
	c.noteZ(e.id(), o, true, z)
	flags, err := e.int(70, 0)
	if err != nil {
		return err
	}
	count, err := e.int(90, -1)
	if err != nil {
		return err
	}
	var vs []vertex
	for _, t := range e.tags {
		switch t.code {
		case 10, 20, 42:
			v, err := parseFloat(t)
			if err != nil {
				return err
			}
			if t.code == 10 {
				vs = append(vs, vertex{x: v})
				continue
			}
			if len(vs) == 0 {
				return fmt.Errorf("%w: line %d: LWPOLYLINE %s group code %d before first vertex", ErrInvalid, t.line, e.id(), t.code)
			}
			if t.code == 20 {
				vs[len(vs)-1].y = v
			} else {
				vs[len(vs)-1].bulge = v
			}
		}
	}
	if count >= 0 && count != len(vs) {
		return e.errorf(ErrInvalid, "declares %d vertices but has %d", count, len(vs))
	}
	c.addPolyline(o, e.id(), vs, flags&1 != 0)
	return nil
}

// polyline converts a POLYLINE and its trailing VERTEX/SEQEND entities. It returns the number of following entities consumed.
func (c *converter) polyline(e rawEntity, rest []rawEntity) (int, error) {
	o, err := entityOCS(e)
	if err != nil {
		return 0, err
	}
	flags, err := e.int(70, 0)
	if err != nil {
		return 0, err
	}
	const is3D, isMesh, isPolyface = 8, 16, 64
	if flags&(isMesh|isPolyface) != 0 {
		return 0, e.errorf(ErrUnsupportedContent, "is a mesh")
	}
	// A 3D polyline is straight segments between world points. Exporters often write flat outlines this way; any vertex off the drawing's plane is reported by its height.
	threeD := flags&is3D != 0
	if threeD {
		o = ocs{}
	} else if z, err := e.float(30, 0); err != nil { // a 2D polyline's elevation
		return 0, err
	} else {
		c.noteZ(e.id(), o, true, z)
	}
	var vs []vertex
	for n, v := range rest {
		switch v.kind {
		case "VERTEX":
			vflags, err := v.int(70, 0)
			if err != nil {
				return 0, err
			}
			const splineFrame = 16
			if vflags&splineFrame != 0 {
				continue // control point, not on the curve
			}
			xyb, err := floats(v, 10, 20)
			if err != nil {
				return 0, err
			}
			bulge, err := v.float(42, 0)
			if err != nil {
				return 0, err
			}
			if threeD {
				z, err := v.float(30, 0)
				if err != nil {
					return 0, err
				}
				c.noteZ(e.id(), o, false, z)
				bulge = 0 // 3D polylines have no arcs
			}
			vs = append(vs, vertex{x: xyb[0], y: xyb[1], bulge: bulge})
		case "SEQEND":
			c.addPolyline(o, e.id(), vs, flags&1 != 0)
			return n + 1, nil
		default:
			return 0, e.errorf(ErrInvalid, "not terminated by SEQEND")
		}
	}
	return 0, e.errorf(ErrInvalid, "not terminated by SEQEND")
}

func (c *converter) ellipse(e rawEntity) error {
	o, err := entityOCS(e)
	if err != nil {
		return err
	}
	v, err := floats(e, 10, 20, 11, 21, 40)
	if err != nil {
		return err
	}
	z, err := e.float(30, 0)
	if err != nil {
		return err
	}
	c.noteZ(e.id(), o, false, z)
	t0, err := e.float(41, 0)
	if err != nil {
		return err
	}
	t1, err := e.float(42, 2*math.Pi)
	if err != nil {
		return err
	}
	ratio := v[4]
	if !(ratio > 0) || math.IsInf(ratio, 0) || math.Hypot(v[2], v[3]) == 0 {
		return e.errorf(ErrInvalid, "has a zero major axis or a ratio that is not positive")
	}
	// Center and major axis are world coordinates; the minor axis is the extrusion direction crossed with the major axis.
	s := 1.0
	if o.mirror {
		s = -1
	}
	if ratio > 1 {
		// Some writers give the "minor" axis as the longer one. It is the same ellipse with the axes swapped and the parameters a quarter turn earlier.
		v[2], v[3] = -s*v[3]*ratio, s*v[2]*ratio
		ratio = 1 / ratio
		t0, t1 = t0-math.Pi/2, t1-math.Pi/2
	}
	// Center and axes are mapped by the placement, which keeps an ellipse an ellipse whatever its scale.
	center := c.wpt(v[0], v[1])
	cx, cy := center.X, center.Y
	mx, my := c.vec(v[2], v[3])
	nx, ny := c.vec(-s*v[3]*ratio, s*v[2]*ratio)
	for t1 <= t0 {
		t1 += 2 * math.Pi
	}
	// An ellipse spans at most one turn; a larger parameter range is the same closed curve, and fitting it turn by turn would only cost time.
	t1 = min(t1, t0+2*math.Pi)
	full := t1-t0 >= 2*math.Pi-1e-9
	if _, similar := c.xf.similar(); full && ratio == 1 && similar {
		c.circles = append(c.circles, geometry.Circle{Center: geometry.Point{X: cx, Y: cy}, Radius: math.Hypot(mx, my), EntityIDs: []string{e.id()}})
		return nil
	}
	curve := func(t float64) (geometry.Point, geometry.Point) {
		cos, sin := math.Cos(t), math.Sin(t)
		// Each product is converted before it is added, so no CPU fuses them (see AGENTS.md).
		return geometry.Point{X: cx + float64(cos*mx) + float64(sin*nx), Y: cy + float64(cos*my) + float64(sin*ny)},
			geometry.Point{X: float64(-sin*mx) + float64(cos*nx), Y: float64(-sin*my) + float64(cos*ny)}
	}
	// Fit at least per quarter turn so the pieces start at the axes.
	n := max(1, int(math.Ceil((t1-t0)/(math.Pi/2)-1e-9)))
	var segs []geometry.Segment
	for i := range n {
		a, b := t0+(t1-t0)*float64(i)/float64(n), t0+(t1-t0)*float64(i+1)/float64(n)
		piece, err := geometry.FitArcs(curve, a, b, e.id())
		if err != nil {
			return fmt.Errorf("%w: ELLIPSE %s (line %d): %v", ErrUnsupportedContent, e.id(), e.line, err)
		}
		if len(segs) > 0 && len(piece) > 0 {
			piece[0].Start = segs[len(segs)-1].End
		}
		segs = append(segs, piece...)
	}
	c.addCurve(segs)
	return nil
}

// stretchedArc converts a circular arc (or a full circle, sweeping 2π) inside a block reference scaled differently along its axes, where it becomes elliptical: it is fitted with arcs like an ellipse.
func (c *converter) stretchedArc(e rawEntity, o ocs, cx, cy, r, t0, t1 float64) error {
	m := 1.0
	if o.mirror {
		m = -1
	}
	curve := func(t float64) (geometry.Point, geometry.Point) {
		cos, sin := math.Cos(t), math.Sin(t)
		dx, dy := c.vec(-m*r*sin, r*cos)
		return c.pt(o, cx+float64(r*cos), cy+float64(r*sin)), geometry.Point{X: dx, Y: dy}
	}
	n := max(1, int(math.Ceil((t1-t0)/(math.Pi/2)-1e-9)))
	var segs []geometry.Segment
	for i := range n {
		a, b := t0+(t1-t0)*float64(i)/float64(n), t0+(t1-t0)*float64(i+1)/float64(n)
		piece, err := geometry.FitArcs(curve, a, b, e.id())
		if err != nil {
			return fmt.Errorf("%w: %s %s (line %d): %v", ErrUnsupportedContent, e.kind, e.id(), e.line, err)
		}
		if len(segs) > 0 && len(piece) > 0 {
			piece[0].Start = segs[len(segs)-1].End
		}
		segs = append(segs, piece...)
	}
	c.addCurve(segs)
	return nil
}

func (c *converter) spline(e rawEntity) error {
	degree, err := e.int(71, 0)
	if err != nil {
		return err
	}
	nCtrl, err := e.int(73, 0)
	if err != nil {
		return err
	}
	nFit, err := e.int(74, 0)
	if err != nil {
		return err
	}
	var knots, weights, zs []float64
	var pts []geometry.Point
	for _, t := range e.tags {
		switch t.code {
		case 40, 41, 10, 20, 30:
			v, err := parseFloat(t)
			if err != nil {
				return err
			}
			switch t.code {
			case 40:
				knots = append(knots, v)
			case 41:
				weights = append(weights, v)
			case 10:
				pts = append(pts, geometry.Point{X: v})
			case 20, 30:
				if len(pts) == 0 {
					return fmt.Errorf("%w: line %d: SPLINE %s group code %d before first control point", ErrInvalid, t.line, e.id(), t.code)
				}
				if t.code == 20 {
					pts[len(pts)-1].Y = v
				} else {
					zs = append(zs, v)
				}
			}
		}
	}
	if len(pts) == 0 && nFit > 0 {
		return e.errorf(ErrUnsupportedContent, "has fit points but no control points; export it with control points")
	}
	if nCtrl != len(pts) {
		return e.errorf(ErrInvalid, "declares %d control points but has %d", nCtrl, len(pts))
	}
	for _, z := range zs {
		if math.Abs(z-zs[0]) > 1e-9*max(1, math.Abs(zs[0])) {
			return e.errorf(ErrUnsupportedContent, "is not in the XY plane")
		}
	}
	if len(weights) > 0 && len(weights) != len(pts) {
		return e.errorf(ErrInvalid, "has %d weights for %d control points", len(weights), len(pts))
	}
	if len(weights) == 0 {
		weights = nil
	}
	if len(zs) > 0 {
		c.noteZ(e.id(), ocs{}, false, zs[0])
	} else {
		c.noteZ(e.id(), ocs{}, false, 0)
	}
	// A NURBS curve is placed by placing its control points.
	for i, p := range pts {
		pts[i] = c.wpt(p.X, p.Y)
	}
	n := geometry.NURBS{Degree: degree, Knots: knots, Points: pts, Weights: weights}
	if err := n.Validate(); err != nil {
		return fmt.Errorf("%w: SPLINE %s (line %d): %v", ErrInvalid, e.id(), e.line, err)
	}
	segs, err := n.Segments(e.id())
	if err != nil {
		return fmt.Errorf("%w: SPLINE %s (line %d): %v", ErrUnsupportedContent, e.id(), e.line, err)
	}
	c.addCurve(segs)
	return nil
}
