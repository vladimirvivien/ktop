package application

import "context"

// Runner is the top-level runtime contract implemented by interactive TUI and
// headless application modes.
type Runner interface {
	Run(ctx context.Context) error
	Stop() error
}

// Compile-time check that *Application implements Runner.
var _ Runner = (*Application)(nil)
