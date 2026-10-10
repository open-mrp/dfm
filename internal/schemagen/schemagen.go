// Package schemagen generates JSON Schemas from request and resource Go types, so the published schemas cannot drift from the code. Descriptions come from the types' doc comments.
package schemagen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Mode selects how required fields are decided.
type Mode int

const (
	// Request: a field is required when its validate tag says so.
	Request Mode = iota
	// Response: every field is always present (null when absent).
	Response
)

// DecimalPattern is the wire grammar of decimal strings.
const DecimalPattern = `^-?[0-9]+(\.[0-9]+)?$`

type elemTyper interface{ ElemType() reflect.Type }

type enum interface {
	IsValid() bool
	EnumValues() []string
}

// openEnum is an enum that gains values over time (processes, formats, units, finding codes). A request still has to use a value this engine knows, so request schemas list them; a response may carry a value a client has not seen, so response schemas describe the known values without closing the set.
type openEnum interface {
	Open() bool
}

var (
	elemTyperType = reflect.TypeFor[elemTyper]()
	enumType      = reflect.TypeFor[enum]()
	openEnumType  = reflect.TypeFor[openEnum]()
)

// Docs maps "pkg.Type" and "pkg.Type.Field" to doc comments.
type Docs map[string]string

// LoadDocs reads the doc comments of the Go files in dirs.
func LoadDocs(dirs ...string) (Docs, error) {
	docs := Docs{}
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			return nil, err
		}
		fset := token.NewFileSet()
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			file, err := parser.ParseFile(fset, f, src, parser.ParseComments)
			if err != nil {
				return nil, err
			}
			pkg := file.Name.Name
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					typeDoc := ts.Doc
					if typeDoc == nil {
						typeDoc = gen.Doc
					}
					docs[pkg+"."+ts.Name.Name] = withoutSubject(clean(typeDoc), ts.Name.Name)
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, fld := range st.Fields.List {
						for _, n := range fld.Names {
							docs[pkg+"."+ts.Name.Name+"."+n.Name] = clean(fld.Doc)
						}
					}
				}
			}
		}
	}
	return docs, nil
}

// withoutSubject turns Go's "Name is a thing" doc style into "A thing".
func withoutSubject(d, name string) string {
	rest, ok := strings.CutPrefix(d, name+" ")
	if !ok {
		return d
	}
	for _, verb := range []string{"is ", "are "} {
		rest = strings.TrimPrefix(rest, verb)
	}
	if rest == "" {
		return d
	}
	return strings.ToUpper(rest[:1]) + rest[1:]
}

// clean joins each comment paragraph onto one line.
func clean(g *ast.CommentGroup) string {
	if g == nil {
		return ""
	}
	var paras []string
	for _, p := range strings.Split(strings.TrimSpace(g.Text()), "\n\n") {
		paras = append(paras, strings.Join(strings.Fields(p), " "))
	}
	return strings.Join(paras, "\n\n")
}

type generator struct {
	mode Mode
	docs Docs
	defs map[string]any
}

// Generate returns a schema document whose root is t.
func Generate(t reflect.Type, mode Mode, docs Docs, id, title string) map[string]any {
	g := &generator{mode: mode, docs: docs, defs: map[string]any{}}
	root := g.schemaFor(t)
	out := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     id,
		"title":   title,
	}
	for k, v := range root {
		out[k] = v
	}
	if len(g.defs) > 0 {
		out["$defs"] = g.defs
	}
	return out
}

var nonIdent = regexp.MustCompile(`[^A-Za-z0-9]+`)

func defName(t reflect.Type) string {
	name := t.Name()
	if i := strings.Index(name, "["); i >= 0 {
		inner := name[i+1 : len(name)-1]
		inner = inner[strings.LastIndex(inner, ".")+1:]
		name = name[:i] + "_" + inner
	}
	return nonIdent.ReplaceAllString(name, "_")
}

func docKey(t reflect.Type) string {
	pkg := t.PkgPath()
	pkg = pkg[strings.LastIndex(pkg, "/")+1:]
	name := t.Name()
	if i := strings.Index(name, "["); i >= 0 {
		name = name[:i]
	}
	return pkg + "." + name
}

func (g *generator) schemaFor(t reflect.Type) map[string]any {
	if t.Implements(elemTyperType) {
		t = reflect.Zero(t).Interface().(elemTyper).ElemType()
	}
	if t.Kind() == reflect.Pointer {
		return g.schemaFor(t.Elem())
	}
	switch t.Kind() {
	case reflect.Struct:
		name := defName(t)
		if _, ok := g.defs[name]; !ok {
			g.defs[name] = nil // reserve against recursion
			g.defs[name] = g.object(t)
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": g.schemaFor(t.Elem())}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.String:
		s := map[string]any{"type": "string"}
		if t.Implements(enumType) {
			values := reflect.Zero(t).Interface().(enum).EnumValues()
			if t.Implements(openEnumType) && g.mode != Request {
				s["description"] = "Open enum: one of " + strings.Join(values, ", ") + " today; values are added as processes are, so treat an unknown one as not handled rather than as an error."
			} else {
				s["enum"] = values
			}
		} else if g.mode == Request {
			s["minLength"] = 1
		}
		return s
	default:
		panic("schemagen: unsupported kind " + t.Kind().String() + " in " + t.String())
	}
}

func (g *generator) object(t reflect.Type) map[string]any {
	props := map[string]any{}
	required := []any{}
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" || !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		s := g.schemaFor(f.Type)
		if f.Tag.Get("format") == "decimal" {
			s = map[string]any{"type": "string", "pattern": DecimalPattern}
		}
		if c, ok := f.Tag.Lookup("const"); ok {
			s = map[string]any{"const": c}
		}
		if n, ok := f.Tag.Lookup("minItems"); ok {
			s["minItems"], _ = strconv.Atoi(n)
		}
		nullable := g.mode == Response && (f.Type.Kind() == reflect.Pointer || f.Type.Kind() == reflect.Slice)
		if nullable {
			s = map[string]any{"anyOf": []any{s, map[string]any{"type": "null"}}}
		}
		if d := g.docs[docKey(t)+"."+f.Name]; d != "" {
			s["description"] = d
		}
		props[name] = s
		isRequired := strings.Contains(f.Tag.Get("validate"), "required")
		if g.mode == Response {
			isRequired = !strings.Contains(opts, "omitempty")
		}
		if isRequired {
			required = append(required, name)
		}
	}
	out := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
		"required":             required,
	}
	if d := g.docs[docKey(t)]; d != "" {
		out["description"] = d
	}
	return out
}
