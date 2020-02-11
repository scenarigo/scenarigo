package cmd

import (
	"context"
	"os"

	"github.com/go-language-server/jsonrpc2"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/zoncoen/scenarigo/lsp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var lspFlags {}

type lspFlags struct {
	port  int
	stdio bool
}

func init() {
	lspCmd.Flags().IntVarP()
	rootCmd.AddCommand(lspCmd)
}

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "run the server which implements the Language Server Protocol",
	RunE: func(cmd *cobra.Command, args []string) error {
		fpath, err := homedir.Expand("~/.scenarigo/log/lsp.log")
		if err != nil {
			return err
		}
		f, err := os.OpenFile(fpath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return errors.Wrap(err, "failed to open log file")
		}

		logger := zap.New(zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewDevelopmentEncoderConfig()),
			zapcore.AddSync(f),
			zap.DebugLevel,
		))

		stream := jsonrpc2.NewStream(os.Stdin, os.Stdout)
		srv, err := lsp.NewServer(context.Background(), stream, lsp.WithLogger(logger))
		if err != nil {
			return err
		}
		return srv.Run(context.Background())
	},
}
