package util

import (
	"os"
	"path/filepath"
	"strings"
)

func HasExtension(src string, ext string) bool {
	s := strings.TrimSpace(src)
	e := strings.TrimSpace(ext)
	return s != "" && e != "" && strings.EqualFold(filepath.Ext(s), e)
}

func RenameWithSameExt(src string, newName string) (string, error) {
	new := filepath.Join(filepath.Dir(src), newName+filepath.Ext(src))
	if err := os.Rename(src, new); err != nil {
		return src, err
	}
	return new, nil
}
