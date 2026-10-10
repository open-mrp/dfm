package schema

import (
	"bytes"
	"os"
	"testing"

	"github.com/open-mrp/dfm/internal/schemacheck"
	"github.com/open-mrp/dfm/request"
)

// TestCommittedSchemasUpToDate fails if a schema file differs from what the Go types generate; run go run ./schema/gen after changing them.
func TestCommittedSchemasUpToDate(t *testing.T) {
	built, err := Build("..")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range built {
		got, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("schema/%s is out of date; run go run ./schema/gen", name)
		}
	}
}

func TestSchemasLoadAndExamplesValidate(t *testing.T) {
	for _, name := range Names() {
		data, ok := Get(name)
		if !ok {
			t.Fatalf("schema %s not embedded", name)
		}
		if _, err := schemacheck.Load(data); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for file, name := range map[string]string{"../examples/laser.json": "capability", "../examples/check-request.json": "check-request"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := Get(name)
		s, _ := schemacheck.Load(raw)
		if err := s.Validate(data); err != nil {
			t.Errorf("%s does not validate against %s: %v", file, name, err)
		}
	}
	if !bytes.Contains(mustGet(t, "capability"), []byte(`"const": "`+request.CapabilitySchemaVersion+`"`)) {
		t.Error("capability schema does not pin the current schema_version")
	}
}

func mustGet(t *testing.T, name string) []byte {
	t.Helper()
	data, ok := Get(name)
	if !ok {
		t.Fatalf("no schema %s", name)
	}
	return data
}
