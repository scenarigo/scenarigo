package scenarigo

import (
	gocontext "context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/scenarigo/scenarigo/color"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/filepathutil"
	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/protocol/grpc"
	"github.com/scenarigo/scenarigo/protocol/http"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
)

func init() {
	http.Register()
	grpc.Register()
}

// Runner represents a test runner.
type Runner struct {
	vars            map[string]any
	secrets         map[string]any
	pluginDir       *string
	plugins         schema.OrderedMap[string, schema.PluginConfig]
	protocols       schema.ProtocolOptions
	scenarioFiles   []string
	scenarioReaders []io.Reader
	colorConfig     *color.Config
	rootDir         string
	inputConfig     schema.InputConfig
	reportConfig    schema.ReportConfig
	configNode      ast.Node
}

// NewRunner returns a new test runner.
func NewRunner(opts ...func(*Runner) error) (*Runner, error) {
	r := &Runner{} //nolint:exhaustruct
	r.colorConfig = color.New()
	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}
	if r.rootDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		r.rootDir = wd
	}
	return r, nil
}

// WithConfig returns a option which sets configuration.
func WithConfig(config *schema.Config) func(*Runner) error {
	return func(r *Runner) error {
		if config == nil {
			return nil
		}

		r.vars = config.Vars
		r.secrets = config.Secrets

		r.rootDir = config.Root
		scenarios := make([]string, len(config.Scenarios))
		for i, s := range config.Scenarios {
			scenarios[i] = filepath.Join(r.rootDir, s)
		}

		var opts []func(r *Runner) error
		opts = append(opts, WithScenarios(scenarios...))
		if config.PluginDirectory != "" {
			opts = append(opts, WithPluginDir(filepath.Join(r.rootDir, config.PluginDirectory)))
		}
		for _, opt := range opts {
			if err := opt(r); err != nil {
				return err
			}
		}
		r.plugins = config.Plugins
		r.protocols = config.Protocols
		if config.Output.Colored != nil {
			r.colorConfig.SetEnabled(*config.Output.Colored)
		}
		r.inputConfig = config.Input
		r.reportConfig = config.Output.Report
		r.configNode = config.Node
		return nil
	}
}

// WithScenarios returns a option which finds and sets test scenario files.
func WithScenarios(paths ...string) func(*Runner) error {
	return func(r *Runner) error {
		for i, path := range paths {
			abs, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("failed to find test scenarios: %w", err)
			}
			paths[i] = abs
		}
		files, err := getAllFiles(paths...)
		if err != nil {
			return fmt.Errorf("failed to find test scenarios: %w", err)
		}
		r.scenarioFiles = files
		return nil
	}
}

// WithPluginDir returns a option which sets plugin root directory.
func WithPluginDir(path string) func(*Runner) error {
	return func(r *Runner) error {
		abs, err := filepath.Abs(path)
		if err != nil {
			return errors.Wrapf(err, `failed to set plugin directory "%s"`, path)
		}
		r.pluginDir = &abs
		return nil
	}
}

// WithScenariosFromReader returns a option which sets readers to read scenario contents.
func WithScenariosFromReader(readers ...io.Reader) func(*Runner) error {
	return func(r *Runner) error {
		r.scenarioReaders = readers
		return nil
	}
}

// WithOptionsFromEnv returns a option which sets flag whether accepts configuration from ENV.
// Currently Available ENV variables are the following.
//   - SCENARIGO_COLOR=(1|true|TRUE)
func WithOptionsFromEnv(isEnv bool) func(*Runner) error {
	return func(r *Runner) error {
		if isEnv {
			r.setOptionsFromEnv()
		}
		return nil
	}
}

var yamlPattern = regexp.MustCompile(`(?i)\.ya?ml$`)

func looksLikeYAML(path string) bool {
	return yamlPattern.MatchString(path)
}

func getAllFiles(paths ...string) ([]string, error) {
	files := []string{}
	for _, path := range paths {
		if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if looksLikeYAML(path) {
				files = append(files, path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func (r *Runner) setOptionsFromEnv() {
	// do nothing. colorConfig is already reflected by SCENARIGO_COLOR.
}

// ScenarioFiles returns all scenario file paths.
func (r *Runner) ScenarioFiles() []string {
	return r.scenarioFiles
}

// Run runs all tests.
func (r *Runner) Run(ctx *context.Context) {
	// setup context
	ctx = ctx.WithColorConfig(r.colorConfig)
	if r.vars != nil {
		vars, err := ctx.ExecuteTemplate(r.vars)
		if err != nil {
			ctx.Reporter().Fatalf(
				"invalid vars: %s",
				errors.WithNodeAndColored(
					errors.WithPath(err, "vars"),
					r.configNode,
					r.colorConfig.IsEnabled(),
				),
			)
		}
		ctx = ctx.WithVars(vars)
	}
	if r.secrets != nil {
		secrets, err := ctx.ExecuteTemplate(r.secrets)
		if err != nil {
			ctx.Reporter().Fatalf(
				"invalid secrets: %s",
				errors.WithNodeAndColored(
					errors.WithPath(err, "secrets"),
					r.configNode,
					r.colorConfig.IsEnabled(),
				),
			)
		}
		ctx = ctx.WithSecrets(secrets)
	}
	if r.pluginDir != nil {
		ctx = ctx.WithPluginDir(*r.pluginDir)
	}

	// open plugins
	pluginDir := r.rootDir
	if dir := ctx.PluginDir(); dir != "" {
		pluginDir = dir
	}
	var setups setupFuncList
	for _, item := range r.plugins.ToSlice() {
		p, err := plugin.Open(filepath.Join(pluginDir, item.Key))
		if err != nil {
			setups = append(setups, setupFunc{
				name: item.Key,
				f: func(ctx *context.Context) (*context.Context, func(*context.Context)) {
					ctx.Reporter().Fatalf("failed to open plugin: %s", err)
					return nil, nil
				},
			})
			continue
		}
		if setup := p.GetSetup(); setup != nil {
			setups = append(setups, setupFunc{
				name: item.Key,
				f:    setup,
			})
		}
	}
	ctx, teardown := setups.setup(ctx)
	if ctx.Reporter().Failed() {
		teardown(ctx)
		return
	}

	if err := r.protocols.Set(); err != nil {
		ctx.Reporter().Error(err)
		teardown(ctx)
		return
	}

	opts := []schema.LoadOption{
		schema.WithInputConfig(r.rootDir, r.inputConfig),
		schema.WithColorConfig(r.colorConfig),
	}

FILE_LOOP:
	for _, f := range r.scenarioFiles {
		testName, err := filepath.Rel(r.rootDir, f)
		if err != nil {
			testName = f
		}
		for _, exclude := range r.inputConfig.Excludes {
			if exclude.MatchString(testName) {
				continue FILE_LOOP
			}
		}
		ctx.Run(testName, func(ctx *context.Context) {
			ctx.Reporter().Parallel()
			scns, err := schema.LoadScenarios(f, opts...)
			if err != nil {
				ctx.Reporter().Fatalf("failed to load scenarios: %s", err)
			}
			for _, scn := range scns {
				ctx = ctx.WithNode(scn.Node)
				ctx.Run(scn.Title, func(ctx *context.Context) {
					ctx.Reporter().Parallel()
					_ = RunScenario(ctx, scn)
				})
			}
		})
	}
	for i, reader := range r.scenarioReaders {
		ctx.Run(fmt.Sprint(i), func(ctx *context.Context) {
			ctx.Reporter().Parallel()
			scns, err := schema.LoadScenariosFromReader(reader, schema.WithColorConfig(ctx.ColorConfig()))
			if err != nil {
				ctx.Reporter().Fatalf("failed to load scenarios: %s", err)
			}
			for _, scn := range scns {
				ctx = ctx.WithNode(scn.Node)
				ctx.Run(scn.Title, func(ctx *context.Context) {
					ctx.Reporter().Parallel()
					_ = RunScenario(ctx, scn)
				})
			}
		})
	}
	teardown(ctx)
}

// CreateTestReport creates test reports.
func (r *Runner) CreateTestReport(rptr reporter.Reporter) error {
	if r.reportConfig.JSON.Filename == "" && r.reportConfig.JUnit.Filename == "" {
		return nil
	}

	report, err := reporter.GenerateTestReport(rptr)
	if err != nil {
		return fmt.Errorf("failed to generate test report: %w", err)
	}
	if r.reportConfig.JSON.Filename != "" {
		f, err := os.Create(filepathutil.From(r.rootDir, r.reportConfig.JSON.Filename))
		if err != nil {
			return fmt.Errorf("failed to write JSON test report: %w", err)
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return fmt.Errorf("failed to write JSON test report: %w", err)
		}
	}
	if r.reportConfig.JUnit.Filename != "" {
		f, err := os.Create(filepathutil.From(r.rootDir, r.reportConfig.JUnit.Filename))
		if err != nil {
			return fmt.Errorf("failed to write JUnit test report: %w", err)
		}
		defer f.Close()
		enc := xml.NewEncoder(f)
		enc.Indent("", "  ")
		if err := enc.Encode(report); err != nil {
			return fmt.Errorf("failed to write JUnit test report: %w", err)
		}
	}
	return nil
}

// Dump dumps all test scenarios.
func (r *Runner) Dump(ctx gocontext.Context, w io.Writer) error {
	enc := yaml.NewEncoder(w)
	defer enc.Close()
	opts := []schema.LoadOption{
		schema.WithInputConfig(r.rootDir, r.inputConfig),
		schema.WithColorConfig(r.colorConfig),
	}
FILE_LOOP:
	for _, f := range r.scenarioFiles {
		testName, err := filepath.Rel(r.rootDir, f)
		if err != nil {
			testName = f
		}
		for _, exclude := range r.inputConfig.Excludes {
			if exclude.MatchString(testName) {
				continue FILE_LOOP
			}
		}
		scns, err := schema.LoadScenarios(f, opts...)
		if err != nil {
			return fmt.Errorf("failed to load scenarios: %w", err)
		}
		for _, scn := range scns {
			if err := enc.EncodeContext(ctx, scn); err != nil {
				return fmt.Errorf("failed to encode scenarios: %w", err)
			}
		}
	}
	return nil
}
