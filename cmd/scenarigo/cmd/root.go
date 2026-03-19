package cmd

import (
	"context"
	"fmt"

	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/spf13/cobra"
)

const appName = "scenarigo"

func init() {
	rootCmd.PersistentFlags().StringVarP(&config.ConfigPath, "config", "c", "", `specify the configuration file path (default: scenarigo.yaml, use '-' for stdin)`)
	rootCmd.PersistentFlags().StringVarP(&config.Root, "root", "", "", `specify root directory (default value is the directory of configuration file)`)
}

var rootCmd = &cobra.Command{
	Use:   appName,
	Short: fmt.Sprintf("%s is a scenario-based API testing tool.", appName),
}

// Execute executes the root command.
func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}
