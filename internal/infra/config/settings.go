package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultNetworkTimeout = 30 * time.Second

type Settings struct {
	NetworkTimeout time.Duration
}

func DefaultSettings() Settings {
	return Settings{
		NetworkTimeout: defaultNetworkTimeout,
	}
}

func SettingsPath() string {
	return filepath.Join(ConfigDir, "settings.toml")
}

func LoadSettings() (Settings, error) {
	settings := DefaultSettings()
	path := SettingsPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return settings, err
	}

	lines := strings.Split(string(data), "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return settings, fmt.Errorf("invalid settings line %q", line)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"")

		switch key {
		case "network_timeout":
			timeout, err := time.ParseDuration(value)
			if err != nil {
				return settings, fmt.Errorf("invalid network_timeout %q", value)
			}
			if timeout <= 0 {
				return settings, fmt.Errorf("invalid network_timeout %q", value)
			}
			settings.NetworkTimeout = timeout
		default:
			return settings, fmt.Errorf("unknown setting %q", key)
		}
	}

	return settings, nil
}
