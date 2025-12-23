package core

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	crypt "github.com/slobbe/appimage-manager/internal/helpers/crypt"
	file "github.com/slobbe/appimage-manager/internal/helpers/filesystem"
)

type AppInfo struct {
	Name    string
	Version string
}

func IntegrateAppImage(appImageSrc string, move bool) error {
	home, _ := os.UserHomeDir()

	desktopDir := filepath.Join(home, ".local/share/applications")
	aimDir := filepath.Join(home, ".local/share/appimagemanager")

	base := strings.TrimSuffix(filepath.Base(appImageSrc), filepath.Ext(appImageSrc))

	tempDir := filepath.Join(aimDir, ".tmp")
	tempExtractDir := filepath.Join(tempDir, base)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return err
	}

	// extract
	if err := ExtractAppImage(appImageSrc, tempExtractDir); err != nil {
		return err
	}

	// locate desktop and icon files
	desktopSrc, err := LocateDesktop(tempExtractDir)
	if err != nil {
		return err
	}

	iconSrc, err := LocateIcon(tempExtractDir)
	if err != nil {
		return err
	}

	info, err := ExtractAppInfo(desktopSrc)
	var appName, appVersion, appSlug string
	if err != nil {
		appName = base
		appSlug = strings.ToLower(base)
		appVersion = "unknown"
		log.Fatal(err)
	} else {
		appName = info.Name //strings.ToLower(info.Name)
		appSlug = strings.ToLower(info.Name)
		appVersion = info.Version

	}
	

	extractDir := filepath.Join(aimDir, appSlug)

	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}

	// move desktop, icon, and appimage to extract dir
	var newAppImageSrc string
	if move {
		newAppImageSrc, err = file.Move(appImageSrc, filepath.Join(extractDir, appSlug+".AppImage"))
		if err != nil {
			return err
		}
	} else {
		newAppImageSrc, err = file.Copy(appImageSrc, filepath.Join(extractDir, appSlug+".AppImage"))
		if err != nil {
			return err
		}
	}

	newDesktopSrc, err := file.Move(desktopSrc, filepath.Join(extractDir, appSlug+".desktop"))
	if err != nil {
		return err
	}

	newIconSrc, err := file.Move(iconSrc, filepath.Join(extractDir, appSlug+filepath.Ext(iconSrc)))
	if err != nil {
		return err
	}

	os.RemoveAll(tempExtractDir)

	// make appimage executable
	if err := file.MakeExecutable(newAppImageSrc); err != nil {
		return err
	}

	if err := UpdateDesktop(newDesktopSrc, newAppImageSrc, newIconSrc); err != nil {
		return err
	}

	// make desktop symlink for system integration
	desktopLink := filepath.Join(desktopDir, "aim-"+appSlug+".desktop")
	_ = os.Remove(desktopLink)
	if err := os.Symlink(newDesktopSrc, desktopLink); err != nil {
		return err
	}

	// add to db
	dbPath := filepath.Join(aimDir, "db.json")

	db, err := LoadDB(dbPath)
	if err != nil {
		return err
	}
	
	sum, err := crypt.Sha256File(newAppImageSrc)
	if err != nil {
		return err
	}

	db.Apps[appSlug] = &App{
		Name:        appName,
		Version:     appVersion,
		AppImageSrc: newAppImageSrc,
		SHA256:      sum,
		DesktopSrc:  newDesktopSrc,
		DesktopLink: desktopLink,
		IconSrc:     newIconSrc,
		AddedAt:     NowISO(),
	}

	if err := SaveDB(dbPath, db); err != nil {
		return err
	}

	// refresh desktop cache best-effort
	_ = exec.Command("update-desktop-database", desktopDir).Run()
	
	fmt.Printf("Successfully added %s (v%s)", appName, appVersion)
	return nil
}

func ExtractAppImage(appImageSrc, outDir string) error {
	if err := file.MakeExecutable(appImageSrc); err != nil {
		return err
	}
	cmd := exec.Command(appImageSrc, "--appimage-extract")
	cmd.Dir = filepath.Dir(outDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract failed: %w\n%s", err, out)
	}
	if err := os.Rename(filepath.Join(filepath.Dir(outDir), "squashfs-root"), outDir); err != nil {
		return err
	}

	return nil
}

func LocateDesktop(dir string) (string, error) {
	desktopGlob, _ := filepath.Glob(filepath.Join(dir, "*.desktop"))
	if len(desktopGlob) == 0 {
		return "", fmt.Errorf("no desktop file found inside AppImage")
	}
	return desktopGlob[0], nil
}

func LocateIcon(dir string) (string, error) {
	iconGlob, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	if len(iconGlob) == 0 {
		iconGlob, _ = filepath.Glob(filepath.Join(dir, "*.svg"))
	}

	if len(iconGlob) > 0 {
		iconSrc := iconGlob[0]
		real, err := filepath.EvalSymlinks(iconSrc)
		if err != nil {
			return "", err
		}

		return real, nil
	} else {
		return "", fmt.Errorf("no icon found inside AppImage")
	}
}

func UpdateDesktop(desktopSrc string, execCmd string, iconSrc string) error {
	content, err := os.ReadFile(desktopSrc)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "Exec=") {
			lines[i] = "Exec=" + execCmd
		}
		if strings.HasPrefix(l, "Icon=") {
			lines[i] = "Icon=" + iconSrc
		}
	}

	return os.WriteFile(desktopSrc, []byte(strings.Join(lines, "\n")), 0644)
}

func ExtractAppInfo(desktopPath string) (*AppInfo, error) {
	file, err := os.Open(desktopPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	info := &AppInfo{}
	scanner := bufio.NewScanner(file)
	inDesktopEntry := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check if we entered [Desktop Entry] section
		if line == "[Desktop Entry]" {
			inDesktopEntry = true
			continue
		}

		// Stop if we hit another section
		if len(line) > 0 && line[0] == '[' && line != "[Desktop Entry]" {
			break
		}

		if !inDesktopEntry {
			continue
		}

		// Parse key=value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Name":
			info.Name = value
		case "X-AppImage-Version":
			info.Version = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return info, nil
}
