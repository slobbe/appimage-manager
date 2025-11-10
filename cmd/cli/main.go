package main

import (
	"os"
	"fmt"
	"log"
	"strings"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/appimage"
	"github.com/slobbe/appimage-manager/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	new, err := getNewAppImages(cfg.ScanDirs, cfg.LibraryDir)
	if err != nil {
		log.Fatal(err)
	}

	for _, app := range new {
		fmt.Println(app)
		appimage.MakeExecutable(app)
		meta, err := appimage.ExtractMetadata(app)
		if err != nil {
			log.Fatal(err)
		}

		entry := meta.Data["Desktop Entry"]
		entry["Exec"] = app

		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		name, _ := entry["Name"]
		desktopFile := fmt.Sprintf("%s.desktop", strings.ToLower(name))
		meta.SaveDesktop(filepath.Join(home, ".local", "share", "applications", desktopFile))

		jsonStr, _ := meta.ToJSON(true)
		fmt.Println(jsonStr)
	}
}

func getNewAppImages(scanDirs []string, libraryDir string) ([]string, error) {
	// Scan new AppImage files
	newAppImages, err := appimage.Scan(scanDirs)
	if err != nil {
		return nil, err
	}

	// Move to library
	movedAppImages, err := appimage.MoveToLibrary(newAppImages, libraryDir)
	if err != nil {
		return nil, err
	}

	return movedAppImages, nil
}
