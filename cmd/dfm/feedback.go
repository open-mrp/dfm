package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-mrp/dfm"
	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/feedback"
	"github.com/open-mrp/dfm/request"
	"github.com/open-mrp/dfm/resource"
)

// storedCheck reads a stored check: the design file, its check request JSON and its result JSON.
func storedCheck(design, requestPath, resultPath string) (dfm.CheckRequest, []byte, resource.Check, *apierror.Error) {
	data, err := os.ReadFile(design)
	if err != nil {
		return dfm.CheckRequest{}, nil, resource.Check{}, apierror.ParameterInvalid("design", "reading design: %v", err)
	}
	reqJSON, err := os.ReadFile(requestPath)
	if err != nil {
		return dfm.CheckRequest{}, nil, resource.Check{}, apierror.ParameterInvalid("request", "reading request: %v", err)
	}
	req, aerr := dfm.RequestFromJSON(data, reqJSON)
	if aerr != nil {
		return dfm.CheckRequest{}, nil, resource.Check{}, aerr
	}
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		return dfm.CheckRequest{}, nil, resource.Check{}, apierror.ParameterInvalid("result", "reading result: %v", err)
	}
	var stored resource.Check
	if err := json.Unmarshal(raw, &stored); err != nil || stored.Object != "dfm_check" {
		return dfm.CheckRequest{}, nil, resource.Check{}, apierror.ParameterInvalid("result", "the stored result is not a dfm_check: %v", err)
	}
	return req, reqJSON, stored, nil
}

// runReplay re-runs a stored check on this engine and reports what changed. It exits 0 when nothing did and 1 when something did, like diff.
func runReplay(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	reqPath := fs.String("request", "", "")
	resPath := fs.String("result", "", "")
	asJSON := fs.Bool("json", false, "")
	positional, err := parseInterleaved(fs, args)
	if err != nil || len(positional) != 1 || *reqPath == "" || *resPath == "" {
		return fail(stdout, stderr, *asJSON, apierror.ParameterInvalid("", "expected one design file plus --request and --result"))
	}
	req, _, stored, aerr := storedCheck(positional[0], *reqPath, *resPath)
	if aerr != nil {
		return fail(stdout, stderr, *asJSON, aerr)
	}
	diff, err := feedback.Replay(ctx, req, stored)
	if err != nil {
		return fail(stdout, stderr, *asJSON, dfm.AsError(err))
	}
	if *asJSON {
		writeJSON(stdout, diff)
	} else {
		fmt.Fprintf(stdout, "engine %s -> %s; verdict %s -> %s; %d unchanged, %d changed\n",
			diff.StoredEngineVersion, diff.EngineVersion, diff.StoredVerdict, diff.Verdict, diff.Unchanged, len(diff.Changes))
		for _, c := range diff.Changes {
			o := c.Now
			if o == nil {
				o = c.Stored
			}
			fmt.Fprintf(stdout, "  %-7s %s %s %s %v\n", c.Change, o.Severity, o.Code, o.ID, o.EntityIDs)
		}
	}
	if diff.Changed() {
		return ExitNotManufacturable // exit 1: the result changed
	}
	return nil
}

// runFeedbackFixture turns what a report said about a stored check into a feedback fixture: a directory holding the design, its request and an expectation.
func runFeedbackFixture(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("feedback-fixture", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	reqPath := fs.String("request", "", "")
	resPath := fs.String("result", "", "")
	id := fs.String("id", "", "")
	note := fs.String("note", "", "")
	name := fs.String("name", "", "")
	dir := fs.String("dir", filepath.Join("fixtures", "feedback"), "")
	var absent, actual, present repeated
	fs.Var(&absent, "absent", "")
	fs.Var(&actual, "actual", "")
	fs.Var(&present, "present", "")
	positional, err := parseInterleaved(fs, args)
	if err != nil || len(positional) != 1 || *reqPath == "" || *resPath == "" || *name == "" || *note == "" || len(absent)+len(actual)+len(present) == 0 {
		return fail(stdout, stderr, false, apierror.ParameterInvalid("", "expected one design file plus --request, --result, --name, --note and at least one of --absent, --actual and --present"))
	}
	_, reqJSON, stored, aerr := storedCheck(positional[0], *reqPath, *resPath)
	if aerr != nil {
		return fail(stdout, stderr, false, aerr)
	}
	var corrected []feedback.Correction
	for _, a := range actual {
		occID, value, ok := strings.Cut(a, "=")
		if !ok || occID == "" || value == "" {
			return fail(stdout, stderr, false, apierror.ParameterInvalid("actual", "--actual takes <occurrence id>=<value>, got %q", a))
		}
		corrected = append(corrected, feedback.Correction{ID: occID, Value: value})
	}
	var expected []request.ExpectedFinding
	for _, p := range present {
		var f request.ExpectedFinding
		if err := request.Decode([]byte(p), &f, "present"); err != nil {
			return fail(stdout, stderr, false, err)
		}
		expected = append(expected, f)
	}
	ref := *id
	if ref == "" {
		ref = *name
	}
	exp, err := feedback.Expectation(ref, *note, absent, corrected, expected, stored)
	if err != nil {
		return fail(stdout, stderr, false, apierror.ParameterInvalid("absent", "%v", err))
	}
	out := filepath.Join(*dir, *name)
	if _, err := os.Stat(out); err == nil {
		return fail(stdout, stderr, false, apierror.ParameterInvalid("name", "%s already exists", out))
	}
	design, _ := os.ReadFile(positional[0])
	var expJSON bytes.Buffer
	enc := json.NewEncoder(&expJSON)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(exp)
	if err := errors.Join(
		os.MkdirAll(out, 0o755),
		os.WriteFile(filepath.Join(out, "design"+filepath.Ext(positional[0])), design, 0o644),
		os.WriteFile(filepath.Join(out, "request.json"), reqJSON, 0o644),
		os.WriteFile(filepath.Join(out, "expect.json"), expJSON.Bytes(), 0o644),
	); err != nil {
		return fail(stdout, stderr, false, apierror.ParameterInvalid("dir", "writing the fixture: %v", err))
	}
	fmt.Fprintln(stdout, out)
	return nil
}

// repeated is a flag that may be given more than once.
type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ", ") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

// parseInterleaved parses flags given before and after the positional arguments; flag.ErrHelp comes back as it is, for the caller to print the usage.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}
