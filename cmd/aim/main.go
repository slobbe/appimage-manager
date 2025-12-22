package main

import (
	"fmt"
	"os"
	"strings"

	command "github.com/slobbe/appimage-manager/internal/commands"
)

func main() {

	args := os.Args[1:]

	if len(args) == 0 {
		command.Help()
		return
	}

	switch strings.ToLower(args[0]) {
	case "--help", "-h", "help":
		command.Help()
	case "list":
		command.List()
	case "add":
		if len(args) < 2 {
			command.Help()
			return
		}
		command.Add(args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		os.Exit(1)
	}
}
