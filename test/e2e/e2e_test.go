//go:build !race
// +build !race

package scenarigo

import (
	"bytes"
	gocontext "context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/sergi/go-diff/diffmatchpatch"
	"google.golang.org/grpc"

	"github.com/scenarigo/scenarigo"
	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/internal/testutil"
	"github.com/scenarigo/scenarigo/logger"
	"github.com/scenarigo/scenarigo/mock"
	"github.com/scenarigo/scenarigo/reporter"
	"github.com/scenarigo/scenarigo/schema"
	"github.com/scenarigo/scenarigo/testdata/gen/pb/test"
)

func TestE2E(t *testing.T) {
	dir := "testdata/testcases"
	infos, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	files := []string{}
	for _, info := range infos {
		if info.IsDir() {
			continue
		}
		if strings.HasSuffix(info.Name(), ".yaml") {
			files = append(files, filepath.Join(dir, info.Name()))
		}
	}

	teardown := startGRPCServer(t)
	defer teardown()

	pluginTypes := []string{".so", ".wasm"}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			f, err := os.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			var tc TestCase
			if err := yaml.NewDecoder(f).Decode(&tc); err != nil {
				t.Fatal(err)
			}

			for _, scenario := range tc.Scenarios {
				for _, pluginType := range pluginTypes {
					// Skip WASM tests for scenarios that don't use plugins
					testName := fmt.Sprintf("%s-%s", scenario.Filename, strings.TrimPrefix(pluginType, "."))
					t.Run(testName, func(t *testing.T) {
						if scenario.Mocks != "" {
							teardown := runMockServer(t, filepath.Join(dir, "mocks", scenario.Mocks))
							defer teardown(t)
						}

						config := &schema.Config{
							Vars: map[string]any{
								"global": `{{"aaa"}}`,
							},
							PluginDirectory: "testdata/gen/plugins",
							Plugins:         schema.NewOrderedMap[string, schema.PluginConfig](),
						}
						for _, p := range scenario.Plugins {
							// For WASM tests, adjust plugin names to match modified scenario content
							pluginName := p
							if pluginType == ".wasm" {
								// Replace .so extension with .wasm in plugin name
								if strings.HasSuffix(p, ".so") {
									pluginName = strings.TrimSuffix(p, ".so") + ".wasm"
								}
							}
							config.Plugins.Set(pluginName, schema.PluginConfig{})
						}

						var scenarioPath string
						if pluginType == ".so" {
							// Use original file path for .so tests to maintain backward compatibility
							scenarioPath = filepath.Join(dir, "scenarios", scenario.Filename)
						} else {
							// Use modified scenario for .wasm tests
							scenarioContent, err := loadAndModifyScenario(filepath.Join(dir, "scenarios", scenario.Filename), pluginType)
							if err != nil {
								t.Fatal(err)
							}

							tmpFile, err := os.CreateTemp("", "scenario-*.yaml")
							if err != nil {
								t.Fatal(err)
							}
							defer os.Remove(tmpFile.Name())

							if _, err := tmpFile.Write(scenarioContent); err != nil {
								t.Fatal(err)
							}
							tmpFile.Close()

							scenarioPath = tmpFile.Name()
						}

						r, err := scenarigo.NewRunner(
							scenarigo.WithConfig(config),
							scenarigo.WithScenarios(scenarioPath),
						)
						if err != nil {
							t.Fatal(err)
						}

						var b bytes.Buffer
						opts := []reporter.Option{reporter.WithWriter(&b)}
						if scenario.Verbose {
							opts = append(opts, reporter.WithVerboseLog())
						}
						ok := reporter.Run(func(rptr reporter.Reporter) {
							r.Run(context.New(rptr))
						}, opts...)
						if ok != scenario.Success {
							fmt.Println(b.String())
							t.Errorf("expect %t but got %t", scenario.Success, ok)
						}

						expectedStdout, err := loadAndModifyExpectedOutput(filepath.Join(dir, "stdout", scenario.Output.Stdout), pluginType)
						if err != nil {
							t.Fatal(err)
						}

						if got, expect := testutil.ReplaceOutput(b.String()), string(expectedStdout); got != expect {
							dmp := diffmatchpatch.New()
							diffs := dmp.DiffMain(expect, got, false)
							t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
						}
					})
				}
			}
		})
	}
}

type TestCase struct {
	Tilte     string         `yaml:"title"`
	Scenarios []TestScenario `yaml:"scenarios"`
}

type TestScenario struct {
	Filename string       `yaml:"filename"`
	Mocks    string       `yaml:"mocks"`
	Success  bool         `yaml:"success"`
	Output   ExpectOutput `yaml:"output"`
	Verbose  bool         `yaml:"verbose"`
	Plugins  []string     `yaml:"plugins"`
}

type ExpectOutput struct {
	Stdout string `yaml:"stdout"`
}

func loadAndModifyScenario(scenarioPath, pluginType string) ([]byte, error) {
	if pluginType == ".so" {
		// For .so plugins, return original content without modification
		return os.ReadFile(scenarioPath)
	}

	// Read and parse YAML manually to handle deprecated fields
	content, err := os.ReadFile(scenarioPath)
	if err != nil {
		return nil, err
	}

	// Parse YAML documents (handle multiple scenarios in one file)
	var scenarios []map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	for {
		var scenario map[string]any
		if err := decoder.Decode(&scenario); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}
		scenarios = append(scenarios, scenario)
	}

	// Modify plugins for WASM tests
	for _, scenario := range scenarios {
		if plugins, ok := scenario["plugins"].(map[string]any); ok {
			for pluginName, pluginPathAny := range plugins {
				if pluginPath, ok := pluginPathAny.(string); ok && strings.HasSuffix(pluginPath, ".so") {
					// Replace .so with .wasm
					plugins[pluginName] = strings.TrimSuffix(pluginPath, ".so") + ".wasm"
				}
			}
		}
	}

	// Marshal back to YAML
	var buf bytes.Buffer
	for i, scenario := range scenarios {
		if i > 0 {
			// Add document separator for multiple scenarios
			buf.WriteString("\n---\n")
		}
		encoder := yaml.NewEncoder(&buf)
		if err := encoder.Encode(scenario); err != nil {
			encoder.Close()
			return nil, fmt.Errorf("failed to encode scenario: %w", err)
		}
		encoder.Close()
	}

	return buf.Bytes(), nil
}

func loadAndModifyExpectedOutput(stdoutPath, pluginType string) ([]byte, error) {
	content, err := os.ReadFile(stdoutPath)
	if err != nil {
		return nil, err
	}

	// Replace .so with the target plugin type (.wasm or .so) in expected output
	if pluginType == ".wasm" {
		// Replace plugin references from .so to .wasm
		re := regexp.MustCompile(`([a-zA-Z0-9_-]+)\.so`)
		content = re.ReplaceAll(content, []byte("${1}.wasm"))
	}

	return content, nil
}

func normalizePathForComparison(output, pluginType string) string {
	if pluginType == ".wasm" {
		// For WASM tests, replace temporary file paths with normalized paths
		// This is a simple approach - replace temp file paths with testdata paths
		tempPathRe := regexp.MustCompile(`/var/folders/[^/]+/[^/]+/T/scenario[^/]*\.yaml`)
		output = tempPathRe.ReplaceAllString(output, "testdata/testcases/scenarios/{{scenario}}.yaml")
	}
	return output
}

func startGRPCServer(t *testing.T) func() {
	t.Helper()

	token := "XXXXX"
	testServer := &testGRPCServer{
		users: map[string]string{
			token: "test user",
		},
	}
	s := grpc.NewServer()
	test.RegisterTestServer(s, testServer)

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	t.Setenv("TEST_GRPC_SERVER_ADDR", ln.Addr().String())
	t.Setenv("TEST_TOKEN", token)

	go func() {
		_ = s.Serve(ln)
	}()

	return func() {
		s.Stop()
	}
}

func runMockServer(t *testing.T, filename string) func(*testing.T) {
	t.Helper()

	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var config mock.ServerConfig
	if err := yaml.NewDecoder(f, yaml.Strict()).Decode(&config); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	l := logger.NewLogger(log.New(&b, "", log.LstdFlags), logger.LogLevelAll)
	srv, err := mock.NewServer(&config, l)
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan error)
	go func() {
		ch <- srv.Start(gocontext.Background())
	}()
	ctx, cancel := gocontext.WithTimeout(gocontext.Background(), time.Second)
	defer cancel()
	if err := srv.Wait(ctx); err != nil {
		t.Fatalf("failed to wait: %s", err)
	}
	addrs, err := srv.Addrs()
	if err != nil {
		t.Fatal(err)
	}
	for p, addr := range addrs {
		t.Setenv(fmt.Sprintf("TEST_%s_ADDR", strings.ToUpper(p)), addr)
	}
	return func(t *testing.T) {
		t.Helper()
		c, cancel := gocontext.WithTimeout(gocontext.Background(), time.Second)
		defer cancel()
		if err := srv.Stop(c); err != nil {
			if err != nil {
				t.Fatalf("failed to stop: %s", err)
			}
		}
		if err := <-ch; err != nil {
			t.Fatalf("failed to start: %s", err)
		}
		if t.Failed() {
			t.Log(b.String())
		}
	}
}
