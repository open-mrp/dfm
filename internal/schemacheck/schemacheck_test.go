package schemacheck

import (
	"strings"
	"testing"
)

const testSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["name", "size", "tags"],
  "properties": {
    "name": {"const": "x"},
    "id": {"type": "string", "pattern": "^[a-z]+$"},
    "opts": {"type": "object", "minProperties": 1},
    "nick": {"type": "string", "minLength": 2},
    "maybe": {"anyOf": [{"type": "string"}, {"type": "null"}]},
    "kind": {"enum": ["a", "b"]},
    "size": {"type": "number", "exclusiveMinimum": 0},
    "min": {"type": "number", "minimum": 0},
    "tags": {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/tag"}}
  },
  "$defs": {"tag": {"type": "string"}}
}`

func TestValidate(t *testing.T) {
	s, err := Load([]byte(testSchema))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		doc  string
		want string // substring of the error, "" for valid
	}{
		{`{"name":"x","size":1,"tags":["t"]}`, ""},
		{`{"name":"x","kind":"b","size":1,"min":0,"tags":["t"]}`, ""},
		{`{"size":1,"tags":["t"]}`, `missing required property "name"`},
		{`{"name":"y","size":1,"tags":["t"]}`, "want x"},
		{`{"name":"x","kind":"c","size":1,"tags":["t"]}`, "not one of"},
		{`{"name":"x","size":0,"tags":["t"]}`, "not greater than 0"},
		{`{"name":"x","size":1,"min":-1,"tags":["t"]}`, "less than minimum"},
		{`{"name":"x","size":"1","tags":["t"]}`, "want type number"},
		{`{"name":"x","size":1,"tags":[]}`, "at least 1 items"},
		{`{"name":"x","size":1,"tags":[3]}`, "$.tags[0]: want type string"},
		{`{"name":"x","size":1,"tags":["t"],"extra":true}`, `unexpected property "extra"`},
		{`[]`, "want type object"},
		{`{"name":"x","size":1,"tags":["t"],"id":"abc","opts":{"a":1}}`, ""},
		{`{"name":"x","size":1,"tags":["t"],"id":"ABC"}`, "does not match"},
		{`{"name":"x","size":1,"tags":["t"],"opts":{}}`, "at least 1 properties"},
		{`{"name":"x","size":1,"tags":["t"],"nick":"a"}`, "at least 2 characters"},
		{`{"name":"x","size":1,"tags":["t"],"maybe":null}`, ""},
		{`{"name":"x","size":1,"tags":["t"],"maybe":"s"}`, ""},
		{`{"name":"x","size":1,"tags":["t"],"maybe":3}`, "none of the anyOf"},
	}
	for _, tt := range tests {
		err := s.Validate([]byte(tt.doc))
		if tt.want == "" && err != nil {
			t.Errorf("%s: unexpected error %v", tt.doc, err)
		}
		if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%s: error %v, want %q", tt.doc, err, tt.want)
		}
	}
}

func TestLoadRejectsUnsupportedKeywords(t *testing.T) {
	for _, doc := range []string{
		`{"type":"string","maxLength":3}`,
		`{"type":"string","pattern":"("}`,
		`{"properties":{"a":{"maxLength":3}}}`,
		`{"additionalProperties":{"type":"string"}}`,
		`{"oneOf":[]}`,
	} {
		if _, err := Load([]byte(doc)); err == nil {
			t.Errorf("accepted %s", doc)
		}
	}
}
