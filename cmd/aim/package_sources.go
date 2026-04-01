package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/slobbe/appimage-manager/internal/core"
	"github.com/slobbe/appimage-manager/internal/discovery"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/spf13/cobra"
)

func validateGitHubRepoFlag(value string) (discovery.PackageRef, error) {
	ref, err := discovery.ParseGitHubRepoValue(value)
	if err != nil {
		return discovery.PackageRef{}, usageError(fmt.Errorf("--github must be in owner/repo form"))
	}
	return ref, nil
}

func validateGitLabProjectFlag(value string) (discovery.PackageRef, error) {
	ref, err := discovery.ParseGitLabProjectValue(value)
	if err != nil {
		return discovery.PackageRef{}, usageError(fmt.Errorf("--gitlab must be in namespace/project form"))
	}
	return ref, nil
}

func resolveProviderFlagRef(cmd *cobra.Command, args []string) (discovery.PackageRef, bool, error) {
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
		return discovery.PackageRef{}, false, usageError(fmt.Errorf("--github and --gitlab are mutually exclusive"))
	}
	if len(args) > 0 {
		return discovery.PackageRef{}, false, usageError(fmt.Errorf("when using --github or --gitlab, do not pass a positional target"))
	}

	if hasGitHub {
		ref, err := validateGitHubRepoFlag(githubValue)
		return ref, true, err
	}

	ref, err := validateGitLabProjectFlag(gitlabValue)
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
		return usageError(fmt.Errorf("github:... refs are no longer accepted; use 'aim %s --github owner/repo'", cmdName))
	case strings.HasPrefix(trimmed, "gitlab:"):
		return usageError(fmt.Errorf("gitlab:... refs are no longer accepted; use 'aim %s --gitlab namespace/project'", cmdName))
	default:
		return usageError(fmt.Errorf("unsupported provider ref %q", input))
	}
}

func looksLikeGitHubPackageURL(input string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	return strings.HasPrefix(trimmed, "https://github.com/")
}

func looksLikeGitLabPackageURL(input string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	return strings.HasPrefix(trimmed, "https://gitlab.com/")
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
	return isLegacyPackageRef(trimmed)
}

func positionalAddRemoteGuidance(input string) error {
	trimmed := strings.TrimSpace(input)
	switch {
	case isLegacyPackageRef(trimmed):
		return legacyProviderRefGuidance("add", trimmed)
	case looksLikeGitHubPackageURL(trimmed):
		if err := providerURLGuidance("add", "--github", "GitHub sources", trimmed); err != nil {
			return err
		}
	case looksLikeGitLabPackageURL(trimmed):
		if err := providerURLGuidance("add", "--gitlab", "GitLab sources", trimmed); err != nil {
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
	case isLegacyPackageRef(trimmed):
		return legacyProviderRefGuidance("info", trimmed)
	case looksLikeGitHubPackageURL(trimmed):
		return providerURLGuidance("info", "--github", "GitHub package lookups", trimmed)
	case looksLikeGitLabPackageURL(trimmed):
		return providerURLGuidance("info", "--gitlab", "GitLab package lookups", trimmed)
	default:
		return nil
	}
}

func resolveInfoInput(cmd *cobra.Command, args []string) (string, discovery.PackageRef, bool, error) {
	ref, ok, err := resolveProviderFlagRef(cmd, args)
	if err != nil || ok {
		return "", ref, ok, err
	}
	value, err := resolveSingleInputOrPrompt(cmd, args, "<id|Path/To.AppImage>", "Managed app id or local AppImage path: ", missingInputErrorForInfo())
	if err != nil {
		if isMissingArgumentError(err) || err.Error() == missingInputErrorForInfo().Error() {
			return "", discovery.PackageRef{}, false, printConciseHelpError(cmd, missingInputErrorForInfo().Error())
		}
		return "", discovery.PackageRef{}, false, err
	}
	return value, discovery.PackageRef{}, false, nil
}

func resolvePackageRefInput(input string) (discovery.PackageRef, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return discovery.PackageRef{}, usageError(fmt.Errorf("missing package ref"))
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

	return nil, unavailableError(fmt.Errorf("no discovery backend available for %s", discovery.FormatPackageRef(ref)))
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
		return nil, unavailableError(fmt.Errorf("failed to resolve package metadata for %s", discovery.FormatPackageRef(ref)))
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

func resolvePackageMetadataWithProgress(cmd *cobra.Command, label string, resolve func() (*discovery.PackageMetadata, error)) (*discovery.PackageMetadata, error) {
	return runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", label), resolve)
}

func requireInstallablePackageMetadata(metadata *discovery.PackageMetadata) (*discovery.PackageMetadata, error) {
	if metadata != nil && metadata.Installable {
		return metadata, nil
	}
	if metadata == nil {
		return nil, softwareError(fmt.Errorf("package metadata cannot be empty"))
	}
	return nil, usageError(fmt.Errorf("package is not installable: %s", strings.TrimSpace(metadata.InstallReason)))
}

func resolveInstallablePackageMetadataFromTarget(ctx context.Context, cmd *cobra.Command, refArg string, target *installTarget, assetPattern string) (*discovery.PackageMetadata, error) {
	metadata, err := resolvePackageMetadataWithProgress(cmd, installTargetLabel(target), func() (*discovery.PackageMetadata, error) {
		return resolvePackageMetadataFromInput(ctx, refArg, assetPattern)
	})
	if err != nil {
		return nil, err
	}
	return requireInstallablePackageMetadata(metadata)
}

func resolveInstallablePackageMetadataFromRef(ctx context.Context, cmd *cobra.Command, ref discovery.PackageRef, assetPattern string) (*discovery.PackageMetadata, error) {
	metadata, err := resolvePackageMetadataWithProgress(cmd, formatProviderRef(ref), func() (*discovery.PackageMetadata, error) {
		return resolvePackageMetadataFromRef(ctx, ref, assetPattern)
	})
	if err != nil {
		return nil, err
	}
	return requireInstallablePackageMetadata(metadata)
}

func installPackageMetadata(ctx context.Context, cmd *cobra.Command, metadata *discovery.PackageMetadata) (*models.App, error) {
	if metadata == nil {
		return nil, softwareError(fmt.Errorf("package metadata cannot be empty"))
	}

	switch metadata.Ref.Kind {
	case discovery.ProviderGitHub:
		return installResolvedGitHubPackage(ctx, cmd, metadata)
	case discovery.ProviderGitLab:
		return installResolvedGitLabPackage(ctx, cmd, metadata)
	default:
		return nil, softwareError(fmt.Errorf("unsupported add provider %q", metadata.Ref.Kind))
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

func installTargetLabel(target *installTarget) string {
	if target == nil {
		return "package"
	}

	switch target.Kind {
	case installTargetGitHub:
		if value := strings.TrimSpace(target.Repo); value != "" {
			return "GitHub " + value
		}
	case installTargetGitLab:
		if value := strings.TrimSpace(target.Project); value != "" {
			return "GitLab " + value
		}
	case installTargetDirectURL:
		if value := strings.TrimSpace(target.URL); value != "" {
			return value
		}
	}

	return "package"
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
