package domain

import "fmt"

type ErrorCode string

const (
	ErrInvalidInput      ErrorCode = "INVALID_INPUT"
	ErrNotFound          ErrorCode = "NOT_FOUND"
	ErrConflict          ErrorCode = "CONFLICT"
	ErrInvalidTransition ErrorCode = "INVALID_TRANSITION"
	ErrIdempotency       ErrorCode = "IDEMPOTENCY_CONFLICT"
	ErrForbidden         ErrorCode = "FORBIDDEN"
	ErrIntegrity         ErrorCode = "INTEGRITY_ERROR"
)

type BusinessError struct {
	Code    ErrorCode
	Message string
	Details map[string]any
}

func (e *BusinessError) Error() string { return string(e.Code) + ": " + e.Message }
func Invalid(message string) error     { return &BusinessError{Code: ErrInvalidInput, Message: message} }
func NotFound(message string) error    { return &BusinessError{Code: ErrNotFound, Message: message} }
func Conflict(message string) error    { return &BusinessError{Code: ErrConflict, Message: message} }
func Transition(message string) error {
	return &BusinessError{Code: ErrInvalidTransition, Message: message}
}
func Forbidden(message string) error { return &BusinessError{Code: ErrForbidden, Message: message} }
func Integrity(message string) error { return &BusinessError{Code: ErrIntegrity, Message: message} }
func Wrap(code ErrorCode, message string, details map[string]any) error {
	return &BusinessError{Code: code, Message: message, Details: details}
}
func IsCode(err error, code ErrorCode) bool {
	if e, ok := err.(*BusinessError); ok {
		return e.Code == code
	}
	return false
}
func Ensure(condition bool, message string) error {
	if !condition {
		return Invalid(message)
	}
	return nil
}
func RevisionError(expected, actual int64) error {
	return Wrap(ErrConflict, fmt.Sprintf("revision 冲突，期望 %d，实际 %d", expected, actual), map[string]any{"expected_revision": expected, "actual_revision": actual})
}
