//go:build !race
// +build !race

package scenarigo

import (
	"bytes"
	gocontext "context"
	"fmt"
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

	pluginTypes := []string{"so", "wasm"}
	for _, pluginType := range pluginTypes {
		t.Run(pluginType, func(t *testing.T) {
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
						t.Run(scenario.Filename, func(t *testing.T) {
							if scenario.Mocks != "" {
								teardown := runMockServer(t, filepath.Join(dir, "mocks", scenario.Mocks))
								defer teardown(t)
							}

							// Get absolute paths before potentially changing working directory
							pluginDir, err := filepath.Abs("testdata/gen/plugins")
							if err != nil {
								t.Fatal(err)
							}

							expectedOutputPath, err := filepath.Abs(filepath.Join(dir, "stdout", scenario.Output.Stdout))
							if err != nil {
								t.Fatal(err)
							}

							config := &schema.Config{
								Vars: map[string]any{
									"global": `{{"aaa"}}`,
								},
								PluginDirectory: pluginDir,
								Plugins:         schema.NewOrderedMap[string, schema.PluginConfig](),
							}
							for _, p := range scenario.Plugins {
								// For WASM tests, adjust plugin names to match modified scenario content
								pluginName := p
								if pluginType == "wasm" {
									// Replace .so extension with .wasm in plugin name
									if filepath.Ext(p) == ".so" {
										pluginName = strings.TrimSuffix(p, ".so") + ".wasm"
									}
								}
								config.Plugins.Set(pluginName, schema.PluginConfig{})
							}

							var r *scenarigo.Runner
							originalScenarioPath := filepath.Join(dir, "scenarios", scenario.Filename)

							if pluginType == "so" {
								// Use original file path for .so tests to maintain backward compatibility
								var err error
								r, err = scenarigo.NewRunner(
									scenarigo.WithConfig(config),
									scenarigo.WithScenarios(originalScenarioPath),
								)
								if err != nil {
									t.Fatal(err)
								}
							} else {
								// Use modified scenario for .wasm tests
								scenarioContent, err := loadAndModifyScenario(originalScenarioPath, pluginType)
								if err != nil {
									t.Fatal(err)
								}

								// Create a temporary file with the same name as the original
								tmpDir := t.TempDir()

								// Create the same directory structure as the original testdata/testcases
								testcasesDir := filepath.Join(tmpDir, "testdata", "testcases")
								tmpScenarioPath := filepath.Join(testcasesDir, "scenarios", scenario.Filename)
								scenarioDir := filepath.Dir(tmpScenarioPath)
								if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
									t.Fatal(err)
								}

								// Write the modified content to a file with the original filename
								if err := os.WriteFile(tmpScenarioPath, scenarioContent, 0o600); err != nil {
									t.Fatal(err)
								}

								// Copy all YAML files from scenarios directory for include support
								originalScenariosDir := filepath.Join("testdata", "testcases", "scenarios")
								tmpScenariosDir := filepath.Join(testcasesDir, "scenarios")
								if err := copyAllScenarioFiles(originalScenariosDir, tmpScenariosDir, pluginType); err != nil {
									t.Fatal(err)
								}

								// Change to the temporary directory to ensure relative paths work correctly
								t.Chdir(tmpDir)

								// Use the relative path that matches the original structure
								relativePath := filepath.Join("testdata", "testcases", "scenarios", scenario.Filename)
								r, err = scenarigo.NewRunner(
									scenarigo.WithConfig(config),
									scenarigo.WithScenarios(relativePath),
								)
								if err != nil {
									t.Fatal(err)
								}
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
								t.Errorf("expect %t but got %t", scenario.Success, ok)
							}

							expectedStdout, err := loadAndModifyExpectedOutput(expectedOutputPath, pluginType)
							if err != nil {
								t.Fatal(err)
							}

							if got, expect := testutil.ReplaceOutput(b.String()), testutil.ReplaceOutput(string(expectedStdout)); got != expect {
								dmp := diffmatchpatch.New()
								diffs := dmp.DiffMain(expect, got, false)
								t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
							}
						})
					}
				})
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

	// For WASM plugins, use simple string replacement to preserve line numbers
	content, err := os.ReadFile(scenarioPath)
	if err != nil {
		return nil, err
	}

	// Replace .so with .wasm using string replacement to preserve formatting
	modifiedContent := strings.ReplaceAll(string(content), ".so", ".wasm")
	return []byte(modifiedContent), nil
}

func loadAndModifyExpectedOutput(stdoutPath, pluginType string) ([]byte, error) {
	content, err := os.ReadFile(stdoutPath)
	if err != nil {
		return nil, err
	}

	// Replace .so with the target plugin type (.wasm or .so) in expected output
	if pluginType == "wasm" {
		// Replace plugin references from .so to .wasm
		re := regexp.MustCompile(`([a-zA-Z0-9_-]+)\.so`)
		content = re.ReplaceAll(content, []byte("${1}.wasm"))
	}

	return content, nil
}

// copyAllScenarioFiles recursively copies all YAML files from scenarios directory
// for supporting include functionality in WASM tests.
func copyAllScenarioFiles(srcDir, dstDir, pluginType string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only copy YAML files
		if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}

		// Calculate relative path from source directory
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Create destination path
		dstPath := filepath.Join(dstDir, relPath)
		dstFileDir := filepath.Dir(dstPath)

		// Create destination directory if it doesn't exist
		if err := os.MkdirAll(dstFileDir, 0o755); err != nil {
			return err
		}

		// Read the source file
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Apply the same modifications as the main scenario file for WASM tests
		if pluginType == "wasm" {
			modifiedContent := strings.ReplaceAll(string(content), ".so", ".wasm")
			content = []byte(modifiedContent)
		}

		// Write to destination
		return os.WriteFile(dstPath, content, 0o600)
	})
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
