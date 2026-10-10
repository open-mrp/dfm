package constants

// ErrorType is the broad category of an API error.
type ErrorType string

const (
	ErrorTypeInvalidRequest ErrorType = "invalid_request_error"
	ErrorTypeAPI            ErrorType = "api_error"
)

var errorTypes = []ErrorType{ErrorTypeInvalidRequest, ErrorTypeAPI}

func (t ErrorType) IsValid() bool        { return isOneOf(t, errorTypes) }
func (t ErrorType) EnumValues() []string { return enumValues(errorTypes) }
