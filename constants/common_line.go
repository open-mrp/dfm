package constants

// CommonLine says whether a capability cuts parts that share a cut line.
type CommonLine string

const (
	// CommonLineRejected requires every part to have its own outline.
	CommonLineRejected CommonLine = "rejected"
	// CommonLineAccepted cuts a shared line once for both parts.
	CommonLineAccepted CommonLine = "accepted"
)

var commonLines = []CommonLine{CommonLineRejected, CommonLineAccepted}

func (c CommonLine) IsValid() bool        { return isOneOf(c, commonLines) }
func (c CommonLine) EnumValues() []string { return enumValues(commonLines) }
