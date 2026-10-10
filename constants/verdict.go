package constants

// Verdict says whether a design, or one part of it, can be made as checked.
type Verdict string

const (
	// VerdictManufacturable: no error or review findings.
	VerdictManufacturable Verdict = "manufacturable"
	// VerdictNeedsReview: no errors, but review findings a person must judge before the design is made.
	VerdictNeedsReview Verdict = "needs_review"
	// VerdictNotManufacturable: at least one error.
	VerdictNotManufacturable Verdict = "not_manufacturable"
)

var verdicts = []Verdict{VerdictManufacturable, VerdictNeedsReview, VerdictNotManufacturable}

func (v Verdict) IsValid() bool        { return isOneOf(v, verdicts) }
func (v Verdict) EnumValues() []string { return enumValues(verdicts) }
