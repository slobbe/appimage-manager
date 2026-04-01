package main

import (
	"path/filepath"
	"sort"

	"github.com/slobbe/appimage-manager/internal/discovery"
	models "github.com/slobbe/appimage-manager/internal/types"
)

type listOutputRow struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Integrated      bool   `json:"integrated"`
	ExecPath        string `json:"exec_path"`
	UpdateAvailable bool   `json:"update_available"`
	LatestVersion   string `json:"latest_version,omitempty"`
	LastCheckedAt   string `json:"last_checked_at,omitempty"`
}

type updateOutputRow struct {
	ID              string `json:"id"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Status          string `json:"status"`
	DownloadURL     string `json:"download_url,omitempty"`
	Asset           string `json:"asset,omitempty"`
	SourceKind      string `json:"source_kind,omitempty"`
	LastCheckedAt   string `json:"last_checked_at,omitempty"`
}

func newListOutputRow(app *models.App) listOutputRow {
	if app == nil {
		return listOutputRow{}
	}
	return listOutputRow{
		ID:              app.ID,
		Name:            app.Name,
		Version:         app.Version,
		Integrated:      app.DesktopEntryLink != "",
		ExecPath:        app.ExecPath,
		UpdateAvailable: app.UpdateAvailable,
		LatestVersion:   app.LatestVersion,
		LastCheckedAt:   app.LastCheckedAt,
	}
}

func (row listOutputRow) csvRow() []string {
	return []string{
		row.ID,
		row.Name,
		row.Version,
		boolString(row.Integrated),
		row.ExecPath,
		boolString(row.UpdateAvailable),
		row.LatestVersion,
		row.LastCheckedAt,
	}
}

func listCSVHeader() []string {
	return []string{"id", "name", "version", "integrated", "exec_path", "update_available", "latest_version", "last_checked_at"}
}

func newUpdateOutputRow(app *models.App, update *pendingManagedUpdate, status, checkedAt string) updateOutputRow {
	row := updateOutputRow{
		Status:        status,
		LastCheckedAt: checkedAt,
	}
	if app != nil {
		row.ID = app.ID
		row.CurrentVersion = app.Version
		row.LastCheckedAt = app.LastCheckedAt
	}
	if update != nil {
		row.LatestVersion = update.Latest
		row.UpdateAvailable = update.Available && update.URL != ""
		row.DownloadURL = update.URL
		row.Asset = update.Asset
		row.SourceKind = string(update.FromKind)
	}
	return row
}

func (row updateOutputRow) csvRow() []string {
	return []string{
		row.ID,
		row.CurrentVersion,
		row.LatestVersion,
		boolString(row.UpdateAvailable),
		row.Status,
		row.DownloadURL,
		row.Asset,
		row.SourceKind,
		row.LastCheckedAt,
	}
}

func updateCSVHeader() []string {
	return []string{"id", "current_version", "latest_version", "update_available", "status", "download_url", "asset", "source_kind", "last_checked_at"}
}

func packageMetadataOutput(metadata *discovery.PackageMetadata) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	return map[string]interface{}{
		"name":           metadata.Name,
		"provider":       metadata.Provider,
		"ref":            metadata.Ref,
		"repo_url":       metadata.RepoURL,
		"latest_version": metadata.LatestVersion,
		"asset_name":     metadata.AssetName,
		"asset_pattern":  metadata.AssetPattern,
		"download_url":   metadata.DownloadURL,
		"installable":    metadata.Installable,
		"install_reason": metadata.InstallReason,
		"release_tag":    metadata.ReleaseTag,
		"summary":        metadata.Summary,
	}
}

func sortAppsByID(apps map[string]*models.App) []*models.App {
	ids := make([]string, 0, len(apps))
	for id := range apps {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	ordered := make([]*models.App, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, apps[id])
	}
	return ordered
}

func removeDryRunPlan(app *models.App, unlink bool) map[string]interface{} {
	if app == nil {
		return map[string]interface{}{}
	}

	action := "remove"
	paths := []string{app.DesktopEntryLink}
	if unlink {
		action = "unlink"
	} else {
		paths = append(paths, filepath.Dir(app.ExecPath))
		if app.IconPath != "" {
			paths = append(paths, app.IconPath)
		}
	}

	return map[string]interface{}{
		"action": action,
		"unlink": unlink,
		"app":    app,
		"paths":  compactStrings(paths),
	}
}

func compactStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		filtered = append(filtered, value)
	}
	return filtered
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
