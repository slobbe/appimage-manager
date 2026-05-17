package update

import (
	"net/http"
	"time"
)

var sharedHTTPClient = &http.Client{Timeout: 30 * time.Second}

func NewHTTPClient(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }
func SharedHTTPClient() *http.Client                   { return sharedHTTPClient }
func SetGitHubReleaseResolver(resolver GitHubReleaseResolver) {
	defaultGitHubReleaseResolver = resolver
}
func SetZsyncMetadataFetcher(fetcher ZsyncMetadataFetcher)   { defaultZsyncMetadataFetcher = fetcher }
func SetStagedDownloadService(service StagedDownloadService) { defaultStagedDownload = service }
func SetHashVerifier(verifier HashVerifier)                  { defaultHashVerifier = verifier }
func SetUpdateInfoExtractor(extractor UpdateInfoExtractor)   { defaultUpdateInfoExtractor = extractor }
