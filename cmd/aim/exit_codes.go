package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

const (
	exitSuccess     = 0
	exitUsage       = 64
	exitNoInput     = 66
	exitUnavailable = 69
	exitSoftware    = 70
	exitCantCreate  = 73
	exitTempFail    = 75
	exitNoPerm      = 77
)

type cliError struct {
	Code int
	Err  error
}

type displayedError struct {
	Err error
}

func (e *cliError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *cliError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *displayedError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *displayedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func usageError(err error) error {
	return withExitCode(exitUsage, err)
}

func notFoundError(err error) error {
	return withExitCode(exitNoInput, err)
}

func unavailableError(err error) error {
	return withExitCode(exitUnavailable, err)
}

func cantCreateError(err error) error {
	return withExitCode(exitCantCreate, err)
}

func tempFailError(err error) error {
	return withExitCode(exitTempFail, err)
}

func noPermError(err error) error {
	return withExitCode(exitNoPerm, err)
}

func softwareError(err error) error {
	return withExitCode(exitSoftware, err)
}

func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}

	var classified *cliError
	if errors.As(err, &classified) {
		if classified.Code == code {
			return err
		}
	}

	return &cliError{Code: code, Err: err}
}

func exitCodeForError(err error) int {
	if err == nil {
		return exitSuccess
	}

	var classified *cliError
	if errors.As(err, &classified) && classified != nil && classified.Code != 0 {
		return classified.Code
	}
	if errors.Is(err, context.Canceled) {
		return exitTempFail
	}

	if errors.Is(err, os.ErrPermission) {
		return exitNoPerm
	}
	if errors.Is(err, os.ErrNotExist) {
		return exitNoInput
	}
	if errors.Is(err, exec.ErrNotFound) {
		return exitUnavailable
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		switch {
		case errors.Is(pathErr.Err, os.ErrPermission):
			return exitNoPerm
		case errors.Is(pathErr.Err, os.ErrNotExist):
			return exitNoInput
		default:
			return exitCantCreate
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return exitTempFail
		}
		return exitUnavailable
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "confirmation required in non-interactive mode"),
		strings.Contains(msg, "confirmation required with --no-input"):
		return exitNoPerm
	case strings.Contains(msg, "permission denied"):
		return exitNoPerm
	case strings.Contains(msg, "missing required argument"),
		strings.Contains(msg, "unknown command"),
		strings.Contains(msg, "unknown flag"),
		strings.Contains(msg, "unknown shorthand flag"),
		strings.Contains(msg, "unknown help topic"),
		strings.Contains(msg, "too many arguments"),
		strings.Contains(msg, "mutually exclusive"),
		strings.Contains(msg, "--csv is not supported"),
		strings.Contains(msg, "not supported with"),
		strings.Contains(msg, "deprecated"),
		strings.Contains(msg, "must be a valid"),
		strings.Contains(msg, "unknown add target"),
		strings.Contains(msg, "unknown info target"),
		strings.Contains(msg, "unknown inspect target"),
		strings.Contains(msg, "unsupported package ref"),
		strings.Contains(msg, "unsupported package ref url"),
		strings.Contains(msg, "github repo must be in the form"),
		strings.Contains(msg, "gitlab project must be in the form"),
		strings.Contains(msg, "github package ref must be in the form"),
		strings.Contains(msg, "gitlab package ref must be in the form"),
		strings.Contains(msg, "provider ref"),
		strings.Contains(msg, "remote sources are added with"),
		strings.Contains(msg, "local appimages are added with"),
		strings.Contains(msg, "managed app ids are added with"),
		strings.Contains(msg, "direct urls must use https"),
		strings.Contains(msg, "missing update source"),
		strings.Contains(msg, "update source flags"):
		return exitUsage
	case strings.Contains(msg, "does not exists in database"),
		strings.Contains(msg, "no app with id"),
		strings.Contains(msg, "missing app"),
		strings.Contains(msg, "missing app exec path"):
		return exitNoInput
	case strings.Contains(msg, "download failed with status"),
		strings.Contains(msg, "installer script download failed with status"),
		strings.Contains(msg, "failed to resolve package metadata"),
		strings.Contains(msg, "no discovery backend available"),
		strings.Contains(msg, "missing zsync"),
		strings.Contains(msg, "missing zsync url"),
		strings.Contains(msg, "downloaded file sha256 mismatch"),
		strings.Contains(msg, "downloaded file sha1 mismatch"),
		strings.Contains(msg, "executable file not found"),
		strings.Contains(msg, "no such host"):
		return exitUnavailable
	case strings.Contains(msg, "failed to check updates"),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "temporar"):
		return exitTempFail
	case strings.Contains(msg, "failed to persist"),
		strings.Contains(msg, "failed to write"),
		strings.Contains(msg, "failed to create"),
		strings.Contains(msg, "failed to remove"),
		strings.Contains(msg, "failed to rename"),
		strings.Contains(msg, "failed to migrate config directory"),
		strings.Contains(msg, "failed to update"),
		strings.Contains(msg, "failed to verify"),
		strings.Contains(msg, "failed to load current database"):
		return exitCantCreate
	case strings.Contains(msg, "unsupported add provider"),
		strings.Contains(msg, "unsupported update source"),
		strings.Contains(msg, "unsupported embedded update info"),
		strings.Contains(msg, "unsupported add target"),
		strings.Contains(msg, "missing download url"),
		strings.Contains(msg, "invalid app slug"),
		strings.Contains(msg, "database source cannot be empty"):
		return exitSoftware
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.EACCES, syscall.EPERM:
			return exitNoPerm
		case syscall.ENOENT:
			return exitNoInput
		}
	}

	return exitSoftware
}

func renderCommandError(root *cobra.Command, args []string, err error) int {
	if err == nil {
		return exitSuccess
	}
	err = rewriteCommandError(root, args, err)

	var displayed *displayedError
	if errors.As(err, &displayed) {
		return exitCodeForError(err)
	}

	jsonOutput, _ := root.PersistentFlags().GetBool("json")
	if jsonOutput || argsContainFlag(args, "--json") {
		printJSONError(root.ErrOrStderr(), commandNameFromArgs(root, args), argsContainFlag(args, "--dry-run"), err)
	} else {
		debug, _ := root.PersistentFlags().GetBool("debug")
		verboseAlias, _ := root.PersistentFlags().GetBool("verbose")
		lines := renderTextErrorLines(err, suggestionForError(root, err), debug || verboseAlias || argsContainFlag(args, "--debug") || argsContainFlag(args, "-d") || argsContainFlag(args, "--verbose"))
		if opLog := operationLogForCommand(root); opLog != nil {
			entries := opLog.Lines()
			if len(entries) > 0 {
				lines = append(lines, "", "Operation log:")
				for _, entry := range entries {
					lines = append(lines, "  "+strings.TrimSpace(entry))
				}
			}
		}
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			_, _ = fmt.Fprintln(root.ErrOrStderr(), line)
		}
	}

	return exitCodeForError(err)
}

func markErrorDisplayed(err error) error {
	if err == nil {
		return nil
	}
	var displayed *displayedError
	if errors.As(err, &displayed) {
		return err
	}
	return &displayedError{Err: err}
}

func suggestionForError(root *cobra.Command, err error) string {
	if root == nil || err == nil {
		return ""
	}
	name := unknownCommandNameFromError(err)
	if name == "" || name == "upgrade" {
		return ""
	}
	return suggestedCommandName(root, name)
}

func rewriteCommandError(root *cobra.Command, args []string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return withUserMessage(tempFailError(err), "Interrupted.")
	}
	if unknownCommandNameFromError(err) == "upgrade" {
		return withUserGuidance(
			usageError(err),
			`unknown command "upgrade" for "aim"`,
			"Use 'aim --upgrade' to upgrade the aim CLI itself.",
		)
	}
	return err
}

func unknownCommandNameFromError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if !strings.HasPrefix(message, "unknown command ") {
		return ""
	}
	trimmed := strings.TrimPrefix(message, "unknown command ")
	idx := strings.Index(trimmed, `"`)
	if idx != 0 {
		return ""
	}
	trimmed = trimmed[1:]
	end := strings.Index(trimmed, `"`)
	if end < 0 {
		return ""
	}
	return trimmed[:end]
}

func formatExitStatusSection() string {
	lines := []string{
		"0 success",
		"64 invalid command usage",
		"66 requested local input or resource not found",
		"69 external service or tool unavailable",
		"70 internal or uncategorized software failure",
		"73 local write/create/update failure",
		"75 temporary or retryable runtime failure",
		"77 permission, confirmation, or precondition refusal",
	}
	return strings.Join(lines, "\n")
}

func isDatabaseMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exists in database") || strings.Contains(msg, "no app with id")
}

func wrapDatabaseReadError(err error) error {
	if err == nil {
		return nil
	}
	if isDatabaseMissingError(err) {
		return withUserMessage(notFoundError(err), "The requested managed app could not be found.")
	}
	return err
}

func wrapWriteError(err error) error {
	if err == nil {
		return nil
	}
	return rewriteWriteError(err)
}

func wrapManagedAppLookupError(id string, err error) error {
	if err == nil {
		return nil
	}
	if isDatabaseMissingError(err) {
		return rewriteMissingAppError(id, err)
	}
	return wrapDatabaseReadError(err)
}
