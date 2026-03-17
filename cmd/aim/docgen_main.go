//go:build docgen

package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	outputPath := strings.TrimSpace(os.Getenv("AIM_MAN_OUTPUT"))
	if outputPath == "" {
		outputPath = filepath.Join("docs", "aim.1")
	}

	manPage, err := renderManPage(newRootCommand(version), 1)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(outputPath, []byte(manPage), 0o644); err != nil {
		log.Fatal(err)
	}
}
