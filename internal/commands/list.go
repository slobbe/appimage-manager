package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/core"
)

func List() error {
	home, _ := os.UserHomeDir()
	aimDir := filepath.Join(home, ".local/share/appimage-manager")
	dbPath := filepath.Join(aimDir, "apps.json")
	unlinkedDbPath := filepath.Join(aimDir, "unlinked.json")

	db, err := core.LoadDB(dbPath)
	if err != nil {
		return err
	}

	apps := db.Apps

	unlinkedDb, err := core.LoadDB(unlinkedDbPath)
	if err != nil {
		return err
	}

	unlinkedApps := unlinkedDb.Apps

	fmt.Printf("%s%-15s %-20s %-10s%s\n", "\033[1m\033[4m", "ID", "App Name", "Version", "\033[0m")

	for _, app := range apps {
		fmt.Fprintf(os.Stdout, "%-15s %-20s %-10s\n", app.Slug, app.Name, app.Version)
	}

	for _, app := range unlinkedApps {
		fmt.Fprintf(os.Stdout, "%s%-15s %-20s %-10s%s\n", "\033[2m\033[3m", app.Slug, app.Name, app.Version, "\033[0m")
	}

	return nil
}
