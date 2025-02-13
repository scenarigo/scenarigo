package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/scenarigo/scenarigo"
	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
)

var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "dump test scenario files",
	Long: `Dumps test scenario files.
If ytt integration is enabled, displays the test scenario file contents after evaluating as ytt templates.`,
	RunE:          dump,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.AddCommand(dumpCmd)
}

func dump(cmd *cobra.Command, args []string) error {
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
	r, err := scenarigo.NewRunner(opts...)
	if err != nil {
		return err
	}
	return r.Dump(cmd.Context(), cmd.OutOrStdout())
}
