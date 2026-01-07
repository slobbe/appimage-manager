package core

import (
	"bufio"
	"debug/elf"
	"fmt"
	"net/http"
	"strings"
	"time"

	util "github.com/slobbe/appimage-manager/internal/helpers"
	models "github.com/slobbe/appimage-manager/internal/types"
)

type UpdateInfo struct {
	Kind       models.UpdateKind
	UpdateInfo string
	UpdateUrl  string
	Transport  string
}

type UpdateData struct {
	Available        bool
	DownloadUrl      string
	DownloadUrlZsync string
	RemoteTime       string
	RemoteSHA1       string
	RemoteFilename   string
	PreRelease       bool
	AssetName        string
}

func ZsyncUpdateCheck(upd *models.UpdateSource, localSHA1 string) (*UpdateData, error) {
	if upd.Kind != "zsync" || upd.Zsync == nil {
		return nil, fmt.Errorf("no zsync update information")
	}

	resp, err := http.Get(upd.Zsync.UpdateUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	update := UpdateData{}
	update.DownloadUrlZsync = upd.Zsync.UpdateUrl

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}

		sha1, exists := strings.CutPrefix(line, "SHA-1:")
		if exists {
			update.RemoteSHA1 = strings.TrimSpace(sha1)
		}

		filename, exists := strings.CutPrefix(line, "Filename:")
		if exists {
			update.RemoteFilename = strings.TrimSpace(filename)
		}

		mtime, exists := strings.CutPrefix(line, "MTime:")
		if exists {
			t, _ := time.Parse(time.RFC1123, strings.TrimSpace(mtime))
			update.RemoteTime = t.Format(time.RFC3339)
		}
	}

	if update.RemoteFilename != "" {
		update.RemoteFilename = strings.TrimSuffix(update.RemoteFilename, ".zsync")
	}

	lastSlash := strings.LastIndex(update.DownloadUrlZsync, "/")
	update.DownloadUrl = update.DownloadUrlZsync[:lastSlash+1] + update.RemoteFilename

	update.Available = update.RemoteSHA1 != localSHA1

	return &update, nil
}

func GetUpdateInfo(src string) (*UpdateInfo, error) {
	info, err := extractUpdateInfo(src)
	if err != nil {
		return nil, err
	}

	updateInfo := &UpdateInfo{}

	updateInfo.UpdateInfo = info

	parts := strings.Split(info, "|")

	switch parts[0] {
	case "zsync":
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid update info")
		}

		updateInfo.Kind = models.UpdateZsync
		updateInfo.Transport = "zsync"
		updateInfo.UpdateUrl = parts[1]
	case "gh-releases-zsync":
		if len(parts) < 5 {
			return nil, fmt.Errorf("invalid update info")
		}
		updateInfo.Kind = models.UpdateZsync
		updateInfo.Transport = "gh-releases"

		owner := parts[1]
		repo := parts[2]
		tag := parts[3]
		zsyncFile := parts[4]

		if tag == "latest" {
			latestTag, err := githubLatestVersionTag(owner, repo)
			if err != nil {
				return nil, err
			}

			tag = latestTag
		}

		zsyncFile = strings.ReplaceAll(zsyncFile, "*", tag)

		updateInfo.UpdateUrl = strings.Join(
			[]string{"https://github.com", owner, repo, "releases/download", tag, zsyncFile},
			"/")
	}

	return updateInfo, nil
}

func githubLatestVersionTag(owner, repo string) (string, error) {
	url := fmt.Sprintf("https://github.com/%s/%s/releases/latest", owner, repo)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	loc := resp.Header.Get("Location")
	parts := strings.Split(loc, "/")

	return parts[len(parts)-1], nil
}

func extractUpdateInfo(src string) (string, error) {
	if !util.HasExtension(src, ".AppImage") {
		return "", fmt.Errorf("source must be .AppImage file")
	}

	src, err := util.MakeAbsolute(src)
	if err != nil {
		return "", err
	}

	f, err := elf.Open(src)
	if err != nil {
		return "", err
	}
	defer f.Close()

	section := f.Section(".upd_info")
	if section == nil {
		return "", fmt.Errorf("no update information found in ELF headers")
	}

	data, err := section.Data()
	if err != nil {
		return "", err
	}

	strData := string(data)
	if i := strings.Index(strData, "\x00"); i != -1 {
		strData = strData[:i]
	}

	return strings.TrimSpace(strData), nil
}
