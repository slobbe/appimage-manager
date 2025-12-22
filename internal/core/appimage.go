package core

import (
	//"errors"
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	file "github.com/slobbe/appimage-manager/internal/helpers/filesystem"
)

// IntegrateAppImage extracts Foo.AppImage into ~/AppImages/foo and
// links its .desktop and icon into ~/.local/share so GNOME/KDE pick it up.
func IntegrateAppImage(appImageSrc string) error {
	home, _ := os.UserHomeDir()
	base := strings.TrimSuffix(filepath.Base(appImageSrc), filepath.Ext(appImageSrc))

	tempDir := filepath.Join(home, "AppImages", ".tmp")
	desktopDir := filepath.Join(home, ".local/share/applications")

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

	// get app name if available
	appName, err := ExtractAppName(desktopSrc)
	if err != nil {
		appName = base
	}

	appName = strings.ToLower(appName)

	extractDir := filepath.Join(home, "AppImages", appName)

	//iconDir := extractDir    //filepath.Join(home, ".local/share/icons/hicolor/256x256/apps")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}

	// move desktop, icon, and appimage to extract dir

	newAppImageSrc, err := file.Copy(appImageSrc, filepath.Join(extractDir, base+".AppImage"))
	if err != nil {
		return err
	}
	newDesktopSrc, err := file.Move(desktopSrc, filepath.Join(extractDir, base+".desktop"))
	if err != nil {
		return err
	}

	newIconSrc, err := file.Move(iconSrc, filepath.Join(extractDir, appName+filepath.Ext(iconSrc)))
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
	desktopLink := filepath.Join(desktopDir, "aim-"+appName+".desktop")
	_ = os.Remove(desktopLink)
	if err := os.Symlink(newDesktopSrc, desktopLink); err != nil {
		return err
	}

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

func ExtractAppName(desktopSrc string) (string, error) {
	file, err := os.Open(desktopSrc)
	if err != nil {
		return "", err
	}
	defer file.Close()

	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "Name=") {
			return strings.TrimSpace(strings.SplitN(line, "=", 2)[1]), nil
		}
	}
	return "", sc.Err()
}
