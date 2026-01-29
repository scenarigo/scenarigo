//go:build !race
// +build !race

package scenarigo

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"

	"github.com/scenarigo/scenarigo"
	scenarigocontext "github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/plugin/wasm/image"
	"github.com/scenarigo/scenarigo/internal/testutil"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
)

func TestOCIPlugin(t *testing.T) {
	// 1. Start in-memory registry
	reg := registry.New()
	server := httptest.NewServer(reg)
	defer server.Close()

	// Get the registry address (without http:// prefix)
	registryAddr := strings.TrimPrefix(server.URL, "http://")

	// 2. Load existing WASM plugin and push to registry
	wasmPath := "testdata/gen/plugins/simple.wasm"
	img, err := image.Build(wasmPath)
	if err != nil {
		t.Fatalf("failed to build image: %v", err)
	}

	ref := registryAddr + "/test/simple:v1"
	ctx := context.Background()
	_, err = image.Push(ctx, img, ref, image.WithInsecure(true))
	if err != nil {
		t.Fatalf("failed to push image: %v", err)
	}

	// 3. Start HTTP echo server for testing plugin values
	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer echoServer.Close()
	echoAddr := echoServer.Listener.Addr().String()
	t.Setenv("TEST_HTTP_ADDR", echoAddr)

	// 4. Create temp directory with config and scenario
	tmpDir := t.TempDir()

	// Create plugin directory
	pluginDir := filepath.Join(tmpDir, "gen", "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("failed to create plugin directory: %v", err)
	}

	// Create scenarios directory
	scenarioDir := filepath.Join(tmpDir, "scenarios")
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		t.Fatalf("failed to create scenarios directory: %v", err)
	}

	// Write test scenario that uses the plugin variables and functions
	scenarioContent := `title: OCI plugin test
plugins:
  simple: simple.wasm
steps:
- title: check plugin variable
  protocol: http
  request:
    method: POST
    url: "http://{{env.TEST_HTTP_ADDR}}/echo"
    body:
      value: "{{plugins.simple.String}}"
  expect:
    code: 200
    body:
      value: "string"
- title: check plugin function
  protocol: http
  request:
    method: POST
    url: "http://{{env.TEST_HTTP_ADDR}}/echo"
    body:
      value: "{{plugins.simple.Function()}}"
  expect:
    code: 200
    body:
      value: "function"
`
	scenarioPath := filepath.Join(scenarioDir, "oci-plugin.yaml")
	if err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0o644); err != nil {
		t.Fatalf("failed to write scenario file: %v", err)
	}

	// 5. Pull the OCI plugin (simulating what plugin build does)
	pulledImg, err := image.Pull(ctx, ref, image.WithPullInsecure(true))
	if err != nil {
		t.Fatalf("failed to pull image: %v", err)
	}

	wasmBytes, err := image.ExtractWASM(pulledImg)
	if err != nil {
		t.Fatalf("failed to extract WASM: %v", err)
	}

	pluginPath := filepath.Join(pluginDir, "simple.wasm")
	if err := os.WriteFile(pluginPath, wasmBytes, 0o644); err != nil {
		t.Fatalf("failed to write plugin file: %v", err)
	}

	// 6. Create config for running the scenario with pulled plugin
	// Use absolute path for PluginDirectory (same pattern as existing e2e tests)
	runConfig := &schema.Config{
		PluginDirectory: pluginDir,
		Plugins:         schema.NewOrderedMap[string, schema.PluginConfig](),
	}
	runConfig.Plugins.Set("simple.wasm", schema.PluginConfig{})

	// 7. Run scenario with pulled plugin
	r, err := scenarigo.NewRunner(
		scenarigo.WithConfig(runConfig),
		scenarigo.WithScenarios(scenarioPath),
	)
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}

	var b bytes.Buffer
	ok := reporter.Run(func(rptr reporter.Reporter) {
		r.Run(scenarigocontext.New(rptr))
	}, reporter.WithWriter(&b))

	if !ok {
		t.Errorf("scenario failed: %s", testutil.ReplaceOutput(b.String()))
	}
}
