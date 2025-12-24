package main

import (
	"fmt"
	"log"

	"github.com/slobbe/appimage-manager/internal/core"
)

func Add(path string, move bool) {
	fmt.Println("Add", path)

	if err := core.IntegrateAppImage(path, move); err != nil {
		log.Fatal(err)
	}
}
