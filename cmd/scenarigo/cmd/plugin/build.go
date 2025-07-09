package plugin

import (
	"bytes"
	"context"
	_ "embed"
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
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/fatih/color"
	net "github.com/goccy/wasi-go-net"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

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
//
//nolint:cyclop
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
					return fmt.Errorf("failed to modify import path: %w", inspErr)
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

	if opts == nil || !opts.wasm {
		if err := pb.updateGoMod(cmd, goCmd, overrideKeys, overrides); err != nil {
			return err
		}
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
		// Analyze the code used for the plugin and extract a list of exported variables and functions.
		// Then, create a main function that includes a Register() API in github.com/scenarigo/scenarigo/plugin/wasm_guest.go to inform the host of this list, and add it to the build.
		symbols, err := extractExportedSymbols(pb.dir)
		if err != nil {
			return fmt.Errorf("failed to extract exported symbols: %w", err)
		}
		mainPath, err := createWasmMainFile(pb.dir, symbols)
		if err != nil {
			return fmt.Errorf("failed to create wasm.go: %w", err)
		}
		defer os.Remove(mainPath)

		currentVersion := getCurrentScenarigoVersion()
		pluginVersion := getPluginScenarigoVersion(pb.dir)
		scenarigoVersion := selectWasmPluginScenarigoVersion(currentVersion, pluginVersion)

		if _, err := execute(ctx, pb.dir, goCmd, "get", fmt.Sprintf("github.com/scenarigo/scenarigo@%s", scenarigoVersion)); err != nil {
			return fmt.Errorf("failed to go get github.com/scenarigo/scenarigo: %w", err)
		}
		if err := modTidy(cmd, filepath.Dir(pb.gomodPath), goCmd); err != nil {
			return err
		}

		// To replace net.Listen and net.Dialer.DialContext defined in the net package with Listen and DialContext defined in github.com/goccy/wasi-go-net/wasip1, use an overlay.
		// Without doing this, it is not possible to access the network without rewriting the Go source code used for the wasm plugin.
		overlayFile, err := net.CreateReplacedNetPkgOverlayFile(ctx, net.WithGoCommandPath(goCmd))
		if err != nil {
			return fmt.Errorf("failed to create overlay file: %w", err)
		}
		defer overlayFile.Close()

		envs = append(envs, "GOOS=wasip1", "GOARCH=wasm")
		if _, err := executeWithEnvs(ctx, envs, pb.dir, goCmd, "build", "-overlay", overlayFile.Path(), "-o", pb.out); err != nil {
			return fmt.Errorf(`"go build -overlay %s -o %s" failed: %w`, overlayFile.Path(), pb.out, err)
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

// getCurrentScenarigoVersion gets the version of scenarigo module used to build the current binary.
func getCurrentScenarigoVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	// Check main module first
	if info.Main.Path == newScenarigoModPath && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	// Check dependencies
	for _, dep := range info.Deps {
		if dep.Path == newScenarigoModPath && dep.Version != "" && dep.Version != "(devel)" {
			return dep.Version
		}
	}

	return ""
}

// getPluginScenarigoVersion gets the version of scenarigo module from plugin directory's go.mod.
func getPluginScenarigoVersion(pluginDir string) string {
	gomodPath := filepath.Join(pluginDir, "go.mod")
	b, err := os.ReadFile(gomodPath)
	if err != nil {
		return ""
	}

	gomod, err := modfile.Parse(gomodPath, b, nil)
	if err != nil {
		return ""
	}

	for _, req := range gomod.Require {
		if req.Mod.Path == newScenarigoModPath {
			return req.Mod.Version
		}
	}

	return ""
}

// selectWasmPluginScenarigoVersion selects the best version to use for go get.
func selectWasmPluginScenarigoVersion(currentVersion, pluginVersion string) string {
	if currentVersion == "" && pluginVersion == "" {
		return "latest"
	}

	if currentVersion == "" {
		return pluginVersion
	}

	if pluginVersion == "" {
		return currentVersion
	}

	// Compare versions and return the higher one
	if compareVers(currentVersion, pluginVersion) >= 0 {
		return currentVersion
	}

	return pluginVersion
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
					if d.Tok != token.VAR {
						continue
					}
					for _, spec := range d.Specs {
						vs, ok := spec.(*ast.ValueSpec)
						if !ok {
							continue
						}
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
	return &syms, nil
}

//go:embed wasm_main.go.tmpl
var wasmMainTmpl []byte

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
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
