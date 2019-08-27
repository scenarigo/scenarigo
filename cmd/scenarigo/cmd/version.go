package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zoncoen/scenarigo/version"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: fmt.Sprintf("print %s version", appName),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s version %s\n", appName, version.String())
	},
}
