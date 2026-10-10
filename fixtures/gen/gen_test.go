package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// generatedDXF returns the generated fixtures under dir, excluding fixtures/exported, which holds files exported from CAD software.
func generatedDXF(dir string) []string {
	return notExported(dir, "*.dxf")
}

// notExported returns the files under dir matching pattern, excluding fixtures/exported.
func notExported(dir, pattern string) []string {
	var out []string
	paths, _ := filepath.Glob(filepath.Join(dir, "*", pattern))
	for _, p := range paths {
		if filepath.Base(filepath.Dir(p)) != "exported" {
			out = append(out, p)
		}
	}
	return out
}

// incomplete lists fixtures that are deliberately not complete R2000 drawings.
var incomplete = map[string]bool{
	"valid/polyline_r12_no_handles.dxf": true, // R12 needs only ENTITIES
	"malformed/not_a_dxf.dxf":           true,
	"malformed/truncated.dxf":           true,
	"malformed/block_undefined.dxf":     true, // refers to a block it does not define, on purpose
}

// TestCommittedFixturesUpToDate fails if fixtures/ differs from what the generator produces, i.e. someone edited a fixture by hand or forgot to run go run ./fixtures/gen.
func TestCommittedFixturesUpToDate(t *testing.T) {
	dir := t.TempDir()
	if err := generate(dir, filepath.Join("..", "..", "examples")); err != nil {
		t.Fatal(err)
	}
	generated, _ := filepath.Glob(filepath.Join(dir, "*", "*.dxf"))
	for _, pattern := range []string{"*.step", "*.stock.json", "*.capability.json", "*.plan.json"} {
		sidecars, _ := filepath.Glob(filepath.Join(dir, "*", pattern))
		generated = append(generated, sidecars...)
	}
	committed := generatedDXF("..")
	for _, pattern := range []string{"*.step", "*.stock.json", "*.capability.json", "*.plan.json"} {
		committed = append(committed, notExported("..", pattern)...)
	}
	if len(generated) != len(committed) {
		t.Errorf("generator writes %d fixtures, fixtures/ has %d", len(generated), len(committed))
	}
	for _, g := range generated {
		rel, _ := filepath.Rel(dir, g)
		want, _ := os.ReadFile(g)
		got, err := os.ReadFile(filepath.Join("..", rel))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("fixtures/%s is out of date; run go run ./fixtures/gen", filepath.ToSlash(rel))
		}
	}
}

// TestDrawingStructure audits every complete fixture.
func TestDrawingStructure(t *testing.T) {
	paths := generatedDXF("..")
	n := 0
	for _, p := range paths {
		rel, _ := filepath.Rel("..", p)
		rel = filepath.ToSlash(rel)
		if incomplete[rel] {
			continue
		}
		n++
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.HasPrefix(data, []byte(binarySentinel)) {
				t.Skip("binary DXF: audited as the ASCII drawing it encodes")
			}
			for _, problem := range audit(data) {
				t.Error(problem)
			}
		})
	}
	if n == 0 {
		t.Fatal("no fixtures audited")
	}
}

// TestAuditDetectsBrokenDrawings proves the audit is not vacuous.
func TestAuditDetectsBrokenDrawings(t *testing.T) {
	d := newDoc(unitsMM)
	d.rect(0, 0, 10, 10, true)
	d.text(1, 1, 1, "x")
	good := string(d.bytes())
	if problems := audit([]byte(good)); len(problems) > 0 {
		t.Fatalf("good drawing has problems: %v", problems)
	}
	tests := []struct {
		name, old, new, want string
	}{
		{"dangling owner", "330\n1F\n100\nAcDbEntity\n8\n0\n100\nAcDbPolyline", "330\n99\n100\nAcDbEntity\n8\n0\n100\nAcDbPolyline", "330 → 99"},
		{"duplicate handle", "5\n30\n330\n1F", "5\n2F\n330\n1F", "duplicate handle 2F"},
		{"low handseed", "$HANDSEED\n5\n200", "$HANDSEED\n5\n2F", "$HANDSEED"},
		{"unknown layer", "330\n1F\n100\nAcDbEntity\n8\n0\n100\nAcDbText", "330\n1F\n100\nAcDbEntity\n8\nCUT\n100\nAcDbText", `layer "CUT"`},
		{"unknown text style", "7\nStandard\n100\nAcDbText", "7\nBold\n100\nAcDbText", `style "Bold"`},
		{"missing table", "2\nAPPID\n", "2\nAPPIX\n", "APPID"},
		{"undeclared class", "1\nLAYOUT\n2\nAcDbLayout", "1\nLAYOUX\n2\nAcDbLayout", "LAYOUT"},
		{"wrong section order", "2\nCLASSES", "2\nCLASSEZ", "sections"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(good, tt.old) {
				t.Fatalf("mutation target %q not found", tt.old)
			}
			broken := strings.Replace(good, tt.old, tt.new, 1)
			problems := strings.Join(audit([]byte(broken)), "\n")
			if !strings.Contains(problems, tt.want) {
				t.Errorf("want a problem mentioning %q, got:\n%s", tt.want, problems)
			}
		})
	}
}

// --- audit ------------------------------------------------------------------

type tag struct {
	code  int
	value string
}

// object is one DXF object: a "0" tag and the tags up to the next "0".
type object struct {
	kind string
	tags []tag
}

func (o object) get(code int) string {
	for _, t := range o.tags {
		if t.code == code {
			return t.value
		}
	}
	return ""
}

func (o object) handle() string {
	if o.kind == "DIMSTYLE" {
		return o.get(105)
	}
	return o.get(5)
}

// owner returns the first 330 outside a {ACAD_REACTORS ... } group.
func (o object) owner() string {
	inGroup := false
	for _, t := range o.tags {
		switch {
		case t.code == 102:
			inGroup = strings.HasPrefix(t.value, "{")
		case t.code == 330 && !inGroup:
			return t.value
		}
	}
	return ""
}

// last returns the last value for code.
func (o object) last(code int) string {
	v := ""
	for _, t := range o.tags {
		if t.code == code {
			v = t.value
		}
	}
	return v
}

// builtinObjects need no CLASSES entry.
var builtinObjects = map[string]bool{"DICTIONARY": true, "MLINESTYLE": true, "GROUP": true, "XRECORD": true}

var requiredTables = []string{"VPORT", "LTYPE", "LAYER", "STYLE", "VIEW", "UCS", "APPID", "DIMSTYLE", "BLOCK_RECORD"}

// audit checks that data is a structurally complete DXF R2000 drawing and returns every problem found.
func audit(data []byte) (problems []string) {
	fail := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines)%2 != 0 {
		return []string{"odd number of lines"}
	}
	sections := map[string][]object{}
	var order []string
	var cur *[]object
	for i := 0; i < len(lines); i += 2 {
		code, err := strconv.Atoi(strings.TrimSpace(lines[i]))
		if err != nil {
			return []string{fmt.Sprintf("line %d: bad group code", i+1)}
		}
		t := tag{code, lines[i+1]}
		switch {
		case code == 0 && t.value == "SECTION":
			name := lines[i+3]
			order = append(order, name)
			sections[name] = nil
			s := sections[name]
			cur = &s
			i += 2
		case code == 0 && t.value == "ENDSEC":
			sections[order[len(order)-1]] = *cur
			cur = nil
		case code == 0 && t.value == "EOF":
		case code == 0:
			*cur = append(*cur, object{kind: t.value})
		default:
			if cur == nil || len(*cur) == 0 {
				if order[len(order)-1] != "HEADER" {
					fail("line %d: tag outside an object", i+1)
				}
				continue
			}
			o := &(*cur)[len(*cur)-1]
			o.tags = append(o.tags, t)
		}
	}
	if got := strings.Join(order, ","); got != "HEADER,CLASSES,TABLES,BLOCKS,ENTITIES,OBJECTS" {
		fail("sections are %s, want HEADER,CLASSES,TABLES,BLOCKS,ENTITIES,OBJECTS", got)
	}

	// Handles: present, unique, below $HANDSEED.
	byHandle := map[string]object{}
	maxHandle := int64(0)
	for _, name := range []string{"TABLES", "BLOCKS", "ENTITIES", "OBJECTS"} {
		for _, o := range sections[name] {
			if o.kind == "ENDTAB" {
				continue
			}
			h := o.handle()
			if h == "" {
				fail("%s %s has no handle", name, o.kind)
				continue
			}
			if _, dup := byHandle[h]; dup {
				fail("duplicate handle %s", h)
			}
			byHandle[h] = o
			if v, err := strconv.ParseInt(h, 16, 64); err == nil && v > maxHandle {
				maxHandle = v
			}
		}
	}
	header := headerVars(data)
	if seed, err := strconv.ParseInt(header["$HANDSEED"], 16, 64); err != nil || seed <= maxHandle {
		fail("$HANDSEED %q must exceed the largest handle %X", header["$HANDSEED"], maxHandle)
	}

	// Every handle reference resolves.
	for _, o := range byHandle {
		for _, t := range o.tags {
			switch t.code {
			case 330, 340, 350, 360, 390:
				if t.value != "0" {
					if _, ok := byHandle[t.value]; !ok {
						fail("%s %s: reference %d → %s does not exist", o.kind, o.handle(), t.code, t.value)
					}
				}
			}
		}
	}

	// Tables: all present; records owned by their table; collect names.
	names := map[string]map[string]string{} // table → record name → handle
	var table object
	for _, o := range sections["TABLES"] {
		switch o.kind {
		case "TABLE":
			table = o
			names[o.get(2)] = map[string]string{}
		case "ENDTAB":
		default:
			if o.owner() != table.handle() {
				fail("%s %q is owned by %s, not its table %s", o.kind, o.get(2), o.owner(), table.handle())
			}
			names[table.get(2)][o.get(2)] = o.handle()
		}
	}
	for _, tbl := range requiredTables {
		if _, ok := names[tbl]; !ok {
			fail("missing %s table", tbl)
		}
	}
	records := names["BLOCK_RECORD"]

	// Blocks: one BLOCK/ENDBLK per record; contents owned by the record.
	begun := map[string]bool{}
	var open object
	for _, o := range sections["BLOCKS"] {
		switch o.kind {
		case "BLOCK":
			open = o
			rec := records[o.get(2)]
			if rec == "" || o.owner() != rec {
				fail("BLOCK %q is not owned by its BLOCK_RECORD", o.get(2))
			}
			begun[o.get(2)] = true
		case "ENDBLK":
			if o.owner() != open.owner() {
				fail("ENDBLK of %q has the wrong owner", open.get(2))
			}
		default:
			if o.owner() != open.owner() {
				fail("%s %s inside block %q has owner %s", o.kind, o.handle(), open.get(2), o.owner())
			}
		}
	}
	for name := range records {
		if !begun[name] {
			fail("BLOCK_RECORD %q has no BLOCK", name)
		}
	}

	// Layouts link both ways with the space block records.
	for _, space := range []string{"*Model_Space", "*Paper_Space"} {
		rec := byHandle[records[space]]
		lay, ok := byHandle[rec.get(340)]
		if !ok || lay.kind != "LAYOUT" || lay.last(330) != rec.handle() {
			fail("%s and its LAYOUT do not reference each other", space)
		}
	}

	// Entities: owned by a space; names resolve.
	for _, o := range append(append([]object(nil), sections["ENTITIES"]...), sections["BLOCKS"]...) {
		if o.kind == "BLOCK" || o.kind == "ENDBLK" {
			continue
		}
		if layer := o.get(8); names["LAYER"][layer] == "" {
			fail("%s %s uses undefined layer %q", o.kind, o.handle(), layer)
		}
		if lt := o.get(6); lt != "" && names["LTYPE"][lt] == "" {
			fail("%s %s uses undefined linetype %q", o.kind, o.handle(), lt)
		}
		switch o.kind {
		case "TEXT", "MTEXT":
			if st := o.get(7); st != "" && names["STYLE"][st] == "" {
				fail("%s %s uses undefined text style %q", o.kind, o.handle(), st)
			}
		case "INSERT", "DIMENSION":
			if records[o.get(2)] == "" {
				fail("%s %s references undefined block %q", o.kind, o.handle(), o.get(2))
			}
		}
		if o.kind == "DIMENSION" && names["DIMSTYLE"][o.get(3)] == "" {
			fail("DIMENSION %s uses undefined dimension style %q", o.handle(), o.get(3))
		}
	}
	for _, o := range sections["ENTITIES"] {
		want := records["*Model_Space"]
		if o.get(67) == "1" {
			want = records["*Paper_Space"]
		}
		if o.owner() != want {
			fail("%s %s has owner %s, want %s", o.kind, o.handle(), o.owner(), want)
		}
	}
	for v, table := range map[string]string{"$CLAYER": "LAYER", "$CELTYPE": "LTYPE", "$TEXTSTYLE": "STYLE", "$DIMSTYLE": "DIMSTYLE"} {
		if names[table][header[v]] == "" {
			fail("header %s refers to undefined %s %q", v, table, header[v])
		}
	}

	// Objects: class-based objects are declared in CLASSES.
	declared := map[string]bool{}
	for _, c := range sections["CLASSES"] {
		declared[c.get(1)] = true
	}
	for _, o := range sections["OBJECTS"] {
		if !builtinObjects[o.kind] && !declared[o.kind] {
			fail("object type %s is not declared in CLASSES", o.kind)
		}
	}
	return problems
}

// headerVars returns the first value of each $VARIABLE in the HEADER section.
func headerVars(data []byte) map[string]string {
	vars := map[string]string{}
	lines := strings.Split(string(data), "\n")
	for i := 0; i+3 < len(lines); i += 2 {
		if lines[i] == "0" && lines[i+1] == "ENDSEC" {
			break
		}
		if lines[i] == "9" {
			vars[lines[i+1]] = lines[i+3]
		}
	}
	return vars
}
