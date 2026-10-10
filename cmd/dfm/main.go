// Command dfm checks a design against a manufacturing capability.
//
//	dfm check part.dxf --capability capability.json --material mild_steel --thickness 1.5mm [--json]
//
// Exit codes: 0 manufacturable, 1 not manufacturable, 2 invalid input or execution error.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
)

func main() {
	ctx := context.Background()
	err := Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	var exit ExitCode
	switch {
	case err == nil:
	case errors.As(err, &exit):
		os.Exit(int(exit))
	default:
		fmt.Fprintf(os.Stderr, "dfm: %s\n", err)
		os.Exit(int(ExitError))
	}
}
