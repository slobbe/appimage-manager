package services

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorKind classifies app-service errors for presentation-layer mapping.
type ErrorKind string

const (
	ErrorInvalidInput ErrorKind = "invalid_input"
	ErrorNotFound     ErrorKind = "not_found"
	ErrorUnavailable  ErrorKind = "unavailable"
	ErrorPermission   ErrorKind = "permission"
	ErrorConflict     ErrorKind = "conflict"
	ErrorCanceled     ErrorKind = "canceled"
	ErrorInternal     ErrorKind = "internal"
)

// Error carries a semantic app-service error kind across the CLI boundary.
type Error struct {
	Kind ErrorKind
	Op   string
	Err  error
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}

	message := "app service error"
	if err.Err != nil {
		message = err.Err.Error()
	}
	if op := strings.TrimSpace(err.Op); op != "" {
		return op + ": " + message
	}
	return message
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

// NewError wraps err with a semantic app-service kind.
func NewError(kind ErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}
	if kind == "" {
		kind = ErrorInternal
	}
	return &Error{Kind: kind, Op: strings.TrimSpace(op), Err: err}
}

// Errorf creates a semantic app-service error from a formatted message.
func Errorf(kind ErrorKind, op, format string, args ...interface{}) error {
	return NewError(kind, op, fmt.Errorf(format, args...))
}

func invalidInputErrorf(format string, args ...interface{}) error {
	return Errorf(ErrorInvalidInput, "", format, args...)
}

func internalErrorf(format string, args ...interface{}) error {
	return Errorf(ErrorInternal, "", format, args...)
}

// ErrorKindOf returns the semantic kind attached to err, if any.
func ErrorKindOf(err error) (ErrorKind, bool) {
	var serviceErr *Error
	if errors.As(err, &serviceErr) && serviceErr != nil && serviceErr.Kind != "" {
		return serviceErr.Kind, true
	}
	return "", false
}

// IsErrorKind reports whether err carries the requested semantic kind.
func IsErrorKind(err error, kind ErrorKind) bool {
	actual, ok := ErrorKindOf(err)
	return ok && actual == kind
}
