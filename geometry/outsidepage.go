package geometry

// OutsidePage is a cut shape lying wholly outside the drawing's page; Ignored is set when it was left out of the part.
type OutsidePage struct {
	EntityID string
	Location Point
	Ignored  bool
}
