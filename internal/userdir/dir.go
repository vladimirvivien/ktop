// Package userdir resolves and provisions the per-user ktop directory
// (default ~/.ktop), where ktop stores its log file and, from v0.6.3,
// its config file. Honor $KTOP_DIR to relocate the entire tree.
package userdir

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvVar is the environment variable that overrides the default location.
const EnvVar = "KTOP_DIR"

const dirMode = 0o700

// Path returns the resolved ktop directory path. It does not create the
// directory; use Ensure for that.
//
// Resolution order:
//  1. $KTOP_DIR if set and non-empty
//  2. $HOME/.ktop
func Path() (string, error) {
	if dir := os.Getenv(EnvVar); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home directory unavailable: %w", err)
	}
	return filepath.Join(home, ".ktop"), nil
}

// Ensure resolves the ktop directory and creates it (mode 0700) if missing.
// Returns the resolved path on success.
func Ensure() (string, error) {
	dir, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	return dir, nil
}
