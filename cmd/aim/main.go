package main

import (
	"fmt"
	"os"
	"path/filepath"

	"flag"
)

func main() {
	// declare `add` flags
	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	addMove := addCmd.Bool("mv", false, "move AppImage to directory instead of copy")
	addAbsolute := addCmd.Bool("a", false, "AppImage's absolute path is given")

	// declare `remove` flags
	removeCmd := flag.NewFlagSet("rm", flag.ExitOnError)
	removeKeep := removeCmd.Bool("k", false, "keep AppImage file")

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

		if *addAbsolute {
			Add(addCmd.Args()[0], *addMove)
		} else {
			dir, err := os.Getwd()
			if err != nil {
				fmt.Println("Error:", err)
			}

			Add(filepath.Join(dir, addCmd.Args()[0]), *addMove)
		}
	case "list":
		List()
	case "remove", "rm":
		removeCmd.Parse(os.Args[2:])
		Remove(removeCmd.Args()[0], *removeKeep)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[0])
	}
}
