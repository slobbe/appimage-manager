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

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/spf13/cobra"
)

type runtimeOptions struct {
	ConfigRoot string
	DryRun     bool
	Yes        bool
	Output     string
	Verbose    bool
	Quiet      bool
}

type runtimeContextKey struct{}

type commandJSONEnvelope struct {
	Command string      `json:"command"`
	OK      bool        `json:"ok"`
	DryRun  bool        `json:"dry_run"`
	Result  interface{} `json:"result,omitempty"`
	Error   string      `json:"error,omitempty"`
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

	if opts.Verbose {
		writeLogf(cmd, "DEBUG: Using AIM paths: data=%s db=%s temp=%s\n", config.AimDir, config.DbSrc, config.TempDir)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, runtimeContextKey{}, opts)
	cmd.SetContext(ctx)
	return nil
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
	opts.Output, err = flagString(cmd, "output")
	if err != nil {
		return opts, err
	}
	if opts.Output == "" {
		opts.Output = outputText
	}
	opts.Output = strings.ToLower(opts.Output)
	opts.Verbose, err = flagBool(cmd, "verbose")
	if err != nil {
		return opts, err
	}
	opts.Quiet, err = flagBool(cmd, "quiet")
	if err != nil {
		return opts, err
	}

	return opts, nil
}

func validateRuntimeOptions(cmd *cobra.Command, opts runtimeOptions) error {
	if opts.Verbose && opts.Quiet {
		return usageError(fmt.Errorf("--verbose and --quiet are mutually exclusive"))
	}

	switch opts.Output {
	case outputText, outputJSON, outputCSV:
	default:
		return usageError(fmt.Errorf("unsupported output format %q; use text, json, or csv", opts.Output))
	}

	if opts.Output == outputCSV && !commandSupportsCSV(cmd) {
		return usageError(fmt.Errorf("--output csv is not supported for `%s`", strings.TrimSpace(cmd.CommandPath())))
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
		return runtimeOptions{Output: outputText}
	}
	if value, ok := cmd.Context().Value(runtimeContextKey{}).(runtimeOptions); ok {
		return value
	}
	return runtimeOptions{Output: outputText}
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

const (
	outputText = "text"
	outputJSON = "json"
	outputCSV  = "csv"
)

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
	envelope := commandJSONEnvelope{
		Command: command,
		OK:      false,
		DryRun:  dryRun,
		Error:   err.Error(),
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
	return runtimeOptionsFrom(cmd).Output != outputText
}

func shouldRenderLogs(cmd *cobra.Command) bool {
	opts := runtimeOptionsFrom(cmd)
	return opts.Output == outputText
}

func verbosef(cmd *cobra.Command, format string, args ...interface{}) {
	opts := runtimeOptionsFrom(cmd)
	if !opts.Verbose {
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
var terminalInputChecker = detectTerminalInput

func isTerminalOutput() bool {
	return terminalOutputChecker()
}

func detectTerminalOutput() bool {
	stat, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func detectTerminalInput() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
