package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app/discovery"
	appservices "github.com/slobbe/appimage-manager/internal/app/services"
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

func resolvePackageInfoWithProgress(cmd *cobra.Command, label string, resolve func() (*appservices.InfoResult, error)) (*appservices.InfoResult, error) {
	return runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", label), resolve)
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

func resolvePackageViewAmbiguity(cmd *cobra.Command, metadata *appservices.PackageView) (*appservices.PackageView, error) {
	if metadata == nil || !metadata.AssetAmbiguous {
		return metadata, nil
	}
	if runtimeOptionsFrom(cmd).JSON || !canPromptForInput(cmd) {
		return nil, usageError(fmt.Errorf("%s; use --asset to choose one of: %s", packageViewAssetAmbiguityReason(metadata), strings.Join(packageViewAssetCandidateNames(metadata.AssetCandidates), ", ")))
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

func packageViewAssetAmbiguityReason(metadata *appservices.PackageView) string {
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

func packageViewAssetCandidateNames(candidates []appservices.AssetCandidateView) []string {
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Name) != "" {
			names = append(names, candidate.Name)
		}
	}
	return names
}

func printPackageView(cmd *cobra.Command, metadata *appservices.PackageView) {
	if metadata == nil {
		return
	}
	printSection(cmd, metadata.Name)
	writeDataf(cmd, "Provider: %s\n", strings.TrimSpace(metadata.Provider))
	if providerRef := formatProviderViewRef(metadata.Ref); providerRef != "" {
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
	writeDataf(cmd, "  %s\n", formatAddProviderViewCommand(metadata.Ref))
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

func formatProviderViewRef(ref appservices.ProviderRef) string {
	value := strings.TrimSpace(ref.Ref)
	if value == "" {
		return ""
	}

	switch strings.TrimSpace(ref.Provider) {
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

func formatAddProviderViewCommand(ref appservices.ProviderRef) string {
	value := strings.TrimSpace(ref.Ref)
	if value == "" {
		return "aim add"
	}

	switch strings.TrimSpace(ref.Provider) {
	case models.ProviderGitHub:
		return "aim add --github " + value
	default:
		return "aim add"
	}
}
