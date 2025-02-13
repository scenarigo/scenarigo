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

					config := &schema.Config{
						Vars: map[string]any{
							"global": `{{"aaa"}}`,
						},
						PluginDirectory: "testdata/gen/plugins",
						Plugins:         schema.NewOrderedMap[string, schema.PluginConfig](),
					}
					for _, p := range scenario.Plugins {
						config.Plugins.Set(p, schema.PluginConfig{})
					}

					r, err := scenarigo.NewRunner(
						scenarigo.WithConfig(config),
						scenarigo.WithScenarios(filepath.Join(dir, "scenarios", scenario.Filename)),
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
						t.Errorf("expect %t but got %t", scenario.Success, ok)
					}

					f, err := os.Open(filepath.Join(dir, "stdout", scenario.Output.Stdout))
					if err != nil {
						t.Fatal(err)
					}
					defer f.Close()

					stdout, err := io.ReadAll(f)
					if err != nil {
						t.Fatal(err)
					}

					if got, expect := testutil.ReplaceOutput(b.String()), string(stdout); got != expect {
						dmp := diffmatchpatch.New()
						diffs := dmp.DiffMain(expect, got, false)
						t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
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
		os.Setenv(fmt.Sprintf("TEST_%s_ADDR", strings.ToUpper(p)), addr)
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
