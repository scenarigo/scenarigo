package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/build"
	"io"
	"io/fs"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/sosedoff/gitkit"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	"github.com/zoncoen/scenarigo"
	"github.com/zoncoen/scenarigo/cmd/scenarigo/cmd/config"
)

var (
	bash string
	echo string
)

func init() {
	var err error
	bash, err = exec.LookPath("bash")
	if err != nil {
		panic("bash command not found")
	}
	echo, err = exec.LookPath("echo")
	if err != nil {
		panic("echo command not found")
	}
}

func TestBuild(t *testing.T) {
	goVersion := strings.TrimPrefix(goVer, "go")
	pluginCode := `package main

func Greet() string {
	return "Hello, world!"
}
`
	gomod := func(m string) string {
		return fmt.Sprintf(`module %s

go %s
`, m, goVersion)
	}

	tmpl, err := template.ParseFiles("testdata/go.mod.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	vs := getModuleVersions(t)
	var b bytes.Buffer
	if err := tmpl.Execute(&b, map[string]interface{}{
		"goVersion": goVersion,
		"modules":   vs,
	}); err != nil {
		t.Fatal(err)
	}
	gomodWithRequire := b.String()

	goCmd, err := findGoCmd(context.Background())
	if err != nil {
		t.Fatalf("failed to find go command: %s", err)
	}
	setupGitServer(t, goCmd)

	t.Cleanup(func() {
		cache := filepath.Join(build.Default.GOPATH, "pkg", "mod", "127.0.0.1")
		if err := filepath.Walk(cache, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			return os.Chmod(path, 0o777)
		}); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(cache); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			config            string
			files             map[string]string
			expectPluginPaths []string
			expectGoMod       map[string]string
			skipOpen          bool
		}{
			"no plugins": {
				config: `
schemaVersion: config/v1
`,
			},
			"src is a file": {
				config: `
schemaVersion: config/v1
plugins:
  plugin.so:
    src: src/main.go
`,
				files: map[string]string{
					"src/main.go": pluginCode,
				},
				expectPluginPaths: []string{"plugin.so"},
				expectGoMod: map[string]string{
					"src/go.mod": gomod("plugins/plugin"),
				},
			},
			"src is a directory": {
				config: `
schemaVersion: config/v1
plugins:
  gen/plugin.so:
    src: src
`,
				files: map[string]string{
					"src/main.go": pluginCode,
				},
				expectPluginPaths: []string{"gen/plugin.so"},
				expectGoMod: map[string]string{
					"src/go.mod": gomod("plugins/gen/plugin"),
				},
			},
			"specify pluginDirectory": {
				config: `
schemaVersion: config/v1
pluginDirectory: gen
plugins:
  plugin.so:
    src: src
`,
				files: map[string]string{
					"src/main.go": pluginCode,
				},
				expectPluginPaths: []string{"gen/plugin.so"},
				expectGoMod: map[string]string{
					"src/go.mod": gomod("plugins/plugin"),
				},
			},
			"update go.mod": {
				config: `
schemaVersion: config/v1
plugins:
  plugin.so:
    src: src/main.go
`,
				files: map[string]string{
					"src/main.go": `package main

import (
	_ "google.golang.org/grpc"
)

func Greet() string {
	return "Hello, world!"
}
`,
					"src/go.mod": fmt.Sprintf(`module plugins/plugin

go %s

require google.golang.org/grpc v1.63.2
`, goVersion),
				},
				expectPluginPaths: []string{"plugin.so"},
				expectGoMod: map[string]string{
					"src/go.mod": gomodWithRequire,
				},
			},
			"update go.mod (remove replace)": {
				config: `
schemaVersion: config/v1
plugins:
  plugin.so:
    src: src/main.go
`,
				files: map[string]string{
					"src/main.go": `package main

import (
	_ "google.golang.org/grpc"
)

func Greet() string {
	return "Hello, world!"
}
`,
					"src/go.mod": fmt.Sprintf(`module plugins/plugin

go %s

require google.golang.org/grpc v1.63.2

replace google.golang.org/grpc v1.37.1 => google.golang.org/grpc v1.40.0
`, goVersion),
				},
				expectPluginPaths: []string{"plugin.so"},
				expectGoMod: map[string]string{
					"src/go.mod": gomodWithRequire,
				},
			},
			`src is a "go gettable" remote git repository`: {
				config: `
schemaVersion: config/v1
plugins:
  gen/plugin.so:
    src: 127.0.0.1/plugin.git
`,
				files:             map[string]string{},
				expectPluginPaths: []string{"gen/plugin.so"},
				expectGoMod:       map[string]string{},
			},
			`src is a remote git repository with version`: {
				config: `
schemaVersion: config/v1
plugins:
  gen/plugin.so:
    src: 127.0.0.1/plugin.git@v1.0.0
`,
				files:             map[string]string{},
				expectPluginPaths: []string{"gen/plugin.so"},
				expectGoMod:       map[string]string{},
				skipOpen:          true,
			},
			`src is a remote git repository with latest version`: {
				config: `
schemaVersion: config/v1
plugins:
  gen/plugin.so:
    src: 127.0.0.1/plugin.git@latest
`,
				files:             map[string]string{},
				expectPluginPaths: []string{"gen/plugin.so"},
				expectGoMod:       map[string]string{},
				skipOpen:          true,
			},
			`src is a sub directory of remote git repository`: {
				config: `
schemaVersion: config/v1
plugins:
  gen/plugin.so:
    src: 127.0.0.1/sub.git/plugin@v1.0.0
`,
				files:             map[string]string{},
				expectPluginPaths: []string{"gen/plugin.so"},
				expectGoMod:       map[string]string{},
			},
			`should escape file path of remote repository`: {
				config: `
schemaVersion: config/v1
plugins:
  gen/NeedEscape.so:
    src: 127.0.0.1/NeedEscape.git
`,
				files:             map[string]string{},
				expectPluginPaths: []string{"gen/NeedEscape.so"},
				expectGoMod:       map[string]string{},
			},
			"multi plugins that require different module versions": {
				config: `
schemaVersion: config/v1
pluginDirectory: gen
plugins:
  plugin1.so:
    src: src/plugin1
  plugin2.so:
    src: src/plugin2
  plugin3.so:
    src: src/plugin3
`,
				files: map[string]string{
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0
`, goVersion),
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v1.1.0
`, goVersion),
					"src/plugin3/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git/v2"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin3/go.mod": fmt.Sprintf(`module plugin3

go %s

require 127.0.0.1/gomodule.git/v2 v2.0.0
`, goVersion),
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
					"gen/plugin3.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`, goVersion),
					"src/plugin2/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`, goVersion),
					"src/plugin3/go.mod": fmt.Sprintf(`module plugin3

go %s

require 127.0.0.1/gomodule.git/v2 v2.0.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`, goVersion),
				},
			},
			"multi plugins with incompatible module": {
				config: `
schemaVersion: config/v1
pluginDirectory: gen
plugins:
  plugin1.so:
    src: src/plugin1
  plugin2.so:
    src: src/plugin2
`,
				files: map[string]string{
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0
`, goVersion),
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v2.0.0+incompatible
`, goVersion),
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v2.0.0+incompatible

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect
`, goVersion),
					"src/plugin2/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v2.0.0+incompatible

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect
`, goVersion),
				},
				skipOpen: true,
			},
			"override by go get-able module source": {
				config: `
schemaVersion: config/v1
pluginDirectory: gen
plugins:
  plugin1.so:
    src: src/plugin1
  plugin2.so:
    src: 127.0.0.1/sub.git/plugin@v1.0.0
`,
				files: map[string]string{
					"src/plugin1/main.go": `package main

import (
	"127.0.0.1/sub.git/src"
)

var Src = src.Src
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/sub.git v1.1.0
`, goVersion),
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/sub.git v1.0.0
`, goVersion),
				},
				skipOpen: true,
			},
			"override by replace": {
				config: `
schemaVersion: config/v1
pluginDirectory: gen
plugins:
  plugin1.so:
    src: src/plugin1
  plugin2.so:
    src: src/plugin2
  plugin3.so:
    src: src/plugin3
`,
				files: map[string]string{
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => 127.0.0.1/dependent-gomodule.git v1.1.0
`, goVersion),
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect
`, goVersion),
					"src/plugin3/main.go": `package main

import (
	_ "127.0.0.1/dependent-gomodule.git"
)
`,
					"src/plugin3/go.mod": fmt.Sprintf(`module plugin3

go %s

require 127.0.0.1/dependent-gomodule.git v1.0.0
`, goVersion),
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
					"gen/plugin3.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`, goVersion),
					"src/plugin2/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`, goVersion),
					"src/plugin3/go.mod": fmt.Sprintf(`module plugin3

go %s

require 127.0.0.1/dependent-gomodule.git v1.1.0
`, goVersion),
				},
				skipOpen: true,
			},
			"override by local path replace": {
				config: `
schemaVersion: config/v1
pluginDirectory: gen
plugins:
  plugin1.so:
    src: src/plugin1
`,
				files: map[string]string{
					"src/main.go": `package dependent

var Dependency = "local-dependent"
`,
					"src/go.mod": fmt.Sprintf(`module 127.0.0.1/dependent-gomodule.git

go %s
`, goVersion),
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git => ../
`, goVersion),
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../
`, goVersion),
				},
				skipOpen: true,
			},
			"override by local path replace (multi replace but same directory)": {
				config: `
schemaVersion: config/v1
pluginDirectory: gen
plugins:
  plugin1.so:
    src: src/plugin1
  plugin2.so:
    src: src/plugin2/sub
  plugin3.so:
    src: src/plugin3
`,
				files: map[string]string{
					"src/local/main.go": `package dependent

var Dependency = "local-dependent"
`,
					"src/local/go.mod": fmt.Sprintf(`module 127.0.0.1/dependent-gomodule.git

go %s
`, goVersion),
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git => ./../local
`, goVersion),
					"src/plugin2/sub/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/sub/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../../local
`, goVersion),
					"src/plugin3/main.go": `package main

import (
	_ "127.0.0.1/dependent-gomodule.git"
)
`,
					"src/plugin3/go.mod": fmt.Sprintf(`module plugin3

go %s

require 127.0.0.1/dependent-gomodule.git v1.0.0
`, goVersion),
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
					"gen/plugin3.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local
`, goVersion),
					"src/plugin2/sub/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../../local
`, goVersion),
					"src/plugin3/go.mod": fmt.Sprintf(`module plugin3

go %s

require 127.0.0.1/dependent-gomodule.git v1.0.0

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local
`, goVersion),
				},
				skipOpen: true,
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, config.DefaultConfigFileName)
				create(t, configPath, test.config)
				for p, content := range test.files {
					create(t, filepath.Join(tmpDir, p), content)
				}
				cmd := &cobra.Command{}
				config.ConfigPath = configPath
				if err := buildRun(cmd, []string{}); err != nil {
					t.Fatal(err)
				}
				for _, p := range test.expectPluginPaths {
					if _, err := os.Stat(filepath.Join(tmpDir, p)); err != nil {
						t.Fatalf("plugin not found: %s", err)
					}
					if !test.skipOpen {
						openPlugin(t, filepath.Join(tmpDir, p))
					}
				}
				for path, expect := range test.expectGoMod {
					b, err := os.ReadFile(filepath.Join(tmpDir, path))
					if err != nil {
						t.Fatalf("failed read go.mod: %s", err)
					}
					if got := string(b); got != expect {
						dmp := diffmatchpatch.New()
						diffs := dmp.DiffMain(expect, got, false)
						t.Errorf("go.mod differs:\n%s", dmp.DiffPrettyText(diffs))
						t.Errorf("==== got =====\n%s\n", got)
						t.Errorf("=== expect ===\n%s\n", expect)
					}
				}
			})
		}
		t.Run("check cache", func(t *testing.T) {
			m := filepath.Join(build.Default.GOPATH, "pkg", "mod", "127.0.0.1", "plugin.git@v1.0.0")
			if _, err := os.Stat(m); err == nil {
				t.Fatal("plugin module should be removed from the cache")
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed to get file info: %s", err)
			}
		})
	})

	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			config string
			files  map[string]string
			expect string
		}{
			"no config": {
				config: "",
				expect: "config file not found",
			},
			"specify invalid config": {
				config: "schemaVersion: test",
				expect: "failed to load config",
			},
			"build failed": {
				config: `
schemaVersion: config/v1
plugins:
  plugin.so:
    src: src/main.go
`,
				files: map[string]string{
					"src/main.go": `packag plugin`,
				},
				expect: "expected 'package', found packag",
			},
			"go version is too high": {
				config: `
schemaVersion: config/v1
plugins:
  plugin.so:
    src: src/main.go
`,
				files: map[string]string{
					"src/main.go": `package plugin`,
					"src/go.mod":  `go 100.0.0`,
				},
				expect: fmt.Sprintf(`failed to build plugin plugin.so: re-install scenarigo command with go100.0.0: "go mod tidy" failed: go: go.mod requires go >= 100.0.0 (running go %s; GOTOOLCHAIN=%s)`, goVersion, toolchain),
			},
			"invalid version": {
				config: `
schemaVersion: config/v1
plugins:
  plugin.so:
    src: 127.0.0.1/plugin.git@v1.5.0
`,
				expect: "unknown revision v1.5.0",
			},
			"incompatible module": {
				config: `
schemaVersion: config/v1
pluginDirectory: gen
plugins:
  plugin.so:
    src: src/plugin
`,
				files: map[string]string{
					"src/plugin/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v2.0.0
`, goVersion),
				},
				expect: `require 127.0.0.1/gomodule.git: version "v2.0.0" invalid: should be v0 or v1, not v2`,
			},
			"can't build remote module": {
				config: `
schemaVersion: config/v1
plugins:
  plugin.so:
    src: 127.0.0.1/not-plugin.git
`,
				expect: "-buildmode=plugin requires exactly one main package",
			},
			"invalid replace local path": {
				config: `
schemaVersion: config/v1
plugins:
  plugin1.so:
    src: src/plugin1
`,
				files: map[string]string{
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local1
`, goVersion),
				},
				expect: "replacement directory ../local1 does not exist",
			},
			"replace directive conflicts (different versions)": {
				config: `
schemaVersion: config/v1
plugins:
  plugin1.so:
    src: src/plugin1
  plugin2.so:
    src: src/plugin2
`,
				files: map[string]string{
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => 127.0.0.1/dependent-gomodule.git v1.1.0
`, goVersion),
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => 127.0.0.1/dependent-gomodule.git/v2 v2.0.0
`, goVersion),
				},
				expect: "replace 127.0.0.1/dependent-gomodule.git directive conflicts: plugin1.so => 127.0.0.1/dependent-gomodule.git v1.1.0, plugin2.so => 127.0.0.1/dependent-gomodule.git/v2 v2.0.0",
			},
			"replace directive conflicts (version and path)": {
				config: `
schemaVersion: config/v1
plugins:
  plugin1.so:
    src: src/plugin1
  plugin2.so:
    src: src/plugin2
`,
				files: map[string]string{
					"src/local/main.go": `package dependent

var Dependency = "local-dependent"
`,
					"src/local/go.mod": fmt.Sprintf(`module 127.0.0.1/dependent-gomodule.git

go %s
`, goVersion),
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => 127.0.0.1/dependent-gomodule.git v1.1.0
`, goVersion),
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local
`, goVersion),
				},
				expect: "replace 127.0.0.1/dependent-gomodule.git directive conflicts: plugin1.so => 127.0.0.1/dependent-gomodule.git v1.1.0, plugin2.so => ../local",
			},
			"replace directive conflicts (different paths)": {
				config: `
schemaVersion: config/v1
plugins:
  plugin1.so:
    src: src/plugin1
  plugin2.so:
    src: src/plugin2
`,
				files: map[string]string{
					"src/local1/main.go": `package dependent

var Dependency = "local-dependent"
`,
					"src/local1/go.mod": fmt.Sprintf(`module 127.0.0.1/dependent-gomodule.git

go %s
`, goVersion),
					"src/local2/main.go": `package dependent

var Dependency = "local-dependent"
`,
					"src/local2/go.mod": fmt.Sprintf(`module 127.0.0.1/dependent-gomodule.git

go %s
`, goVersion),
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": fmt.Sprintf(`module plugin1

go %s

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local1
`, goVersion),
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": fmt.Sprintf(`module plugin2

go %s

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local2
`, goVersion),
				},
				expect: "replace 127.0.0.1/dependent-gomodule.git directive conflicts: plugin1.so => ../local1, plugin2.so => ../local2",
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				tmpDir := t.TempDir()
				var configPath string
				if test.config != "" {
					configPath = filepath.Join(tmpDir, config.DefaultConfigFileName)
					create(t, configPath, test.config)
				}
				for p, content := range test.files {
					create(t, filepath.Join(tmpDir, p), content)
				}
				cmd := &cobra.Command{}
				config.ConfigPath = configPath
				err := buildRun(cmd, []string{})
				if err == nil {
					t.Fatal("no error")
				}
				if !strings.Contains(err.Error(), test.expect) {
					t.Fatalf("unexpected error: %s", err)
				}
			})
		}
	})
}

func getModuleVersions(t *testing.T) map[string]string {
	t.Helper()
	gomod, err := modfile.Parse("go.mod", scenarigo.GoModBytes, nil)
	if err != nil {
		t.Fatalf("failed to parse go.mod of scenarigo: %s", err)
	}
	vs := map[string]string{}
	for _, r := range gomod.Require {
		vs[r.Mod.Path] = r.Mod.Version
	}
	return vs
}

func TestFindGoCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			cmds   map[string]string
			expect string
			tip    bool
		}{
			"found go command": {
				cmds: map[string]string{
					"go": fmt.Sprintf("go version %s linux/amd64", goVer),
				},
				expect: "go",
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				tmpDir := t.TempDir()
				for cmd, stdout := range test.cmds {
					createExecutable(t, filepath.Join(tmpDir, cmd), stdout)
				}
				t.Setenv("PATH", tmpDir)
				goCmd, err := findGoCmd(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if got, expect := filepath.Base(goCmd), test.expect; got != expect {
					t.Errorf("expect %q but got %q", expect, got)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			cmds   map[string]string
			expect string
		}{
			"command not found": {
				expect: "go command required",
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				tmpDir := t.TempDir()
				for cmd, stdout := range test.cmds {
					createExecutable(t, filepath.Join(tmpDir, cmd), stdout)
				}
				t.Setenv("PATH", tmpDir)
				_, err := findGoCmd(context.Background())
				if err == nil {
					t.Fatal("no error")
				}
				if !strings.Contains(err.Error(), test.expect) {
					t.Fatalf("unexpected error: %s", err)
				}
			})
		}
	})
}

func TestUpdateGoMod(t *testing.T) {
	goVersion := strings.TrimPrefix(goVer, "go")
	modVers := getModuleVersions(t)

	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			gomod        string
			src          string
			overrides    map[string]*overrideModule
			expect       string
			expectStdout string
		}{
			"do nothing": {
				gomod: fmt.Sprintf(`module plugin_module

go %s
`, goVersion),
				expect: fmt.Sprintf(`module plugin_module

go %s
`, goVersion),
			},
			"add toolchain directive": {
				gomod: `module plugin_module

go 1.21
`,
				expect: fmt.Sprintf(`module plugin_module

go 1.21

toolchain go%s
`, goVersion),
				expectStdout: fmt.Sprintf("WARN: test.so: add toolchain go%s by scenarigo\n", goVersion),
			},
			"change toolchain directive": {
				gomod: `module plugin_module

go 1.21

toolchain go1.21.1
`,
				expect: fmt.Sprintf(`module plugin_module

go 1.21

toolchain go%s
`, goVersion),
				expectStdout: fmt.Sprintf("WARN: test.so: change toolchain go1.21.1 ==> go%s by scenarigo\n", goVersion),
			},
			"do nothing (no requires)": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`, goVersion),
				src: `package main

import (
	_ "google.golang.org/grpc"
)
`,
				expect: fmt.Sprintf(`module plugin_module

go %s

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`, goVersion),
			},
			"do nothing (not used)": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require github.com/zoncoen/scenarigo v0.11.2

replace github.com/zoncoen/scenarigo v0.11.2 => github.com/zoncoen/scenarigo v0.11.0
`, goVersion),
				overrides: map[string]*overrideModule{
					"google.golang.org/grpc": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.37.1",
							},
						},
						requiredBy: "test",
					},
				},
				expect: fmt.Sprintf(`module plugin_module

go %s
`, goVersion),
				expectStdout: `WARN: test.so: remove replace github.com/zoncoen/scenarigo v0.11.2 => github.com/zoncoen/scenarigo v0.11.0
`,
			},
			"add require": {
				gomod: fmt.Sprintf(`module plugin_module

go %s
`, goVersion),
				src: `package main

import (
	_ "google.golang.org/grpc"
)
`,
				overrides: map[string]*overrideModule{
					"google.golang.org/grpc": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.37.1",
							},
						},
						requiredBy: "test",
					},
				},
				expect: fmt.Sprintf(`module plugin_module

go %s

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.5.0 // indirect
	golang.org/x/net v0.22.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)
`, goVersion),
				expectStdout: fmt.Sprintf(`WARN: test.so: change require google.golang.org/grpc %s ==> v1.37.1 by test
`, modVers["google.golang.org/grpc"]),
			},
			"overwrite require by require": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`, goVersion),
				src: `package main

import (
	_ "google.golang.org/grpc"
)
`,
				overrides: map[string]*overrideModule{
					"google.golang.org/grpc": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.40.0",
							},
						},
						requiredBy: "test",
					},
				},
				expect: fmt.Sprintf(`module plugin_module

go %s

require google.golang.org/grpc v1.40.0

require (
	github.com/golang/protobuf v1.4.3 // indirect
	golang.org/x/net v0.0.0-20200822124328-c89045814202 // indirect
	golang.org/x/sys v0.0.0-20200323222414-85ca7c5b95cd // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`, goVersion),
				expectStdout: `WARN: test.so: change require google.golang.org/grpc v1.37.1 ==> v1.40.0 by test
`,
			},
			"overwrite require by replace": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`, goVersion),
				src: `package main

import (
	_ "google.golang.org/grpc"
)
`,
				overrides: map[string]*overrideModule{
					"google.golang.org/grpc": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.46.0",
							},
						},
						requiredBy: "test",
						replace: &modfile.Replace{
							Old: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.46.0",
							},
							New: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.40.0",
							},
						},
						replacedBy: "test",
					},
				},
				expect: fmt.Sprintf(`module plugin_module

go %s

require google.golang.org/grpc v1.40.0

require (
	github.com/golang/protobuf v1.4.3 // indirect
	golang.org/x/net v0.0.0-20200822124328-c89045814202 // indirect
	golang.org/x/sys v0.0.0-20200323222414-85ca7c5b95cd // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`, goVersion),
				expectStdout: `WARN: test.so: change require google.golang.org/grpc v1.37.1 ==> v1.40.0 by test
`,
			},
			"do nothing (same version)": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`, goVersion),
				src: `package main

import (
	_ "google.golang.org/grpc"
)
`,
				overrides: map[string]*overrideModule{
					"google.golang.org/grpc": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.37.1",
							},
						},
						requiredBy: "test",
					},
				},
				expect: fmt.Sprintf(`module plugin_module

go %s

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`, goVersion),
			},
			"add replace": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require github.com/zoncoen/scenarigo v0.11.2

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/goccy/go-yaml v1.9.5 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zoncoen/query-go v1.1.0 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	google.golang.org/genproto v0.0.0-20220413183235-5e96e2839df9 // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)
`, goVersion),
				src: `package main

import (
	_ "github.com/zoncoen/scenarigo/protocol/grpc"
)
`,
				overrides: map[string]*overrideModule{
					"google.golang.org/grpc": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.46.0",
							},
						},
						requiredBy: "test",
						replace: &modfile.Replace{
							Old: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.46.0",
							},
							New: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.40.0",
							},
						},
						replacedBy: "test",
					},
				},
				expect: fmt.Sprintf(`module plugin_module

go %s

require github.com/zoncoen/scenarigo v0.11.2

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/goccy/go-yaml v1.9.5 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zoncoen/query-go v1.1.0 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	google.golang.org/genproto v0.0.0-20220413183235-5e96e2839df9 // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0
`, goVersion),
				expectStdout: `WARN: test.so: add replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0 by test
`,
			},
			"override replace by replace": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require github.com/zoncoen/scenarigo v0.11.2

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/goccy/go-yaml v1.9.5 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zoncoen/query-go v1.1.0 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	google.golang.org/genproto v0.0.0-20220413183235-5e96e2839df9 // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0
`, goVersion),
				src: `package main

import (
	_ "github.com/zoncoen/scenarigo/protocol/grpc"
)
`,
				overrides: map[string]*overrideModule{
					"google.golang.org/grpc": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.46.0",
							},
						},
						requiredBy: "test",
						replace: &modfile.Replace{
							Old: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.46.0",
							},
							New: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.40.1",
							},
						},
						replacedBy: "test",
					},
				},
				expect: fmt.Sprintf(`module plugin_module

go %s

require github.com/zoncoen/scenarigo v0.11.2

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/goccy/go-yaml v1.9.5 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zoncoen/query-go v1.1.0 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	google.golang.org/genproto v0.0.0-20220413183235-5e96e2839df9 // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.1
`, goVersion),
				expectStdout: `WARN: test.so: change replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0 ==> google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.1 by test
`,
			},
			"do nothing (already replaced)": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require github.com/zoncoen/scenarigo v0.11.2

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/goccy/go-yaml v1.9.5 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zoncoen/query-go v1.1.0 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	google.golang.org/genproto v0.0.0-20220413183235-5e96e2839df9 // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0
`, goVersion),
				src: `package main

import (
	_ "github.com/zoncoen/scenarigo/protocol/grpc"
)
`,
				overrides: map[string]*overrideModule{
					"google.golang.org/grpc": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.46.0",
							},
						},
						requiredBy: "test",
						replace: &modfile.Replace{
							Old: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.46.0",
							},
							New: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.40.0",
							},
						},
						replacedBy: "test",
					},
				},
				expect: fmt.Sprintf(`module plugin_module

go %s

require github.com/zoncoen/scenarigo v0.11.2

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/goccy/go-yaml v1.9.5 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zoncoen/query-go v1.1.0 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	google.golang.org/genproto v0.0.0-20220413183235-5e96e2839df9 // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0
`, goVersion),
				expectStdout: "", // don't print the warn log if already replaced
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				tmpDir := t.TempDir()
				gomod := filepath.Join(tmpDir, "go.mod")
				create(t, gomod, test.gomod)
				if test.src != "" {
					create(t, filepath.Join(tmpDir, "main.go"), test.src)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				goCmd, err := findGoCmd(ctx)
				if err != nil {
					t.Fatalf("failed to find go command: %s", err)
				}

				overrideKeys := make([]string, 0, len(test.overrides))
				for k := range test.overrides {
					overrideKeys = append(overrideKeys, k)
				}
				sort.Strings(overrideKeys)

				cmd := &cobra.Command{}
				var stdout bytes.Buffer
				cmd.SetOutput(&stdout)
				pb, err := newPluginBuilder(cmd, goCmd, "test.so", gomod, test.src, filepath.Join(tmpDir, "test.so"), "test")
				if err != nil {
					t.Fatalf("failed to create plugin builder: %s", err)
				}

				if err := pb.updateGoMod(cmd, goCmd, overrideKeys, test.overrides); err != nil {
					t.Fatalf("failed to update go.mod: %s", err)
				}
				if err := pb.printUpdatedResult(cmd, goCmd, pb.name, pb.gomodPath, test.overrides); err != nil {
					t.Fatalf("failed to print updatet result: %s", err)
				}

				b, err := os.ReadFile(gomod)
				if err != nil {
					t.Fatalf("failed read go.mod: %s", err)
				}
				if got := string(b); got != test.expect {
					dmp := diffmatchpatch.New()
					diffs := dmp.DiffMain(test.expect, got, false)
					t.Errorf("go.mod differs:\n%s", dmp.DiffPrettyText(diffs))
				}

				if got := strings.ReplaceAll(stdout.String(), gomod, "/path/to/go.mod"); got != test.expectStdout {
					dmp := diffmatchpatch.New()
					diffs := dmp.DiffMain(test.expectStdout, got, false)
					t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
				}
			})
		}
	})

	t.Run("retry", func(t *testing.T) {
		tests := map[string]struct {
			gomod     string
			src       string
			overrides map[string]*overrideModule
			expect    string
		}{
			"change the maximum version": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require github.com/zoncoen/scenarigo v0.11.2

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/goccy/go-yaml v1.9.5 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zoncoen/query-go v1.1.0 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	google.golang.org/genproto v0.0.0-20220413183235-5e96e2839df9 // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0
`, goVersion),
				src: `package main

import (
	_ "github.com/zoncoen/scenarigo/protocol/grpc"
)
`,
				overrides: map[string]*overrideModule{
					"github.com/fatih/color": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "github.com/fatih/color",
								Version: "v1.12.0",
							},
						},
						requiredBy: "test",
					},
				},
				expect: "failed to edit replace directive: retriable error: change the maximum version of github.com/fatih/color from v1.12.0 to v1.13.0",
			},
			"change the replaced old version": {
				gomod: fmt.Sprintf(`module plugin_module

go %s

require github.com/zoncoen/scenarigo v0.11.2

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/goccy/go-yaml v1.9.5 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/zoncoen/query-go v1.1.0 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20220411194840-2f41105eb62f // indirect
	google.golang.org/genproto v0.0.0-20220413183235-5e96e2839df9 // indirect
	google.golang.org/grpc v1.46.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0
`, goVersion),
				src: `package main

import (
	_ "github.com/zoncoen/scenarigo/protocol/grpc"
)
`,
				overrides: map[string]*overrideModule{
					"google.golang.org/grpc": {
						require: &modfile.Require{
							Mod: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.45.0",
							},
						},
						requiredBy: "test",
						replace: &modfile.Replace{
							Old: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.45.0",
							},
							New: module.Version{
								Path:    "google.golang.org/grpc",
								Version: "v1.40.0",
							},
						},
						replacedBy: "test",
					},
				},
				expect: "failed to edit replace directive: retriable error: change the replaced old version of google.golang.org/grpc from v1.45.0 to v1.46.0",
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				tmpDir := t.TempDir()
				gomod := filepath.Join(tmpDir, "go.mod")
				create(t, gomod, test.gomod)
				if test.src != "" {
					create(t, filepath.Join(tmpDir, "main.go"), test.src)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				goCmd, err := findGoCmd(ctx)
				if err != nil {
					t.Fatalf("failed to find go command: %s", err)
				}

				overrideKeys := make([]string, 0, len(test.overrides))
				for k := range test.overrides {
					overrideKeys = append(overrideKeys, k)
				}
				sort.Strings(overrideKeys)

				cmd := &cobra.Command{}
				var stdout bytes.Buffer
				cmd.SetOutput(&stdout)

				pb := &pluginBuilder{
					name:      "test.so",
					gomodPath: gomod,
				}
				err = pb.updateGoMod(cmd, goCmd, overrideKeys, test.overrides)
				if err == nil {
					t.Fatal("no error")
				}
				if !strings.Contains(err.Error(), test.expect) {
					t.Fatalf("expected %q but got %q", test.expect, err)
				}
			})
		}
	})

	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			gomod     string
			src       string
			overrides map[string]*overrideModule
			expect    string
		}{
			"too high": {
				gomod: `module plugin_module

go 100.0.0
`,
				expect: fmt.Sprintf("go.mod requires go >= 100.0.0 (running go %s)", goVer),
			},
		}
		for name, test := range tests {
			test := test
			t.Run(name, func(t *testing.T) {
				tmpDir := t.TempDir()
				gomod := filepath.Join(tmpDir, "go.mod")
				create(t, gomod, test.gomod)
				if test.src != "" {
					create(t, filepath.Join(tmpDir, "main.go"), test.src)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				goCmd, err := findGoCmd(ctx)
				if err != nil {
					t.Fatalf("failed to find go command: %s", err)
				}

				overrideKeys := make([]string, 0, len(test.overrides))
				for k := range test.overrides {
					overrideKeys = append(overrideKeys, k)
				}
				sort.Strings(overrideKeys)

				cmd := &cobra.Command{}
				var stdout bytes.Buffer
				cmd.SetOutput(&stdout)

				pb := &pluginBuilder{
					name:      "test.so",
					gomodPath: gomod,
				}
				err = pb.updateGoMod(cmd, goCmd, overrideKeys, test.overrides)
				if err == nil {
					t.Fatal("no error")
				}
				if !strings.Contains(err.Error(), test.expect) {
					t.Fatalf("expected %q but got %q", test.expect, err)
				}
			})
		}
	})
}

func setupGitServer(t *testing.T, goCmd string) {
	t.Helper()

	tempDir := t.TempDir()
	log.Default().SetOutput(io.Discard)
	git := gitkit.NewSSH(gitkit.Config{
		Dir:    tempDir,
		KeyDir: filepath.Join(tempDir, "ssh"),
	})
	git.PublicKeyLookupFunc = func(_ string) (*gitkit.PublicKey, error) {
		return &gitkit.PublicKey{}, nil
	}
	if err := git.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("failed to listen: %s", err)
	}
	go func() {
		_ = git.Serve()
	}()
	t.Cleanup(func() {
		_ = git.Stop()
	})

	u, err := url.Parse(fmt.Sprintf("http://%s", git.Address()))
	if err != nil {
		t.Fatalf("failed to parse URL: %s", err)
	}
	t.Setenv("GIT_SSH_COMMAND", fmt.Sprintf("ssh -p %s -i %s -oStrictHostKeyChecking=no -F /dev/null", u.Port(), filepath.Join(tempDir, "ssh", "gitkit.rsa")))
	t.Setenv("GOPRIVATE", "127.0.0.1")

	ctx := context.Background()
	envs := []string{
		fmt.Sprintf("GIT_CONFIG_GLOBAL=%s", filepath.Join(tempDir, ".gitconfig")),
		fmt.Sprintf("GOMODCACHE=%s", filepath.Join(tempDir, ".cache")),
	}
	t.Cleanup(func() {
		if _, err := executeWithEnvs(ctx, envs, tempDir, goCmd, "clean", "-modcache"); err != nil {
			t.Errorf("go clean -modcache failed: %s", err)
		}
	})

	// create git objects for test repositories
	if _, err := executeWithEnvs(ctx, envs, tempDir, "git", "config", "--global", "user.name", "scenarigo-test"); err != nil {
		t.Fatalf("git config failed: %s", err)
	}
	if _, err := executeWithEnvs(ctx, envs, tempDir, "git", "config", "--global", "user.email", "scenarigo-test@example.com"); err != nil {
		t.Fatalf("git config failed: %s", err)
	}
	repoDir := filepath.Join("testdata", "git")
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		t.Fatalf("failed to read directory: %s", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wd := filepath.Join(repoDir, e.Name())
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			if _, err := executeWithEnvs(ctx, envs, wd, goCmd, "mod", "tidy"); err != nil {
				t.Fatalf("go mod tidy failed: %s", err)
			}
			t.Cleanup(func() {
				os.RemoveAll(filepath.Join(wd, "go.sum"))
			})
		}
		if _, err := os.Stat(filepath.Join(wd, "v2", "go.mod")); err == nil {
			if _, err := executeWithEnvs(ctx, envs, filepath.Join(wd, "v2"), goCmd, "mod", "tidy"); err != nil {
				t.Fatalf("go mod tidy failed: %s", err)
			}
			t.Cleanup(func() {
				os.RemoveAll(filepath.Join(wd, "v2", "go.sum"))
			})
		}
		if _, err := executeWithEnvs(ctx, envs, wd, "git", "init"); err != nil {
			t.Fatalf("git init failed: %s", err)
		}
		t.Cleanup(func() {
			os.RemoveAll(filepath.Join(wd, ".git"))
		})
		if _, err := executeWithEnvs(ctx, envs, wd, "git", "add", "-A"); err != nil {
			t.Fatalf("git add failed: %s", err)
		}
		if _, err := executeWithEnvs(ctx, envs, wd, "git", "commit", "-m", "commit"); err != nil {
			t.Fatalf("git commit failed: %s", err)
		}
		if _, err := executeWithEnvs(ctx, envs, wd, "git", "tag", "v1.0.0"); err != nil {
			t.Fatalf("git tag failed: %s", err)
		}
		if _, err := executeWithEnvs(ctx, envs, wd, "git", "tag", "v1.1.0"); err != nil {
			t.Fatalf("git tag failed: %s", err)
		}
		if _, err := os.Stat(filepath.Join(wd, "v2")); err == nil {
			if _, err := executeWithEnvs(ctx, envs, wd, "git", "tag", "v2.0.0"); err != nil {
				t.Fatalf("git tag failed: %s", err)
			}
		}
		if err := os.Rename(
			filepath.Join(repoDir, e.Name(), ".git"),
			filepath.Join(tempDir, e.Name()),
		); err != nil {
			t.Fatalf("failed to rename: %s", err)
		}
	}
}

func create(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatalf("failed to create %s: %s", dir, err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create %s: %s", path, err)
	}
	defer f.Close()
	if _, err := f.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write %s: %s", path, err)
	}
}

func createExecutable(t *testing.T, path, stdout string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatalf("failed to create %s: %s", dir, err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o777)
	if err != nil {
		t.Fatalf("failed to create %s: %s", path, err)
	}
	defer f.Close()
	if _, err := f.Write([]byte(fmt.Sprintf("#!%s\n%s %q", bash, echo, stdout))); err != nil {
		t.Fatalf("failed to write %s: %s", path, err)
	}
}

func TestValidateGoVersion(t *testing.T) {
	if tip {
		t.Skip("skip on tip")
	}
	tests := map[string]struct {
		targetVer string
		expect    string
	}{
		"same version": {
			targetVer: goVer,
		},
		"different version": {
			targetVer: "go100.0.0",
			expect:    "require sceanrigo built by go100.0.0, please re-install scenarigo",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			err := validateGoVersion(context.Background(), "", "go", test.targetVer)
			if test.expect == "" && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if test.expect != "" && err == nil {
				t.Fatal("no error")
			}
		})
	}
}
