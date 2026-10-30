package apierr

import (
	"encoding/json"
	"errors"
	"net/http"
)

const (
	CodeUnauthorized      = "UNAUTHORIZED"
	CodePermissionDenied  = "PERMISSION_DENIED"
	CodeMalformed         = "MALFORMED_REQUEST"
	CodeInvalidParams     = "INVALID_PARAMS"
	CodePayloadTooLarge   = "PAYLOAD_TOO_LARGE"
	CodeConflict          = "SAMPLE_CONFLICT"
	CodeReportConflict    = "REPORT_CONFLICT"
	CodeAlreadySetup      = "ALREADY_SETUP"
	CodeEnrollmentInvalid = "ENROLLMENT_INVALID"
	CodeEnrollmentExpired = "ENROLLMENT_EXPIRED"
	CodeEnrollmentUsed    = "ENROLLMENT_USED"
	CodeDeviceRevoked     = "DEVICE_REVOKED"
	CodeDeviceNotFound    = "DEVICE_NOT_FOUND"
	CodeNotFound          = "NOT_FOUND"
	CodeRateLimited       = "RATE_LIMITED"
	CodePushUnconfigured  = "PUSH_UNCONFIGURED"
	CodeUnexpected        = "UNEXPECTED_ERROR"
	CodeSetupRequired     = "SETUP_TOKEN_REQUIRED"
	CodeLastLogin         = "LAST_LOGIN_METHOD"
	CodeVaultUnavailable  = "VAULT_UNAVAILABLE"
	CodeUpstream          = "UPSTREAM_REJECTED"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Error struct {
	Status  int
	Message string
	Code    string
	Fields  []FieldError
}

func (e *Error) Error() string {
	return e.Message
}

func New(status int, code string, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func Invalid(fields []FieldError) *Error {
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    CodeInvalidParams,
		Message: "Os dados não passaram na validação.",
		Fields:  fields,
	}
}

func Write(w http.ResponseWriter, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		apiErr = New(http.StatusInternalServerError, CodeUnexpected, "Erro interno.")
	}
	body := map[string]any{
		"message":    apiErr.Message,
		"error_code": apiErr.Code,
	}
	if len(apiErr.Fields) > 0 {
		body["extra"] = map[string]any{
			"validation_errors": apiErr.Fields,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(apiErr.Status)
	_ = json.NewEncoder(w).Encode(body)
}
