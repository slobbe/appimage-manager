package main

import (
	"fmt"

	"github.com/slobbe/appimage-manager/internal/core"
)

func Remove(appSlug string, keep bool) {
	fmt.Println("removing:", appSlug)
	fmt.Println("keep file:", keep)
	core.RemoveAppImage(appSlug, keep)
}
