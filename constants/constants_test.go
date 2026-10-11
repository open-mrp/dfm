package constants

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type enum interface {
	IsValid() bool
	EnumValues() []string
}

// enums is every enum type in the package. TestEnumDeclarations fails when a type is missing from it.
var enums = map[string]enum{
	"AngleUnit": AngleUnit(""), "AreaUnit": AreaUnit(""), "BendDirection": BendDirection(""), "BendLimitOffset": BendLimitOffset(""), "CommonLine": CommonLine(""),
	"DesignFormat": DesignFormat(""), "ErrorCode": ErrorCode(""), "ErrorType": ErrorType(""),
	"FindingCode": FindingCode(""), "FindingScope": FindingScope(""), "FixAction": FixAction(""),
	"ForceUnit": ForceUnit(""), "HoleOperationType": HoleOperationType(""), "HoleSide": HoleSide(""), "LengthUnit": LengthUnit(""), "LineSide": LineSide(""),
	"MultipleParts": MultipleParts(""), "ObjectType": ObjectType(""), "OperationVerdict": OperationVerdict(""), "Outline": Outline(""), "OutsidePage": OutsidePage(""), "PlanVerdict": PlanVerdict(""),
	"Process": Process(""), "PunchPosition": PunchPosition(""), "Severity": Severity(""), "ShapeOperation": ShapeOperation(""), "StockForm": StockForm(""),
	"ThresholdBasis": ThresholdBasis(""), "Unit": Unit(""), "Verdict": Verdict(""),
}

func TestEnums(t *testing.T) {
	for name, e := range enums {
		if e.IsValid() {
			t.Errorf("%s: empty value is valid", name)
		}
		seen := map[string]bool{}
		for _, v := range e.EnumValues() {
			// Unit symbols keep their SI case (kN).
			if siSymbol := v == "kN"; seen[v] || (v != strings.ToLower(v) && !siSymbol) || strings.ContainsAny(v, " -") {
				t.Errorf("%s: value %q is duplicated or not lowercase snake_case", name, v)
			}
			seen[v] = true
		}
	}
	for _, u := range lengthUnits {
		if u.Nanometers() <= 0 {
			t.Errorf("unit %s has no conversion", u)
		}
	}
	for _, u := range areaUnits {
		if u.SquareMicrometers() <= 0 {
			t.Errorf("unit %s has no conversion", u)
		}
	}
}

// TestEnumDeclarations reads the package source and checks, for every string type, that its EnumValues are exactly its typed constants in declaration order, so a constant cannot be left out of the value slice (invalid on the wire, missing from the schema) or listed out of order, and that the enums table above names every type.
func TestEnumDeclarations(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	type constant struct {
		name string
		pos  token.Pos
	}
	var types []string
	consts := map[string][]constant{} // by type
	exprs := map[string]ast.Expr{}    // by constant name
	for _, f := range pkgs["constants"].Files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if id, ok := s.Type.(*ast.Ident); ok && id.Name == "string" {
						types = append(types, s.Name.Name)
					}
				case *ast.ValueSpec:
					typ, ok := s.Type.(*ast.Ident)
					if gd.Tok != token.CONST || !ok {
						continue
					}
					for i, name := range s.Names {
						exprs[name.Name] = s.Values[i]
						consts[typ.Name] = append(consts[typ.Name], constant{name.Name, name.Pos()})
					}
				}
			}
		}
	}
	var value func(e ast.Expr) string
	value = func(e ast.Expr) string {
		switch e := e.(type) {
		case *ast.BasicLit:
			s, err := strconv.Unquote(e.Value)
			if err != nil {
				t.Fatal(err)
			}
			return s
		case *ast.CallExpr: // a conversion of another constant, Unit(AngleUnitDegree)
			if id, ok := e.Args[0].(*ast.Ident); ok && len(e.Args) == 1 {
				return value(exprs[id.Name])
			}
		}
		t.Fatalf("constant value %s is not a string literal or a conversion of one", fset.Position(e.Pos()))
		return ""
	}
	sort.Strings(types)
	for _, typ := range types {
		e, ok := enums[typ]
		if !ok {
			t.Errorf("%s is not in the enums table", typ)
			continue
		}
		cs := consts[typ]
		slices.SortFunc(cs, func(a, b constant) int { return int(a.pos - b.pos) })
		var want []string
		for _, c := range cs {
			want = append(want, value(exprs[c.name]))
		}
		if got := e.EnumValues(); !slices.Equal(got, want) {
			t.Errorf("%s: EnumValues() = %v\nconstants in declaration order: %v", typ, got, want)
		}
	}
	if len(types) != len(enums) {
		t.Errorf("%d enum types in source, %d in the enums table", len(types), len(enums))
	}
}
