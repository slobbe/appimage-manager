package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app/discovery"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	models "github.com/slobbe/appimage-manager/internal/domain"
	"github.com/spf13/cobra"
)

func validateGitHubRepoFlag(value string) (models.PackageRef, error) {
	ref, err := discovery.ParseGitHubRepoValue(value)
	if err != nil {
		return models.PackageRef{}, usageError(fmt.Errorf("--github must be in owner/repo form"))
	}
	return ref, nil
}

func resolveProviderFlagRef(cmd *cobra.Command, args []string) (models.PackageRef, bool, error) {
	githubValue, err := flagString(cmd, "github")
	if err != nil {
		return models.PackageRef{}, false, err
	}

	hasGitHub := strings.TrimSpace(githubValue) != ""

	if !hasGitHub {
		return models.PackageRef{}, false, nil
	}
	if len(args) > 0 {
		return models.PackageRef{}, false, usageError(fmt.Errorf("when using --github, do not pass a positional target"))
	}

	ref, err := validateGitHubRepoFlag(githubValue)
	return ref, true, err
}

func looksLikeGitHubPackageURL(input string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	return strings.HasPrefix(trimmed, "https://github.com/")
}

func providerURLGuidance(cmdName, providerFlag, subject, input string) error {
	ref, err := discovery.ParsePackageRefURL(input)
	if err != nil {
		return nil
	}
	return usageError(fmt.Errorf("%s must use %s; use 'aim %s %s %s'", subject, providerFlag, cmdName, providerFlag, ref.ProviderRef))
}

func addTargetLooksRemote(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "http://") || strings.HasPrefix(strings.ToLower(trimmed), "https://") {
		return true
	}
	return false
}

func positionalAddRemoteGuidance(input string) error {
	trimmed := strings.TrimSpace(input)
	switch {
	case looksLikeGitHubPackageURL(trimmed):
		if err := providerURLGuidance("add", "--github", "GitHub sources", trimmed); err != nil {
			return err
		}
	case strings.HasPrefix(strings.ToLower(trimmed), "http://"):
		return usageError(fmt.Errorf("direct URLs must use https and be passed with --url; use 'aim add --url https://...'"))
	case isHTTPSURL(trimmed):
		return usageError(fmt.Errorf("direct URLs must be passed with --url; use 'aim add --url %s'", trimmed))
	}
	return usageError(fmt.Errorf("unknown add target %q", input))
}

func positionalInfoRemoteGuidance(input string) error {
	trimmed := strings.TrimSpace(input)
	switch {
	case looksLikeGitHubPackageURL(trimmed):
		return providerURLGuidance("info", "--github", "GitHub package lookups", trimmed)
	default:
		return nil
	}
}

func resolveInfoInput(cmd *cobra.Command, args []string) (string, models.PackageRef, bool, error) {
	ref, ok, err := resolveProviderFlagRef(cmd, args)
	if err != nil || ok {
		return "", ref, ok, err
	}
	value, err := resolveSingleInputOrPrompt(cmd, args, "<id|Path/To.AppImage>", "Managed app id or local AppImage path: ", missingInputErrorForInfo())
	if err != nil {
		if isMissingArgumentError(err) || err.Error() == missingInputErrorForInfo().Error() {
			return "", models.PackageRef{}, false, printConciseHelpError(cmd, missingInputErrorForInfo().Error())
		}
		return "", models.PackageRef{}, false, err
	}
	return value, models.PackageRef{}, false, nil
}

func resolvePackageRefInput(input string) (models.PackageRef, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return models.PackageRef{}, usageError(fmt.Errorf("missing package ref"))
	}

	return discovery.ParsePackageRefURL(trimmed)
}

func backendForRef(ref models.PackageRef) (discovery.DiscoveryBackend, error) {
	for _, backend := range discoveryBackends() {
		switch {
		case ref.Kind == models.ProviderGitHub && strings.EqualFold(backend.Name(), "GitHub"):
			return backend, nil
		}
	}

	return nil, unavailableError(fmt.Errorf("no discovery backend available for %s", models.FormatPackageRef(ref)))
}

func resolvePackageMetadataFromRef(ctx context.Context, ref models.PackageRef, assetOverride string) (*models.PackageMetadata, error) {
	backend, err := backendForRef(ref)
	if err != nil {
		return nil, err
	}

	metadata, err := backend.Resolve(ctx, ref, assetOverride)
	if err != nil {
		return nil, err
	}
	if metadata == nil {
		return nil, unavailableError(fmt.Errorf("failed to resolve package metadata for %s", models.FormatPackageRef(ref)))
	}

	return metadata, nil
}

func resolvePackageMetadataFromInput(ctx context.Context, input, assetOverride string) (*models.PackageMetadata, error) {
	ref, err := resolvePackageRefInput(input)
	if err != nil {
		return nil, err
	}

	return resolvePackageMetadataFromRef(ctx, ref, assetOverride)
}

func resolvePackageMetadataWithProgress(cmd *cobra.Command, label string, resolve func() (*models.PackageMetadata, error)) (*models.PackageMetadata, error) {
	return runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", label), resolve)
}

func requireInstallablePackageMetadata(metadata *models.PackageMetadata) (*models.PackageMetadata, error) {
	if metadata != nil && metadata.Installable {
		return metadata, nil
	}
	if metadata == nil {
		return nil, softwareError(fmt.Errorf("package metadata cannot be empty"))
	}
	return nil, usageError(fmt.Errorf("package is not installable: %s", strings.TrimSpace(metadata.InstallReason)))
}

func resolveGitHubAssetAmbiguity(cmd *cobra.Command, metadata *models.PackageMetadata) (*models.PackageMetadata, error) {
	if metadata == nil || !metadata.AssetAmbiguous {
		return metadata, nil
	}
	if runtimeOptionsFrom(cmd).JSON || !canPromptForInput(cmd) {
		return nil, usageError(fmt.Errorf("%s; use --asset to choose one of: %s", assetAmbiguityReason(metadata), strings.Join(assetCandidateNames(metadata.AssetCandidates), ", ")))
	}

	writeDataf(cmd, "Multiple AppImage assets match this release:\n")
	for i, candidate := range metadata.AssetCandidates {
		label := strings.TrimSpace(candidate.ArchLabel)
		if label == "" {
			label = "generic"
		}
		writeDataf(cmd, "  %d. %s (%s)\n", i+1, candidate.Name, label)
	}
	writeDataf(cmd, "\n")

	value, err := readPromptedValue(cmd, fmt.Sprintf("Select asset [1-%d]: ", len(metadata.AssetCandidates)))
	if err != nil {
		return nil, err
	}
	index, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || index < 1 || index > len(metadata.AssetCandidates) {
		return nil, usageError(fmt.Errorf("invalid asset selection %q", value))
	}

	selected := metadata.AssetCandidates[index-1]
	resolved := *metadata
	resolved.AssetName = selected.Name
	resolved.DownloadURL = selected.DownloadURL
	resolved.AssetAmbiguous = false
	resolved.AssetReason = ""
	return &resolved, nil
}

func assetAmbiguityReason(metadata *models.PackageMetadata) string {
	if metadata != nil && strings.TrimSpace(metadata.AssetReason) != "" {
		return strings.TrimSpace(metadata.AssetReason)
	}
	return "multiple assets match"
}

func assetCandidateNames(candidates []models.AssetCandidate) []string {
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Name) != "" {
			names = append(names, candidate.Name)
		}
	}
	return names
}

func resolveInstallablePackageMetadataFromTarget(ctx context.Context, cmd *cobra.Command, refArg string, target *installTarget, assetPattern string) (*models.PackageMetadata, error) {
	metadata, err := resolvePackageMetadataWithProgress(cmd, installTargetLabel(target), func() (*models.PackageMetadata, error) {
		return resolvePackageMetadataFromInput(ctx, refArg, assetPattern)
	})
	if err != nil {
		return nil, err
	}
	metadata, err = requireInstallablePackageMetadata(metadata)
	if err != nil {
		return nil, err
	}
	return resolveGitHubAssetAmbiguity(cmd, metadata)
}

func resolveInstallablePackageMetadataFromRef(ctx context.Context, cmd *cobra.Command, ref models.PackageRef, assetPattern string) (*models.PackageMetadata, error) {
	metadata, err := resolvePackageMetadataWithProgress(cmd, formatProviderRef(ref), func() (*models.PackageMetadata, error) {
		return resolvePackageMetadataFromRef(ctx, ref, assetPattern)
	})
	if err != nil {
		return nil, err
	}
	metadata, err = requireInstallablePackageMetadata(metadata)
	if err != nil {
		return nil, err
	}
	return resolveGitHubAssetAmbiguity(cmd, metadata)
}

func installPackageMetadata(ctx context.Context, cmd *cobra.Command, metadata *models.PackageMetadata) (*models.App, error) {
	if metadata == nil {
		return nil, softwareError(fmt.Errorf("package metadata cannot be empty"))
	}

	switch metadata.Ref.Kind {
	case models.ProviderGitHub:
		return installResolvedGitHubPackage(ctx, cmd, metadata)
	default:
		return nil, softwareError(fmt.Errorf("unsupported add provider %q", metadata.Ref.Kind))
	}
}

func installResolvedGitHubPackage(ctx context.Context, cmd *cobra.Command, metadata *models.PackageMetadata) (*models.App, error) {
	release := &appupdate.GitHubReleaseAsset{
		DownloadURL:       strings.TrimSpace(metadata.DownloadURL),
		TagName:           strings.TrimSpace(metadata.ReleaseTag),
		NormalizedVersion: strings.TrimSpace(strings.TrimPrefix(metadata.LatestVersion, "v")),
		AssetName:         strings.TrimSpace(metadata.AssetName),
	}
	target := &installTarget{Kind: installTargetGitHub, Repo: metadata.Ref.ProviderRef}
	return integrateGitHubReleaseAsset(ctx, cmd, target, metadata.AssetPattern, release)
}

func printPackageMetadata(cmd *cobra.Command, metadata *models.PackageMetadata) {
	printSection(cmd, metadata.Name)
	writeDataf(cmd, "Provider: %s\n", strings.TrimSpace(metadata.Provider))
	if providerRef := formatProviderRef(metadata.Ref); providerRef != "" {
		writeDataf(cmd, "Provider ref: %s\n", providerRef)
	}
	if strings.TrimSpace(metadata.RepoURL) != "" {
		writeDataf(cmd, "Source URL: %s\n", strings.TrimSpace(metadata.RepoURL))
	}
	if strings.TrimSpace(metadata.Summary) != "" {
		writeDataf(cmd, "Summary: %s\n", strings.TrimSpace(metadata.Summary))
	}

	installable := strings.TrimSpace(metadata.InstallReason) == "" && metadata.Installable
	writeDataf(cmd, "Installable: %s\n", yesNo(installable))

	if !installable && strings.TrimSpace(metadata.InstallReason) != "" {
		writeDataf(cmd, "Reason: %s\n", strings.TrimSpace(metadata.InstallReason))
		return
	}

	if strings.TrimSpace(metadata.LatestVersion) != "" {
		writeDataf(cmd, "Latest release: %s\n", displayVersion(metadata.LatestVersion))
	}
	if strings.TrimSpace(metadata.AssetName) != "" {
		writeDataf(cmd, "Selected asset: %s\n", strings.TrimSpace(metadata.AssetName))
	}
	writeDataf(cmd, "Managed updates: yes\n")

	printSection(cmd, "Install Command")
	writeDataf(cmd, "  %s\n", formatAddProviderCommand(metadata.Ref))
}

func formatProviderRef(ref models.PackageRef) string {
	value := strings.TrimSpace(ref.ProviderRef)
	if value == "" {
		return ""
	}

	switch ref.Kind {
	case models.ProviderGitHub:
		return "GitHub " + value
	default:
		return value
	}
}

func installTargetLabel(target *installTarget) string {
	if target == nil {
		return "package"
	}

	switch target.Kind {
	case installTargetGitHub:
		if value := strings.TrimSpace(target.Repo); value != "" {
			return "GitHub " + value
		}
	case installTargetDirectURL:
		if value := strings.TrimSpace(target.URL); value != "" {
			return value
		}
	}

	return "package"
}

func formatAddProviderCommand(ref models.PackageRef) string {
	value := strings.TrimSpace(ref.ProviderRef)
	if value == "" {
		return "aim add"
	}

	switch ref.Kind {
	case models.ProviderGitHub:
		return "aim add --github " + value
	default:
		return "aim add"
	}
}
