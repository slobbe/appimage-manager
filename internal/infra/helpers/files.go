package util

import (
	"path/filepath"
	"strings"
)

func HasExtension(src string, ext string) bool {
	s := strings.TrimSpace(src)
	e := strings.TrimSpace(ext)
	return s != "" && e != "" && strings.EqualFold(filepath.Ext(s), e)
}
