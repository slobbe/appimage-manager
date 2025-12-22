package main

import (
	"fmt"
	"os"
	//"strings"
	"flag"

	command "github.com/slobbe/appimage-manager/internal/commands"
)

func main() {
	// declare flags
	help := flag.Bool("help", false, "show help")
	
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s <command> [options]\nCommands: add, remove, list\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}
	
	flag.Parse()
	
	if *help || len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(0)
	}
	
	switch flag.Args()[0] {
	case "add":
		command.Add(flag.Args()[1])
	case "list":
		command.List()
	case "remove":
		command.Remove(flag.Args()[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", flag.Args()[0])
		flag.Usage()
		os.Exit(1)
	}
}
