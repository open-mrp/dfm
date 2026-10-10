package rules

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/open-mrp/dfm/constants"
)

// TestChecksDoc holds docs/checks.md to the catalog: each process's table lists exactly the catalog's codes, with the catalog's severities (the table's own grouping is kept).
func TestChecksDoc(t *testing.T) {
	data, err := os.ReadFile("../docs/checks.md")
	if err != nil {
		t.Fatal(err)
	}
	sections := map[string][]constants.Process{
		"Laser cutting":   {constants.ProcessLaserCutting},
		"Bending":         {constants.ProcessBending},
		"Hole operations": {constants.ProcessCountersinking, constants.ProcessTapping, constants.ProcessHardwareInsertion},
	}
	// A row names one code, or several on one row (`a` / `b`); severities are separated by "/" or ",".
	row := regexp.MustCompile("^\\| ((?:`[a-z_]+`(?: / )?)+) *\\| ([a-z ,/]+?) *\\|")
	code := regexp.MustCompile("`([a-z_]+)`")
	var section string
	rows := map[string][][2]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimPrefix(line, "## ")
			continue
		}
		if m := row.FindStringSubmatch(line); m != nil {
			var sev []string
			for _, s := range strings.FieldsFunc(m[2], func(r rune) bool { return r == '/' || r == ',' }) {
				sev = append(sev, strings.TrimSpace(s))
			}
			for _, c := range code.FindAllStringSubmatch(m[1], -1) {
				rows[section] = append(rows[section], [2]string{c[1], strings.Join(sev, " / ")})
			}
		}
	}
	for section, processes := range sections {
		for _, process := range processes {
			var want [][2]string
			for _, c := range Catalog(process) {
				var sev []string
				for _, s := range c.Severities {
					sev = append(sev, string(s))
				}
				want = append(want, [2]string{string(c.Code), strings.Join(sev, " / ")})
			}
			got := map[string]string{}
			for _, r := range rows[section] {
				got[r[0]] = r[1]
			}
			for _, w := range want {
				if sev, ok := got[w[0]]; !ok {
					t.Errorf("%s: %s is not in the table", section, w[0])
				} else if sev != w[1] {
					t.Errorf("%s: %s is %q, catalog says %q", section, w[0], sev, w[1])
				}
				delete(got, w[0])
			}
			if len(processes) == 1 {
				for code := range got {
					t.Errorf("%s: %s is in the table but not the catalog", section, code)
				}
			}
		}
	}
}
