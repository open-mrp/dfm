package geometry

import (
	"github.com/open-mrp/dfm/constants"
)

// Part is the normalized geometry of a flat part.
//
// Closed contours and circles form a containment tree. A loop's Depth is the number of loops enclosing it: depth 0 is an outer material boundary, odd depths are cut-outs (holes, slots), and even depths of 2 or more are material islands left loose inside a cut-out. A well-formed single part has exactly one depth-0 loop and no islands.
type Part struct {
	// Bounds covers every cut path (open or closed).
	Bounds Bounds
	// Contours holds every chained path that is not a full circle.
	Contours []Contour
	// Circles holds every full circle, whether drawn as a CIRCLE entity or as a closed chain of co-circular arcs.
	Circles []Circle
	// Slots holds cut-out contours recognized as slots.
	Slots []Slot
	// HoleOps are the operations assigned to this part's round holes, which were resized (set by the check).
	HoleOps []HoleOp
	// Openings measure the gap of every open contour.
	Openings []Opening
	// Contacts are places where paths touch or cross.
	Contacts []Contact
	// Loops is the containment tree of every closed contour and circle, contours first, each in input order.
	Loops []Loop
	// Outline lists the entities of the part's outer boundary, and Unclosed marks a part whose outline is open paths rather than a closed loop (set by Parts).
	Outline  []string
	Unclosed bool

	// Report is what was found about the design beyond its cut geometry. A part rebuilt from its cut paths (hole operations resize holes) keeps it whole.
	Report

	owned map[string]bool
	// loopOf and entityAt find a loop and a point by entity ID; elems indexes every cut element by position, for nearest-cut queries; primGrids index a loop's primitives, for clearance queries. All are built on first use.
	loopOf    map[string]int
	entityAt  map[string]Point
	elems     *elementIndex
	ownerIDs  [][]string
	primGrids map[int]*boxGrid
	siblings  *clearanceCache
	parts     map[bool][]*Part   // cached by Parts
	shape     *shape             // cached by SameShape
	widths    map[int][2]float64 // cached by WidthAndLength, by loop
	necks     *neckCache         // cached by Necks
}

// Report is what reading a design found beyond its cut geometry: what the parser left out or had to assume, what the first build repaired or recognized in the paths, and what a check assigned. It describes the drawing, not the cut paths, so a part rebuilt from its paths keeps it as it is.
type Report struct {
	// Overcuts are paths that ran on past their start over their own first stretch; they were closed without the repeat.
	Overcuts []Overcut
	// FacetedCurves are curves drawn as many short straight segments.
	FacetedCurves []FacetedCurve
	// CommonLines are cut lines two parts share.
	CommonLines []CommonLine
	// HoleOpMisses and HoleOpConflicts are hole assignments that went wrong (set by the check, on the drawing only).
	HoleOpMisses    []HoleOpMiss
	HoleOpConflicts []HoleOpConflict
	// BendSpecs says how each bend line is bent, by entity ID (set by the bending check).
	BendSpecs map[string]BendSpec
	// CornerLoops are small loops at corners of paths, one entry per path; they were removed.
	CornerLoops []CornerLoops
	// Duplicates are paths and circles that exactly repeat other geometry; they were dropped from the part.
	Duplicates []Duplicate
	// ClosedGaps are breaks between path ends that were bridged because they were within the closure tolerance.
	ClosedGaps []Gap
	// Overlaps are stretches of path drawn more than once.
	Overlaps []Overlap
	// OffPlane lists cut entities not at the drawing's height (set by the design parser); their geometry is projected onto the plane.
	OffPlane []OffPlane
	// OutsidePage lists cut shapes wholly outside an SVG's page (set by the design parser).
	OutsidePage []OutsidePage
	// UnitsAssumed is set when part of the drawing's size had to be assumed, such as the size of an SVG pixel (set by the design parser).
	UnitsAssumed bool
	// ThicknessMM is the sheet thickness of a 3D model the part was unfolded from; 0 for a drawing.
	ThicknessMM float64
	// Units is the unit the drawing was read in, or "" when unknown (set by the design parser; geometry is in millimeters whatever it is).
	Units constants.LengthUnit
	// ZeroLength lists entities that were dropped because they have no length (set by the design parser).
	ZeroLength []string
	// ZeroLengthAt is where a zero-length entity is, when it is only a point and so has no geometry in the part.
	ZeroLengthAt map[string]Point
	// Text lists text entities, which are never cut (set by the design parser).
	Text []Text
	// NonCut lists entities that are not through-cuts (construction, bend and engraving geometry), which were left out of the part (set by the design parser).
	NonCut []NonCut
	// Border lists the entities of a sheet border drawn around the drawing, left out of the part, and BorderAt its corner (set by the design parser; see FindBorder).
	Border   []string
	BorderAt *Point
}

// Holes returns circles that are cut-outs (odd depth), including round cut-outs drawn as polygons, which are read as circles.
func (p *Part) Holes() []Circle {
	var holes []Circle
	for _, c := range p.Circles {
		if c.Depth%2 == 1 {
			holes = append(holes, c)
		}
	}
	return holes
}

// LoopIDs returns the entity IDs of every closed contour and circle at the given depths (selected by keep), contours first, in order.
func (p *Part) LoopIDs(keep func(depth int) bool) [][]string {
	var out [][]string
	for _, c := range p.Contours {
		if c.Closed && keep(c.Depth) {
			out = append(out, c.EntityIDs())
		}
	}
	for _, c := range p.Circles {
		if keep(c.Depth) {
			out = append(out, c.EntityIDs)
		}
	}
	return out
}
