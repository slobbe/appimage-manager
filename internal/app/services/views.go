package services

import (
	"strings"

	appimageapp "github.com/slobbe/appimage-manager/internal/app/appimage"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/domain"
)

const (
	ProviderGitHub = domain.ProviderGitHub

	UpdateKindNone          = domain.UpdateNone
	UpdateKindZsync         = domain.UpdateZsync
	UpdateKindGitHubRelease = domain.UpdateGitHubRelease
)

// UpdateSourceChangeView describes a planned or completed update-source change.
type UpdateSourceChangeView struct {
	ID       string               `json:"id"`
	Current  *domain.UpdateSource `json:"current_source,omitempty"`
	Incoming *domain.UpdateSource `json:"incoming_source,omitempty"`
	Changed  bool                 `json:"changed,omitempty"`
}

// AppImageInfoView is the app-service view of AppImage metadata.
type AppImageInfoView struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	DesktopStem string `json:"desktop_stem,omitempty"`
	Version     string `json:"version"`
}

// ManagedUpdateHandle is an opaque handle for applying a checked managed update.
type ManagedUpdateHandle struct {
	View   appupdate.ManagedUpdate `json:"view"`
	update appupdate.ManagedUpdate
}

func appImageInfoViewFromAppImageInfo(info *appimageapp.AppInfo) *AppImageInfoView {
	if info == nil {
		return nil
	}
	return &AppImageInfoView{
		Name:        strings.TrimSpace(info.Name),
		ID:          strings.TrimSpace(info.ID),
		DesktopStem: strings.TrimSpace(info.DesktopStem),
		Version:     strings.TrimSpace(info.Version),
	}
}

func managedUpdateHandleFromAppUpdate(update *appupdate.ManagedUpdate) *ManagedUpdateHandle {
	if update == nil {
		return nil
	}
	return &ManagedUpdateHandle{View: *update, update: *update}
}
