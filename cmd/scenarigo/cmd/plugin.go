package cmd

import (
	sub "github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/plugin"
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:           "plugin",
	Short:         "provide operations for plugins",
	Long:          "Provides operations for plugins.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	for _, c := range sub.Commands() {
		pluginCmd.AddCommand(c)
	}
	rootCmd.AddCommand(pluginCmd)
}
