package cli

import (
	"context"
	"errors"
	"fmt"
	appservices "github.com/slobbe/appimage-manager/internal/app/services"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/config"
	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type runtimeOptions struct {
	DryRun  bool
	Yes     bool
	NoInput bool
	JSON    bool
	CSV     bool
	Plain   bool
	NoColor bool
	Debug   bool
	Quiet   bool
}

type runtimeContextKey struct{}
type runtimeSettingsContextKey struct{}

type runtimeSettings struct {
	NetworkTimeout time.Duration
}

func formatAppRef(app *domain.App) string {
	if app == nil {
		return "unknown"
	}
	return fmt.Sprintf("%s %s [%s]", app.Name, displayVersion(app.Version), app.ID)
}

func verifyDownloadedUpdate(downloadPath string, update appupdate.ManagedUpdate) error {
	service := appupdate.NewService(appupdate.Service{HashVerifier: hashVerifierAdapter{}})
	return rewriteChecksumError(service.VerifyDownloadedUpdate(downloadPath, update))
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
	settings, err := loadRuntimeSettings()
	if err != nil {
		return err
	}
	setRuntimeDownloadTimeout(settings.NetworkTimeout)

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
	if _, ok := ctx.Value(runtimeServicesContextKey{}).(runtimeServices); !ok {
		ctx = withRuntimeServices(ctx, defaultRuntimeServicesForSettings(settings))
	}
	cmd.SetContext(ctx)
	return nil
}

func runtimePathsFromConfig(paths config.Paths) appservices.RuntimePaths {
	return appservices.RuntimePaths{
		AimDir:       paths.AimDir,
		DesktopDir:   paths.DesktopDir,
		TempDir:      paths.TempDir,
		IconThemeDir: paths.IconThemeDir,
	}
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
	opts.Debug = debug
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

type runtimeServicesContextKey struct{}

type runtimeServices struct {
	Add                   appservices.AddService
	List                  appservices.ListService
	Info                  appservices.InfoService
	Remove                appservices.RemoveService
	Update                appservices.UpdateService
	SelfUpdate            appservices.SelfUpdateService
	Discovery             appservices.DiscoveryService
	Locker                appservices.StateLocker
	ManagedAppCompletions func(string) ([]appservices.ManagedAppCompletion, error)
}

type updateSourceReplaceConfirmerFunc func(existing, incoming *appservices.UpdateSource) (bool, error)

func (fn updateSourceReplaceConfirmerFunc) ConfirmUpdateSourceReplace(existing, incoming *appservices.UpdateSource) (bool, error) {
	return fn(existing, incoming)
}

func defaultRuntimeServices() runtimeServices {
	return defaultRuntimeServicesForSettings(runtimeSettings{NetworkTimeout: 30 * time.Second})
}

func defaultRuntimeServicesForSettings(settings runtimeSettings) runtimeServices {
	locker := stateFileLocker{}
	workflows := appservices.NewDefaultWorkflowServices(appservices.DefaultWorkflowOptions{
		DBPath:    config.DbSrc,
		Paths:     runtimePathsFromConfig(config.CurrentPaths()),
		APIClient: httpclient.New(settings.NetworkTimeout),
		NowISO:    runtimeNowISO,
		Locker:    locker,
		RemoteInstallDownload: func(ctx context.Context, assetURL, destination string) error {
			if progress := remoteInstallProgressFromContext(ctx); progress != nil {
				progress.Stop()
				description := progress.DownloadDescription(downloadDescriptionFromContext(ctx, "Downloading update"))
				ctx = withDownloadDescription(ctx, description)
			}
			return downloadUpdateAsset(ctx, assetURL, destination, isTerminalStderr())
		},
		BeforeRemoteInstallIntegrate: func(ctx context.Context) {
			if progress := remoteInstallProgressFromContext(ctx); progress != nil {
				progress.StartIntegrating()
			}
		},
	})
	return runtimeServices{
		Add:        workflows.Add,
		List:       workflows.List,
		Info:       workflows.Info,
		Remove:     workflows.Remove,
		Update:     workflows.Update,
		SelfUpdate: workflows.SelfUpdate,
		Discovery:  workflows.Discovery,
		Locker:     workflows.Locker,
		ManagedAppCompletions: func(prefix string) ([]appservices.ManagedAppCompletion, error) {
			return appservices.NewDefaultManagedAppCompletions(config.DbSrc, prefix)
		},
	}
}

func withRuntimeServices(ctx context.Context, services runtimeServices) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeServicesContextKey{}, services)
}

func runtimeServicesFrom(cmd *cobra.Command) runtimeServices {
	if cmd != nil && cmd.Context() != nil {
		if services, ok := cmd.Context().Value(runtimeServicesContextKey{}).(runtimeServices); ok {
			return services
		}
	}
	return defaultRuntimeServices()
}

func installRuntimeServicesForTest(cmd *cobra.Command, services runtimeServices) {
	ctx := context.Background()
	if cmd.Context() != nil {
		ctx = cmd.Context()
	}
	cmd.SetContext(withRuntimeServices(ctx, services))
}

func withStateWriteLock(cmd *cobra.Command, fn func() error) error {
	return runtimeServicesFrom(cmd).Locker.WithWriteLock(fn)
}

type stateFileLocker struct{}

func (stateFileLocker) WithWriteLock(fn func() error) error {
	if err := os.MkdirAll(config.ConfigDir, 0o755); err != nil {
		return wrapWriteError(err)
	}

	lockPath := filepath.Join(config.ConfigDir, "state.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return wrapWriteError(err)
	}
	defer file.Close()

	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return noPermError(fmt.Errorf("another aim process is already modifying this AIM state root; wait for it to finish and try again"))
	}
	defer func() {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
	}()

	return fn()
}

type operationLogKey struct{}

type operationLogBuffer struct {
	mu    sync.Mutex
	lines []string
}

func withOperationLog(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if operationLogFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, operationLogKey{}, &operationLogBuffer{})
}

func operationLogFromContext(ctx context.Context) *operationLogBuffer {
	if ctx == nil {
		return nil
	}
	if value, ok := ctx.Value(operationLogKey{}).(*operationLogBuffer); ok {
		return value
	}
	return nil
}

func operationLogForCommand(cmd *cobra.Command) *operationLogBuffer {
	if cmd == nil {
		return nil
	}
	return operationLogFromContext(cmd.Context())
}

func logOperationf(cmd *cobra.Command, format string, args ...interface{}) {
	buffer := operationLogForCommand(cmd)
	if buffer == nil {
		return
	}
	buffer.Logf(format, args...)
}

func logOperationContextf(ctx context.Context, format string, args ...interface{}) {
	buffer := operationLogFromContext(ctx)
	if buffer == nil {
		return
	}
	buffer.Logf(format, args...)
}

func (b *operationLogBuffer) Logf(format string, args ...interface{}) {
	if b == nil {
		return
	}
	line := strings.TrimSpace(fmt.Sprintf(format, args...))
	if line == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
}

func (b *operationLogBuffer) Lines() []string {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	lines := make([]string, len(b.lines))
	copy(lines, b.lines)
	return lines
}
