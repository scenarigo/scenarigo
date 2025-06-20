package plugin

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	goversion "go/version"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"io"

	"github.com/scenarigo/scenarigo"
	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/scenarigo/scenarigo/internal/filepathutil"
	"github.com/scenarigo/scenarigo/schema"
	"github.com/scenarigo/scenarigo/version"
)

const (
	versionTooHighErrorPattern = `^go: go.mod requires go >= ([\d\.]+) .+$`
	toolchainLocal             = "local"
	oldScenarigoModPath        = "github.com/zoncoen/scenarigo"
	newScenarigoModPath        = "github.com/scenarigo/scenarigo"
)

var (
	goVer                     string
	toolchain                 string
	goMinVer                  string
	versionTooHighErrorRegexp *regexp.Regexp
)

func init() {
	goVer, toolchain = parseGoVersion(runtime.Version())
	goMinVer = "1.21.2"
	versionTooHighErrorRegexp = regexp.MustCompile(versionTooHighErrorPattern)
}

func parseGoVersion(ver string) (string, string) {
	tc := ver
	// gotip
	// e.g., go1.25-devel_dad4f399 Tue May 6 13:41:19 2025 -0700
	if v, ok := isGotip(ver); ok {
		ver = v
		tc = toolchainLocal
	}
	// workaround for weird environments (e.g., go1.23.2 X:rangefunc)
	if !goversion.IsValid(ver) {
		if v := strings.Split(ver, " ")[0]; goversion.IsValid(v) {
			ver = v
			tc = v
		} else {
			tc = toolchainLocal
		}
	}
	return ver, tc
}

func isGotip(v string) (string, bool) {
	if strings.Contains(v, "devel") {
		return strings.Split(v, "-")[0], true
	}
	return "", false
}

var (
	verbose       bool
	skipMigration bool
	wasm          bool
)

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "build plugins",
		Long: strings.Trim(`
Builds plugins based on the configuration file.

This command requires go command in $PATH.
`, "\n"),
		Args:          cobra.ExactArgs(0),
		RunE:          buildRun,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print verbose log")
	cmd.Flags().BoolVarP(&skipMigration, "skip-migration", "", false, "skip migration")
	cmd.Flags().BoolVarP(&wasm, "wasm", "", false, "build as WebAssembly")
	return cmd
}

var (
	warnColor  = color.New(color.Bold, color.FgYellow)
	debugColor = color.New(color.Bold)
)

type retriableError struct {
	reason string
}

func (e *retriableError) Error() string {
	return fmt.Sprintf("retriable error: %s", e.reason)
}

type overrideModule struct {
	require          *modfile.Require
	requiredBy       string
	replace          *modfile.Replace
	replacedBy       string
	replaceLocalPath string
	force            bool
}

func (o *overrideModule) requireReplace() (*modfile.Require, string, *modfile.Replace, string) {
	if o.replace != nil {
		if o.require == nil || o.replace.Old.Path == o.replace.New.Path {
			return &modfile.Require{
				Mod:      o.replace.New,
				Indirect: false,
				Syntax:   nil,
			}, o.replacedBy, nil, ""
		}
	}
	return o.require, o.requiredBy, o.replace, o.replacedBy
}

type buildOpts struct {
	skipMigration bool
	wasm          bool
}

func buildRun(cmd *cobra.Command, args []string) error {
	return buildRunWithOpts(cmd, args, &buildOpts{
		skipMigration: skipMigration,
		wasm:          wasm,
	})
}

func buildRunWithOpts(cmd *cobra.Command, args []string, opts *buildOpts) error {
	runtimeVersion := runtime.Version()
	debugLog(cmd, "scenarigo was built with %s", runtimeVersion)
	if !goversion.IsValid(goVer) {
		warnLog(cmd, "failed to parse the Go version that built scenarigo: %s", runtimeVersion)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		return errors.New("config file not found")
	}

	goCmd, err := findGoCmd(ctx(cmd))
	if err != nil {
		return err
	}
	debugLog(cmd, "found go command: %s", goCmd)
	debugLog(cmd, "set GOTOOLCHAIN=%s", toolchain)

	pbs := make([]*pluginBuilder, 0, cfg.Plugins.Len())
	pluginModules := map[string]*overrideModule{}
	pluginDir := filepathutil.From(cfg.Root, cfg.PluginDirectory)
	for _, item := range cfg.Plugins.ToSlice() {
		pb, clean, err := createPluginBuilder(cmd, goCmd, pluginModules, cfg.Root, pluginDir, item, opts)
		defer clean()
		if err != nil {
			return err
		}
		pbs = append(pbs, pb)
	}

	goworkPath, err := checkGowork(ctx(cmd), goCmd, pbs)
	if err != nil {
		return fmt.Errorf("failed to build plugin: %w", err)
	}

	var gowork []byte
	if goworkPath != "" {
		b, err := os.ReadFile(goworkPath)
		if err != nil {
			return fmt.Errorf("failed to build plugin: %w", err)
		}
		gowork = b
	}

	var overrides map[string]*overrideModule
	var retriableErrors []string
	for {
		overrides, err = selectUnifiedVersions(pbs)
		if err != nil {
			return fmt.Errorf("failed to build plugin: %w", err)
		}

		for m, o := range pluginModules {
			overrides[m] = &overrideModule{
				require:    o.require,
				requiredBy: o.requiredBy,
				force:      true,
			}
		}

		requires, err := requiredModulesByScenarigo()
		if err != nil {
			return err
		}
		for _, r := range requires {
			overrides[r.Mod.Path] = &overrideModule{
				require:    r,
				requiredBy: "scenarigo",
				force:      true,
			}
		}

		overrideKeys := make([]string, 0, len(overrides))
		for k := range overrides {
			overrideKeys = append(overrideKeys, k)
		}
		sort.Strings(overrideKeys)

		var retry bool
		for _, pb := range pbs {
			if err := pb.build(cmd, goCmd, overrideKeys, overrides, goworkPath, gowork, opts); err != nil {
				var re *retriableError
				if errors.As(err, &re) {
					msg := fmt.Sprintf("%s: %s", pb.name, err)
					if slices.Contains(retriableErrors, msg) {
						return fmt.Errorf("failed to build plugin %s: failed to unify the module version: %w", pb.name, err)
					}
					retriableErrors = append(retriableErrors, msg)
					retry = true
				} else {
					return fmt.Errorf("failed to build plugin %s: %w", pb.name, err)
				}
			}
		}
		if !retry {
			break
		}
	}

	for _, pb := range pbs {
		if err := pb.printUpdatedResult(cmd, goCmd, pb.name, pb.gomodPath, overrides); err != nil {
			return err
		}
	}

	return nil
}

func findGoCmd(ctx context.Context) (string, error) {
	if goCmd := os.Getenv("SCENARIGO_GO"); goCmd != "" {
		return goCmd, nil
	}
	goCmd, err := exec.LookPath("go")
	if err != nil {
		return "", fmt.Errorf("go command required: %w", err)
	}
	if err := checkGoVersion(ctx, goCmd, goMinVer); err != nil {
		return "", fmt.Errorf("failed to check go version: %w", err)
	}
	return goCmd, nil
}

// 2nd return value should always be called.
func createPluginBuilder(cmd *cobra.Command, goCmd string, pluginModules map[string]*overrideModule, root, pluginDir string, item schema.OrderedMapItem[string, schema.PluginConfig], opts *buildOpts) (*pluginBuilder, func(), error) {
	out := item.Key

	// Check if plugin name has .wasm suffix and enable WASM build automatically
	if strings.HasSuffix(out, ".wasm") && !opts.wasm {
		debugLog(cmd, "detected .wasm suffix in plugin name %s, enabling WASM build", out)
		opts.wasm = true
	}

	mod := filepathutil.From(root, item.Value.Src)
	var src string
	clean := func() {}
	if _, err := os.Stat(mod); err != nil {
		m, s, r, err := downloadModule(ctx(cmd), goCmd, item.Value.Src)
		if err != nil {
			return nil, clean, fmt.Errorf("failed to build plugin %s: %w", out, err)
		}
		debugLog(cmd, "download %s into %s", item.Value.Src, m)

		m, f, err := copyModule(cmd, m)
		clean = f
		if err != nil {
			return nil, clean, fmt.Errorf("failed to build plugin %s: %w", out, err)
		}
		debugLog(cmd, "copy %s into %s", item.Value.Src, m)

		mod = m
		src = s
		pluginModules[r.Mod.Path] = &overrideModule{
			require:    r,
			requiredBy: out,
		}
	}
	// NOTE: All module names must be unique and different from the standard modules.
	defaultModName := filepath.Join("plugins", strings.TrimSuffix(out, filepath.Ext(out)))
	pb, err := newPluginBuilder(cmd, goCmd, out, mod, src, filepathutil.From(pluginDir, out), defaultModName)
	if err != nil {
		return nil, clean, fmt.Errorf("failed to build plugin %s: %w", out, err)
	}

	if !opts.skipMigration {
		// replace zoncoen/scenarigo to scenarigo/scenarigo
		if _, ok := pb.initialRequires[oldScenarigoModPath]; ok {
			debugLog(cmd, "replace %s => %s in %s", oldScenarigoModPath, newScenarigoModPath, item.Value.Src)

			fset := token.NewFileSet()
			if err := filepath.Walk(pb.dir, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() || filepath.Ext(info.Name()) != ".go" {
					return nil
				}
				b, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("failed to read file: %w", err)
				}
				f, err := parser.ParseFile(fset, info.Name(), b, parser.ParseComments)
				if err != nil {
					return fmt.Errorf("failed to parse file: %w", err)
				}
				var (
					inspErr error
					found   bool
				)
				ast.Inspect(f, func(n ast.Node) bool {
					if x, ok := n.(*ast.ImportSpec); ok {
						p, err := strconv.Unquote(x.Path.Value)
						if err != nil {
							inspErr = err
							return false
						}
						if strings.HasPrefix(p, oldScenarigoModPath) {
							found = true
							x.Path.Value = strconv.Quote(strings.Replace(p, oldScenarigoModPath, newScenarigoModPath, 1))
						}
					}
					return true
				})
				if inspErr != nil {
					return fmt.Errorf("failed to modify import path: %w", err)
				}
				if !found {
					return nil
				}
				fd, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, info.Mode())
				if err != nil {
					return fmt.Errorf("failed to open file: %w", err)
				}
				if err := format.Node(fd, fset, f); err != nil {
					return fmt.Errorf("failed to modify import path: %w", err)
				}
				return nil
			}); err != nil {
				return nil, clean, err
			}

			if err := pb.editGoMod(cmd, goCmd, func(gomod *modfile.File) error {
				if err := gomod.DropRequire(oldScenarigoModPath); err != nil {
					return fmt.Errorf("%s: %w", pb.gomodPath, err)
				}
				v := version.String()
				if strings.HasSuffix(v, "-dev") {
					v = "latest"
				}
				if err := gomod.AddRequire(newScenarigoModPath, v); err != nil {
					return fmt.Errorf("%s: %w", pb.gomodPath, err)
				}
				return nil
			}); err != nil {
				return nil, clean, fmt.Errorf("failed to edit require directive: %w", err)
			}
		}
	}

	return pb, clean, nil
}

func checkGowork(ctx context.Context, goCmd string, pbs []*pluginBuilder) (string, error) {
	env := os.Getenv("GOWORK")
	// prioritize explicit config
	// ref. https://go.dev/blog/get-familiar-with-workspaces#workspace-commands
	if env != "" {
		if env == "off" {
			return "", nil
		}
		if strings.HasSuffix(env, ".work") {
			return env, nil
		}
	}

	files := []goworkConfig{}
	for _, pb := range pbs {
		gowork, err := execute(ctx, pb.dir, goCmd, "env", "GOWORK")
		if err != nil {
			return "", fmt.Errorf("failed to build plugin %s: %w", pb.out, err)
		}
		gowork = strings.Trim(gowork, "\n")
		if gowork != "" {
			files = append(files, goworkConfig{pb.out, gowork})
		}
	}
	if len(files) == 0 {
		return "", nil
	}

	var gowork string
	for _, f := range files {
		if gowork != "" && gowork != f.gowork {
			buf := bytes.NewBufferString("found multiple workspace files\n")
			for _, f := range files {
				fmt.Fprintf(buf, "    %s:\n      %s\n", f.out, f.gowork)
			}
			return "", errors.New(buf.String())
		}
		gowork = f.gowork
	}
	return gowork, nil
}

type goworkConfig struct {
	out    string
	gowork string
}

func selectUnifiedVersions(pbs []*pluginBuilder) (map[string]*overrideModule, error) {
	overrides := map[string]*overrideModule{}
	for _, pb := range pbs {
		// maximum version selection
		for _, r := range pb.gomod.Require {
			o, ok := overrides[r.Mod.Path]
			if !ok {
				overrides[r.Mod.Path] = &overrideModule{
					require:    r,
					requiredBy: pb.name,
				}
				continue
			}
			if compareVers(o.require.Mod.Version, r.Mod.Version) < 0 {
				overrides[r.Mod.Path].require = r
				overrides[r.Mod.Path].requiredBy = pb.name
			}
		}
		for _, r := range pb.gomod.Replace {
			var localPath string
			if r.New.Version == "" {
				// already checked that the path exists by "go mod tidy"
				localPath = filepathutil.From(filepath.Dir(pb.gomodPath), r.New.Path)
			}
			o, ok := overrides[r.Old.Path]
			if !ok {
				overrides[r.Old.Path] = &overrideModule{
					replace:          r,
					replacedBy:       pb.name,
					replaceLocalPath: localPath,
				}
				continue
			}
			if o.replace != nil {
				if o.replace.New.Path != r.New.Path || o.replace.New.Version != r.New.Version {
					if (localPath == "" && o.replaceLocalPath == "") || localPath != o.replaceLocalPath {
						return nil, fmt.Errorf("%s: replace %s directive conflicts: %s => %s, %s => %s", pb.name, r.Old.Path, o.replacedBy, replacePathVersion(o.replace.New.Path, o.replace.New.Version), pb.name, replacePathVersion(r.New.Path, r.New.Version))
					}
				}
			}
			o.replace = r
			o.replacedBy = pb.name
			o.replaceLocalPath = localPath
			if localPath != "" {
				o.force = true
			}
			overrides[r.Old.Path] = o
		}
	}
	return overrides, nil
}

func ctx(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func replacePathVersion(p, v string) string {
	if v == "" {
		return p
	}
	return fmt.Sprintf("%s %s", p, v)
}

func checkGoVersion(ctx context.Context, goCmd, minVer string) error {
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, goCmd, "version")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return err
	}
	items := strings.Split(stdout.String(), " ")
	if len(items) != 4 {
		if len(items) > 4 {
			// gotip
			// e.g., go version go1.25-devel_30b2b76 Tue May 6 09:59:00 2025 -0700 darwin/arm64
			if v, ok := isGotip(items[2]); ok {
				items[2] = v
			} else {
				return fmt.Errorf("invalid version output or scenarigo bug: %s", stdout.String())
			}
		} else {
			return fmt.Errorf("invalid version output or scenarigo bug: %s", stdout.String())
		}
	}
	ver := strings.TrimPrefix(items[2], "go")
	if compareVers(ver, minVer) == -1 {
		return fmt.Errorf(`required go %s or later but installed %s`, minVer, ver)
	}
	return nil
}

func downloadModule(ctx context.Context, goCmd, p string) (string, string, *modfile.Require, error) {
	tempDir, err := os.MkdirTemp("", "scenarigo-plugin-gomod-")
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create a temporary directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	if _, err := execute(ctx, tempDir, goCmd, "mod", "init", "download_module"); err != nil {
		return "", "", nil, fmt.Errorf("failed to initialize go.mod: %w", err)
	}
	if _, err := execute(ctx, tempDir, goCmd, downloadCmd(p)...); err != nil {
		return "", "", nil, fmt.Errorf("failed to download %s: %w", p, err)
	}
	mod, src, req, err := modSrcPath(tempDir, p)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to get module path: %w", err)
	}

	return mod, src, req, nil
}

func modSrcPath(tempDir, mod string) (string, string, *modfile.Require, error) {
	if i := strings.Index(mod, "@"); i >= 0 { // trim version
		mod = mod[:i]
	}
	b, err := os.ReadFile(filepath.Join(tempDir, "go.mod"))
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to read file: %w", err)
	}
	gomod, err := modfile.Parse("go.mod", b, nil)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to parse go.mod: %w", err)
	}
	parts := strings.Split(mod, "/")
	for i := len(parts); i > 1; i-- {
		m := strings.Join(parts[:i], "/")
		for _, r := range gomod.Require {
			if r.Mod.Path == m {
				p, err := module.EscapePath(r.Mod.Path)
				if err != nil {
					return "", "", nil, fmt.Errorf("failed to escape module path %s: %w", r.Mod.Path, err)
				}
				return filepath.Join(
					build.Default.GOPATH, "pkg", "mod",
					fmt.Sprintf("%s@%s", p, r.Mod.Version),
				), filepath.Join(parts[i:]...), r, nil
			}
		}
	}
	return "", "", nil, errors.New("module not found on go.mod")
}

func copyModule(cmd *cobra.Command, mod string) (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "scenarigo-plugin-")
	if err != nil {
		return "", func() {}, fmt.Errorf("failed to create a temporary directory: %w", err)
	}
	clean := func() {
		os.RemoveAll(tempDir)
	}

	if err := os.CopyFS(tempDir, os.DirFS(mod)); err != nil {
		return "", clean, fmt.Errorf("failed to create a temporary directory: %w", err)
	}

	return tempDir, clean, nil
}

type pluginBuilder struct {
	name            string
	dir             string
	src             string
	gomodPath       string
	gomod           *modfile.File
	initialRequires map[string]modfile.Require
	initialReplaces map[string]modfile.Replace
	out             string
}

func newPluginBuilder(cmd *cobra.Command, goCmd, name, mod, src, out, defaultModName string) (*pluginBuilder, error) {
	ctx := ctx(cmd)
	dir := mod
	info, err := os.Stat(mod)
	if err != nil {
		return nil, fmt.Errorf("failed to find plugin src %s: %w", mod, err)
	}
	if !info.IsDir() {
		dir, src = filepath.Split(mod)
	}
	src = fmt.Sprintf(".%c%s", filepath.Separator, src) // modify the path to explicit relative

	gomodPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(gomodPath); err != nil {
		if _, err := execute(ctx, dir, goCmd, "mod", "init"); err != nil {
			// ref. https://github.com/golang/go/wiki/Modules#why-does-go-mod-init-give-the-error-cannot-determine-module-path-for-source-directory
			if strings.Contains(err.Error(), "cannot determine module path") {
				if _, err := execute(ctx, dir, goCmd, "mod", "init", defaultModName); err != nil {
					return nil, fmt.Errorf("failed to initialize go.mod: %w", err)
				}
			} else {
				return nil, fmt.Errorf("failed to initialize go.mod: %w", err)
			}
		}
	}

	if err := modTidy(cmd, dir, goCmd); err != nil {
		if ok, verr := asVersionTooHighError(err); ok {
			err = fmt.Errorf("re-install scenarigo command with go%s: %w", verr.requiredVersion, err)
		}
		return nil, err
	}

	b, err := os.ReadFile(gomodPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", gomodPath, err)
	}
	gomod, err := modfile.Parse(gomodPath, b, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", gomodPath, err)
	}

	initialRequires, initialReplaces := getInitialState(gomod)

	return &pluginBuilder{
		name:            name,
		dir:             dir,
		src:             src,
		gomodPath:       gomodPath,
		gomod:           gomod,
		initialRequires: initialRequires,
		initialReplaces: initialReplaces,
		out:             out,
	}, nil
}

func modTidy(cmd *cobra.Command, dir, goCmd string) error {
	ctx := ctx(cmd)

	if tidyCmd := os.Getenv("GO_MOD_TIDY"); tidyCmd != "" {
		if _, err := execute(ctx, dir, goCmd, strings.Split(tidyCmd, " ")...); err != nil {
			return err
		}
		return nil
	}

	// FIXME: workaround for CI
	// sometimes "go mod tidy" fail on CI w/o error message
	var err error
	var retry uint64
	if e := os.Getenv("SCENARIGO_GO_MOD_TIDY_RETRY"); e != "" {
		retry, err = strconv.ParseUint(e, 0, 64)
		if err != nil {
			return fmt.Errorf("invalid SCENARIGO_GO_MOD_TIDY_RETRY: %w", err)
		}
	}
	for i := uint64(0); i <= retry; i++ {
		_, err = execute(ctx, dir, goCmd, "mod", "tidy")
		if err == nil {
			return nil
		}
		if i != retry {
			warnLog(cmd, `"go mod tidy" failed, retry...`)
		}
		if i > 0 && err.Error() == "" {
			// attempted retries and got no output
			// ignore the error to avoid failure on macOS CI
			return nil
		}
	}
	return fmt.Errorf(`%s: "go mod tidy" failed: %w`, dir, err)
}

func getInitialState(gomod *modfile.File) (map[string]modfile.Require, map[string]modfile.Replace) {
	initialRequires := map[string]modfile.Require{}
	for _, r := range gomod.Require {
		initialRequires[r.Mod.Path] = *r
	}
	initialReplaces := map[string]modfile.Replace{}
	for _, r := range gomod.Replace {
		initialReplaces[r.Old.Path] = *r
	}
	return initialRequires, initialReplaces
}

func (pb *pluginBuilder) build(cmd *cobra.Command, goCmd string, overrideKeys []string, overrides map[string]*overrideModule, goworkPath string, gowork []byte, opts *buildOpts) error {
	ctx := ctx(cmd)

	if err := pb.updateGoMod(cmd, goCmd, overrideKeys, overrides); err != nil {
		return err
	}
	if err := os.RemoveAll(pb.out); err != nil {
		return fmt.Errorf("failed to delete the old plugin %s: %w", pb.out, err)
	}
	var envs []string
	if goworkPath == "" {
		envs = append(envs, "GOWORK=off")
	} else {
		envs = append(envs, fmt.Sprintf("GOWORK=%s", goworkPath))
		defer func() {
			// restore go.work
			f, err := os.OpenFile(goworkPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err == nil {
				defer f.Close()
				_, _ = f.Write(gowork)
			}
		}()
		if _, err := executeWithEnvs(ctx, envs, pb.dir, goCmd, "work", "use", "."); err != nil {
			return fmt.Errorf(`"go work use ." failed: %w`, err)
		}
	}

	if opts != nil && opts.wasm {
		// Extract symbols & generate main.go
		symbols, err := extractExportedSymbols(pb.dir)
		if err != nil {
			return fmt.Errorf("failed to extract exported symbols: %w", err)
		}
		mainPath, err := createWasmMainFile(pb.dir, symbols)
		if err != nil {
			return fmt.Errorf("failed to create wasm.go: %w", err)
		}
		defer func() {
			os.Remove(mainPath)
		}()

		// Create overlay files for net package replacement
		overlayFiles, err := createNetPackageOverlay(ctx, goCmd)
		if err != nil {
			return fmt.Errorf("failed to create net package overlay: %w", err)
		}
		defer func() {
			for _, file := range overlayFiles {
				os.Remove(file)
			}
		}()

		// Create overlay JSON
		overlayJSON, err := createOverlayJSON(overlayFiles)
		if err != nil {
			return fmt.Errorf("failed to create overlay JSON: %w", err)
		}
		defer os.Remove(overlayJSON)

		envs = append(envs, "GOOS=wasip1", "GOARCH=wasm")
		if _, err := executeWithEnvs(ctx, envs, pb.dir, goCmd, "build", "-overlay", overlayJSON, "-o", pb.out); err != nil {
			return fmt.Errorf(`"go build -overlay %s -o %s" failed: %w`, overlayJSON, pb.out, err)
		}
		return nil
	}

	if _, err := executeWithEnvs(ctx, envs, pb.dir, goCmd, "build", "-buildmode=plugin", "-o", pb.out, pb.src); err != nil {
		return fmt.Errorf(`"go build -buildmode=plugin -o %s %s" failed: %w`, pb.out, pb.src, err)
	}
	return nil
}

func execute(ctx context.Context, wd, name string, args ...string) (string, error) {
	return executeWithEnvs(ctx, nil, wd, name, args...)
}

func executeWithEnvs(ctx context.Context, envs []string, wd, name string, args ...string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	envs = append(envs, fmt.Sprintf("GOTOOLCHAIN=%s", toolchain))
	cmd.Env = append(os.Environ(), envs...)
	if wd != "" {
		cmd.Dir = wd
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", wrapVersionTooHighError(errors.New(strings.TrimSuffix(stderr.String(), "\n")))
	}
	return stdout.String(), nil
}

func (pb *pluginBuilder) updateGoMod(cmd *cobra.Command, goCmd string, overrideKeys []string, overrides map[string]*overrideModule) error {
	if err := pb.editGoMod(cmd, goCmd, func(gomod *modfile.File) error {
		if toolchain == toolchainLocal {
			if gomod.Toolchain != nil {
				warnLog(cmd, "%s: remove toolchain by scenarigo", pb.name)
				gomod.DropToolchainStmt()
			}
			return nil
		}

		switch compareVers(gomod.Go.Version, toolchain) {
		case -1: // go.mod < scenarigo go version
			if gomod.Toolchain == nil {
				warnLog(cmd, "%s: add toolchain %s by scenarigo", pb.name, toolchain)
			} else if gomod.Toolchain.Name != toolchain {
				warnLog(cmd, "%s: change toolchain %s ==> %s by scenarigo", pb.name, gomod.Toolchain.Name, toolchain)
			}
			if err := gomod.AddToolchainStmt(toolchain); err != nil {
				return fmt.Errorf("%s: %w", pb.gomodPath, err)
			}
		case 1: // go.mod > scenarigo go version
			return fmt.Errorf("%s: go: go.mod requires go >= %s (scenarigo was built with %s)", pb.gomodPath, gomod.Go.Version, toolchain)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to edit toolchain directive: %w", err)
	}

	if err := pb.updateRequireDirectives(cmd, goCmd, overrides); err != nil {
		return err
	}
	if err := pb.updateReplaceDirectives(cmd, goCmd, overrides, overrideKeys); err != nil {
		return err
	}
	return nil
}

func (pb *pluginBuilder) updateRequireDirectives(cmd *cobra.Command, goCmd string, overrides map[string]*overrideModule) error {
	if err := pb.editGoMod(cmd, goCmd, func(gomod *modfile.File) error {
		for _, o := range overrides {
			require, _, _, _ := o.requireReplace()
			if err := gomod.AddRequire(require.Mod.Path, require.Mod.Version); err != nil {
				return fmt.Errorf("%s: %w", pb.gomodPath, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to edit require directive: %w", err)
	}
	return nil
}

func (pb *pluginBuilder) updateReplaceDirectives(cmd *cobra.Command, goCmd string, overrides map[string]*overrideModule, overrideKeys []string) error {
	if err := pb.editGoMod(cmd, goCmd, func(gomod *modfile.File) error {
		requires := map[string]string{}
		for _, r := range gomod.Require {
			requires[r.Mod.Path] = r.Mod.Version
		}
		replaces := map[string]modfile.Replace{}
		for _, r := range gomod.Replace {
			if _, ok := requires[r.Old.Path]; !ok {
				if err := gomod.DropReplace(r.Old.Path, r.Old.Version); err != nil {
					return fmt.Errorf("%s: %w", pb.gomodPath, err)
				}
				continue
			}
			replaces[r.Old.Path] = *r
		}
		for _, k := range overrideKeys {
			o := overrides[k]

			require, _, replace, _ := o.requireReplace()
			if v, ok := replaces[require.Mod.Path]; ok {
				if err := gomod.DropReplace(require.Mod.Path, v.Old.Version); err != nil {
					return fmt.Errorf("%s: %w", pb.gomodPath, err)
				}
				if o.replace != nil && !o.force && v.Old.Version != o.replace.Old.Version {
					return &retriableError{
						reason: fmt.Sprintf("change the replaced old version of %s from %s to %s", o.replace.Old.Path, o.replace.Old.Version, v.Old.Version),
					}
				}
			}
			if replace != nil {
				if v, ok := requires[replace.Old.Path]; ok {
					path := replace.New.Path
					if o.replaceLocalPath != "" {
						rel, err := filepath.Rel(filepath.Dir(pb.gomodPath), o.replaceLocalPath)
						if err != nil {
							return fmt.Errorf("%s: %w", pb.gomodPath, err)
						}
						// must be rooted or staring with ./ or ../
						if sep := string(filepath.Separator); !strings.Contains(rel, sep) {
							rel += sep
						}
						path = rel
					}
					if err := gomod.AddReplace(replace.Old.Path, v, path, replace.New.Version); err != nil {
						return fmt.Errorf("%s: %w", pb.gomodPath, err)
					}
				}
			} else {
				if v, ok := requires[require.Mod.Path]; ok {
					switch compareVers(require.Mod.Version, v) {
					case -1: // require.Mod.Version < v
						if !o.force {
							if o.replace == nil {
								return &retriableError{
									reason: fmt.Sprintf("change the maximum version of %s from %s to %s", require.Mod.Path, require.Mod.Version, v),
								}
							}
						}
					case 0: // require.Mod.Version == v
						continue
					}
					if err := gomod.AddReplace(require.Mod.Path, v, require.Mod.Path, require.Mod.Version); err != nil {
						return fmt.Errorf("%s: %w", pb.gomodPath, err)
					}
				}
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to edit replace directive: %w", err)
	}
	return nil
}

type requireDiff struct {
	old modfile.Require
	new modfile.Require
}

type replaceDiff struct {
	old modfile.Replace
	new modfile.Replace
}

func (pb *pluginBuilder) printUpdatedResult(cmd *cobra.Command, goCmd, name, gomodPath string, overrides map[string]*overrideModule) error {
	gomod, err := parseGoMod(cmd, goCmd, gomodPath)
	if err != nil {
		return err
	}
	printUpdatedRequires(cmd, name, overrides, pb.initialRequires, gomod)
	printUpdatedReplaces(cmd, name, overrides, pb.initialReplaces, gomod)
	return nil
}

func printUpdatedRequires(cmd *cobra.Command, name string, overrides map[string]*overrideModule, initialRequires map[string]modfile.Require, gomod *modfile.File) {
	requireKeys := []string{}
	requireDiffs := map[string]*requireDiff{}
	for _, r := range initialRequires {
		requireKeys = append(requireKeys, r.Mod.Path)
		requireDiffs[r.Mod.Path] = &requireDiff{
			old: r,
		}
	}
	for _, r := range gomod.Require {
		diff, ok := requireDiffs[r.Mod.Path]
		if ok {
			diff.new = *r
		} else {
			requireKeys = append(requireKeys, r.Mod.Path)
			requireDiffs[r.Mod.Path] = &requireDiff{
				new: *r,
			}
		}
	}
	sort.Strings(requireKeys)

	for _, k := range requireKeys {
		diff := requireDiffs[k]
		switch {
		case diff.old.Mod.Path == "":
			if !diff.new.Indirect {
				if o := overrides[k]; o != nil {
					_, requiredBy, _, _ := o.requireReplace()
					warnLog(cmd, "%s: add require %s %s by %s", name, k, diff.new.Mod.Version, requiredBy)
				} else {
					warnLog(cmd, "%s: add require %s %s", name, k, diff.new.Mod.Version)
				}
			}
		case diff.new.Mod.Path == "":
			if !diff.old.Indirect {
				warnLog(cmd, "%s: remove require %s %s", name, k, diff.old.Mod.Version)
			}
		case diff.old.Mod.Version != diff.new.Mod.Version:
			if !diff.old.Indirect || !diff.new.Indirect {
				if o := overrides[k]; o != nil {
					_, requiredBy, _, _ := o.requireReplace()
					warnLog(cmd, "%s: change require %s %s ==> %s by %s", name, k, diff.old.Mod.Version, diff.new.Mod.Version, requiredBy)
				} else {
					warnLog(cmd, "%s: change require %s %s ==> %s", name, k, diff.old.Mod.Version, diff.new.Mod.Version)
				}
			}
		}
	}
}

func printUpdatedReplaces(cmd *cobra.Command, name string, overrides map[string]*overrideModule, initialReplaces map[string]modfile.Replace, gomod *modfile.File) {
	replaceKeys := []string{}
	replaceDiffs := map[string]*replaceDiff{}
	for _, r := range initialReplaces {
		replaceKeys = append(replaceKeys, r.Old.Path)
		replaceDiffs[r.Old.Path] = &replaceDiff{
			old: r,
		}
	}
	for _, r := range gomod.Replace {
		diff, ok := replaceDiffs[r.Old.Path]
		if ok {
			diff.new = *r
		} else {
			replaceKeys = append(replaceKeys, r.Old.Path)
			replaceDiffs[r.Old.Path] = &replaceDiff{
				new: *r,
			}
		}
	}
	sort.Strings(replaceKeys)

	for _, k := range replaceKeys {
		diff := replaceDiffs[k]
		switch {
		case diff.old.Old.Path == "":
			if o := overrides[k]; o != nil {
				_, by, replace, replaceBy := o.requireReplace()
				if replace != nil {
					by = replaceBy
				}
				warnLog(cmd, "%s: add replace %s => %s by %s", name, replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version), by)
			} else {
				warnLog(cmd, "%s: add replace %s => %s", name, replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version))
			}
		case diff.new.Old.Path == "":
			warnLog(cmd, "%s: remove replace %s => %s", name, replacePathVersion(k, diff.old.Old.Version), replacePathVersion(diff.old.New.Path, diff.old.New.Version))
		case diff.old.New.Path != diff.new.New.Path || diff.old.New.Version != diff.new.New.Version:
			if o := overrides[k]; o != nil {
				_, by, replace, replaceBy := o.requireReplace()
				if replace != nil {
					by = replaceBy
				}
				warnLog(cmd, "%s: change replace %s => %s ==> %s => %s by %s", name, replacePathVersion(k, diff.old.Old.Version), replacePathVersion(diff.old.New.Path, diff.old.New.Version), replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version), by)
			} else {
				warnLog(cmd, "%s: change replace %s => %s ==> %s => %s", name, replacePathVersion(k, diff.old.Old.Version), replacePathVersion(diff.old.New.Path, diff.old.New.Version), replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version))
			}
		}
	}
}

func (pb *pluginBuilder) editGoMod(cmd *cobra.Command, goCmd string, edit func(*modfile.File) error) error {
	if pb.gomod == nil {
		gomod, err := parseGoMod(cmd, goCmd, pb.gomodPath)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", pb.gomodPath, err)
		}
		pb.gomod = gomod
	}

	editErr := edit(pb.gomod)
	if editErr != nil {
		var re *retriableError
		if !errors.As(editErr, &re) {
			return fmt.Errorf("failed to edit %s: %w", pb.gomodPath, editErr)
		}
		return editErr
	}
	pb.gomod.Cleanup()
	edited, err := pb.gomod.Format()
	if err != nil {
		return fmt.Errorf("failed to edit %s: %w", pb.gomodPath, err)
	}

	f, err := os.Create(pb.gomodPath)
	if err != nil {
		return fmt.Errorf("failed to edit %s: %w", pb.gomodPath, err)
	}
	defer f.Close()
	if _, err := f.Write(edited); err != nil {
		return fmt.Errorf("failed to edit %s: %w", pb.gomodPath, err)
	}
	if err := modTidy(cmd, filepath.Dir(pb.gomodPath), goCmd); err != nil {
		return err
	}

	pb.gomod, err = parseGoMod(cmd, goCmd, pb.gomodPath)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", pb.gomodPath, err)
	}

	return editErr
}

func parseGoMod(cmd *cobra.Command, goCmd, gomodPath string) (*modfile.File, error) {
	b, err := os.ReadFile(gomodPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", gomodPath, err)
	}
	gomod, err := modfile.Parse(gomodPath, b, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", gomodPath, err)
	}
	return gomod, nil
}

func requiredModulesByScenarigo() ([]*modfile.Require, error) {
	gomod, err := modfile.Parse("go.mod", scenarigo.GoModBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod of scenarigo: %w", err)
	}
	if v := version.String(); !strings.HasSuffix(v, "-dev") {
		return append([]*modfile.Require{{
			Mod: module.Version{
				Path:    newScenarigoModPath,
				Version: v,
			},
		}}, gomod.Require...), nil
	}
	return gomod.Require, nil
}

func compareVers(v, w string) int {
	v = strings.TrimPrefix(v, "go")
	w = strings.TrimPrefix(w, "go")
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !strings.HasPrefix(w, "v") {
		w = "v" + w
	}
	return semver.Compare(v, w)
}

type versionTooHighError struct {
	err             error
	requiredVersion string
}

func (e *versionTooHighError) Error() string {
	return e.err.Error()
}

func (e *versionTooHighError) Unwrap() error {
	return e.err
}

func wrapVersionTooHighError(err error) error {
	if err == nil {
		return nil
	}
	if strings.HasPrefix(err.Error(), "go: go.mod requires go >= ") {
		var requiredVersion string
		result := versionTooHighErrorRegexp.FindAllStringSubmatch(err.Error(), 1)
		if len(result) > 0 && len(result[0]) > 1 {
			requiredVersion = result[0][1]
		}
		err = &versionTooHighError{
			err:             err,
			requiredVersion: requiredVersion,
		}
	}
	return err
}

func asVersionTooHighError(err error) (bool, *versionTooHighError) {
	if err == nil {
		return false, nil
	}
	var e *versionTooHighError
	if errors.As(err, &e) {
		return true, e
	}
	return false, nil
}

func warnLog(cmd *cobra.Command, format string, a ...any) {
	fmt.Fprintf(cmd.ErrOrStderr(), fmt.Sprintf("%s: %s\n", warnColor.Sprint("WARN"), format), a...)
}

func debugLog(cmd *cobra.Command, format string, a ...any) {
	if !verbose {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), fmt.Sprintf("%s: %s\n", debugColor.Sprint("DEBUG"), format), a...)
}

// copyFile copies a file from src to dst with the given mode.
func copyFile(src, dst string, mode fs.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

type exportedSymbols struct {
	Funcs  []string
	Values []string
}

// extractExportedSymbols extracts exported function and variable names from Go files in dir.
func extractExportedSymbols(dir string) (*exportedSymbols, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var syms exportedSymbols
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					if d.Recv == nil && d.Name.IsExported() {
						syms.Funcs = append(syms.Funcs, d.Name.Name)
					}
				case *ast.GenDecl:
					if d.Tok == token.VAR {
						for _, spec := range d.Specs {
							if vs, ok := spec.(*ast.ValueSpec); ok {
								for _, name := range vs.Names {
									if name.IsExported() {
										syms.Values = append(syms.Values, name.Name)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return &syms, nil
}

//go:embed wasm_main.go.tmpl
var wasmMainTmpl []byte

// createWasmMainFile generates a wasm.go that prints all exported symbols.
func createWasmMainFile(dir string, symbols *exportedSymbols) (string, error) {
	tmpl, err := template.New("").Parse(string(wasmMainTmpl))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, symbols); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("main_%d.go", time.Now().Unix()))
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// netFunctionInfo holds information about net package functions
type netFunctionInfo struct {
	FunctionName string
	FileName     string
	FilePath     string
}

// createNetPackageOverlay creates modified net package files for WASM overlay
func createNetPackageOverlay(ctx context.Context, goCmd string) (map[string]string, error) {
	// Get GOROOT
	goroot, err := execute(ctx, "", goCmd, "env", "GOROOT")
	if err != nil {
		return nil, fmt.Errorf("failed to get GOROOT: %w", err)
	}
	goroot = strings.TrimSpace(goroot)
	netPkgDir := filepath.Join(goroot, "src", "net")

	filePathMap := make(map[string]struct{})
	if p := findNetFunction(netPkgDir, "DialContext"); p == "" {
		return nil, errors.New("failed to find DialContext method from net package")
	} else {
		filePathMap[p] = struct{}{}
	}

	if p := findNetFunction(netPkgDir, "Listen"); p == "" {
		return nil, errors.New("failed to find Listen from net package")
	} else {
		filePathMap[p] = struct{}{}
	}

	overlayFiles := make(map[string]string)

	for p := range filePathMap {
		modifiedFile, err := createModifiedNetFile(p)
		if err != nil {
			return nil, fmt.Errorf("failed to create modified file: %w", err)
		}
		overlayFiles[p] = modifiedFile
	}
	return overlayFiles, nil
}

// findNetFunction finds a specific function or method in the net package
func findNetFunction(netPkgDir, functionName string) string {
	fset := token.NewFileSet()

	var ret string
	_ = filepath.Walk(netPkgDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(info.Name()) != ".go" {
			return nil
		}

		// Skip test files
		if strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		file, err := parser.ParseFile(fset, info.Name(), src, parser.ParseComments)
		if err != nil {
			return nil // Skip files that can't be parsed
		}

		for _, decl := range file.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				// Check for both function and method
				if funcDecl.Name.Name == functionName {
					ret = path
					return errors.New("found")
				}
			}
		}
		return nil
	})
	return ret
}

// createModifiedNetFile creates a modified version of a net package file using AST manipulation
func createModifiedNetFile(path string) (string, error) {
	// Read the original file
	src, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}

	// Parse the file to AST
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, filepath.Base(path), src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("failed to parse file %s: %w", path, err)
	}

	// Add unsafe import if not present
	hasUnsafeImport := false
	for _, imp := range astFile.Imports {
		if imp.Path.Value == `"unsafe"` {
			hasUnsafeImport = true
			break
		}
	}

	if !hasUnsafeImport {
		unsafeImport := &ast.ImportSpec{
			Name: &ast.Ident{Name: "_"},
			Path: &ast.BasicLit{Kind: token.STRING, Value: `"unsafe"`},
		}
		astFile.Imports = append(astFile.Imports, unsafeImport)

		// Find the last import in the file to determine where to insert
		var lastImportDecl *ast.GenDecl
		for _, decl := range astFile.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
				lastImportDecl = genDecl
			}
		}

		if lastImportDecl != nil {
			lastImportDecl.Specs = append(lastImportDecl.Specs, unsafeImport)
		} else {
			// Create new import declaration
			importDecl := &ast.GenDecl{
				Tok:   token.IMPORT,
				Specs: []ast.Spec{unsafeImport},
			}
			astFile.Decls = append([]ast.Decl{importDecl}, astFile.Decls...)
		}
	}

	// Track which target functions we found and modified
	foundDialContext := false
	foundListen := false

	// Find the target functions and modify them
	for _, decl := range astFile.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			funcName := funcDecl.Name.Name
			switch {
			case funcName == "DialContext" && funcDecl.Recv != nil: // Method, not function
				foundDialContext = true
				funcDecl.Body = &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ReturnStmt{
							Results: []ast.Expr{
								&ast.CallExpr{
									Fun: &ast.Ident{Name: "_dialContext"},
									Args: []ast.Expr{
										&ast.Ident{Name: "ctx"},
										&ast.Ident{Name: "network"},
										&ast.Ident{Name: "address"},
									},
								},
							},
						},
					},
				}
			case funcName == "Listen" && funcDecl.Recv == nil:
				foundListen = true
				funcDecl.Body = &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ReturnStmt{
							Results: []ast.Expr{
								&ast.CallExpr{
									Fun: &ast.Ident{Name: "_listen"},
									Args: []ast.Expr{
										&ast.Ident{Name: "network"},
										&ast.Ident{Name: "address"},
									},
								},
							},
						},
					},
				}
			}
		}
	}

	// Check if at least one target function was found
	if !foundDialContext && !foundListen {
		return "", fmt.Errorf("no target functions (DialContext or Listen) found in %s", path)
	}

	// Phase 1: Create functions without comments first
	// This will be simpler and avoid position conflicts

	// Create linkname functions without comments initially
	dialContextFuncDecl := &ast.FuncDecl{
		Name: &ast.Ident{Name: "_dialContext"},
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{{Name: "ctx"}},
						Type:  &ast.SelectorExpr{X: &ast.Ident{Name: "context"}, Sel: &ast.Ident{Name: "Context"}},
					},
					{
						Names: []*ast.Ident{{Name: "network"}},
						Type:  &ast.Ident{Name: "string"},
					},
					{
						Names: []*ast.Ident{{Name: "address"}},
						Type:  &ast.Ident{Name: "string"},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: &ast.Ident{Name: "Conn"}},
					{Type: &ast.Ident{Name: "error"}},
				},
			},
		},
		Body: nil, // No body for external linkage
	}

	listenFuncDecl := &ast.FuncDecl{
		Name: &ast.Ident{Name: "_listen"},
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{{Name: "network"}},
						Type:  &ast.Ident{Name: "string"},
					},
					{
						Names: []*ast.Ident{{Name: "address"}},
						Type:  &ast.Ident{Name: "string"},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: &ast.Ident{Name: "Listener"}},
					{Type: &ast.Ident{Name: "error"}},
				},
			},
		},
		Body: nil, // No body for external linkage
	}

	// Add linkname comments to the file's comment map
	if astFile.Comments == nil {
		astFile.Comments = []*ast.CommentGroup{}
	}

	// No comments in phase 1 - we'll add them after re-parsing

	// Set positions for the function declarations to match comment positions
	// Note: We can't directly set Pos() as it's a method, but the position is managed by the AST

	// Add the function declarations without comments
	astFile.Decls = append(astFile.Decls, dialContextFuncDecl, listenFuncDecl)

	// Phase 1: Format the AST with functions (no comments yet) to source code
	var phase1Buf bytes.Buffer
	if err := format.Node(&phase1Buf, fset, astFile); err != nil {
		return "", fmt.Errorf("failed to format phase 1 AST: %w", err)
	}

	// Phase 2: Re-parse the generated code and add comments
	newFset := token.NewFileSet()
	newAstFile, err := parser.ParseFile(newFset, filepath.Base(path), phase1Buf.String(), parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("failed to re-parse generated code: %w", err)
	}

	// Find and add Doc comments to the linkname functions
	for _, decl := range newAstFile.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			switch funcDecl.Name.Name {
			case "_dialContext":
				dialContextComment := &ast.CommentGroup{
					List: []*ast.Comment{
						{Text: "//go:linkname _dialContext github.com/goccy/wasi-go-net/wasip1.DialContext"},
					},
				}
				funcDecl.Doc = dialContextComment
				newAstFile.Comments = append(newAstFile.Comments, dialContextComment)
			case "_listen":
				listenComment := &ast.CommentGroup{
					List: []*ast.Comment{
						{Text: "//go:linkname _listen github.com/goccy/wasi-go-net/wasip1.Listen"},
					},
				}
				funcDecl.Doc = listenComment
				newAstFile.Comments = append(newAstFile.Comments, listenComment)
			}
		}
	}

	// Format the final AST with comments
	var finalBuf bytes.Buffer
	if err := format.Node(&finalBuf, newFset, newAstFile); err != nil {
		return "", fmt.Errorf("failed to format final AST: %w", err)
	}
	// Write modified file to temp location
	tempFile, err := os.CreateTemp("", fmt.Sprintf("net_%s.go", filepath.Base(path)))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	if _, err := tempFile.Write(finalBuf.Bytes()); err != nil {
		return "", fmt.Errorf("failed to write modified file: %w", err)
	}

	return tempFile.Name(), nil
}

// createOverlayJSON creates the JSON file for go build -overlay
func createOverlayJSON(overlayFiles map[string]string) (string, error) {
	overlayData, err := json.Marshal(map[string]interface{}{
		"Replace": overlayFiles,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal overlay JSON: %w", err)
	}

	tempFile, err := os.CreateTemp("", "scenarigo_overlay.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp overlay file: %w", err)
	}
	defer tempFile.Close()

	if _, err := tempFile.Write(overlayData); err != nil {
		return "", fmt.Errorf("failed to write overlay JSON: %w", err)
	}

	return tempFile.Name(), nil
}
