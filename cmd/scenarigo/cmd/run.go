package cmd

import (
	"fmt"
	"runtime"

	"github.com/pkg/errors"
	"github.com/scenarigo/scenarigo"
	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/scenarigo/scenarigo/color"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
	"github.com/spf13/cobra"
)

// ErrTestFailed is the error returned when the test failed.
var ErrTestFailed = errors.New("test failed")

var (
	verbose     bool
	parallel    int
	reportJSON  string
	reportJUnit string
)

func init() {
	runCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	runCmd.Flags().IntVarP(&parallel, "parallel", "", 0, "specify the number of workers to run tests in parallel (the default value is the number of logical CPUs usable by the current process)")
	runCmd.Flags().StringVar(&reportJSON, "report-json", "", "output JSON test report to specified file")
	runCmd.Flags().StringVar(&reportJUnit, "report-junit", "", "output JUnit XML test report to specified file")
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run [file...]",
	Short: "run test scenarios",
	Long: `Runs test scenarios.

You can specify the file paths of the tests you want to run as arguments.
If you do not specify any arguments, it will execute the tests specified in the configuration file.`,
	RunE:          run,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func run(cmd *cobra.Command, args []string) error {
	opts := []func(*scenarigo.Runner) error{}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply CLI flags for report options (overrides config file settings)
	if cmd.Flags().Changed("report-json") {
		if cfg == nil {
			cfg = &schema.Config{}
		}
		cfg.Output.Report.JSON.Filename = reportJSON
	}
	if cmd.Flags().Changed("report-junit") {
		if cfg == nil {
			cfg = &schema.Config{}
		}
		cfg.Output.Report.JUnit.Filename = reportJUnit
	}

	if cfg != nil {
		if len(args) > 0 {
			cfg.Scenarios = nil
		}
		opts = append(opts, scenarigo.WithConfig(cfg))
	}
	if len(args) > 0 {
		opts = append(opts, scenarigo.WithScenarios(args...))
	}
	r, err := scenarigo.NewRunner(opts...)
	if err != nil {
		return err
	}

	reporterOpts := []reporter.Option{
		reporter.WithWriter(cmd.OutOrStdout()),
	}

	if (cfg != nil && cfg.Output.Verbose) || verbose {
		reporterOpts = append(reporterOpts, reporter.WithVerboseLog())
	}

	// Create color config and determine final setting based on schema config
	colorConfig := color.New()
	if cfg != nil && cfg.Output.Colored != nil {
		colorConfig.SetEnabled(*cfg.Output.Colored)
	}

	reporterOpts = append(reporterOpts, reporter.WithColorConfig(colorConfig))

	if cfg != nil && cfg.Output.Summary {
		reporterOpts = append(reporterOpts, reporter.WithTestSummary())
	}

	// flag優先
	parallelNum := runtime.NumCPU()
	if cfg != nil && cfg.Execution.Parallel > 0 {
		parallelNum = cfg.Execution.Parallel
	}
	if parallel > 0 {
		parallelNum = parallel
	}
	if parallelNum > 0 {
		reporterOpts = append(reporterOpts, reporter.WithMaxParallel(parallelNum))
	}

	var reportErr error
	success := reporter.Run(
		func(rptr reporter.Reporter) {
			ctx := context.New(rptr)
			r.Run(ctx)
			// Generate report in Cleanup to ensure all parallel tests complete
			rptr.Cleanup(func() {
				reportErr = r.CreateTestReport(rptr)
			})
		},
		reporterOpts...,
	)
	if reportErr != nil {
		return fmt.Errorf("failed to create test reports: %w", reportErr)
	}
	if !success {
		return ErrTestFailed
	}
	return nil
}
