package update

import (
	"net/http"
	"time"

	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
)

var sharedHTTPClient = httpclient.New(30 * time.Second)

func NewHTTPClient(timeout time.Duration) *http.Client {
	return httpclient.New(timeout)
}

func SetHTTPClientTimeout(timeout time.Duration) {
	if sharedHTTPClient == nil {
		sharedHTTPClient = NewHTTPClient(timeout)
		return
	}
	sharedHTTPClient.Timeout = timeout
}

func SharedHTTPClient() *http.Client {
	if sharedHTTPClient == nil {
		sharedHTTPClient = NewHTTPClient(30 * time.Second)
	}
	return sharedHTTPClient
}
