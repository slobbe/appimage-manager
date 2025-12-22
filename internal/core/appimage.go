package core

import (
	//"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	
	"github.com/slobbe/appimage-manager/internal/helpers/filesystem"
)

// IntegrateAppImage extracts Foo.AppImage into ~/AppImages/foo and
// links its .desktop and icon into ~/.local/share so GNOME/KDE pick it up.
func IntegrateAppImage(appImageSrc string) error {
	home, _ := os.UserHomeDir()
	base := strings.TrimSuffix(filepath.Base(appImageSrc), filepath.Ext(appImageSrc))
	
	tempDir := filepath.Join(home, "AppImages", ".tmp")
	
	extractDir := filepath.Join(home, "AppImages", base)
	desktopDir := extractDir //filepath.Join(home, ".local/share/applications")
	iconDir := extractDir //filepath.Join(home, ".local/share/icons/hicolor/256x256/apps")

	
	tempExtractDir := filepath.Join(tempDir, base)
	for _, d := range []string{tempDir, extractDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	
	
	// extract
	if err := ExtractAppImage(appImageSrc, tempExtractDir); err != nil {
		return err
	}
	
	// locate desktop file
	desktopSrc, err := LocateDesktop(tempExtractDir)
	if err != nil {
		return err
	}
	
	//locate icon
	iconSrc, err := LocateIcon(tempExtractDir)
	if err != nil {
		return err
	}
	
	
	// move desktop, icon, and appimage to single dir
	fmt.Println(desktopSrc)
	newDesktopSrc, err := file.Move(desktopSrc, filepath.Join(desktopDir, base+".desktop"))
	if err != nil {
		return err
	}
	fmt.Println(newDesktopSrc)
	
	fmt.Println(iconSrc)
	newIconSrc, err := file.Move(iconSrc, filepath.Join(iconDir, base+filepath.Ext(iconSrc)))
	if  err != nil {
		return err
	}
	fmt.Println(newIconSrc)
	
	fmt.Println(appImageSrc)
	newAppImageSrc, err := file.Copy(appImageSrc, filepath.Join(extractDir, base+".AppImage"))
	if err != nil {
		return err
	}
	fmt.Println(newAppImageSrc)
	
	
	os.RemoveAll(tempExtractDir)
	
	// make appimage executable
	if err := file.MakeExecutable(newAppImageSrc); err != nil {
		return err
	}
	
	if err := UpdateDesktop(newDesktopSrc, newAppImageSrc, newIconSrc); err != nil {
		return err
	}
	
	
	desktopLink := filepath.Join(home, ".local", "share", "applications", base+".desktop")
	_ = os.Remove(desktopLink) // ignore if not exists
	if err := os.Symlink(newDesktopSrc, desktopLink); err != nil {
		return err
	}
	
	
	
	
	
	/* 
	// 3. symlink into place
	desktopLink := filepath.Join(desktopDir, base+".desktop")
	_ = os.Remove(desktopLink) // ignore if not exists
	if err := os.Symlink(desktopSrc, desktopLink); err != nil {
		return err
	}
	if iconSrc != "" {
		ext := filepath.Ext(iconSrc)
		iconLink := filepath.Join(iconDir, base+ext)
		_ = os.Remove(iconLink)
		if err := os.Symlink(iconSrc, iconLink); err != nil {
			return err
		}
	}

	// 4. fix Exec= and Icon= inside desktop file
	/* 
	content, err := os.ReadFile(desktopLink)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "Exec=") {
			lines[i] = "Exec=" + filepath.Join(extractDir, "AppRun")
		}
		if strings.HasPrefix(l, "Icon=") {
			lines[i] = "Icon=" + base
		}
	}
	
	return os.WriteFile(desktopLink, []byte(strings.Join(lines, "\n")), 0644)
	*/
	
	
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

func UpdateDesktop(desktopSrc string, execCmd string, iconSrc string) (error) {
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
