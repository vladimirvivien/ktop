package ui

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

// ErrClipboardUnavailable indicates no supported clipboard tool is available
var ErrClipboardUnavailable = errors.New("no clipboard tool available")

// CopyToClipboard copies the given text to the system clipboard using native platform utilities.
// Supports macOS (pbcopy), Windows (clip), and Linux (wl-copy, xclip, xsel).
func CopyToClipboard(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("pbcopy"); err != nil {
			return ErrClipboardUnavailable
		}
		cmd = exec.CommandContext(ctx, "pbcopy")
	case "windows":
		if _, err := exec.LookPath("clip.exe"); err == nil {
			cmd = exec.CommandContext(ctx, "clip.exe")
		} else if _, err := exec.LookPath("clip"); err == nil {
			cmd = exec.CommandContext(ctx, "clip")
		} else {
			return ErrClipboardUnavailable
		}
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.CommandContext(ctx, "wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.CommandContext(ctx, "xsel", "--clipboard", "--input")
		} else {
			return ErrClipboardUnavailable
		}
	}

	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := in.Write([]byte(text)); err != nil {
		_ = in.Close()
		return err
	}
	_ = in.Close()

	return cmd.Wait()
}
