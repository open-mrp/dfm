// Package capabilities holds reference capabilities: presets for the machines most likely used in production, one list per process. Laser cutters (laser_cutting/): generic fiber classes of 2 to 12 kW, named TRUMPF, AMADA and Mazak machines, CO2 lasers from desktop to 4 x 8 ft. Press brakes (bending/): TRUMPF, AMADA and Bystronic machines from 36 to 150 t. A manufacturer may have any combination; grouping them by company is a separate step. They give a sense of whether a design can be made on common machines, and are never applied on their own: a caller chooses them and passes them as capabilities, like any other.
//
// Each laser preset's feature limits are those of the corpus capability it names (limits_from), so the corpus measures what the presets check; every press brake shares one set of bending limits.
package capabilities

import (
	"embed"
	"encoding/json"
	"path"

	"github.com/open-mrp/dfm/constants"
)

//go:embed laser_cutting bending
var files embed.FS

// processDirs lists each process's preset directory, in list order.
var processDirs = []struct {
	process constants.Process
	dir     string
}{{constants.ProcessLaserCutting, "laser_cutting"}, {constants.ProcessBending, "bending"}}

// Preset describes one reference capability.
type Preset struct {
	ID         string            `json:"id"`
	Process    constants.Process `json:"process"`
	Version    int               `json:"version"`
	Title      string            `json:"title"`
	LimitsFrom string            `json:"limits_from,omitempty"`
	Sources    []string          `json:"sources"`
	// Notes say how strong the evidence is.
	Notes string `json:"notes"`
}

// presets is each process's index, read once from the embedded files; the embedded data is tested, so a malformed index is a build problem, not a runtime one.
var presets = func() map[constants.Process][]Preset {
	out := map[constants.Process][]Preset{}
	for _, d := range processDirs {
		data, err := files.ReadFile(path.Join(d.dir, "index.json"))
		if err != nil {
			panic(err)
		}
		var idx struct {
			Presets []Preset `json:"presets"`
		}
		if err := json.Unmarshal(data, &idx); err != nil {
			panic(err)
		}
		for i := range idx.Presets {
			idx.Presets[i].Process = d.process
		}
		out[d.process] = idx.Presets
	}
	return out
}()

// List returns every preset: laser cutters, then press brakes.
func List() []Preset {
	var out []Preset
	for _, d := range processDirs {
		out = append(out, ListFor(d.process)...)
	}
	return out
}

// ListFor returns the presets of one process, in index order. The slice is shared: callers must not change it.
func ListFor(process constants.Process) []Preset {
	return presets[process]
}

// Get returns the capability JSON document of preset id.
func Get(id string) ([]byte, bool) {
	for _, d := range processDirs {
		for _, p := range presets[d.process] {
			if p.ID == id {
				data, err := files.ReadFile(path.Join(d.dir, id+".json"))
				return data, err == nil
			}
		}
	}
	return nil, false
}
