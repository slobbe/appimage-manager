package commands

import (
	"fmt"
	"log"

	"github.com/slobbe/appimage-manager/internal/core"
)


func Add(path string) {
	fmt.Println("Add", path)

	if err := core.IntegrateAppImage(path); err != nil {
		log.Fatal(err)
	}
}
