package dfm_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/request"
)

// externalSlow is how long one external file may take before it is recorded as slow.
const externalSlow = 20 * time.Second

// externalDir holds the outside datasets and test suites, part of the private corpus (docs/private-corpus.md): a manifest, a record of outcomes per source and the sources' files under opaque IDs.
const externalDir = "fixtures/private/external"

// externalSourceEntry is what the tests need of a source in the manifest.
type externalSourceEntry struct{ ID, Kind string }

// externalSources reads the manifest of the outside data, or skips the test when it is not fetched.
func externalSources(t *testing.T) []externalSourceEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(externalDir, "manifest.json"))
	if err != nil {
		t.Skip("outside data not fetched; scripts/private-corpus fetch-external")
	}
	var m struct{ Sources []externalSourceEntry }
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m.Sources
}

// externalSource is the ID of the source of a kind.
func externalSource(t *testing.T, kind string) string {
	t.Helper()
	for _, s := range externalSources(t) {
		if s.Kind == kind {
			return s.ID
		}
	}
	t.Fatalf("no source of kind %q in the outside data's manifest", kind)
	return ""
}

// TestExternalSuites checks every design file of the outside test suites (the sources of kind "formats") through Check, and compares each file's outcome with the suite's record under external/records: a verdict, or an error code. A panic (internal_error) or a slow file always fails. Suites not fetched are skipped.
func TestExternalSuites(t *testing.T) {
	c := planCapability(t, "examples/laser.json")
	for _, s := range externalSources(t) {
		if s.Kind != "formats" {
			continue
		}
		t.Run(s.ID, func(t *testing.T) {
			dir := filepath.Join(externalDir, s.ID)
			if _, err := os.Stat(filepath.Join(dir, ".fetched")); err != nil {
				t.Skip("not fetched; scripts/private-corpus fetch-external " + s.ID)
			}
			var files []string
			filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() && externalFormat(p) != "" {
					files = append(files, p)
				}
				return nil
			})
			slices.Sort(files)
			got := make(map[string]string, len(files))
			var mu sync.Mutex
			var wg sync.WaitGroup
			work := make(chan string)
			for range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for p := range work {
						o := externalOutcome(p, c)
						rel, _ := filepath.Rel(dir, p)
						mu.Lock()
						got[filepath.ToSlash(rel)] = o
						mu.Unlock()
					}
				}()
			}
			for _, p := range files {
				work <- p
			}
			close(work)
			wg.Wait()
			counts := map[string]int{}
			for f, o := range got {
				counts[o]++
				if strings.HasPrefix(o, "error internal_error") || strings.HasPrefix(o, "slow") {
					t.Errorf("%s: %s", f, o)
				}
			}
			t.Logf("%d files: %v", len(files), counts)
			compareRecord(t, filepath.Join(externalDir, "records", s.ID+".json"), got)
		})
	}
}

// externalFormat is the design format of a file by its extension, or "".
func externalFormat(p string) constants.DesignFormat {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".dxf":
		return constants.DesignFormatDXF
	case ".svg":
		return constants.DesignFormatSVG
	case ".step", ".stp":
		return constants.DesignFormatSTEP
	case ".dwg":
		return "dwg"
	}
	return ""
}

// externalOutcome checks one file: "manufacturable", "needs_review", "not_manufacturable", "error <code>", or "slow <duration>". A file without units is read in millimeters.
func externalOutcome(p string, c capability.Capability) string {
	data, err := os.ReadFile(p)
	if err != nil {
		return "error unreadable"
	}
	stock := capability.Stock{MaterialCode: "mild_steel", Thickness: 1_500_000}
	req := dfm.CheckRequest{Design: design.Design{Format: externalFormat(p), Data: data}, Capability: c, Stock: stock}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 4*externalSlow)
	defer cancel()
	res, err := dfm.Check(ctx, req)
	var e *apierror.Error
	if errors.As(err, &e) && e.Code == constants.ErrorCodeDesignUnitsUnknown {
		req.Design.Units = constants.LengthUnitMillimeter
		res, err = dfm.Check(ctx, req)
	}
	if d := time.Since(start); d > externalSlow {
		return fmt.Sprintf("slow %s", d.Round(time.Second))
	}
	switch {
	case errors.As(err, &e):
		if e.Code == constants.ErrorCodeInternalError {
			return "error internal_error: " + e.Message
		}
		return "error " + string(e.Code)
	case err != nil:
		return "error " + err.Error()
	}
	return string(res.Verdict)
}

// compareRecord compares a record of outcomes with its file in the private corpus, or rewrites it with -update.
func compareRecord(t *testing.T, path string, got any) {
	t.Helper()
	out, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, '\n')
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing %s (run with -update): %v", path, err)
	}
	if string(want) != string(out) {
		t.Errorf("%s changed; review the difference and run with -update", path)
	}
}

var _ = request.Decode
