package field

import (
	"encoding/json"
	"testing"
)

// TestOptional checks that a field reads as set only when the document gives it a value: absent and null both leave it unset, and it writes back as null when unset.
func TestOptional(t *testing.T) {
	type doc struct {
		A Optional[int]    `json:"a,omitzero"`
		B Optional[string] `json:"b,omitzero"`
		C Optional[int]    `json:"c,omitzero"`
	}
	var d doc
	if err := json.Unmarshal([]byte(`{"a": 0, "b": null}`), &d); err != nil {
		t.Fatal(err)
	}
	if v, ok := d.A.Value(); !ok || v != 0 {
		t.Errorf("a: %v %v, want set to 0", v, ok)
	}
	if _, ok := d.B.Value(); ok || d.B.Ptr() != nil || !d.B.IsZero() {
		t.Error("b: null read as set")
	}
	if _, ok := d.C.Value(); ok {
		t.Error("c: absent read as set")
	}
	if out, _ := json.Marshal(doc{A: Some(1)}); string(out) != `{"a":1}` {
		t.Errorf("marshal: %s", out)
	}
	if out, _ := json.Marshal(d.B); string(out) != "null" {
		t.Errorf("unset marshals as %s", out)
	}
}
