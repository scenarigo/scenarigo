package cmd

import (
	"github.com/scenarigo/scenarigo/internal/lsp"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(lspCmd)
}

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "start the LSP server",
	Long:  "Start the Language Server Protocol server for scenarigo YAML files (config and test scenarios).",
	RunE: func(cmd *cobra.Command, args []string) error {
		server := lsp.NewServer()
		return server.Run()
	},
}
