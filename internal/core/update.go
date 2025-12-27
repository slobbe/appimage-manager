package core

import (
	"bufio"
	"debug/elf"
	"fmt"
	"net/http"
	"strings"

	util "github.com/slobbe/appimage-manager/internal/helpers"
)

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

func IsUpdateAvailable(localSrc, zsyncUrl string) (bool, string, error) {
	resp, err := http.Get(zsyncUrl)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	var remoteSha1 string
	var remoteFilename string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "SHA-1:") {
			remoteSha1 = strings.TrimSpace(strings.TrimPrefix(line, "SHA-1:"))
		}
		if strings.HasPrefix(line, "Filename:") {
			remoteFilename = strings.TrimSpace(strings.TrimPrefix(line, "Filename:"))
		}
	}

	if remoteFilename == "" {
		remoteFilename = strings.TrimSuffix(zsyncUrl, ".zsync")
	}
	lastSlash := strings.LastIndex(zsyncUrl, "/")
	downloadLink := zsyncUrl[:lastSlash+1] + remoteFilename

	localPath, _ := util.MakeAbsolute(localSrc)
	localSha1, err := util.Sha1(localPath)
	if err != nil {
		return false, "", err
	}

	return localSha1 != remoteSha1, downloadLink, nil
}
