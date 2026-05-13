package app

import (
	"net/http"
	"time"

	"github.com/slobbe/appimage-manager/internal/infra/github"
	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
)

var sharedHTTPClient = NewHTTPClient(30 * time.Second)
var sharedDownloadHTTPClient = NewDownloadHTTPClient(30 * time.Second)

func NewHTTPClient(timeout time.Duration) *http.Client {
	return httpclient.New(timeout)
}

func NewDownloadHTTPClient(timeout time.Duration) *http.Client {
	return httpclient.NewDownload(timeout)
}

func SetHTTPClientTimeout(timeout time.Duration) {
	if sharedHTTPClient == nil {
		sharedHTTPClient = NewHTTPClient(timeout)
		github.SetReleaseHTTPClient(sharedHTTPClient)
		return
	}
	sharedHTTPClient.Timeout = timeout
	github.SetReleaseHTTPClient(sharedHTTPClient)
}

func SetDownloadHTTPClientTimeout(timeout time.Duration) {
	if sharedDownloadHTTPClient == nil {
		sharedDownloadHTTPClient = NewDownloadHTTPClient(timeout)
		return
	}
	sharedDownloadHTTPClient.Timeout = 0
	sharedDownloadHTTPClient.Transport = httpclient.NewTransport(timeout, timeout, timeout)
}

func SharedHTTPClient() *http.Client {
	if sharedHTTPClient == nil {
		sharedHTTPClient = NewHTTPClient(30 * time.Second)
	}
	return sharedHTTPClient
}

func SharedDownloadHTTPClient() *http.Client {
	if sharedDownloadHTTPClient == nil {
		sharedDownloadHTTPClient = NewDownloadHTTPClient(30 * time.Second)
	}
	return sharedDownloadHTTPClient
}
