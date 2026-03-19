package config

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const DefaultConfigFileName = "scenarigo.yaml"

//go:embed default.scenarigo.yaml
var defaultConfig []byte

var initCmd = &cobra.Command{
	Use:           "init",
	Short:         "create a new configuration file",
	Long:          "Creates a new configuration file.",
	Args:          cobra.ExactArgs(0),
	RunE:          initRun,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func initRun(cmd *cobra.Command, args []string) error {
	f, err := os.OpenFile(DefaultConfigFileName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o666)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%s already exists", DefaultConfigFileName)
		}
		return err
	}
	defer f.Close()

	if _, err := f.Write(defaultConfig); err != nil {
		return err
	}
	return err
}
