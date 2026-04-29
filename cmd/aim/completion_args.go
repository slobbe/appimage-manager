package main

import (
	"sort"
	"strings"

	repo "github.com/slobbe/appimage-manager/internal/repository"
	"github.com/spf13/cobra"
)

type managedAppCompletion struct {
	id   string
	name string
}

func completeManagedAppIDs(toComplete string, directive cobra.ShellCompDirective) ([]cobra.Completion, cobra.ShellCompDirective) {
	apps, err := repo.GetAllApps()
	if err != nil {
		return nil, directive
	}

	prefix := strings.TrimSpace(toComplete)
	rows := make([]managedAppCompletion, 0, len(apps))
	seen := make(map[string]bool, len(apps))

	for key, app := range apps {
		if app == nil {
			continue
		}

		id := strings.TrimSpace(app.ID)
		if id == "" {
			id = strings.TrimSpace(key)
		}
		if id == "" || seen[id] {
			continue
		}
		if prefix != "" && !strings.HasPrefix(id, prefix) {
			continue
		}

		seen[id] = true
		rows = append(rows, managedAppCompletion{
			id:   id,
			name: strings.TrimSpace(app.Name),
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].id < rows[j].id
	})

	completions := make([]cobra.Completion, 0, len(rows))
	for _, row := range rows {
		if row.name != "" && row.name != row.id {
			completions = append(completions, cobra.CompletionWithDesc(row.id, row.name))
			continue
		}
		completions = append(completions, row.id)
	}

	return completions, directive
}

func looksLikePathCompletionToken(toComplete string) bool {
	trimmed := strings.TrimSpace(toComplete)
	if trimmed == "" {
		return false
	}

	return strings.HasPrefix(trimmed, ".") ||
		strings.HasPrefix(trimmed, "/") ||
		strings.HasPrefix(trimmed, "~") ||
		strings.Contains(trimmed, "/")
}

func completeManagedAppIDTarget(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeManagedAppIDs(toComplete, cobra.ShellCompDirectiveNoFileComp)
}

func completeAddTarget(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if flagChanged(cmd, "url") || flagChanged(cmd, "github") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if looksLikePathCompletionToken(toComplete) {
		return nil, cobra.ShellCompDirectiveDefault
	}
	return completeManagedAppIDs(toComplete, cobra.ShellCompDirectiveNoFileComp)
}

func completeInfoTarget(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if flagChanged(cmd, "github") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if looksLikePathCompletionToken(toComplete) {
		return nil, cobra.ShellCompDirectiveDefault
	}
	return completeManagedAppIDs(toComplete, cobra.ShellCompDirectiveNoFileComp)
}

func completeUpdateTarget(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if flagChanged(cmd, "set") || flagChanged(cmd, "unset") || hasUpdateSetFlags(cmd) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeManagedAppIDs(toComplete, cobra.ShellCompDirectiveNoFileComp)
}

func completeManagedAppIDFlagValue(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return completeManagedAppIDs(toComplete, cobra.ShellCompDirectiveNoFileComp)
}

func mustRegisterFlagCompletion(cmd *cobra.Command, name string, fn cobra.CompletionFunc) {
	if err := cmd.RegisterFlagCompletionFunc(name, fn); err != nil {
		panic(err)
	}
}
