package output

import (
	"fmt"
	"io"

	"github.com/slobbe/appimage-manager/internal/app"
)

func WriteWarnings(w io.Writer, warnings []app.OperationWarning) error {
	for _, warning := range warnings {
		if warning.AppID == "" {
			if _, err := fmt.Fprintf(w, "Warning [%s]: %s\n", warning.Kind, warning.Error); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(w, "Warning [%s, %s]: %s\n", warning.AppID, warning.Kind, warning.Error); err != nil {
			return err
		}
	}
	return nil
}
