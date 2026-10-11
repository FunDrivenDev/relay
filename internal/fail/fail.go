// Package fail holds the errors a command returns: each carries a stable code
// (codes.go) that the error envelope {"error":{"code","message"}} reports, and
// the exit status follows from it (2 for a usage error, 1 otherwise).
package fail

import (
	"errors"
	"fmt"
)

// Error is a command failure with a stable code.
type Error struct {
	Code    Code
	Message string
	// Silent errors print nothing: the command already wrote its verdict
	// (a "done": false result, a list of unmet conditions) and only the exit
	// status remains to report.
	Silent bool
	// Err is the underlying cause, kept for errors.Is and errors.As.
	Err error
}

func (e *Error) Error() string { return e.Message }

func (e *Error) Unwrap() error { return e.Err }

// FailCode reports the error's code (see Coder).
func (e *Error) FailCode() Code { return e.Code }

// Coder is implemented by any error that knows its code, such as run.Error.
type Coder interface {
	FailCode() Code
}

// New returns an error with the given code and a formatted message.
func New(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap returns an error with the given code and message that wraps cause.
func Wrap(code Code, cause error, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Err: cause}
}

// Usagef returns a usage error (exit status 2).
func Usagef(format string, args ...any) *Error {
	return New(Usage, format, args...)
}

// Quiet returns an error that only sets the exit status: the command has
// already reported the failure in its output.
func Quiet(code Code) *Error {
	return &Error{Code: code, Message: string(code), Silent: true}
}

// CodeOf returns the code of err: its own when it has one, Failure otherwise.
func CodeOf(err error) Code {
	var c Coder
	if errors.As(err, &c) {
		return c.FailCode()
	}
	return Failure
}

// ExitStatus is the process exit status for err: 0 without an error, 2 for a
// usage error, 1 for any other.
func ExitStatus(err error) int {
	switch {
	case err == nil:
		return 0
	case CodeOf(err) == Usage:
		return 2
	default:
		return 1
	}
}

// IsSilent reports whether err must not be printed.
func IsSilent(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Silent
}
