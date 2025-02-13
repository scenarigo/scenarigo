package plugin

import (
	"os"
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/context"
)

func init() {
	// HACK: Enable to do "go test" against main packages of plugins.
	var isTest bool
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") {
			isTest = true
			break
		}
	}
	if isTest {
		newPlugin = &openedPlugin{} //nolint:exhaustruct
	}
}

// Setup calls the registered functions by RegisterSetup.
func Setup(t *testing.T) {
	t.Helper()

	setup := newPlugin.GetSetup()
	if setup == nil {
		return
	}

	ctx := context.FromT(t)
	ctx, teardown := setup(ctx)
	if teardown != nil {
		t.Cleanup(func() {
			teardown(ctx)
		})
	}
}

// SetupEachScenario calls the registered functions by RegisterSetupEachScenario.
func SetupEachScenario(t *testing.T) {
	t.Helper()

	setup := newPlugin.GetSetupEachScenario()
	if setup == nil {
		return
	}

	ctx := context.FromT(t)
	ctx, teardown := setup(ctx)
	if teardown != nil {
		t.Cleanup(func() {
			teardown(ctx)
		})
	}
}
