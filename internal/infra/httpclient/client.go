package httpclient

import (
	"net"
	"net/http"
	"time"
)

func New(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewTransport(10*time.Second, 10*time.Second, 0),
	}
}

func NewDownload(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   0,
		Transport: NewTransport(timeout, timeout, timeout),
	}
}

func NewTransport(dialTimeout, tlsHandshakeTimeout, responseHeaderTimeout time.Duration) *http.Transport {
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
