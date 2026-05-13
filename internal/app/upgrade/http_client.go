package upgrade

import (
	"net/http"
	"time"

	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
)

var sharedHTTPClient = httpclient.New(30 * time.Second)

func SetHTTPClientTimeout(timeout time.Duration) {
	if sharedHTTPClient == nil {
		sharedHTTPClient = httpclient.New(timeout)
		return
	}
	sharedHTTPClient.Timeout = timeout
}

func SharedHTTPClient() *http.Client {
	if sharedHTTPClient == nil {
		sharedHTTPClient = httpclient.New(30 * time.Second)
	}
	return sharedHTTPClient
}
