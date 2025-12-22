package main

import (
	"os"
	"github.com/slobbe/appimage-manager/internal/commands"
)

func main() {
	if len(os.Args) > 1 {		
		commands.Execute(os.Args[1:])
	}
}
