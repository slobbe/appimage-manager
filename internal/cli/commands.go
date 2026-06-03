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
	writeDataf(cmd, "%s", renderConciseHelp(cmd))
	return nil
}

func SelfUpdateCmd(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError(fmt.Errorf("self-update does not accept positional arguments"))
	}
	preRelease, err := cmd.Flags().GetBool("pre")
	if err != nil {
		return usageError(err)
	}
	if err := mustEnsureRuntimeDirs(); err != nil {
		return err
	}
	return runSelfUpdate(cmd.Context(), cmd, preRelease)
}

func runSelfUpdate(ctx context.Context, cmd *cobra.Command, preRelease bool) error {
	if ctx == nil {
		ctx = context.Background()
	}

	logOperationf(cmd, "Checking for aim updates")
	result, err := runWithBusyIndicator(cmd, progressSelfUpdateAim(), func() (*appservices.SelfUpdateResult, error) {
		return runtimeServicesFrom(cmd).SelfUpdate.SelfUpdate(ctx, appservices.SelfUpdateRequest{CurrentVersion: version, PreRelease: preRelease})
	})
	if err != nil {
		return err
	}
	return renderSelfUpdateResult(cmd, result)
}

func renderSelfUpdateResult(cmd *cobra.Command, result *appservices.SelfUpdateResult) error {
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
			"Updated aim %s -> %s",
			displayVersion(result.CurrentVersion),
			displayVersion(result.InstalledVersion),
		))
		return nil
	}
	printSuccess(cmd, "Updated aim")
	return nil
}

func AddCmd(cmd *cobra.Command, args []string) error {
	selection, err := resolveAddInput(cmd, args)
	if err != nil {
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
	opts := runtimeOptionsFrom(cmd)
	if strings.TrimSpace(selection.Positional) != "" {
		if err := validateAddIntegrateFlags(cmd); err != nil {
			return err
		}
	}
	if selection.HasRef && strings.TrimSpace(sha256) != "" {
		return usageError(fmt.Errorf("--sha256 is only supported with direct https URLs"))
	}

	var provider *appservices.ProviderRef
	if selection.HasRef {
		provider = &selection.Ref
	}
	req := appservices.AddRequest{
		Target: appservices.AddTargetInput{
			Positional: selection.Positional,
			URL:        selection.DirectURL,
			Provider:   provider,
		},
		DryRun:       opts.DryRun,
		SHA256:       sha256,
		AssetPattern: assetPattern,
		ConfirmUpdateSourceReplace: updateSourceReplaceConfirmerFunc(func(existing, incoming *appservices.UpdateSourceView) (bool, error) {
			printCurrentIncoming(cmd, updateSourceViewSummaryOrNone(existing), updateSourceViewSummaryOrNone(incoming))
			prompt := formatPrompt("Replace source from", "AppImage metadata")
			return confirmAction(cmd, prompt)
		}),
		ResolvePackageAmbiguity: packageAmbiguityResolverFunc(func(metadata *appservices.PackageView) (*appservices.PackageView, error) {
			resolved, err := resolvePackageViewAmbiguity(cmd, metadata)
			if err != nil {
				return nil, err
			}
			if resolved != nil && strings.TrimSpace(resolved.AssetName) != "" {
				writeDataf(cmd, "Integrating %s...\n", strings.TrimSpace(resolved.AssetName))
			}
			return resolved, nil
		}),
	}

	run := func() error {
		if !opts.DryRun && strings.TrimSpace(selection.DirectURL) != "" && strings.TrimSpace(sha256) == "" {
			printWarning(cmd, "No SHA-256 provided; skipping checksum verification")
		}
		result, err := runAddRequest(cmd.Context(), cmd, selection, req)
		if err != nil {
			if !selection.HasRef && strings.TrimSpace(selection.DirectURL) == "" && strings.TrimSpace(selection.Positional) != "" {
				if addTargetLooksRemote(selection.Positional) {
					return positionalAddRemoteGuidance(selection.Positional)
				}
				return usageError(fmt.Errorf("unknown add target %q; expected <id> or <Path/To.AppImage>", selection.Positional))
			}
			return err
		}
		return renderAddResult(cmd, selection, result)
	}

	if opts.DryRun {
		return run()
	}
	if err := mustEnsureRuntimeDirs(); err != nil {
		return err
	}
	return withStateWriteLock(cmd, run)
}

type addInputSelection struct {
	Positional string
	DirectURL  string
	Ref        appservices.ProviderRef
	HasRef     bool
}

func runAddRequest(ctx context.Context, cmd *cobra.Command, selection addInputSelection, req appservices.AddRequest) (*appservices.AddResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Target.Provider != nil {
		label := appservices.FormatProviderRef(*req.Target.Provider)
		if req.DryRun {
			return resolveAddPlanWithProgress(cmd, label, func() (*appservices.AddResult, error) {
				return runtimeServicesFrom(cmd).Add.Add(ctx, req)
			})
		}
		return runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", label), func() (*appservices.AddResult, error) {
			return runtimeServicesFrom(cmd).Add.Add(ctx, req)
		})
	}
	if !req.DryRun && strings.TrimSpace(selection.Positional) != "" && runtimeHasExtension(selection.Positional, ".AppImage") {
		inputLabel := strings.TrimSpace(filepath.Base(selection.Positional))
		if inputLabel == "" || inputLabel == "." || inputLabel == string(filepath.Separator) {
			inputLabel = strings.TrimSpace(selection.Positional)
		}
		return runWithBusyIndicator(cmd, fmt.Sprintf("Integrating %s", inputLabel), func() (*appservices.AddResult, error) {
			return runtimeServicesFrom(cmd).Add.Add(ctx, req)
		})
	}
	return runtimeServicesFrom(cmd).Add.Add(ctx, req)
}

func resolveAddPlanWithProgress(cmd *cobra.Command, label string, fn func() (*appservices.AddResult, error)) (*appservices.AddResult, error) {
	return runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", label), fn)
}

func renderAddResult(cmd *cobra.Command, selection addInputSelection, result *appservices.AddResult) error {
	if result == nil {
		return softwareError(fmt.Errorf("add result cannot be empty"))
	}
	opts := runtimeOptionsFrom(cmd)
	if result.Status == "dry_run" {
		return renderAddDryRunResult(cmd, result)
	}
	if result.AlreadyIntegrated || result.Status == "already_integrated" {
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{"status": "already_integrated", "app": result.App})
		}
		printSuccess(cmd, fmt.Sprintf("Already integrated: %s", formatAppDetailsRef(result.App)))
		return nil
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = string(result.Action)
	}
	if opts.JSON {
		return printJSONSuccess(cmd, map[string]interface{}{"status": status, "app": result.App})
	}
	switch status {
	case "integrated":
		printSuccess(cmd, fmt.Sprintf("Integrated: %s", formatAppDetailsRef(result.App)))
	case "reintegrated":
		printSuccess(cmd, fmt.Sprintf("Reintegrated: %s", formatAppDetailsRef(result.App)))
	case "installed":
		printSuccess(cmd, fmt.Sprintf("Installed: %s", formatAppDetailsRef(result.App)))
	default:
		return softwareError(fmt.Errorf("unknown add result status %q for target %q", status, selection.Positional))
	}
	return nil
}

func renderAddDryRunResult(cmd *cobra.Command, result *appservices.AddResult) error {
	opts := runtimeOptionsFrom(cmd)
	if result.Action == appservices.AddActionReintegrate {
		payload := map[string]interface{}{"status": "dry_run", "action": "reintegrate", "app": result.App}
		if opts.JSON {
			return printJSONSuccess(cmd, payload)
		}
		if result.App == nil {
			return softwareError(fmt.Errorf("reintegrate target missing app"))
		}
		writeDataf(cmd, "Dry run: would reintegrate %s [%s]\n", result.App.Name, result.App.ID)
		return nil
	}
	plan := result.Plan
	if plan == nil {
		return softwareError(fmt.Errorf("add dry-run result missing plan"))
	}
	if opts.JSON {
		return printJSONSuccess(cmd, plan.Values)
	}
	switch result.Action {
	case appservices.AddActionIntegrate:
		writeDataf(cmd, "Dry run: would integrate %s\n", plan.Values["input"])
		if appID, ok := plan.Values["app_id"].(string); ok && appID != "" {
			writeDataf(cmd, "  Managed ID: %s\n", appID)
		}
		if paths, ok := plan.Values["planned_paths"].([]string); ok {
			for _, path := range paths {
				writeDataf(cmd, "  %s\n", path)
			}
		}
	case appservices.AddActionInstall:
		writeDataf(cmd, "Dry run: would install %s\n", plan.Values["target"])
	default:
		return softwareError(fmt.Errorf("unknown add dry-run action %q", result.Action))
	}
	return nil
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
	req := appservices.AddRequest{Target: appservices.AddTargetInput{Positional: input}, DryRun: runtimeOptionsFrom(cmd).DryRun}
	result, err := runAddRequest(ctx, cmd, addInputSelection{Positional: input}, req)
	if err != nil {
		return usageError(err)
	}
	return renderAddResult(cmd, addInputSelection{Positional: input}, result)
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

	var provider *appservices.ProviderRef
	if ok {
		provider = &ref
	}
	result, err := loadInfoResult(cmd.Context(), cmd, appservices.InfoRequest{Input: input, Provider: provider})
	if err != nil {
		trimmed := strings.TrimSpace(input)
		if provider == nil {
			if refErr := positionalInfoRemoteGuidance(trimmed); refErr != nil {
				return refErr
			}
			if trimmed != "" && !runtimeHasExtension(trimmed, ".AppImage") {
				return usageError(fmt.Errorf("unknown info target %q; expected <id> or <Path/To.AppImage>", trimmed))
			}
		}
		return err
	}
	return renderInfoResult(cmd, input, result)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func loadInfoResult(ctx context.Context, cmd *cobra.Command, req appservices.InfoRequest) (*appservices.InfoResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Provider != nil {
		label := appservices.FormatProviderRef(*req.Provider)
		return resolvePackageInfoWithProgress(cmd, label, func() (*appservices.InfoResult, error) {
			return runtimeServicesFrom(cmd).Info.Info(ctx, req)
		})
	}
	if runtimeHasExtension(req.Input, ".AppImage") && !req.ManagedOnly {
		label := strings.TrimSpace(filepath.Base(req.Input))
		if label == "" || label == "." || label == string(filepath.Separator) {
			label = strings.TrimSpace(req.Input)
		}
		return runWithBusyIndicator(cmd, fmt.Sprintf("Inspecting %s", label), func() (*appservices.InfoResult, error) {
			return runtimeServicesFrom(cmd).Info.Info(ctx, req)
		})
	}
	return runtimeServicesFrom(cmd).Info.Info(ctx, req)
}

func renderInfoResult(cmd *cobra.Command, input string, result *appservices.InfoResult) error {
	if result == nil {
		return softwareError(fmt.Errorf("info result cannot be empty"))
	}
	switch result.Kind {
	case appservices.InfoKindManagedApp:
		return renderManagedAppInfo(cmd, result)
	case appservices.InfoKindLocalAppImage:
		return renderLocalAppImageInfo(cmd, input, result)
	case appservices.InfoKindPackage:
		return renderPackageInfo(cmd, result)
	default:
		return softwareError(fmt.Errorf("unknown info result kind %q", result.Kind))
	}
}

func renderPackageInfo(cmd *cobra.Command, result *appservices.InfoResult) error {
	metadata := result.PackageView
	if runtimeOptionsFrom(cmd).JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"kind":     "package_metadata",
			"metadata": metadata,
		})
	}
	var err error
	metadata, err = resolvePackageViewAmbiguity(cmd, metadata)
	if err != nil {
		return err
	}
	printPackageView(cmd, metadata)
	return nil
}

func runShowPackageRef(ctx context.Context, cmd *cobra.Command, ref appservices.ProviderRef) error {
	result, err := loadInfoResult(ctx, cmd, appservices.InfoRequest{Provider: &ref})
	if err != nil {
		return err
	}
	return renderInfoResult(cmd, "", result)
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
	if !runtimeOptionsFrom(cmd).DryRun && strings.TrimSpace(sha256) == "" {
		printWarning(cmd, "No SHA-256 provided; skipping checksum verification")
	}
	req := appservices.AddRequest{Target: appservices.AddTargetInput{URL: target.URL}, DryRun: runtimeOptionsFrom(cmd).DryRun, SHA256: sha256}
	selection := addInputSelection{DirectURL: target.URL}
	result, err := runAddRequest(ctx, cmd, selection, req)
	if err != nil {
		return err
	}
	return renderAddResult(cmd, selection, result)
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
	selection := addInputSelection{Ref: ref, HasRef: true}
	req := appservices.AddRequest{Target: appservices.AddTargetInput{Provider: &ref}, DryRun: runtimeOptionsFrom(cmd).DryRun, AssetPattern: assetPattern}
	result, err := runAddRequest(ctx, cmd, selection, req)
	if err != nil {
		return err
	}
	return renderAddResult(cmd, selection, result)
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

func managedAppInfo(ctx context.Context, cmd *cobra.Command, id string) (*appservices.InfoResult, error) {
	info, err := loadInfoResult(ctx, cmd, appservices.InfoRequest{Input: id, ManagedOnly: true})
	if err != nil {
		return nil, wrapManagedAppLookupError(id, err)
	}
	return info, nil
}

func renderManagedAppInfo(cmd *cobra.Command, info *appservices.InfoResult) error {
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

	return nil
}

func inspectLocalAppImage(ctx context.Context, cmd *cobra.Command, src string) error {
	result, err := loadInfoResult(ctx, cmd, appservices.InfoRequest{Input: src})
	if err != nil {
		return err
	}
	return renderLocalAppImageInfo(cmd, src, result)
}

func renderLocalAppImageInfo(cmd *cobra.Command, src string, result *appservices.InfoResult) error {
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

func updateSourceViewIsNone(update *appservices.UpdateSourceView) bool {
	return update == nil || strings.TrimSpace(update.Kind) == "" || strings.TrimSpace(update.Kind) == appservices.UpdateKindNone
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
	embedded, err := flagBool(cmd, "embedded")
	if err != nil {
		return err
	}
	var incomingSource *appservices.UpdateSourceInput
	if embedded {
		if err := validateEmbeddedUpdateSetFlags(cmd); err != nil {
			return err
		}
	} else {
		incomingSource, err = resolveUpdateSourceFromSetFlags(cmd)
		if err != nil {
			return err
		}
	}
	result, err := runtimeServicesFrom(cmd).Update.Update(cmd.Context(), appservices.UpdateRequest{
		TargetID:          id,
		Mode:              appservices.UpdateModeSetSource,
		DryRun:            runtimeOptionsFrom(cmd).DryRun,
		Source:            incomingSource,
		UseEmbeddedSource: embedded,
		ConfirmSourceReplace: updateSourceReplaceConfirmerFunc(func(existing, incoming *appservices.UpdateSourceView) (bool, error) {
			printCurrentIncoming(cmd, updateSourceViewSummaryOrNone(existing), updateSourceViewSummaryOrNone(incoming))
			return confirmAction(cmd, formatPrompt("Replace source for", id))
		}),
		ConfirmSourceUnset: updateSourceUnsetConfirmerFunc(func(current *appservices.UpdateSourceView) (bool, error) {
			printCurrentValue(cmd, updateSourceViewSummaryOrNone(current))
			return confirmAction(cmd, formatPrompt("Unset source for", id))
		}),
	})
	if err != nil {
		return wrapManagedAppLookupError(id, err)
	}
	return renderUpdateSourceModeResult(cmd, id, result)
}

func runUpdateUnsetMode(cmd *cobra.Command, id string) error {
	if strings.TrimSpace(id) == "" {
		return printConciseHelpError(cmd, "missing required input; pass --unset <id> to remove an update source")
	}
	result, err := runtimeServicesFrom(cmd).Update.Update(cmd.Context(), appservices.UpdateRequest{
		TargetID: id,
		Mode:     appservices.UpdateModeUnsetSource,
		DryRun:   runtimeOptionsFrom(cmd).DryRun,
		ConfirmSourceUnset: updateSourceUnsetConfirmerFunc(func(current *appservices.UpdateSourceView) (bool, error) {
			printCurrentValue(cmd, updateSourceViewSummaryOrNone(current))
			return confirmAction(cmd, formatPrompt("Unset source for", id))
		}),
	})
	if err != nil {
		return wrapManagedAppLookupError(id, err)
	}
	return renderUpdateSourceModeResult(cmd, id, result)
}

type updateSourceUnsetConfirmerFunc func(current *appservices.UpdateSourceView) (bool, error)

func (fn updateSourceUnsetConfirmerFunc) ConfirmUpdateSourceUnset(current *appservices.UpdateSourceView) (bool, error) {
	return fn(current)
}

func renderUpdateSourceModeResult(cmd *cobra.Command, id string, result *appservices.UpdateResult) error {
	if result == nil {
		return softwareError(fmt.Errorf("update source result missing"))
	}
	opts := runtimeOptionsFrom(cmd)
	if result.NoEmbeddedSource {
		printWarning(cmd, warningNoEmbeddedSource())
	}
	if result.Plan != nil {
		if opts.JSON {
			return printJSONSuccess(cmd, updateSourcePlanOutput(result.Plan))
		}
		if result.Mode == appservices.UpdateModeUnsetSource {
			writeDataf(cmd, "Dry run: would unset update source for %s\n", id)
			return nil
		}
		writeDataf(cmd, "Dry run: would set update source for %s\n", id)
		return nil
	}
	if result.SourceUnchanged {
		var currentSource *appservices.UpdateSourceView
		if result.SourceChange != nil {
			currentSource = result.SourceChange.Current
		}
		if result.NoEmbeddedSource && updateSourceViewIsNone(currentSource) {
			if opts.JSON {
				return printJSONSuccess(cmd, updateSourceChangeOutput("unset_update_source", id, result.SourceChange, true))
			}
			return nil
		}
		if result.Mode == appservices.UpdateModeUnsetSource && result.Source != nil && !result.Source.Changed {
			printSuccess(cmd, fmt.Sprintf("No update source configured for %s", id))
			return nil
		}
		printWarning(cmd, "Update source unchanged")
		return nil
	}
	if opts.JSON {
		action := "set_update_source"
		if result.Mode == appservices.UpdateModeUnsetSource {
			action = "unset_update_source"
		}
		if result.SourceChange != nil {
			return printJSONSuccess(cmd, updateSourceChangeOutput(action, id, result.SourceChange, true))
		}
		return printJSONSuccess(cmd, map[string]interface{}{"action": action, "id": id, "source": result.Source})
	}
	if result.Mode == appservices.UpdateModeUnsetSource {
		printSuccess(cmd, "Update source unset")
		return nil
	}
	if result.Source == nil {
		return softwareError(fmt.Errorf("set update source result missing"))
	}
	printSuccess(cmd, fmt.Sprintf("Update source set: %s", updateSourceViewSummaryOrNone(result.Source.Source)))
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
