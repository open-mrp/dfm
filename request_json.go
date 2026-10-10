package dfm

import (
	"fmt"

	"github.com/open-mrp/dfm/apierror"
	"github.com/open-mrp/dfm/capability"
	"github.com/open-mrp/dfm/design"
	"github.com/open-mrp/dfm/field"
	"github.com/open-mrp/dfm/request"
)

// RequestFromJSON builds a check request from a design file and a check request JSON document (schema check-request), as the platform stores a check. Its errors are request errors with the parameter at fault.
func RequestFromJSON(designData, requestJSON []byte) (CheckRequest, *apierror.Error) {
	in, err := request.DecodeCheckRequest(requestJSON)
	if err != nil {
		return CheckRequest{}, err
	}
	c, err := in.Capability.ToDomain("capability")
	if err != nil {
		return CheckRequest{}, err
	}
	d, stock, holes, bends, err := job(designData, in.Design, in.Stock, in.HoleOperations, in.Bends)
	if err != nil {
		return CheckRequest{}, err
	}
	return CheckRequest{Design: d, Capability: c, Stock: stock, HoleOperations: holes, Bends: bends}, nil
}

// PlanFromJSON builds a plan request from a design file and a plan request JSON document (schema plan-request).
func PlanFromJSON(designData, requestJSON []byte) (PlanRequest, *apierror.Error) {
	in, err := request.DecodePlanRequest(requestJSON)
	if err != nil {
		return PlanRequest{}, err
	}
	d, stock, holes, bends, err := job(designData, in.Design, in.Stock, in.HoleOperations, in.Bends)
	if err != nil {
		return PlanRequest{}, err
	}
	out := PlanRequest{Design: d, Stock: stock, HoleOperations: holes, Bends: bends}
	for i, ci := range in.Capabilities {
		c, err := ci.ToDomain(fmt.Sprintf("capabilities[%d]", i))
		if err != nil {
			return PlanRequest{}, err
		}
		out.Capabilities = append(out.Capabilities, c)
	}
	return out, nil
}

// job converts what a check and a plan request share: the design with its file, the stock, and the hole and bend assignments.
func job(designData []byte, d request.DesignInput, s request.StockInput, ops field.Optional[[]request.HoleAssignmentInput], bends field.Optional[[]request.BendAssignmentInput]) (design.Design, capability.Stock, []capability.HoleAssignment, []capability.BendAssignment, *apierror.Error) {
	stock, err := s.ToDomain()
	if err != nil {
		return design.Design{}, capability.Stock{}, nil, nil, err
	}
	opsIn, _ := ops.Value()
	holes, err := request.HoleAssignmentsToDomain(opsIn)
	if err != nil {
		return design.Design{}, capability.Stock{}, nil, nil, err
	}
	bendsIn, _ := bends.Value()
	assigned, err := request.BendAssignmentsToDomain(bendsIn)
	if err != nil {
		return design.Design{}, capability.Stock{}, nil, nil, err
	}
	return d.ToDomain(designData), stock, holes, assigned, nil
}
