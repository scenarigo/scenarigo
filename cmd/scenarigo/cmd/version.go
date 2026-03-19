package cmd

import (
	"fmt"
	"runtime"

	"github.com/scenarigo/scenarigo/version"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: fmt.Sprintf("print %s version", appName),
	Long:  fmt.Sprintf("Prints %s version.", appName),
	Run:   printVersion,
}

func printVersion(cmd *cobra.Command, args []string) {
	fmt.Fprintf(cmd.OutOrStdout(), "%s version %s %s %s/%s\n", appName, version.String(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
