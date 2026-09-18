package clienv

import "errors"

const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// ExitError lets commands select a process exit code independently from
// whether an error message should be rendered.
type ExitError struct {
	Code   int
	Err    error
	Silent bool
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func UsageError(err error) error {
	return &ExitError{Code: ExitUsage, Err: err}
}

func SilentFailure() error {
	return &ExitError{Code: ExitFailure, Silent: true}
}

func ResolveExit(err error) (code int, message error) {
	if err == nil {
		return ExitSuccess, nil
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		return ExitFailure, err
	}
	if exitErr.Silent {
		return exitErr.Code, nil
	}
	return exitErr.Code, exitErr.Err
}
