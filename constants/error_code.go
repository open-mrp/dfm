package constants

// ErrorCode is the machine-readable code of an API error.
type ErrorCode string

const (
	ErrorCodeInvalidFormat            ErrorCode = "invalid_format"
	ErrorCodeParameterMissing         ErrorCode = "parameter_missing"
	ErrorCodeParameterUnknown         ErrorCode = "parameter_unknown"
	ErrorCodeParameterInvalid         ErrorCode = "parameter_invalid"
	ErrorCodeDesignInvalid            ErrorCode = "design_invalid"
	ErrorCodeDesignFormatUnsupported  ErrorCode = "design_format_unsupported"
	ErrorCodeDesignContentUnsupported ErrorCode = "design_content_unsupported"
	ErrorCodeDesignUnitsUnknown       ErrorCode = "design_units_unknown"
	ErrorCodeInternalError            ErrorCode = "internal_error"
)

var errorCodes = []ErrorCode{
	ErrorCodeInvalidFormat, ErrorCodeParameterMissing, ErrorCodeParameterUnknown,
	ErrorCodeParameterInvalid, ErrorCodeDesignInvalid, ErrorCodeDesignFormatUnsupported,
	ErrorCodeDesignContentUnsupported, ErrorCodeDesignUnitsUnknown, ErrorCodeInternalError,
}

func (c ErrorCode) IsValid() bool        { return isOneOf(c, errorCodes) }
func (c ErrorCode) EnumValues() []string { return enumValues(errorCodes) }
