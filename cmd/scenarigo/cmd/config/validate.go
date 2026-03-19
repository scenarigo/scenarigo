package config

import (
	"errors"

	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:           "validate",
	Short:         "validate the configuration file",
	Long:          "Validates the configuration file.",
	Args:          cobra.ExactArgs(0),
	RunE:          validate,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func validate(cmd *cobra.Command, args []string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	if cfg == nil {
		return errors.New("config file not found")
	}
	return nil
}
