package constants

// ThresholdBasis names the term of a threshold that set a requirement.
type ThresholdBasis string

const (
	ThresholdBasisMin            ThresholdBasis = "min"
	ThresholdBasisThicknessRatio ThresholdBasis = "thickness_ratio"
	ThresholdBasisKerfRatio      ThresholdBasis = "kerf_ratio"
	ThresholdBasisMax            ThresholdBasis = "max"
)

var thresholdBases = []ThresholdBasis{ThresholdBasisMin, ThresholdBasisThicknessRatio, ThresholdBasisKerfRatio, ThresholdBasisMax}

func (b ThresholdBasis) IsValid() bool        { return isOneOf(b, thresholdBases) }
func (b ThresholdBasis) EnumValues() []string { return enumValues(thresholdBases) }
