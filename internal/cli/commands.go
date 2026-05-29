package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	appservices "github.com/slobbe/appimage-manager/internal/app/services"
	"github.com/spf13/cobra"
	"net/url"
	"path/filepath"
	"strings"
)

func RootCmd(cmd *cobra.Command, args []string) error {
	upgrade, err := cmd.Flags().GetBool("upgrade")
	if err != nil {
		return usageError(err)
	}
	if upgrade {
		if len(args) > 0 {
			return usageError(fmt.Errorf("--upgrade does not accept positional arguments"))
		}
		if err := mustEnsureRuntimeDirs(); err != nil {
			return err
		}
		return runUpgrade(cmd.Context(), cmd)
	}

	writeDataf(cmd, "%s", renderConciseHelp(cmd))
	return nil
}

func runUpgrade(ctx context.Context, cmd *cobra.Command) error {
	if ctx == nil {
		ctx = context.Background()
	}

	logOperationf(cmd, "Checking for aim updates")
	result, err := runWithBusyIndicator(cmd, progressUpgradeAim(), func() (*appservices.UpgradeResult, error) {
		return runtimeServicesFrom(cmd).Upgrade.Upgrade(ctx, appservices.UpgradeRequest{CurrentVersion: version})
	})
	if err != nil {
		return err
	}
	return renderUpgradeResult(cmd, result)
}

func renderUpgradeResult(cmd *cobra.Command, result *appservices.UpgradeResult) error {
	if result != nil && result.UpToDate {
		current := result.LatestVersion
		if strings.TrimSpace(current) == "" {
			current = result.CurrentVersion
		}
		printSuccess(cmd, fmt.Sprintf("aim is up to date (%s)", displayVersion(current)))
		return nil
	}
	if result != nil && strings.TrimSpace(result.InstalledVersion) != "" {
		printSuccess(cmd, fmt.Sprintf(
			"Upgraded aim %s -> %s",
			displayVersion(result.CurrentVersion),
			displayVersion(result.InstalledVersion),
		))
		return nil
	}
	printSuccess(cmd, "Upgraded aim")
	return nil
}

func AddCmd(cmd *cobra.Command, args []string) error {
	selection, err := resolveAddInput(cmd, args)
	if err != nil {
		return err
	}

	run := func() error {
		if selection.HasRef {
			return runInstallPackageRef(cmd.Context(), cmd, selection.Ref)
		}
		if strings.TrimSpace(selection.DirectURL) != "" {
			return runInstallTarget(cmd.Context(), cmd, selection.DirectURL)
		}
		if _, err := runtimeServicesFrom(cmd).Add.ResolveIntegrateTarget(cmd.Context(), selection.Positional); err == nil {
			if err := validateAddIntegrateFlags(cmd); err != nil {
				return err
			}
			return runIntegrateTarget(cmd.Context(), cmd, selection.Positional)
		}

		return usageError(fmt.Errorf("unknown add target %q; expected <id> or <Path/To.AppImage>", selection.Positional))
	}

	if runtimeOptionsFrom(cmd).DryRun {
		return run()
	}
	return withStateWriteLock(cmd, run)
}

type addInputSelection struct {
	Positional string
	DirectURL  string
	Ref        appservices.ProviderRef
	HasRef     bool
}

func resolveAddInput(cmd *cobra.Command, args []string) (addInputSelection, error) {
	ref, ok, err := resolveProviderFlagRef(cmd, args)
	if err != nil || ok {
		return addInputSelection{Ref: ref, HasRef: ok}, err
	}

	urlValue, err := flagString(cmd, "url")
	if err != nil {
		return addInputSelection{}, err
	}

	targetCount := 0
	if len(args) > 0 {
		targetCount++
	}
	if strings.TrimSpace(urlValue) != "" {
		targetCount++
	}
	if targetCount > 1 {
		return addInputSelection{}, usageError(fmt.Errorf("choose exactly one add selector: positional target, --url, or --github"))
	}

	if strings.TrimSpace(urlValue) != "" {
		target, err := resolveInstallTarget(urlValue)
		if err != nil {
			return addInputSelection{}, err
		}
		if err := validateInstallTargetFlags(cmd, target); err != nil {
			return addInputSelection{}, err
		}
		return addInputSelection{DirectURL: target.URL}, nil
	}

	value, err := resolveSingleInputOrPrompt(cmd, args, "<id|Path/To.AppImage>", "Local AppImage path or managed app id: ", missingInputErrorForAdd())
	if err != nil {
		if isMissingArgumentError(err) || err.Error() == missingInputErrorForAdd().Error() {
			return addInputSelection{}, printConciseHelpError(cmd, missingInputErrorForAdd().Error())
		}
		return addInputSelection{}, err
	}

	if addTargetLooksRemote(value) {
		return addInputSelection{}, positionalAddRemoteGuidance(value)
	}

	return addInputSelection{Positional: value}, nil
}

func runIntegrateTarget(ctx context.Context, cmd *cobra.Command, input string) error {
	target, err := runtimeServicesFrom(cmd).Add.ResolveIntegrateTarget(ctx, input)
	if err != nil {
		return usageError(err)
	}
	opts := runtimeOptionsFrom(cmd)

	switch target.Kind {
	case appservices.IntegrateTargetIntegrated:
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{
				"status": "already_integrated",
				"app":    target.App,
			})
		}
		printSuccess(cmd, fmt.Sprintf("Already integrated: %s", formatAppDetailsRef(target.App)))
		return nil
	case appservices.IntegrateTargetUnlinked:
		if opts.DryRun {
			result := map[string]interface{}{
				"status": "dry_run",
				"action": "reintegrate",
				"app":    target.App,
			}
			if opts.JSON {
				return printJSONSuccess(cmd, result)
			}
			if target.App == nil {
				return softwareError(fmt.Errorf("reintegrate target missing app"))
			}
			writeDataf(cmd, "Dry run: would reintegrate %s [%s]\n", target.App.Name, target.App.ID)
			return nil
		}
		if err := mustEnsureRuntimeDirs(); err != nil {
			return err
		}
		if target.App == nil {
			return softwareError(fmt.Errorf("reintegrate target missing app"))
		}
		result, err := runtimeServicesFrom(cmd).Add.Reintegrate(ctx, target.App.ID)
		if err != nil {
			return err
		}
		app := result.App
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{
				"status": "reintegrated",
				"app":    app,
			})
		}
		printSuccess(cmd, fmt.Sprintf("Reintegrated: %s", formatAppDetailsRef(app)))
		return nil
	case appservices.IntegrateTargetLocalFile:
		if opts.DryRun {
			plan, err := runtimeServicesFrom(cmd).Add.PlanLocalIntegration(ctx, target.LocalPath)
			if err != nil {
				return err
			}
			if opts.JSON {
				return printJSONSuccess(cmd, plan.Values)
			}
			writeDataf(cmd, "Dry run: would integrate %s\n", plan.Values["input"])
			if appID, ok := plan.Values["app_id"].(string); ok && appID != "" {
				writeDataf(cmd, "  Managed ID: %s\n", appID)
			}
			for _, path := range plan.Values["planned_paths"].([]string) {
				writeDataf(cmd, "  %s\n", path)
			}
			return nil
		}
		if err := mustEnsureRuntimeDirs(); err != nil {
			return err
		}
		inputLabel := strings.TrimSpace(filepath.Base(target.LocalPath))
		if inputLabel == "" || inputLabel == "." || inputLabel == string(filepath.Separator) {
			inputLabel = strings.TrimSpace(target.LocalPath)
		}

		app, err := runWithBusyIndicator(cmd, fmt.Sprintf("Integrating %s", inputLabel), func() (*appservices.AppDetails, error) {
			result, err := runtimeServicesFrom(cmd).Add.IntegrateLocal(ctx, appservices.IntegrateLocalRequest{
				Path: target.LocalPath,
				ConfirmUpdateSourceReplace: updateSourceReplaceConfirmerFunc(func(existing, incoming *appservices.UpdateSourceView) (bool, error) {
					printCurrentIncoming(cmd, updateSourceViewSummaryOrNone(existing), updateSourceViewSummaryOrNone(incoming))
					prompt := formatPrompt("Replace source from", "AppImage metadata")
					return confirmAction(cmd, prompt)
				}),
			})
			if result == nil {
				return nil, err
			}
			return result.App, err
		})
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{
				"status": "integrated",
				"app":    app,
			})
		}
		printSuccess(cmd, fmt.Sprintf("Integrated: %s", formatAppDetailsRef(app)))
		return nil
	default:
		return softwareError(fmt.Errorf("unknown integrate target %q", input))
	}
}

type installTargetKind string

const (
	installTargetDirectURL installTargetKind = "direct_url"
	installTargetGitHub    installTargetKind = "github_release"
)

type installTarget struct {
	Kind installTargetKind
	URL  string
	Repo string
}

func resolveInstallTarget(input string) (*installTarget, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, usageError(fmt.Errorf("missing required argument <url>"))
	}

	if isHTTPSURL(trimmed) {
		return &installTarget{Kind: installTargetDirectURL, URL: trimmed}, nil
	}

	if strings.HasPrefix(strings.ToLower(trimmed), "http://") {
		return nil, usageError(fmt.Errorf("--url must use https"))
	}

	if runtimeHasExtension(trimmed, ".AppImage") {
		return nil, usageError(fmt.Errorf("local AppImages are added with 'aim add <Path/To.AppImage>'"))
	}

	return nil, usageError(fmt.Errorf("--url must be a valid https URL; managed app IDs are added with 'aim add <id>'"))
}

func validateInstallTargetFlags(cmd *cobra.Command, target *installTarget) error {
	if target == nil {
		return usageError(fmt.Errorf("missing add target"))
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
	case installTargetGitHub:
		if sha256 != "" {
			return usageError(fmt.Errorf("--sha256 is only supported with direct https URLs"))
		}
	case installTargetDirectURL:
		if assetPattern != "" {
			return usageError(fmt.Errorf("--asset is only supported with GitHub provider sources"))
		}
		if sha256 != "" && !isSHA256Hex(sha256) {
			return usageError(fmt.Errorf("--sha256 must be a valid 64-character hexadecimal SHA-256"))
		}
	default:
		return softwareError(fmt.Errorf("unsupported add target"))
	}

	return nil
}

type packageAmbiguityResolverFunc func(*appservices.PackageView) (*appservices.PackageView, error)

func (fn packageAmbiguityResolverFunc) ResolvePackageViewAmbiguity(metadata *appservices.PackageView) (*appservices.PackageView, error) {
	return fn(metadata)
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
	id, err := resolveSingleInputOrPrompt(cmd, args, "<id>", "Managed app id to remove: ", missingInputErrorForRemove())
	if err != nil {
		if isMissingArgumentError(err) || err.Error() == missingInputErrorForRemove().Error() {
			return printConciseHelpError(cmd, missingInputErrorForRemove().Error())
		}
		return err
	}
	unlink, err := flagBool(cmd, "link")
	if err != nil {
		return err
	}

	opts := runtimeOptionsFrom(cmd)
	return runCommand(cmd, commandRunOptions{RequiresRuntimeDirs: !opts.DryRun, RequiresWriteLock: !opts.DryRun}, func(ctx context.Context) (*appservices.RemoveResult, error) {
		if !opts.DryRun {
			logOperationf(cmd, "Removing %s", id)
		}
		result, err := runtimeServicesFrom(cmd).Remove.Remove(ctx, appservices.RemoveRequest{ID: id, Unlink: unlink, DryRun: opts.DryRun})
		if err != nil {
			return nil, wrapManagedAppLookupError(id, err)
		}
		return result, nil
	}, func(result *appservices.RemoveResult) error {
		if result == nil || result.App == nil {
			if opts.DryRun {
				return softwareError(fmt.Errorf("remove dry-run plan missing app"))
			}
			return softwareError(fmt.Errorf("remove result missing app"))
		}
		if opts.DryRun {
			if opts.JSON {
				return printJSONSuccess(cmd, map[string]interface{}{
					"action": result.Action,
					"unlink": unlink,
					"app":    result.App,
					"paths":  result.Paths,
				})
			}
			writeDataf(cmd, "Dry run: would %s %s [%s]\n", result.Action, result.App.Name, result.App.ID)
			for _, path := range result.Paths {
				writeDataf(cmd, "  %s\n", path)
			}
			return nil
		}

		label := "Removed"
		if unlink {
			label = "Unlinked"
		}
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{
				"action": strings.ToLower(label),
				"app":    result.App,
				"unlink": result.Unlink,
				"paths":  result.Paths,
			})
		}
		printSuccess(cmd, fmt.Sprintf("%s: %s [%s]", label, result.App.Name, result.App.ID))
		return nil
	})
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
		return usageError(fmt.Errorf("flags --all, --integrated, and --unlinked are mutually exclusive"))
	}

	if !all && !integrated && !unlinked {
		all = true
	}

	filter := appservices.ListAll
	if integrated {
		filter = appservices.ListIntegrated
	}
	if unlinked {
		filter = appservices.ListUnlinked
	}

	result, err := runtimeServicesFrom(cmd).List.List(cmd.Context(), appservices.ListRequest{Filter: filter})
	if err != nil {
		return err
	}

	opts := runtimeOptionsFrom(cmd)
	selected := sortAppDetailsByID(appDetailsByID(result.Apps))
	if len(selected) == 0 {
		if opts.JSON {
			return printJSONSuccess(cmd, []listOutputRow{})
		}
		if opts.CSV {
			return writeCSV(cmd, listCSVHeader(), nil)
		}
		if opts.Plain {
			writePlainList(cmd, nil)
			return nil
		}
		if filter == appservices.ListIntegrated && result.TotalCount > 0 {
			printSuccess(cmd, "No integrated apps")
			return nil
		}
		if filter == appservices.ListUnlinked && result.TotalCount > 0 {
			printSuccess(cmd, "No unlinked apps")
			return nil
		}
		printSuccess(cmd, "No managed apps")
		return nil
	}

	integratedRows := make([]*appservices.AppDetails, 0, len(selected))
	unlinkedRows := make([]*appservices.AppDetails, 0, len(selected))
	for _, app := range selected {
		if app.Integrated {
			integratedRows = append(integratedRows, app)
			continue
		}
		unlinkedRows = append(unlinkedRows, app)
	}

	if opts.JSON {
		rows := make([]listOutputRow, 0, len(selected))
		for _, app := range selected {
			rows = append(rows, newListOutputRow(app))
		}
		return printJSONSuccess(cmd, rows)
	}
	if opts.CSV {
		rows := make([][]string, 0, len(selected))
		for _, app := range selected {
			rows = append(rows, newListOutputRow(app).csvRow())
		}
		return writeCSV(cmd, listCSVHeader(), rows)
	}
	if opts.Plain {
		writePlainList(cmd, selected)
		return nil
	}

	idWidth := listIDColumnWidth(integratedRows, unlinkedRows)
	nameWidth := listNameDisplayWidth(integratedRows, unlinkedRows)
	header := fmt.Sprintf("%-*s %-*s %s", idWidth, "ID", nameWidth, "App Name", "Version")
	printSection(cmd, header)

	if all || integrated {
		for _, app := range integratedRows {
			writeDataf(cmd, "%s\n", formatListRow(app, idWidth, nameWidth))
		}
	}

	if all || unlinked {
		for _, app := range unlinkedRows {
			row := formatListRow(app, idWidth, nameWidth)
			writeDataf(cmd, "%s\n", colorize(shouldColorStdout(cmd), "\033[2m\033[3m", row))
		}
	}

	return nil
}

func InfoCmd(cmd *cobra.Command, args []string) error {
	input, ref, ok, err := resolveInfoInput(cmd, args)
	if err != nil {
		return err
	}
	if ok {
		return runShowPackageRef(cmd.Context(), cmd, ref)
	}

	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return usageError(fmt.Errorf("missing required argument <id|Path/To.AppImage>"))
	}
	if runtimeHasExtension(trimmed, ".AppImage") {
		return inspectLocalAppImage(cmd.Context(), cmd, trimmed)
	}
	if refErr := positionalInfoRemoteGuidance(trimmed); refErr != nil {
		return refErr
	}
	if err := inspectManagedApp(cmd.Context(), cmd, trimmed); err != nil {
		return usageError(fmt.Errorf("unknown info target %q; expected <id> or <Path/To.AppImage>", trimmed))
	}
	return nil
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func runShowPackageRef(ctx context.Context, cmd *cobra.Command, ref appservices.ProviderRef) error {
	result, err := resolvePackageInfoWithProgress(cmd, appservices.FormatProviderRef(ref), func() (*appservices.InfoResult, error) {
		return runtimeServicesFrom(cmd).Info.PackageRefInfo(ctx, appservices.PackageRefInfoRequest{Ref: ref})
	})
	if err != nil {
		return err
	}
	metadata := result.PackageView
	if runtimeOptionsFrom(cmd).JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"kind":     "package_metadata",
			"metadata": metadata,
		})
	}
	metadata, err = resolvePackageViewAmbiguity(cmd, metadata)
	if err != nil {
		return err
	}

	printPackageView(cmd, metadata)
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

	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}
	opts := runtimeOptionsFrom(cmd)

	if opts.DryRun {
		plan, err := runtimeServicesFrom(cmd).Add.PlanDirectURLInstall(ctx, appservices.InstallDirectURLRequest{URL: target.URL, SHA256: sha256})
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSONSuccess(cmd, plan.Values)
		}
		writeDataf(cmd, "Dry run: would install %s\n", plan.Values["target"])
		return nil
	}
	if err := mustEnsureRuntimeDirs(); err != nil {
		return err
	}

	if strings.TrimSpace(sha256) == "" {
		printWarning(cmd, "No SHA-256 provided; skipping checksum verification")
	}
	result, err := runtimeServicesFrom(cmd).Add.InstallDirectURL(ctx, appservices.InstallDirectURLRequest{URL: target.URL, SHA256: sha256})
	if err != nil {
		return err
	}
	app := result.App

	if opts.JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"status": "installed",
			"app":    app,
		})
	}
	printSuccess(cmd, fmt.Sprintf("Installed: %s", formatAppDetailsRef(app)))
	return nil
}

func runInstallPackageRef(ctx context.Context, cmd *cobra.Command, ref appservices.ProviderRef) error {
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}
	if sha256 != "" {
		return usageError(fmt.Errorf("--sha256 is only supported with direct https URLs"))
	}
	opts := runtimeOptionsFrom(cmd)

	if opts.DryRun {
		plan, err := resolvePackagePlanWithProgress(cmd, appservices.FormatProviderRef(ref), func() (*appservices.DryRunPlan, error) {
			return runtimeServicesFrom(cmd).Add.PlanPackageRefInstall(ctx, appservices.InstallPackageRefRequest{Ref: ref, AssetPattern: assetPattern})
		})
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSONSuccess(cmd, plan.Values)
		}
		writeDataf(cmd, "Dry run: would install %s\n", appservices.FormatProviderRef(ref))
		return nil
	}
	if err := mustEnsureRuntimeDirs(); err != nil {
		return err
	}

	result, err := runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", appservices.FormatProviderRef(ref)), func() (*appservices.AddResult, error) {
		return runtimeServicesFrom(cmd).Add.InstallPackageRef(ctx, appservices.InstallPackageRefRequest{
			Ref:          ref,
			AssetPattern: assetPattern,
			ResolveViewAmbiguity: packageAmbiguityResolverFunc(func(metadata *appservices.PackageView) (*appservices.PackageView, error) {
				resolved, err := resolvePackageViewAmbiguity(cmd, metadata)
				if err != nil {
					return nil, err
				}
				if resolved != nil && strings.TrimSpace(resolved.AssetName) != "" {
					writeDataf(cmd, "Integrating %s...\n", strings.TrimSpace(resolved.AssetName))
				}
				return resolved, nil
			}),
		})
	})
	if err != nil {
		return err
	}
	app := result.App

	if opts.JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"status": "installed",
			"app":    app,
		})
	}
	printSuccess(cmd, fmt.Sprintf("Installed: %s", formatAppDetailsRef(app)))
	return nil
}

func commandSingleArg(args []string, usage string) (string, error) {
	if len(args) > 1 {
		return "", usageError(fmt.Errorf("too many arguments"))
	}

	value := ""
	if len(args) > 0 {
		value = strings.TrimSpace(args[0])
	}
	if value == "" {
		return "", usageError(fmt.Errorf("missing required argument %s", usage))
	}

	return value, nil
}

func validateAddIntegrateFlags(cmd *cobra.Command) error {
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	if assetPattern != "" {
		return usageError(fmt.Errorf("--asset is only supported with GitHub provider sources"))
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}
	if sha256 != "" {
		return usageError(fmt.Errorf("--sha256 is only supported with direct https:// add sources"))
	}

	return nil
}

func inspectManagedApp(ctx context.Context, cmd *cobra.Command, id string) error {
	info, err := runtimeServicesFrom(cmd).Info.ManagedAppInfo(ctx, id)
	if err != nil {
		return wrapManagedAppLookupError(id, err)
	}
	app := info.AppDetails
	if app == nil {
		return fmt.Errorf("managed app cannot be empty")
	}
	embeddedSource := info.EmbeddedUpdate

	if runtimeOptionsFrom(cmd).JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"kind":            "managed_app",
			"app":             app,
			"embedded_update": embeddedSource,
		})
	}

	printSection(cmd, sectionApp)
	writeDataf(cmd, "Name: %s\n", strings.TrimSpace(app.Name))
	writeDataf(cmd, "ID: %s\n", strings.TrimSpace(app.ID))
	writeDataf(cmd, "Version: %s\n", displayVersion(app.Version))
	writeDataf(cmd, "Exec path: %s\n", strings.TrimSpace(app.ExecPath))

	printSection(cmd, sectionUpdates)
	writeDataf(cmd, "Configured source: %s\n", updateSourceViewSummaryOrNone(app.UpdateSource))
	writeDataf(cmd, "Embedded source: %s\n", updateSourceViewSummaryOrNone(embeddedSource))

	printSection(cmd, sectionState)
	writeDataf(cmd, "Update available: %s\n", yesNo(app.UpdateAvailable))
	if strings.TrimSpace(app.LatestVersion) != "" {
		writeDataf(cmd, "Latest known version: %s\n", displayVersion(app.LatestVersion))
	}
	if strings.TrimSpace(app.LastCheckedAt) != "" {
		writeDataf(cmd, "Last checked: %s\n", strings.TrimSpace(app.LastCheckedAt))
	}

	_ = ctx
	return nil
}

func inspectLocalAppImage(ctx context.Context, cmd *cobra.Command, src string) error {
	label := strings.TrimSpace(filepath.Base(src))
	if label == "" || label == "." || label == string(filepath.Separator) {
		label = strings.TrimSpace(src)
	}

	result, err := runWithBusyIndicator(cmd, fmt.Sprintf("Inspecting %s", label), func() (*appservices.InfoResult, error) {
		return runtimeServicesFrom(cmd).Info.LocalAppImageInfo(ctx, src)
	})
	if err != nil {
		return err
	}
	info := result.AppImageInfo
	embeddedSource := result.EmbeddedUpdate

	if runtimeOptionsFrom(cmd).JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"kind":            "local_appimage",
			"path":            strings.TrimSpace(src),
			"app":             info,
			"embedded_update": embeddedSource,
		})
	}

	printSection(cmd, sectionAppImage)
	writeDataf(cmd, "Path: %s\n", strings.TrimSpace(src))
	writeDataf(cmd, "Name: %s\n", strings.TrimSpace(info.Name))
	writeDataf(cmd, "ID: %s\n", strings.TrimSpace(info.ID))
	writeDataf(cmd, "Version: %s\n", displayVersion(info.Version))

	printSection(cmd, sectionUpdates)
	writeDataf(cmd, "Embedded source: %s\n", updateSourceViewSummaryOrNone(embeddedSource))

	return nil
}

func updateSourceViewSummaryOrNone(update *appservices.UpdateSourceView) string {
	if update == nil || strings.TrimSpace(update.Kind) == appservices.UpdateKindNone {
		return "none"
	}

	switch strings.TrimSpace(update.Kind) {
	case appservices.UpdateKindZsync:
		if update.Zsync == nil {
			return "zsync: <missing>"
		}
		if strings.TrimSpace(update.Zsync.UpdateInfo) != "" {
			return fmt.Sprintf("zsync: %s", strings.TrimSpace(update.Zsync.UpdateInfo))
		}
		return "zsync"
	case appservices.UpdateKindGitHubRelease:
		if update.GitHubRelease == nil {
			return "github: <missing>"
		}
		return fmt.Sprintf("github: %s, asset: %s", strings.TrimSpace(update.GitHubRelease.Repo), strings.TrimSpace(update.GitHubRelease.Asset))
	default:
		return strings.TrimSpace(update.Kind)
	}
}

func updateSourceViewFromAppDetails(app *appservices.AppDetails) *appservices.UpdateSourceView {
	if app == nil {
		return nil
	}
	return app.UpdateSource
}

func updateSourceViewFromInput(input *appservices.UpdateSourceInput) *appservices.UpdateSourceView {
	if input == nil {
		return nil
	}
	return &appservices.UpdateSourceView{Kind: input.Kind, Zsync: input.Zsync, GitHubRelease: input.GitHubRelease}
}

func updateSourceViewIsNone(update *appservices.UpdateSourceView) bool {
	return update == nil || strings.TrimSpace(update.Kind) == "" || strings.TrimSpace(update.Kind) == appservices.UpdateKindNone
}

func updateSourceViewsEqual(left, right *appservices.UpdateSourceView) bool {
	if updateSourceViewIsNone(left) && updateSourceViewIsNone(right) {
		return true
	}
	if updateSourceViewIsNone(left) || updateSourceViewIsNone(right) {
		return false
	}
	if strings.TrimSpace(left.Kind) != strings.TrimSpace(right.Kind) {
		return false
	}
	switch strings.TrimSpace(left.Kind) {
	case appservices.UpdateKindZsync:
		if left.Zsync == nil || right.Zsync == nil {
			return left.Zsync == nil && right.Zsync == nil
		}
		return strings.TrimSpace(left.Zsync.UpdateInfo) == strings.TrimSpace(right.Zsync.UpdateInfo) &&
			strings.TrimSpace(left.Zsync.Transport) == strings.TrimSpace(right.Zsync.Transport)
	case appservices.UpdateKindGitHubRelease:
		if left.GitHubRelease == nil || right.GitHubRelease == nil {
			return left.GitHubRelease == nil && right.GitHubRelease == nil
		}
		return strings.TrimSpace(left.GitHubRelease.Repo) == strings.TrimSpace(right.GitHubRelease.Repo) &&
			strings.TrimSpace(left.GitHubRelease.Asset) == strings.TrimSpace(right.GitHubRelease.Asset) &&
			strings.TrimSpace(left.GitHubRelease.ReleaseKind) == strings.TrimSpace(right.GitHubRelease.ReleaseKind)
	default:
		return true
	}
}

func updateSourcePlanOutput(plan *appservices.DryRunPlan) map[string]interface{} {
	if plan == nil {
		return map[string]interface{}{}
	}
	return updateSourceChangeOutput(plan.Action, plan.Target, plan.UpdateSourceChange, plan.DBWrite)
}

func updateSourceChangeOutput(action, id string, change *appservices.UpdateSourceChangeView, dbWrite bool) map[string]interface{} {
	result := map[string]interface{}{
		"action":   strings.TrimSpace(action),
		"id":       strings.TrimSpace(id),
		"db_write": dbWrite,
	}
	if change == nil {
		return result
	}
	if strings.TrimSpace(result["id"].(string)) == "" {
		result["id"] = strings.TrimSpace(change.ID)
	}
	if change.Current != nil {
		result["current_source"] = change.Current
	}
	if change.Incoming != nil {
		result["incoming_source"] = change.Incoming
	}
	return result
}

func UpdateCmd(cmd *cobra.Command, args []string) error {
	setID, err := flagString(cmd, "set")
	if err != nil {
		return err
	}
	unsetID, err := flagString(cmd, "unset")
	if err != nil {
		return err
	}
	checkOnlyChanged := flagChanged(cmd, "check-only")
	setSpecified := flagChanged(cmd, "set")
	unsetSpecified := flagChanged(cmd, "unset")
	hasSourceFlags := hasUpdateSetFlags(cmd)

	if len(args) > 1 {
		return usageError(fmt.Errorf("too many arguments"))
	}
	targetID := ""
	if len(args) == 1 {
		targetID = strings.TrimSpace(args[0])
		if targetID == "" {
			return usageError(fmt.Errorf("missing required argument <id>"))
		}
	}
	setID = strings.TrimSpace(setID)
	unsetID = strings.TrimSpace(unsetID)

	switch {
	case setSpecified && unsetSpecified:
		return usageError(fmt.Errorf("--set and --unset are mutually exclusive"))
	case setSpecified && targetID != "":
		return usageError(fmt.Errorf("--set is not supported with positional update targets; use either 'aim update <id>' or 'aim update --set <id> ...'"))
	case unsetSpecified && targetID != "":
		return usageError(fmt.Errorf("--unset is not supported with positional update targets; use either 'aim update <id>' or 'aim update --unset <id>'"))
	case setSpecified && checkOnlyChanged:
		return usageError(fmt.Errorf("--check-only is not supported with 'aim update --set'"))
	case unsetSpecified && checkOnlyChanged:
		return usageError(fmt.Errorf("--check-only is not supported with 'aim update --unset'"))
	case unsetSpecified && hasSourceFlags:
		return usageError(fmt.Errorf("update source flags can only be used with 'aim update --set <id> ...'"))
	case setSpecified && setID == "":
		return printConciseHelpError(cmd, "missing required input; pass --set <id> to configure an update source")
	case unsetSpecified && unsetID == "":
		return printConciseHelpError(cmd, "missing required input; pass --unset <id> to remove an update source")
	case !setSpecified && !unsetSpecified && hasSourceFlags:
		return usageError(fmt.Errorf("update source flags can only be used with 'aim update --set <id> ...'"))
	}

	if setSpecified {
		return runUpdateSetMode(cmd, setID)
	}
	if unsetSpecified {
		return runUpdateUnsetMode(cmd, unsetID)
	}

	return runManagedUpdate(cmd.Context(), cmd, targetID)
}

func hasUpdateSetFlags(cmd *cobra.Command) bool {
	keys := []string{"github", "asset", "zsync", "embedded"}
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
	zsyncURL, err := flagString(cmd, "zsync")
	if err != nil {
		return err
	}
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}

	if assetPattern != "" {
		return usageError(fmt.Errorf("--asset is only supported with --github"))
	}

	selectorCount := 1
	for _, value := range []string{githubRepo, zsyncURL} {
		if value != "" {
			selectorCount++
		}
	}
	if selectorCount > 1 {
		return usageError(fmt.Errorf("update source flags are mutually exclusive"))
	}

	return nil
}

func runUpdateSetMode(cmd *cobra.Command, id string) error {
	if strings.TrimSpace(id) == "" {
		return printConciseHelpError(cmd, "missing required input; pass --set <id> to configure an update source")
	}
	opts := runtimeOptionsFrom(cmd)
	embedded, err := flagBool(cmd, "embedded")
	if err != nil {
		return err
	}

	var incomingSource *appservices.UpdateSourceInput
	var incomingView *appservices.UpdateSourceView
	if !embedded {
		incomingSource, err = resolveUpdateSourceFromSetFlags(cmd)
		if err != nil {
			return err
		}
		incomingView = updateSourceViewFromInput(incomingSource)
	}

	info, err := runtimeServicesFrom(cmd).Info.ManagedAppInfo(cmd.Context(), id)
	if err != nil {
		return wrapManagedAppLookupError(id, err)
	}
	appDetails := info.AppDetails
	currentSource := updateSourceViewFromAppDetails(appDetails)

	if embedded {
		if err := validateEmbeddedUpdateSetFlags(cmd); err != nil {
			return err
		}

		embeddedSource, err := runtimeServicesFrom(cmd).Update.EmbeddedSource(cmd.Context(), id)
		if err != nil {
			return err
		}
		if embeddedSource != nil {
			incomingView = embeddedSource.Source
		}
		if updateSourceViewIsNone(incomingView) {
			printWarning(cmd, warningNoEmbeddedSource())
			if updateSourceViewIsNone(currentSource) {
				if opts.JSON {
					return printJSONSuccess(cmd, updateSourceChangeOutput("unset_update_source", id, &appservices.UpdateSourceChangeView{ID: id, Current: currentSource}, true))
				}
				return nil
			}

			printCurrentValue(cmd, updateSourceViewSummaryOrNone(currentSource))
			prompt := formatPrompt("Unset source for", id)
			confirmed, err := confirmAction(cmd, prompt)
			if err != nil {
				return err
			}
			if !confirmed {
				printWarning(cmd, "Update source unchanged")
				return nil
			}
			_, err = runtimeServicesFrom(cmd).Update.UnsetSource(cmd.Context(), id)
			if err != nil {
				return err
			}
			printSuccess(cmd, "Update source unset")
			return nil
		}
	}

	if !updateSourceViewIsNone(currentSource) && !updateSourceViewsEqual(currentSource, incomingView) {
		printCurrentIncoming(cmd, updateSourceViewSummaryOrNone(currentSource), updateSourceViewSummaryOrNone(incomingView))
		prompt := formatPrompt("Replace source for", id)
		confirmed, err := confirmAction(cmd, prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			printWarning(cmd, "Update source unchanged")
			return nil
		}
	}

	if opts.DryRun {
		var plan *appservices.DryRunPlan
		if embedded {
			plan, err = runtimeServicesFrom(cmd).Update.PlanSetEmbeddedSource(cmd.Context(), id)
		} else {
			plan, err = runtimeServicesFrom(cmd).Update.PlanSetSource(cmd.Context(), appservices.UpdateSourceRequest{ID: id, Source: incomingSource})
		}
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSONSuccess(cmd, updateSourcePlanOutput(plan))
		}
		writeDataf(cmd, "Dry run: would set update source for %s\n", id)
		return nil
	}

	var setResult *appservices.UpdateSourceResult
	if err := withStateWriteLock(cmd, func() error {
		logOperationf(cmd, "Setting update source for %s", id)
		var setErr error
		if embedded {
			setResult, setErr = runtimeServicesFrom(cmd).Update.SetEmbeddedSource(cmd.Context(), id)
		} else {
			setResult, setErr = runtimeServicesFrom(cmd).Update.SetSource(cmd.Context(), appservices.UpdateSourceRequest{ID: id, Source: incomingSource})
		}
		if setErr != nil {
			return wrapWriteError(setErr)
		}
		return nil
	}); err != nil {
		return err
	}

	if setResult == nil {
		return softwareError(fmt.Errorf("set update source result missing"))
	}
	if opts.JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"action": "set_update_source",
			"id":     id,
			"source": setResult.Source,
		})
	}
	printSuccess(cmd, fmt.Sprintf("Update source set: %s", updateSourceViewSummaryOrNone(setResult.Source)))
	return nil
}

func runUpdateUnsetMode(cmd *cobra.Command, id string) error {
	if strings.TrimSpace(id) == "" {
		return printConciseHelpError(cmd, "missing required input; pass --unset <id> to remove an update source")
	}

	info, err := runtimeServicesFrom(cmd).Info.ManagedAppInfo(cmd.Context(), id)
	if err != nil {
		return wrapManagedAppLookupError(id, err)
	}
	appDetails := info.AppDetails
	currentSource := updateSourceViewFromAppDetails(appDetails)

	if runtimeOptionsFrom(cmd).DryRun {
		plan, err := runtimeServicesFrom(cmd).Update.PlanUnsetSource(cmd.Context(), id)
		if err != nil {
			return err
		}
		if runtimeOptionsFrom(cmd).JSON {
			return printJSONSuccess(cmd, updateSourcePlanOutput(plan))
		}
		writeDataf(cmd, "Dry run: would unset update source for %s\n", id)
		return nil
	}

	if updateSourceViewIsNone(currentSource) {
		printSuccess(cmd, fmt.Sprintf("No update source configured for %s", id))
		return nil
	}
	printCurrentValue(cmd, updateSourceViewSummaryOrNone(currentSource))
	prompt := formatPrompt("Unset source for", id)
	confirmed, err := confirmAction(cmd, prompt)
	if err != nil {
		return err
	}
	if !confirmed {
		printWarning(cmd, "Update source unchanged")
		return nil
	}
	if err := withStateWriteLock(cmd, func() error {
		logOperationf(cmd, "Unsetting update source for %s", id)
		_, unsetErr := runtimeServicesFrom(cmd).Update.UnsetSource(cmd.Context(), id)
		if unsetErr != nil {
			return wrapWriteError(unsetErr)
		}
		return nil
	}); err != nil {
		return err
	}
	printSuccess(cmd, "Update source unset")
	return nil
}

func resolveUpdateSourceFromSetFlags(cmd *cobra.Command) (*appservices.UpdateSourceInput, error) {
	githubRepo, err := flagString(cmd, "github")
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

	selectorCount := 0
	for _, value := range []string{githubRepo, zsyncURL} {
		if value != "" {
			selectorCount++
		}
	}

	if selectorCount == 0 {
		return nil, usageError(fmt.Errorf("missing update source; set one of --github, --zsync, or --embedded"))
	}
	if selectorCount > 1 {
		return nil, usageError(fmt.Errorf("update source flags are mutually exclusive"))
	}

	if githubRepo != "" {
		ref, err := validateGitHubRepoFlag(githubRepo)
		if err != nil {
			return nil, err
		}
		if assetPattern == "" {
			assetPattern = defaultReleaseAssetPattern
		}
		return &appservices.UpdateSourceInput{
			Kind: appservices.UpdateKindGitHubRelease,
			GitHubRelease: &appservices.GitHubReleaseUpdateView{
				Repo:  ref.Ref,
				Asset: assetPattern,
			},
		}, nil
	}

	if zsyncURL != "" {
		if assetPattern != "" {
			return nil, usageError(fmt.Errorf("--asset is only supported with --github"))
		}
		if !isHTTPSURL(zsyncURL) {
			return nil, usageError(fmt.Errorf("--zsync must be a valid https URL"))
		}
		return &appservices.UpdateSourceInput{
			Kind: appservices.UpdateKindZsync,
			Zsync: &appservices.ZsyncUpdateSourceView{
				UpdateInfo: "zsync|" + zsyncURL,
				Transport:  "zsync",
			},
		}, nil
	}

	if assetPattern != "" {
		return nil, usageError(fmt.Errorf("--asset is only supported with --github"))
	}
	return nil, usageError(fmt.Errorf("missing update source; set one of --github, --zsync, or --embedded"))
}

type managedCheckFailure struct {
	AppID  string
	Reason string
}

const defaultReleaseAssetPattern = "*.AppImage"

func displayVersion(value string) string {
	v := strings.TrimSpace(strings.Trim(value, `"'`))
	if v == "" {
		return "unknown"
	}

	if strings.EqualFold(v, "dev") {
		return "dev"
	}

	if normalized := appservices.NormalizeComparableVersion(v); normalized != "" {
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
	return appservices.ManagedUpdateDownloadFilename(assetName, downloadURL)
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

const maxListNameColumnWidth = 28

func listIDColumnWidth(groups ...[]*appservices.AppDetails) int {
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

func listNameDisplayWidth(groups ...[]*appservices.AppDetails) int {
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

func formatListRow(app *appservices.AppDetails, idWidth, nameWidth int) string {
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
