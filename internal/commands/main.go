package commands

import (
	"fmt"
	"os"
	"strings"
)

func Execute(args []string) {
	if len(args) == 0 {
		Help()
		return
	}

	switch strings.ToLower(args[0]) {
	case "--help", "-h", "help":
		Help()
	case "list":
		List()
	case "extract":
		if len(args) < 2 {
			Help()
			return
		}
		Extract(args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		os.Exit(1)
	}
}
