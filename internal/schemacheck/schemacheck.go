// Package schemacheck is a minimal JSON Schema validator used by tests to prove that the published schemas in schema/ agree with the Go implementation.
//
// It supports exactly the keywords those schemas use. Any other keyword is an error when the schema is loaded, so a schema can never be silently under-validated because this package ignored part of it.
package schemacheck

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// annotations carry documentation only and do not affect validation.
var annotations = map[string]bool{
	"$schema": true, "$id": true, "title": true, "description": true,
	"examples": true, "$defs": true,
}

// keywords are the validation keywords this package implements.
var keywords = map[string]bool{
	"type": true, "properties": true, "required": true,
	"additionalProperties": true, "enum": true, "const": true,
	"minimum": true, "exclusiveMinimum": true, "items": true,
	"minItems": true, "$ref": true, "minProperties": true, "pattern": true,
	"minLength": true, "anyOf": true,
}

// Schema is a loaded schema document.
type Schema struct {
	root map[string]any
}

// Load parses a schema document and rejects unsupported keywords.
func Load(data []byte) (*Schema, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if err := checkKeywords(root, "#"); err != nil {
		return nil, err
	}
	return &Schema{root: root}, nil
}

func checkKeywords(s map[string]any, path string) error {
	for k, v := range s {
		if !annotations[k] && !keywords[k] {
			return fmt.Errorf("%s: unsupported keyword %q", path, k)
		}
		switch k {
		case "properties", "$defs":
			for name, sub := range v.(map[string]any) {
				if err := checkKeywords(sub.(map[string]any), path+"/"+k+"/"+name); err != nil {
					return err
				}
			}
		case "items":
			if err := checkKeywords(v.(map[string]any), path+"/items"); err != nil {
				return err
			}
		case "anyOf":
			for i, sub := range v.([]any) {
				if err := checkKeywords(sub.(map[string]any), fmt.Sprintf("%s/anyOf/%d", path, i)); err != nil {
					return err
				}
			}
		case "additionalProperties":
			if b, ok := v.(bool); !ok || b {
				return fmt.Errorf("%s: only additionalProperties: false is supported", path)
			}
		case "pattern":
			if _, err := regexp.Compile(v.(string)); err != nil {
				return fmt.Errorf("%s: pattern: %v", path, err)
			}
		}
	}
	return nil
}

// Validate checks a JSON document against the schema and returns every violation found.
func (s *Schema) Validate(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	var errs []string
	s.validate(s.root, v, "$", &errs)
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (s *Schema) validate(schema map[string]any, v any, path string, errs *[]string) {
	fail := func(format string, args ...any) {
		*errs = append(*errs, path+": "+fmt.Sprintf(format, args...))
	}

	if ref, ok := schema["$ref"].(string); ok {
		name, found := strings.CutPrefix(ref, "#/$defs/")
		def, ok := s.root["$defs"].(map[string]any)[name].(map[string]any)
		if !found || !ok {
			fail("unresolvable $ref %q", ref)
			return
		}
		s.validate(def, v, path, errs)
	}

	if t, ok := schema["type"].(string); ok && !hasType(v, t) {
		fail("want type %s, got %s", t, typeOf(v))
		return
	}
	if c, ok := schema["const"]; ok && !reflect.DeepEqual(v, c) {
		fail("want %v, got %v", c, v)
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			found = found || reflect.DeepEqual(v, e)
		}
		if !found {
			fail("%v is not one of %v", v, enum)
		}
	}
	if n, ok := v.(float64); ok {
		if min, ok := schema["minimum"].(float64); ok && n < min {
			fail("%v is less than minimum %v", n, min)
		}
		if min, ok := schema["exclusiveMinimum"].(float64); ok && n <= min {
			fail("%v is not greater than %v", n, min)
		}
	}

	if alts, ok := schema["anyOf"].([]any); ok {
		matched := false
		for _, alt := range alts {
			var sub []string
			s.validate(alt.(map[string]any), v, path, &sub)
			matched = matched || len(sub) == 0
		}
		if !matched {
			fail("matches none of the anyOf alternatives")
		}
	}
	if minLen, ok := schema["minLength"].(float64); ok {
		if str, isStr := v.(string); isStr && float64(len([]rune(str))) < minLen {
			fail("want at least %v characters", minLen)
		}
	}
	if pat, ok := schema["pattern"].(string); ok {
		if str, isStr := v.(string); isStr && !regexp.MustCompile(pat).MatchString(str) {
			fail("%q does not match %s", str, pat)
		}
	}

	switch val := v.(type) {
	case map[string]any:
		if min, ok := schema["minProperties"].(float64); ok && float64(len(val)) < min {
			fail("want at least %v properties, got %d", min, len(val))
		}
		props, _ := schema["properties"].(map[string]any)
		required, _ := schema["required"].([]any)
		for _, r := range required {
			if _, ok := val[r.(string)]; !ok {
				fail("missing required property %q", r)
			}
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sub, ok := props[k].(map[string]any)
			if !ok {
				if _, closed := schema["additionalProperties"]; closed {
					fail("unexpected property %q", k)
				}
				continue
			}
			s.validate(sub, val[k], path+"."+k, errs)
		}
	case []any:
		if min, ok := schema["minItems"].(float64); ok && float64(len(val)) < min {
			fail("want at least %v items, got %d", min, len(val))
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, item := range val {
				s.validate(items, item, fmt.Sprintf("%s[%d]", path, i), errs)
			}
		}
	}
}

func hasType(v any, t string) bool {
	switch t {
	case "object", "array", "string", "boolean", "null":
		return typeOf(v) == t
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	}
	return false
}

func typeOf(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case nil:
		return "null"
	}
	return fmt.Sprintf("%T", v)
}
