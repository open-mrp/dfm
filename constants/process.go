package constants

// Process is a manufacturing process a capability describes.
type Process string

const (
	// ProcessLaserCutting is 2D laser cutting of sheet stock.
	ProcessLaserCutting Process = "laser_cutting"
	// ProcessBending is air bending of a cut flat part on a press brake.
	ProcessBending Process = "bending"
	// ProcessCountersinking, ProcessTapping and ProcessHardwareInsertion are the hole operations of a plan. They have no capability of their own: the laser-cutting capability's hole_operations offers them.
	ProcessCountersinking    Process = "countersinking"
	ProcessTapping           Process = "tapping"
	ProcessHardwareInsertion Process = "hardware_insertion"
)

var processes = []Process{ProcessLaserCutting, ProcessBending, ProcessCountersinking, ProcessTapping, ProcessHardwareInsertion}

// HasCapability reports whether a capability can describe the process: the hole operations are offered by a laser-cutting capability instead.
func (p Process) HasCapability() bool { return p == ProcessLaserCutting || p == ProcessBending }

func (p Process) IsValid() bool        { return isOneOf(p, processes) }
func (p Process) EnumValues() []string { return enumValues(processes) }

// Open marks the enum as gaining values: processes are added.
func (p Process) Open() bool { return true }
