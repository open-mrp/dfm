package constants

// PlanVerdict says whether a design can be made by a plan: its operations' verdicts folded into one, with incomplete for a plan that could not check every operation.
type PlanVerdict string

const (
	// PlanVerdictManufacturable: every operation is manufacturable.
	PlanVerdictManufacturable PlanVerdict = PlanVerdict(VerdictManufacturable)
	// PlanVerdictNeedsReview: no operation failed or went unchecked, but one needs review.
	PlanVerdictNeedsReview PlanVerdict = PlanVerdict(VerdictNeedsReview)
	// PlanVerdictNotManufacturable: an operation is not manufacturable.
	PlanVerdictNotManufacturable PlanVerdict = PlanVerdict(VerdictNotManufacturable)
	// PlanVerdictIncomplete: an operation was not checked (no capability for its process) and none failed.
	PlanVerdictIncomplete PlanVerdict = "incomplete"
)

var planVerdicts = []PlanVerdict{PlanVerdictManufacturable, PlanVerdictNeedsReview, PlanVerdictNotManufacturable, PlanVerdictIncomplete}

func (v PlanVerdict) IsValid() bool        { return isOneOf(v, planVerdicts) }
func (v PlanVerdict) EnumValues() []string { return enumValues(planVerdicts) }
