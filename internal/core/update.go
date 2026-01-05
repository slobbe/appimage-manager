package core

import (
	//"bufio"
	"debug/elf"
	"fmt"
	"net/http"
	"strings"

	//"time"

	util "github.com/slobbe/appimage-manager/internal/helpers"
	models "github.com/slobbe/appimage-manager/internal/types"
	//repo "github.com/slobbe/appimage-manager/internal/repository"
)

/*
func CheckForUpdate(input string) (bool, string, error) {
	inputType, src, err := IdentifyInput(input)
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
		app, err := repo.GetApp(src)
		if err != nil {
			return false, "", err
		}

		if app.Type == "type-1" {
			return false, "", fmt.Errorf("%s is a type-1 appimage and doesn't contain update information", app.Name)
		}
		info = app.UpdateInfo
		sha1 = app.SHA1
		updatedTimestamp = app.UpdatedAt
	case InputTypeUnlinked:
		app, err := repo.GetApp(src)
		if err != nil {
			return false, "", err
		}
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
*/


type UpdateInfo struct {
	Kind models.UpdateKind
	
	UpdateInfo string
	UpdateUrl string
	Transport string
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

		zsyncFile = strings.Replace(zsyncFile, "*", tag, -1)

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

/*
type UpdateData struct {
	Available        bool
	DownloadUrl      string
	DownloadUrlZsync string
	RemoteTime       string
	RemoteSha1       string
	RemoteFilename   string
}

func IsUpdateAvailable(localSha1 string, localTime string, zsyncUrl string) (UpdateData, error) {
	update := UpdateData{
		Available:        false,
		DownloadUrl:      "",
		DownloadUrlZsync: zsyncUrl,
		RemoteSha1:       "",
		RemoteTime:       "",
		RemoteFilename:   "",
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
		}
	}

	if update.RemoteFilename == "" {
		update.RemoteFilename = strings.TrimSuffix(zsyncUrl, ".zsync")
	}
	lastSlash := strings.LastIndex(zsyncUrl, "/")
	update.DownloadUrl = zsyncUrl[:lastSlash+1] + update.RemoteFilename

	//localT, _ := time.Parse(time.RFC3339, localTime)
	//remoteT, _ := time.Parse(time.RFC3339, update.RemoteTime)

	update.Available = update.RemoteSha1 != localSha1

	return update, nil
}
 */
