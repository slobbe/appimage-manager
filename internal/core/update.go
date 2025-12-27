package core

import (
	"debug/elf"
	"fmt"

	util "github.com/slobbe/appimage-manager/internal/helpers"
)

func GetUpdateInfo(path string) (string, error) {
	path, err := util.MakeAbsolute(path)
	if err != nil {
		return "", err
	}
	
	f, err := elf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	section := f.Section(".upd_info")
	if section == nil {
		return "", fmt.Errorf("no update information found in ELF headers")
	}

	data, err := section.Data()
	if err != nil {
		return "", err
	}

	return string(data), nil
}
