package cmd

import (
	"context"
	"errors"
	"net"

	"github.com/vladimirvivien/ktop/collector"
)

// Operational exit codes for ktop.
const (
	ExitOK                 = 0
	ExitGeneralError       = 1
	ExitInvalidFlags       = 2
	ExitK8sConnectError    = 3
	ExitMetricsUnavailable = 4
	ExitCollectionTimeout  = 5
)

// ExitError associates an operational exit code with a returned error.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WrapExitError attaches code to err unless err already carries an *ExitError.
func WrapExitError(code int, err error) error {
	if err == nil {
		return nil
	}
	var existing *ExitError
	if errors.As(err, &existing) {
		return err
	}
	return &ExitError{Code: code, Err: err}
}

// ExitCode maps a command error to the documented ktop process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	if isTimeoutError(err) {
		return ExitCollectionTimeout
	}
	return ExitGeneralError
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, collector.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
