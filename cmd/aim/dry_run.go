package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/discovery"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/spf13/cobra"
)

func buildLocalIntegrateDryRunPlan(ctx context.Context, path string) (map[string]interface{}, error) {
	info, err := readAppImageInfo(ctx, path)
	if err != nil {
		return nil, err
	}

	appID := strings.TrimSpace(info.ID)
	if appID == "" {
		appID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	appDir := filepath.Join(config.AimDir, appID)

	return map[string]interface{}{
		"action": "integrate",
		"input":  strings.TrimSpace(path),
		"app_id": appID,
		"app": map[string]string{
			"name":    strings.TrimSpace(info.Name),
			"id":      appID,
			"version": strings.TrimSpace(info.Version),
		},
		"planned_paths": compactStrings([]string{
			filepath.Join(appDir, appID+".AppImage"),
			filepath.Join(appDir, appID+".desktop"),
			filepath.Join(config.DesktopDir, appID+".desktop"),
		}),
		"db_write": true,
	}, nil
}

func buildInstallDryRunPlan(ctx context.Context, cmd *cobra.Command, refArg string, target *installTarget, assetPattern, sha256 string) (map[string]interface{}, error) {
	if target == nil {
		return nil, fmt.Errorf("missing install target")
	}

	switch target.Kind {
	case installTargetDirectURL:
		return map[string]interface{}{
			"action":          "install",
			"target":          strings.TrimSpace(target.URL),
			"target_kind":     string(target.Kind),
			"expected_sha256": strings.TrimSpace(sha256),
			"download_url":    strings.TrimSpace(target.URL),
			"db_write":        true,
		}, nil
	case installTargetGitHub:
		metadata, err := resolvePackageMetadataWithProgress(cmd, installTargetLabel(target), func() (*discovery.PackageMetadata, error) {
			return resolvePackageMetadataFromInput(ctx, refArg, assetPattern)
		})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"action":      "install",
			"target":      installTargetLabel(target),
			"target_kind": string(target.Kind),
			"metadata":    packageMetadataOutput(metadata),
			"db_write":    true,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported add target")
	}
}

func buildUpdateSetDryRunResult(id string, current, incoming *models.UpdateSource) map[string]interface{} {
	return map[string]interface{}{
		"action":          "set_update_source",
		"id":              strings.TrimSpace(id),
		"current_source":  current,
		"incoming_source": incoming,
	}
}

func buildUpdateUnsetDryRunResult(id string, current *models.UpdateSource) map[string]interface{} {
	return map[string]interface{}{
		"action":         "unset_update_source",
		"id":             strings.TrimSpace(id),
		"current_source": current,
	}
}
