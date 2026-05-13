package github

import (
	"net/http"
	"time"

	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
)

type Client struct {
	HTTPClient *http.Client
}

func (c Client) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return httpclient.New(30 * time.Second)
}
