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

	"github.com/Masterminds/semver"
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

	// Old Go versions need to trim patch versions.
	v, err := semver.NewVersion(goVersion)
	if err != nil {
		t.Fatal(err)
	}
	go121, err := semver.NewVersion("1.21.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.LessThan(go121) {
		if parts := strings.Split(goVersion, "."); len(parts) > 2 {
			goVersion = fmt.Sprintf("%s.%s", parts[0], parts[1])
		}
	}

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

	goCmd, err := findGoCmd(context.Background(), false)
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

require google.golang.org/grpc v1.37.1
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

require google.golang.org/grpc v1.37.1

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
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0
`,
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v1.1.0
`,
					"src/plugin3/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git/v2"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin3/go.mod": `module plugin3

go 1.21

require 127.0.0.1/gomodule.git/v2 v2.0.0
`,
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
					"gen/plugin3.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`,
					"src/plugin2/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`,
					"src/plugin3/go.mod": `module plugin3

go 1.21

require 127.0.0.1/gomodule.git/v2 v2.0.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`,
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
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0
`,
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v2.0.0+incompatible
`,
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v2.0.0+incompatible

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect
`,
					"src/plugin2/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v2.0.0+incompatible

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect
`,
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
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/sub.git v1.1.0
`,
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/sub.git v1.0.0
`,
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
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => 127.0.0.1/dependent-gomodule.git v1.1.0
`,
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect
`,
					"src/plugin3/main.go": `package main

import (
	_ "127.0.0.1/dependent-gomodule.git"
)
`,
					"src/plugin3/go.mod": `module plugin3

go 1.21

require 127.0.0.1/dependent-gomodule.git v1.0.0
`,
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
					"gen/plugin3.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`,
					"src/plugin2/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.1.0 // indirect
`,
					"src/plugin3/go.mod": `module plugin3

go 1.21

require 127.0.0.1/dependent-gomodule.git v1.1.0
`,
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
					"src/go.mod": `module 127.0.0.1/dependent-gomodule.git

go 1.21
`,
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git => ../
`,
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../
`,
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
					"src/local/go.mod": `module 127.0.0.1/dependent-gomodule.git

go 1.21
`,
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git => ./../local
`,
					"src/plugin2/sub/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/sub/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../../local
`,
					"src/plugin3/main.go": `package main

import (
	_ "127.0.0.1/dependent-gomodule.git"
)
`,
					"src/plugin3/go.mod": `module plugin3

go 1.21

require 127.0.0.1/dependent-gomodule.git v1.0.0
`,
				},
				expectPluginPaths: []string{
					"gen/plugin1.so",
					"gen/plugin2.so",
					"gen/plugin3.so",
				},
				expectGoMod: map[string]string{
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local
`,
					"src/plugin2/sub/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../../local
`,
					"src/plugin3/go.mod": `module plugin3

go 1.21

require 127.0.0.1/dependent-gomodule.git v1.0.0

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local
`,
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
					"src/plugin/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v2.0.0
`,
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
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local1
`,
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
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => 127.0.0.1/dependent-gomodule.git v1.1.0
`,
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => 127.0.0.1/dependent-gomodule.git/v2 v2.0.0
`,
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
					"src/local/go.mod": `module 127.0.0.1/dependent-gomodule.git

go 1.21
`,
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => 127.0.0.1/dependent-gomodule.git v1.1.0
`,
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local
`,
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
					"src/local1/go.mod": `module 127.0.0.1/dependent-gomodule.git

go 1.21
`,
					"src/local2/main.go": `package dependent

var Dependency = "local-dependent"
`,
					"src/local2/go.mod": `module 127.0.0.1/dependent-gomodule.git

go 1.21
`,
					"src/plugin1/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin1/go.mod": `module plugin1

go 1.21

require 127.0.0.1/gomodule.git v1.0.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local1
`,
					"src/plugin2/main.go": `package main

import (
	"fmt"

	"127.0.0.1/gomodule.git"
)

var Dependency = fmt.Sprintf("plugin => %s", gomodule.Dependency)
`,
					"src/plugin2/go.mod": `module plugin2

go 1.21

require 127.0.0.1/gomodule.git v1.1.0

require 127.0.0.1/dependent-gomodule.git v1.0.0 // indirect

replace 127.0.0.1/dependent-gomodule.git v1.0.0 => ../local2
`,
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
			"minimum go version": {
				cmds: map[string]string{
					"go": fmt.Sprintf("go version %s linux/amd64", gomodVer),
				},
				expect: "go",
			},
			"found gotip command": {
				cmds: map[string]string{
					"gotip": fmt.Sprintf("go version %s linux/amd64", goVer),
				},
				expect: "gotip",
				tip:    true,
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
				goCmd, err := findGoCmd(context.Background(), test.tip)
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
			"old go version": {
				cmds: map[string]string{
					"go": "go version go1.20 linux/amd64",
				},
				expect: fmt.Sprintf("required go %s or later but installed 1.20", gomodVer),
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
				_, err := findGoCmd(context.Background(), false)
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
	tests := map[string]struct {
		gomod        string
		src          string
		overrides    map[string]*overrideModule
		expect       string
		expectStdout string
	}{
		"do nothing": {
			gomod: `module plugin_module

go 1.21
`,
			expect: `module plugin_module

go 1.21
`,
		},
		"do nothing (no requires)": {
			gomod: `module plugin_module

go 1.21

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`,
			src: `package main

import (
	_ "google.golang.org/grpc"
)
`,
			expect: `module plugin_module

go 1.21

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`,
		},
		"do nothing (not used)": {
			gomod: `module plugin_module

go 1.21

require github.com/zoncoen/scenarigo v0.11.2

replace github.com/zoncoen/scenarigo v0.11.2 => github.com/zoncoen/scenarigo v0.11.0
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
			expect: `module plugin_module

go 1.21
`,
			expectStdout: `WARN: test.so: remove replace github.com/zoncoen/scenarigo v0.11.2 => github.com/zoncoen/scenarigo v0.11.0
`,
		},
		"add require": {
			gomod: `module plugin_module

go 1.21
`,
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
			expect: `module plugin_module

go 1.21

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.5.0 // indirect
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240227224415-6ceb2ff114de // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)
`,
			expectStdout: `WARN: test.so: change require google.golang.org/grpc v1.63.2 ==> v1.37.1 by test
`,
		},
		"overwrite require by require": {
			gomod: `module plugin_module

go 1.21

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`,
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
			expect: `module plugin_module

go 1.21

require google.golang.org/grpc v1.40.0

require (
	github.com/golang/protobuf v1.4.3 // indirect
	golang.org/x/net v0.0.0-20200822124328-c89045814202 // indirect
	golang.org/x/sys v0.0.0-20200323222414-85ca7c5b95cd // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`,
			expectStdout: `WARN: test.so: change require google.golang.org/grpc v1.37.1 ==> v1.40.0 by test
`,
		},
		"overwrite require by replace": {
			gomod: `module plugin_module

go 1.21

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`,
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
			expect: `module plugin_module

go 1.21

require google.golang.org/grpc v1.40.0

require (
	github.com/golang/protobuf v1.4.3 // indirect
	golang.org/x/net v0.0.0-20200822124328-c89045814202 // indirect
	golang.org/x/sys v0.0.0-20200323222414-85ca7c5b95cd // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`,
			expectStdout: `WARN: test.so: change require google.golang.org/grpc v1.37.1 ==> v1.40.0 by test
`,
		},
		"do nothing (same version)": {
			gomod: `module plugin_module

go 1.21

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`,
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
			expect: `module plugin_module

go 1.21

require google.golang.org/grpc v1.37.1

require (
	github.com/golang/protobuf v1.4.2 // indirect
	golang.org/x/net v0.0.0-20190311183353-d8887717615a // indirect
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a // indirect
	golang.org/x/text v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)
`,
		},
		"add replace": {
			gomod: `module plugin_module

go 1.21

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
`,
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
							Version: "v1.40.0",
						},
					},
					requiredBy: "test",
				},
			},
			expect: `module plugin_module

go 1.21

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
`,
			expectStdout: `WARN: test.so: add replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0 by test
`,
		},
		"overwrite replace by require": {
			gomod: `module plugin_module

go 1.21

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
`,
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
							Version: "v1.40.1",
						},
					},
					requiredBy: "test",
				},
			},
			expect: `module plugin_module

go 1.21

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
`,
			expectStdout: `WARN: test.so: change replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0 ==> google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.1 by test
`,
		},
		"override replace by replace": {
			gomod: `module plugin_module

go 1.21

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
`,
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
			expect: `module plugin_module

go 1.21

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
`,
			expectStdout: `WARN: test.so: change replace google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.0 ==> google.golang.org/grpc v1.46.0 => google.golang.org/grpc v1.40.1 by test
`,
		},
		"do nothing (alredy replaced)": {
			gomod: `module plugin_module

go 1.21

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
`,
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
							Version: "v1.40.0",
						},
					},
					requiredBy: "test",
				},
			},
			expect: `module plugin_module

go 1.21

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
`,
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
			goCmd, err := findGoCmd(ctx, false)
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
			if err := updateGoMod(cmd, goCmd, "test.so", gomod, overrideKeys, test.overrides); err != nil {
				t.Fatalf("failed to update go.mod: %s", err)
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
		if err := executeWithEnvs(ctx, envs, tempDir, goCmd, "clean", "-modcache"); err != nil {
			t.Errorf("go clean -modcache failed: %s", err)
		}
	})

	// create git objects for test repositories
	if err := executeWithEnvs(ctx, envs, tempDir, "git", "config", "--global", "user.name", "scenarigo-test"); err != nil {
		t.Fatalf("git config failed: %s", err)
	}
	if err := executeWithEnvs(ctx, envs, tempDir, "git", "config", "--global", "user.email", "scenarigo-test@example.com"); err != nil {
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
			if err := executeWithEnvs(ctx, envs, wd, goCmd, "mod", "tidy"); err != nil {
				t.Fatalf("go mod tidy failed: %s", err)
			}
			t.Cleanup(func() {
				os.RemoveAll(filepath.Join(wd, "go.sum"))
			})
		}
		if _, err := os.Stat(filepath.Join(wd, "v2", "go.mod")); err == nil {
			if err := executeWithEnvs(ctx, envs, filepath.Join(wd, "v2"), goCmd, "mod", "tidy"); err != nil {
				t.Fatalf("go mod tidy failed: %s", err)
			}
			t.Cleanup(func() {
				os.RemoveAll(filepath.Join(wd, "v2", "go.sum"))
			})
		}
		if err := executeWithEnvs(ctx, envs, wd, "git", "init"); err != nil {
			t.Fatalf("git init failed: %s", err)
		}
		t.Cleanup(func() {
			os.RemoveAll(filepath.Join(wd, ".git"))
		})
		if err := executeWithEnvs(ctx, envs, wd, "git", "add", "-A"); err != nil {
			t.Fatalf("git add failed: %s", err)
		}
		if err := executeWithEnvs(ctx, envs, wd, "git", "commit", "-m", "commit"); err != nil {
			t.Fatalf("git commit failed: %s", err)
		}
		if err := executeWithEnvs(ctx, envs, wd, "git", "tag", "v1.0.0"); err != nil {
			t.Fatalf("git tag failed: %s", err)
		}
		if err := executeWithEnvs(ctx, envs, wd, "git", "tag", "v1.1.0"); err != nil {
			t.Fatalf("git tag failed: %s", err)
		}
		if _, err := os.Stat(filepath.Join(wd, "v2")); err == nil {
			if err := executeWithEnvs(ctx, envs, wd, "git", "tag", "v2.0.0"); err != nil {
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
