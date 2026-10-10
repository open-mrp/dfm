package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// This file writes complete DXF R2000 (AC1015) drawings: HEADER, CLASSES, TABLES, BLOCKS, ENTITIES and OBJECTS, with every table record, block, layout and dictionary that a CAD application expects, and with every owner and cross-reference handle filled in.
//
// Handles are allocated from three fixed ranges so that adding structure never renumbers the entities that findings refer to:
//
//	0x01–0x2E  drawing structure (tables, standard records, dictionaries, layouts)
//	0x2F–      model- and paper-space entities, in the order they are drawn
//	0x200–     user block definitions and their contents

// Fixed handles of the drawing structure.
const (
	hBlockRecordTable   = 0x01
	hLayerTable         = 0x02
	hStyleTable         = 0x03
	hLtypeTable         = 0x05
	hViewTable          = 0x06
	hUCSTable           = 0x07
	hVportTable         = 0x08
	hAppidTable         = 0x09
	hDimstyleTable      = 0x0A
	hRootDict           = 0x0C
	hGroupDict          = 0x0D
	hPlotStyleDict      = 0x0E
	hPlotStyleNormal    = 0x0F
	hLayer0             = 0x10
	hStyleStandard      = 0x11
	hAppidACAD          = 0x12
	hLtypeByBlock       = 0x14
	hLtypeByLayer       = 0x15
	hLtypeContinuous    = 0x16
	hMlineStyleDict     = 0x17
	hMlineStyleStandard = 0x18
	hLayoutDict         = 0x1A
	hPaperSpaceRecord   = 0x1B
	hPaperSpaceBlock    = 0x1C
	hPaperSpaceEnd      = 0x1D
	hPaperLayout        = 0x1E
	hModelSpaceRecord   = 0x1F
	hModelSpaceBlock    = 0x20
	hModelSpaceEnd      = 0x21
	hModelLayout        = 0x22
	hLtypeDashed        = 0x23
	hFirstExtraLayer    = 0x24 // up to three extra layers, 0x24-0x26
	hDimstyleStandard   = 0x27
	hActiveVport        = 0x29

	firstEntityHandle = 0x2F
	firstBlockHandle  = 0x200
)

// Values for the $INSUNITS header variable.
const (
	unitsNone = 0
	unitsInch = 1
	unitsMM   = 4
	unitsCM   = 5
)

func hex(h int) string { return strings.ToUpper(strconv.FormatInt(int64(h), 16)) }

func num(v float64) string {
	if v == 0 {
		v = 0 // normalize -0
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// pairs writes alternating group codes and values.
func pairs(b *strings.Builder, kv ...any) {
	for i := 0; i < len(kv); i += 2 {
		var v string
		switch x := kv[i+1].(type) {
		case float64:
			v = num(x)
		case int:
			v = strconv.Itoa(x)
		case string:
			v = x
		default:
			panic(fmt.Sprintf("unsupported value %T", x))
		}
		fmt.Fprintf(b, "%d\n%s\n", kv[i].(int), v)
	}
}

// block is a user block definition (named or anonymous).
type block struct {
	name               string
	record, begin, end int
	anonymous          bool
	body               strings.Builder
}

// doc accumulates one DXF drawing.
type doc struct {
	units      int
	nextEntity int
	nextBlock  int
	minX, minY float64
	maxX, maxY float64
	entities   strings.Builder
	blocks     []*block
	// layer is the layer model-space entities are drawn on; "" is 0.
	layer string
	// linetype is the line type model-space entities are drawn with; "" is BYLAYER.
	linetype string
	// dashed adds a DASHED line type to the LTYPE table.
	dashed bool
	// layers are layers besides 0, as {name, line type} pairs.
	layers [][2]string
}

func newDoc(units int) *doc {
	return &doc{
		units: units, nextEntity: firstEntityHandle, nextBlock: firstBlockHandle,
		minX: math.Inf(1), minY: math.Inf(1), maxX: math.Inf(-1), maxY: math.Inf(-1),
	}
}

// extend grows the drawing extents written to the header and viewport.
func (d *doc) extend(x, y float64) {
	d.minX, d.minY = math.Min(d.minX, x), math.Min(d.minY, y)
	d.maxX, d.maxY = math.Max(d.maxX, x), math.Max(d.maxY, y)
}

func (d *doc) entityHandle() int {
	h := d.nextEntity
	d.nextEntity++
	if h >= firstBlockHandle {
		panic("too many entities for the entity handle range")
	}
	return h
}

func (d *doc) blockHandle() int {
	h := d.nextBlock
	d.nextBlock++
	return h
}

// entityTo writes an entity with handle h and owner block record owner.
func entityTo(b *strings.Builder, h, owner int, paperSpace bool, layer, linetype, kind, subclass string, data ...any) {
	pairs(b, 0, kind, 5, hex(h), 330, hex(owner), 100, "AcDbEntity")
	if paperSpace {
		pairs(b, 67, 1)
	}
	if layer == "" {
		layer = "0"
	}
	pairs(b, 8, layer)
	if linetype != "" {
		pairs(b, 6, linetype)
	}
	if subclass != "" {
		pairs(b, 100, subclass)
	}
	pairs(b, data...)
}

// entity writes a model-space entity.
func (d *doc) entity(kind, subclass string, data ...any) {
	entityTo(&d.entities, d.entityHandle(), hModelSpaceRecord, false, d.layer, d.linetype, kind, subclass, data...)
}

// paperEntity writes an entity on the paper-space layout.
func (d *doc) paperEntity(kind, subclass string, data ...any) {
	entityTo(&d.entities, d.entityHandle(), hPaperSpaceRecord, true, "", "", kind, subclass, data...)
}

// newBlock starts a block definition. Anonymous blocks (dimension graphics) get names like *D1.
func (d *doc) newBlock(name string, anonymous bool) *block {
	blk := &block{name: name, anonymous: anonymous, record: d.blockHandle(), begin: d.blockHandle(), end: d.blockHandle()}
	d.blocks = append(d.blocks, blk)
	return blk
}

// blockEntity writes an entity inside blk.
func (d *doc) blockEntity(blk *block, kind, subclass string, data ...any) {
	entityTo(&blk.body, d.blockHandle(), blk.record, false, "", "", kind, subclass, data...)
}

// --- primitives -----------------------------------------------------------

func (d *doc) line(x0, y0, x1, y1 float64) {
	d.extend(x0, y0)
	d.extend(x1, y1)
	d.entity("LINE", "AcDbLine", 10, x0, 20, y0, 30, 0.0, 11, x1, 21, y1, 31, 0.0)
}

func (d *doc) circle(cx, cy, r float64) {
	d.extend(cx-r, cy-r)
	d.extend(cx+r, cy+r)
	d.entity("CIRCLE", "AcDbCircle", 10, cx, 20, cy, 30, 0.0, 40, r)
}

// arc writes a counter-clockwise ARC from start to end degrees. With mirrored, the arc uses extrusion (0,0,-1) and its center and angles are in that mirrored object coordinate system.
func (d *doc) arc(cx, cy, r, start, end float64, mirrored bool) {
	wx := cx
	data := []any{10, cx, 20, cy, 30, 0.0, 40, r}
	if mirrored {
		wx = -cx
		data = append(data, 210, 0.0, 220, 0.0, 230, -1.0)
	}
	d.extend(wx-r, cy-r)
	d.extend(wx+r, cy+r)
	data = append(data, 100, "AcDbArc", 50, start, 51, end)
	d.entity("ARC", "AcDbCircle", data...)
}

// vertex is a polyline vertex; bulge applies to the segment that starts at it.
type vertex struct{ x, y, bulge float64 }

func (d *doc) lwpolyline(vs []vertex, closed bool) {
	flags := 0
	if closed {
		flags = 1
	}
	data := []any{90, len(vs), 70, flags, 43, 0.0}
	for _, v := range vs {
		d.extend(v.x, v.y)
		data = append(data, 10, v.x, 20, v.y)
		if v.bulge != 0 {
			data = append(data, 42, v.bulge)
		}
	}
	d.entity("LWPOLYLINE", "AcDbPolyline", data...)
}

func textData(x, y, height float64, s string) []any {
	return []any{10, x, 20, y, 30, 0.0, 40, height, 1, s, 7, "Standard", 100, "AcDbText"}
}

func (d *doc) text(x, y, height float64, s string) {
	d.entity("TEXT", "AcDbText", textData(x, y, height, s)...)
}

func (d *doc) mtext(x, y, height float64, s string) {
	d.entity("MTEXT", "AcDbMText", 10, x, 20, y, 30, 0.0, 40, height, 41, 0.0, 71, 1, 72, 5, 1, s, 7, "Standard")
}

func (d *doc) point(x, y float64) {
	d.entity("POINT", "AcDbPoint", 10, x, 20, y, 30, 0.0)
}

// linearDim writes a horizontal DIMENSION between x0 and x1 (on y = y0) with the dimension line at dimY, together with the anonymous block that holds its graphics, as CAD applications expect.
func (d *doc) linearDim(x0, x1, y0, dimY float64) {
	blk := d.newBlock(fmt.Sprintf("*D%d", len(d.blocks)+1), true)
	const ext, arrow, textH = 1.25, 2.5, 2.5
	measure := num(x1 - x0)
	d.blockEntity(blk, "LINE", "AcDbLine", 10, x0, 20, y0-ext, 30, 0.0, 11, x0, 21, dimY-ext, 31, 0.0)
	d.blockEntity(blk, "LINE", "AcDbLine", 10, x1, 20, y0-ext, 30, 0.0, 11, x1, 21, dimY-ext, 31, 0.0)
	d.blockEntity(blk, "LINE", "AcDbLine", 10, x0, 20, dimY, 30, 0.0, 11, x1, 21, dimY, 31, 0.0)
	for _, a := range []struct{ tip, dir float64 }{{x0, 1}, {x1, -1}} {
		base := a.tip + a.dir*arrow
		d.blockEntity(blk, "SOLID", "AcDbTrace",
			10, a.tip, 20, dimY, 30, 0.0,
			11, base, 21, dimY-arrow/6, 31, 0.0,
			12, base, 22, dimY+arrow/6, 32, 0.0,
			13, base, 23, dimY+arrow/6, 33, 0.0)
	}
	mid := (x0 + x1) / 2
	d.blockEntity(blk, "MTEXT", "AcDbMText", 10, mid, 20, dimY+textH/2, 30, 0.0, 40, textH, 41, 0.0, 71, 8, 72, 5, 1, measure, 7, "Standard")
	d.extend(x0, dimY-ext)
	d.entity("DIMENSION", "AcDbDimension",
		2, blk.name, 10, x1, 20, dimY, 30, 0.0, 11, mid, 21, dimY+textH/2, 31, 0.0,
		70, 32, 71, 5, 3, "Standard",
		100, "AcDbAlignedDimension", 13, x0, 23, y0, 33, 0.0, 14, x1, 24, y0, 34, 0.0, 50, 0.0,
		100, "AcDbRotatedDimension")
}

// solidHatch writes a solid-fill HATCH bounded by an axis-aligned rectangle.
func (d *doc) solidHatch(x, y, w, h float64) {
	data := []any{10, 0.0, 20, 0.0, 30, 0.0, 210, 0.0, 220, 0.0, 230, 1.0,
		2, "SOLID", 70, 1, 71, 0, 91, 1, 92, 3, 72, 0, 73, 1, 93, 4}
	for _, p := range [][2]float64{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}} {
		data = append(data, 10, p[0], 20, p[1])
	}
	data = append(data, 97, 0, 75, 1, 76, 1, 98, 1, 10, x+w/2, 20, y+h/2)
	d.entity("HATCH", "AcDbHatch", data...)
}

// --- document structure -----------------------------------------------------

// bytes renders the complete drawing.
func (d *doc) bytes() []byte {
	if math.IsInf(d.minX, 1) {
		d.minX, d.minY, d.maxX, d.maxY = 0, 0, 0, 0
	}
	var b strings.Builder
	d.writeHeader(&b)
	writeClasses(&b)
	d.writeTables(&b)
	d.writeBlocks(&b)
	pairs(&b, 0, "SECTION", 2, "ENTITIES")
	b.WriteString(d.entities.String())
	pairs(&b, 0, "ENDSEC")
	writeObjects(&b)
	pairs(&b, 0, "EOF")
	return []byte(b.String())
}

func (d *doc) writeHeader(b *strings.Builder) {
	measurement := 1 // metric
	if d.units == unitsInch {
		measurement = 0
	}
	pairs(b, 0, "SECTION", 2, "HEADER",
		9, "$ACADVER", 1, "AC1015",
		9, "$DWGCODEPAGE", 3, "ANSI_1252",
		9, "$INSBASE", 10, 0.0, 20, 0.0, 30, 0.0,
		9, "$EXTMIN", 10, d.minX, 20, d.minY, 30, 0.0,
		9, "$EXTMAX", 10, d.maxX, 20, d.maxY, 30, 0.0,
		9, "$LIMMIN", 10, 0.0, 20, 0.0,
		9, "$LIMMAX", 10, 420.0, 20, 297.0,
		9, "$CLAYER", 8, "0",
		9, "$CELTYPE", 6, "ByLayer",
		9, "$TEXTSTYLE", 7, "Standard",
		9, "$DIMSTYLE", 2, "Standard",
		9, "$CMLSTYLE", 2, "Standard",
		9, "$MEASUREMENT", 70, measurement,
		9, "$INSUNITS", 70, d.units,
		9, "$HANDSEED", 5, hex(d.nextBlock),
		0, "ENDSEC")
}

func writeClasses(b *strings.Builder) {
	pairs(b, 0, "SECTION", 2, "CLASSES")
	for _, c := range [][2]string{
		{"ACDBDICTIONARYWDFLT", "AcDbDictionaryWithDefault"},
		{"ACDBPLACEHOLDER", "AcDbPlaceHolder"},
		{"LAYOUT", "AcDbLayout"},
	} {
		pairs(b, 0, "CLASS", 1, c[0], 2, c[1], 3, "ObjectDBX Classes", 90, 0, 280, 0, 281, 0)
	}
	pairs(b, 0, "ENDSEC")
}

// table writes a symbol table header; records are written by fn.
func table(b *strings.Builder, name string, handle, count int, fn func()) {
	pairs(b, 0, "TABLE", 2, name, 5, hex(handle), 330, "0", 100, "AcDbSymbolTable", 70, count)
	if fn != nil {
		fn()
	}
	pairs(b, 0, "ENDTAB")
}

func record(b *strings.Builder, kind string, handle, owner int, subclass string, data ...any) {
	pairs(b, 0, kind, 5, hex(handle), 330, hex(owner), 100, "AcDbSymbolTableRecord", 100, subclass)
	pairs(b, data...)
}

func (d *doc) writeTables(b *strings.Builder) {
	pairs(b, 0, "SECTION", 2, "TABLES")

	// The active viewport frames the drawing extents.
	cx, cy := (d.minX+d.maxX)/2, (d.minY+d.maxY)/2
	height := math.Max(d.maxY-d.minY, (d.maxX-d.minX)/1.5) * 1.1
	if height == 0 {
		height = 100
	}
	table(b, "VPORT", hVportTable, 1, func() {
		record(b, "VPORT", hActiveVport, hVportTable, "AcDbViewportTableRecord",
			2, "*ACTIVE", 70, 0,
			10, 0.0, 20, 0.0, 11, 1.0, 21, 1.0, 12, cx, 22, cy,
			13, 0.0, 23, 0.0, 14, 10.0, 24, 10.0, 15, 10.0, 25, 10.0,
			16, 0.0, 26, 0.0, 36, 1.0, 17, 0.0, 27, 0.0, 37, 0.0,
			40, height, 41, 1.5, 42, 50.0, 43, 0.0, 44, 0.0, 50, 0.0, 51, 0.0,
			71, 0, 72, 100, 73, 1, 74, 3, 75, 0, 76, 0, 77, 0, 78, 0)
	})
	if len(d.layers) > 3 {
		panic("too many layers for the structure handle range")
	}
	ltypes := 3
	if d.dashed {
		ltypes = 4
	}
	table(b, "LTYPE", hLtypeTable, ltypes, func() {
		for _, lt := range []struct {
			h          int
			name, desc string
		}{{hLtypeByBlock, "ByBlock", ""}, {hLtypeByLayer, "ByLayer", ""}, {hLtypeContinuous, "Continuous", "Solid line"}} {
			record(b, "LTYPE", lt.h, hLtypeTable, "AcDbLinetypeTableRecord",
				2, lt.name, 70, 0, 3, lt.desc, 72, 65, 73, 0, 40, 0.0)
		}
		if d.dashed {
			record(b, "LTYPE", hLtypeDashed, hLtypeTable, "AcDbLinetypeTableRecord",
				2, "DASHED", 70, 0, 3, "Dashed __ __ __", 72, 65, 73, 2, 40, 0.75,
				49, 0.5, 74, 0, 49, -0.25, 74, 0)
		}
	})
	table(b, "LAYER", hLayerTable, 1+len(d.layers), func() {
		record(b, "LAYER", hLayer0, hLayerTable, "AcDbLayerTableRecord",
			2, "0", 70, 0, 62, 7, 6, "Continuous", 370, -3, 390, hex(hPlotStyleNormal))
		for i, l := range d.layers {
			record(b, "LAYER", hFirstExtraLayer+i, hLayerTable, "AcDbLayerTableRecord",
				2, l[0], 70, 0, 62, 1, 6, l[1], 370, -3, 390, hex(hPlotStyleNormal))
		}
	})
	table(b, "STYLE", hStyleTable, 1, func() {
		record(b, "STYLE", hStyleStandard, hStyleTable, "AcDbTextStyleTableRecord",
			2, "Standard", 70, 0, 40, 0.0, 41, 1.0, 50, 0.0, 71, 0, 42, 2.5, 3, "txt", 4, "")
	})
	table(b, "VIEW", hViewTable, 0, nil)
	table(b, "UCS", hUCSTable, 0, nil)
	table(b, "APPID", hAppidTable, 1, func() {
		record(b, "APPID", hAppidACAD, hAppidTable, "AcDbRegAppTableRecord", 2, "ACAD", 70, 0)
	})

	// DIMSTYLE records use group code 105 for their handle.
	pairs(b, 0, "TABLE", 2, "DIMSTYLE", 5, hex(hDimstyleTable), 330, "0",
		100, "AcDbSymbolTable", 70, 1, 100, "AcDbDimStyleTable", 71, 0)
	pairs(b, 0, "DIMSTYLE", 105, hex(hDimstyleStandard), 330, hex(hDimstyleTable),
		100, "AcDbSymbolTableRecord", 100, "AcDbDimStyleTableRecord",
		2, "Standard", 70, 0, 340, hex(hStyleStandard))
	pairs(b, 0, "ENDTAB")

	table(b, "BLOCK_RECORD", hBlockRecordTable, 2+len(d.blocks), func() {
		record(b, "BLOCK_RECORD", hModelSpaceRecord, hBlockRecordTable, "AcDbBlockTableRecord",
			2, "*Model_Space", 340, hex(hModelLayout))
		record(b, "BLOCK_RECORD", hPaperSpaceRecord, hBlockRecordTable, "AcDbBlockTableRecord",
			2, "*Paper_Space", 340, hex(hPaperLayout))
		for _, blk := range d.blocks {
			record(b, "BLOCK_RECORD", blk.record, hBlockRecordTable, "AcDbBlockTableRecord",
				2, blk.name, 340, "0")
		}
	})
	pairs(b, 0, "ENDSEC")
}

func blockBegin(b *strings.Builder, handle, record int, paperSpace bool, name string, flags int) {
	pairs(b, 0, "BLOCK", 5, hex(handle), 330, hex(record), 100, "AcDbEntity")
	if paperSpace {
		pairs(b, 67, 1)
	}
	pairs(b, 8, "0", 100, "AcDbBlockBegin", 2, name, 70, flags,
		10, 0.0, 20, 0.0, 30, 0.0, 3, name, 1, "")
}

func blockEnd(b *strings.Builder, handle, record int, paperSpace bool) {
	pairs(b, 0, "ENDBLK", 5, hex(handle), 330, hex(record), 100, "AcDbEntity")
	if paperSpace {
		pairs(b, 67, 1)
	}
	pairs(b, 8, "0", 100, "AcDbBlockEnd")
}

func (d *doc) writeBlocks(b *strings.Builder) {
	pairs(b, 0, "SECTION", 2, "BLOCKS")
	blockBegin(b, hModelSpaceBlock, hModelSpaceRecord, false, "*Model_Space", 0)
	blockEnd(b, hModelSpaceEnd, hModelSpaceRecord, false)
	blockBegin(b, hPaperSpaceBlock, hPaperSpaceRecord, true, "*Paper_Space", 0)
	blockEnd(b, hPaperSpaceEnd, hPaperSpaceRecord, true)
	for _, blk := range d.blocks {
		flags := 0
		if blk.anonymous {
			flags = 1
		}
		blockBegin(b, blk.begin, blk.record, false, blk.name, flags)
		b.WriteString(blk.body.String())
		blockEnd(b, blk.end, blk.record, false)
	}
	pairs(b, 0, "ENDSEC")
}

// dictionary writes a DICTIONARY owned by owner with sorted entries.
func dictionary(b *strings.Builder, kind string, handle, owner int, entries map[string]int, tail ...any) {
	pairs(b, 0, kind, 5, hex(handle))
	if owner != 0 {
		pairs(b, 102, "{ACAD_REACTORS", 330, hex(owner), 102, "}")
	}
	pairs(b, 330, hex(owner), 100, "AcDbDictionary", 281, 1)
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		pairs(b, 3, n, 350, hex(entries[n]))
	}
	pairs(b, tail...)
}

func layout(b *strings.Builder, handle int, name string, tab, record int, plotFlags int) {
	pairs(b, 0, "LAYOUT", 5, hex(handle),
		102, "{ACAD_REACTORS", 330, hex(hLayoutDict), 102, "}", 330, hex(hLayoutDict),
		100, "AcDbPlotSettings", 1, "", 2, "none_device", 4, "", 6, "",
		40, 0.0, 41, 0.0, 42, 0.0, 43, 0.0, 44, 0.0, 45, 0.0, 46, 0.0, 47, 0.0,
		48, 0.0, 49, 0.0, 140, 0.0, 141, 0.0, 142, 1.0, 143, 1.0,
		70, plotFlags, 72, 1, 73, 0, 74, 5, 7, "", 75, 16, 147, 1.0, 148, 0.0, 149, 0.0,
		100, "AcDbLayout", 1, name, 70, 1, 71, tab,
		10, 0.0, 20, 0.0, 11, 420.0, 21, 297.0, 12, 0.0, 22, 0.0, 32, 0.0,
		14, 0.0, 24, 0.0, 34, 0.0, 15, 0.0, 25, 0.0, 35, 0.0, 146, 0.0,
		13, 0.0, 23, 0.0, 33, 0.0, 16, 1.0, 26, 0.0, 36, 0.0, 17, 0.0, 27, 1.0, 37, 0.0,
		76, 0, 330, hex(record))
}

func writeObjects(b *strings.Builder) {
	pairs(b, 0, "SECTION", 2, "OBJECTS")
	dictionary(b, "DICTIONARY", hRootDict, 0, map[string]int{
		"ACAD_GROUP":         hGroupDict,
		"ACAD_LAYOUT":        hLayoutDict,
		"ACAD_MLINESTYLE":    hMlineStyleDict,
		"ACAD_PLOTSTYLENAME": hPlotStyleDict,
	})
	dictionary(b, "DICTIONARY", hGroupDict, hRootDict, nil)
	dictionary(b, "DICTIONARY", hLayoutDict, hRootDict, map[string]int{
		"Model":   hModelLayout,
		"Layout1": hPaperLayout,
	})
	dictionary(b, "DICTIONARY", hMlineStyleDict, hRootDict, map[string]int{"Standard": hMlineStyleStandard})
	dictionary(b, "ACDBDICTIONARYWDFLT", hPlotStyleDict, hRootDict, map[string]int{"Normal": hPlotStyleNormal},
		100, "AcDbDictionaryWithDefault", 340, hex(hPlotStyleNormal))
	pairs(b, 0, "ACDBPLACEHOLDER", 5, hex(hPlotStyleNormal),
		102, "{ACAD_REACTORS", 330, hex(hPlotStyleDict), 102, "}", 330, hex(hPlotStyleDict))
	pairs(b, 0, "MLINESTYLE", 5, hex(hMlineStyleStandard),
		102, "{ACAD_REACTORS", 330, hex(hMlineStyleDict), 102, "}", 330, hex(hMlineStyleDict),
		100, "AcDbMlineStyle", 2, "Standard", 70, 0, 3, "", 62, 256, 51, 90.0, 52, 90.0,
		71, 2, 49, 0.5, 62, 256, 6, "BYLAYER", 49, -0.5, 62, 256, 6, "BYLAYER")
	layout(b, hModelLayout, "Model", 0, hModelSpaceRecord, 1712)
	layout(b, hPaperLayout, "Layout1", 1, hPaperSpaceRecord, 688)
	pairs(b, 0, "ENDSEC")
}
