package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Show documentation",
	Long:  "Show the README documentation for datadog-cli.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if readmeContents == "" {
			return fmt.Errorf("documentation not available")
		}
		fmt.Print(readmeContents)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(docsCmd)
}
