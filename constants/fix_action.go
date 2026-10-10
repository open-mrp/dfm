package constants

// FixAction is a change to how a drawing is read: a finding suggests one as its fix, and a request accepts it as an adjustment.
type FixAction string

const (
	// FixActionMarkBendLine reads the entities as bend lines: not cut.
	FixActionMarkBendLine FixAction = "mark_bend_line"
	// FixActionRemove leaves the entities out of the drawing.
	FixActionRemove FixAction = "remove"
	// FixActionSetUnits reads the drawing in other units.
	FixActionSetUnits FixAction = "set_units"
)

var fixActions = []FixAction{FixActionMarkBendLine, FixActionRemove, FixActionSetUnits}

func (a FixAction) IsValid() bool        { return isOneOf(a, fixActions) }
func (a FixAction) EnumValues() []string { return enumValues(fixActions) }
