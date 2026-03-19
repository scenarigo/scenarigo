package cmd

import (
	"github.com/spf13/cobra"

	sub "github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
)

var configCmd = &cobra.Command{
	Use:           "config",
	Short:         "manage the scenarigo configuration file",
	Long:          "Manages the scenarigo configuration file.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	for _, c := range sub.Commands() {
		configCmd.AddCommand(c)
	}
	rootCmd.AddCommand(configCmd)
}
