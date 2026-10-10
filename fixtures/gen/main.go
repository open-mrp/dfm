// Command gen regenerates the DXF fixture corpus.
//
//	go run ./fixtures/gen                 # from the repository root
//	go test . -run TestFixtures -update   # refresh golden results, then review the diff
//
// Fixtures are complete DXF R2000 drawings (see dxf.go) and are byte-for-byte reproducible. Two fixtures are deliberately not: the R12 file written without handles, and the files in malformed/.
//
// All dimensions are millimeters unless a fixture says otherwise. The corpus is checked against examples/laser.json (bed 1500x3000, min hole 1.5, min slot 1.0).
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if _, err := os.Stat("go.mod"); err != nil {
		log.Fatal("run from the repository root: go run ./fixtures/gen")
	}
	if err := generate("fixtures", "examples"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("fixtures written to fixtures/")
}

// --- shapes -------------------------------------------------------------

// rectLines writes a rectangle as four separate LINE entities.
func (d *doc) rectLines(x, y, w, h float64) {
	d.line(x, y, x+w, y)
	d.line(x+w, y, x+w, y+h)
	d.line(x+w, y+h, x, y+h)
	d.line(x, y+h, x, y)
}

func (d *doc) rect(x, y, w, h float64, closed bool) {
	d.lwpolyline([]vertex{{x, y, 0}, {x + w, y, 0}, {x + w, y + h, 0}, {x, y + h, 0}}, closed)
}

// frame maps local slot coordinates (s along the slot, t across it) to world coordinates for a slot centered at (cx, cy) rotated by deg.
func frame(cx, cy, deg float64) func(s, t float64) (float64, float64) {
	a := deg * math.Pi / 180
	ux, uy := math.Cos(a), math.Sin(a)
	return func(s, t float64) (float64, float64) {
		return cx + float64(s*ux) - float64(t*uy), cy + float64(s*uy) + float64(t*ux)
	}
}

// obroundPolyline writes an obround as one closed LWPOLYLINE whose two semicircular ends are bulge=1 segments.
func (d *doc) obroundPolyline(cx, cy, width, length, deg float64) {
	r, half := width/2, length/2-width/2
	p := frame(cx, cy, deg)
	v := func(s, t, bulge float64) vertex { x, y := p(s, t); return vertex{x, y, bulge} }
	d.lwpolyline([]vertex{v(-half, -r, 0), v(half, -r, 1), v(half, r, 0), v(-half, r, 1)}, true)
}

// obroundEntities writes a horizontal obround as two LINEs and two ARCs.
func (d *doc) obroundEntities(cx, cy, width, length float64) {
	r, half := width/2, length/2-width/2
	d.line(cx-half, cy-r, cx+half, cy-r)
	d.arc(cx+half, cy, r, -90, 90, false)
	d.line(cx+half, cy+r, cx-half, cy+r)
	d.arc(cx-half, cy, r, 90, 270, false)
}

func (d *doc) rectSlot(cx, cy, width, length, deg float64) {
	p := frame(cx, cy, deg)
	var vs []vertex
	for _, c := range [][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		x, y := p(c[0]*length/2, c[1]*width/2)
		vs = append(vs, vertex{x, y, 0})
	}
	d.lwpolyline(vs, true)
}

// roundedRect writes a rounded rectangle as separate LINE and ARC entities, deliberately out of order and with some lines reversed, to exercise contour chaining. With mirrored the corner arcs use extrusion (0,0,-1).
func (d *doc) roundedRect(x, y, w, h, r float64, mirrored bool) {
	lines := [][4]float64{
		{x + r, y, x + w - r, y},
		{x + w, y + h - r, x + w, y + r}, // reversed
		{x + w - r, y + h, x + r, y + h},
		{x, y + r, x, y + h - r}, // reversed
	}
	for _, i := range []int{2, 0, 3, 1} {
		l := lines[i]
		d.line(l[0], l[1], l[2], l[3])
	}
	arcs := []struct{ cx, cy, s, e float64 }{
		{x + w - r, y + r, -90, 0},
		{x + w - r, y + h - r, 0, 90},
		{x + r, y + h - r, 90, 180},
		{x + r, y + r, 180, 270},
	}
	for _, a := range arcs {
		if mirrored {
			// World (cx, cy) is OCS (-cx, cy); a CCW world arc s..e is the CCW OCS arc (180-e)..(180-s).
			d.arc(-a.cx, a.cy, r, 180-a.e, 180-a.s, true)
		} else {
			d.arc(a.cx, a.cy, r, a.s, a.e, false)
		}
	}
}

// hourglass is a cut-out pinched to the given waist at y = 25.
func hourglass(waist float64) []vertex {
	h := waist / 2
	return []vertex{{40, 10, 0}, {60, 10, 0}, {50 + h, 25, 0}, {60, 40, 0}, {40, 40, 0}, {50 - h, 25, 0}}
}

// finger is a 40 x 20 mm base with a 20 mm finger of width w on top.
func finger(w float64) []vertex {
	return []vertex{{0, 0, 0}, {40, 0, 0}, {40, 20, 0}, {20 + w/2, 20, 0}, {20 + w/2, 40, 0}, {20 - w/2, 40, 0}, {20 - w/2, 20, 0}, {0, 20, 0}}
}

// roundedRect is a w x h rectangle at (x, y) with corner radius r, as one closed polyline.
func roundedRect(x, y, w, h, r float64) []vertex {
	const q = 0.41421356237309503 // bulge of a counter-clockwise quarter arc
	return []vertex{
		{x + r, y, 0}, {x + w - r, y, q}, {x + w, y + r, 0}, {x + w, y + h - r, q},
		{x + w - r, y + h, 0}, {x + r, y + h, q}, {x, y + h - r, 0}, {x, y + r, q},
	}
}

// --- fixtures -------------------------------------------------------------

// generate writes the whole corpus under root. Capability sidecars are derived from the example capability in examplesDir.
func generate(root, examplesDir string) error {
	files := map[string][]byte{}
	add := func(group, name string, d *doc) { files[group+"/"+name+".dxf"] = d.bytes() }
	// addBinary writes the drawing as binary DXF.
	addBinary := func(group, name string, d *doc, oneByte bool) error {
		data, err := toBinaryDXF(d.bytes(), oneByte)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", group, name, err)
		}
		files[group+"/"+name+".dxf"] = data
		return nil
	}
	// addWithStock also writes a <name>.stock.json sidecar: the job stock to check this fixture with instead of the corpus default.
	addWithStock := func(group, name string, d *doc, stock string) {
		add(group, name, d)
		files[group+"/"+name+".stock.json"] = []byte(stock + "\n")
	}
	// addWithCapability also writes a <name>.capability.json sidecar: the example capability with extra laser_cutting settings merged in, and extra limits merged into its limits.
	addWithCapability := func(group, name string, d *doc, settings, extraLimits map[string]any) error {
		add(group, name, d)
		data, err := os.ReadFile(filepath.Join(examplesDir, "laser.json"))
		if err != nil {
			return err
		}
		var c map[string]any
		if err := json.Unmarshal(data, &c); err != nil {
			return err
		}
		lc := c["laser_cutting"].(map[string]any)
		for k, v := range settings {
			lc[k] = v
		}
		limits := lc["limits"].(map[string]any)
		for k, v := range extraLimits {
			limits[k] = v
		}
		out, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return err
		}
		files[group+"/"+name+".capability.json"] = append(out, '\n')
		return nil
	}

	// valid: manufacturable with examples/laser.json

	d := newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	add("valid", "valid_rectangle", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 200, 100, true)
	d.circle(20, 50, 0.75) // exactly the 1.5 mm limit
	d.circle(60, 50, 2.5)
	d.circle(120, 50, 5)
	d.arc(170, 50, 3, 0, 180, false) // 6 mm hole drawn as two arcs
	d.arc(170, 50, 3, 180, 360, false)
	add("valid", "valid_holes", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 200, 100, true)
	d.obroundPolyline(40, 30, 1.0, 20, 0) // exactly the 1.0 mm limit
	d.obroundPolyline(100, 50, 6, 40, 30)
	d.rectSlot(160, 50, 2, 30, 90)
	add("valid", "valid_slots", d)

	d = newDoc(unitsMM)
	d.roundedRect(0, 0, 120, 80, 10, false)
	d.circle(60, 40, 4)
	add("valid", "lines_and_arcs", d)

	d = newDoc(unitsMM)
	d.roundedRect(0, 0, 120, 80, 10, true)
	add("valid", "mirrored_arcs", d)

	d = newDoc(unitsMM)
	d.circle(0, 0, 50) // outer profile, not a hole
	d.circle(0, 0, 5)
	add("valid", "disc", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 2900, 1400, true) // fits a 1500x3000 bed only when rotated
	add("valid", "rotated_fit", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 1500, 3000, true) // exactly the bed
	add("valid", "envelope_boundary", d)

	d = newDoc(unitsInch)
	d.rect(0, 0, 4, 2, true)
	d.circle(1, 1, 0.03125) // 1/16" = 1.5875 mm
	add("valid", "inches", d)

	// A hole drawn 5 mm above the plate: a 3D drawing, not a flat part.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.entity("CIRCLE", "AcDbCircle", 10, 50.0, 20, 25.0, 30, 5.0, 40, 5.0)
	add("invalid", "not_flat", d)

	// A 4 x 2 in plate drawn in inches but declared as millimeters: read as 4 x 2 mm, it is implausibly small.
	d = newDoc(unitsMM)
	d.rect(0, 0, 4, 2, true)
	add("invalid", "inch_drawing_declared_mm", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.text(10, 20, 5, "PART-001")
	d.mtext(10, 40, 2.5, "Material: 2mm steel")
	d.point(50, 25)
	d.linearDim(0, 100, 0, -10)
	d.solidHatch(60, 5, 30, 10)
	// Paper-space geometry is not part of the model and must be ignored.
	d.paperEntity("CIRCLE", "AcDbCircle", 10, 10.0, 20, 10.0, 30, 0.0, 40, 0.1)
	add("valid", "with_annotations", d)

	// invalid: not manufacturable with examples/laser.json

	d = newDoc(unitsMM)
	d.rect(0, 0, 1600, 3100, true)
	add("invalid", "too_large", d)

	// Bend lines on a dashed BEND layer, drawn edge to edge across the flat pattern: they are not cut, so they do not split the part.
	d = newDoc(unitsMM)
	d.dashed = true
	d.layers = [][2]string{{"BEND", "DASHED"}}
	d.rect(0, 0, 100, 50, true)
	d.layer = "BEND"
	d.line(30, 0, 30, 50)
	d.line(70, 0, 70, 50)
	add("valid", "bend_lines", d)

	// Holes placed as blocks: as drawn, scaled, in an array, through a nested block turned 90°, and stretched into an ellipse by an uneven scale. Each copy's entities are named after the reference that placed them.
	d = newDoc(unitsMM)
	hole := d.newBlock("HOLE", false)
	d.blockEntity(hole, "CIRCLE", "AcDbCircle", 10, 0.0, 20, 0.0, 30, 0.0, 40, 2.0)
	pair := d.newBlock("PAIR", false)
	d.blockEntity(pair, "INSERT", "AcDbBlockReference", 2, "HOLE", 10, 0.0, 20, 0.0, 30, 0.0)
	d.blockEntity(pair, "INSERT", "AcDbBlockReference", 2, "HOLE", 10, 8.0, 20, 0.0, 30, 0.0)
	d.rect(0, 0, 120, 60, true)
	d.entity("INSERT", "AcDbBlockReference", 2, "HOLE", 10, 15.0, 20, 15.0, 30, 0.0)
	d.entity("INSERT", "AcDbBlockReference", 2, "HOLE", 10, 15.0, 20, 40.0, 30, 0.0, 41, 1.5, 42, 1.5, 43, 1.0)
	d.entity("INSERT", "AcDbBlockReference", 2, "HOLE", 10, 40.0, 20, 15.0, 30, 0.0, 70, 3, 71, 1, 44, 10.0, 45, 0.0)
	d.entity("INSERT", "AcDbBlockReference", 2, "PAIR", 10, 90.0, 20, 15.0, 30, 0.0, 50, 90.0)
	d.entity("INSERT", "AcDbBlockReference", 2, "HOLE", 10, 60.0, 20, 45.0, 30, 0.0, 41, 3.0, 42, 1.0, 43, 1.0)
	add("valid", "blocks", d)

	// The block fixture again as binary DXF: with two-byte group codes, and with the one-byte codes R12 writes.
	for _, b := range []struct {
		name    string
		oneByte bool
	}{{"binary_blocks", false}, {"binary_blocks_r12_codes", true}} {
		if err := addBinary("valid", b.name, d, b.oneByte); err != nil {
			return err
		}
	}

	// SVG drawings. svgFile wraps shapes in an SVG document of the given size, with a viewBox in the same numbers.
	svgFile := func(rootAttrs, body string) []byte {
		return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" ` + rootAttrs + `>
` + body + `</svg>
`)
	}
	// A 100 x 60 mm plate: a round hole, a rounded slot-like cut-out, a cut-out drawn with an arc and a Bezier curve; a filled logo to engrave and a note.
	files["valid/svg_plate.svg"] = svgFile(`width="100mm" height="60mm" viewBox="0 0 100 60"`, `<g fill="none" stroke="#0000ff" stroke-width="0.1">
  <rect id="outline" x="0" y="0" width="100" height="60"/>
  <circle id="hole" cx="15" cy="15" r="4"/>
  <rect id="cutout" x="30" y="10" width="30" height="10" rx="5"/>
  <path id="curved" d="M70 40 A8 8 0 0 1 86 40 C86 50 70 50 70 40 Z"/>
</g>
<path id="logo" d="M10 40 h10 v10 h-10 z" fill="#808080"/>
<text id="note" x="30" y="55">PART-7</text>
`)
	// The same kind of plate sized in pixels, read at 96 per inch.
	files["valid/svg_pixels.svg"] = svgFile(`width="378" height="227" viewBox="0 0 378 227"`, `<g fill="none" stroke="black">
  <rect id="outline" x="0" y="0" width="378" height="227"/>
  <circle id="hole" cx="57" cy="57" r="15"/>
</g>
`)
	// A cut-out drawn as one open path that comes back to its start and runs on over its first edge (an overcut), as some laser templates do.
	files["valid/svg_overcut.svg"] = svgFile(`width="60mm" height="40mm" viewBox="0 0 60 40"`, `<g fill="none" stroke="#0000ff">
  <rect id="outline" x="0" y="0" width="60" height="40"/>
  <path id="cutout" d="m 23,13 h 14 v 14 h -14 v -14 h 14"/>
</g>
`)

	// A plate on the page and an assembly preview drawn beside it, which is cut too unless a capability ignores shapes outside the page.
	files["valid/svg_off_page.svg"] = svgFile(`width="60mm" height="40mm" viewBox="0 0 60 40"`, `<g fill="none" stroke="black">
  <rect id="plate" x="0" y="0" width="60" height="40"/>
  <circle id="hole" cx="30" cy="20" r="3"/>
  <rect id="preview" x="80" y="0" width="30" height="20"/>
</g>
`)

	// A cut-out with a small loop at each corner, as some box generators draw so the beam clears the corner (a corner relief).
	files["valid/svg_corner_loops.svg"] = svgFile(`width="60mm" height="40mm" viewBox="0 0 60 40"`, `<g fill="none" stroke="black">
  <rect id="outline" x="0" y="0" width="60" height="40"/>
  <path id="cutout" d="M 20 15 H 40 C 40.2 15 40 14.8 40 15 V 25 C 40 25.2 40.2 25 40 25 H 20 C 19.8 25 20 25.2 20 25 V 15 C 20 14.8 19.8 15 20 15 Z"/>
</g>
`)

	// A 1 mm hole, below the 1.5 mm minimum.
	files["invalid/svg_small_hole.svg"] = svgFile(`width="100mm" height="50mm" viewBox="0 0 100 50"`, `<g fill="none" stroke="black">
  <rect id="outline" x="0" y="0" width="100" height="50"/>
  <circle id="hole" cx="50" cy="25" r="0.5"/>
</g>
`)

	// A plate drawn with its dimensions and center lines on layers named for them: the annotation is left out.
	d = newDoc(unitsMM)
	d.layers = [][2]string{{"DIMENSION", "Continuous"}, {"CENTERLINE", "Continuous"}}
	d.rect(0, 0, 100, 50, true)
	d.circle(50, 25, 5)
	d.layer = "DIMENSION"
	d.line(0, -10, 100, -10)
	d.layer = "CENTERLINE"
	d.line(40, 25, 60, 25)
	d.line(50, 15, 50, 35)
	add("valid", "annotation_layers", d)

	// A 10 x 5 cm plate drawn in centimeters, with a capability sidecar accepting only millimeters and inches.
	d = newDoc(unitsCM)
	d.rect(0, 0, 10, 5, true)
	if err := addWithCapability("invalid", "centimeters_unsupported", d, map[string]any{"drawing_units": []string{"mm", "in"}}, nil); err != nil {
		return err
	}

	// A logo outline on an engraving layer is not cut through.
	d = newDoc(unitsMM)
	d.layers = [][2]string{{"ENGRAVE", "Continuous"}}
	d.rect(0, 0, 100, 50, true)
	d.layer = "ENGRAVE"
	d.lwpolyline([]vertex{{x: 10, y: 10}, {x: 30, y: 10}, {x: 20, y: 25}}, true)
	add("valid", "engraved_logo", d)

	// Dashed center lines through a hole on the cut layer: they are not cut, and whether they should be is ambiguous.
	d = newDoc(unitsMM)
	d.dashed = true
	d.rect(0, 0, 100, 50, true)
	d.circle(50, 25, 5)
	d.linetype = "DASHED"
	d.line(40, 25, 60, 25)
	d.line(50, 15, 50, 35)
	add("invalid", "center_lines", d)

	// A plate with a line drawn 0.04 mm beside its bottom edge, closer than half the kerf: cut as one kerf, so the copy is dropped with a warning.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.line(0, 0.04, 100, 0.04)
	add("valid", "near_duplicate_line", d)

	// Two parts on a sheet with a double rectangular border drawn around them on the cut layer: the border is left out, and the parts pass.
	d = newDoc(unitsMM)
	d.rect(0, 0, 200, 120, true)
	d.rect(5, 5, 190, 110, true)
	d.rect(20, 20, 60, 40, true)
	d.circle(50, 40, 5)
	d.rect(110, 20, 60, 70, true)
	add("valid", "sheet_border", d)
	// The same parts inside a single rectangle: each part holds a hole, so the rectangle is a border, not a plate with cut-outs.
	d = newDoc(unitsMM)
	d.rect(0, 0, 200, 120, true)
	d.rect(20, 20, 60, 40, true)
	d.circle(50, 40, 5)
	d.rect(110, 20, 60, 70, true)
	d.circle(140, 55, 5)
	add("valid", "sheet_border_single", d)
	// Tab-and-slot slots for 1.5 mm stock: one 1.6 mm wide (0.1 mm clearance), one drawn at exactly 1.5 mm (none), with a capability asking 0.05 mm.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.rect(20, 20, 10, 1.6, true)
	d.rect(60, 20, 10, 1.5, true)
	if err := addWithCapability("invalid", "slot_fit_tight", d, nil, map[string]any{"slot_fit_clearance": map[string]any{"error": map[string]any{"min": map[string]any{"value": "0.05", "unit": "mm"}}}}); err != nil {
		return err
	}
	// A 30 x 20 mm part on a bed whose supports are 50 mm apart: it can drop between them when cut free.
	d = newDoc(unitsMM)
	d.rect(0, 0, 30, 20, true)
	if err := addWithCapability("valid", "part_falls_through", d, map[string]any{"support_spacing": map[string]any{"value": "50", "unit": "mm"}}, nil); err != nil {
		return err
	}

	// Plans: designs checked through their operations (dfm plan), each with a sidecar giving the stock and capabilities (paths or preset IDs). planWithBends also gives the plan request's bends (a JSON array).
	planWithBends := func(name string, d *doc, stock, bends string, caps ...string) {
		add("plan", name, d)
		quoted := make([]string, len(caps))
		for i, c := range caps {
			quoted[i] = `"` + c + `"`
		}
		extra := ""
		if bends != "" {
			extra = `, "bends": ` + bends
		}
		files["plan/"+name+".plan.json"] = []byte(`{ "stock": ` + stock + `, "capabilities": [` + strings.Join(quoted, ", ") + `]` + extra + ` }` + "\n")
	}
	plan := func(name string, d *doc, stock string, caps ...string) { planWithBends(name, d, stock, "", caps...) }
	steel := func(t string) string {
		return `{ "material_code": "mild_steel", "thickness": { "value": "` + t + `", "unit": "mm" } }`
	}
	// bentPlate is a w x h plate with a bend at bendY between square reliefs r wide on each edge.
	reliefPlate := func(w, h, bendY, r float64) *doc {
		d := newDoc(unitsMM)
		d.lwpolyline([]vertex{{x: 0, y: 0}, {x: w, y: 0}, {x: w, y: bendY - r/2}, {x: w - r, y: bendY - r/2}, {x: w - r, y: bendY + r/2},
			{x: w, y: bendY + r/2}, {x: w, y: h}, {x: 0, y: h}, {x: 0, y: bendY + r/2}, {x: r, y: bendY + r/2}, {x: r, y: bendY - r/2}, {x: 0, y: bendY - r/2}}, true)
		d.layers = [][2]string{{"BEND", "Continuous"}}
		d.layer = "BEND"
		d.line(r, bendY, w-r, bendY)
		d.layer = "0"
		return d
	}
	bentPlate := func(w, h, bendY float64) *doc { return reliefPlate(w, h, bendY, 4) }
	laser, brake := "examples/laser.json", "preset:trumpf-trubend-3100"
	// A bracket with one bend between reliefs: cut, then bent.
	d = bentPlate(100, 60, 30)
	d.circle(20, 12, 2.5)
	d.circle(80, 48, 2.5)
	plan("bent_bracket", d, steel("1.5"), laser, brake)
	// The same bracket with no press brake given: bending is not checked.
	d = bentPlate(100, 60, 30)
	d.circle(20, 12, 2.5)
	d.circle(80, 48, 2.5)
	plan("no_press_brake", d, steel("1.5"), laser)
	// A flat plate has only one operation.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	plan("flat_part", d, steel("1.5"), laser, brake)
	// A 4 mm flange beside the bend, below 3 t (4.5 mm) in 1.5 mm steel.
	plan("short_flange", bentPlate(100, 40, 4), steel("1.5"), laser, brake)
	// A hole 1.5 mm from the bend line, below 2 t (3 mm).
	d = bentPlate(100, 60, 30)
	d.circle(50, 33.5, 2)
	plan("hole_near_bend", d, steel("1.5"), laser, brake)
	// A 1192 mm bend on a 1020 mm press brake.
	plan("long_bend", bentPlate(1200, 200, 100), steel("2"), laser, "preset:trumpf-trubend-7036")
	// A 2476 mm bend in 10 mm steel needs about 1820 kN; the brake has 1000.
	plan("heavy_bend", reliefPlate(2500, 300, 150, 12), steel("10"), laser, brake)
	// channel is a 100 x 60 mm plate bent across at y0 and y1 into a U.
	channel := func(y0, y1 float64) *doc {
		d := newDoc(unitsMM)
		d.rect(0, 0, 100, 60, true)
		d.layers = [][2]string{{"BEND", "Continuous"}}
		d.layer = "BEND"
		d.line(0, y0, 100, y0)
		d.line(0, y1, 100, y1)
		d.layer = "0"
		return d
	}
	// A U channel with a 20 mm base between 20 mm sides.
	plan("u_channel", channel(20, 40), steel("1.5"), laser, brake)
	// A U channel whose base is 4 mm between the bends, below 3 t (4.5 mm).
	plan("narrow_channel", channel(28, 32), steel("1.5"), laser, brake)
	// punchDie is a press brake that gives its punch and die: a straight punch 12 mm wide and 100 mm tall, and a 12 mm die.
	punchDie := "fixtures/plan/press_brake_punch_die.json"
	// A U channel 20 mm wide with 20 mm sides: forming the second side swings the first into the straight punch, in either order.
	plan("channel_hits_punch", channel(20, 40), steel("1.5"), laser, punchDie)
	// A U channel 40 mm wide with 10 mm sides clears the punch.
	plan("channel_clears_punch", channel(10, 50), steel("1.5"), laser, punchDie)
	// The same deep channel on a gooseneck punch, whose throat is set back behind the tip, clears: the first side swings into the throat.
	plan("channel_clears_gooseneck", channel(20, 40), steel("1.5"), laser, "fixtures/plan/press_brake_gooseneck.json")
	// tab is a 100 x 60 mm plate with a 40 x 30 mm tab bent up along y=60, the tab's sides dropped depth mm below the bend into slots w mm wide (no slots when depth is 0).
	tab := func(depth, w float64) *doc {
		d := newDoc(unitsMM)
		v := []vertex{{x: 0, y: 0}, {x: 100, y: 0}, {x: 100, y: 60}}
		if depth > 0 {
			v = append(v, vertex{x: 70 + w, y: 60}, vertex{x: 70 + w, y: 60 - depth}, vertex{x: 70, y: 60 - depth})
		}
		v = append(v, vertex{x: 70, y: 60}, vertex{x: 70, y: 90}, vertex{x: 30, y: 90}, vertex{x: 30, y: 60})
		if depth > 0 {
			v = append(v[:len(v)-1], vertex{x: 30, y: 60 - depth}, vertex{x: 30 - w, y: 60 - depth}, vertex{x: 30 - w, y: 60})
		}
		v = append(v, vertex{x: 0, y: 60})
		d.lwpolyline(v, true)
		d.layers = [][2]string{{"BEND", "Continuous"}}
		d.layer = "BEND"
		d.line(30, 60, 70, 60)
		d.layer = "0"
		return d
	}
	// A tab bent off the edge of a plate with its bend ending at the inside corners: the bend tears the plate there.
	plan("tab_without_relief", tab(0, 0), steel("1.5"), laser, brake)
	// The same tab with relief slots reaching 3 mm past the bend.
	plan("tab_with_relief", tab(3, 1.5), steel("1.5"), laser, brake)
	// Reliefs 1 mm wide: above the 0.75 mm minimum (t / 2), below the 1.5 mm warning (t).
	plan("tab_narrow_relief", tab(3, 1), steel("1.5"), laser, brake)
	// A tab whose right relief slot leaves a 0.5 mm strip to the plate's edge (Inventor's minimum remnant): the strip is a thin feature.
	d = newDoc(unitsMM)
	d.lwpolyline([]vertex{{x: 0, y: 0}, {x: 72, y: 0}, {x: 72, y: 60}, {x: 71.5, y: 60}, {x: 71.5, y: 57}, {x: 70, y: 57}, {x: 70, y: 90},
		{x: 30, y: 90}, {x: 30, y: 57}, {x: 28.5, y: 57}, {x: 28.5, y: 60}, {x: 0, y: 60}}, true)
	d.layers = [][2]string{{"BEND", "Continuous"}}
	d.layer = "BEND"
	d.line(30, 60, 70, 60)
	d.layer = "0"
	plan("relief_thin_remnant", d, steel("1.5"), laser, brake)
	// Reliefs reaching 1.5 mm past the bend, below 1.5 t (2.25 mm).
	plan("tab_shallow_relief", tab(1.5, 1.5), steel("1.5"), laser, brake)
	// A U channel whose bends are given: one 90° down at the tooling's 1.5 mm radius, one 45° up with its radius left to the tooling.
	planWithBends("bends_given", channel(20, 40), steel("1.5"), `[{ "entity_ids": ["30"], "angle": { "value": "90", "unit": "deg" }, "direction": "down", "inside_radius": { "value": "1.5", "unit": "mm" } }, { "entity_ids": ["31"], "angle": { "value": "45", "unit": "deg" } }]`, laser, brake)
	// notched is a 100 x 60 mm plate bent across at y = 30 between V notches 1 mm deep at each edge, the bend line inset mm from their tips.
	notched := func(inset float64) *doc {
		d := newDoc(unitsMM)
		d.lwpolyline([]vertex{{x: 0, y: 0}, {x: 100, y: 0}, {x: 100, y: 29}, {x: 99, y: 30}, {x: 100, y: 31}, {x: 100, y: 60},
			{x: 0, y: 60}, {x: 0, y: 31}, {x: 1, y: 30}, {x: 0, y: 29}}, true)
		d.layers = [][2]string{{"BEND", "Continuous"}}
		d.layer = "BEND"
		d.line(1+inset, 30, 99-inset, 30)
		d.layer = "0"
		return d
	}
	// A bend ending at the tips of V notches: the notches relieve it.
	plan("v_notch_relief", notched(0), steel("1.5"), laser, brake)
	// The same bend line stopping 1 mm short of the notches, as some CAD draws fold lines: it bends to them.
	plan("bend_line_short", notched(1), steel("1.5"), laser, brake)
	// A tapped M3 hole 2.75 mm from the bend, against a laser capability whose taps keep 4 mm from bends, and an M3 nut pressed in far from it: cut, tap, bend, then press the nut in.
	{
		data, err := os.ReadFile(filepath.Join(examplesDir, "laser.json"))
		if err != nil {
			return err
		}
		var c map[string]any
		if err := json.Unmarshal(data, &c); err != nil {
			return err
		}
		for _, op := range c["hole_operations"].([]any) {
			if o := op.(map[string]any); o["kind"] == "tap" {
				o["bend_distance"] = map[string]any{"error": map[string]any{"min": map[string]any{"value": "4", "unit": "mm"}}}
			}
		}
		out, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return err
		}
		files["plan/laser_tap_bend_distance.json"] = append(out, '\n')
	}
	d = bentPlate(100, 60, 30)
	d.circle(50, 34, 1.25)
	d.circle(20, 50, 2.11)
	add("plan", "tapped_near_bend", d)
	files["plan/tapped_near_bend.plan.json"] = []byte(`{ "stock": ` + steel("1.5") + `, "capabilities": ["fixtures/plan/laser_tap_bend_distance.json", "` + brake + `"], "hole_operations": [{ "operation": "tap_m3", "holes": { "diameter": { "value": "2.5", "unit": "mm" } } }, { "operation": "nut_m3_steel", "holes": { "diameter": { "value": "4.22", "unit": "mm" } }, "side": "top" }] }` + "\n")
	// STEP models of sheet metal, unfolded before they are checked: a 60 mm wide bracket in 1.5 mm steel, a 40 mm flange with a 6 mm hole bent 90° up at 1.5 mm into a 30 mm flange.
	stepBracket := sheetModel{W: 60, A: 40, T: 1.5, Angle: 90, R: 1.5, B: 30, Holes: [][3]float64{{30, 15, 3}}}
	planStep := func(name string, m sheetModel, stock string, caps ...string) {
		files["plan/"+name+".step"] = m.bytes()
		quoted := make([]string, len(caps))
		for i, c := range caps {
			quoted[i] = `"` + c + `"`
		}
		files["plan/"+name+".plan.json"] = []byte(`{ "stock": ` + stock + `, "capabilities": [` + strings.Join(quoted, ", ") + `] }` + "\n")
	}
	planStep("step_bracket", stepBracket, steel("1.5"), laser, brake)
	// A base with a wall up each of two adjacent edges and a tab on each wall's side edge, the tabs pointing along the walls. In the model nothing touches (one wall stops 20 mm short of the corner, the tabs clear each other); unfolded, the two tabs land on the same corner of the flat, so the part cannot be cut from one sheet.
	cornerTabs := plateModel{T: 1.5, A: 80, B: 60, Bends: []plateBend{
		{Parent: 0, Edge: 1, S0: 0, S1: 60, Angle: 90, R: 1.5, Length: 30}, // wall A up the x = 80 edge
		{Parent: 0, Edge: 0, S0: 0, S1: 60, Angle: 90, R: 1.5, Length: 30}, // wall B up the y = 60 edge, stopping 20 mm short of the corner
		{Parent: 1, Edge: 3, S0: 5, S1: 25, Angle: 90, R: 1.5, Length: 15}, // a tab on wall A's edge at the corner, pointing along wall B's line
		{Parent: 2, Edge: 1, S0: 5, S1: 25, Angle: 90, R: 1.5, Length: 30}, // a tab on wall B's free end, pointing along wall A's line
	}}
	files["plan/corner_tabs_overlap.step"] = cornerTabs.bytes()
	// The same model as a laser-cutting fixture: unfolded, it is not manufacturable.
	files["invalid/corner_tabs_overlap.step"] = cornerTabs.bytes()
	files["plan/corner_tabs_overlap.plan.json"] = []byte(`{ "stock": ` + steel("1.5") + `, "capabilities": ["` + laser + `", "` + brake + `"] }` + "\n")
	// The same bracket with a 0.5 mm inside radius, which the tooling does not form (it forms 1.5 mm).
	tight := stepBracket
	tight.R = 0.5
	planStep("step_tight_bend", tight, steel("1.5"), laser, brake)
	// Drawn at 0.2 mm, below 0.25 t (0.375 mm): no brake forms it.
	sharp := stepBracket
	sharp.R = 0.2
	planStep("step_sharp_bend", sharp, steel("1.5"), laser, brake)
	// A flat 1.5 mm plate checked as 2 mm stock: the model is not the stock's thickness.
	files["invalid/step_thickness_mismatch.step"] = sheetModel{W: 60, A: 40, T: 1.5, Holes: [][3]float64{{30, 20, 3}}}.bytes()
	files["invalid/step_thickness_mismatch.stock.json"] = []byte(steel("2") + "\n")
	// The same mismatch on a plate whose hole is tapped: assigning the operation rebuilds the part, which must keep the model's thickness.
	files["invalid/step_tapped_thickness_mismatch.step"] = sheetModel{W: 60, A: 40, T: 1.5, Holes: [][3]float64{{30, 20, 1.65}}}.bytes()
	files["invalid/step_tapped_thickness_mismatch.stock.json"] = []byte(steel("2") + "\n")
	files["invalid/step_tapped_thickness_mismatch.hole_operations.json"] = []byte(`[
  { "operation": "tap_m4", "holes": { "diameter": { "value": "3.3", "unit": "mm" } } }
]
`)
	// Two plates in one file: unfolded side by side, two parts.
	files["valid/step_two_plates.step"] = stepFile(sheetModel{W: 60, A: 40, T: 1.5, Holes: [][3]float64{{30, 20, 3}}}, sheetModel{W: 30, A: 20, T: 1.5, DX: 100})
	// A plate with a blind pocket 0.5 mm deep: the thickness varies, so it cannot be cut from flat sheet.
	files["malformed/step_pocket.step"] = sheetModel{W: 60, A: 40, T: 1.5, Pockets: [][4]float64{{30, 20, 5, 0.5}}}.bytes()
	// The flat plate in its own thickness.
	files["valid/step_plate.step"] = sheetModel{W: 60, A: 40, T: 1.5, Holes: [][3]float64{{30, 20, 3}}}.bytes()
	// tooling is a press brake whose tooling forms at most 135° and an inside radius of 1 t, 2 t in 5052 aluminum.
	tooling := "fixtures/plan/press_brake_tooling.json"
	// tray is a 50 x 50 mm base with a 20 mm wall on its bottom and right edges; drawn without relief, the walls are the base's full width and the bends meet at its corner (70, 20). With relief, each wall ends 3 mm (the radius plus the thickness) short of the other's bend line and the 3 mm square between them is cut from the base's corner, so the bends end on edges that cross them and the folded walls clear each other (a wall drawn to the corner would end 0.2 mm inside the other: the arc pulls a flange inside its bend line).
	tray := func(relief bool) *doc {
		d := newDoc(unitsMM)
		corner := []vertex{{x: 70, y: 20}}
		end1, start2 := 70.0, 20.0
		if relief {
			corner = []vertex{{x: 67, y: 0}, {x: 67, y: 23}, {x: 90, y: 23}}
			end1, start2 = 67, 23
		}
		v := []vertex{{x: 20, y: 0}}
		if !relief {
			v = append(v, vertex{x: 70, y: 0})
		}
		v = append(v, corner...)
		if !relief {
			v = append(v, vertex{x: 90, y: 20})
		}
		d.lwpolyline(append(v, vertex{x: 90, y: 70}, vertex{x: 20, y: 70}), true)
		d.layers = [][2]string{{"BEND", "Continuous"}}
		d.layer = "BEND"
		d.line(20, 20, end1, 20)
		d.line(70, start2, 70, 70)
		d.layer = "0"
		return d
	}
	// Bends meeting at the corner with no relief: the corner tears.
	plan("box_corner_no_relief", tray(false), steel("1.5"), laser, brake)
	// The same with a corner relief both bends end on, the walls set back so they clear once folded.
	plan("box_corner_relief", tray(true), steel("1.5"), laser, brake)
	// lippedTray is a tray like the relieved one with a 70 x 70 base at (40, 40), 15 mm walls bent at y = 40 and x = 110, and a 15 mm lip folded in over the base from each, bent at y = 25 and x = 125. Mitered, each lip's end is cut at 45° from its root at the corner to its tip, so the two meet along the corner's diagonal (the base is wide enough that the miter stays in the end tenth of the bend, where flange_too_short does not look); otherwise they are the walls' full width and lie over each other at the corner.
	lippedTray := func(mitered bool) *doc {
		d := newDoc(unitsMM)
		v := []vertex{{x: 40, y: 10}}
		if mitered {
			v = append(v, vertex{x: 92, y: 10}, vertex{x: 107, y: 25}, vertex{x: 107, y: 43}, vertex{x: 125, y: 43}, vertex{x: 140, y: 58})
		} else {
			v = append(v, vertex{x: 107, y: 10}, vertex{x: 107, y: 43}, vertex{x: 140, y: 43})
		}
		d.lwpolyline(append(v, vertex{x: 140, y: 110}, vertex{x: 40, y: 110}), true)
		d.layers = [][2]string{{"BEND", "Continuous"}}
		d.layer = "BEND"
		d.line(40, 40, 107, 40)
		d.line(40, 25, 107, 25)
		d.line(110, 43, 110, 110)
		d.line(125, 43, 125, 110)
		d.layer = "0"
		return d
	}
	// Lips folded in from two walls that meet at a corner, each the wall's full width: folded, they lie over each other (bend_flanges_collide).
	plan("box_lips_collide", lippedTray(false), steel("1.5"), laser, brake)
	// The same lips mitered at the corner: they clear.
	plan("box_lips_mitered", lippedTray(true), steel("1.5"), laser, brake)
	// A 100 x 40 mm base with a 15 mm wall up, a 15 mm lip folded in over the base and a 25 mm return folded down from the lip: deeper than the wall is tall, the return passes through the base (bend_flanges_collide).
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 95, true)
	d.layers = [][2]string{{"BEND", "Continuous"}}
	d.layer = "BEND"
	d.line(0, 55, 100, 55)
	d.line(0, 40, 100, 40)
	d.line(0, 25, 100, 25)
	d.layer = "0"
	plan("return_hits_base", d, steel("1.5"), laser, brake)
	// Two bends crossing in the middle of a plate.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 100, true)
	d.layers = [][2]string{{"BEND", "Continuous"}}
	d.layer = "BEND"
	d.line(0, 50, 100, 50)
	d.line(50, 0, 50, 100)
	d.layer = "0"
	plan("bend_lines_cross", d, steel("1.5"), laser, brake)
	// An 8 mm bend where the tooling holds at least 10 mm.
	plan("bend_too_short", reliefPlate(100, 60, 30, 46), steel("1.5"), laser, tooling)
	// A 5° bend where the brake holds at least 10°.
	planWithBends("bend_too_shallow", bentPlate(100, 60, 30), steel("1.5"), `[{ "entity_ids": ["30"], "angle": { "value": "5", "unit": "deg" } }]`, laser, tooling)
	// A 150° bend where the tooling forms at most 135°.
	planWithBends("bend_too_sharp", bentPlate(100, 60, 30), steel("1.5"), `[{ "entity_ids": ["30"], "angle": { "value": "150", "unit": "deg" } }]`, laser, tooling)
	// Two bends 3 mm apart in 1.5 mm steel: each 90° bend at the 1.5 mm radius takes up about 3.39 mm of flat, so their zones overlap.
	plan("bend_zones_overlap", channel(28.5, 31.5), steel("1.5"), laser, brake)
	// A hole whose edge is 5 mm from the bend in 2 mm 5052, which the tooling bends at 4 mm: t + R is 6 mm (it would be 4 mm at R = t).
	d = bentPlate(100, 60, 30)
	d.circle(50, 38, 3)
	plan("hole_near_wide_bend", d, `{ "material_code": "aluminum_5052", "thickness": { "value": "2", "unit": "mm" } }`, laser, tooling)
	// A bend drawn for a 1 mm inside radius in 2 mm 5052 aluminum, which the tooling forms at 4 mm.
	planWithBends("bend_radius_not_formed", bentPlate(100, 60, 30), `{ "material_code": "aluminum_5052", "thickness": { "value": "2", "unit": "mm" } }`, `[{ "entity_ids": ["30"], "inside_radius": { "value": "1", "unit": "mm" } }]`, laser, tooling)

	// A 100 x 60 mm flat pattern with 4 mm bend reliefs and a dashed bend line on the cut layer between them, the way quote services ask bend lines to be drawn: it is a bend line, not construction geometry.
	bendPlate := func() *doc {
		d := newDoc(unitsMM)
		d.dashed = true
		d.lwpolyline([]vertex{{x: 0, y: 0}, {x: 100, y: 0}, {x: 100, y: 28}, {x: 96, y: 28}, {x: 96, y: 32},
			{x: 100, y: 32}, {x: 100, y: 60}, {x: 0, y: 60}, {x: 0, y: 32}, {x: 4, y: 32}, {x: 4, y: 28}, {x: 0, y: 28}}, true)
		d.circle(20, 15, 2.5)
		d.circle(80, 45, 2.5)
		return d
	}
	d = bendPlate()
	d.linetype = "DASHED"
	d.line(4, 30, 96, 30)
	add("valid", "dashed_bend_line", d)

	// The same flat pattern with the bend line drawn solid on the cut layer: cutting it would cut the part in two, so it is a suspected bend line.
	d = bendPlate()
	d.line(4, 30, 96, 30)
	add("invalid", "solid_bend_line", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 1600, 2000, true) // long side fits, short side does not
	add("invalid", "too_large_short_side", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(25, 25, 0.4) // 0.8 mm
	d.circle(75, 25, 5)
	add("invalid", "small_hole", d)

	// Holes drawn as 36-sided polygons, as mesh exports write them: a 1 mm hole is too small, a 10 mm one is fine; both are faceted.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	for _, h := range [][3]float64{{25, 25, 0.5}, {75, 25, 5}} {
		var vs []vertex
		for i := range 36 {
			a := float64(i) * math.Pi / 18
			vs = append(vs, vertex{x: h[0] + float64(h[2]*math.Cos(a)), y: h[1] + float64(h[2]*math.Sin(a))})
		}
		d.lwpolyline(vs, true)
	}
	add("invalid", "faceted_holes", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(50, 25, 0.745) // 1.49 mm, just under 1.5
	add("invalid", "hole_below_limit", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, false)
	add("invalid", "open_contour", d)

	d = newDoc(unitsMM)
	d.line(0, 0, 100, 0)
	d.line(100, 0, 100, 50)
	d.line(100, 50, 0, 50)
	d.line(0, 50, 0, 0.1) // 0.1 mm short of closing: beyond the closure tolerance
	add("invalid", "open_gap", d)

	d = newDoc(unitsMM)
	d.line(0, 0, 100, 0)
	d.line(100, 0, 100, 50)
	d.line(100, 50, 0, 50)
	d.line(0, 50, 0, 0.02) // 0.02 mm short of closing: within the closure tolerance
	add("valid", "small_gap", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.obroundEntities(50, 25, 0.8, 20)
	add("invalid", "narrow_slot", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.rectSlot(50, 25, 0.5, 10, 45)
	add("invalid", "narrow_rect_slot", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 1600, 3100, true)
	d.circle(100, 100, 0.5)
	d.obroundPolyline(300, 300, 0.6, 30, 0)
	d.line(500, 500, 600, 600) // stray open cut
	add("invalid", "multiple_violations", d)

	// drawing hygiene and topology

	d = newDoc(unitsMM)
	d.rectLines(0, 0, 100, 50)
	d.line(0, 0, 100, 0) // bottom edge drawn twice
	add("valid", "duplicate_line", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(50, 25, 5)
	d.circle(50, 25, 5) // same hole drawn twice
	add("valid", "duplicate_circle", d)

	d = newDoc(unitsMM)
	d.line(0, 0, 60, 0)
	d.line(50, 0, 100, 0) // overlaps the first line by 10 mm and leaves it unjoined
	d.line(100, 0, 100, 50)
	d.line(100, 50, 0, 50)
	d.line(0, 50, 0, 0)
	add("invalid", "overlapping_edge", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.lwpolyline([]vertex{{30, 10, 0}, {70, 40, 0}, {30, 40, 0}, {70, 10, 0}}, true) // bow-tie cut-out
	add("invalid", "self_intersecting_cutout", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(40, 25, 5)
	d.circle(49, 25, 5) // overlaps the first hole
	add("invalid", "holes_crossing", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(40, 25, 5)
	d.circle(50, 25, 5) // exactly tangent: zero material between the holes
	add("invalid", "holes_tangent", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(40, 25, 5)
	d.circle(50.01, 25, 5)                    // 0.01 mm apart: not touching
	add("invalid", "holes_nearly_tangent", d) // not touching, but far too thin a web

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(5, 25, 5) // touches the left edge
	add("invalid", "hole_touching_edge", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 40, 40, true)
	d.rect(60, 0, 40, 40, true) // a second, separate part
	add("valid", "two_parts", d)

	// Hole operations, assigned by the <name>.hole_operations.json sidecar. Four 3.3 mm holes tapped M4, two 5 mm holes fitted with M3 nuts (resized to 4.22 mm) and a 6 mm hole left plain.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	for _, c := range [][2]float64{{10, 10}, {90, 10}, {90, 50}, {10, 50}} {
		d.circle(c[0], c[1], 1.65)
	}
	d.circle(35, 30, 2.5)
	d.circle(65, 30, 2.5)
	d.circle(50, 30, 3)
	add("valid", "tapped_plate", d)
	files["valid/tapped_plate.hole_operations.json"] = []byte(`[
  { "operation": "tap_m4", "holes": { "diameter": { "value": "3.3", "unit": "mm" } } },
  { "operation": "nut_m3_steel", "holes": { "diameter": { "value": "5", "unit": "mm" } }, "side": "top" }
]
`)

	// Two M4 countersinks in 6 mm aluminum, opening on the bottom face.
	d = newDoc(unitsMM)
	d.rect(0, 0, 80, 40, true)
	d.circle(20, 20, 2.02)
	d.circle(60, 20, 2.02)
	addWithStock("valid", "countersunk_plate", d, `{ "material_code": "aluminum_5052", "thickness": { "value": "6", "unit": "mm" } }`)
	files["valid/countersunk_plate.hole_operations.json"] = []byte(`[
  { "operation": "csk_m4", "holes": { "entity_ids": ["30", "31"] }, "side": "bottom" }
]
`)

	// Hole operations that cannot be done in 1.5 mm mild steel: an M4 tap 2.5 mm from the edge, two M4 nuts 6 mm apart, an M4 countersink (not offered in steel, and deeper than 60% of 1.5 mm), a 6 mm hole given two taps, a 7 mm diameter and the plate's outline that are not holes.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	d.circle(2.5, 30, 1.65)
	d.circle(40, 30, 2.705)
	d.circle(46, 30, 2.705)
	d.circle(70, 30, 2.02)
	d.circle(85, 15, 3)
	add("invalid", "hole_operation_problems", d)
	files["invalid/hole_operation_problems.hole_operations.json"] = []byte(`[
  { "operation": "tap_m4", "holes": { "diameter": { "value": "3.3", "unit": "mm" } } },
  { "operation": "nut_m4_steel", "holes": { "diameter": { "value": "5.41", "unit": "mm" } }, "side": "top" },
  { "operation": "csk_m4", "holes": { "diameter": { "value": "4.04", "unit": "mm" } }, "side": "top" },
  { "operation": "tap_m6", "holes": { "diameter": { "value": "6", "unit": "mm" } } },
  { "operation": "tap_m8", "holes": { "diameter": { "value": "6", "unit": "mm" } } },
  { "operation": "tap_m5", "holes": { "diameter": { "value": "7", "unit": "mm" } } },
  { "operation": "tap_m5", "holes": { "entity_ids": ["2F"] } }
]
`)

	// L brackets on a sheet: parts 1, 2 (turned 90°) and 5 (turned 180°) are copies; part 3 is a mirror image, part 4 has a larger hole and part 6 has a line drawn twice, so they are listed apart.
	d = newDoc(unitsMM)
	bracket := func(x, y, deg float64, mirror bool, holeR float64, twice bool) {
		s, c := math.Sincos(deg * math.Pi / 180)
		at := func(px, py float64) (float64, float64) {
			if mirror {
				px = -px
			}
			return x + float64(c*px) - float64(s*py), y + float64(s*px) + float64(c*py)
		}
		var vs []vertex
		for _, p := range [][2]float64{{0, 0}, {60, 0}, {60, 15}, {15, 15}, {15, 40}, {0, 40}} {
			px, py := at(p[0], p[1])
			vs = append(vs, vertex{x: px, y: py})
		}
		d.lwpolyline(vs, true)
		hx, hy := at(7.5, 7.5)
		d.circle(hx, hy, holeR)
		if twice {
			x0, y0 := at(0, 0)
			x1, y1 := at(60, 0)
			d.line(x0, y0, x1, y1)
		}
	}
	bracket(0, 0, 0, false, 2, false)
	bracket(150, 0, 90, false, 2, false)
	bracket(250, 0, 0, true, 2, false)
	bracket(300, 0, 0, false, 3, false)
	bracket(60, 120, 180, false, 2, false)
	bracket(100, 100, 0, false, 2, true)
	add("valid", "identical_parts", d)

	// Three parts nested on one sheet; only the middle one has a hole below the 1.5 mm minimum, so only part 2 is not manufacturable.
	d = newDoc(unitsMM)
	d.rect(0, 0, 40, 40, true)
	d.circle(20, 20, 5)
	d.rect(50, 0, 40, 40, true)
	d.circle(70, 20, 0.5)
	d.rect(100, 0, 40, 40, true)
	add("invalid", "nested_sheet", d)

	// Drawing problems on a nested sheet are reported against the part they are in: a hole drawn twice in part 1, an edge that parts 2 and 3 both draw (common-line cutting), part 4's outline 0.5 mm short of closing with a hole inside it, and a note that is in no part.
	d = newDoc(unitsMM)
	d.rect(0, 0, 40, 40, true)
	d.circle(20, 20, 5)
	d.circle(20, 20, 5)
	d.rect(50, 0, 40, 40, true)
	d.rect(90, 0, 40, 40, true)
	d.line(140, 0, 180, 0)
	d.line(180, 0, 180, 40)
	d.line(180, 40, 140, 40)
	d.line(140, 40, 140, 0.5)
	d.circle(160, 20, 5)
	d.text(0, 60, 5, "SHEET-7")
	add("invalid", "nested_sheet_problems", d)

	// A plate with a hole nested in a frame's window, for a capability that accepts parts in cut-outs: two parts, each checked on its own.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 100, true)
	d.rect(10, 10, 80, 80, true)
	d.rect(30, 30, 40, 40, true)
	d.circle(50, 50, 5)
	if err := addWithCapability("valid", "part_in_cutout", d, map[string]any{"multiple_parts": "accepted_in_cutouts"}, nil); err != nil {
		return err
	}

	// Three 40 x 30 mm panels side by side sharing their cut lines (common-line cutting): the first drawn closed, the others as open U shapes using the edge of the one before, each with a hole. Rejected by default; the accepted copy has a capability sidecar that accepts common lines.
	panels := func() *doc {
		d := newDoc(unitsMM)
		d.rect(0, 0, 40, 30, true)
		d.lwpolyline([]vertex{{x: 40, y: 0}, {x: 80, y: 0}, {x: 80, y: 30}, {x: 40, y: 30}}, false)
		d.lwpolyline([]vertex{{x: 80, y: 0}, {x: 120, y: 0}, {x: 120, y: 30}, {x: 80, y: 30}}, false)
		for _, x := range []float64{20, 60, 100} {
			d.circle(x, 15, 3)
		}
		return d
	}
	add("invalid", "common_line", panels())
	if err := addWithCapability("valid", "common_line_accepted", panels(), map[string]any{"common_line": "accepted"}, nil); err != nil {
		return err
	}

	// A 30 x 12 mm tag, below a 15 x 40 mm minimum part size: wide enough, too short.
	d = newDoc(unitsMM)
	d.rect(0, 0, 30, 12, true)
	mm := func(v string) map[string]any { return map[string]any{"value": v, "unit": "mm"} }
	if err := addWithCapability("invalid", "short_part", d, nil, map[string]any{
		"part_width":  map[string]any{"error": map[string]any{"min": mm("10")}},
		"part_length": map[string]any{"error": map[string]any{"min": mm("40")}},
	}); err != nil {
		return err
	}

	// A 60 x 60 mm L bracket with 8 mm legs: it passes a 20 x 40 mm minimum part size but its area, 896 mm², is below a 1000 mm² minimum.
	d = newDoc(unitsMM)
	d.lwpolyline([]vertex{{x: 0, y: 0}, {x: 60, y: 0}, {x: 60, y: 8}, {x: 8, y: 8}, {x: 8, y: 60}, {x: 0, y: 60}}, true)
	if err := addWithCapability("invalid", "small_area_part", d, nil, map[string]any{
		"part_width":  map[string]any{"error": map[string]any{"min": mm("20")}},
		"part_length": map[string]any{"error": map[string]any{"min": mm("40")}},
		"part_area":   map[string]any{"error": map[string]any{"value": "1000", "unit": "mm2"}},
	}); err != nil {
		return err
	}

	// A 300 x 200 mm plate with 64 holes packed into one corner: the whole plate loses 6% of its area, but a 60 mm window there loses more than half.
	d = newDoc(unitsMM)
	d.rect(0, 0, 300, 200, true)
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			d.circle(15+10*float64(col), 15+10*float64(row), 4.3)
		}
	}
	if err := addWithCapability("valid", "dense_patch", d, nil, map[string]any{"removed_area_window": mm("60")}); err != nil {
		return err
	}

	// A 200 x 120 mm plate with a 3 mm notch 110 mm deep from its top edge, where cut-outs (and notches) from 100 mm long must be 5 mm wide.
	d = newDoc(unitsMM)
	d.lwpolyline([]vertex{{x: 0, y: 0}, {x: 200, y: 0}, {x: 200, y: 120}, {x: 101.5, y: 120}, {x: 101.5, y: 10}, {x: 98.5, y: 10}, {x: 98.5, y: 120}, {x: 0, y: 120}}, true)
	if err := addWithCapability("invalid", "slender_notch", d, nil, map[string]any{
		"slender_cutout": []any{map[string]any{"min_length": mm("100"), "width": map[string]any{"error": map[string]any{"min": mm("5")}}}},
	}); err != nil {
		return err
	}

	// A 200 x 60 mm plate with a 150 x 3 mm cut-out, where cut-outs from 100 mm long must be 5 mm wide.
	d = newDoc(unitsMM)
	d.rect(0, 0, 200, 60, true)
	d.rect(25, 28.5, 150, 3, true)
	if err := addWithCapability("invalid", "slender_cutout", d, nil, map[string]any{
		"slender_cutout": []any{map[string]any{"min_length": mm("100"), "width": map[string]any{"error": map[string]any{"min": mm("5")}}}},
	}); err != nil {
		return err
	}

	// A 300 x 12 mm strip, where parts from 100 mm long must be 15 mm wide.
	d = newDoc(unitsMM)
	d.rect(0, 0, 300, 12, true)
	if err := addWithCapability("invalid", "slender_strip", d, nil, map[string]any{
		"slender_part": []any{
			map[string]any{"min_length": mm("100"), "width": map[string]any{"error": map[string]any{"min": mm("15")}}},
			map[string]any{"min_length": mm("750"), "width": map[string]any{"error": map[string]any{"min": mm("40")}}},
		},
	}); err != nil {
		return err
	}

	// Two parts side by side, for a capability that requires one part per drawing.
	d = newDoc(unitsMM)
	d.rect(0, 0, 40, 40, true)
	d.rect(60, 0, 40, 40, true)
	if err := addWithCapability("invalid", "one_part_per_drawing", d, map[string]any{"multiple_parts": "rejected"}, nil); err != nil {
		return err
	}

	d = newDoc(unitsMM)
	d.rect(0, 0, 50, 50, true)
	d.circle(25, 25, 10) // Ø20 cut-out ...
	d.circle(25, 25, 5)  // ... with a Ø10 disc of material left loose inside it
	add("invalid", "floating_island", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.line(20, 20, 20, 20) // zero-length line: a warning, still manufacturable
	add("valid", "zero_length_line", d)

	// spacing, edges, part size and density (limits at 1.5 mm stock: web error < 0.75, warning < 1.5; spacing warning < 3; edge error < 1.5, warning < 3; part width error < 2.54, warning < 6; density >= 50%)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(40, 25, 3)
	d.circle(46.5, 25, 3) // 0.5 mm web
	add("invalid", "thin_web", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(40, 25, 3)
	d.circle(47, 25, 3) // 1 mm web: makeable, below the warning tier
	add("valid", "web_warning", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(5.5, 25, 5) // 0.5 mm from the left edge
	add("invalid", "hole_near_edge", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.circle(5.75, 25, 5) // exactly 0.75 mm from the edge: passes the error tier, warns
	add("valid", "hole_edge_at_limit", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 2, 40, true) // narrower than 2.54 mm
	add("invalid", "narrow_part", d)

	d = newDoc(unitsMM)
	d.lwpolyline([]vertex{{0, 0, 0}, {28.284271, 28.284271, 0}, {25.455844, 31.112698, 0}, {-2.828427, 2.828427, 0}}, true) // 4 mm wide strip at 45°
	add("valid", "slender_part_rotated", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 200, 120, true)
	for row := range 6 {
		for col := range 10 {
			x, y := 7.25+float64(col)*19, 5.25+float64(row)*19
			d.rect(x, y, 14.5, 14.5, true) // 60 squares remove 52.5625% of the plate
		}
	}
	add("valid", "vent_panel", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	d.circle(20, 30, 0.6) // Ø1.2: below the 1.5 mm hole minimum
	d.circle(6, 45, 5)    // 1 mm from the left edge
	d.circle(60, 30, 4)
	d.circle(68.5, 30, 4) // 0.5 mm web
	add("invalid", "bracket", d)

	// local width and corners (limits at 1.5 mm stock: cut-out error < 1, warning < 1.5; feature error < 1.5, warning < 3)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	d.lwpolyline(hourglass(0.6), true)
	add("invalid", "narrow_cutout", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	d.lwpolyline(hourglass(1.2), true)
	add("valid", "cutout_warning", d)

	d = newDoc(unitsMM)
	d.lwpolyline(finger(0.8), true)
	add("invalid", "thin_finger", d)

	d = newDoc(unitsMM)
	d.lwpolyline(finger(2), true)
	add("valid", "finger_warning", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	d.lwpolyline([]vertex{{20, 10, 0}, {60, 10, 0}, {60, 40, 0}, {40.5, 40, 0}, {40.5, 20, 0}, {39.5, 20, 0}, {39.5, 40, 0}, {20, 40, 0}}, true) // U cut-out around a 1 mm tongue
	add("invalid", "thin_tongue", d)

	d = newDoc(unitsMM)
	d.lwpolyline([]vertex{{0, 0, 0}, {100, 0, 0}, {100, 50, 0}, {50.4, 50, 0}, {50.4, 40, 0}, {49.6, 40, 0}, {49.6, 50, 0}, {0, 50, 0}}, true) // 0.8 mm notch
	add("invalid", "narrow_notch", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	d.lwpolyline([]vertex{{10, 10, 0}, {30, 10, 0}, {40, 20, 0}, {10, 20, 0}}, true) // sharp-cornered cut-out
	d.lwpolyline(roundedRect(60, 10, 20, 10, 1), true)                               // 1 mm fillets: above the limit
	if err := addWithCapability("valid", "sharp_cutout_corners", d, nil, map[string]any{
		"internal_corner_radius": map[string]any{"warning": map[string]any{"min": map[string]any{"value": "0.5", "unit": "mm"}}},
	}); err != nil {
		return err
	}

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	d.lwpolyline([]vertex{{10, 23, 0}, {50, 30, 0}, {10, 37, 0}}, true)   // 19.85° tip: warning
	d.lwpolyline([]vertex{{70, 15, 0}, {84, 15, 0}, {77, 34.2, 0}}, true) // 40° tip: above the limit
	add("valid", "sharp_tip_cutout", d)

	d = newDoc(unitsMM)
	d.lwpolyline([]vertex{{0, 0, 0}, {80, 0, 0}, {80, 20, 0}, {0, 20, 0}, {-50, 10, 0}}, true) // 22.62° point
	add("valid", "spike_outline", d)

	// curves: replaced by tangent-continuous arcs

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 60, true)
	d.entity("ELLIPSE", "AcDbEllipse", 10, 50.0, 20, 30.0, 30, 0.0, 11, 20.0, 21, 0.0, 31, 0.0,
		210, 0.0, 220, 0.0, 230, 1.0, 40, 0.5, 41, 0.0, 42, 2*math.Pi)
	add("valid", "ellipse_cutout", d)

	d = newDoc(unitsMM)
	h := math.Sqrt2 / 2 // a rational quadratic spline that is exactly a Ø20 circle
	d.entity("SPLINE", "AcDbSpline", 210, 0.0, 220, 0.0, 230, 1.0,
		70, 8+4+1, 71, 3, 72, 11, 73, 7, 74, 0, 42, 1e-10, 43, 1e-10,
		40, 0.0, 40, 0.0, 40, 0.0, 40, 0.0, 40, 0.25, 40, 0.5, 40, 0.75, 40, 1.0, 40, 1.0, 40, 1.0, 40, 1.0,
		10, 50.0, 20, 0.0, 30, 0.0, 10, 110.0, 20, 0.0, 30, 0.0, 10, 110.0, 20, 70.0, 30, 0.0,
		10, 50.0, 20, 80.0, 30, 0.0, 10, -10.0, 20, 70.0, 30, 0.0, 10, -10.0, 20, 0.0, 30, 0.0,
		10, 50.0, 20, 0.0, 30, 0.0)
	d.entity("SPLINE", "AcDbSpline", 210, 0.0, 220, 0.0, 230, 1.0,
		70, 8+4+1, 71, 2, 72, 12, 73, 9, 74, 0, 42, 1e-10, 43, 1e-10,
		40, 0.0, 40, 0.0, 40, 0.0, 40, 0.25, 40, 0.25, 40, 0.5, 40, 0.5, 40, 0.75, 40, 0.75, 40, 1.0, 40, 1.0, 40, 1.0,
		41, 1.0, 41, h, 41, 1.0, 41, h, 41, 1.0, 41, h, 41, 1.0, 41, h, 41, 1.0,
		10, 60.0, 20, 35.0, 30, 0.0, 10, 60.0, 20, 45.0, 30, 0.0, 10, 50.0, 20, 45.0, 30, 0.0,
		10, 40.0, 20, 45.0, 30, 0.0, 10, 40.0, 20, 35.0, 30, 0.0, 10, 40.0, 20, 25.0, 30, 0.0,
		10, 50.0, 20, 25.0, 30, 0.0, 10, 60.0, 20, 25.0, 30, 0.0, 10, 60.0, 20, 35.0, 30, 0.0)
	d.extend(-10, 0)
	d.extend(110, 80)
	add("valid", "spline_outline", d) // smooth cubic outline around a spline circle

	// stock-dependent: same geometry, different job stock

	// A 2500 x 600 mm plate fits the 1500 x 3000 mm usable area, but not the 1000 x 2000 mm area examples/laser.json gives mild steel over 12.7 mm thick.
	d = newDoc(unitsMM)
	d.rect(0, 0, 2500, 600, true)
	addWithStock("invalid", "too_large_for_thick_plate", d, `{"material_code": "mild_steel", "thickness": {"value": "16", "unit": "mm"}}`)

	// examples/laser.json stocks stainless 304 at listed thicknesses from 0.5 mm: 1.4 mm is not stocked, 0.4 mm is too thin.
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	addWithStock("invalid", "thickness_not_stocked", d, `{"material_code": "stainless_304", "thickness": {"value": "1.4", "unit": "mm"}}`)
	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	addWithStock("invalid", "material_too_thin", d, `{"material_code": "stainless_304", "thickness": {"value": "0.4", "unit": "mm"}}`)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	addWithStock("invalid", "material_too_thick", d, `{"material_code": "mild_steel", "thickness": {"value": "20", "unit": "mm"}}`)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	addWithStock("invalid", "material_unsupported", d, `{"material_code": "titanium", "thickness": {"value": "1.5", "unit": "mm"}}`)

	// malformed: complete drawings the engine must refuse

	d = newDoc(unitsNone)
	d.rect(0, 0, 100, 50, true)
	add("malformed", "unitless", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.entity("SPLINE", "AcDbSpline", 210, 0.0, 220, 0.0, 230, 1.0,
		70, 8, 71, 3, 72, 8, 73, 4, 74, 0, 42, 1e-10, 43, 1e-10,
		40, 0.0, 40, 0.0, 40, 0.0, 40, 0.0, 40, 1.0, 40, 1.0, 40, 1.0, 40, 1.0,
		10, 10.0, 20, 10.0, 30, 0.0, 10, 20.0, 20, 30.0, 30, 0.0,
		10, 40.0, 20, 20.0, 30, 0.0, 10, 60.0, 20, 40.0, 30, 0.0)
	d.extend(60, 40)
	add("invalid", "open_spline", d) // a curve that does not close

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.entity("SPLINE", "AcDbSpline", 210, 0.0, 220, 0.0, 230, 1.0,
		70, 8, 71, 3, 72, 0, 73, 0, 74, 3, 44, 1e-10,
		11, 10.0, 21, 10.0, 31, 0.0, 11, 30.0, 21, 30.0, 31, 0.0, 11, 60.0, 21, 20.0, 31, 0.0)
	add("malformed", "spline_fit_points_only", d)

	d = newDoc(unitsMM)
	d.rect(0, 0, 100, 50, true)
	d.entity("INSERT", "AcDbBlockReference", 2, "MISSING", 10, 50.0, 20, 25.0, 30, 0.0)
	add("malformed", "block_undefined", d)

	d = newDoc(unitsMM)
	d.text(0, 0, 2.5, "nothing to cut")
	add("malformed", "no_geometry", d)

	// Deliberately incomplete files.
	files["valid/polyline_r12_no_handles.dxf"] = []byte(r12Polyline)
	files["malformed/not_a_dxf.dxf"] = []byte("This is not a DXF file.\nIt is plain text.\n")
	full := string(files["valid/valid_rectangle.dxf"])
	files["malformed/truncated.dxf"] = []byte(full[:strings.Index(full, "AcDbPolyline")])

	// Benchmark drawings (fixtures/perf): synthetic stand-ins for the kinds of real drawing that cost the most, so the benchmarks need nothing outside the repository.
	perfFiles(files)
	for name, data := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// r12Polyline is an R12-style file written without handles, using the older POLYLINE/VERTEX/SEQEND representation. R12 files need only an ENTITIES section, so this is still a complete drawing for that version.
const r12Polyline = `0
SECTION
2
HEADER
9
$ACADVER
1
AC1009
9
$INSUNITS
70
4
0
ENDSEC
0
SECTION
2
ENTITIES
0
POLYLINE
8
0
66
1
70
1
0
VERTEX
8
0
10
0.0
20
0.0
0
VERTEX
8
0
10
50.0
20
0.0
0
VERTEX
8
0
10
50.0
20
50.0
0
VERTEX
8
0
10
0.0
20
50.0
0
SEQEND
8
0
0
CIRCLE
8
0
10
25.0
20
25.0
40
1.5
0
ENDSEC
0
EOF
`

// perfFiles adds the benchmark drawings. Products are converted before they are added so no CPU fuses them (see AGENTS.md): the files must be the same bytes on every machine.
func perfFiles(files map[string][]byte) {
	// A sheet of 100 nested parts whose curves are dense polylines, as CAM exports write them: about 100,000 segments.
	var sheet strings.Builder
	sheet.WriteString(`<g fill="none" stroke="black" stroke-width="0.1">
`)
	poly := func(cx, cy, rx, ry float64, n int, lobes int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			r := float64(0.08*math.Cos(float64(lobes)*a)) + 1
			x := float64(rx*r*math.Cos(a)) + cx
			y := float64(ry*r*math.Sin(a)) + cy
			fmt.Fprintf(&b, "%.4f,%.4f ", x, y)
		}
		return b.String()
	}
	for row := 0; row < 10; row++ {
		for col := 0; col < 10; col++ {
			cx, cy := float64(col*98)+50, float64(row*78)+45
			fmt.Fprintf(&sheet, `  <polygon points="%s"/>
`, poly(cx, cy, 44, 34, 600, 3+(row+col)%4))
			fmt.Fprintf(&sheet, `  <polygon points="%s"/>
`, poly(cx-14, cy, 9, 9, 200, 0))
			fmt.Fprintf(&sheet, `  <polygon points="%s"/>
`, poly(cx+14, cy+4, 7, 11, 200, 2))
		}
	}
	sheet.WriteString("</g>\n")
	files["perf/nested_sheet.svg"] = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="1000mm" height="800mm" viewBox="0 0 1000 800">
` + sheet.String() + `</svg>
`)
	// A frame of 300 rounded windows drawn with cubic curves, which are fitted to arcs when read.
	var frame strings.Builder
	frame.WriteString(`<g fill="none" stroke="black" stroke-width="0.1">
  <rect x="0" y="0" width="1000" height="750" rx="12"/>
`)
	for row := 0; row < 15; row++ {
		for col := 0; col < 20; col++ {
			x, y := float64(col*47)+15, float64(row*47)+15
			w, h, r := 38.0, 38.0, 9.0
			k := r * 0.55
			fmt.Fprintf(&frame, `  <path d="M%.2f %.2f h%.2f c%.2f 0 %.2f %.2f %.2f %.2f v%.2f c0 %.2f %.2f %.2f %.2f %.2f h%.2f c%.2f 0 %.2f %.2f %.2f %.2f v%.2f c0 %.2f %.2f %.2f %.2f %.2f z"/>
`, x+r, y, w-2*r, k, r, r-k, r, r, h-2*r, k, -(r - k), r, -r, r, -(w - 2*r), -k, -r, -(r - k), -r, -r, -(h - 2*r), -k, r-k, -r, r, -r)
		}
	}
	frame.WriteString("</g>\n")
	files["perf/frame.svg"] = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="1000mm" height="750mm" viewBox="0 0 1000 750">
` + frame.String() + `</svg>
`)
	// A plate with many narrow places: a comb of 200 teeth 1.2 mm apart, drawn as one polyline, and 100 small windows. The DXF writer holds a few hundred entities, so the comb is one entity of 800 vertices.
	d := newDoc(unitsMM)
	comb := []vertex{{x: 0, y: 0}, {x: 1000, y: 0}, {x: 1000, y: 150}}
	for i := 199; i >= 0; i-- {
		x := float64(float64(i)*4.8) + 20
		comb = append(comb, vertex{x: x + 3.6, y: 150}, vertex{x: x + 3.6, y: 40}, vertex{x: x + 2.4, y: 40}, vertex{x: x + 2.4, y: 150})
	}
	comb = append(comb, vertex{x: 0, y: 150})
	d.lwpolyline(comb, true)
	for i := 0; i < 50; i++ {
		for j := 0; j < 2; j++ {
			d.rect(float64(i*19)+12, float64(j*12)+10, 14, 8, true)
		}
	}
	files["perf/narrow_places.dxf"] = d.bytes()
	// A bracket: a plate with rounded corners, 280 holes and 30 slots.
	d = newDoc(unitsMM)
	d.roundedRect(0, 0, 900, 500, 8, false)
	for i := 0; i < 14; i++ {
		for j := 0; j < 20; j++ {
			d.circle(float64(i*60)+30, float64(j*22)+20, 3.3)
		}
	}
	for i := 0; i < 30; i++ {
		d.obroundEntities(float64(i*28)+25, 485, 4, 14)
	}
	files["perf/bracket.dxf"] = d.bytes()
	// A bent panel with 150 holes.
	var holes [][3]float64
	for i := 0; i < 15; i++ {
		for j := 0; j < 10; j++ {
			holes = append(holes, [3]float64{float64(i*22) + 20, float64(j*12) + 15, 2.5})
		}
	}
	files["perf/bent.step"] = sheetModel{W: 360, A: 150, T: 1.6, Angle: 90, R: 1.6, B: 60, Holes: holes}.bytes()
}
