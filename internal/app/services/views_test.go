package services

import (
	"testing"

	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestManagedUpdateHandleKeepsCheckedUpdate(t *testing.T) {
	update := &appupdate.ManagedUpdate{
		App:       &domain.App{ID: "app", Name: "App"},
		URL:       "https://example.com/app.AppImage",
		Asset:     "app.AppImage",
		Available: true,
		Latest:    "2.0.0",
		FromKind:  domain.UpdateZsync,
	}

	handle := managedUpdateHandleFromAppUpdate(update)
	if handle == nil || handle.View.App == nil || handle.View.App.ID != "app" || handle.View.FromKind != domain.UpdateZsync || handle.View.URL != "https://example.com/app.AppImage" {
		t.Fatalf("unexpected managed update handle: %+v", handle)
	}
}
