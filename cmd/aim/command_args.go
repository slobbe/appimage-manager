package main

import (
	"strings"

	"github.com/spf13/cobra"
)

func argsContainFlag(args []string, flag string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == flag {
			return true
		}
	}
	return false
}

func commandNameFromArgs(root *cobra.Command, args []string) string {
	if root == nil {
		return "aim"
	}

	command, _, err := root.Find(args)
	if err != nil || command == nil {
		return "aim"
	}
	return commandName(command)
}
