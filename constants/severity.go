package constants

// Severity classifies a finding. An error makes a design not manufacturable. A review finding means the engine cannot decide, and a human must look at the design before it is made, so the design is not manufacturable as checked but needs review rather than failing. A warning never blocks.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityReview  Severity = "review"
	SeverityWarning Severity = "warning"
)

var severities = []Severity{SeverityError, SeverityReview, SeverityWarning}

func (s Severity) IsValid() bool        { return isOneOf(s, severities) }
func (s Severity) EnumValues() []string { return enumValues(severities) }
