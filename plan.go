package dfm

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/design"
)

// PlanRequest asks for the operations that make a design, each checked against the capability for its process.
type PlanRequest struct {
	Design design.Design
	Stock  capability.Stock
	// Capabilities are the machines available, at most one per process, in any order (e.g. a laser cutter and a press brake).
	Capabilities []capability.Capability
	// HoleOperations go to the laser-cutting check.
	HoleOperations []capability.HoleAssignment
	// Bends go to the bending check.
	Bends []capability.BendAssignment
}

// Manufacturable reports whether the plan's verdict is manufacturable.
func (r PlanResult) Manufacturable() bool { return r.Verdict == constants.PlanVerdictManufacturable }

// Operation is one step of a plan.
type Operation struct {
	// Sequence is the step's 1-based place in the plan.
	Sequence int
	Process  constants.Process
	// Reason says why the design needs the step.
	Reason string
	// Result is the step's check, or nil when no capability for its process was given (the step is not checked).
	Result *CheckResult
}

// PlanResult is the operations that make a design, in order, each with its check.
type PlanResult struct {
	EngineVersion string
	// Verdict is not_manufacturable when an operation is, incomplete when an operation was not checked (no capability for its process), needs_review when review findings keep any from passing, and manufacturable when all passed.
	Verdict    constants.PlanVerdict
	Operations []Operation
}

// precedence orders processes: a flat part is cut before it is bent, and nothing is cut on a laser bed after bending.
var precedence = map[constants.Process]int{
	constants.ProcessLaserCutting:      1,
	constants.ProcessCountersinking:    2,
	constants.ProcessTapping:           3,
	constants.ProcessBending:           4,
	constants.ProcessHardwareInsertion: 5,
}

// holeSteps are the plan steps of each kind of hole operation, and why a design needs them. Countersinks and taps are made on the flat part (a flange would be in the way of the tool after bending); press-in fasteners go in after bending, so the brake does not crush them.
var holeSteps = map[constants.HoleOperationType]struct {
	process constants.Process
	reason  string
}{
	constants.HoleOperationCountersink: {constants.ProcessCountersinking, "holes are countersunk"},
	constants.HoleOperationTap:         {constants.ProcessTapping, "holes are tapped"},
	constants.HoleOperationHardware:    {constants.ProcessHardwareInsertion, "fasteners are pressed in"},
}

// CheckPlan works out which operations a design needs (laser cutting for any flat part; bending when the drawing has bend lines), puts them in the order they must happen, and checks each against the capability given for its process. The drawing is read the way the laser-cutting capability says, when one is given, so every step sees the same part.
func CheckPlan(ctx context.Context, req PlanRequest) (res PlanResult, err error) {
	defer recoverAsInternal(&err)
	return checkPlan(ctx, req)
}

func checkPlan(ctx context.Context, req PlanRequest) (PlanResult, error) {
	byProcess := map[constants.Process]capability.Capability{}
	for i, c := range req.Capabilities {
		if err := capability.Validate(c); err != nil {
			if rest, ok := strings.CutPrefix(err.Param, "capability"); ok {
				err.Param = fmt.Sprintf("capabilities[%d]%s", i, rest)
			}
			return PlanResult{}, err
		}
		if _, dup := byProcess[c.Process]; dup {
			return PlanResult{}, apierror.ParameterInvalid(fmt.Sprintf("capabilities[%d].process", i), "capabilities[%d] is a second %s capability; give one per process", i, c.Process)
		}
		byProcess[c.Process] = c
	}
	if err := capability.ValidateBendAssignments(req.Bends); err != nil {
		return PlanResult{}, err
	}
	if e := validateDesign(req.Design); e != nil {
		return PlanResult{}, e
	}
	d := req.Design
	if c, ok := byProcess[constants.ProcessLaserCutting]; ok {
		profile, err := capability.Resolve(c, req.Stock)
		if err != nil {
			return PlanResult{}, err
		}
		d = readAs(d, profile)
	}
	if c, ok := byProcess[constants.ProcessBending]; ok {
		// Every step unfolds a 3D model with the press brake's K-factor, so each sees the same flat part.
		profile, err := capability.Resolve(c, req.Stock)
		if err != nil {
			return PlanResult{}, err
		}
		d.KFactor = profile.Bending.KFactor
	}
	part, _, perr := readPart(d)
	if perr != nil {
		return PlanResult{}, designError(perr)
	}
	ops := []Operation{{Process: constants.ProcessLaserCutting, Reason: "the part is cut from flat stock"}}
	if n := len(part.BendLines()); n > 0 {
		reason := fmt.Sprintf("the drawing has %d bend line%s", n, map[bool]string{true: "", false: "s"}[n == 1])
		if d.Format == constants.DesignFormatSTEP {
			reason = fmt.Sprintf("the model has %d bend%s", n, map[bool]string{true: "", false: "s"}[n == 1])
		}
		ops = append(ops, Operation{Process: constants.ProcessBending, Reason: reason})
	}
	if laser, ok := byProcess[constants.ProcessLaserCutting]; ok && len(req.HoleOperations) > 0 {
		kinds := map[string]constants.HoleOperationType{}
		for _, op := range laser.HoleOperations {
			kinds[op.Code] = op.Type
		}
		codes := map[constants.HoleOperationType][]string{}
		for _, a := range req.HoleOperations {
			if k, ok := kinds[a.Operation]; ok && !slices.Contains(codes[k], a.Operation) {
				codes[k] = append(codes[k], a.Operation)
			}
		}
		for _, k := range []constants.HoleOperationType{constants.HoleOperationCountersink, constants.HoleOperationTap, constants.HoleOperationHardware} {
			if len(codes[k]) > 0 {
				slices.Sort(codes[k])
				s := holeSteps[k]
				ops = append(ops, Operation{Process: s.process, Reason: fmt.Sprintf("%s (%s)", s.reason, strings.Join(codes[k], ", "))})
			}
		}
	}
	slices.SortStableFunc(ops, func(a, b Operation) int { return precedence[a.Process] - precedence[b.Process] })
	res := PlanResult{EngineVersion: EngineVersion}
	failed, unchecked, review := false, false, false
	for i := range ops {
		ops[i].Sequence = i + 1
		// A step is checked by the capability of its own process, except a hole operation step, which the laser-cutting capability's catalog checks for that process, with the press brake bending the part when one is given.
		process := ops[i].Process
		checkedBy, step := process, constants.Process("")
		var bending *capability.BendingProfile
		if !process.HasCapability() {
			checkedBy, step = constants.ProcessLaserCutting, process
			if c, ok := byProcess[constants.ProcessBending]; ok {
				p, err := capability.Resolve(c, req.Stock)
				if err != nil {
					return PlanResult{}, err
				}
				bending = p.Bending
			}
		}
		c, ok := byProcess[checkedBy]
		if !ok {
			unchecked = true
			continue
		}
		creq := CheckRequest{Design: d, Capability: c, Stock: req.Stock}
		switch process {
		case constants.ProcessLaserCutting:
			creq.HoleOperations = req.HoleOperations
		case constants.ProcessBending:
			creq.Bends = req.Bends
		default:
			creq.HoleOperations, creq.Bends = req.HoleOperations, req.Bends
		}
		r, err := check(ctx, creq, step, bending)
		if err != nil {
			return PlanResult{}, err
		}
		ops[i].Result = &r
		failed = failed || r.Verdict == constants.VerdictNotManufacturable
		review = review || r.Verdict == constants.VerdictNeedsReview
	}
	res.Operations = ops
	switch {
	case failed:
		res.Verdict = constants.PlanVerdictNotManufacturable
	case unchecked:
		res.Verdict = constants.PlanVerdictIncomplete
	case review:
		res.Verdict = constants.PlanVerdictNeedsReview
	default:
		res.Verdict = constants.PlanVerdictManufacturable
	}
	return res, nil
}
