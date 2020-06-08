package scenarigo

import (
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/schema"

	// register default protocols
	_ "github.com/zoncoen/scenarigo/protocol/grpc"
	_ "github.com/zoncoen/scenarigo/protocol/http"
)

// Runner represents a test runner.
type Runner struct {
	pluginDir               *string
	scenarioFiles           []string
	isEnabledOptionsFromEnv bool
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
func WithOptionsFromEnv(isEnv bool) func(*Runner) error {
	return func(r *Runner) error {
		r.isEnabledOptionsFromEnv = isEnv
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

// Run runs all tests.
func (r *Runner) Run(ctx *context.Context) {
	if r.pluginDir != nil {
		ctx = ctx.WithPluginDir(*r.pluginDir)
	}
	if r.isEnabledOptionsFromEnv {
		ctx = ctx.WithEnabledOptionsFromEnv()
	}
	for _, f := range r.scenarioFiles {
		ctx.Run(f, func(ctx *context.Context) {
			scns, err := schema.LoadScenarios(f)
			if err != nil {
				ctx.Reporter().Fatalf("failed to load scenarios: %s", err)
			}
			for idx, scn := range scns {
				scn := scn
				yml, err := context.NewYAML(f, idx)
				if err != nil {
					log.Fatal(err)
				}
				ctx = ctx.WithYAML(yml)
				ctx.Run(scn.Title, func(ctx *context.Context) {
					ctx.Reporter().Parallel()
					_ = runScenario(ctx, scn)
				})
			}
		})
	}
}
