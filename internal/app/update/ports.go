package update

import "context"

const MetadataMaxBytes = 1 << 20

type ZsyncRunner interface {
	Apply(ctx context.Context, currentPath, zsyncURL, destination string) error
}

type ZsyncMetadataFetcher interface {
	FetchMetadata(url string) ([]byte, error)
}

type DownloadProgress struct {
	Downloaded int64
	Total      int64
}

type StagedDownloadService interface {
	AppImageFilename(assetName, downloadURL string) string
	Download(ctx context.Context, assetURL, destination string, onProgress func(DownloadProgress)) error
	RemoveStaged(downloadPath string)
	StableDestination(dir, assetURL, nameHint string) (string, error)
}

type HashVerifier interface {
	VerifyHashes(path, expectedSHA256, expectedSHA1 string) error
}

type PathResolver interface {
	MakeAbsolute(path string) (string, error)
}

var (
	defaultZsyncMetadataFetcher ZsyncMetadataFetcher
	defaultStagedDownload       StagedDownloadService
	defaultHashVerifier         HashVerifier
	defaultPathResolver         PathResolver
)

func SetZsyncMetadataFetcher(fetcher ZsyncMetadataFetcher) {
	defaultZsyncMetadataFetcher = fetcher
}

func SetStagedDownloadService(service StagedDownloadService) {
	defaultStagedDownload = service
}

func SetHashVerifier(verifier HashVerifier) {
	defaultHashVerifier = verifier
}

func SetPathResolver(resolver PathResolver) {
	defaultPathResolver = resolver
}
