package constants

// OutsidePage says what a capability does with SVG shapes lying wholly outside the page.
type OutsidePage string

const (
	// OutsidePageCut cuts them like the rest.
	OutsidePageCut OutsidePage = "cut"
	// OutsidePageIgnore leaves them out.
	OutsidePageIgnore OutsidePage = "ignore"
)

var outsidePages = []OutsidePage{OutsidePageCut, OutsidePageIgnore}

func (o OutsidePage) IsValid() bool        { return isOneOf(o, outsidePages) }
func (o OutsidePage) EnumValues() []string { return enumValues(outsidePages) }
