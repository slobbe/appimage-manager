package core

import (
	"net"
	"net/http"
	"time"
)

var sharedHTTPClient = NewHTTPClient(30 * time.Second)
var sharedDownloadHTTPClient = NewDownloadHTTPClient(30 * time.Second)

func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: newHTTPTransport(10*time.Second, 10*time.Second, 0),
	}
}

func NewDownloadHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   0,
		Transport: newHTTPTransport(timeout, timeout, timeout),
	}
}

func SetHTTPClientTimeout(timeout time.Duration) {
	if sharedHTTPClient == nil {
		sharedHTTPClient = NewHTTPClient(timeout)
		return
	}
	sharedHTTPClient.Timeout = timeout
}

func SetDownloadHTTPClientTimeout(timeout time.Duration) {
	if sharedDownloadHTTPClient == nil {
		sharedDownloadHTTPClient = NewDownloadHTTPClient(timeout)
		return
	}
	sharedDownloadHTTPClient.Timeout = 0
	sharedDownloadHTTPClient.Transport = newHTTPTransport(timeout, timeout, timeout)
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

func newHTTPTransport(dialTimeout, tlsHandshakeTimeout, responseHeaderTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
