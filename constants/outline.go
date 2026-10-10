package constants

// Outline says whether a part's outer boundary closes.
type Outline string

const (
	// OutlineClosed: the outline is a closed loop; the part's checks run.
	OutlineClosed Outline = "closed"
	// OutlineOpen: the outline's paths do not meet; the part's own checks wait until they do.
	OutlineOpen Outline = "open"
)

var outlines = []Outline{OutlineClosed, OutlineOpen}

func (o Outline) IsValid() bool        { return isOneOf(o, outlines) }
func (o Outline) EnumValues() []string { return enumValues(outlines) }
