package upgrade

import (
	"net/http"
	"time"
)

var sharedHTTPClient = &http.Client{Timeout: 30 * time.Second}

func SetHTTPClientTimeout(timeout time.Duration) {
	if sharedHTTPClient == nil {
		sharedHTTPClient = &http.Client{Timeout: timeout}
		return
	}
	sharedHTTPClient.Timeout = timeout
}

func SharedHTTPClient() *http.Client {
	if sharedHTTPClient == nil {
		sharedHTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return sharedHTTPClient
}
