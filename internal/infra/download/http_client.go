package download

import (
	"net/http"
	"time"

	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
)

var sharedHTTPClient = NewHTTPClient(30 * time.Second)

func NewHTTPClient(timeout time.Duration) *http.Client {
	return httpclient.NewDownload(timeout)
}

func SetHTTPClientTimeout(timeout time.Duration) {
	if sharedHTTPClient == nil {
		sharedHTTPClient = NewHTTPClient(timeout)
		return
	}
	sharedHTTPClient.Timeout = 0
	sharedHTTPClient.Transport = httpclient.NewTransport(timeout, timeout, timeout)
}

func SharedHTTPClient() *http.Client {
	if sharedHTTPClient == nil {
		sharedHTTPClient = NewHTTPClient(30 * time.Second)
	}
	return sharedHTTPClient
}
