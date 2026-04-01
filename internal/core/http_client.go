package core

import (
	"net"
	"net/http"
	"time"
)

var sharedHTTPClient = NewHTTPClient(30 * time.Second)

func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
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
