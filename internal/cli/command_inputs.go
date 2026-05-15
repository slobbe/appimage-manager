package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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

type stringMetavarValue struct {
	value    *string
	typeName string
}

func (v *stringMetavarValue) String() string {
	if v == nil || v.value == nil {
		return ""
	}
	return *v.value
}

func (v *stringMetavarValue) Set(value string) error {
	if v == nil || v.value == nil {
		return nil
	}
	*v.value = value
	return nil
}

func (v *stringMetavarValue) Type() string {
	return v.typeName
}

func stringFlagWithMetavar(fs *pflag.FlagSet, name, shorthand, defaultValue, usage, typeName string) {
	value := defaultValue
	flagValue := &stringMetavarValue{
		value:    &value,
		typeName: typeName,
	}
	if shorthand != "" {
		fs.VarP(flagValue, name, shorthand, usage)
		return
	}
	fs.Var(flagValue, name, usage)
}

func printPrompt(cmd *cobra.Command, prompt string) {
	writeLogf(cmd, "%s", prompt)
}

func confirmAction(cmd *cobra.Command, prompt string) (bool, error) {
	opts := runtimeOptionsFrom(cmd)
	if opts.Yes {
		return true, nil
	}
	if opts.DryRun {
		return true, nil
	}
	if opts.NoInput {
		return false, noPermError(fmt.Errorf("confirmation required with --no-input; rerun with --yes to continue non-interactively"))
	}
	if !terminalInputChecker() {
		return false, noPermError(fmt.Errorf("confirmation required in non-interactive mode; rerun with --yes"))
	}

	printPrompt(cmd, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return false, noPermError(err)
		}
		return false, softwareError(err)
	}

	answer := strings.TrimSpace(line)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func canPromptForInput(cmd *cobra.Command) bool {
	opts := runtimeOptionsFrom(cmd)
	return !opts.NoInput && terminalInputChecker()
}

func readPromptedValue(cmd *cobra.Command, prompt string) (string, error) {
	printPrompt(cmd, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", softwareError(err)
	}
	return strings.TrimSpace(line), nil
}

func resolveSingleInputOrPrompt(cmd *cobra.Command, args []string, usage, prompt string, missingErr error) (string, error) {
	value, err := commandSingleArg(args, usage)
	if err == nil {
		return value, nil
	}
	if len(args) > 1 {
		return "", err
	}
	if !canPromptForInput(cmd) {
		if missingErr != nil && isMissingArgumentError(err) {
			return "", missingErr
		}
		return "", err
	}

	value, promptErr := readPromptedValue(cmd, prompt)
	if promptErr != nil {
		return "", promptErr
	}
	if strings.TrimSpace(value) == "" {
		if missingErr != nil {
			return "", missingErr
		}
		return "", usageError(fmt.Errorf("missing required argument %s", usage))
	}
	return value, nil
}

func isMissingArgumentError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "missing required argument")
}

func missingInputErrorForAdd() error {
	return usageError(fmt.Errorf("missing required input; pass <id|Path/To.AppImage> or one of --url or --github"))
}

func missingInputErrorForInfo() error {
	return usageError(fmt.Errorf("missing required input; pass <id|Path/To.AppImage> or --github"))
}

func missingInputErrorForManagedID() error {
	return usageError(fmt.Errorf("missing required input; pass <id> as a positional argument"))
}

func missingInputErrorForRemove() error {
	return missingInputErrorForManagedID()
}

const (
	bashCompletionRelativePath = "share/bash-completion/completions/aim"
	zshCompletionRelativePath  = "share/zsh/site-functions/_aim"
	fishCompletionRelativePath = "share/fish/vendor_completions.d/aim.fish"
)

func renderBashCompletion(cmd *cobra.Command) (string, error) {
	var buf bytes.Buffer
	if err := cmd.GenBashCompletionV2(&buf, true); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderZshCompletion(cmd *cobra.Command) (string, error) {
	var buf bytes.Buffer
	if err := cmd.GenZshCompletion(&buf); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderFishCompletion(cmd *cobra.Command) (string, error) {
	var buf bytes.Buffer
	if err := cmd.GenFishCompletion(&buf, true); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func writeCompletionFiles(cmd *cobra.Command, baseDir string) error {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return fmt.Errorf("completion output directory cannot be empty")
	}

	files := []struct {
		relativePath string
		render       func(*cobra.Command) (string, error)
	}{
		{relativePath: bashCompletionRelativePath, render: renderBashCompletion},
		{relativePath: zshCompletionRelativePath, render: renderZshCompletion},
		{relativePath: fishCompletionRelativePath, render: renderFishCompletion},
	}

	for _, file := range files {
		content, err := file.render(cmd)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(baseDir, file.relativePath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
			return err
		}
	}

	return nil
}

type managedAppCompletion struct {
	id   string
	name string
}

func completeManagedAppIDs(toComplete string, directive cobra.ShellCompDirective) ([]cobra.Completion, cobra.ShellCompDirective) {
	apps, err := getAllManagedApps()
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
