package core

import (
	"encoding/hex"
	"net/url"
	"path"
	"strings"
)

func isHTTPSURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return false
	}
	if strings.TrimSpace(u.Host) == "" {
		return false
	}
	return true
}

func isValidSHA256Hex(value string) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}

func filenameFromURL(value string) string {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(path.Base(u.Path))
	if name == "." || name == "/" {
		return ""
	}
	return name
}
