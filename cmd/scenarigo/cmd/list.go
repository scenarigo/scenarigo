package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scenarigo/scenarigo"
	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:           "list",
	Short:         "list the test scenario files",
	Long:          "Lists the test scenario files as relative paths from the current directory.",
	RunE:          list,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func list(cmd *cobra.Command, args []string) error {
	opts := []func(*scenarigo.Runner) error{}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg != nil {
		if len(args) > 0 {
			cfg.Scenarios = nil
		}
		opts = append(opts, scenarigo.WithConfig(cfg))
	}
	if len(args) > 0 {
		opts = append(opts, scenarigo.WithScenarios(args...))
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	r, err := scenarigo.NewRunner(opts...)
	if err != nil {
		return err
	}

	for _, file := range r.ScenarioFiles() {
		rel, err := filepath.Rel(wd, file)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), rel)
	}
	return nil
}
