package cmd

import (
	"context"
	"errors"

	"github.com/scenarigo/scenarigo/internal/lsp"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(lspCmd)
}

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "start the LSP server",
	Long: `Start the Language Server Protocol server for scenarigo YAML files (config and test scenarios).

The server talks to the editor over stdin and stdout. It reads scenarigo.yaml
from the workspace root sent by the editor; the --config and --root flags are
ignored.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		server := lsp.NewServer()
		if err := server.Run(cmd.Context()); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	},
}
