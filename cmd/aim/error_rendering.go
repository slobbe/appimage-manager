package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type userFacingError struct {
	Summary    string
	Detail     string
	Hint       string
	Reportable bool
	Err        error
}

func (e *userFacingError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Summary) != "" {
		return e.Summary
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return ""
}

func (e *userFacingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func withUserMessage(err error, summary string) error {
	return withUserMessageDetails(err, summary, "", false)
}

func withUserGuidance(err error, summary, hint string) error {
	return withUserMessageDetails(err, summary, hint, false)
}

func withReportableInternalError(err error, summary string) error {
	return withUserMessageDetails(softwareError(err), summary, "", true)
}

func withUserMessageDetails(err error, summary, hint string, reportable bool) error {
	if err == nil {
		return nil
	}

	detail := rawErrorMessage(err)
	var existing *userFacingError
	if errors.As(err, &existing) && existing != nil {
		if strings.TrimSpace(summary) == "" {
			summary = existing.Summary
		}
		if strings.TrimSpace(hint) == "" {
			hint = existing.Hint
		}
		if strings.TrimSpace(existing.Detail) != "" {
			detail = existing.Detail
		}
		reportable = reportable || existing.Reportable
	}

	return &userFacingError{
		Summary:    strings.TrimSpace(summary),
		Detail:     strings.TrimSpace(detail),
		Hint:       strings.TrimSpace(hint),
		Reportable: reportable,
		Err:        err,
	}
}

func userMessageForError(err error) userFacingError {
	if err == nil {
		return userFacingError{}
	}

	var userErr *userFacingError
	if errors.As(err, &userErr) && userErr != nil {
		out := *userErr
		if strings.TrimSpace(out.Summary) == "" {
			out.Summary = rawErrorMessage(err)
		}
		if strings.TrimSpace(out.Detail) == "" {
			out.Detail = rawErrorMessage(err)
		}
		if exitCodeForError(err) == exitSoftware {
			out.Reportable = true
		}
		if out.Reportable && strings.TrimSpace(out.Hint) == "" {
			out.Hint = "Rerun with --verbose to include more diagnostic detail."
		}
		return out
	}

	summary := strings.TrimSpace(err.Error())
	out := userFacingError{
		Summary:    summary,
		Detail:     rawErrorMessage(err),
		Reportable: exitCodeForError(err) == exitSoftware,
	}
	if out.Reportable {
		out.Hint = "Rerun with --verbose to include more diagnostic detail."
	}
	return out
}

func rawErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	current := err
	last := strings.TrimSpace(err.Error())
	for current != nil {
		switch unwrapped := errors.Unwrap(current); {
		case unwrapped == nil:
			return last
		default:
			current = unwrapped
			text := strings.TrimSpace(current.Error())
			if text != "" {
				last = text
			}
		}
	}
	return last
}

func renderTextErrorLines(err error, suggestion string, verbose bool) []string {
	userErr := userMessageForError(err)
	lines := []string{userErr.Summary}
	if strings.TrimSpace(userErr.Hint) != "" && !userErr.Reportable {
		lines = append(lines, userErr.Hint)
	}
	if strings.TrimSpace(suggestion) != "" {
		lines = append(lines, fmt.Sprintf("Did you mean %q?", suggestion))
	}
	if verbose {
		detail := strings.TrimSpace(userErr.Detail)
		if detail != "" && detail != strings.TrimSpace(userErr.Summary) {
			lines = append(lines, "Details: "+detail)
		}
	}
	if userErr.Reportable {
		lines = append(lines,
			"This looks like an internal aim error.",
			"Rerun with --verbose to include more diagnostic detail.",
			"Report it: "+rootCommandIssuesURL,
		)
	}
	return lines
}

func rewriteWriteError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, os.ErrPermission) {
		if path := pathFromError(err); path != "" {
			return withUserGuidance(
				noPermError(err),
				fmt.Sprintf("Can't write to %s.", path),
				fmt.Sprintf("Check directory permissions for %s or rerun with -C to use a writable AIM state root.", path),
			)
		}
		return withUserGuidance(
			noPermError(err),
			"Can't write AIM state on disk.",
			"Check directory permissions or rerun with -C to use a writable AIM state root.",
		)
	}

	if path := pathFromError(err); path != "" {
		return withUserGuidance(
			cantCreateError(err),
			fmt.Sprintf("Can't write AIM state at %s.", path),
			"Check that the destination exists and is writable, or rerun with -C to use a different AIM state root.",
		)
	}

	return withUserGuidance(
		cantCreateError(err),
		"Can't update AIM state on disk.",
		"Check that the target directories are writable, or rerun with -C to use a different AIM state root.",
	)
}

func rewriteMissingAppError(id string, err error) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return notFoundError(err)
	}
	return withUserGuidance(
		notFoundError(err),
		fmt.Sprintf("No managed app with id %q.", trimmed),
		"Run 'aim list' to see installed app IDs.",
	)
}

func rewriteDependencyError(err error, summary, hint string) error {
	return withUserGuidance(unavailableError(err), summary, hint)
}

func rewriteChecksumError(err error) error {
	return withUserGuidance(
		unavailableError(err),
		"Downloaded update failed integrity verification.",
		"The remote file does not match the expected checksum. Try again later or verify the configured update source.",
	)
}

func rewriteBatchCheckFailure(appID string, err error) string {
	msg := strings.TrimSpace(userMessageForError(err).Summary)
	msg = strings.TrimPrefix(msg, "Can't ")
	msg = strings.TrimPrefix(msg, "can't ")
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "update check failed"
	}
	if strings.HasSuffix(msg, ".") {
		msg = strings.TrimSuffix(msg, ".")
	}
	if strings.TrimSpace(appID) == "" {
		return msg
	}
	return fmt.Sprintf("%s: %s", appID, msg)
}

func pathFromError(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr != nil {
		return strings.TrimSpace(pathErr.Path)
	}
	return ""
}

func rewriteNetworkDownloadError(err error) error {
	if err == nil {
		return nil
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) {
		return withUserGuidance(
			tempFailError(err),
			"Can't reach the update server.",
			"Check your network connection and try again.",
		)
	}
	return withUserGuidance(
		unavailableError(err),
		"Can't reach the update server.",
		"Check your network connection and try again.",
	)
}

func rewriteZsyncFailure(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return withUserGuidance(
			unavailableError(err),
			"Can't apply a delta update because 'zsync' is not installed.",
			"Install zsync or switch this app to a non-zsync update source with 'aim update set'.",
		)
	}
	return withUserGuidance(
		unavailableError(err),
		"Delta update failed while running 'zsync'.",
		"Rerun with --verbose for the raw zsync error, or switch this app to a non-zsync update source with 'aim update set'.",
	)
}
