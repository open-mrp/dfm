package capabilities

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/request"
)

func TestPresets(t *testing.T) {
	seen := map[string]bool{}
	var brakeLimits map[string]any
	for _, d := range processDirs {
		var files_ []string
		for _, p := range ListFor(d.process) {
			if seen[p.ID] {
				t.Errorf("%s: id used twice", p.ID)
			}
			seen[p.ID] = true
			files_ = append(files_, path.Join(d.dir, p.ID+".json"))
			data, ok := Get(p.ID)
			if !ok {
				t.Fatalf("%s: no file", p.ID)
			}
			c, aerr := request.DecodeCapability(data)
			if aerr == nil {
				aerr = capability.Validate(c)
			}
			if aerr != nil {
				t.Errorf("%s: %v", p.ID, aerr)
			}
			if c.Process != d.process {
				t.Errorf("%s: process %s in the %s list", p.ID, c.Process, d.process)
			}
			if p.Version < 1 || p.Title == "" || len(p.Sources) == 0 || p.Notes == "" {
				t.Errorf("%s: needs a version, a title, sources and notes", p.ID)
			}
			switch d.process {
			case constants.ProcessLaserCutting:
				// Feature limits are the corpus capability's, so the corpus measures what the preset checks.
				base, err := os.ReadFile(filepath.Join("..", p.LimitsFrom))
				if err != nil {
					t.Fatalf("%s: %v", p.ID, err)
				}
				if a, b := block(t, data, "laser_cutting", "usable_area", "kerf_width"), block(t, base, "laser_cutting", "usable_area", "kerf_width"); !reflect.DeepEqual(a, b) {
					t.Errorf("%s: limits differ from %s", p.ID, p.LimitsFrom)
				}
			case constants.ProcessBending:
				// Every press brake shares one set of bending limits.
				l := block(t, data, "bending", "max_bend_length", "max_force")
				if brakeLimits == nil {
					brakeLimits = l
				} else if !reflect.DeepEqual(l, brakeLimits) {
					t.Errorf("%s: bending limits differ from the other press brakes'", p.ID)
				}
			}
		}
		// Every preset file of the process is in its index.
		matches, _ := fs.Glob(files, path.Join(d.dir, "*-*.json"))
		slices.Sort(files_)
		if !slices.Equal(matches, files_) {
			t.Errorf("%s: preset files %v, index %v", d.process, matches, files_)
		}
	}
	if len(ListFor(constants.ProcessBending)) == 0 || len(List()) != len(seen) {
		t.Errorf("lists: %d presets, %d bending", len(List()), len(ListFor(constants.ProcessBending)))
	}
}

// block returns a capability's process block without what a machine sets.
func block(t *testing.T, data []byte, name string, machine ...string) map[string]any {
	t.Helper()
	var c map[string]any
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	b := c[name].(map[string]any)
	for _, k := range machine {
		delete(b, k)
	}
	return b
}
