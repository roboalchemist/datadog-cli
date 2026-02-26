package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage the datadog-cli Claude Code skill",
	Long:  "Commands for working with the datadog-cli Claude Code skill.",
}

var skillPrintCmd = &cobra.Command{
	Use:   "print",
	Short: "Print the skill markdown to stdout",
	RunE: func(cmd *cobra.Command, args []string) error {
		if skillMD == "" {
			return fmt.Errorf("skill documentation not available")
		}
		fmt.Print(skillMD)
		return nil
	},
}

var skillAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Install the skill to ~/.claude/skills/datadog-cli/",
	Long:  "Copies the skill files to ~/.claude/skills/datadog-cli/ for use with Claude Code.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if skillFS == nil {
			return fmt.Errorf("skill filesystem not available")
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}

		destDir := filepath.Join(home, ".claude", "skills", "datadog-cli")
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("creating skill directory: %w", err)
		}

		// Walk and copy skill files
		err = fs.WalkDir(skillFS, "skill", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Compute destination path (strip "skill/" prefix)
			rel, err := filepath.Rel("skill", path)
			if err != nil {
				return err
			}
			dest := filepath.Join(destDir, rel)

			if d.IsDir() {
				return os.MkdirAll(dest, 0755)
			}

			data, err := fs.ReadFile(skillFS, path)
			if err != nil {
				return fmt.Errorf("reading %s: %w", path, err)
			}

			if err := os.WriteFile(dest, data, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", dest, err)
			}

			fmt.Printf("Installed: %s\n", dest)
			return nil
		})
		if err != nil {
			return fmt.Errorf("copying skill files: %w", err)
		}

		fmt.Printf("\nSkill installed to %s\n", destDir)
		return nil
	},
}

func init() {
	skillCmd.AddCommand(skillPrintCmd)
	skillCmd.AddCommand(skillAddCmd)
	rootCmd.AddCommand(skillCmd)
}
