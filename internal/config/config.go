package config

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// Config structure
type Config struct {
	ScanDirs    []string `json:"scan_dirs"`
	LibraryDir  string   `json:"library_dir"`
	DefaultIcon string   `json:"default_icon"`
}

// Default configuration values
func Default() Config {
	home, _ := os.UserHomeDir()
	return Config{
		ScanDirs:    []string{"Downloads", "Applications", "AppImages"},
		LibraryDir:  ".appimages",
		DefaultIcon: filepath.Join(home, ".config", "AppImager", "default.png"),
	}
}

// Initialize ensures ~/.config/AppImager exists, default.png and config.json exist.
func Initialize() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	cfgDir := filepath.Join(home, ".config", "AppImager")
	iconDest := filepath.Join(cfgDir, "default.png")
	iconSrc := filepath.Join("assets", "default.png")
	cfgPath := filepath.Join(cfgDir, "config.json")

	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return err
	}

	// copy default icon if missing
	if _, err := os.Stat(iconDest); os.IsNotExist(err) {
		src, err := os.Open(iconSrc)
		if err != nil {
			return err
		}
		defer src.Close()

		dest, err := os.Create(iconDest)
		if err != nil {
			return err
		}
		defer dest.Close()

		if _, err := io.Copy(dest, src); err != nil {
			return err
		}
	}

	// create config.json if missing
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfg := Default()
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(cfgPath, data, 0644); err != nil {
			return err
		}
	}

	return nil
}

// Load reads the configuration, calling Initialize first.
func Load() (Config, error) {
	if err := Initialize(); err != nil {
		return Default(), err
	}

	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, ".config", "AppImager", "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return Default(), err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), err
	}

	return cfg, nil
}

func Save(cfg Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "AppImager")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
