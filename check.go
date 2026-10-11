// Package dfm checks whether a design can be manufactured with a given capability and stock.
//
// The pipeline is:
//
//	design file → design.Parse → geometry.Part → rules → CheckResult
//
// Check is the service layer: the library and the CLI (and the platform API, in its own repository) all call it. Wire formats live in packages request and resource.
package dfm

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/preview"
	"github.com/open-mrp/dfm/rules"
)

// CheckRequest is the input to Check.
type CheckRequest struct {
	Design     design.Design
	Capability capability.Capability
	Stock      capability.Stock
	// HoleOperations assigns operations from the capability's catalog to round holes of the design.
	HoleOperations []capability.HoleAssignment
	// Bends gives bend lines their angle, direction and inside radius, for a bending check (ignored by other processes).
	Bends []capability.BendAssignment
}

// Check validates the request, resolves the capability for the stock, parses and normalizes the design, and runs the process's rules. A design that fails rules is a result, not an error. Errors are *apierror.Error, or the context's error when ctx is done.
func Check(ctx context.Context, req CheckRequest) (res CheckResult, err error) {
	defer recoverAsInternal(&err)
	return check(ctx, req, "", nil)
}

// recoverAsInternal turns a panic into an internal error, so a design that trips a bug fails its request instead of the process serving it.
func recoverAsInternal(err *error) {
	if r := recover(); r != nil {
		*err = apierror.Internal(fmt.Errorf("engine panic: %v", r))
	}
}

// check is Check, or with step set, a plan's hole operation step: the laser-cutting capability's catalog checked for that process, with the press brake's bending profile (when one is given) bending the part.
func check(ctx context.Context, req CheckRequest, step constants.Process, bending *capability.BendingProfile) (CheckResult, error) {
	if err := capability.Validate(req.Capability); err != nil {
		return CheckResult{}, err
	}
	if err := capability.ValidateHoleAssignments(req.Capability, req.HoleOperations); err != nil {
		return CheckResult{}, err
	}
	if err := capability.ValidateBendAssignments(req.Bends); err != nil {
		return CheckResult{}, err
	}
	profile, err := capability.Resolve(req.Capability, req.Stock)
	if err != nil {
		return CheckResult{}, err
	}
	if step != "" {
		profile.Process, profile.Bending = step, bending
	}
	if e := validateDesign(req.Design); e != nil {
		return CheckResult{}, e
	}
	if err := ctx.Err(); err != nil {
		return CheckResult{}, err
	}
	d := readAs(req.Design, profile)
	part, modelBends, perr := readPart(d)
	if perr != nil {
		return CheckResult{}, designError(perr)
	}

	// A capability that requires one part per drawing checks the drawing whole, as one part; file_multiple_parts then fails it.
	mode := constants.MultiplePartsAccepted
	if profile.LaserCutting != nil {
		mode = profile.LaserCutting.MultipleParts
	}
	split := func() []*geometry.Part {
		if mode == constants.MultiplePartsRejected {
			return []*geometry.Part{part}
		}
		return part.Parts(mode == constants.MultiplePartsAcceptedInCutouts)
	}
	parts := split()
	if len(req.HoleOperations) > 0 {
		// Resizing holes keeps every loop, so the parts and their numbers stay the same.
		part = applyHoleOps(part, parts, req.HoleOperations, profile)
		parts = split()
	}
	if profile.Bending != nil {
		if err := applyBends(part, req.Bends, modelBends, profile); err != nil {
			return CheckResult{}, err
		}
		// The bend order is searched once: the collision rule reports from it and the result's bends say how each was formed.
		part.BendPlan = rules.PlanBends(part, profile)
	}
	res := CheckResult{Process: profile.Process, EngineVersion: EngineVersion, Verdict: constants.VerdictManufacturable, Findings: []rules.Finding{}, Skipped: rules.Skipped(profile)}
	for i, p := range parts {
		res.Parts = append(res.Parts, describePart(i+1, p))
		res.Parts[i].Holes = holeGroups(p, profile)
		if profile.Bending != nil {
			res.Parts[i].Bends = partBends(part, p)
		}
	}
	for _, r := range rules.ForProcess(profile.Process) {
		if err := ctx.Err(); err != nil {
			return CheckResult{}, err
		}
		if !rules.Enabled(r, profile) {
			continue
		}
		if rules.ChecksDrawing(r) {
			for _, f := range r.Check(part, profile) {
				f = rules.Locate(f, part)
				res.add(f, attribute(f, parts))
			}
			continue
		}
		for i, p := range parts {
			// An unclosed outline has no inside to measure; its open contour finding says what to fix first.
			if p.Unclosed {
				continue
			}
			for _, f := range r.Check(p, profile) {
				res.add(rules.Locate(f, p), []int{i + 1})
			}
		}
	}
	if err := keepSafeUnitsFix(ctx, req, &res); err != nil {
		return CheckResult{}, err
	}
	assignIDs(res.Findings)
	res.markCopies(parts)
	return res, nil
}

// readAs sets how the design is read from the resolved profile: the gap closing tolerance and what shapes are for.
func readAs(d design.Design, profile *capability.Profile) design.Design {
	if profile.LaserCutting != nil {
		d.ClosureTolerance = profile.LaserCutting.ContourClosureTolerance
		d.NearDuplicate = profile.LaserCutting.KerfWidth / 2
		d.Operations = design.Operations{ByColor: profile.LaserCutting.ColorOperations, Filled: profile.LaserCutting.FilledShapes, OutsidePage: profile.LaserCutting.OutsidePage}
	}
	if profile.Bending != nil && d.KFactor == 0 {
		d.KFactor = profile.Bending.KFactor
	}
	return d
}

// Repair returns the design file with the request's design adjustments written into it (see design.Repair), read the way Check reads it so the adjustments name the same entities. Its errors are those of Check.
func Repair(ctx context.Context, req CheckRequest) (out []byte, err error) {
	defer recoverAsInternal(&err)
	return repair(ctx, req)
}

func repair(ctx context.Context, req CheckRequest) ([]byte, error) {
	if err := capability.Validate(req.Capability); err != nil {
		return nil, err
	}
	profile, err := capability.Resolve(req.Capability, req.Stock)
	if err != nil {
		return nil, err
	}
	if e := validateDesign(req.Design); e != nil {
		return nil, e
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Design.Format == constants.DesignFormatSTEP {
		return nil, apierror.New(constants.ErrorCodeDesignContentUnsupported, "design.format", "fixes are not written into STEP models; change the model in CAD")
	}
	out, rerr := design.Repair(readAs(req.Design, profile))
	if rerr != nil {
		return nil, designError(rerr)
	}
	return out, nil
}

// Preview returns an SVG of the design as Check reads it, with the check's findings marked (see preview.Render). Holes are drawn at their drawn size, before any hole operation resizes them. Its errors are those of Check.
func Preview(ctx context.Context, req CheckRequest) (out []byte, err error) {
	defer recoverAsInternal(&err)
	return renderPreview(ctx, req)
}

// renderPreview checks the design, then reads it a second time: the check resizes holes for their operations, and the preview draws them as drawn.
func renderPreview(ctx context.Context, req CheckRequest) ([]byte, error) {
	res, err := Check(ctx, req)
	if err != nil {
		return nil, err
	}
	profile, aerr := capability.Resolve(req.Capability, req.Stock)
	if aerr != nil {
		return nil, aerr
	}
	part, _, perr := readPart(readAs(req.Design, profile))
	if perr != nil {
		return nil, designError(perr)
	}
	return preview.Render(part, res.Findings), nil
}

// keepSafeUnitsFix checks the design again in the units a set_units fix suggests, and drops the fix when that would raise the errors: the units are a guess from the drawing's size, not something to accept blindly.
func keepSafeUnitsFix(ctx context.Context, req CheckRequest, res *CheckResult) error {
	for i, f := range res.Findings {
		if f.Fix == nil || f.Fix.Action != constants.FixActionSetUnits {
			continue
		}
		again := req
		again.Design.Units = ""
		again.Design.Adjustments = append(slices.Clone(req.Design.Adjustments), design.Adjustment{Action: constants.FixActionSetUnits, Units: f.Fix.Units})
		other, err := Check(ctx, again)
		if err != nil {
			var e *apierror.Error
			if errors.As(err, &e) && e.Type != constants.ErrorTypeAPI {
				res.Findings[i].Fix = nil
				continue
			}
			return err
		}
		if other.ErrorCount > res.ErrorCount {
			res.Findings[i].Fix = nil
		}
	}
	return nil
}
