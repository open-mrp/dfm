package constants

// FindingScope says what a finding code is about.
type FindingScope string

const (
	// FindingScopeDrawing findings are about the drawing as a whole (its paths and the stock); they name the parts they fall in, if any.
	FindingScopeDrawing FindingScope = "drawing"
	// FindingScopePart findings are about one part.
	FindingScopePart FindingScope = "part"
)

var findingScopes = []FindingScope{FindingScopeDrawing, FindingScopePart}

func (s FindingScope) IsValid() bool        { return isOneOf(s, findingScopes) }
func (s FindingScope) EnumValues() []string { return enumValues(findingScopes) }
