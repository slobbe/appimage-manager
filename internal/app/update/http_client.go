package update

import (
	"net/http"
	"time"
)

var sharedHTTPClient = &http.Client{Timeout: 30 * time.Second}

func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
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
