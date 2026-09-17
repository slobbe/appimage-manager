package prompt

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

type ConfirmOptions struct {
	AutoConfirm    bool
	NonInteractive bool
}

func (o ConfirmOptions) RequiresInput() bool {
	return !o.AutoConfirm && !o.NonInteractive
}

func ConfirmYesNo(ctx context.Context, in io.Reader, out io.Writer, question string, options ConfirmOptions) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if options.AutoConfirm {
		return true, nil
	}
	if options.NonInteractive {
		return false, fmt.Errorf("confirmation required in non-interactive mode; rerun with --yes")
	}

	if _, err := fmt.Fprint(out, question); err != nil {
		return false, err
	}

	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if _, printErr := fmt.Fprintln(out); printErr != nil {
		return false, printErr
	}
	if err != nil && len(answer) == 0 {
		return false, err
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}
