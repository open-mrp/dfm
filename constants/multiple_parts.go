package constants

// MultipleParts says how a capability treats a drawing with more than one part.
type MultipleParts string

const (
	// MultiplePartsRejected requires one part per drawing.
	MultiplePartsRejected MultipleParts = "rejected"
	// MultiplePartsAccepted checks each part of a drawing with several side by side separately. Material inside a cut-out is a loose island.
	MultiplePartsAccepted MultipleParts = "accepted"
	// MultiplePartsAcceptedInCutouts also reads material inside a cut-out as a part nested there (part-in-part).
	MultiplePartsAcceptedInCutouts MultipleParts = "accepted_in_cutouts"
)

var multipleParts = []MultipleParts{MultiplePartsRejected, MultiplePartsAccepted, MultiplePartsAcceptedInCutouts}

func (m MultipleParts) IsValid() bool        { return isOneOf(m, multipleParts) }
func (m MultipleParts) EnumValues() []string { return enumValues(multipleParts) }
