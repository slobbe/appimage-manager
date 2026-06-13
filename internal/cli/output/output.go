package output

import (
	"encoding/json"
	"io"
)

func Write(w io.Writer, asJSON bool, jsonValue any, writeText func(io.Writer) error) error {
	if asJSON {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(jsonValue)
	}

	return writeText(w)
}
