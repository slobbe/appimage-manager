package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"flag"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/core"
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
			CmdAdd(addCmd.Args()[0], *addMove)
		} else {
			dir, err := os.Getwd()
			if err != nil {
				fmt.Println("Error:", err)
			}

			CmdAdd(filepath.Join(dir, addCmd.Args()[0]), *addMove)
		}
	case "list":
		CmdList()
	case "remove", "rm":
		removeCmd.Parse(os.Args[2:])
		CmdRemove(removeCmd.Args()[0], *removeKeep)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[0])
	}
}

func CmdAdd(path string, move bool) {
	fmt.Printf("Adding %s ...\n", path)

	if err := core.IntegrateAppImage(path, move); err != nil {
		log.Fatal(err)
	}
}

func CmdRemove(appSlug string, keep bool) {
	fmt.Printf("Removing %s ...\n", appSlug)

	if err := core.RemoveAppImage(appSlug, keep); err != nil {
		log.Fatal(err)
	}
}

func CmdList() error {
	db, err := core.LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	apps := db.Apps

	unlinkedDb, err := core.LoadDB(config.UnlinkedDbSrc)
	if err != nil {
		return err
	}

	unlinkedApps := unlinkedDb.Apps

	fmt.Printf("%s%-15s %-20s %-10s%s\n", "\033[1m\033[4m", "ID", "App Name", "Version", "\033[0m")

	for _, app := range apps {
		fmt.Fprintf(os.Stdout, "%-15s %-20s %-10s\n", app.Slug, app.Name, app.Version)
	}

	for _, app := range unlinkedApps {
		fmt.Fprintf(os.Stdout, "%s%-15s %-20s %-10s%s\n", "\033[2m\033[3m", app.Slug, app.Name, app.Version, "\033[0m")
	}

	return nil
}
