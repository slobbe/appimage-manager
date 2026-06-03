package domain

import "testing"

func TestShouldRemoveStaleInstalledIcon(t *testing.T) {
	apps := map[string]*App{
		"current": {ID: "current", IconPath: "/icons/current.svg"},
		"other":   {ID: "other", IconPath: "/icons/shared.svg"},
	}

	if !ShouldRemoveStaleInstalledIcon("/icons/old.svg", "/icons/new.svg", "current", "/apps/current", apps) {
		t.Fatal("expected stale unowned icon to be removable")
	}
	if ShouldRemoveStaleInstalledIcon("/icons/shared.svg", "/icons/new.svg", "current", "/apps/current", apps) {
		t.Fatal("expected icon referenced by another app to be kept")
	}
	if ShouldRemoveStaleInstalledIcon("/apps/current/icon.svg", "/icons/new.svg", "current", "/apps/current", apps) {
		t.Fatal("expected icon inside app dir to be kept")
	}
	if ShouldRemoveStaleInstalledIcon("/icons/new.svg", "/icons/new.svg", "current", "/apps/current", apps) {
		t.Fatal("expected unchanged icon path to be kept")
	}
}
