// Command gendocs generates markdown documentation for datadog-cli.
// Usage: go run ./cmd/gendocs [output-dir]
package main

import (
	"log"
	"os"
)

func main() {
	dir := "./docs"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("creating docs directory: %v", err)
	}

	// TODO: Generate cobra markdown docs for all commands.
	// This requires exporting the root cobra command from cmd/root.go.
	// Placeholder until cmd package exports RootCmd.
	log.Printf("Documentation would be written to %s/ (not yet implemented)", dir)
}
