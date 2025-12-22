package commands

import (
	"fmt"
	"log"

	"github.com/slobbe/appimage-manager/internal/core"
)


func Extract(path string) {
	fmt.Println("Extract", path)

	if err := core.IntegrateAppImage(path); err != nil {
		log.Fatal(err)
	}
}
