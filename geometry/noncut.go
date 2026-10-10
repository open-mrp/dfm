package geometry

// NonCutKind says why an entity is not a through-cut.
type NonCutKind int

const (
	// NonCutConstruction is construction or hidden geometry: invisible, drawn with a line type that has gaps, or on a hidden layer.
	NonCutConstruction NonCutKind = iota
	// NonCutBend is a bend line.
	NonCutBend
	// NonCutEngrave is engraving or marking.
	NonCutEngrave
	// NonCutAnnotation is drawing annotation: dimensions, center lines, notes, title blocks and construction lines on a layer named for them.
	NonCutAnnotation
	// NonCutIgnored is geometry that is not drawn (hidden or invisible SVG shapes) or that the capability's color operations ignore; it is not reported.
	NonCutIgnored
	// NonCutSuspectedBend is a solid line on a cut layer that runs through the material from one cut to another, as a bend line does. It is left out of the part, and reported as an error until the drawing says what it is.
	NonCutSuspectedBend
)

// NonCut is an entity left out of the part because it is not a through-cut. Position is a point on it, or nil when it has none (no length, or an entity that is not converted, such as a block reference).
type NonCut struct {
	EntityID string
	Kind     NonCutKind
	Position *Point
	// Line holds the ends of a dashed construction entity that is one straight line: a candidate bend line.
	Line *[2]Point
	// Copies are entities drawn exactly on top of a suspected bend line, left out with it.
	Copies []string
}
