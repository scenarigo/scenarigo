package scenarigo

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/schema"

	// register default protocols
	_ "github.com/zoncoen/scenarigo/protocol/grpc"
	_ "github.com/zoncoen/scenarigo/protocol/http"
)

// Runner represents a test runner.
type Runner struct {
	pluginDir     *string
	scenarioFiles []string
	enabledColor  bool
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

// NewRunner returns a new test runner.
func NewRunner(opts ...func(*Runner) error) (*Runner, error) {
	r := &Runner{}
	r.enabledColor = isatty.IsTerminal(os.Stdout.Fd())
	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// WithScenarios returns a option which finds and sets test scenario files.
func WithScenarios(paths ...string) func(*Runner) error {
	return func(r *Runner) error {
		files, err := getAllFiles(paths...)
		if err != nil {
			return err
		}
		r.scenarioFiles = files
		return nil
	}
}

// WithOptionsFromEnv returns a option which sets flag whether accepts configuration from ENV.
// Currently Available ENV variables are the following.
// - SCENARIGO_COLOR=(1|true|TRUE)
// nolint:stylecheck
func WithOptionsFromEnv(isEnv bool) func(*Runner) error {
	return func(r *Runner) error {
		if isEnv {
			r.setOptionsFromEnv()
		}
		return nil
	}
}

var (
	yamlPattern = regexp.MustCompile(`(?i)\.ya?ml$`)
)

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

const (
	envScenarigoColor = "SCENARIGO_COLOR"
)

func (r *Runner) setOptionsFromEnv() {
	r.setEnabledColor(os.Getenv(envScenarigoColor))
}

func (r *Runner) setEnabledColor(envColor string) {
	if envColor == "" {
		return
	}
	result, _ := strconv.ParseBool(envColor)
	r.enabledColor = result
}

func newYAMLNode(path string, docIdx int) (ast.Node, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, err := parser.ParseBytes(bytes, 0)
	if err != nil {
		return nil, err
	}
	return file.Docs[docIdx].Body, nil
}

// ScenariosFiles returns all scenario file paths.
func (r *Runner) ScenarioFiles() []string {
	return r.scenarioFiles
}

// Scenarios returns map for all scenarios ( key is scenario name and value is steps of scenario ).
func (r *Runner) ScenarioMap(ctx *context.Context, path string) (map[string][]string, error) {
	scns, err := schema.LoadScenarios(path)
	if err != nil {
		return nil, err
	}

	scenarioMap := map[string][]string{}
	ctx.Run(path, func(ctx *context.Context) {
		for _, scn := range scns {
			ctx.Run(scn.Title, func(ctx *context.Context) {
				scenarioName := ctx.Reporter().Name()
				scenarioMap[scenarioName] = []string{}
				for _, step := range scn.Steps {
					ctx.Run(step.Title, func(ctx *context.Context) {
						stepName := ctx.Reporter().Name()
						scenarioMap[scenarioName] = append(scenarioMap[scenarioName], stepName)
					})
				}
			})
		}
	})
	return scenarioMap, nil
}

// Run runs all tests.
func (r *Runner) Run(ctx *context.Context) {
	if r.pluginDir != nil {
		ctx = ctx.WithPluginDir(*r.pluginDir)
	}
	ctx = ctx.WithEnabledColor(r.enabledColor)
	for _, f := range r.scenarioFiles {
		ctx.Run(f, func(ctx *context.Context) {
			scns, err := schema.LoadScenarios(f)
			if err != nil {
				ctx.Reporter().Fatalf("failed to load scenarios: %s", err)
			}
			for idx, scn := range scns {
				scn := scn
				node, err := newYAMLNode(f, idx)
				if err != nil {
					ctx.Reporter().Fatalf("failed to create ast: %s", err)
				}
				ctx = ctx.WithNode(node)
				ctx.Run(scn.Title, func(ctx *context.Context) {
					ctx.Reporter().Parallel()
					_ = runScenario(ctx, scn)
				})
			}
		})
	}
}
