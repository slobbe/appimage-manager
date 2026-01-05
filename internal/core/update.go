package core

import (
	"bufio"
	"debug/elf"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	util "github.com/slobbe/appimage-manager/internal/helpers"
)

func CheckForUpdate(input string) (bool, string, error) {
	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return false, "", err
	}

	inputType, src, err := IdentifyInput(input, db)
	if err != nil {
		return false, "", err
	}

	if inputType == InputTypeUnknown {
		return false, "", fmt.Errorf("unknown input type")
	}

	var info string
	var sha1 string
	var updatedTimestamp string

	switch inputType {
	case InputTypeAppImage:
		info, err = GetUpdateInfo(src)
		if err != nil {
			return false, "", err
		}

		sha1, err = util.Sha1(src)
		if err != nil {
			return false, "", err
		}
		updatedTimestamp = ""
	case InputTypeIntegrated:
		app := db.Apps[src]
		if app.Type == "type-1" {
			return false, "", fmt.Errorf("%s is a type-1 appimage and doesn't contain update information", app.Name)
		}
		info = app.UpdateInfo
		sha1 = app.SHA1
		updatedTimestamp = app.UpdatedAt
	case InputTypeUnlinked:
		app := db.Apps[src]
		if app.Type == "type-1" {
			return false, "", fmt.Errorf("%s is a type-1 appimage and doesn't contain update information", app.Name)
		}
		info = app.UpdateInfo
		sha1 = app.SHA1
		updatedTimestamp = app.UpdatedAt
	default:
		return false, "", fmt.Errorf("unknown input")
	}

	url, err := ResolveUpdateInfo(info)
	if err != nil {
		return false, "", err
	}

	update, err := IsUpdateAvailable(sha1, updatedTimestamp, url)
	if err != nil {
		return false, "", err
	}

	return update.Available, update.DownloadUrl, nil
}

func GetUpdateInfo(path string) (string, error) {
	path, err := util.MakeAbsolute(path)
	if err != nil {
		return "", err
	}

	f, err := elf.Open(path)
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

func ResolveUpdateInfo(info string) (string, error) {
	parts := strings.Split(info, "|")

	var url string
	switch parts[0] {
	case "zsync":
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid update info")
		}

		url = parts[1]
	case "gh-releases-zsync":
		if len(parts) < 5 {
			return "", fmt.Errorf("invalid update info")
		}
		owner := parts[1]
		repo := parts[2]
		tag := parts[3]
		zsyncFile := parts[4]

		if tag == "latest" {
			latestTag, err := GithubLatestVersionTag(owner, repo)
			if err != nil {
				return "", err
			}

			tag = latestTag
		}

		zsyncFile = strings.Replace(zsyncFile, "*", tag, -1)

		url = strings.Join(
			[]string{"https://github.com", owner, repo, "releases/download", tag, zsyncFile},
			"/")
	}

	return url, nil
}

func GithubLatestVersionTag(owner, repo string) (string, error) {
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

type UpdateData struct {
	Available bool
	DownloadUrl string
	DownloadUrlZsync string
	RemoteTime string
	RemoteSha1 string
	RemoteFilename string
}

func IsUpdateAvailable(localSha1 string, localTime string, zsyncUrl string) (UpdateData, error) {
	update := UpdateData{
		Available: false,
		DownloadUrl: "",
		DownloadUrlZsync: zsyncUrl,
		RemoteSha1: "",
		RemoteTime: "",
		RemoteFilename: "",
	}
	
	resp, err := http.Get(zsyncUrl)
	if err != nil {
		return update, err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "SHA-1:") {
			update.RemoteSha1 = strings.TrimSpace(strings.TrimPrefix(line, "SHA-1:"))
		}
		if strings.HasPrefix(line, "Filename:") {
			update.RemoteFilename = strings.TrimSpace(strings.TrimPrefix(line, "Filename:"))
		}
		if strings.HasPrefix(line, "MTime:") {
			t, _ := time.Parse(time.RFC1123, strings.TrimSpace(strings.TrimPrefix(line, "MTime:")))
			update.RemoteTime = t.Format(time.RFC3339)
			fmt.Printf("Remote time %s\n", update.RemoteTime)
		}
	}

	if update.RemoteFilename == "" {
		update.RemoteFilename = strings.TrimSuffix(zsyncUrl, ".zsync")
	}
	lastSlash := strings.LastIndex(zsyncUrl, "/")
	update.DownloadUrl = zsyncUrl[:lastSlash+1] + update.RemoteFilename

	localT, _ := time.Parse(time.RFC3339, localTime)
	fmt.Printf("Local time %s\n", localT.Format(time.RFC3339))
	remoteT, _ := time.Parse(time.RFC3339, update.RemoteTime)

	update.Available = remoteT.After(localT) && update.RemoteSha1 != localSha1

	return update, nil
}
