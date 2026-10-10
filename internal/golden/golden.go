// Package golden keeps the engine version out of the fixture *.expected.json golden files, so they change only when findings change.
//
// Golden files hold Version where a response holds dfm.EngineVersion. fixtures/results.json still records a hash of every golden file with the engine version, so a change in results still needs a version bump.
package golden

import "bytes"

// Version stands in for the engine version in golden files.
const Version = "dev"

// Mask replaces engine version v with Version in an indented response.
func Mask(b []byte, v string) []byte {
	return bytes.ReplaceAll(b, field(v), field(Version))
}

// Unmask replaces Version with engine version v in a golden file, for comparing it with a real response.
func Unmask(b []byte, v string) []byte {
	return bytes.ReplaceAll(b, field(Version), field(v))
}

func field(v string) []byte {
	return []byte(`"engine_version": "` + v + `"`)
}
