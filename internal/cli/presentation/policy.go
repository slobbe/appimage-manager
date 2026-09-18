package presentation

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

const (
	ColorAuto   = "auto"
	ColorAlways = "always"
	ColorNever  = "never"
)

// Policy is the single source of truth for terminal-dependent presentation.
type Policy struct {
	noColor bool
	isTTY   func(io.Writer) bool
}

func NewPolicy() Policy {
	_, noColor := os.LookupEnv("NO_COLOR")
	return Policy{
		noColor: noColor,
		isTTY:   writerIsTTY,
	}
}

func ValidateColorMode(mode string) error {
	switch mode {
	case ColorAuto, ColorAlways, ColorNever:
		return nil
	default:
		return fmt.Errorf("invalid color mode %q: must be auto, always, or never", mode)
	}
}

func (p Policy) ColorEnabled(w io.Writer, mode string, jsonOutput bool) bool {
	if jsonOutput {
		return false
	}
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default:
		return !p.noColor && p.isTTY(w)
	}
}

func (p Policy) ActivityEnabled(w io.Writer, jsonOutput bool) bool {
	return !jsonOutput && p.isTTY(w)
}

func (p Policy) Success(w io.Writer, mode string, jsonOutput bool, text string) string {
	if !p.ColorEnabled(w, mode, jsonOutput) {
		return text
	}
	return "\033[32m" + text + "\033[0m"
}

func (p Policy) Emphasis(w io.Writer, mode string, jsonOutput bool, text string) string {
	if !p.ColorEnabled(w, mode, jsonOutput) {
		return text
	}
	return "\033[1m" + text + "\033[0m"
}

func writerIsTTY(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetTermios(int(file.Fd()), unix.TCGETS)
	return err == nil
}
