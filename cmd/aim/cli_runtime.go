package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/core"
	"github.com/slobbe/appimage-manager/internal/discovery"
	"github.com/spf13/cobra"
)

type runtimeOptions struct {
	ConfigRoot string
	DryRun     bool
	Yes        bool
	NoInput    bool
	JSON       bool
	CSV        bool
	Plain      bool
	NoColor    bool
	Debug      bool
	Quiet      bool
}

type runtimeContextKey struct{}
type runtimeSettingsContextKey struct{}

type runtimeSettings struct {
	NetworkTimeout time.Duration
}

type commandJSONEnvelope struct {
	Command     string      `json:"command"`
	OK          bool        `json:"ok"`
	DryRun      bool        `json:"dry_run"`
	Result      interface{} `json:"result,omitempty"`
	Error       string      `json:"error,omitempty"`
	Hint        string      `json:"hint,omitempty"`
	ReportIssue bool        `json:"report_issue,omitempty"`
	IssuesURL   string      `json:"issues_url,omitempty"`
}

func prepareRuntime(cmd *cobra.Command) error {
	opts, err := parseRuntimeOptions(cmd)
	if err != nil {
		return err
	}
	if err := validateRuntimeOptions(cmd, opts); err != nil {
		return err
	}
	applyRuntimePaths(opts)
	settings, err := loadRuntimeSettings()
	if err != nil {
		return err
	}
	core.SetHTTPClientTimeout(settings.NetworkTimeout)
	discovery.SetHTTPClientTimeout(settings.NetworkTimeout)

	if opts.Debug {
		writeLogf(cmd, "DEBUG: Using AIM paths: data=%s db=%s temp=%s config=%s timeout=%s\n", config.AimDir, config.DbSrc, config.TempDir, config.ConfigDir, settings.NetworkTimeout)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = withOperationLog(ctx)
	ctx = context.WithValue(ctx, runtimeContextKey{}, opts)
	ctx = context.WithValue(ctx, runtimeSettingsContextKey{}, settings)
	cmd.SetContext(ctx)
	return nil
}

func loadRuntimeSettings() (runtimeSettings, error) {
	settings, err := config.LoadSettings()
	if err != nil {
		return runtimeSettings{}, usageError(fmt.Errorf("invalid settings file %s: %w", config.SettingsPath(), err))
	}
	return runtimeSettings{
		NetworkTimeout: settings.NetworkTimeout,
	}, nil
}

func parseRuntimeOptions(cmd *cobra.Command) (runtimeOptions, error) {
	var opts runtimeOptions
	var err error

	opts.ConfigRoot, err = flagString(cmd, "config")
	if err != nil {
		return opts, err
	}
	opts.ConfigRoot, err = normalizeConfigRoot(opts.ConfigRoot)
	if err != nil {
		return opts, err
	}
	opts.DryRun, err = flagBool(cmd, "dry-run")
	if err != nil {
		return opts, err
	}
	opts.Yes, err = flagBool(cmd, "yes")
	if err != nil {
		return opts, err
	}
	opts.NoInput, err = flagBool(cmd, "no-input")
	if err != nil {
		return opts, err
	}
	opts.JSON, err = flagBool(cmd, "json")
	if err != nil {
		return opts, err
	}
	opts.CSV, err = flagBool(cmd, "csv")
	if err != nil {
		return opts, err
	}
	opts.Plain, err = flagBool(cmd, "plain")
	if err != nil {
		return opts, err
	}
	opts.NoColor, err = flagBool(cmd, "no-color")
	if err != nil {
		return opts, err
	}
	debug, err := flagBool(cmd, "debug")
	if err != nil {
		return opts, err
	}
	verboseAlias, err := flagBool(cmd, "verbose")
	if err != nil {
		return opts, err
	}
	opts.Debug = debug || verboseAlias
	opts.Quiet, err = flagBool(cmd, "quiet")
	if err != nil {
		return opts, err
	}

	return opts, nil
}

func validateRuntimeOptions(cmd *cobra.Command, opts runtimeOptions) error {
	if opts.Debug && opts.Quiet {
		return usageError(fmt.Errorf("--debug and --quiet are mutually exclusive"))
	}

	if opts.JSON && opts.CSV {
		return usageError(fmt.Errorf("--json and --csv are mutually exclusive"))
	}
	if opts.Plain && opts.JSON {
		return usageError(fmt.Errorf("--plain and --json are mutually exclusive"))
	}
	if opts.Plain && opts.CSV {
		return usageError(fmt.Errorf("--plain and --csv are mutually exclusive"))
	}
	if opts.CSV && !commandSupportsCSV(cmd) {
		return usageError(fmt.Errorf("--csv is not supported for `%s`", strings.TrimSpace(cmd.CommandPath())))
	}

	return nil
}

func applyRuntimePaths(opts runtimeOptions) {
	if strings.TrimSpace(opts.ConfigRoot) == "" {
		return
	}

	config.ApplyPaths(config.ResolvePathsFromStateRoot(opts.ConfigRoot))
}

func runtimeOptionsFrom(cmd *cobra.Command) runtimeOptions {
	if cmd == nil || cmd.Context() == nil {
		return runtimeOptions{}
	}
	if value, ok := cmd.Context().Value(runtimeContextKey{}).(runtimeOptions); ok {
		return value
	}
	return runtimeOptions{}
}

func runtimePrepared(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Context() == nil {
		return false
	}
	_, ok := cmd.Context().Value(runtimeContextKey{}).(runtimeOptions)
	return ok
}

func runtimeSettingsFrom(cmd *cobra.Command) runtimeSettings {
	if cmd == nil || cmd.Context() == nil {
		return runtimeSettings{NetworkTimeout: config.DefaultSettings().NetworkTimeout}
	}
	if value, ok := cmd.Context().Value(runtimeSettingsContextKey{}).(runtimeSettings); ok {
		return value
	}
	return runtimeSettings{NetworkTimeout: config.DefaultSettings().NetworkTimeout}
}

func mustEnsureRuntimeDirs() error {
	if err := config.EnsureDirsExist(); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return noPermError(err)
		}
		return cantCreateError(err)
	}
	return nil
}

func normalizeConfigRoot(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}

	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func commandSupportsCSV(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	switch strings.TrimSpace(commandName(cmd)) {
	case "list", "update":
		return true
	default:
		return false
	}
}

func commandName(cmd *cobra.Command) string {
	if cmd == nil {
		return "aim"
	}

	path := strings.TrimSpace(cmd.CommandPath())
	if path == "" || path == "aim" {
		return "aim"
	}
	return strings.TrimSpace(strings.TrimPrefix(path, "aim "))
}

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

func shouldUseStructuredOutput(cmd *cobra.Command) bool {
	opts := runtimeOptionsFrom(cmd)
	return opts.JSON || opts.CSV
}

func shouldRenderLogs(cmd *cobra.Command) bool {
	return !shouldUseStructuredOutput(cmd)
}

func verbosef(cmd *cobra.Command, format string, args ...interface{}) {
	opts := runtimeOptionsFrom(cmd)
	if !opts.Debug {
		return
	}
	writeLogf(cmd, "DEBUG: "+format+"\n", args...)
}

func printPrompt(cmd *cobra.Command, prompt string) {
	writeLogf(cmd, "%s", prompt)
}

func confirmAction(cmd *cobra.Command, prompt string) (bool, error) {
	opts := runtimeOptionsFrom(cmd)
	if opts.Yes {
		return true, nil
	}
	if opts.DryRun {
		return true, nil
	}
	if opts.NoInput {
		return false, noPermError(fmt.Errorf("confirmation required with --no-input; rerun with --yes to continue non-interactively"))
	}
	if !terminalInputChecker() {
		return false, noPermError(fmt.Errorf("confirmation required in non-interactive mode; rerun with --yes"))
	}

	printPrompt(cmd, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return false, noPermError(err)
		}
		return false, softwareError(err)
	}

	answer := strings.TrimSpace(line)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

var terminalOutputChecker = detectTerminalOutput
var terminalStdoutChecker = detectTerminalStdout
var terminalStderrChecker = detectTerminalStderr
var terminalInputChecker = detectTerminalInput

func isTerminalStdout() bool {
	return terminalStdoutChecker()
}

func isTerminalStderr() bool {
	return terminalStderrChecker()
}

func detectTerminalOutput() bool {
	return detectTerminalStderr()
}

func detectTerminalStdout() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func detectTerminalStderr() bool {
	stat, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func colorsDisabledByEnv() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv("AIM_NO_COLOR")) != "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb")
}

func streamSupportsColor(isTTY bool, opts runtimeOptions) bool {
	return isTTY && !opts.NoColor && !colorsDisabledByEnv()
}

func shouldColorStdout(cmd *cobra.Command) bool {
	return streamSupportsColor(isTerminalStdout(), runtimeOptionsFrom(cmd))
}

func shouldColorStderr(cmd *cobra.Command) bool {
	return streamSupportsColor(isTerminalStderr(), runtimeOptionsFrom(cmd))
}

func detectTerminalInput() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
