package constants

// DesignFormat is the encoding of a design file.
type DesignFormat string

const (
	// DesignFormatDXF is a DXF drawing, ASCII or binary.
	DesignFormatDXF DesignFormat = "dxf"
	// DesignFormatSVG is an SVG drawing.
	DesignFormatSVG DesignFormat = "svg"
	// DesignFormatSTEP is a STEP model (ISO 10303-21) of a solid part.
	DesignFormatSTEP DesignFormat = "step"
)

var designFormats = []DesignFormat{DesignFormatDXF, DesignFormatSVG, DesignFormatSTEP}

func (f DesignFormat) IsValid() bool        { return isOneOf(f, designFormats) }
func (f DesignFormat) EnumValues() []string { return enumValues(designFormats) }

// Open marks the enum as gaining values: formats are added with processes.
func (f DesignFormat) Open() bool { return true }
