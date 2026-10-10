// Command gen regenerates the JSON Schemas in schema/ from the Go types.
//
//	go run ./schema/gen    # from the repository root
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/open-mrp/dfm/schema"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("run from the repository root: go run ./schema/gen")
	}
	files, err := schema.Build(".")
	if err != nil {
		return err
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join("schema", name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
