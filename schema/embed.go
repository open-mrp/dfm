// Package schema holds the JSON Schemas of the public JSON contracts. They are generated from the request and resource types (go run ./schema/gen) and embedded so they can be served without reading from disk.
package schema

import (
	"bytes"
	"embed"
	"encoding/json"
	"path/filepath"
	"reflect"

	"github.com/open-mrp/dfm/internal/schemagen"
	"github.com/open-mrp/dfm/request"
	"github.com/open-mrp/dfm/resource"
)

//go:embed *.schema.json
var files embed.FS

// Get returns the schema with a public name such as "capability".
func Get(name string) ([]byte, bool) {
	for _, d := range documents {
		if d.name == name {
			data, err := files.ReadFile(name + ".schema.json")
			return data, err == nil
		}
	}
	return nil, false
}

// Names lists the public schema names.
func Names() []string {
	out := make([]string, len(documents))
	for i, d := range documents {
		out[i] = d.name
	}
	return out
}

const baseID = "https://github.com/open-mrp/dfm/schema/"

var documents = []struct {
	name, title string
	root        reflect.Type
	mode        schemagen.Mode
}{
	{"check-request", "DFM check request", reflect.TypeFor[request.CheckRequest](), schemagen.Request},
	{"capability", "DFM capability document", reflect.TypeFor[request.CapabilityInput](), schemagen.Request},
	{"check", "DFM check result", reflect.TypeFor[resource.Check](), schemagen.Response},
	{"error", "DFM error response", reflect.TypeFor[resource.ErrorResponse](), schemagen.Response},
	{"finding-codes", "DFM finding codes", reflect.TypeFor[resource.List[resource.FindingCodeInfo]](), schemagen.Response},
	{"plan-request", "DFM plan request", reflect.TypeFor[request.PlanRequest](), schemagen.Request},
	{"plan", "DFM plan result", reflect.TypeFor[resource.Plan](), schemagen.Response},
	{"feedback-expectation", "DFM feedback fixture expectation", reflect.TypeFor[request.FeedbackExpectation](), schemagen.Request},
}

// Build generates every schema document from the Go types, reading doc comments from the source tree rooted at repoRoot.
func Build(repoRoot string) (map[string][]byte, error) {
	docs, err := schemagen.LoadDocs(
		filepath.Join(repoRoot, "request"),
		filepath.Join(repoRoot, "resource"),
		filepath.Join(repoRoot, "apierror"),
	)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, d := range documents {
		doc := schemagen.Generate(d.root, d.mode, docs, baseID+d.name+".schema.json", d.title)
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(doc); err != nil {
			return nil, err
		}
		out[d.name+".schema.json"] = buf.Bytes()
	}
	return out, nil
}
