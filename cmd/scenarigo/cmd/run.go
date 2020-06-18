package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"github.com/zoncoen/scenarigo"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/reporter"
)

var (
	// ErrTestFailed is the error returned when the test failed.
	ErrTestFailed = errors.New("test failed")
)

var (
	verbose bool
	dryRun  bool
)

func init() {
	runCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print verbose log")
	runCmd.Flags().BoolVarP(&dryRun, "dry-run", "", false, "running without request invoking and assertion")
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:           "run",
	Short:         "run test scenarios",
	Args:          cobra.MinimumNArgs(1),
	RunE:          run,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func run(cmd *cobra.Command, args []string) error {
	opts := []func(*scenarigo.Runner) error{}
	for _, arg := range args {
		opts = append(opts, scenarigo.WithScenarios(arg))
	}
	opts = append(opts, scenarigo.WithDryRun(dryRun))
	r, err := scenarigo.NewRunner(opts...)
	if err != nil {
		return err
	}

	reporterOpts := []reporter.Option{
		reporter.WithWriter(os.Stdout),
	}
	if verbose {
		reporterOpts = append(reporterOpts, reporter.WithVerboseLog())
	}

	success := reporter.Run(
		func(rptr reporter.Reporter) {
			r.Run(context.New(rptr))
		},
		reporterOpts...,
	)
	if !success {
		return ErrTestFailed
	}
	return nil
}
