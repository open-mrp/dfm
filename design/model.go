// Package design turns source design files into normalized geometry.
//
// It is the only package that knows about file formats. Rules consume the geometry.Part it produces and never see format-specific entities.
package design

import (
	"errors"
	"fmt"

	"github.com/open-mrp/dfm/constants"
	"github.com/open-mrp/dfm/geometry"
	"github.com/open-mrp/dfm/measure"
)

// Design is a source design file.
type Design struct {
	Format constants.DesignFormat
	Data   []byte
	// Units overrides the drawing units declared in the file. Leave it empty to use the file's declaration; files that declare no units are rejected with ErrUnitsUnknown unless Units is set.
	Units constants.LengthUnit
	// ClosureTolerance closes gaps between path ends up to this size (see geometry.BuildOptions). Check sets it from the capability.
	ClosureTolerance measure.Length
	// Operations says which shapes of an SVG drawing are cut. Check sets it from the capability.
	Operations Operations
	// Adjustments change how the drawing is read, as accepted from the fixes findings suggest.
	Adjustments []Adjustment
	// NearDuplicate drops a line drawn within this of another, as a copy (see geometry.BuildOptions). Check sets it from the kerf width.
	NearDuplicate measure.Length
	// KFactor unfolds a sheet-metal model (STEP) into its flat pattern; 0 means the default. Check sets it from the press brake.
	KFactor measure.Ratio
}

// Adjustment is a change to how a drawing is read. MarkBendLine and Remove name the entities they apply to; SetUnits names the units.
type Adjustment struct {
	Action    constants.FixAction
	EntityIDs []string
	Units     constants.LengthUnit
}

// AdjustmentError is returned when adjustment Index names an entity the drawing does not have.
type AdjustmentError struct {
	Index    int
	EntityID string
}

func (e *AdjustmentError) Error() string {
	return fmt.Sprintf("the drawing has no entity %q", e.EntityID)
}

// Operations says what the shapes of a drawing are for. ByColor applies to SVG and DXF; Filled to SVG only.
type Operations struct {
	// ByColor maps a color, "#rrggbb" in lower case, to an operation. A shape's color is its stroke, or its fill when it has no stroke.
	ByColor map[string]constants.ShapeOperation
	// Filled is for shapes of other colors filled but not stroked; empty means ShapeOperationEngrave. Stroked shapes of other colors are cut.
	Filled constants.ShapeOperation
	// OutsidePage is for SVG shapes wholly outside the page; empty means OutsidePageCut.
	OutsidePage constants.OutsidePage
}

// shapeOp is what a shape of a drawing is for, once its color, stroke, fill and visibility have been read against the capability's Operations.
type shapeOp int

const (
	opCut          shapeOp = iota // a stroked outline
	opEngrave                     // filled without a stroke: area engraving
	opConstruction                // hidden, neither stroked nor filled, or ignored
)

// operationOf converts a capability's shape operation.
func operationOf(op constants.ShapeOperation) shapeOp {
	switch op {
	case constants.ShapeOperationCut:
		return opCut
	case constants.ShapeOperationEngrave:
		return opEngrave
	}
	return opConstruction
}

var (
	// ErrUnsupportedFormat is returned for a Format the engine does not read, and by Parse for a 3D format, which ParseModel reads.
	ErrUnsupportedFormat = errors.New("unsupported design format")
	// ErrInvalid is returned when the file cannot be parsed or contains no cut geometry.
	ErrInvalid = errors.New("invalid design")
	// ErrUnsupportedContent is returned when the file is well-formed but uses features this engine cannot evaluate (e.g. external references, fit-point splines, meshes, unknown units). The engine refuses rather than silently ignoring geometry that may be cut.
	ErrUnsupportedContent = errors.New("unsupported design content")
	// ErrUnitsUnknown is returned when the file does not declare its units and none were supplied in Design.Units. Guessing would risk checking an inch drawing at 1/25.4 scale.
	ErrUnitsUnknown = errors.New("design units unknown")
)

// MaxBytes is the largest design file read. The readers also cap what a file may expand to (block copies, SVG nodes, STEP faces).
const MaxBytes = 64 << 20

// MaxSegments is the most cut segments and circles a drawing may hold. The checks are near linear in segments, but a drawing past this is minutes of work, and no real part needs it: the largest in the corpus, a sheet of 67 nested parts with every curve drawn as a polyline, has 99,000.
const MaxSegments = 250_000

// tooManySegments refuses a drawing over MaxSegments.
func tooManySegments(paths []geometry.Contour, circles []geometry.Circle) error {
	n := len(circles)
	for _, c := range paths {
		n += len(c.Segments)
	}
	if n > MaxSegments {
		return fmt.Errorf("%w: the drawing has %d cut segments; at most %d are read (export curves as arcs or splines rather than dense polylines)", ErrUnsupportedContent, n, MaxSegments)
	}
	return nil
}

// tooLarge refuses a file over MaxBytes.
func tooLarge(d Design) error {
	if len(d.Data) > MaxBytes {
		return fmt.Errorf("%w: the file is %.1f MB; at most %d MB is read", ErrUnsupportedContent, float64(len(d.Data))/(1<<20), MaxBytes>>20)
	}
	return nil
}

// Parse decodes d into a normalized part.
func Parse(d Design) (*geometry.Part, error) {
	if err := tooLarge(d); err != nil {
		return nil, err
	}
	units := d.Units
	for _, a := range d.Adjustments {
		if a.Action == constants.FixActionSetUnits {
			units = a.Units
		}
	}
	opts := geometry.BuildOptions{ClosureToleranceMM: d.ClosureTolerance.MM(), NearDuplicateMM: d.NearDuplicate.MM()}
	switch d.Format {
	case constants.DesignFormatDXF:
		return parseDXF(d.Data, units, d.Operations, d.Adjustments, opts)
	case constants.DesignFormatSVG:
		return parseSVG(d.Data, units, d.Operations, d.Adjustments, opts)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, d.Format)
	}
}
