// Package request decodes and validates wire requests and converts them to domain types. Decimal values are JSON strings; physical amounts are quantities {value, unit}.
package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/constants"
)

type elemTyper interface{ ElemType() reflect.Type }

type enum interface {
	IsValid() bool
	EnumValues() []string
}

var (
	elemTyperType = reflect.TypeFor[elemTyper]()
	enumType      = reflect.TypeFor[enum]()
)

// Decode strictly decodes data into dst (a pointer to a request struct). Unknown parameters, missing required parameters, explicit nulls, blanks, wrong JSON types (including numbers where a decimal string is expected), invalid enum values, a value other than a field's `const` tag (a schema version) and an array shorter than its `minItems` tag are rejected with the offending param path, prefixed by prefix.
func Decode(data []byte, dst any, prefix string) *apierror.Error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return apierror.New(constants.ErrorCodeInvalidFormat, prefix, "request is not valid JSON: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return apierror.New(constants.ErrorCodeInvalidFormat, prefix, "request has data after the JSON document")
	}
	var errs []*apierror.Error
	check(raw, reflect.TypeOf(dst).Elem(), prefix, &errs)
	if e := apierror.Join(errs); e != nil {
		return e
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return apierror.Internal(fmt.Errorf("decoding a request that passed validation: %w", err))
	}
	return nil
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// check validates v against type t, appending every failing parameter to errs so a caller sees them all at once.
func check(v any, t reflect.Type, path string, errs *[]*apierror.Error) {
	if t.Implements(elemTyperType) {
		t = reflect.Zero(t).Interface().(elemTyper).ElemType()
	}
	add := func(e *apierror.Error) { *errs = append(*errs, e) }
	switch t.Kind() {
	case reflect.Struct:
		checkObject(v, t, path, errs)
	case reflect.Slice:
		arr, ok := v.([]any)
		if !ok {
			add(apierror.ParameterInvalid(path, "%s must be an array", path))
			return
		}
		for i, item := range arr {
			if item == nil {
				add(apierror.ParameterInvalid(fmt.Sprintf("%s[%d]", path, i), "%s[%d] cannot be null", path, i))
				continue
			}
			check(item, t.Elem(), fmt.Sprintf("%s[%d]", path, i), errs)
		}
	case reflect.String:
		if e := checkString(v, t, path); e != nil {
			add(e)
		}
	case reflect.Bool:
		if _, ok := v.(bool); !ok {
			add(apierror.ParameterInvalid(path, "%s must be true or false", path))
		}
	case reflect.Int:
		n, ok := v.(json.Number)
		if _, err := n.Int64(); !ok || err != nil {
			add(apierror.ParameterInvalid(path, "%s must be a whole number", path))
		}
	default:
		add(apierror.Internal(fmt.Errorf("request type %s has unsupported field kind %s", t, t.Kind())))
	}
}

func checkString(v any, t reflect.Type, path string) *apierror.Error {
	s, ok := v.(string)
	if !ok {
		if _, isNumber := v.(json.Number); isNumber {
			return apierror.ParameterInvalid(path, "%s must be a string; decimal values are sent as strings, e.g. \"1.5\"", path)
		}
		return apierror.ParameterInvalid(path, "%s must be a string", path)
	}
	if s == "" {
		return apierror.ParameterInvalid(path, "%s must not be blank", path)
	}
	if t.Implements(enumType) {
		e := reflect.ValueOf(s).Convert(t).Interface().(enum)
		if !e.IsValid() {
			return apierror.ParameterInvalid(path, "%s %q is not one of %s", path, s, strings.Join(e.EnumValues(), ", "))
		}
	}
	return nil
}

func checkObject(v any, t reflect.Type, path string, errs *[]*apierror.Error) {
	add := func(e *apierror.Error) { *errs = append(*errs, e) }
	m, ok := v.(map[string]any)
	if !ok {
		add(apierror.ParameterInvalid(path, "%s must be an object", path))
		return
	}
	known := map[string]bool{}
	for i := range t.NumField() {
		if name := jsonName(t.Field(i)); name != "" {
			known[name] = true
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !known[k] {
			e := apierror.ParameterUnknown(join(path, k))
			if s := suggest(k, known); s != "" {
				e.Message += fmt.Sprintf("; did you mean %s?", join(path, s))
			}
			add(e)
		}
	}
	for i := range t.NumField() {
		f := t.Field(i)
		name := jsonName(f)
		if name == "" {
			continue
		}
		val, present := m[name]
		required := strings.Contains(f.Tag.Get("validate"), "required")
		p := join(path, name)
		switch {
		case !present && required, present && val == nil && required:
			add(apierror.ParameterMissing(p))
			continue
		case !present:
			continue
		case val == nil:
			add(apierror.ParameterInvalid(p, "%s cannot be null; omit it instead", p))
			continue
		}
		if n, ok := f.Tag.Lookup("minItems"); ok {
			least, _ := strconv.Atoi(n)
			if arr, isArray := val.([]any); isArray && len(arr) < least {
				if least == 1 {
					add(apierror.ParameterInvalid(p, "%s must not be empty", p))
				} else {
					add(apierror.ParameterInvalid(p, "%s must list at least %d", p, least))
				}
				continue
			}
		}
		if want, ok := f.Tag.Lookup("const"); ok {
			if s, isString := val.(string); isString && s != want {
				add(apierror.ParameterInvalid(p, "%s %q is not supported (supported: %q)", name, s, want))
				continue
			}
		}
		check(val, f.Type, p, errs)
	}
}

func jsonName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "-" || !f.IsExported() {
		return ""
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return f.Name
	}
	return name
}

// suggest returns the known name closest to k, if it is a plausible typo.
func suggest(k string, known map[string]bool) string {
	best, bestDist := "", 3
	names := make([]string, 0, len(known))
	for n := range known {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if d := levenshtein(k, n); d < bestDist {
			best, bestDist = n, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
