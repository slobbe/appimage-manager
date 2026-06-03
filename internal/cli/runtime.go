package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	appimageapp "github.com/slobbe/appimage-manager/internal/app/appimage"
	appintegrate "github.com/slobbe/appimage-manager/internal/app/integrate"
	appremove "github.com/slobbe/appimage-manager/internal/app/remove"
	appselfupdate "github.com/slobbe/appimage-manager/internal/app/selfupdate"
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

func checkAppUpdate(app *domain.App) (*appupdate.ManagedUpdate, error) {
	update, err := appupdate.NewManagedUpdateChecker(appupdate.ManagedUpdateChecker{
		ZsyncCheck:         runZsyncUpdateCheck,
		GitHubReleaseCheck: runGitHubReleaseUpdateCheck,
	}).Check(app)
	if err != nil {
		return nil, softwareError(err)
	}
	return update, nil
}

var (
	downloadManagedRemoteAsset = func(ctx context.Context, assetURL, destination string, interactive bool, onProgress func(int64, int64)) error {
		return appupdate.Service{
			TempDir:        runtimeTempDir(),
			NowISO:         runtimeNowISO,
			StagedDownload: stagedDownloadAdapter{client: runtimeDownloadHTTPClient},
		}.DownloadManagedUpdateAsset(ctx, assetURL, destination, onProgress)
	}
)

func managedUpdateService() appupdate.Service {
	return appupdate.Service{
		TempDir:        runtimeTempDir(),
		NowISO:         runtimeNowISO,
		Zsync:          runtimeZsyncRunner(),
		StagedDownload: stagedDownloadAdapter{client: runtimeDownloadHTTPClient},
		HashVerifier:   hashVerifierAdapter{},
		DownloadAsset: func(ctx context.Context, assetURL, destination string, onProgress func(downloaded, total int64)) error {
			return downloadManagedRemoteAsset(ctx, assetURL, destination, false, onProgress)
		},
		Integrate: func(ctx context.Context, src string, confirm func(existing, incoming *domain.UpdateSource) (bool, error)) (*domain.App, error) {
			return integrateManagedUpdate(ctx, src, confirm)
		},
	}
}

func applyManagedUpdate(ctx context.Context, update appupdate.ManagedUpdate, reporter appservices.ManagedApplyReporter) (*domain.App, error) {
	return managedUpdateService().ApplyManagedUpdate(ctx, update, reporter)
}

func applyZsyncUpdate(ctx context.Context, update appupdate.ManagedUpdate, destination string) error {
	return rewriteZsyncFailure(appupdate.Service{
		Zsync: runtimeZsyncRunner(),
	}.ApplyManagedZsyncUpdate(ctx, update, destination))
}

func verifyDownloadedUpdate(downloadPath string, update appupdate.ManagedUpdate) error {
	service := appupdate.NewService(appupdate.Service{HashVerifier: hashVerifierAdapter{}})
	return rewriteChecksumError(service.VerifyDownloadedUpdate(downloadPath, update))
}

func updateCheckMetadata(app *domain.App, checked, available bool, latest string) error {
	if app == nil {
		return nil
	}
	lastCheckedAt := runtimeNowISO()
	if err := updateCheckMetadataBatch([]appservices.CheckMetadataUpdate{{
		ID:            app.ID,
		Checked:       checked,
		Available:     available,
		Latest:        latest,
		LastCheckedAt: lastCheckedAt,
	}}); err != nil {
		return wrapWriteError(err)
	}
	if checked {
		app.UpdateAvailable = available
		app.LatestVersion = strings.TrimSpace(latest)
	}
	app.LastCheckedAt = lastCheckedAt
	return nil
}

func loadUpdateCheckCache() (*appupdate.CheckCacheFile, error) {
	path := updateCheckCacheFilePath()
	data, ok, err := runtimeReadFileIfExists(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return appupdate.NewCheckCacheFile(), nil
	}

	var cache appupdate.CheckCacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return appupdate.NormalizeCheckCache(&cache), nil
}

func saveUpdateCheckCache(cache *appupdate.CheckCacheFile) error {
	if cache == nil {
		return nil
	}
	if err := runtimeEnsureDir(runtimeTempDir()); err != nil {
		return err
	}
	appupdate.NormalizeCheckCache(cache)

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return runtimeWriteAtomicFile(updateCheckCacheFilePath(), data, 0o644)
}

func invalidateCachedManagedUpdates(cache *appupdate.CheckCacheFile, appIDs ...string) {
	appupdate.InvalidateCachedManagedUpdates(cache, appIDs...)
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
	configureAppPorts(settings.NetworkTimeout)
	configureRuntimeWorkflowServices()

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

func appimagePathsFromConfig(paths config.Paths) appimageapp.Paths {
	return appimageapp.Paths{
		AimDir:  paths.AimDir,
		TempDir: paths.TempDir,
	}
}

func integratePathsFromConfig(paths config.Paths) appintegrate.Paths {
	return appintegrate.Paths{
		AimDir:       paths.AimDir,
		DesktopDir:   paths.DesktopDir,
		TempDir:      paths.TempDir,
		IconThemeDir: paths.IconThemeDir,
	}
}

func removePathsFromConfig(paths config.Paths) appremove.Paths {
	return appremove.Paths{
		AimDir:       paths.AimDir,
		DesktopDir:   paths.DesktopDir,
		IconThemeDir: paths.IconThemeDir,
	}
}

func selfUpdatePathsFromConfig(paths config.Paths) appselfupdate.Paths {
	return appselfupdate.Paths{
		TempDir: paths.TempDir,
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
	Add        appservices.AddService
	List       appservices.ListService
	Info       appservices.InfoService
	Remove     appservices.RemoveService
	Update     appservices.UpdateService
	SelfUpdate appservices.SelfUpdateService
	Discovery  appservices.DiscoveryService
	Locker     appservices.StateLocker
}

type updateSourceReplaceConfirmerFunc func(existing, incoming *appservices.UpdateSourceView) (bool, error)

func (fn updateSourceReplaceConfirmerFunc) ConfirmUpdateSourceReplace(existing, incoming *appservices.UpdateSourceView) (bool, error) {
	return fn(existing, incoming)
}

func defaultRuntimeServices() runtimeServices {
	return defaultRuntimeServicesForSettings(runtimeSettings{NetworkTimeout: 30 * time.Second})
}

func defaultRuntimeServicesForSettings(settings runtimeSettings) runtimeServices {
	store := repositoryStore()
	discoveryService := appservices.NewDiscoveryWorkflowService(appservices.DiscoveryWorkflowService{BackendsFunc: discoveryBackends})
	apiClient := httpclient.New(settings.NetworkTimeout)
	selfUpdater := selfUpdaterAdapter{client: apiClient}
	selfUpdateService := appselfupdate.NewService(appselfupdate.Service{
		TempDir:     config.TempDir,
		SelfUpdater: selfUpdater,
	})
	remoteInstallService := appservices.NewRemoteInstallService(appservices.RemoteInstallService{
		Store:             store,
		Filename:          updateDownloadFilename,
		StableDestination: stableDownloadDestination,
		Download: func(ctx context.Context, assetURL, destination string) error {
			return downloadRemoteAsset(ctx, assetURL, destination, isTerminalStderr())
		},
		VerifySHA256: func(path, expectedSHA256 string) error {
			return verifyDownloadedUpdate(path, appupdate.ManagedUpdate{ExpectedSHA256: expectedSHA256})
		},
		IntegrateLocalApp: integrateLocalApp,
		PersistApp:        addSingleApp,
		RemoveApp:         removeManagedApp,
		RemoveStaged:      removeStagedDownload,
	})
	return runtimeServices{
		Add: appservices.NewAddWorkflowService(appservices.AddWorkflowService{
			Store:             store,
			Discovery:         discoveryService,
			Installer:         remoteInstallService,
			HasExtension:      runtimeHasExtension,
			IntegrateLocalApp: integrateLocalApp,
			ReintegrateApp:    integrateExistingApp,
			AppImageInfo:      appservices.AppImageInfoReaderFunc(readAppImageInfo),
			AimDir:            config.AimDir,
			DesktopDir:        config.DesktopDir,
		}),
		List: appservices.NewStoreListService(store),
		Info: appservices.NewStoreInfoService(appservices.StoreInfoService{
			Store:      store,
			AppImage:   appservices.AppImageInfoReaderFunc(readAppImageInfo),
			UpdateInfo: appservices.UpdateInfoReaderFunc(getAppImageUpdateInfo),
			Discovery:  discoveryService,
		}),
		Remove: appservices.NewRemoveWorkflowService(appservices.RemoveWorkflowService{Store: store, RemoveFunc: removeManagedApp}),
		Update: appservices.NewSourceUpdateWorkflowService(appservices.SourceUpdateService{
			Store:                store,
			Locker:               stateFileLocker{},
			UpdateInfo:           appservices.UpdateInfoReaderFunc(getAppImageUpdateInfo),
			CheckManagedUpdate:   runAppUpdateCheck,
			LoadCheckCache:       loadUpdateCheckCache,
			SaveCheckCache:       saveUpdateCheckCache,
			PersistCheckMetadata: updateCheckMetadataBatch,
			InvalidateCheckCache: func(ids []string) error {
				cache, err := loadUpdateCheckCache()
				if err != nil {
					return err
				}
				invalidateCachedManagedUpdates(cache, ids...)
				return saveUpdateCheckCache(cache)
			},
			ApplyManagedUpdate: runManagedApply,
			PersistApps:        addAppsBatch,
			PersistApp:         addSingleApp,
			RemoveApp:          removeManagedApp,
			NowISO:             runtimeNowISO,
			RefreshCaches: func(ctx context.Context) {
				appintegrate.RefreshDesktopIntegrationCaches(ctx)
			},
		}),
		SelfUpdate: appservices.NewSelfUpdateWorkflowService(appservices.SelfUpdateWorkflowService{
			CheckFunc: func(ctx context.Context, currentVersion string, preRelease bool) (*appselfupdate.AimSelfUpdateCheckResult, error) {
				if checkAimSelfUpdate != nil {
					return checkAimSelfUpdate(ctx, currentVersion, preRelease)
				}
				return selfUpdateService.Check(ctx, currentVersion, preRelease)
			},
			SelfUpdateFunc: func(ctx context.Context, req appselfupdate.InstallerSelfUpdateRequest) (*appselfupdate.InstallerSelfUpdateResult, error) {
				if runSelfUpdateInstaller != nil {
					return runSelfUpdateInstaller(ctx, req)
				}
				return selfUpdateService.SelfUpdate(ctx, req)
			},
		}),
		Discovery: discoveryService,
		Locker:    stateFileLocker{},
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
