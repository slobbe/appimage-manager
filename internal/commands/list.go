package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/core"
)

func List() error {
	home, _ := os.UserHomeDir()
	aimDir := filepath.Join(home, ".local/share/appimagemanager")
	dbPath := filepath.Join(aimDir, "apps.json")

	db, err := core.LoadDB(dbPath)
	if err != nil {
		return err
	}

	apps := db.Apps

	fmt.Printf("%s%-15s %-20s %-10s%s\n", "\033[1m", "ID", "App Name", "Version", "\033[0m")

	for _, app := range apps {
		fmt.Fprintf(os.Stdout, "%-15s %-20s %-10s\n", app.Slug, app.Name, app.Version)
	}

	return nil
}
