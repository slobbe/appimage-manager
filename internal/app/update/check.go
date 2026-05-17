package update

import (
	"fmt"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type UpdateInfo struct {
	Kind       models.UpdateKind
	UpdateInfo string
	UpdateUrl  string
	Transport  string
}

type UpdateData struct {
	Available         bool
	DownloadUrl       string
	DownloadUrlZsync  string
	RemoteTime        string
	RemoteSHA1        string
	RemoteFilename    string
	NormalizedVersion string
	PreRelease        bool
	AssetName         string
}

func ZsyncUpdateCheck(upd *models.UpdateSource, localSHA1 string) (*UpdateData, error) {
	return ZsyncUpdateCheckWithFetcher(upd, localSHA1, defaultZsyncMetadataFetcher)
}

func ZsyncUpdateCheckWithFetcher(upd *models.UpdateSource, localSHA1 string, fetcher ZsyncMetadataFetcher) (*UpdateData, error) {
	if upd.Kind != models.UpdateZsync || upd.Zsync == nil {
		return nil, fmt.Errorf("no zsync update information")
	}

	if strings.TrimSpace(upd.Zsync.UpdateInfo) == "" {
		return nil, fmt.Errorf("missing zsync update info")
	}

	updateInfo, err := parseUpdateInfoString(upd.Zsync.UpdateInfo)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(updateInfo.UpdateUrl) == "" {
		return nil, fmt.Errorf("missing zsync update url")
	}

	if fetcher == nil {
		return nil, fmt.Errorf("zsync metadata fetcher is not configured")
	}
	metadata, err := fetcher.FetchMetadata(updateInfo.UpdateUrl)
	if err != nil {
		return nil, err
	}

	update := UpdateData{}
	update.DownloadUrlZsync = updateInfo.UpdateUrl
	update.RemoteSHA1 = metadata.RemoteSHA1
	update.RemoteFilename = metadata.RemoteFilename
	update.RemoteTime = metadata.RemoteTime
	update.NormalizedVersion = models.NormalizeComparableVersion(update.RemoteFilename)

	lastSlash := strings.LastIndex(update.DownloadUrlZsync, "/")
	update.DownloadUrl = update.DownloadUrlZsync[:lastSlash+1] + update.RemoteFilename
	update.AssetName = update.RemoteFilename

	update.Available = update.RemoteSHA1 != localSHA1

	return &update, nil
}

func GetUpdateInfo(src string) (*UpdateInfo, error) {
	return Service{UpdateInfoExtractor: defaultUpdateInfoExtractor}.GetUpdateInfo(src)
}

func (service Service) GetUpdateInfo(src string) (*UpdateInfo, error) {
	info, err := service.extractUpdateInfo(src)
	if err != nil {
		return nil, err
	}

	return parseUpdateInfoString(info)
}

func parseUpdateInfoString(info string) (*UpdateInfo, error) {
	info = strings.TrimSpace(info)
	if info == "" {
		return nil, fmt.Errorf("empty update info")
	}

	updateInfo := &UpdateInfo{
		UpdateInfo: info,
	}

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
			latestTag, err := ResolveLatestGitHubReleaseTag(owner, repo)
			if err != nil {
				return nil, err
			}

			tag = latestTag
		}

		zsyncFile = strings.ReplaceAll(zsyncFile, "*", tag)

		updateInfo.UpdateUrl = strings.Join(
			[]string{"https://github.com", owner, repo, "releases/download", tag, zsyncFile},
			"/")
	default:
		return nil, fmt.Errorf("unsupported update info kind %q", parts[0])
	}

	return updateInfo, nil
}

func (service Service) extractUpdateInfo(src string) (string, error) {
	if service.UpdateInfoExtractor == nil {
		return "", fmt.Errorf("update info extractor is not configured")
	}
	return service.UpdateInfoExtractor.ExtractUpdateInfo(src)
}
