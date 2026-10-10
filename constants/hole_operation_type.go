package constants

// HoleOperationType is what a hole operation does to a hole.
type HoleOperationType string

const (
	// HoleOperationTap cuts a thread in the hole.
	HoleOperationTap HoleOperationType = "tap"
	// HoleOperationCountersink cuts a cone at one face for a flat head.
	HoleOperationCountersink HoleOperationType = "countersink"
	// HoleOperationHardware presses a self-clinching fastener into the hole.
	HoleOperationHardware HoleOperationType = "hardware"
)

var holeOperationTypes = []HoleOperationType{HoleOperationTap, HoleOperationCountersink, HoleOperationHardware}

func (k HoleOperationType) IsValid() bool        { return isOneOf(k, holeOperationTypes) }
func (k HoleOperationType) EnumValues() []string { return enumValues(holeOperationTypes) }
