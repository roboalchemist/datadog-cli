// Command gendocs generates documentation for datadog-cli.
//
// Usage:
//
//	go run ./cmd/gendocs man   — generate man pages to man/man1/
//	go run ./cmd/gendocs docs  — generate markdown reference to docs/cli/
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra/doc"

	"gitea.roboalch.com/roboalchemist/datadog-cli/cmd"
)

func main() {
	mode := "docs"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	switch mode {
	case "man":
		generateMan()
	case "docs":
		generateMarkdown()
	default:
		log.Fatalf("unknown mode %q: use 'man' or 'docs'", mode)
	}
}

func generateMan() {
	outDir := filepath.Join("man", "man1")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("creating man directory %s: %v", outDir, err)
	}

	header := &doc.GenManHeader{
		Title:   "DATADOG-CLI",
		Section: "1",
		Source:  "datadog-cli",
		Manual:  "datadog-cli Manual",
	}

	if err := doc.GenManTree(cmd.RootCmd, header, outDir); err != nil {
		log.Fatalf("generating man pages: %v", err)
	}

	log.Printf("Man pages written to %s/", outDir)
}

func generateMarkdown() {
	outDir := filepath.Join("docs", "cli")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("creating docs directory %s: %v", outDir, err)
	}

	if err := doc.GenMarkdownTree(cmd.RootCmd, outDir); err != nil {
		log.Fatalf("generating markdown docs: %v", err)
	}

	log.Printf("Markdown docs written to %s/", outDir)
}
