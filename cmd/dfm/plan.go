package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/request"
	"github.com/open-mrp/dfm/resource"
)

// capabilityList collects repeated --capability flags.
type capabilityList []string

func (l *capabilityList) String() string     { return strings.Join(*l, ",") }
func (l *capabilityList) Set(v string) error { *l = append(*l, v); return nil }

// runPlan works out the operations that make a design and checks each against the capability given for its process.
func runPlan(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var f checkFlags
	var caps capabilityList
	fs.Var(&caps, "capability", "")
	fs.StringVar(&f.material, "material", "", "")
	fs.StringVar(&f.thickness, "thickness", "", "")
	fs.StringVar(&f.units, "units", "", "")
	fs.StringVar(&f.holeOperations, "hole-operations", "", "")
	fs.StringVar(&f.bends, "bends", "", "")
	fs.StringVar(&f.adjustments, "adjustments", "", "")
	fs.BoolVar(&f.json, "json", false, "")
	positional, err := parseInterleaved(fs, args)
	if err != nil || len(positional) != 1 || len(caps) == 0 || f.material == "" || f.thickness == "" {
		return fail(stdout, stderr, f.json, apierror.ParameterInvalid("", "expected one design file plus --capability (one or more), --material and --thickness"))
	}
	f.capability = caps[0]
	first, err := checkRequest(positional[0], f)
	if err != nil {
		return fail(stdout, stderr, f.json, dfm.AsError(err))
	}
	req := dfm.PlanRequest{Design: first.Design, Stock: first.Stock, Capabilities: []capability.Capability{first.Capability}, HoleOperations: first.HoleOperations, Bends: first.Bends}
	for _, arg := range caps[1:] {
		data, aerr := readCapability(arg)
		if aerr != nil {
			return fail(stdout, stderr, f.json, aerr)
		}
		c, aerr := request.DecodeCapability(data)
		if aerr != nil {
			return fail(stdout, stderr, f.json, aerr)
		}
		req.Capabilities = append(req.Capabilities, c)
	}
	res, err := dfm.CheckPlan(ctx, req)
	if err != nil {
		return fail(stdout, stderr, f.json, dfm.AsError(err))
	}
	plan := resource.PresentPlan(res)
	if f.json {
		writeJSON(stdout, plan)
	} else {
		verdict := statusOf(plan.Verdict)
		fmt.Fprintf(stdout, "%s %s: %d operation%s\n", verdict, positional[0], len(plan.Operations.Data), map[bool]string{true: "", false: "s"}[len(plan.Operations.Data) == 1])
		for _, op := range plan.Operations.Data {
			fmt.Fprintf(stdout, "\n%d. %s (%s)\n", op.Sequence, op.Process, op.Reason)
			if op.Result == nil {
				fmt.Fprintf(stdout, "   not checked: no %s capability given\n", op.Process)
				continue
			}
			writeHuman(stdout, positional[0], op.Result)
		}
	}
	return exitFor(res.Verdict)
}
