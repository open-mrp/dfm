package constants

// OperationVerdict is the verdict of one operation of a plan: its check's, or not_checked when the plan had no capability for its process.
type OperationVerdict string

const (
	OperationVerdictManufacturable    OperationVerdict = OperationVerdict(VerdictManufacturable)
	OperationVerdictNeedsReview       OperationVerdict = OperationVerdict(VerdictNeedsReview)
	OperationVerdictNotManufacturable OperationVerdict = OperationVerdict(VerdictNotManufacturable)
	// OperationVerdictNotChecked: no capability for the operation's process was given, so it has no result.
	OperationVerdictNotChecked OperationVerdict = "not_checked"
)

var operationVerdicts = []OperationVerdict{OperationVerdictManufacturable, OperationVerdictNeedsReview, OperationVerdictNotManufacturable, OperationVerdictNotChecked}

func (v OperationVerdict) IsValid() bool        { return isOneOf(v, operationVerdicts) }
func (v OperationVerdict) EnumValues() []string { return enumValues(operationVerdicts) }
