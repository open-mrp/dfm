package geometry

// Text is a text entity in the drawing. Position is its insertion point, or nil when the file does not give a usable one.
type Text struct {
	EntityID string
	Position *Point
}
