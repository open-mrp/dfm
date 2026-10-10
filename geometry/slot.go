package geometry

// SlotShape identifies how a slot was recognized.
type SlotShape string

const (
	// SlotObround is a stadium shape: two parallel lines joined by two semicircular ends.
	SlotObround SlotShape = "obround"
	// SlotRectangular is a four-sided cut-out with right-angled corners.
	SlotRectangular SlotShape = "rectangular"
)

// Slot is an interior elongated cut-out recognized from a closed contour.
type Slot struct {
	Shape     SlotShape
	WidthMM   float64
	LengthMM  float64
	EntityIDs []string
	// Contour indexes the slot's outline in Part.Contours.
	Contour int
}
