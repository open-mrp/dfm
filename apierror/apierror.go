// Package apierror is the error type every layer returns. It mirrors the platform API's error contract so callers handle dfm errors the same way.
package apierror

import (
	"encoding/json"
	"fmt"

	"github.com/open-mrp/dfm/constants"
)

// Error is a structured, client-safe error.
type Error struct {
	// Code is the machine-readable error code.
	Code constants.ErrorCode `json:"code"`
	// Type is the broad error category.
	Type constants.ErrorType `json:"type"`
	// Message is the human-readable message returned to the client.
	Message string `json:"message"`
	// Param is the request parameter that caused the error, as a dotted path (e.g. "stock.thickness.value"); null when the error is not about one. When several parameters fail, it is the first of Errors.
	Param string `json:"param"`
	// IsTransient is true when retrying the same request may succeed.
	IsTransient bool `json:"is_transient"`
	// Errors lists every failing parameter when the request fails validation, so a caller fixes them all at once; empty for other errors.
	Errors []FieldError `json:"errors"`
	// Internal is the underlying error, never sent to clients.
	Internal error `json:"-"`
}

// FieldError is one failing request parameter.
type FieldError struct {
	Param   string              `json:"param"`
	Code    constants.ErrorCode `json:"code"`
	Message string              `json:"message"`
}

// MarshalJSON writes param as null when the error is not about a parameter and errors as an empty list rather than null, as the platform API does.
func (e *Error) MarshalJSON() ([]byte, error) {
	var param *string
	if e.Param != "" {
		param = &e.Param
	}
	errs := e.Errors
	if errs == nil {
		errs = []FieldError{}
	}
	return json.Marshal(struct {
		Code        constants.ErrorCode `json:"code"`
		Type        constants.ErrorType `json:"type"`
		Message     string              `json:"message"`
		Param       *string             `json:"param"`
		IsTransient bool                `json:"is_transient"`
		Errors      []FieldError        `json:"errors"`
	}{e.Code, e.Type, e.Message, param, e.IsTransient, errs})
}

// Join makes one error of several parameter errors: the first's code, type, message and param, with every one listed in Errors.
func Join(errs []*Error) *Error {
	if len(errs) == 0 {
		return nil
	}
	first := *errs[0]
	first.Errors = make([]FieldError, 0, len(errs))
	for _, e := range errs {
		first.Errors = append(first.Errors, FieldError{Param: e.Param, Code: e.Code, Message: e.Message})
	}
	return &first
}

func (e *Error) Error() string {
	s := string(e.Code) + ": " + e.Message
	if e.Param != "" {
		s += " (param " + e.Param + ")"
	}
	return s
}

func (e *Error) Unwrap() error { return e.Internal }

func newError(code constants.ErrorCode, typ constants.ErrorType, param string, format string, args ...any) *Error {
	return &Error{Code: code, Type: typ, Message: fmt.Sprintf(format, args...), Param: param}
}

// ParameterMissing reports a required parameter that was absent or null.
func ParameterMissing(param string) *Error {
	return newError(constants.ErrorCodeParameterMissing, constants.ErrorTypeInvalidRequest, param, "%s is required", param)
}

// ParameterUnknown reports a parameter the request does not define.
func ParameterUnknown(param string) *Error {
	return newError(constants.ErrorCodeParameterUnknown, constants.ErrorTypeInvalidRequest, param, "%s is not a recognized parameter", param)
}

// ParameterInvalid reports a parameter with an unacceptable value.
func ParameterInvalid(param, format string, args ...any) *Error {
	return newError(constants.ErrorCodeParameterInvalid, constants.ErrorTypeInvalidRequest, param, format, args...)
}

// New builds an error with an explicit code; param may be empty.
func New(code constants.ErrorCode, param, format string, args ...any) *Error {
	typ := constants.ErrorTypeInvalidRequest
	if code == constants.ErrorCodeInternalError {
		typ = constants.ErrorTypeAPI
	}
	return newError(code, typ, param, format, args...)
}

// Internal wraps an unexpected failure; the cause is kept out of the message.
func Internal(err error) *Error {
	e := newError(constants.ErrorCodeInternalError, constants.ErrorTypeAPI, "", "Something went wrong.")
	e.Internal = err
	return e
}

// NextStep says what a caller, such as an agent, should do about an error with this code.
func NextStep(code constants.ErrorCode) string {
	switch code {
	case constants.ErrorCodeInvalidFormat:
		return "Send the request as a JSON object that follows the check-request schema."
	case constants.ErrorCodeParameterMissing:
		return "Add the parameter named in param and send the request again."
	case constants.ErrorCodeParameterUnknown:
		return "Remove or rename the parameter named in param; the message suggests the likely intended name."
	case constants.ErrorCodeParameterInvalid:
		return "Correct the value of the parameter named in param as the message says, and send the request again."
	case constants.ErrorCodeDesignInvalid:
		return "Export the design again from the CAD program (the message names the line or entity at fault); the file is damaged, truncated or has no cut geometry."
	case constants.ErrorCodeDesignFormatUnsupported:
		return "Export the design as DXF or SVG."
	case constants.ErrorCodeDesignContentUnsupported:
		return "Export the design again with plain geometry: explode blocks and convert splines, meshes and 3D solids to lines, arcs and polylines (the message names what is unsupported)."
	case constants.ErrorCodeDesignUnitsUnknown:
		return "Ask the user which units the drawing is in (the message may give the file's hint), then send design.units with them."
	case constants.ErrorCodeInternalError:
		return "Retry once; if it fails again, report it with the request and design."
	default:
		return ""
	}
}
