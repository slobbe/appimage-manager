package prompt

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

func ConfirmYesNo(ctx context.Context, in io.Reader, out io.Writer, question string, autoConfirm bool) (bool, error) {
	if autoConfirm {
		return true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	fmt.Fprint(out, question)

	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	fmt.Fprintln(out)
	if err != nil && len(answer) == 0 {
		return false, err
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}
