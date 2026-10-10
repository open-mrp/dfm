package design

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/open-mrp/dfm/constants"
)

// Repair returns d's file with its adjustments written into it, so the file says what the adjustments told the engine: removed entities are deleted, bend lines are moved to a layer named BEND, and set_units (or d.Units, when no adjustment sets the units) declares the units. Everything else is left byte for byte as it was.
//
// The adjustments are first checked against the drawing as Parse reads it. A binary DXF is corrected and written back as ASCII DXF. An entity inside a block placed once (by one INSERT, not an array) is edited in the block's definition. Entities of blocks placed more than once, nested blocks and SVG elements placed by a <use> are not edited (the change would apply wherever the block or element is placed): an adjustment naming one is ErrUnsupportedContent.
func Repair(d Design) ([]byte, error) {
	if _, err := Parse(d); err != nil {
		return nil, err
	}
	units := d.Units
	var entities []Adjustment
	for _, a := range d.Adjustments {
		if a.Action == constants.FixActionSetUnits {
			units = a.Units
			continue
		}
		for _, id := range a.EntityIDs {
			if strings.Contains(id, "/") && d.Format != constants.DesignFormatDXF {
				return nil, fmt.Errorf("%w: a corrected file cannot be written for element %q, which is placed by a <use>", ErrUnsupportedContent, id)
			}
		}
		entities = append(entities, a)
	}
	switch d.Format {
	case constants.DesignFormatDXF:
		return repairDXF(d.Data, entities, units)
	case constants.DesignFormatSVG:
		return repairSVG(d.Data, entities, units)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, d.Format)
	}
}

// bendLayer is the layer a corrected DXF puts bend lines on.
const bendLayer = "BEND"

// dxfUnits are the $INSUNITS values a corrected DXF declares.
var dxfUnits = map[constants.LengthUnit]int{
	constants.LengthUnitInch: 1, constants.LengthUnitFoot: 2, constants.LengthUnitMillimeter: 4,
	constants.LengthUnitCentimeter: 5, constants.LengthUnitMeter: 6, constants.LengthUnitMicrometer: 13,
}

// lineEdits collects edits to a file's lines, by 0-based line index, and applies them all at once so earlier edits do not move later ones.
type lineEdits struct {
	replace map[int]string
	drop    map[int]bool
	insert  map[int][]string // lines to insert before the index
}

func (e *lineEdits) apply(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i <= len(lines); i++ {
		out = append(out, e.insert[i]...)
		if i == len(lines) {
			break
		}
		if e.drop[i] {
			continue
		}
		if v, ok := e.replace[i]; ok {
			out = append(out, v)
			continue
		}
		out = append(out, lines[i])
	}
	return out
}

func repairDXF(data []byte, adj []Adjustment, units constants.LengthUnit) ([]byte, error) {
	if bytes.HasPrefix(data, []byte(binarySentinel)) {
		// A binary DXF is corrected as the ASCII DXF it reads as, with the same handles, and written back as ASCII.
		tags, err := readBinaryTags(data)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, t := range tags {
			fmt.Fprintf(&b, "%3d\n%s\n", t.code, t.value)
		}
		data = []byte(b.String())
	}
	f, err := readDXF(data)
	if err != nil {
		return nil, err
	}
	tags := f.tags
	bom := bytes.HasPrefix(data, []byte("\xef\xbb\xbf"))
	text := string(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	trailing := strings.HasSuffix(text, "\n")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if trailing {
		lines = lines[:len(lines)-1]
	}
	e := &lineEdits{replace: map[int]string{}, drop: map[int]bool{}, insert: map[int][]string{}}

	// An entity spans from its group 0 line to its last value line; a POLYLINE takes its VERTEX and SEQEND entities with it.
	index := map[string]int{}
	for i, ent := range f.entities {
		index[ent.id()] = i
	}
	spanOf := func(list []rawEntity, i int) []rawEntity {
		out := []rawEntity{list[i]}
		if list[i].kind == "POLYLINE" {
			for j := i + 1; j < len(list); j++ {
				out = append(out, list[j])
				if list[j].kind == "SEQEND" {
					break
				}
			}
		}
		return out
	}
	// placements counts the INSERTs of each block, in the drawing and in other blocks.
	placements := map[string]int{}
	count := func(list []rawEntity) {
		for _, ent := range list {
			if ent.kind == "INSERT" {
				placements[strings.ToUpper(ent.str(2))]++
			}
		}
	}
	count(f.entities)
	for _, b := range f.blocks {
		count(b.entities)
	}
	// find returns the entity an ID names: in the drawing, or in the definition of a block placed once.
	find := func(id string) ([]rawEntity, error) {
		insertID, childID, nested := strings.Cut(id, "/")
		if !nested {
			i, ok := index[id]
			if !ok {
				return nil, fmt.Errorf("%w: entity %q is not in the ENTITIES section", ErrUnsupportedContent, id)
			}
			return spanOf(f.entities, i), nil
		}
		i, ok := index[insertID]
		if !ok || strings.ContainsAny(insertID, "[") || strings.Contains(childID, "/") || f.entities[i].kind != "INSERT" {
			return nil, fmt.Errorf("%w: a corrected file cannot be written for entity %q, inside a nested or arrayed block reference", ErrUnsupportedContent, id)
		}
		name := strings.ToUpper(f.entities[i].str(2))
		if n := placements[name]; n != 1 {
			return nil, fmt.Errorf("%w: a corrected file cannot be written for entity %q: its block %q is placed %d times, and changing it would change them all", ErrUnsupportedContent, id, f.entities[i].str(2), n)
		}
		list := f.blocks[name].entities
		for j, ent := range list {
			if ent.id() == childID {
				return spanOf(list, j), nil
			}
		}
		return nil, fmt.Errorf("%w: entity %q is not in its block", ErrUnsupportedContent, id)
	}
	bent := false
	for _, a := range adj {
		for _, id := range a.EntityIDs {
			ents, err := find(id)
			if err != nil {
				return nil, err
			}
			for _, ent := range ents {
				switch a.Action {
				case constants.FixActionRemove:
					last := ent.line
					for _, t := range ent.tags {
						last = max(last, t.line)
					}
					for l := ent.line - 1; l <= last; l++ {
						e.drop[l] = true
					}
				case constants.FixActionMarkBendLine:
					bent = true
					layered := false
					for _, t := range ent.tags {
						if t.code == 8 {
							e.replace[t.line] = bendLayer
							layered = true
						}
					}
					if !layered {
						at := ent.line + 1 // after the entity's type
						for _, t := range ent.tags {
							if t.code == 100 && t.value == "AcDbEntity" {
								at = t.line + 1
								break
							}
						}
						e.insert[at] = append(e.insert[at], "  8", bendLayer)
					}
				}
			}
		}
	}
	if _, ok := f.layers[bendLayer]; bent && !ok {
		addLayer(tags, e, bendLayer)
	}
	if units != "" {
		setInsUnits(tags, e, dxfUnits[units])
	}
	out := strings.Join(e.apply(lines), eol)
	if trailing {
		out += eol
	}
	if bom {
		out = "\xef\xbb\xbf" + out
	}
	return []byte(out), nil
}

// addLayer adds a continuous, white layer record to the LAYER table, with a new handle when the table's records have handles. A file without a LAYER table is left as it is: readers take an unlisted layer as a plain one.
func addLayer(tags []tag, e *lineEdits, name string) {
	start := -1
	for i := 0; i+1 < len(tags); i++ {
		if tags[i].code == 0 && tags[i].value == "TABLE" && tags[i+1].code == 2 && tags[i+1].value == "LAYER" {
			start = i
			break
		}
	}
	if start < 0 {
		return
	}
	owner, handles, endtab := "", false, -1
	for i := start + 2; i < len(tags); i++ {
		t := tags[i]
		if t.code == 0 && t.value == "ENDTAB" {
			endtab = i
			break
		}
		if t.code == 5 && owner == "" {
			owner = t.value
		}
		if t.code == 0 && t.value == "LAYER" {
			for _, u := range tags[i+1:] {
				if u.code == 0 {
					break
				}
				if u.code == 5 {
					handles = true
				}
			}
		}
	}
	if endtab < 0 {
		return
	}
	record := []string{"  0", "LAYER"}
	if handles {
		h := newHandle(tags, e)
		record = append(record, "  5", h, "330", owner, "100", "AcDbSymbolTableRecord", "100", "AcDbLayerTableRecord")
	}
	record = append(record, "  2", name, " 70", "     0", " 62", "     7", "  6", "CONTINUOUS")
	at := tags[endtab].line - 1
	e.insert[at] = append(e.insert[at], record...)
}

// newHandle returns a handle above every handle in the file, $HANDSEED when the file keeps it (the next free handle), and moves $HANDSEED past it.
func newHandle(tags []tag, e *lineEdits) string {
	var top uint64
	seed := -1
	for i := 0; i < len(tags); i++ {
		t := tags[i]
		if t.code == 9 && t.value == "$HANDSEED" && i+1 < len(tags) {
			seed = i + 1
			if v, err := strconv.ParseUint(tags[i+1].value, 16, 64); err == nil && v > 0 {
				top = max(top, v-1)
			}
			i++ // the seed is not a handle in use
			continue
		}
		if t.code == 5 || t.code == 105 {
			if v, err := strconv.ParseUint(t.value, 16, 64); err == nil {
				top = max(top, v)
			}
		}
	}
	h := strconv.FormatUint(top+1, 16)
	if seed >= 0 {
		e.replace[tags[seed].line] = strings.ToUpper(strconv.FormatUint(top+2, 16))
	}
	return strings.ToUpper(h)
}

// setInsUnits sets $INSUNITS, adding it to the HEADER section, or adding a HEADER section, when the file has none.
func setInsUnits(tags []tag, e *lineEdits, code int) {
	value := fmt.Sprintf("%6d", code)
	for i, t := range tags {
		if t.code == 9 && t.value == "$INSUNITS" && i+1 < len(tags) {
			e.replace[tags[i+1].line] = value
			return
		}
	}
	for i := 0; i+1 < len(tags); i++ {
		if tags[i].code == 0 && tags[i].value == "SECTION" && tags[i+1].code == 2 && tags[i+1].value == "HEADER" {
			at := tags[i+1].line + 1
			e.insert[at] = append(e.insert[at], "  9", "$INSUNITS", " 70", value)
			return
		}
	}
	e.insert[0] = append(e.insert[0], "  0", "SECTION", "  2", "HEADER", "  9", "$INSUNITS", " 70", value, "  0", "ENDSEC")
}

// byteEdit replaces data[from:to] with text.
type byteEdit struct {
	from, to int64
	text     string
}

func repairSVG(data []byte, adj []Adjustment, units constants.LengthUnit) ([]byte, error) {
	root, err := readSVG(data)
	if err != nil {
		return nil, err
	}
	nodes := map[string]*svgNode{}
	ids := map[string]bool{}
	var walk func(n *svgNode)
	walk = func(n *svgNode) {
		if n.attrs["id"] != "" {
			ids[n.attrs["id"]] = true
		}
		if drawable[n.name] {
			if _, dup := nodes[svgID(n)]; !dup {
				nodes[svgID(n)] = n
			}
		}
		for _, ch := range n.children {
			walk(ch)
		}
	}
	walk(root)
	var edits []byteEdit
	group := 0
	for _, a := range adj {
		for _, id := range a.EntityIDs {
			n, ok := nodes[id]
			if !ok {
				return nil, fmt.Errorf("%w: element %q is not drawn directly in the SVG", ErrUnsupportedContent, id)
			}
			switch a.Action {
			case constants.FixActionRemove:
				edits = append(edits, byteEdit{n.start, n.end, ""})
			case constants.FixActionMarkBendLine:
				// A group around the element, in its place, keeps its transforms; the group's id names it a bend layer.
				gid := ""
				for gid == "" || ids[gid] {
					group++
					gid = fmt.Sprintf("%s-%d", bendLayer, group)
				}
				ids[gid] = true
				edits = append(edits, byteEdit{n.start, n.start, `<g id="` + gid + `">`}, byteEdit{n.end, n.end, "</g>"})
			}
		}
	}
	if units != "" {
		e, err := svgUnitsEdit(data, root, units)
		if err != nil {
			return nil, err
		}
		edits = append(edits, e)
	}
	// Apply from the end of the file back, so offsets stay valid; at one offset, a closing tag goes before an opening one.
	slices.SortStableFunc(edits, func(x, y byteEdit) int {
		if x.from != y.from {
			return int(y.from - x.from)
		}
		return strings.Compare(y.text, x.text)
	})
	out := slices.Clone(data)
	for _, e := range edits {
		out = slices.Concat(out[:e.from], []byte(e.text), out[e.to:])
	}
	return out, nil
}

// svgSize gives a length in SVG units: the unit written and the multiple of a drawing unit it stands for.
var svgSize = map[constants.LengthUnit]struct {
	unit   string
	factor float64
}{
	constants.LengthUnitMillimeter: {"mm", 1}, constants.LengthUnitCentimeter: {"cm", 1},
	constants.LengthUnitInch: {"in", 1}, constants.LengthUnitMicrometer: {"mm", 0.001},
	constants.LengthUnitMeter: {"mm", 1000}, constants.LengthUnitFoot: {"in", 12},
}

var (
	sizeAttr    = regexp.MustCompile(`\s(width|height)\s*=\s*("[^"]*"|'[^']*')`)
	viewBoxAttr = regexp.MustCompile(`\sviewBox\s*=`)
)

// svgUnitsEdit rewrites the root's start tag so one viewBox unit is one of units, as reading the drawing in those units does: width and height are the viewBox's, in those units. Without a viewBox, one is added from a width and height in plain user units.
func svgUnitsEdit(data []byte, root *svgNode, units constants.LengthUnit) (byteEdit, error) {
	s := svgSize[units]
	var w, h float64
	if vb := strings.FieldsFunc(root.attrs["viewBox"], func(r rune) bool { return r == ' ' || r == ',' }); len(vb) == 4 {
		var err1, err2 error
		w, err1 = strconv.ParseFloat(vb[2], 64)
		h, err2 = strconv.ParseFloat(vb[3], 64)
		if err1 != nil || err2 != nil {
			return byteEdit{}, fmt.Errorf("%w: the SVG's viewBox %q is not four numbers", ErrInvalid, truncate(root.attrs["viewBox"]))
		}
	} else {
		var err1, err2 error
		w, err1 = strconv.ParseFloat(strings.TrimSuffix(root.attrs["width"], "px"), 64)
		h, err2 = strconv.ParseFloat(strings.TrimSuffix(root.attrs["height"], "px"), 64)
		if err1 != nil || err2 != nil {
			return byteEdit{}, fmt.Errorf("%w: a corrected file cannot set the units of an SVG without a viewBox whose width and height are not plain numbers", ErrUnsupportedContent)
		}
	}
	num := func(v float64) string { return strconv.FormatFloat(v*s.factor, 'f', -1, 64) + s.unit }
	tag := string(data[root.start:root.tagEnd])
	tag = sizeAttr.ReplaceAllString(tag, "")
	extra := fmt.Sprintf(` width="%s" height="%s"`, num(w), num(h))
	if !viewBoxAttr.MatchString(tag) {
		extra += fmt.Sprintf(` viewBox="0 0 %s %s"`, strconv.FormatFloat(w, 'f', -1, 64), strconv.FormatFloat(h, 'f', -1, 64))
	}
	// After the element name: "<svg" is the tag's first four bytes.
	tag = tag[:4] + extra + tag[4:]
	return byteEdit{root.start, root.tagEnd, tag}, nil
}
