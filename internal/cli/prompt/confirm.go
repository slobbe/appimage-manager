package prompt

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

func ConfirmYesNo(ctx context.Context, in io.Reader, out io.Writer, question string, autoConfirm bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if autoConfirm {
		return true, nil
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
