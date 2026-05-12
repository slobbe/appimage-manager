package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSettingsDefaultsWhenMissing(t *testing.T) {
	originalConfigDir := ConfigDir
	ConfigDir = t.TempDir()
	t.Cleanup(func() {
		ConfigDir = originalConfigDir
	})

	settings, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings returned error: %v", err)
	}
	if settings.NetworkTimeout != 30*time.Second {
		t.Fatalf("NetworkTimeout = %s, want 30s", settings.NetworkTimeout)
	}
}

func TestLoadSettingsParsesNetworkTimeout(t *testing.T) {
	originalConfigDir := ConfigDir
	ConfigDir = t.TempDir()
	t.Cleanup(func() {
		ConfigDir = originalConfigDir
	})

	if err := os.WriteFile(filepath.Join(ConfigDir, "settings.toml"), []byte("network_timeout = \"45s\"\n"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	settings, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings returned error: %v", err)
	}
	if settings.NetworkTimeout != 45*time.Second {
		t.Fatalf("NetworkTimeout = %s, want 45s", settings.NetworkTimeout)
	}
}

func TestLoadSettingsRejectsInvalidTimeout(t *testing.T) {
	originalConfigDir := ConfigDir
	ConfigDir = t.TempDir()
	t.Cleanup(func() {
		ConfigDir = originalConfigDir
	})

	if err := os.WriteFile(filepath.Join(ConfigDir, "settings.toml"), []byte("network_timeout = \"not-a-duration\"\n"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	if _, err := LoadSettings(); err == nil {
		t.Fatal("expected invalid settings error")
	}
}
