package cmd

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/zoncoen/scenarigo"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/reporter"
)

var listCmd = &cobra.Command{
	Use:           "list",
	Short:         "list test scenarios",
	Args:          cobra.MinimumNArgs(1),
	RunE:          list,
	SilenceErrors: true,
	SilenceUsage:  true,
}

var (
	verboseList bool
	fileList    bool
)

func init() {
	listCmd.Flags().BoolVarP(&verboseList, "verbose", "v", false, "show steps of scenario")
	listCmd.Flags().BoolVarP(&fileList, "file", "f", false, "show file names only")
	rootCmd.AddCommand(listCmd)
}

func sortedScenarioNames(scenarioMap map[string][]string) []string {
	scenarioNames := []string{}
	for scenarioName := range scenarioMap {
		scenarioNames = append(scenarioNames, scenarioName)
	}
	sort.Strings(scenarioNames)
	return scenarioNames
}

func list(cmd *cobra.Command, args []string) error {
	opts := []func(*scenarigo.Runner) error{}
	for _, arg := range args {
		opts = append(opts, scenarigo.WithScenarios(arg))
	}
	r, err := scenarigo.NewRunner(opts...)
	if err != nil {
		return err
	}

	var b bytes.Buffer
	reporterOpts := []reporter.Option{reporter.WithWriter(&b)}
	reporter.Run(func(rptr reporter.Reporter) {
		for _, file := range r.ScenarioFiles() {
			scenarioMap, err := r.ScenarioMap(context.New(rptr), file)
			if err != nil {
				continue
			}
			if fileList {
				fmt.Println(file)
				continue
			}
			for _, name := range sortedScenarioNames(scenarioMap) {
				fmt.Println(name)
				if !verboseList {
					continue
				}
				for _, step := range scenarioMap[name] {
					fmt.Println(step)
				}
			}
		}
	}, reporterOpts...)
	return nil
}
