package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capabilities"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/measure"
	"github.com/open-mrp/dfm/request"
	"github.com/open-mrp/dfm/resource"
)

// ExitCode is returned by Run when the process should exit non-zero after output has already been written.
type ExitCode int

const (
	ExitNotManufacturable ExitCode = 1
	ExitError             ExitCode = 2
	ExitNeedsReview       ExitCode = 3
)

func (e ExitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

const usage = `Usage:
  dfm check <design.dxf|design.svg|design.step> --capability <capability.json> --material <code> --thickness <value><unit>
            [--units um|mm|cm|m|in|ft] [--hole-operations <assignments.json>]
            [--bends <bends.json>] [--adjustments <adjustments.json>] [--json]
  dfm plan <design> --capability <a> [--capability <b> ...] --material <code> --thickness <value><unit>
            [--units ...] [--hole-operations ...] [--bends ...] [--adjustments ...] [--json]
                         work out the operations (laser cutting, then bending when the
                         drawing has bend lines) and check each against its capability
  dfm fix <design> <the same options as check> --adjustments <adjustments.json> --output <file>
                         write the design with accepted fixes in it
  dfm preview <design> <the same options as check> --output <file.svg>
                         draw the design as read, with its findings marked
  dfm codes [--json]     list every finding code with its meaning and how to fix it
  dfm capabilities [--json]
                         list the reference capabilities (machine presets)
  dfm replay <design> --request <request.json> --result <result.json> [--json]
                         re-run a stored check on this engine and list what changed
                         (exit 0 unchanged, 1 changed)
  dfm feedback-fixture <design> --request <request.json> --result <result.json>
            --name <name> --note <why> [--id <report id>] [--dir fixtures/feedback]
            [--absent <occurrence id>] [--actual <occurrence id>=<value>] [--present <finding JSON>]
                         turn what a report said about a stored result into a feedback fixture:
                         an occurrence that must go (a false positive), one whose measurement is
                         wrong, or a finding that is missing ({"code": ..., "entity_ids": [...]})
  dfm version            print the engine version

  --capability is a capability document, or preset:<id> for a reference
    capability (see dfm capabilities).
  --material is a material code from the capability document.
  --thickness is the stock thickness with its unit, e.g. 1.5mm or 0.0625in.
  --units overrides the drawing units declared in the design file; it is
    required for files that declare none.
  --hole-operations assigns taps, countersinks and press-in fasteners from the
    capability's hole_operations to holes: a JSON array like a check
    request's hole_operations.
  --bends gives bend lines their angle, direction and inside radius for a
    bending check: a JSON array like a plan request's bends. A bend line not
    listed is bent 90 degrees up at the brake's inside radius.
  --adjustments accepts fixes from a previous result: a JSON array like a
    check request's design.adjustments (each occurrence's fix).

Exit codes:
  0  manufacturable
  1  not manufacturable
  2  invalid input or execution error
  3  needs review: no errors, but findings a person must judge
`

// Run executes the command line in args.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stderr, usage)
		if len(args) == 0 {
			return ExitError
		}
		return nil
	}
	switch args[0] {
	case "check":
		return runCheck(ctx, args[1:], stdout, stderr, "check")
	case "fix", "preview":
		return runCheck(ctx, args[1:], stdout, stderr, args[0])
	case "codes":
		return runCodes(args[1:], stdout, stderr)
	case "capabilities":
		return runCapabilities(args[1:], stdout, stderr)
	case "plan":
		return runPlan(ctx, args[1:], stdout, stderr)
	case "replay":
		return runReplay(ctx, args[1:], stdout, stderr)
	case "feedback-fixture":
		return runFeedbackFixture(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, dfm.EngineVersion)
		return nil
	}
	fmt.Fprintf(stderr, "dfm: unknown command %q\n\n%s", args[0], usage)
	return ExitError
}

func runCodes(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("codes", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		fmt.Fprint(stderr, usage)
		return ExitError
	}
	cat := resource.PresentCodes()
	if *asJSON {
		writeJSON(stdout, cat)
		return nil
	}
	for _, c := range cat.Data {
		sev := make([]string, len(c.Severities))
		for i, s := range c.Severities {
			sev[i] = string(s)
		}
		fmt.Fprintf(stdout, "%s (%s; %s)\n  %s\n  Fix: %s\n", c.Code, strings.Join(sev, "/"), c.Scope, c.Description, c.Remediation)
		if c.Limit != nil {
			fmt.Fprintf(stdout, "  Limit: %s\n", *c.Limit)
		}
	}
	return nil
}

// readCapability reads a capability document from a file, or a reference capability named preset:<id>.
func readCapability(arg string) ([]byte, *apierror.Error) {
	if id, ok := strings.CutPrefix(arg, "preset:"); ok {
		data, found := capabilities.Get(id)
		if !found {
			return nil, apierror.ParameterInvalid("capability", "no reference capability %q (see dfm capabilities)", id)
		}
		return data, nil
	}
	data, err := os.ReadFile(arg)
	if err != nil {
		return nil, apierror.ParameterInvalid("capability", "reading capability: %v", err)
	}
	return data, nil
}

func runCapabilities(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("capabilities", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		fmt.Fprint(stderr, usage)
		return ExitError
	}
	if *asJSON {
		writeJSON(stdout, capabilities.List())
		return nil
	}
	for _, p := range capabilities.List() {
		fmt.Fprintf(stdout, "preset:%s (v%d)  %s: %s\n", p.ID, p.Version, p.Process, p.Title)
	}
	return nil
}

type checkFlags struct {
	capability, material, thickness, units string
	holeOperations, bends                  string
	adjustments                            string
	output                                 string
	json                                   bool
}

// runCheck runs dfm check, fix or preview: the same inputs; fix writes the corrected design file and preview an SVG of the design with its findings, instead of the result.
func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer, mode string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var f checkFlags
	fs.StringVar(&f.capability, "capability", "", "path to the capability JSON document")
	fs.StringVar(&f.material, "material", "", "material code from the capability")
	fs.StringVar(&f.thickness, "thickness", "", "stock thickness with unit, e.g. 1.5mm")
	fs.StringVar(&f.units, "units", "", "drawing units override")
	fs.StringVar(&f.holeOperations, "hole-operations", "", "path to a JSON array of hole operation assignments")
	fs.StringVar(&f.bends, "bends", "", "path to a JSON array of bend assignments")
	fs.StringVar(&f.adjustments, "adjustments", "", "path to a JSON array of accepted fixes")
	fs.BoolVar(&f.json, "json", false, "write only structured JSON to stdout")
	fs.StringVar(&f.output, "output", "", "where dfm fix writes the corrected file")

	positional, err := parseInterleaved(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stderr, usage)
		return nil
	}
	if err != nil {
		return fail(stdout, stderr, f.json || hasJSONFlag(args), apierror.ParameterInvalid("", "%v", err))
	}
	if len(positional) != 1 || f.capability == "" || f.material == "" || f.thickness == "" {
		return fail(stdout, stderr, f.json, apierror.ParameterInvalid("", "expected one design file plus --capability, --material and --thickness"))
	}

	req, err := checkRequest(positional[0], f)
	if err != nil {
		return fail(stdout, stderr, f.json, dfm.AsError(err))
	}
	if mode != "check" {
		if f.output == "" {
			return fail(stdout, stderr, f.json, apierror.ParameterInvalid("", "dfm %s needs --output", mode))
		}
		write := dfm.Repair
		if mode == "preview" {
			write = dfm.Preview
		}
		out, err := write(ctx, req)
		if err != nil {
			return fail(stdout, stderr, f.json, dfm.AsError(err))
		}
		if err := os.WriteFile(f.output, out, 0o644); err != nil {
			return fail(stdout, stderr, f.json, apierror.ParameterInvalid("", "writing %s: %v", f.output, err))
		}
		return nil
	}
	res, err := dfm.Check(ctx, req)
	if err != nil {
		return fail(stdout, stderr, f.json, dfm.AsError(err))
	}
	if f.json {
		writeJSON(stdout, resource.PresentCheck(res))
	} else {
		writeHuman(stdout, positional[0], resource.PresentCheck(res))
	}
	return exitFor(res.Verdict)
}

// checkRequest builds the check request from the command line.
func checkRequest(path string, f checkFlags) (dfm.CheckRequest, error) {
	capData, aerr := readCapability(f.capability)
	if aerr != nil {
		return dfm.CheckRequest{}, aerr
	}
	c, aerr := request.DecodeCapability(capData)
	if aerr != nil {
		return dfm.CheckRequest{}, aerr
	}
	thickness, aerr := parseQuantity(f.thickness, "stock.thickness")
	if aerr != nil {
		return dfm.CheckRequest{}, aerr
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return dfm.CheckRequest{}, apierror.ParameterInvalid("design", "reading design: %v", err)
	}
	format, aerr := formatFromPath(path)
	if aerr != nil {
		return dfm.CheckRequest{}, aerr
	}
	var holes []capability.HoleAssignment
	if f.holeOperations != "" {
		raw, err := os.ReadFile(f.holeOperations)
		if err != nil {
			return dfm.CheckRequest{}, apierror.ParameterInvalid("hole_operations", "reading hole operations: %v", err)
		}
		if holes, aerr = request.DecodeHoleAssignments(raw); aerr != nil {
			return dfm.CheckRequest{}, aerr
		}
	}
	var bends []capability.BendAssignment
	if f.bends != "" {
		raw, err := os.ReadFile(f.bends)
		if err != nil {
			return dfm.CheckRequest{}, apierror.ParameterInvalid("bends", "reading bends: %v", err)
		}
		if bends, aerr = request.DecodeBendAssignments(raw); aerr != nil {
			return dfm.CheckRequest{}, aerr
		}
	}
	var adj []design.Adjustment
	if f.adjustments != "" {
		raw, err := os.ReadFile(f.adjustments)
		if err != nil {
			return dfm.CheckRequest{}, apierror.ParameterInvalid("design.adjustments", "reading adjustments: %v", err)
		}
		if adj, aerr = request.DecodeAdjustments(raw); aerr != nil {
			return dfm.CheckRequest{}, aerr
		}
	}
	return dfm.CheckRequest{
		Design:         design.Design{Format: format, Data: data, Units: constants.LengthUnit(f.units), Adjustments: adj},
		Capability:     c,
		Stock:          capability.Stock{MaterialCode: f.material, Thickness: thickness},
		HoleOperations: holes,
		Bends:          bends,
	}, nil
}

var quantityText = regexp.MustCompile(`^(-?[0-9]+(?:\.[0-9]+)?)\s*([a-z]+)$`)

// parseQuantity parses "1.5mm" or "0.0625 in". A unit is always required.
func parseQuantity(s, param string) (measure.Length, *apierror.Error) {
	m := quantityText.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, apierror.ParameterInvalid(param, "%q must be a decimal value followed by a unit, e.g. 1.5mm", s)
	}
	l, err := measure.ParseLength(m[1], constants.LengthUnit(m[2]))
	if err != nil {
		return 0, apierror.ParameterInvalid(param, "%v", err)
	}
	return l, nil
}

func formatFromPath(path string) (constants.DesignFormat, *apierror.Error) {
	f := constants.DesignFormat(strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."))
	if f == "stp" {
		f = constants.DesignFormatSTEP
	}
	if !f.IsValid() {
		return "", apierror.New(constants.ErrorCodeDesignFormatUnsupported, "design.format", "cannot infer a supported design format from %q (supported: %s)", path, strings.Join(f.EnumValues(), ", "))
	}
	return f, nil
}

func fail(stdout, stderr io.Writer, asJSON bool, e *apierror.Error) error {
	if asJSON {
		writeJSON(stdout, resource.ErrorResponse{Error: e})
	} else {
		fmt.Fprintf(stderr, "dfm: %s\n", e)
		if e.Param == "" && e.Code == constants.ErrorCodeParameterInvalid {
			fmt.Fprint(stderr, "\n"+usage)
		}
	}
	return ExitError
}

// hasJSONFlag lets flag-parse errors still honor --json.
func hasJSONFlag(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" || a == "--json=true" || a == "-json=true" {
			return true
		}
	}
	return false
}

// statusOf is a verdict's word at the start of human output, for a check's, a plan's or an operation's verdict.
func statusOf[V ~string](v V) string {
	switch constants.Verdict(v) {
	case constants.VerdictManufacturable:
		return "PASS"
	case constants.VerdictNeedsReview:
		return "REVIEW"
	}
	return "FAIL"
}

// exitFor is the exit code of a verdict: 0 manufacturable, 3 needs review, 1 otherwise (not manufacturable, or a plan step not checked).
func exitFor[V ~string](v V) error {
	switch constants.Verdict(v) {
	case constants.VerdictManufacturable:
		return nil
	case constants.VerdictNeedsReview:
		return ExitNeedsReview
	}
	return ExitNotManufacturable
}

func writeJSON(w io.Writer, v any) {
	// Compact, for programs; pipe through a formatter to read it.
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeHuman(w io.Writer, path string, c *resource.Check) {
	status, verdict := statusOf(c.Verdict), strings.ReplaceAll(string(c.Verdict), "_", " ")
	reviews := ""
	if c.ReviewCount > 0 {
		reviews = fmt.Sprintf(", %d to review", c.ReviewCount)
	}
	fmt.Fprintf(w, "%s %s: %s (%d error%s%s, %d warning%s)\n", status, path, verdict, c.ErrorCount, plural(c.ErrorCount), reviews, c.WarningCount, plural(c.WarningCount))
	if parts := c.Parts.Data; c.PartCount > 1 {
		failed := 0
		for _, p := range parts {
			if p.Verdict != constants.VerdictManufacturable {
				failed += p.CopyCount
			}
		}
		distinct := ""
		if len(parts) < c.PartCount {
			distinct = fmt.Sprintf(" (%d distinct)", len(parts))
		}
		fmt.Fprintf(w, "%d parts%s, %d not manufacturable\n", c.PartCount, distinct, failed)
	}
	for _, p := range c.Parts.Data {
		if len(p.Issues.Data) == 0 {
			continue
		}
		state := strings.ReplaceAll(string(p.Verdict), "_", " ")
		outline := ""
		if p.Sheet.Outline != constants.OutlineClosed {
			outline = ", outline not closed"
		}
		copies := ""
		if p.CopyCount > 1 {
			numbers := make([]string, len(p.Instances.Data))
			for i, in := range p.Instances.Data {
				numbers[i] = strconv.Itoa(in.Number)
			}
			copies = fmt.Sprintf(" x%d (parts %s; locations are on part %d)", p.CopyCount, strings.Join(numbers, ", "), p.Number)
		}
		fmt.Fprintf(w, "Part %d%s (%s x %s%s): %s\n", p.Number, copies, display(p.Sheet.Width), display(p.Sheet.Height), outline, state)
		writeIssues(w, p.Issues.Data)
	}
	if issues := c.DrawingIssues.Data; len(issues) > 0 {
		fmt.Fprintln(w, "Drawing:")
		writeIssues(w, issues)
	}
	if skipped := c.SkippedChecks.Data; len(skipped) > 0 {
		codes := make([]string, len(skipped))
		for i, s := range skipped {
			codes[i] = string(s.Code)
		}
		fmt.Fprintf(w, "Not checked (limit not set in the capability): %s\n", strings.Join(codes, ", "))
	}
}

// writeIssues prints each issue once, then a line per occurrence.
func writeIssues(w io.Writer, issues []resource.Issue) {
	for _, is := range issues {
		fmt.Fprintf(w, "  %-7s %s: %s", is.Severity, is.Code, is.Message)
		if is.Required != nil {
			fmt.Fprintf(w, " (required %s", display(is.Required))
			if is.RequiredBasis != nil {
				fmt.Fprintf(w, " from %s", *is.RequiredBasis)
			}
			fmt.Fprint(w, ")")
		}
		fmt.Fprintln(w)
		for _, o := range is.Occurrences.Data {
			var parts []string
			if o.Actual != nil {
				parts = append(parts, display(o.Actual))
			}
			if o.Location != nil {
				parts = append(parts, fmt.Sprintf("at (%s, %s)", o.Location.X, o.Location.Y))
			}
			if ids := o.EntityIDs; len(ids) > 6 {
				parts = append(parts, fmt.Sprintf("[%s, and %d more]", strings.Join(ids[:5], ", "), len(ids)-5))
			} else if len(ids) > 0 {
				parts = append(parts, "["+strings.Join(ids, ", ")+"]")
			}
			fmt.Fprintf(w, "          - %s\n", strings.Join(parts, " "))
		}
		fmt.Fprintf(w, "          Fix: %s\n", is.Remediation)
		fixes, action := 0, constants.FixAction("")
		for _, o := range is.Occurrences.Data {
			if o.Fix != nil {
				fixes, action = fixes+1, o.Fix.Action
			}
		}
		if fixes == 1 {
			fmt.Fprintf(w, "          1 suggested fix to accept (%s): see --json, then pass it to --adjustments\n", action)
		} else if fixes > 1 {
			fmt.Fprintf(w, "          %d suggested fixes to accept (%s): see --json, then pass them to --adjustments\n", fixes, action)
		}
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
