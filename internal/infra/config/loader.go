package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/infra/xdg"

	"github.com/pelletier/go-toml/v2"
)

type fileConfig struct {
	AppImageDir string `toml:"appimage_dir"`
}

func DefaultAppConfig(dirs xdg.Dirs) app.Config {
	return app.Config{
		ConfigFile:  xdg.ConfigFile(dirs),
		AppImageDir: xdg.DefaultAppImageDir(dirs),
		DesktopDir:  xdg.DesktopDir(dirs),
		IconDir:     xdg.IconDir(dirs),
	}
}

func Load(path string, dirs xdg.Dirs) (app.Config, error) {
	cfg := DefaultAppConfig(dirs)

	bytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}

		return app.Config{}, fmt.Errorf("read config file %q: %w", path, err)
	}

	var fileCfg fileConfig
	if err := toml.Unmarshal(bytes, &fileCfg); err != nil {
		return app.Config{}, fmt.Errorf("parse config file %q: %w", path, err)
	}

	if fileCfg.AppImageDir != "" {
		resolved, err := resolveUserPath(fileCfg.AppImageDir)
		if err != nil {
			return app.Config{}, fmt.Errorf("resolve appimage_dir: %w", err)
		}

		cfg.AppImageDir = resolved
	}

	return cfg, nil
}

func resolveUserPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}

	if path == "~" {
		return os.UserHomeDir()
	}

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}

	return filepath.Clean(path), nil
}
