package geometry

// OffPlane is a cut entity that is not at the drawing's height: it lies above or below the plane most entities share, or slopes across it, by up to DistanceMM.
type OffPlane struct {
	EntityID   string
	DistanceMM float64
}
