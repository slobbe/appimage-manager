package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/core"
	"github.com/slobbe/appimage-manager/internal/discovery"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/spf13/cobra"
)

func RootCmd(cmd *cobra.Command, args []string) error {
	upgrade, err := cmd.Flags().GetBool("upgrade")
	if err != nil {
		return err
	}
	if upgrade {
		if len(args) > 0 {
			return fmt.Errorf("--upgrade does not accept positional arguments")
		}
		return runUpgrade(cmd.Context(), cmd)
	}
	return cmd.Help()
}

func maybeRunRootUpgradeFlag(ctx context.Context, cmd *cobra.Command, args []string) (bool, error) {
	if cmd == nil || cmd.Name() != "aim" || len(args) == 0 {
		return false, nil
	}

	switch args[0] {
	case "--upgrade", "-U":
		if len(args) != 1 {
			return true, fmt.Errorf("--upgrade does not accept positional arguments")
		}
		return true, runUpgrade(ctx, cmd)
	default:
		return false, nil
	}
}

func UpgradeCmd(cmd *cobra.Command, args []string) error {
	_ = args
	return runUpgrade(cmd.Context(), cmd)
}

func runUpgrade(ctx context.Context, cmd *cobra.Command) error {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := runUpgradeViaInstaller(ctx, version)
	if err != nil {
		return err
	}
	if result != nil && strings.TrimSpace(result.InstalledVersion) != "" {
		printSuccess(cmd, fmt.Sprintf(
			"Updated aim %s -> %s",
			displayVersion(result.PreviousVersion),
			displayVersion(result.InstalledVersion),
		))
		return nil
	}
	printSuccess(cmd, "Updated aim via installer")
	return nil
}

func VersionCmd(cmd *cobra.Command, args []string) error {
	_ = cmd
	_ = args

	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Repository: %s\n", rootCommandRepositoryURL)
	fmt.Printf("License: %s\n", rootCommandLicense)
	fmt.Printf("Issues: %s\n", rootCommandIssuesURL)
	fmt.Printf("Author: %s\n", rootCommandAuthor)
	fmt.Printf("Copyright: %s\n", rootCommandCopyright)

	return nil
}

func AddCmd(cmd *cobra.Command, args []string) error {
	if ref, ok, err := resolveAddProviderRef(cmd, args); err != nil {
		return err
	} else if ok {
		return runInstallPackageRef(cmd.Context(), cmd, ref)
	}

	input, err := commandSingleArg(args, "<https-url|github-url|gitlab-url|id|Path/To.AppImage>")
	if err != nil {
		return err
	}

	if isRemoteAddInput(input) {
		return runInstallTarget(cmd.Context(), cmd, input)
	}

	if _, err := resolveIntegrateTarget(input); err == nil {
		if err := validateAddIntegrateFlags(cmd); err != nil {
			return err
		}
		return runIntegrateTarget(cmd.Context(), cmd, input)
	}

	return fmt.Errorf("unknown add target %q; expected https://..., GitHub/GitLab repo URL, <id>, or <Path/To.AppImage>", input)
}

func resolveAddProviderRef(cmd *cobra.Command, args []string) (discovery.PackageRef, bool, error) {
	return resolveProviderFlagRef(cmd, args, "add")
}

func resolveInfoProviderRef(cmd *cobra.Command, args []string) (discovery.PackageRef, bool, error) {
	return resolveProviderFlagRef(cmd, args, "info")
}

func resolveProviderFlagRef(cmd *cobra.Command, args []string, cmdName string) (discovery.PackageRef, bool, error) {
	githubValue, err := flagString(cmd, "github")
	if err != nil {
		return discovery.PackageRef{}, false, err
	}
	gitlabValue, err := flagString(cmd, "gitlab")
	if err != nil {
		return discovery.PackageRef{}, false, err
	}

	hasGitHub := strings.TrimSpace(githubValue) != ""
	hasGitLab := strings.TrimSpace(gitlabValue) != ""

	if !hasGitHub && !hasGitLab {
		return discovery.PackageRef{}, false, nil
	}
	if hasGitHub && hasGitLab {
		return discovery.PackageRef{}, false, fmt.Errorf("--github and --gitlab are mutually exclusive")
	}
	if len(args) > 0 {
		return discovery.PackageRef{}, false, fmt.Errorf("when using --github or --gitlab, do not pass a positional target")
	}

	if hasGitHub {
		ref, err := discovery.ParseGitHubRepoValue(githubValue)
		return ref, true, err
	}

	ref, err := discovery.ParseGitLabProjectValue(gitlabValue)
	return ref, true, err
}

func isLegacyPackageRef(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "github:") || strings.HasPrefix(trimmed, "gitlab:")
}

func legacyProviderRefGuidance(cmdName, input string) error {
	trimmed := strings.TrimSpace(input)
	switch {
	case strings.HasPrefix(trimmed, "github:"):
		return fmt.Errorf("github:... refs are no longer accepted; use 'aim %s --github owner/repo' or a GitHub repo URL", cmdName)
	case strings.HasPrefix(trimmed, "gitlab:"):
		return fmt.Errorf("gitlab:... refs are no longer accepted; use 'aim %s --gitlab namespace/project' or a GitLab project URL", cmdName)
	default:
		return fmt.Errorf("unsupported provider ref %q", input)
	}
}

type integrateTargetKind string

const (
	integrateTargetLocalFile  integrateTargetKind = "local_file"
	integrateTargetUnlinked   integrateTargetKind = "unlinked"
	integrateTargetIntegrated integrateTargetKind = "integrated"
)

type integrateTarget struct {
	Kind      integrateTargetKind
	App       *models.App
	LocalPath string
}

func runIntegrateTarget(ctx context.Context, cmd *cobra.Command, input string) error {
	target, err := resolveIntegrateTarget(input)
	if err != nil {
		return err
	}

	switch target.Kind {
	case integrateTargetIntegrated:
		printSuccess(cmd, fmt.Sprintf("Already integrated: %s", formatAppRef(target.App)))
		return nil
	case integrateTargetUnlinked:
		app, err := integrateExistingApp(ctx, target.App.ID)
		if err != nil {
			return err
		}
		printSuccess(cmd, fmt.Sprintf("Reintegrated: %s", formatAppRef(app)))
		return nil
	case integrateTargetLocalFile:
		inputLabel := strings.TrimSpace(filepath.Base(target.LocalPath))
		if inputLabel == "" || inputLabel == "." || inputLabel == string(filepath.Separator) {
			inputLabel = strings.TrimSpace(target.LocalPath)
		}

		printInfo(cmd, fmt.Sprintf("Integrating %s", inputLabel))

		app, err := integrateLocalApp(ctx, target.LocalPath, func(existing, incoming *models.UpdateSource) (bool, error) {
			fmt.Println("Current update source:")
			fmt.Println("  " + updateSummary(existing))
			fmt.Println("Incoming AppImage update info:")
			fmt.Println("  " + updateSummary(incoming))
			prompt := fmt.Sprintf("Replace update source %s with AppImage update info? [y/N]: ", existing.Kind)
			return confirmOverwrite(prompt)
		})
		if err != nil {
			return err
		}
		printSuccess(cmd, fmt.Sprintf("Integrated: %s", formatAppRef(app)))
		return nil
	default:
		return fmt.Errorf("unknown integrate target %q", input)
	}
}

func resolveIntegrateTarget(input string) (*integrateTarget, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, fmt.Errorf("missing required argument <Path/To.AppImage|id>")
	}

	if app, err := repo.GetApp(trimmed); err == nil {
		kind := integrateTargetIntegrated
		if strings.TrimSpace(app.DesktopEntryLink) == "" {
			kind = integrateTargetUnlinked
		}
		return &integrateTarget{Kind: kind, App: app}, nil
	}

	if strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "github:") || strings.HasPrefix(trimmed, "gitlab:") {
		return nil, fmt.Errorf("remote sources are added with 'aim add'")
	}

	if strings.HasPrefix(strings.ToLower(trimmed), "http://") {
		return nil, fmt.Errorf("direct URLs must use https; use 'aim add https://...'")
	}

	if util.HasExtension(trimmed, ".AppImage") {
		return &integrateTarget{Kind: integrateTargetLocalFile, LocalPath: trimmed}, nil
	}

	return nil, fmt.Errorf("unknown argument %s", input)
}

type installTargetKind string

const (
	installTargetDirectURL installTargetKind = "direct_url"
	installTargetGitHub    installTargetKind = "github_release"
	installTargetGitLab    installTargetKind = "gitlab_release"
)

type installTarget struct {
	Kind    installTargetKind
	URL     string
	Repo    string
	Project string
}

func resolveInstallTarget(input string) (*installTarget, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, fmt.Errorf("missing required argument <ref>")
	}

	if isLegacyPackageRef(trimmed) {
		return nil, legacyProviderRefGuidance("add", trimmed)
	}

	if ref, err := discovery.ParsePackageRefURL(trimmed); err == nil {
		switch ref.Kind {
		case discovery.ProviderGitHub:
			return &installTarget{Kind: installTargetGitHub, Repo: ref.ProviderRef}, nil
		case discovery.ProviderGitLab:
			return &installTarget{Kind: installTargetGitLab, Project: ref.ProviderRef}, nil
		}
	}

	if isHTTPSURL(trimmed) {
		return &installTarget{Kind: installTargetDirectURL, URL: trimmed}, nil
	}

	if strings.HasPrefix(strings.ToLower(trimmed), "http://") {
		return nil, fmt.Errorf("direct URLs must use https")
	}

	if app, err := repo.GetApp(trimmed); err == nil && app != nil {
		return nil, fmt.Errorf("managed app IDs are added with 'aim add <id>'")
	}

	if util.HasExtension(trimmed, ".AppImage") {
		return nil, fmt.Errorf("local AppImages are added with 'aim add <Path/To.AppImage>'")
	}

	return nil, fmt.Errorf("unknown add target %s", input)
}

func validateInstallTargetFlags(cmd *cobra.Command, target *installTarget) error {
	if target == nil {
		return fmt.Errorf("missing add target")
	}

	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}

	switch target.Kind {
	case installTargetGitHub, installTargetGitLab:
		if sha256 != "" {
			return fmt.Errorf("--sha256 is only supported with direct https URLs")
		}
	case installTargetDirectURL:
		if assetPattern != "" {
			return fmt.Errorf("--asset is only supported with GitHub or GitLab provider sources")
		}
		if sha256 != "" && !isSHA256Hex(sha256) {
			return fmt.Errorf("--sha256 must be a valid 64-character hexadecimal SHA-256")
		}
	default:
		return fmt.Errorf("unsupported add target")
	}

	return nil
}

func integrateFromDirectURL(ctx context.Context, cmd *cobra.Command, target *installTarget, sha256 string) (*models.App, error) {
	if strings.TrimSpace(sha256) == "" {
		printWarning(cmd, "No SHA-256 provided; skipping checksum verification")
	}

	return integrateRemoteInstall(ctx, cmd, remoteInstallRequest{
		DisplayLabel:   target.URL,
		DownloadURL:    target.URL,
		ExpectedSHA256: sha256,
		BuildSource: func(app *models.App) models.Source {
			return models.Source{
				Kind: models.SourceDirectURL,
				DirectURL: &models.DirectURLSource{
					URL:          target.URL,
					SHA256:       sha256,
					DownloadedAt: app.UpdatedAt,
				},
			}
		},
		BuildUpdate: func(*models.App) *models.UpdateSource {
			return &models.UpdateSource{Kind: models.UpdateNone}
		},
	})
}

func integrateFromGitHubRelease(ctx context.Context, cmd *cobra.Command, target *installTarget, assetPattern string) (*models.App, error) {
	if strings.TrimSpace(assetPattern) == "" {
		assetPattern = defaultReleaseAssetPattern
	}

	printInfo(cmd, fmt.Sprintf("Resolving GitHub release for %s", target.Repo))
	release, err := resolveGitHubReleaseAsset(target.Repo, assetPattern)
	if err != nil {
		return nil, err
	}

	return integrateGitHubReleaseAsset(ctx, cmd, target, assetPattern, release)
}

func integrateGitHubReleaseAsset(ctx context.Context, cmd *cobra.Command, target *installTarget, assetPattern string, release *core.GitHubReleaseAsset) (*models.App, error) {
	return integrateRemoteInstall(ctx, cmd, remoteInstallRequest{
		DisplayLabel: target.Repo,
		DownloadURL:  release.DownloadURL,
		AssetName:    release.AssetName,
		BuildSource: func(app *models.App) models.Source {
			return models.Source{
				Kind: models.SourceGitHubRelease,
				GitHubRelease: &models.GitHubReleaseSource{
					Repo:         target.Repo,
					Asset:        assetPattern,
					Tag:          release.TagName,
					AssetName:    release.AssetName,
					DownloadedAt: app.UpdatedAt,
				},
			}
		},
		BuildUpdate: func(*models.App) *models.UpdateSource {
			return &models.UpdateSource{
				Kind: models.UpdateGitHubRelease,
				GitHubRelease: &models.GitHubReleaseUpdateSource{
					Repo:  target.Repo,
					Asset: assetPattern,
				},
			}
		},
	})
}

func integrateFromGitLabRelease(ctx context.Context, cmd *cobra.Command, target *installTarget, assetPattern string) (*models.App, error) {
	if strings.TrimSpace(assetPattern) == "" {
		assetPattern = defaultReleaseAssetPattern
	}

	printInfo(cmd, fmt.Sprintf("Resolving GitLab release for %s", target.Project))
	release, err := resolveGitLabReleaseAsset(target.Project, assetPattern)
	if err != nil {
		return nil, err
	}

	return integrateGitLabReleaseAsset(ctx, cmd, target, assetPattern, release)
}

func integrateGitLabReleaseAsset(ctx context.Context, cmd *cobra.Command, target *installTarget, assetPattern string, release *core.GitLabReleaseAsset) (*models.App, error) {
	return integrateRemoteInstall(ctx, cmd, remoteInstallRequest{
		DisplayLabel: target.Project,
		DownloadURL:  release.DownloadURL,
		AssetName:    release.AssetName,
		BuildSource: func(app *models.App) models.Source {
			return models.Source{
				Kind: models.SourceGitLabRelease,
				GitLabRelease: &models.GitLabReleaseSource{
					Project:      target.Project,
					Asset:        assetPattern,
					Tag:          release.TagName,
					AssetName:    release.AssetName,
					DownloadedAt: app.UpdatedAt,
				},
			}
		},
		BuildUpdate: func(*models.App) *models.UpdateSource {
			return &models.UpdateSource{
				Kind: models.UpdateGitLabRelease,
				GitLabRelease: &models.GitLabReleaseUpdateSource{
					Project: target.Project,
					Asset:   assetPattern,
				},
			}
		},
	})
}

type remoteInstallRequest struct {
	DisplayLabel   string
	DownloadURL    string
	AssetName      string
	ExpectedSHA256 string
	BuildSource    func(app *models.App) models.Source
	BuildUpdate    func(app *models.App) *models.UpdateSource
}

func integrateRemoteInstall(ctx context.Context, cmd *cobra.Command, req remoteInstallRequest) (*models.App, error) {
	tempDir, err := os.MkdirTemp(config.TempDir, "aim-install-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileName := updateDownloadFilename(req.AssetName, req.DownloadURL)
	downloadPath := filepath.Join(tempDir, fileName)

	printInfo(cmd, fmt.Sprintf("Downloading %s", strings.TrimSpace(req.DisplayLabel)))
	if err := downloadRemoteAsset(ctx, req.DownloadURL, downloadPath, isTerminalOutput()); err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.ExpectedSHA256) != "" {
		fmt.Println("  Verifying")
		if err := verifyDownloadedUpdate(downloadPath, pendingManagedUpdate{ExpectedSHA256: req.ExpectedSHA256}); err != nil {
			return nil, err
		}
	}

	printInfo(cmd, fmt.Sprintf("Integrating %s", fileName))
	app, err := integrateLocalApp(ctx, downloadPath, func(existing, incoming *models.UpdateSource) (bool, error) {
		_ = existing
		_ = incoming
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	incomingUpdate := req.BuildUpdate(app)
	finalUpdate, err := chooseRemoteUpdateSource(app.ID, incomingUpdate)
	if err != nil {
		return nil, err
	}

	app.Source = req.BuildSource(app)
	app.Update = finalUpdate

	if err := addSingleApp(app, true); err != nil {
		return nil, err
	}

	return app, nil
}

func chooseRemoteUpdateSource(id string, incoming *models.UpdateSource) (*models.UpdateSource, error) {
	if incoming == nil {
		incoming = &models.UpdateSource{Kind: models.UpdateNone}
	}

	existingApp, err := repo.GetApp(id)
	if err != nil {
		return incoming, nil
	}

	existing := existingApp.Update
	if existing == nil || existing.Kind == models.UpdateNone {
		return incoming, nil
	}
	if updateSourcesEqual(existing, incoming) {
		return incoming, nil
	}

	fmt.Printf("Current update source for %s:\n", id)
	fmt.Println("  " + updateSummary(existing))
	fmt.Println("New update source:")
	fmt.Println("  " + updateSummary(incoming))
	prompt := fmt.Sprintf("Replace update source for %s? [y/N]: ", id)
	confirmed, err := confirmOverwrite(prompt)
	if err != nil {
		return nil, err
	}
	if !confirmed {
		return existing, nil
	}

	return incoming, nil
}

func updateSourcesEqual(a, b *models.UpdateSource) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}

	switch a.Kind {
	case models.UpdateNone:
		return true
	case models.UpdateGitHubRelease:
		return a.GitHubRelease != nil && b.GitHubRelease != nil &&
			strings.TrimSpace(a.GitHubRelease.Repo) == strings.TrimSpace(b.GitHubRelease.Repo) &&
			strings.TrimSpace(a.GitHubRelease.Asset) == strings.TrimSpace(b.GitHubRelease.Asset)
	case models.UpdateGitLabRelease:
		return a.GitLabRelease != nil && b.GitLabRelease != nil &&
			strings.TrimSpace(a.GitLabRelease.Project) == strings.TrimSpace(b.GitLabRelease.Project) &&
			strings.TrimSpace(a.GitLabRelease.Asset) == strings.TrimSpace(b.GitLabRelease.Asset)
	case models.UpdateZsync:
		return a.Zsync != nil && b.Zsync != nil &&
			strings.TrimSpace(a.Zsync.UpdateInfo) == strings.TrimSpace(b.Zsync.UpdateInfo) &&
			strings.TrimSpace(a.Zsync.Transport) == strings.TrimSpace(b.Zsync.Transport)
	default:
		return false
	}
}

func isSHA256Hex(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != 64 {
		return false
	}
	_, err := hex.DecodeString(trimmed)
	return err == nil
}

func RemoveCmd(cmd *cobra.Command, args []string) error {
	id, err := commandSingleArg(args, "<id>")
	if err != nil {
		return err
	}
	unlink, err := flagBool(cmd, "unlink")
	if err != nil {
		return err
	}

	app, err := removeManagedApp(cmd.Context(), id, unlink)

	if err == nil {
		label := "Removed"
		if unlink {
			label = "Unlinked"
		}
		printSuccess(cmd, fmt.Sprintf("%s: %s [%s]", label, app.Name, app.ID))
	}

	return err
}

func ListCmd(cmd *cobra.Command, args []string) error {
	_ = args
	all, err := flagBool(cmd, "all")
	if err != nil {
		return err
	}
	integrated, err := flagBool(cmd, "integrated")
	if err != nil {
		return err
	}
	unlinked, err := flagBool(cmd, "unlinked")
	if err != nil {
		return err
	}

	if (integrated && unlinked) || (all && (integrated || unlinked)) {
		return fmt.Errorf("flags --all, --integrated, and --unlinked are mutually exclusive")
	}

	if !all && !integrated && !unlinked {
		all = true
	}

	apps, err := repo.GetAllApps()
	if err != nil {
		return err
	}

	if len(apps) == 0 {
		printSuccess(cmd, "No managed AppImages")
		return nil
	}

	integratedRows := make([]*models.App, 0, len(apps))
	unlinkedRows := make([]*models.App, 0, len(apps))
	for _, app := range apps {
		if len(app.DesktopEntryLink) > 0 {
			integratedRows = append(integratedRows, app)
			continue
		}
		unlinkedRows = append(unlinkedRows, app)
	}

	if integrated && len(integratedRows) == 0 {
		printSuccess(cmd, "No integrated AppImages")
		return nil
	}
	if unlinked && len(unlinkedRows) == 0 {
		printSuccess(cmd, "No unlinked AppImages")
		return nil
	}

	idWidth := listIDColumnWidth(integratedRows, unlinkedRows)
	nameWidth := listNameDisplayWidth(integratedRows, unlinkedRows)
	header := fmt.Sprintf("%-*s %-*s %s", idWidth, "ID", nameWidth, "App Name", "Version")
	printSection(cmd, header)

	if all || integrated {
		for _, app := range integratedRows {
			fmt.Fprintln(os.Stdout, formatListRow(app, idWidth, nameWidth))
		}
	}

	if all || unlinked {
		for _, app := range unlinkedRows {
			row := formatListRow(app, idWidth, nameWidth)
			fmt.Println(colorize(useColor(cmd), "\033[2m\033[3m", row))
		}
	}

	return nil
}

func InfoCmd(cmd *cobra.Command, args []string) error {
	if ref, ok, err := resolveInfoProviderRef(cmd, args); err != nil {
		return err
	} else if ok {
		return runShowPackageRef(cmd.Context(), cmd, ref)
	}

	input, err := commandSingleArg(args, "<target>")
	if err != nil {
		return err
	}

	if ref, err := resolvePackageRefInput(input); err == nil {
		return runShowPackageRef(cmd.Context(), cmd, ref)
	}

	if _, err := resolveInspectTarget(input); err == nil {
		return runInspectTarget(cmd.Context(), cmd, input)
	}

	return fmt.Errorf("unknown info target %q; expected GitHub/GitLab repo URL, <id>, or <Path/To.AppImage>", input)
}

func resolvePackageMetadataFromRef(ctx context.Context, ref discovery.PackageRef, assetOverride string) (*discovery.PackageMetadata, error) {
	backend, err := backendForRef(ref)
	if err != nil {
		return nil, err
	}

	metadata, err := backend.Resolve(ctx, ref, assetOverride)
	if err != nil {
		return nil, err
	}
	if metadata == nil {
		return nil, fmt.Errorf("failed to resolve package metadata for %s", discovery.FormatPackageRef(ref))
	}

	return metadata, nil
}

func resolvePackageMetadataFromInput(ctx context.Context, input, assetOverride string) (*discovery.PackageMetadata, error) {
	ref, err := resolvePackageRefInput(input)
	if err != nil {
		return nil, err
	}

	return resolvePackageMetadataFromRef(ctx, ref, assetOverride)
}

func resolvePackageRefInput(input string) (discovery.PackageRef, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return discovery.PackageRef{}, fmt.Errorf("missing package ref")
	}

	if isLegacyPackageRef(trimmed) {
		return discovery.PackageRef{}, legacyProviderRefGuidance("info", trimmed)
	}

	return discovery.ParsePackageRefURL(trimmed)
}

func backendForRef(ref discovery.PackageRef) (discovery.DiscoveryBackend, error) {
	for _, backend := range discoveryBackends() {
		switch {
		case ref.Kind == discovery.ProviderGitHub && strings.EqualFold(backend.Name(), "GitHub"):
			return backend, nil
		case ref.Kind == discovery.ProviderGitLab && strings.EqualFold(backend.Name(), "GitLab"):
			return backend, nil
		}
	}

	return nil, fmt.Errorf("no discovery backend available for %s", discovery.FormatPackageRef(ref))
}

func installPackageMetadata(ctx context.Context, cmd *cobra.Command, metadata *discovery.PackageMetadata) (*models.App, error) {
	if metadata == nil {
		return nil, fmt.Errorf("package metadata cannot be empty")
	}

	switch metadata.Ref.Kind {
	case discovery.ProviderGitHub:
		return installResolvedGitHubPackage(ctx, cmd, metadata)
	case discovery.ProviderGitLab:
		return installResolvedGitLabPackage(ctx, cmd, metadata)
	default:
		return nil, fmt.Errorf("unsupported add provider %q", metadata.Ref.Kind)
	}
}

func installResolvedGitHubPackage(ctx context.Context, cmd *cobra.Command, metadata *discovery.PackageMetadata) (*models.App, error) {
	release := &core.GitHubReleaseAsset{
		DownloadURL:       strings.TrimSpace(metadata.DownloadURL),
		TagName:           strings.TrimSpace(metadata.ReleaseTag),
		NormalizedVersion: strings.TrimSpace(strings.TrimPrefix(metadata.LatestVersion, "v")),
		AssetName:         strings.TrimSpace(metadata.AssetName),
	}
	target := &installTarget{Kind: installTargetGitHub, Repo: metadata.Ref.ProviderRef}
	return integrateGitHubReleaseAsset(ctx, cmd, target, metadata.AssetPattern, release)
}

func installResolvedGitLabPackage(ctx context.Context, cmd *cobra.Command, metadata *discovery.PackageMetadata) (*models.App, error) {
	release := &core.GitLabReleaseAsset{
		DownloadURL:       strings.TrimSpace(metadata.DownloadURL),
		TagName:           strings.TrimSpace(metadata.ReleaseTag),
		NormalizedVersion: strings.TrimSpace(strings.TrimPrefix(metadata.LatestVersion, "v")),
		AssetName:         strings.TrimSpace(metadata.AssetName),
	}
	target := &installTarget{Kind: installTargetGitLab, Project: metadata.Ref.ProviderRef}
	return integrateGitLabReleaseAsset(ctx, cmd, target, metadata.AssetPattern, release)
}

func printPackageMetadata(cmd *cobra.Command, metadata *discovery.PackageMetadata) {
	printSection(cmd, metadata.Name)
	fmt.Printf("Provider: %s\n", strings.TrimSpace(metadata.Provider))
	if providerRef := formatProviderRef(metadata.Ref); providerRef != "" {
		fmt.Printf("Provider ref: %s\n", providerRef)
	}
	if strings.TrimSpace(metadata.RepoURL) != "" {
		fmt.Printf("Source URL: %s\n", strings.TrimSpace(metadata.RepoURL))
	}
	if strings.TrimSpace(metadata.Summary) != "" {
		fmt.Printf("Summary: %s\n", strings.TrimSpace(metadata.Summary))
	}

	installable := strings.TrimSpace(metadata.InstallReason) == "" && metadata.Installable
	fmt.Printf("Installable: %s\n", yesNo(installable))

	if !installable && strings.TrimSpace(metadata.InstallReason) != "" {
		fmt.Printf("Reason: %s\n", strings.TrimSpace(metadata.InstallReason))
		return
	}

	if strings.TrimSpace(metadata.LatestVersion) != "" {
		fmt.Printf("Latest release: %s\n", displayVersion(metadata.LatestVersion))
	}
	if strings.TrimSpace(metadata.AssetName) != "" {
		fmt.Printf("Selected asset: %s\n", strings.TrimSpace(metadata.AssetName))
	}
	fmt.Println("Managed updates: yes")

	printSection(cmd, "Install Command")
	fmt.Printf("  %s\n", formatAddProviderCommand(metadata.Ref))
}

func formatProviderRef(ref discovery.PackageRef) string {
	value := strings.TrimSpace(ref.ProviderRef)
	if value == "" {
		return ""
	}

	switch ref.Kind {
	case discovery.ProviderGitHub:
		return "GitHub " + value
	case discovery.ProviderGitLab:
		return "GitLab " + value
	default:
		return value
	}
}

func formatAddProviderCommand(ref discovery.PackageRef) string {
	value := strings.TrimSpace(ref.ProviderRef)
	if value == "" {
		return "aim add"
	}

	switch ref.Kind {
	case discovery.ProviderGitHub:
		return "aim add --github " + value
	case discovery.ProviderGitLab:
		return "aim add --gitlab " + value
	default:
		return "aim add"
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

type inspectTargetKind string

const (
	inspectTargetManaged inspectTargetKind = "managed"
	inspectTargetLocal   inspectTargetKind = "local"
)

type inspectTarget struct {
	Kind inspectTargetKind
	App  *models.App
	Path string
}

func runInspectTarget(ctx context.Context, cmd *cobra.Command, input string) error {
	target, err := resolveInspectTarget(input)
	if err != nil {
		return err
	}

	switch target.Kind {
	case inspectTargetManaged:
		return inspectManagedApp(ctx, cmd, target.App)
	case inspectTargetLocal:
		return inspectLocalAppImage(ctx, cmd, target.Path)
	default:
		return fmt.Errorf("unknown inspect target %q", input)
	}
}

func runShowTarget(ctx context.Context, cmd *cobra.Command, refArg string) error {
	ref, err := resolvePackageRefInput(refArg)
	if err != nil {
		return err
	}

	return runShowPackageRef(ctx, cmd, ref)
}

func runShowPackageRef(ctx context.Context, cmd *cobra.Command, ref discovery.PackageRef) error {
	metadata, err := resolvePackageMetadataFromRef(ctx, ref, "")
	if err != nil {
		return err
	}

	printPackageMetadata(cmd, metadata)
	return nil
}

func runInstallTarget(ctx context.Context, cmd *cobra.Command, refArg string) error {
	target, err := resolveInstallTarget(refArg)
	if err != nil {
		return err
	}
	if err := validateInstallTargetFlags(cmd, target); err != nil {
		return err
	}

	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}

	var app *models.App
	switch target.Kind {
	case installTargetDirectURL:
		app, err = integrateFromDirectURL(ctx, cmd, target, sha256)
	case installTargetGitHub, installTargetGitLab:
		var metadata *discovery.PackageMetadata
		metadata, err = resolvePackageMetadataFromInput(ctx, refArg, assetPattern)
		if err == nil && !metadata.Installable {
			err = fmt.Errorf("package is not installable: %s", strings.TrimSpace(metadata.InstallReason))
		}
		if err == nil {
			app, err = installPackageMetadata(ctx, cmd, metadata)
		}
	default:
		err = fmt.Errorf("unsupported add target")
	}
	if err != nil {
		return err
	}

	printSuccess(cmd, fmt.Sprintf("Installed: %s", formatAppRef(app)))
	return nil
}

func runInstallPackageRef(ctx context.Context, cmd *cobra.Command, ref discovery.PackageRef) error {
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}
	if sha256 != "" {
		return fmt.Errorf("--sha256 is only supported with direct https URLs")
	}

	metadata, err := resolvePackageMetadataFromRef(ctx, ref, assetPattern)
	if err == nil && !metadata.Installable {
		err = fmt.Errorf("package is not installable: %s", strings.TrimSpace(metadata.InstallReason))
	}
	if err != nil {
		return err
	}

	app, err := installPackageMetadata(ctx, cmd, metadata)
	if err != nil {
		return err
	}

	printSuccess(cmd, fmt.Sprintf("Installed: %s", formatAppRef(app)))
	return nil
}

func commandSingleArg(args []string, usage string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("too many arguments")
	}

	value := ""
	if len(args) > 0 {
		value = strings.TrimSpace(args[0])
	}
	if value == "" {
		return "", fmt.Errorf("missing required argument %s", usage)
	}

	return value, nil
}

func isRemoteAddInput(input string) bool {
	trimmed := strings.TrimSpace(input)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(trimmed, "https://") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(trimmed, "github:") ||
		strings.HasPrefix(trimmed, "gitlab:")
}

func validateAddIntegrateFlags(cmd *cobra.Command) error {
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	if assetPattern != "" {
		return fmt.Errorf("--asset is only supported with GitHub or GitLab provider sources")
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}
	if sha256 != "" {
		return fmt.Errorf("--sha256 is only supported with direct https:// add sources")
	}

	return nil
}

func resolveInspectTarget(input string) (*inspectTarget, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, fmt.Errorf("missing required argument <id|Path/To.AppImage>")
	}

	if app, err := repo.GetApp(trimmed); err == nil {
		return &inspectTarget{Kind: inspectTargetManaged, App: app}, nil
	}

	if util.HasExtension(trimmed, ".AppImage") {
		return &inspectTarget{Kind: inspectTargetLocal, Path: trimmed}, nil
	}

	return nil, fmt.Errorf("unknown inspect target %s", input)
}

func inspectManagedApp(ctx context.Context, cmd *cobra.Command, app *models.App) error {
	if app == nil {
		return fmt.Errorf("managed app cannot be empty")
	}

	embeddedSource, _ := embeddedUpdateSourceForPath(app.ExecPath)

	printSection(cmd, "App")
	fmt.Printf("Name: %s\n", strings.TrimSpace(app.Name))
	fmt.Printf("ID: %s\n", strings.TrimSpace(app.ID))
	fmt.Printf("Version: %s\n", displayVersion(app.Version))
	fmt.Printf("Exec Path: %s\n", strings.TrimSpace(app.ExecPath))

	printSection(cmd, "Management")
	fmt.Printf("Configured update source: %s\n", updateSummaryOrNone(app.Update))
	fmt.Printf("Embedded update source: %s\n", updateSummaryOrNone(embeddedSource))

	printSection(cmd, "Update State")
	fmt.Printf("Update available: %s\n", yesNo(app.UpdateAvailable))
	if strings.TrimSpace(app.LatestVersion) != "" {
		fmt.Printf("Latest known version: %s\n", displayVersion(app.LatestVersion))
	}
	if strings.TrimSpace(app.LastCheckedAt) != "" {
		fmt.Printf("Last checked: %s\n", strings.TrimSpace(app.LastCheckedAt))
	}

	_ = ctx
	return nil
}

func inspectLocalAppImage(ctx context.Context, cmd *cobra.Command, src string) error {
	info, err := readAppImageInfo(ctx, src)
	if err != nil {
		return err
	}

	embeddedSource, _ := embeddedUpdateSourceForPath(src)

	printSection(cmd, "AppImage")
	fmt.Printf("Path: %s\n", strings.TrimSpace(src))
	fmt.Printf("Name: %s\n", strings.TrimSpace(info.Name))
	fmt.Printf("ID: %s\n", strings.TrimSpace(info.ID))
	fmt.Printf("Version: %s\n", displayVersion(info.Version))

	printSection(cmd, "Embedded Update")
	fmt.Printf("Embedded update source: %s\n", updateSummaryOrNone(embeddedSource))

	return nil
}

func updateSummaryOrNone(update *models.UpdateSource) string {
	if update == nil || update.Kind == models.UpdateNone {
		return "none"
	}
	return updateSummary(update)
}

func updateSourceFromEmbeddedInfo(info *core.UpdateInfo) (*models.UpdateSource, error) {
	if info == nil {
		return nil, fmt.Errorf("missing embedded update info")
	}
	if info.Kind != models.UpdateZsync {
		return nil, fmt.Errorf("unsupported embedded update info kind %q", info.Kind)
	}

	return &models.UpdateSource{
		Kind: models.UpdateZsync,
		Zsync: &models.ZsyncUpdateSource{
			UpdateInfo: strings.TrimSpace(info.UpdateInfo),
			Transport:  strings.TrimSpace(info.Transport),
		},
	}, nil
}

func embeddedUpdateSourceForPath(path string) (*models.UpdateSource, error) {
	info, err := getAppImageUpdateInfo(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	return updateSourceFromEmbeddedInfo(info)
}

func UpdateCmd(cmd *cobra.Command, args []string) error {
	if hasUpdateSetFlags(cmd) {
		return fmt.Errorf("update source flags can only be used with `aim update set`")
	}

	targetID := ""
	if len(args) > 0 {
		targetID = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}

	return runManagedUpdate(cmd.Context(), cmd, targetID)
}

func UpdateSetCmd(cmd *cobra.Command, args []string) error {
	if flagChanged(cmd, "yes") || flagChanged(cmd, "check-only") {
		return fmt.Errorf("flags --yes/-y and --check-only/-c are not supported with `aim update set`")
	}

	id, err := commandSingleArg(args, "<id>")
	if err != nil {
		return err
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return err
	}

	var incomingSource *models.UpdateSource
	embedded, err := flagBool(cmd, "embedded")
	if err != nil {
		return err
	}
	if embedded {
		if err := validateEmbeddedUpdateSetFlags(cmd); err != nil {
			return err
		}

		incomingSource, err = embeddedUpdateSourceForPath(app.ExecPath)
		if err != nil {
			printWarning(cmd, "No embedded update source found in the current AppImage.")
			if app.Update == nil || app.Update.Kind == models.UpdateNone {
				return nil
			}

			prompt := fmt.Sprintf("Unset current update source %s for %s? [y/N]: ", updateSummary(app.Update), id)
			_, err := unsetManagedUpdateSource(cmd, app, prompt, false)
			return err
		}
	} else {
		incomingSource, err = resolveUpdateSourceFromSetFlags(cmd)
		if err != nil {
			return err
		}
	}

	if app.Update != nil && app.Update.Kind != models.UpdateNone && !updateSourcesEqual(app.Update, incomingSource) {
		fmt.Printf("Current update source for %s:\n", id)
		fmt.Println("  " + updateSummary(app.Update))
		fmt.Println("New update source:")
		fmt.Println("  " + updateSummary(incomingSource))
		prompt := fmt.Sprintf("Replace update source for %s? [y/N]: ", id)
		confirmed, err := confirmOverwrite(prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			printWarning(cmd, "Update source unchanged")
			return nil
		}
	}

	app.Update = incomingSource

	if err := repo.UpdateApp(app); err != nil {
		return err
	}

	printSuccess(cmd, fmt.Sprintf("Update source set: %s", updateSummary(incomingSource)))
	return nil
}

func UpdateUnsetCmd(cmd *cobra.Command, args []string) error {
	if flagChanged(cmd, "yes") || flagChanged(cmd, "check-only") {
		return fmt.Errorf("flags --yes/-y and --check-only/-c are not supported with `aim update unset`")
	}
	if hasUpdateSetFlags(cmd) {
		return fmt.Errorf("update source flags are not supported with `aim update unset`")
	}

	id, err := commandSingleArg(args, "<id>")
	if err != nil {
		return err
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("Unset update source for %s? [y/N]: ", id)
	_, err = unsetManagedUpdateSource(cmd, app, prompt, true)
	return err
}

func UpdateCheckRemovedCmd(cmd *cobra.Command, args []string) error {
	_ = cmd
	_ = args
	return fmt.Errorf("`aim update check` has been removed; use `aim update [<id>]` for managed apps")
}

func unsetManagedUpdateSource(cmd *cobra.Command, app *models.App, prompt string, showCurrent bool) (bool, error) {
	if app == nil {
		return false, fmt.Errorf("managed app cannot be empty")
	}
	if app.Update == nil || app.Update.Kind == models.UpdateNone {
		printSuccess(cmd, fmt.Sprintf("No update source configured for %s", app.ID))
		return false, nil
	}

	if showCurrent {
		fmt.Printf("Current update source for %s:\n", app.ID)
		fmt.Println("  " + updateSummary(app.Update))
	}

	confirmed, err := confirmOverwrite(prompt)
	if err != nil {
		return false, err
	}
	if !confirmed {
		printWarning(cmd, "Update source unchanged")
		return false, nil
	}

	app.Update = &models.UpdateSource{Kind: models.UpdateNone}
	if err := repo.UpdateApp(app); err != nil {
		return false, err
	}

	printSuccess(cmd, "Update source unset")
	return true, nil
}

func hasUpdateSetFlags(cmd *cobra.Command) bool {
	keys := []string{"github", "gitlab", "asset", "zsync", "embedded", "manifest-url", "url", "sha256"}
	for _, key := range keys {
		if flagChanged(cmd, key) {
			return true
		}
	}
	return false
}

func validateEmbeddedUpdateSetFlags(cmd *cobra.Command) error {
	githubRepo, err := flagString(cmd, "github")
	if err != nil {
		return err
	}
	gitlabProject, err := flagString(cmd, "gitlab")
	if err != nil {
		return err
	}
	zsyncURL, err := flagString(cmd, "zsync")
	if err != nil {
		return err
	}
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	manifestURL, err := flagString(cmd, "manifest-url")
	if err != nil {
		return err
	}
	directURL, err := flagString(cmd, "url")
	if err != nil {
		return err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}

	if manifestURL != "" {
		return fmt.Errorf("--manifest-url is no longer supported; use --github, --gitlab, --zsync, or --embedded")
	}
	if directURL != "" {
		return fmt.Errorf("--url is no longer supported; use --github, --gitlab, --zsync, or --embedded")
	}
	if sha256 != "" {
		return fmt.Errorf("--sha256 is no longer supported; use --github, --gitlab, --zsync, or --embedded")
	}
	if assetPattern != "" {
		return fmt.Errorf("--asset is only supported with --github or --gitlab")
	}

	selectorCount := 1
	for _, value := range []string{githubRepo, gitlabProject, zsyncURL} {
		if value != "" {
			selectorCount++
		}
	}
	if selectorCount > 1 {
		return fmt.Errorf("update source flags are mutually exclusive")
	}

	return nil
}

func resolveUpdateSourceFromSetFlags(cmd *cobra.Command) (*models.UpdateSource, error) {
	githubRepo, err := flagString(cmd, "github")
	if err != nil {
		return nil, err
	}
	gitlabProject, err := flagString(cmd, "gitlab")
	if err != nil {
		return nil, err
	}
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return nil, err
	}
	zsyncURL, err := flagString(cmd, "zsync")
	if err != nil {
		return nil, err
	}
	manifestURL, err := flagString(cmd, "manifest-url")
	if err != nil {
		return nil, err
	}
	directURL, err := flagString(cmd, "url")
	if err != nil {
		return nil, err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return nil, err
	}

	if manifestURL != "" {
		return nil, fmt.Errorf("--manifest-url is no longer supported; use --github, --gitlab, --zsync, or --embedded")
	}
	if directURL != "" {
		return nil, fmt.Errorf("--url is no longer supported; use --github, --gitlab, --zsync, or --embedded")
	}
	if sha256 != "" {
		return nil, fmt.Errorf("--sha256 is no longer supported; use --github, --gitlab, --zsync, or --embedded")
	}

	selectorCount := 0
	for _, value := range []string{githubRepo, gitlabProject, zsyncURL} {
		if value != "" {
			selectorCount++
		}
	}

	if selectorCount == 0 {
		return nil, fmt.Errorf("missing update source; set one of --github, --gitlab, --zsync, or --embedded")
	}
	if selectorCount > 1 {
		return nil, fmt.Errorf("update source flags are mutually exclusive")
	}

	if githubRepo != "" {
		if assetPattern == "" {
			assetPattern = defaultReleaseAssetPattern
		}
		return &models.UpdateSource{
			Kind: models.UpdateGitHubRelease,
			GitHubRelease: &models.GitHubReleaseUpdateSource{
				Repo:  githubRepo,
				Asset: assetPattern,
			},
		}, nil
	}

	if gitlabProject != "" {
		if assetPattern == "" {
			assetPattern = defaultReleaseAssetPattern
		}
		return &models.UpdateSource{
			Kind: models.UpdateGitLabRelease,
			GitLabRelease: &models.GitLabReleaseUpdateSource{
				Project: gitlabProject,
				Asset:   assetPattern,
			},
		}, nil
	}

	if zsyncURL != "" {
		if assetPattern != "" {
			return nil, fmt.Errorf("--asset is only supported with --github or --gitlab")
		}
		if !isHTTPSURL(zsyncURL) {
			return nil, fmt.Errorf("--zsync must be a valid https URL")
		}
		return &models.UpdateSource{
			Kind: models.UpdateZsync,
			Zsync: &models.ZsyncUpdateSource{
				UpdateInfo: "zsync|" + zsyncURL,
				Transport:  "zsync",
			},
		}, nil
	}

	if assetPattern != "" {
		return nil, fmt.Errorf("--asset is only supported with --github or --gitlab")
	}
	return nil, fmt.Errorf("missing update source; set one of --github, --gitlab, --zsync, or --embedded")
}

type pendingManagedUpdate struct {
	App            *models.App
	URL            string
	Asset          string
	Label          string
	Available      bool
	Latest         string
	ExpectedSHA1   string
	ExpectedSHA256 string
	Transport      string
	ZsyncURL       string
	FromKind       models.UpdateKind
}

type managedCheckResult struct {
	app    *models.App
	update *pendingManagedUpdate
	err    error
}

var runAppUpdateCheck = checkAppUpdate
var runZsyncUpdateCheck = core.ZsyncUpdateCheck
var runGitHubReleaseUpdateCheck = core.GitHubReleaseUpdateCheck
var runGitLabReleaseUpdateCheck = core.GitLabReleaseUpdateCheck
var discoveryBackends = func() []discovery.DiscoveryBackend {
	return []discovery.DiscoveryBackend{
		discovery.GitHubBackend{},
		discovery.GitLabBackend{},
	}
}
var resolveGitHubReleaseAsset = core.ResolveGitHubReleaseAsset
var resolveGitLabReleaseAsset = core.ResolveGitLabReleaseAsset
var downloadRemoteAsset = downloadUpdateAsset
var downloadManagedRemoteAsset = downloadUpdateAssetWithProgress
var integrateManagedUpdate = core.IntegrateFromLocalFileWithoutCacheRefreshOrPersist
var zsyncLookPath = exec.LookPath
var zsyncCommandContext = exec.CommandContext
var runUpgradeViaInstaller = core.UpgradeViaInstaller
var runManagedApply = applyManagedUpdate
var integrateExistingApp = core.IntegrateExisting
var integrateLocalApp = core.IntegrateFromLocalFile
var readAppImageInfo = core.ReadAppImageInfo
var getAppImageUpdateInfo = core.GetUpdateInfo
var removeManagedApp = core.Remove
var addAppsBatch = repo.AddAppsBatch
var addSingleApp = repo.AddApp
var terminalOutputChecker = detectTerminalOutput

const defaultReleaseAssetPattern = "*.AppImage"

func runManagedUpdate(ctx context.Context, cmd *cobra.Command, targetID string) error {
	apps, err := collectManagedUpdateTargets(targetID)
	if err != nil {
		return err
	}

	autoApply, err := flagBool(cmd, "yes")
	if err != nil {
		return err
	}
	checkOnly, err := flagBool(cmd, "check-only")
	if err != nil {
		return err
	}

	var pending []pendingManagedUpdate
	checkFailures := 0
	singleStatusPrinted := false
	checkResults := runManagedChecks(apps)
	metadataUpdates := make([]repo.CheckMetadataUpdate, 0, len(checkResults))

	for _, result := range checkResults {
		app := result.app
		update := result.update
		err := result.err
		if err != nil {
			metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
				ID:            app.ID,
				Checked:       false,
				Available:     app.UpdateAvailable,
				Latest:        app.LatestVersion,
				LastCheckedAt: util.NowISO(),
			})

			if targetID != "" {
				if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
					return metaErr
				}
				metadataUpdates = metadataUpdates[:0]
				return fmt.Errorf("failed to check updates for %s: %w", app.ID, err)
			}

			checkFailures++
			printError(cmd, fmt.Sprintf("Failed to check updates for %s: %v", app.ID, err))
			continue
		}

		if update == nil {
			if targetID != "" {
				if app.Update == nil || app.Update.Kind == models.UpdateNone {
					printSuccess(cmd, fmt.Sprintf("No update source configured for %s", app.ID))
				} else {
					printSuccess(cmd, fmt.Sprintf("No update information for %s", app.ID))
				}
				singleStatusPrinted = true
			}
			continue
		}

		metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
			ID:            app.ID,
			Checked:       true,
			Available:     update.Available,
			Latest:        update.Latest,
			LastCheckedAt: util.NowISO(),
		})

		if targetID != "" {
			if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
				return metaErr
			}
			metadataUpdates = metadataUpdates[:0]
		}

		if update.URL == "" {
			if targetID != "" {
				printSuccess(cmd, fmt.Sprintf("Up to date: %s %s", app.ID, displayVersion(app.Version)))
				singleStatusPrinted = true
			}
			continue
		}

		msg := buildManagedUpdateMessage(*update, checkOnly)
		if targetID == "" {
			header := fmt.Sprintf("[%s]", app.ID)
			printSection(cmd, header)
		}
		printWarning(cmd, msg)
		if checkOnly {
			fmt.Printf("  Download: %s\n", update.URL)
			if showManagedUpdateAsset(update.Asset) {
				fmt.Printf("  Asset: %s\n", strings.TrimSpace(update.Asset))
			}
		}

		pending = append(pending, *update)
	}

	if err := flushManagedCheckMetadata(metadataUpdates); err != nil {
		if targetID != "" {
			return err
		}
		checkFailures++
		printError(cmd, fmt.Sprintf("Failed to persist update state: %v", err))
	}

	if len(pending) == 0 {
		if targetID != "" && singleStatusPrinted {
			return nil
		}
		if checkFailures > 0 {
			printWarning(cmd, "No updates applied; some checks failed")
			return nil
		}
		printSuccess(cmd, "All apps are up to date")
		return nil
	}

	if checkOnly {
		return nil
	}

	if !autoApply {
		prompt := fmt.Sprintf("Apply %d updates? [y/N]: ", len(pending))
		if targetID != "" {
			prompt = fmt.Sprintf("Apply update for %s? [y/N]: ", targetID)
		}

		confirmed, err := confirmOverwrite(prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			printWarning(cmd, "No updates applied")
			return nil
		}
	}

	applyResults := runManagedApplies(ctx, cmd, pending)
	applyFailures := 0
	appliedApps := make([]*models.App, 0, len(applyResults))
	for _, result := range applyResults {
		if result.err != nil {
			applyFailures++
			continue
		}
		if result.updatedApp != nil {
			appliedApps = append(appliedApps, result.updatedApp)
		}
	}

	if len(appliedApps) > 0 {
		core.RefreshDesktopIntegrationCaches(ctx)
	}

	persistErr := persistManagedAppliedApps(appliedApps)

	if applyFailures > 0 {
		if persistErr != nil {
			return fmt.Errorf("%d update(s) failed; failed to persist applied updates: %w", applyFailures, persistErr)
		}
		return fmt.Errorf("%d update(s) failed", applyFailures)
	}

	if persistErr != nil {
		return persistErr
	}

	return nil
}

func runManagedChecks(apps []*models.App) []managedCheckResult {
	results := make([]managedCheckResult, len(apps))
	if len(apps) == 0 {
		return results
	}

	groups := make(map[string][]int, len(apps))
	orderedKeys := make([]string, 0, len(apps))
	for idx, app := range apps {
		key := managedCheckCacheKey(app, idx)
		if _, exists := groups[key]; !exists {
			orderedKeys = append(orderedKeys, key)
		}
		groups[key] = append(groups[key], idx)
	}

	jobs := make(chan int, len(orderedKeys))
	workerCount := managedCheckWorkerCount(len(orderedKeys))

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for keyIdx := range jobs {
				key := orderedKeys[keyIdx]
				indices := groups[key]
				firstIdx := indices[0]
				primaryApp := apps[firstIdx]

				update, err := runAppUpdateCheck(primaryApp)
				for _, idx := range indices {
					app := apps[idx]
					results[idx] = managedCheckResult{
						app:    app,
						update: clonePendingManagedUpdateForApp(update, app),
						err:    err,
					}
				}
			}
		}()
	}

	for keyIdx := range orderedKeys {
		jobs <- keyIdx
	}
	close(jobs)

	wg.Wait()
	return results
}

func managedCheckCacheKey(app *models.App, fallbackIdx int) string {
	if app == nil || app.Update == nil {
		return fmt.Sprintf("none:%d", fallbackIdx)
	}

	kind := strings.TrimSpace(string(app.Update.Kind))
	version := normalizeCheckKeyValue(app.Version)
	sha1 := normalizeCheckKeyValue(app.SHA1)

	switch app.Update.Kind {
	case models.UpdateZsync:
		if app.Update.Zsync == nil {
			return fmt.Sprintf("zsync:missing:%s:%s", sha1, kind)
		}
		return fmt.Sprintf("zsync:%s:%s:%s", normalizeCheckKeyValue(app.Update.Zsync.UpdateInfo), normalizeCheckKeyValue(app.Update.Zsync.Transport), sha1)
	case models.UpdateGitHubRelease:
		if app.Update.GitHubRelease == nil {
			return fmt.Sprintf("github:missing:%s", version)
		}
		return fmt.Sprintf("github:%s:%s:%s", normalizeCheckKeyValue(app.Update.GitHubRelease.Repo), normalizeCheckKeyValue(app.Update.GitHubRelease.Asset), version)
	case models.UpdateGitLabRelease:
		if app.Update.GitLabRelease == nil {
			return fmt.Sprintf("gitlab:missing:%s", version)
		}
		return fmt.Sprintf("gitlab:%s:%s:%s", normalizeCheckKeyValue(app.Update.GitLabRelease.Project), normalizeCheckKeyValue(app.Update.GitLabRelease.Asset), version)
	default:
		return fmt.Sprintf("kind:%s:%s:%d", kind, version, fallbackIdx)
	}
}

func normalizeCheckKeyValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func clonePendingManagedUpdateForApp(update *pendingManagedUpdate, app *models.App) *pendingManagedUpdate {
	if update == nil {
		return nil
	}

	clone := *update
	clone.App = app
	return &clone
}

func managedCheckWorkerCount(total int) int {
	if total <= 0 {
		return 0
	}

	const maxWorkers = 4
	if total < maxWorkers {
		return total
	}

	return maxWorkers
}

func flushManagedCheckMetadata(updates []repo.CheckMetadataUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	return repo.UpdateCheckMetadataBatch(updates)
}

func persistManagedAppliedApps(apps []*models.App) error {
	if len(apps) == 0 {
		return nil
	}

	if err := addAppsBatch(apps, true); err == nil {
		return nil
	}

	fallbackErrors := make([]string, 0)
	for _, app := range apps {
		if app == nil {
			continue
		}
		if err := addSingleApp(app, true); err != nil {
			fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s: %v", app.ID, err))
		}
	}

	if len(fallbackErrors) > 0 {
		return fmt.Errorf("failed to persist applied updates: %s", strings.Join(fallbackErrors, "; "))
	}

	return nil
}

func collectManagedUpdateTargets(targetID string) ([]*models.App, error) {
	if strings.TrimSpace(targetID) != "" {
		app, err := repo.GetApp(targetID)
		if err != nil {
			return nil, err
		}
		return []*models.App{app}, nil
	}

	allApps, err := repo.GetAllApps()
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(allApps))
	for id := range allApps {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	apps := make([]*models.App, 0, len(ids))
	for _, id := range ids {
		apps = append(apps, allApps[id])
	}

	return apps, nil
}

func checkAppUpdate(app *models.App) (*pendingManagedUpdate, error) {
	if app == nil || app.Update == nil || app.Update.Kind == models.UpdateNone {
		return nil, nil
	}

	switch app.Update.Kind {
	case models.UpdateZsync:
		update, err := runZsyncUpdateCheck(app.Update, app.SHA1)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateZsync,
			}, nil
		}
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateZsync,
			}, nil
		}
		label := "Update available"
		if update.PreRelease {
			label = "Pre-release update available"
		}
		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadUrl,
			Asset:        update.AssetName,
			Label:        label,
			Available:    true,
			Latest:       "",
			ExpectedSHA1: strings.TrimSpace(update.RemoteSHA1),
			FromKind:     models.UpdateZsync,
		}, nil
	case models.UpdateGitHubRelease:
		update, err := runGitHubReleaseUpdateCheck(app.Update, app.Version, app.SHA1)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateGitHubRelease,
			}, nil
		}

		latest := strings.TrimSpace(update.NormalizedVersion)
		if latest == "" {
			latest = strings.TrimSpace(update.TagName)
		}

		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateGitHubRelease,
			}, nil
		}
		label := "Update available"
		if update.PreRelease {
			label = "Pre-release update available"
		}
		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadUrl,
			Asset:        update.AssetName,
			Label:        label,
			Available:    true,
			Latest:       latest,
			Transport:    update.Transport,
			ZsyncURL:     update.ZsyncURL,
			ExpectedSHA1: strings.TrimSpace(update.ExpectedSHA1),
			FromKind:     models.UpdateGitHubRelease,
		}, nil
	case models.UpdateGitLabRelease:
		update, err := runGitLabReleaseUpdateCheck(app.Update, app.Version, app.SHA1)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateGitLabRelease,
			}, nil
		}

		latest := strings.TrimSpace(update.NormalizedVersion)
		if latest == "" {
			latest = strings.TrimSpace(update.TagName)
		}
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateGitLabRelease,
			}, nil
		}

		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadURL,
			Asset:        update.AssetName,
			Label:        "Update available",
			Available:    true,
			Latest:       latest,
			Transport:    update.Transport,
			ZsyncURL:     update.ZsyncURL,
			ExpectedSHA1: strings.TrimSpace(update.ExpectedSHA1),
			FromKind:     models.UpdateGitLabRelease,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported update source for %s: %q. Reconfigure with `aim update set`", app.ID, app.Update.Kind)
	}
}

func updateCheckMetadata(app *models.App, checked, available bool, latest string) error {
	if app == nil {
		return nil
	}

	lastCheckedAt := util.NowISO()

	if err := repo.UpdateCheckMetadataBatch([]repo.CheckMetadataUpdate{{
		ID:            app.ID,
		Checked:       checked,
		Available:     available,
		Latest:        latest,
		LastCheckedAt: lastCheckedAt,
	}}); err != nil {
		return err
	}

	if checked {
		app.UpdateAvailable = available
		app.LatestVersion = strings.TrimSpace(latest)
	}
	app.LastCheckedAt = lastCheckedAt

	return nil
}

func applyManagedUpdate(ctx context.Context, update pendingManagedUpdate, reporter managedApplyReporter) (*models.App, error) {
	emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageQueued})

	if strings.TrimSpace(update.URL) == "" {
		err := fmt.Errorf("missing download URL")
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "aim-update-*")
	if err != nil {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileName := updateDownloadFilename(update.Asset, update.URL)
	downloadPath := filepath.Join(tempDir, fileName)
	usedZsync := false
	if strings.TrimSpace(update.ZsyncURL) != "" {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageZsync})
		if err := applyZsyncUpdate(ctx, update, downloadPath); err == nil {
			usedZsync = true
		}
	}

	if !usedZsync {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageDownload})
		if err := downloadManagedRemoteAsset(ctx, update.URL, downloadPath, false, func(downloaded, total int64) {
			emitManagedApplyEvent(reporter, managedApplyEvent{
				Stage:         managedApplyStageDownload,
				Downloaded:    downloaded,
				DownloadTotal: total,
			})
		}); err != nil {
			emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
			return nil, err
		}
	}

	emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageVerify})
	if err := verifyDownloadedUpdate(downloadPath, update); err != nil {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageIntegrate})
	app, err := integrateManagedUpdate(ctx, downloadPath, func(existing, incoming *models.UpdateSource) (bool, error) {
		return false, nil
	})
	if err != nil {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	emitManagedApplyEvent(reporter, managedApplyEvent{
		Stage:   managedApplyStageDone,
		Version: app.Version,
	})
	return app, nil
}

func applyZsyncUpdate(ctx context.Context, update pendingManagedUpdate, destination string) error {
	if update.App == nil {
		return fmt.Errorf("missing app")
	}
	if strings.TrimSpace(update.App.ExecPath) == "" {
		return fmt.Errorf("missing app exec path")
	}
	if strings.TrimSpace(update.ZsyncURL) == "" {
		return fmt.Errorf("missing zsync url")
	}

	binary, err := zsyncLookPath("zsync")
	if err != nil {
		return err
	}

	cmd := zsyncCommandContext(ctx, binary, "-q", "-i", update.App.ExecPath, "-o", destination, update.ZsyncURL)
	cmd.Dir = filepath.Dir(destination)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}

	if _, err := os.Stat(destination); err != nil {
		return err
	}

	return nil
}

func verifyDownloadedUpdate(downloadPath string, update pendingManagedUpdate) error {
	expectedSHA256 := strings.ToLower(strings.TrimSpace(update.ExpectedSHA256))
	expectedSHA1 := strings.ToLower(strings.TrimSpace(update.ExpectedSHA1))

	if expectedSHA256 != "" && expectedSHA1 != "" {
		sha256sum, sha1sum, err := util.Sha256AndSha1(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sha256sum) != expectedSHA256 {
			return fmt.Errorf("downloaded file sha256 mismatch")
		}
		if strings.ToLower(sha1sum) != expectedSHA1 {
			return fmt.Errorf("downloaded file sha1 mismatch")
		}
		return nil
	}

	if expectedSHA256 != "" {
		sum, err := util.Sha256File(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA256 {
			return fmt.Errorf("downloaded file sha256 mismatch")
		}
	}

	if expectedSHA1 != "" {
		sum, err := util.Sha1(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA1 {
			return fmt.Errorf("downloaded file sha1 mismatch")
		}
	}

	return nil
}

func buildManagedUpdateMessage(update pendingManagedUpdate, checkOnly bool) string {
	base := update.Label
	if update.App != nil {
		base = fmt.Sprintf("%s: %s", base, update.App.ID)
		if transition := updateVersionTransition(update); transition != "" {
			base = fmt.Sprintf("%s %s", base, transition)
		}
	} else if transition := updateVersionTransition(update); transition != "" {
		base = fmt.Sprintf("%s %s", base, transition)
	}

	if !checkOnly {
		return base
	}

	return base
}

func updateVersionTransition(update pendingManagedUpdate) string {
	if update.App == nil {
		return ""
	}

	current := displayVersion(update.App.Version)
	latestRaw := strings.TrimSpace(update.Latest)
	if latestRaw == "" {
		return formatVersionTransition(current, "unknown")
	}

	latest := displayVersion(latestRaw)
	return formatVersionTransition(current, latest)
}

func displayVersion(value string) string {
	v := strings.TrimSpace(strings.Trim(value, `"'`))
	if v == "" {
		return "unknown"
	}

	if strings.EqualFold(v, "dev") {
		return "dev"
	}

	if normalized := core.NormalizeComparableVersion(v); normalized != "" {
		v = normalized
	}

	v = strings.TrimSpace(v)
	lower := strings.ToLower(v)
	if lower == "" || lower == "n/a" || lower == "na" || lower == "none" || lower == "unknown" || lower == "-" {
		return "unknown"
	}
	if strings.HasPrefix(lower, "v") {
		return v
	}
	return "v" + v
}

func updateDownloadFilename(assetName, downloadURL string) string {
	name := strings.TrimSpace(filepath.Base(assetName))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = strings.TrimSpace(filepath.Base(downloadURL))
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "update.AppImage"
	}
	if !util.HasExtension(name, ".AppImage") {
		name = name + ".AppImage"
	}
	return name
}

func downloadUpdateAsset(ctx context.Context, assetURL, destination string, interactive bool) error {
	return downloadUpdateAssetWithProgress(ctx, assetURL, destination, interactive, nil)
}

func downloadUpdateAssetWithProgress(ctx context.Context, assetURL, destination string, interactive bool, onProgress func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("download failed with status %s", resp.Status)
	}

	f, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	var (
		total      = resp.ContentLength
		downloaded int64
		buffer     = make([]byte, 32*1024)
		frame      int
		lastDraw   time.Time
	)

	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, err := f.Write(buffer[:n]); err != nil {
				return err
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(downloaded, total)
			}
		}

		if interactive {
			now := time.Now()
			if now.Sub(lastDraw) >= 120*time.Millisecond || readErr == io.EOF {
				line := buildDownloadProgressLine(downloaded, total, frame)
				fmt.Printf("\r%s", line)
				lastDraw = now
				frame++
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	if interactive {
		line := buildDownloadProgressLine(downloaded, total, frame)
		fmt.Printf("\r%s\n", line)
	} else {
		if onProgress != nil {
			onProgress(downloaded, total)
		}
		if onProgress == nil {
			fmt.Printf("  Downloaded %s\n", formatByteSize(downloaded))
		}
	}

	return nil
}

func buildDownloadProgressLine(downloaded, total int64, frame int) string {
	if total > 0 {
		percent := float64(downloaded) / float64(total)
		if percent < 0 {
			percent = 0
		}
		if percent > 1 {
			percent = 1
		}

		barWidth := 24
		filled := int(percent * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		bar := strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled)
		return fmt.Sprintf("  Downloading [%s] %6.2f%% (%s/%s)", bar, percent*100, formatByteSize(downloaded), formatByteSize(total))
	}

	spinnerFrames := []string{"|", "/", "-", "\\"}
	frameLabel := spinnerFrames[frame%len(spinnerFrames)]
	return fmt.Sprintf("  Downloading %s %s", frameLabel, formatByteSize(downloaded))
}

func formatByteSize(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%dB", value)
	}

	units := []string{"KB", "MB", "GB", "TB"}
	size := float64(value)
	unit := -1
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}

	rounded := strconv.FormatFloat(size, 'f', 1, 64)
	return rounded + units[unit]
}

func isHTTPSURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return false
	}
	if strings.TrimSpace(u.Host) == "" {
		return false
	}
	return true
}

func useColor(cmd *cobra.Command) bool {
	_ = cmd
	return isTerminalOutput()
}

func isTerminalOutput() bool {
	return terminalOutputChecker()
}

func detectTerminalOutput() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) != 0
}

func colorize(enabled bool, code, value string) string {
	if !enabled {
		return value
	}

	return code + value + "\033[0m"
}

func printSuccess(cmd *cobra.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[0;32m", text))
}

func printWarning(cmd *cobra.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[0;33m", text))
}

func printError(cmd *cobra.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[0;31m", text))
}

func printInfo(cmd *cobra.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[0;36m", text))
}

func printSection(cmd *cobra.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[1m", text))
}

func confirmOverwrite(prompt string) (bool, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	answer := strings.TrimSpace(line)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func updateSummary(update *models.UpdateSource) string {
	if update == nil {
		return ""
	}

	switch update.Kind {
	case models.UpdateZsync:
		if update.Zsync == nil {
			return "zsync: <missing>"
		}
		if update.Zsync.UpdateInfo != "" {
			return fmt.Sprintf("zsync: %s", update.Zsync.UpdateInfo)
		}
		return "zsync"
	case models.UpdateGitHubRelease:
		if update.GitHubRelease == nil {
			return "github: <missing>"
		}
		return fmt.Sprintf("github: %s, asset: %s", update.GitHubRelease.Repo, update.GitHubRelease.Asset)
	case models.UpdateGitLabRelease:
		if update.GitLabRelease == nil {
			return "gitlab: <missing>"
		}
		return fmt.Sprintf("gitlab: %s, asset: %s", update.GitLabRelease.Project, update.GitLabRelease.Asset)
	default:
		return string(update.Kind)
	}
}

func formatAppRef(app *models.App) string {
	if app == nil {
		return "unknown"
	}
	return fmt.Sprintf("%s %s [%s]", app.Name, displayVersion(app.Version), app.ID)
}

func formatVersionTransition(current, latest string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		current = "unknown"
	}
	latest = strings.TrimSpace(latest)
	if latest == "" {
		latest = "unknown"
	}
	return fmt.Sprintf("%s -> %s", current, latest)
}

func showManagedUpdateAsset(asset string) bool {
	trimmed := strings.TrimSpace(asset)
	return trimmed != "" && trimmed != "update.AppImage"
}

const maxListNameColumnWidth = 28

func listIDColumnWidth(groups ...[]*models.App) int {
	width := len("ID")
	for _, group := range groups {
		for _, app := range group {
			if app == nil {
				continue
			}
			if l := len(app.ID); l > width {
				width = l
			}
		}
	}

	return width
}

func listNameDisplayWidth(groups ...[]*models.App) int {
	width := len("App Name")
	for _, group := range groups {
		for _, app := range group {
			if app == nil {
				continue
			}
			if l := len([]rune(strings.TrimSpace(app.Name))); l > width {
				width = l
			}
		}
	}
	if width > maxListNameColumnWidth {
		return maxListNameColumnWidth
	}
	return width
}

func formatListRow(app *models.App, idWidth, nameWidth int) string {
	if app == nil {
		return ""
	}

	return fmt.Sprintf(
		"%-*s %-*s %s",
		idWidth,
		app.ID,
		nameWidth,
		truncateForDisplay(app.Name, nameWidth),
		app.Version,
	)
}

func truncateForDisplay(value string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= width {
		return string(runes)
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}

	return string(runes[:width-3]) + "..."
}
