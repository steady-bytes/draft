package logging

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// customError is a custom error type that wraps a standard Go error with additional contextual information
type customError struct {
	cause    error
	function string
	fields   Fields
	file     string
	line     int
}

// NewError creates a standard Go error from a given error and message. This is useful when returning a standard Go error
// from a third party module that you don't control but want to add a custom message to the error before calling logger.WrapError().
func NewError(err error, message string) error {
	return fmt.Errorf("%s: %v", message, err)
}

// Error extends the standard Go error interface with a custom implementation of Error() and Unwrap() to build out a call stack
// and keep logger fields from the root of the call stack
type Error interface {
	error
	// Unwrap returns the underlying error. If wrapping has occurred it will take the shape of:
	//   "[main.FunctionA]->[module/package1.FunctionB]->[module/package2.FunctionC]->[original error message]"
	Unwrap() error
	// Fields returns the logger fields from the context of the root error (the lowest `logger.WrapError()` call on the call stack).
	// This preserves the logger context which will have logger fields added throughout the call stack down to where the error was created.
	Fields() Fields
}

// Error returns the error message as a string in the form of:
//
//	"[main.FunctionA]->[module/package1.FunctionB]->[module/package2.FunctionC]->[original error message]"
func (r customError) Error() string {
	var frames []string
	// add first frame to stack
	frames = append(frames, fmt.Sprintf("[%s]", r.function))
	// add all other frames to stack
	for e := errors.Unwrap(r); e != nil; e = errors.Unwrap(e) {
		if e, ok := e.(customError); ok {
			frames = append(frames, fmt.Sprintf("[%s]", e.function))
			continue
		}
		// this should be the final error in the stack as it is a standard Go error
		frames = append(frames, fmt.Sprintf("[%s]", e.Error()))
	}
	// join all frames with a "->"
	return strings.Join(frames, "->")
}

// Unwrap returns the underlying error. If wrapping has occurred it will take the shape of:
//
//	"[main.FunctionA]->[module/package1.FunctionB]->[module/package2.FunctionC]->[original error message]"
func (r customError) Unwrap() error {
	return r.cause
}

// Fields returns the logger fields from the context of the root error (the lowest `logger.WrapError()` call on the call stack).
// This preserves the logger context which will have logger fields added throughout the call stack down to where the error was created.
func (r customError) Fields() Fields {
	var calls []string

	// add first call to stack
	calls = append(calls, fmt.Sprintf("%s:%d", r.file, r.line))
	// start with fields of first error in wrapping chain
	fields := r.fields
	// add all other calls to stack (ignoring standard Go errors)
	for e := errors.Unwrap(r); e != nil; e = errors.Unwrap(e) {
		if e, ok := e.(customError); ok {
			calls = append(calls, fmt.Sprintf("%s:%d", e.file, e.line))
			fields = e.fields
		}
	}
	// add calls to fields
	fields["call_stack"] = calls
	return fields
}

// wrap creates a customError from a given standard Go error and logger fields. It pulls out the
// caller function name, file name, and line number from the runtime.
func wrap(e error, f Fields) Error {
	// get program counter, file, and line number from the function invocation
	pc, file, line, ok := runtime.Caller(2)
	// return nil if the information cannot be recovered
	if !ok {
		return nil
	}
	// convert program counter to function name
	functionStr := runtime.FuncForPC(pc).Name()
	// create custom error
	return customError{
		cause:    e,
		function: functionStr,
		fields:   f,
		file:     file,
		line:     line,
	}
}
