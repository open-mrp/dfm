package design

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// This file is a deliberately small, strict DXF reader (ASCII here; binary DXF is decoded to the same tags by readBinaryTags). It extracts only what the engine needs: the $INSUNITS header variable, the LTYPE and LAYER tables, and the raw entities of the ENTITIES section. Any structural problem is an error; nothing is silently skipped.

// tag is one DXF group-code/value pair.
type tag struct {
	code  int
	value string
	line  int // 1-based line number of the group code
}

// rawEntity is an entity from the ENTITIES section with its tags.
type rawEntity struct {
	kind    string
	line    int
	ordinal int // 1-based position in the ENTITIES section
	tags    []tag
}

// dxfFile is the subset of a DXF document the engine reads.
type dxfFile struct {
	insUnits    int
	hasInsUnits bool
	// measurement is $MEASUREMENT: 0 imperial, 1 metric; -1 when absent.
	measurement int
	// linetypes maps upper-case line type names to whether the pattern has gaps (dashed, center, phantom, ...).
	linetypes map[string]bool
	// layers maps upper-case layer names to their table entries.
	layers map[string]layer
	// blocks maps upper-case block names to their definitions.
	blocks   map[string]block
	entities []rawEntity
	// tags is the whole file, for a repair to edit by line.
	tags []tag
	// ops are the capability's color operations (set by parseDXF).
	ops Operations
}

// block is a block definition from the BLOCKS section.
type block struct {
	name         string
	baseX, baseY float64
	external     bool // an external reference (xref), whose geometry is in another file
	entities     []rawEntity
}

// layer is the part of a LAYER table entry the engine reads.
type layer struct {
	linetype string // upper case; empty means CONTINUOUS
	hidden   bool   // off, frozen or not plotted
	color    string // "#rrggbb", aciForeground or "" (see entityColor)
}

func readTags(data []byte) ([]tag, error) {
	if bytes.HasPrefix(data, []byte(binarySentinel)) {
		return readBinaryTags(data)
	}
	if bytes.HasPrefix(data, []byte("AutoCAD Binary DXF")) {
		return nil, fmt.Errorf("%w: binary DXF without a valid header", ErrInvalid)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	// A trailing newline yields one empty final element; drop it.
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines)%2 != 0 {
		return nil, fmt.Errorf("%w: odd number of lines (truncated file?); group code without value at line %d", ErrInvalid, len(lines))
	}
	tags := make([]tag, 0, len(lines)/2)
	for i := 0; i < len(lines); i += 2 {
		code, err := strconv.Atoi(strings.TrimSpace(lines[i]))
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: expected integer group code, got %q", ErrInvalid, i+1, truncate(lines[i]))
		}
		if code == 999 {
			continue // comment, allowed anywhere
		}
		tags = append(tags, tag{code: code, value: strings.TrimSpace(lines[i+1]), line: i + 1})
	}
	return tags, nil
}

func readDXF(data []byte) (*dxfFile, error) {
	tags, err := readTags(data)
	if err != nil {
		return nil, err
	}
	f := &dxfFile{measurement: -1, tags: tags}
	sawEntities := false
	i := 0
	for i < len(tags) {
		t := tags[i]
		if t.code != 0 {
			return nil, fmt.Errorf("%w: line %d: expected SECTION or EOF, got group code %d", ErrInvalid, t.line, t.code)
		}
		if t.value == "EOF" {
			break
		}
		if t.value != "SECTION" {
			return nil, fmt.Errorf("%w: line %d: expected SECTION or EOF, got %q", ErrInvalid, t.line, truncate(t.value))
		}
		if i+1 >= len(tags) || tags[i+1].code != 2 {
			return nil, fmt.Errorf("%w: line %d: SECTION without name", ErrInvalid, t.line)
		}
		name := tags[i+1].value
		end := i + 2
		for end < len(tags) && (tags[end].code != 0 || tags[end].value != "ENDSEC") {
			end++
		}
		if end == len(tags) {
			return nil, fmt.Errorf("%w: line %d: section %s is not terminated by ENDSEC (truncated file?)", ErrInvalid, t.line, name)
		}
		body := tags[i+2 : end]
		switch name {
		case "HEADER":
			if err := f.readHeader(body); err != nil {
				return nil, err
			}
		case "BLOCKS":
			if err := f.readBlocks(body); err != nil {
				return nil, err
			}
		case "TABLES":
			if err := f.readTables(body); err != nil {
				return nil, err
			}
		case "ENTITIES":
			sawEntities = true
			if err := f.readEntities(body); err != nil {
				return nil, err
			}
		}
		i = end + 1
	}
	if !sawEntities {
		return nil, fmt.Errorf("%w: no ENTITIES section", ErrInvalid)
	}
	return f, nil
}

func (f *dxfFile) readHeader(body []tag) error {
	f.measurement = -1
	for i := range len(body) {
		if body[i].code == 9 && body[i].value == "$MEASUREMENT" && i+1 < len(body) && body[i+1].code == 70 {
			if v, err := strconv.Atoi(strings.TrimSpace(body[i+1].value)); err == nil && (v == 0 || v == 1) {
				f.measurement = v
			}
			continue
		}
		if body[i].code != 9 || body[i].value != "$INSUNITS" {
			continue
		}
		if i+1 >= len(body) || body[i+1].code != 70 {
			return fmt.Errorf("%w: line %d: $INSUNITS without value", ErrInvalid, body[i].line)
		}
		v, err := strconv.Atoi(body[i+1].value)
		if err != nil {
			return fmt.Errorf("%w: line %d: invalid $INSUNITS %q", ErrInvalid, body[i+1].line, truncate(body[i+1].value))
		}
		f.insUnits, f.hasInsUnits = v, true
	}
	return nil
}

// readTables reads the LTYPE and LAYER tables. Other tables are skipped.
func (f *dxfFile) readTables(body []tag) error {
	f.linetypes, f.layers = map[string]bool{}, map[string]layer{}
	var records []rawEntity
	for _, t := range body {
		if t.code == 0 {
			records = append(records, rawEntity{kind: t.value, line: t.line})
		} else if len(records) == 0 {
			return fmt.Errorf("%w: line %d: group code %d before first table", ErrInvalid, t.line, t.code)
		} else {
			records[len(records)-1].tags = append(records[len(records)-1].tags, t)
		}
	}
	for _, r := range records {
		name := strings.ToUpper(r.str(2))
		switch r.kind {
		case "LTYPE":
			gaps := false
			for _, t := range r.tags {
				if t.code != 49 {
					continue
				}
				v, err := parseFloat(t)
				if err != nil {
					return err
				}
				gaps = gaps || v < 0
			}
			f.linetypes[name] = gaps
		case "LAYER":
			color, err := r.int(62, 7)
			if err != nil {
				return err
			}
			flags, err := r.int(70, 0)
			if err != nil {
				return err
			}
			plot, err := r.int(290, 1)
			if err != nil {
				return err
			}
			l := layer{
				linetype: strings.ToUpper(r.str(6)),
				hidden:   color < 0 || flags&1 != 0 || plot == 0 || name == "DEFPOINTS",
				color:    aciColor(abs(color)),
			}
			if v := r.str(420); v != "" {
				if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
					l.color = fmt.Sprintf("#%06x", n&0xffffff)
				}
			}
			f.layers[name] = l
		}
	}
	return nil
}

// readBlocks reads the block definitions: each BLOCK record, the entities up to its ENDBLK, in order.
func (f *dxfFile) readBlocks(body []tag) error {
	f.blocks = map[string]block{}
	var cur *block
	var rec rawEntity // the open block's BLOCK record
	// target receives the tags that follow: the BLOCK record, the latest entity, or an ENDBLK record (whose tags are not needed).
	var target *rawEntity
	for _, t := range body {
		if t.code != 0 {
			if target == nil {
				return fmt.Errorf("%w: line %d: group code %d outside a block", ErrInvalid, t.line, t.code)
			}
			target.tags = append(target.tags, t)
			continue
		}
		switch {
		case t.value == "BLOCK":
			if cur != nil {
				return fmt.Errorf("%w: line %d: BLOCK inside another block", ErrInvalid, t.line)
			}
			cur, rec = &block{}, rawEntity{kind: t.value, line: t.line}
			target = &rec
		case t.value == "ENDBLK":
			if cur == nil {
				return fmt.Errorf("%w: line %d: ENDBLK without BLOCK", ErrInvalid, t.line)
			}
			if err := cur.fill(rec); err != nil {
				return err
			}
			f.blocks[strings.ToUpper(cur.name)] = *cur
			cur, target = nil, &rawEntity{kind: t.value, line: t.line}
		case cur == nil:
			return fmt.Errorf("%w: line %d: %s outside a block", ErrInvalid, t.line, truncate(t.value))
		default:
			cur.entities = append(cur.entities, rawEntity{kind: t.value, line: t.line, ordinal: len(cur.entities) + 1})
			target = &cur.entities[len(cur.entities)-1]
		}
	}
	if cur != nil {
		return fmt.Errorf("%w: block %q is not terminated by ENDBLK", ErrInvalid, truncate(cur.name))
	}
	return nil
}

// fill reads a block's name, base point and flags from its BLOCK record.
func (b *block) fill(rec rawEntity) error {
	b.name = rec.str(2)
	if b.name == "" {
		return fmt.Errorf("%w: line %d: BLOCK without a name", ErrInvalid, rec.line)
	}
	var err error
	if b.baseX, err = rec.float(10, 0); err != nil {
		return err
	}
	if b.baseY, err = rec.float(20, 0); err != nil {
		return err
	}
	flags, err := rec.int(70, 0)
	if err != nil {
		return err
	}
	b.external = flags&(4|8) != 0
	return nil
}

func (f *dxfFile) readEntities(body []tag) error {
	for _, t := range body {
		if t.code == 0 {
			f.entities = append(f.entities, rawEntity{kind: t.value, line: t.line, ordinal: len(f.entities) + 1})
			continue
		}
		if len(f.entities) == 0 {
			return fmt.Errorf("%w: line %d: group code %d before first entity", ErrInvalid, t.line, t.code)
		}
		e := &f.entities[len(f.entities)-1]
		e.tags = append(e.tags, t)
	}
	return nil
}

// id returns the entity handle (group 5), or "#<ordinal>" when the file was written without handles.
func (e rawEntity) id() string {
	for _, t := range e.tags {
		if t.code == 5 {
			return t.value
		}
	}
	return "#" + strconv.Itoa(e.ordinal)
}

// layer is the entity's layer name; an entity without one is on layer 0.
func (e rawEntity) layer() string {
	if name := e.str(8); name != "" {
		return name
	}
	return "0"
}

// errorf is an error about the entity, naming its kind, ID and line, wrapping base.
func (e rawEntity) errorf(base error, format string, args ...any) error {
	return fmt.Errorf("%w: %s %s (line %d) %s", base, e.kind, e.id(), e.line, fmt.Sprintf(format, args...))
}

// str returns the first value for code, or "" if absent.
func (e rawEntity) str(code int) string {
	for _, t := range e.tags {
		if t.code == code {
			return t.value
		}
	}
	return ""
}

// has reports whether the entity carries group code.
func (e rawEntity) has(code int) bool {
	for _, t := range e.tags {
		if t.code == code {
			return true
		}
	}
	return false
}

// float returns the first value for code, or def if absent.
func (e rawEntity) float(code int, def float64) (float64, error) {
	for _, t := range e.tags {
		if t.code == code {
			return parseFloat(t)
		}
	}
	return def, nil
}

// int returns the first value for code, or def if absent.
func (e rawEntity) int(code int, def int) (int, error) {
	for _, t := range e.tags {
		if t.code == code {
			v, err := strconv.Atoi(t.value)
			if err != nil {
				return 0, fmt.Errorf("%w: line %d: invalid integer %q for group code %d", ErrInvalid, t.line, truncate(t.value), t.code)
			}
			return v, nil
		}
	}
	return def, nil
}

// maxMagnitude bounds every number read from a DXF file (in drawing units). Real parts are many orders of magnitude smaller; the bound keeps every derived measurement well inside the exact nanometer range.
const maxMagnitude = 1e9

func parseFloat(t tag) (float64, error) {
	v, err := strconv.ParseFloat(t.value, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%w: line %d: invalid number %q for group code %d", ErrInvalid, t.line, truncate(t.value), t.code)
	}
	if math.Abs(v) > maxMagnitude {
		return 0, fmt.Errorf("%w: line %d: number %q for group code %d is out of range (|value| > 1e9)", ErrInvalid, t.line, truncate(t.value), t.code)
	}
	return v, nil
}

func truncate(s string) string {
	const limit = 40
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}
