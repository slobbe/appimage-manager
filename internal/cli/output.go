package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	appservices "github.com/slobbe/appimage-manager/internal/app/services"
	"github.com/spf13/cobra"
	"io"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
)

func dataWriter(cmd *cobra.Command) io.Writer {
	return cmd.OutOrStdout()
}

func logWriter(cmd *cobra.Command) io.Writer {
	return cmd.ErrOrStderr()
}

func writeDataf(cmd *cobra.Command, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(dataWriter(cmd), format, args...)
}

func writeLogf(cmd *cobra.Command, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(logWriter(cmd), format, args...)
}

func writeProcessLogf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format, args...)
}

func printJSONSuccess(cmd *cobra.Command, result interface{}) error {
	opts := runtimeOptionsFrom(cmd)
	envelope := commandJSONEnvelope{
		Command: commandName(cmd),
		OK:      true,
		DryRun:  opts.DryRun,
		Result:  result,
	}

	encoder := json.NewEncoder(dataWriter(cmd))
	encoder.SetIndent("", "  ")
	return encoder.Encode(envelope)
}

func printJSONError(writer io.Writer, command string, dryRun bool, err error) {
	userErr := userMessageForError(err)
	envelope := commandJSONEnvelope{
		Command: command,
		OK:      false,
		DryRun:  dryRun,
		Error:   userErr.Summary,
		Hint:    userErr.Hint,
	}
	if userErr.Reportable {
		envelope.ReportIssue = true
		envelope.IssuesURL = rootCommandIssuesURL
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(envelope)
}

func writeCSV(cmd *cobra.Command, header []string, rows [][]string) error {
	w := csv.NewWriter(dataWriter(cmd))
	if err := w.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

type listOutputRow struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Integrated      bool   `json:"integrated"`
	ExecPath        string `json:"exec_path"`
	UpdateAvailable bool   `json:"update_available"`
	LatestVersion   string `json:"latest_version,omitempty"`
	LastCheckedAt   string `json:"last_checked_at,omitempty"`
}

type updateOutputRow struct {
	ID              string `json:"id"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Status          string `json:"status"`
	DownloadURL     string `json:"download_url,omitempty"`
	Asset           string `json:"asset,omitempty"`
	SourceKind      string `json:"source_kind,omitempty"`
	LastCheckedAt   string `json:"last_checked_at,omitempty"`
}

func newListOutputRow(app *appservices.AppDetails) listOutputRow {
	if app == nil {
		return listOutputRow{}
	}
	return listOutputRow{
		ID:              app.ID,
		Name:            app.Name,
		Version:         app.Version,
		Integrated:      app.Integrated,
		ExecPath:        app.ExecPath,
		UpdateAvailable: app.UpdateAvailable,
		LatestVersion:   app.LatestVersion,
		LastCheckedAt:   app.LastCheckedAt,
	}
}

func (row listOutputRow) csvRow() []string {
	return []string{
		row.ID,
		row.Name,
		row.Version,
		boolString(row.Integrated),
		row.ExecPath,
		boolString(row.UpdateAvailable),
		row.LatestVersion,
		row.LastCheckedAt,
	}
}

func listCSVHeader() []string {
	return []string{"id", "name", "version", "integrated", "exec_path", "update_available", "latest_version", "last_checked_at"}
}

func newUpdateOutputRow(app *appservices.AppSummary, update *appservices.ManagedUpdateView, status, checkedAt string) updateOutputRow {
	row := updateOutputRow{
		Status:        status,
		LastCheckedAt: checkedAt,
	}
	if app != nil {
		row.ID = app.ID
		row.CurrentVersion = app.Version
		row.LastCheckedAt = app.LastCheckedAt
	}
	if update != nil {
		row.LatestVersion = update.Latest
		row.UpdateAvailable = update.Available && update.URL != ""
		row.DownloadURL = update.URL
		row.Asset = update.Asset
		row.SourceKind = update.FromKind
	}
	return row
}

func (row updateOutputRow) csvRow() []string {
	return []string{
		row.ID,
		row.CurrentVersion,
		row.LatestVersion,
		boolString(row.UpdateAvailable),
		row.Status,
		row.DownloadURL,
		row.Asset,
		row.SourceKind,
		row.LastCheckedAt,
	}
}

func updateCSVHeader() []string {
	return []string{"id", "current_version", "latest_version", "update_available", "status", "download_url", "asset", "source_kind", "last_checked_at"}
}

func (row listOutputRow) plainRow() []string {
	return []string{
		row.ID,
		row.Name,
		row.Version,
		boolString(row.Integrated),
		row.ExecPath,
	}
}

func listPlainHeader() []string {
	return []string{"id", "name", "version", "integrated", "exec_path"}
}

func (row updateOutputRow) plainRow() []string {
	return []string{
		row.ID,
		row.CurrentVersion,
		row.LatestVersion,
		row.Status,
		row.SourceKind,
	}
}

func updatePlainHeader() []string {
	return []string{"id", "current_version", "latest_version", "status", "source_kind"}
}

func appDetailsByID(apps []*appservices.AppDetails) map[string]*appservices.AppDetails {
	byID := make(map[string]*appservices.AppDetails, len(apps))
	for _, app := range apps {
		if app != nil {
			byID[app.ID] = app
		}
	}
	return byID
}

func sortAppDetailsByID(apps map[string]*appservices.AppDetails) []*appservices.AppDetails {
	ids := make([]string, 0, len(apps))
	for id := range apps {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	ordered := make([]*appservices.AppDetails, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, apps[id])
	}
	return ordered
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func writePlainRows(cmd *cobra.Command, header []string, rows [][]string) {
	writeDataf(cmd, "%s\n", strings.Join(header, "\t"))
	for _, row := range rows {
		writeDataf(cmd, "%s\n", strings.Join(row, "\t"))
	}
}

func writePlainList(cmd *cobra.Command, apps []*appservices.AppDetails) {
	rows := make([][]string, 0, len(apps))
	for _, app := range apps {
		rows = append(rows, newListOutputRow(app).plainRow())
	}
	writePlainRows(cmd, listPlainHeader(), rows)
}

func writePlainUpdateRows(cmd *cobra.Command, rows []updateOutputRow) {
	plainRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		plainRows = append(plainRows, row.plainRow())
	}
	writePlainRows(cmd, updatePlainHeader(), plainRows)
}

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
			out.Hint = "Rerun with --debug to include more diagnostic detail."
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
		out.Hint = "Rerun with --debug to include more diagnostic detail."
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
			"Rerun with --debug to include more diagnostic detail.",
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
				fmt.Sprintf("Check directory permissions for %s or make the AIM state directories writable.", path),
			)
		}
		return withUserGuidance(
			noPermError(err),
			"Can't write AIM state on disk.",
			"Check directory permissions or make the AIM state directories writable.",
		)
	}

	if path := pathFromError(err); path != "" {
		return withUserGuidance(
			cantCreateError(err),
			fmt.Sprintf("Can't write AIM state at %s.", path),
			"Check that the destination exists and is writable.",
		)
	}

	return withUserGuidance(
		cantCreateError(err),
		"Can't update AIM state on disk.",
		"Check that the target directories are writable.",
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
			"Install zsync or switch this app to a non-zsync update source with 'aim update --set <id> ...'.",
		)
	}
	return withUserGuidance(
		unavailableError(err),
		"Delta update failed while running 'zsync'.",
		"Rerun with --debug for the raw zsync error, or switch this app to a non-zsync update source with 'aim update --set <id> ...'.",
	)
}

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
		strings.Contains(msg, "must be a valid"),
		strings.Contains(msg, "unknown add target"),
		strings.Contains(msg, "unknown info target"),
		strings.Contains(msg, "unknown inspect target"),
		strings.Contains(msg, "unsupported package ref"),
		strings.Contains(msg, "unsupported package ref url"),
		strings.Contains(msg, "github repo must be in the form"),
		strings.Contains(msg, "github package ref must be in the form"),
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
		lines := renderTextErrorLines(err, suggestionForError(root, err), debug || argsContainFlag(args, "--debug") || argsContainFlag(args, "-d"))
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
