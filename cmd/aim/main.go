package main

import (
	"fmt"
	"os"
	"path/filepath"

	"flag"

	command "github.com/slobbe/appimage-manager/internal/commands"
)

func main() {
	// declare `add` flags
	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	addMove := addCmd.Bool("move", false, "move AppImage to directory instead of copy")
	addAbsolute := addCmd.Bool("a", false, "AppImage's absolute path is given")

	// declare `remove` flags
	removeCmd := flag.NewFlagSet("rm", flag.ExitOnError)
	removeKeep := removeCmd.Bool("keep", false, "keep AppImage file")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s <command> [options] {args}\nCommands: add, remove, list\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}

	if len(os.Args) < 1 {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[0])
	}

	switch os.Args[1] {
	case "add":
		addCmd.Parse(os.Args[2:])
		fmt.Println("add", addCmd.Args()[0], "|| move app?", *addMove, "abs path?", *addAbsolute)

		if *addAbsolute {
			command.Add(addCmd.Args()[0], *addMove)
		} else {
			dir, err := os.Getwd()
			if err != nil {
				fmt.Println("Error:", err)
			}
			fmt.Println("Current directory:", dir)

			command.Add(filepath.Join(dir, addCmd.Args()[0]), *addMove)
		}
	case "list":
		command.List()
	case "rm":
		removeCmd.Parse(os.Args[2:])
		fmt.Println("rm", removeCmd.Args()[0], "|| keep file?", *removeKeep)
		command.Remove(removeCmd.Args()[0], *removeKeep)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[0])
	}
}
