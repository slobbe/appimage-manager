package main

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func flagString(cmd *cobra.Command, name string) (string, error) {
	flag := lookupFlag(cmd, name)
	if flag == nil {
		value, err := cmd.Flags().GetString(name)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(value), nil
	}
	return strings.TrimSpace(flag.Value.String()), nil
}

func flagBool(cmd *cobra.Command, name string) (bool, error) {
	flag := lookupFlag(cmd, name)
	if flag == nil {
		return cmd.Flags().GetBool(name)
	}
	return strconv.ParseBool(flag.Value.String())
}

func flagChanged(cmd *cobra.Command, name string) bool {
	if flag := lookupFlag(cmd, name); flag != nil && flag.Changed {
		return true
	}
	return false
}

func lookupFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag
	}
	if flag := cmd.PersistentFlags().Lookup(name); flag != nil {
		return flag
	}
	if flag := cmd.InheritedFlags().Lookup(name); flag != nil {
		return flag
	}
	return nil
}
