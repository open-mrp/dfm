package dfm_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/open-mrp/dfm"
)

// resultsRecord is fixtures/results.json: the engine version and a digest of every committed expected result produced by it.
type resultsRecord struct {
	EngineVersion string `json:"engine_version"`
	SHA256        string `json:"sha256"`
}

const resultsRecordPath = "fixtures/results.json"

// TestEngineVersion fails when the expected results or dfm.EngineVersion no longer match fixtures/results.json. After a deliberate change in results, run
//
//	go test . -update
//
// which rewrites the expected results and this record, and title the PR feat or fix: the digest changing is what the pr-title workflow checks, so the next release gives the new results a new version. Release-please sets EngineVersion and the record's engine_version together; neither is edited by hand. It lives in its own file so it runs after the fixture tests have rewritten their results.
func TestEngineVersion(t *testing.T) {
	var paths []string
	for _, pattern := range []string{"fixtures/*/*.expected.json"} {
		m, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, m...)
	}
	slices.Sort(paths)
	h := sha256.New()
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(filepath.ToSlash(p) + "\n"))
		h.Write(data)
	}
	got := resultsRecord{EngineVersion: dfm.EngineVersion, SHA256: hex.EncodeToString(h.Sum(nil))}

	var want resultsRecord
	data, err := os.ReadFile(resultsRecordPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		t.Fatal(err)
	default:
		if err := json.Unmarshal(data, &want); err != nil {
			t.Fatal(err)
		}
	}
	if got == want {
		return
	}
	if !*update {
		t.Fatalf("%s is out of date (engine %s); run go test . -update", resultsRecordPath, got.EngineVersion)
	}
	out, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultsRecordPath, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
