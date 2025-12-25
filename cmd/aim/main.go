package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"flag"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/core"
)

func main() {
	// declare `add` flags
	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	addMove := addCmd.Bool("mv", false, "move AppImage to directory instead of copy")

	// declare `remove` flags
	removeCmd := flag.NewFlagSet("rm", flag.ExitOnError)
	removeKeep := removeCmd.Bool("k", false, "keep AppImage file")
	
	// declare `list` flags
	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	listIntegrated := listCmd.Bool("i", false, "list only integrated appimages")
	listUnlinked := listCmd.Bool("u", false, "list only unlinked appimages")
	listAll := listCmd.Bool("a", false, "list all appimages")

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
		CmdAdd(addCmd.Args()[0], *addMove)
	case "list":
		listCmd.Parse(os.Args[2:])
		listCategory := "all"
		if *listIntegrated && *listUnlinked {
			listCategory = "all"
		} else if *listAll {
			listCategory = "all"
		} else if *listIntegrated {
			listCategory = "integrated"
		} else if *listUnlinked {
			listCategory = "unlinked"
		}
		
		CmdList(listCategory)
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

func CmdList(category string) error {
	db, err := core.LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	apps := db.Apps

	fmt.Printf("\033[1m\033[4m%-15s %-20s %-15s %-10s\033[0m\n", "ID", "App Name", "Version", "Added")

	addedAtFormat := time.DateOnly

	if category == "all" || category == "integrated" {
		for _, app := range apps {
			if len(app.DesktopLink) > 0 {
				addedAt, _ := time.Parse(time.RFC3339, app.AddedAt)
				fmt.Fprintf(os.Stdout, "%-15s %-20s %-15s %-10s\n", app.Slug, app.Name, app.Version, addedAt.Format(addedAtFormat))
			}
		}
	}

	if category == "all" || category == "unlinked" {
		for _, app := range apps {
			if len(app.DesktopLink) == 0 {
				addedAt, _ := time.Parse(time.RFC3339, app.AddedAt)
				fmt.Fprintf(os.Stdout, "\033[2m\033[3m%-15s %-20s %-15s %-10s\033[0m\n", app.Slug, app.Name, app.Version, addedAt.Format(addedAtFormat))
			}
		}
	}
	return nil
}
