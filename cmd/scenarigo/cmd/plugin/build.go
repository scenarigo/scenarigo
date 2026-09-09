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
	"maps"
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
	wasiext "github.com/goccy/wasi-go/ext"
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
	develVersion               = "(devel)"
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
	cmd.Flags().BoolVarP(&skipMigration, "skip-migration", "", false, "leave plugin sources untouched: skip rewriting the github.com/zoncoen/scenarigo and query-go v1 import paths and skip migrating extractor calls to the query-go v2 signatures")
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
	debugLogf(cmd, "scenarigo was built with %s", runtimeVersion)
	if !goversion.IsValid(goVer) {
		warnLogf(cmd, "failed to parse the Go version that built scenarigo: %s", runtimeVersion)
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
	debugLogf(cmd, "found go command: %s", goCmd)
	debugLogf(cmd, "set GOTOOLCHAIN=%s", toolchain)

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

	// From here on the migration may rewrite the plugin's own sources, so
	// whatever ends this function has to put them back unless the build that
	// followed them succeeded: an error return or a panic. An interrupt runs
	// through here too - main installs signal.NotifyContext and every go
	// command runs under that context, so a signal cancels the build and
	// returns an error rather than killing the process.
	built := false
	defer func() {
		if !built {
			restoreMigratedSources(cmd, pbs)
		}
	}()

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

	testHookAfterBuildLoop()

	built = true

	for _, pb := range pbs {
		if err := pb.printUpdatedResult(cmd, goCmd, pb.name, pb.gomodPath, overrides); err != nil {
			return err
		}
	}

	return nil
}

// testHookAfterBuildLoop runs after the build loop, before the build counts as
// done. Tests replace it to end the command in a way the loop's own error
// returns cannot - the deferred restore has to cover those endings too.
var testHookAfterBuildLoop = func() {}

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

// rewriteImports rewrites the import paths of the Go sources under dir.
// rewrite returns the new path for an import it applies to, and the empty
// string for one it leaves alone.
//
// A vendor tree is never entered: it holds other modules' sources, which no
// rewrite may touch. Test files are skipped when skipTestFiles says so - the
// build does not compile them, so a rewrite there is not needed to build and
// cannot be followed up by the call migration, which only sees what a failed
// build compiles.
func rewriteImports(dir string, skipTestFiles bool, rewrite func(string) string) error {
	fset := token.NewFileSet()
	return filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(info.Name()) != ".go" {
			return nil
		}
		if skipTestFiles && strings.HasSuffix(info.Name(), "_test.go") {
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
			x, ok := n.(*ast.ImportSpec)
			if !ok {
				return true
			}
			p, err := strconv.Unquote(x.Path.Value)
			if err != nil {
				inspErr = err
				return false
			}
			if to := rewrite(p); to != "" {
				found = true
				x.Path.Value = strconv.Quote(to)
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
		defer fd.Close()
		if err := format.Node(fd, fset, f); err != nil {
			return fmt.Errorf("failed to modify import path: %w", err)
		}
		return nil
	})
}

// queryGoV1Modules are the module paths of query-go v1 and its extractor
// modules, longest first so that an extractor import is matched as its own
// module rather than as a subpackage of the root one.
var queryGoV1Modules = []string{
	"github.com/zoncoen/query-go/extractor/protobuf",
	"github.com/zoncoen/query-go/extractor/yaml",
	"github.com/zoncoen/query-go",
}

// rewriteQueryGoV1 returns the v2 import path for an import of query-go v1 or
// of one of its extractor modules, and the empty string for any other import.
// The /v2 goes right after the module path, wherever the import points inside
// the module; an import that already carries it is left alone.
func rewriteQueryGoV1(path string) string {
	for _, m := range queryGoV1Modules {
		if path != m && !strings.HasPrefix(path, m+"/") {
			continue
		}
		rest := strings.TrimPrefix(path, m)
		if rest == "/v2" || strings.HasPrefix(rest, "/v2/") {
			return ""
		}
		return m + "/v2" + rest
	}
	return ""
}

// The 2nd return value should always be called.
func createPluginBuilder(cmd *cobra.Command, goCmd string, pluginModules map[string]*overrideModule, root, pluginDir string, item schema.OrderedMapItem[string, schema.PluginConfig], opts *buildOpts) (*pluginBuilder, func(), error) {
	out := item.Key

	// Check if plugin name has .wasm suffix and enable WASM build automatically
	if strings.HasSuffix(out, ".wasm") && !opts.wasm {
		debugLogf(cmd, "detected .wasm suffix in plugin name %s, enabling WASM build", out)
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
		debugLogf(cmd, "download %s into %s", item.Value.Src, m)

		m, f, err := copyModule(cmd, m)
		clean = f
		if err != nil {
			return nil, clean, fmt.Errorf("failed to build plugin %s: %w", out, err)
		}
		debugLogf(cmd, "copy %s into %s", item.Value.Src, m)

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
			debugLogf(cmd, "replace %s => %s in %s", oldScenarigoModPath, newScenarigoModPath, item.Value.Src)

			if err := rewriteImports(pb.dir, false, func(path string) string {
				if !strings.HasPrefix(path, oldScenarigoModPath) {
					return ""
				}
				return strings.Replace(path, oldScenarigoModPath, newScenarigoModPath, 1)
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

		// Move query-go itself to v2. A plugin that still imports v1 builds
		// fine - the v1 and v2 module paths coexist - so the migration that
		// runs on a build failure never sees it; it would just keep handing
		// scenarigo v1 option types that v2 silently drops. Rewrite the
		// imports up front instead. A compile error that follows, such as an
		// extractor call with the v1 signature, is exactly what the
		// on-failure migration exists to fix.
		var hasQueryGoV1 bool
		for _, m := range queryGoV1Modules {
			if _, ok := pb.initialRequires[m]; ok {
				hasQueryGoV1 = true
				break
			}
		}
		if hasQueryGoV1 {
			debugLogf(cmd, "replace query-go v1 imports with /v2 in %s", item.Value.Src)
			// Test files keep their v1 imports: the build does not compile
			// them, so the on-failure migration can never fix their extractor
			// calls, and a /v2 import over v1-shaped calls would not compile
			// at all. The tidy that follows keeps the v1 require around for
			// them, as a test-only dependency, so the plugin's own tests
			// still compile.
			if err := rewriteImports(pb.dir, true, rewriteQueryGoV1); err != nil {
				return nil, clean, err
			}
			// Dropping the v1 requires is enough: the build forces the /v2
			// modules in, since scenarigo itself requires them.
			if err := pb.editGoMod(cmd, goCmd, func(gomod *modfile.File) error {
				for _, m := range queryGoV1Modules {
					if _, ok := pb.initialRequires[m]; !ok {
						continue
					}
					if err := gomod.DropRequire(m); err != nil {
						return fmt.Errorf("%s: %w", pb.gomodPath, err)
					}
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

	// migratedSources holds the original content of every source file the
	// extractor migration rewrote, so that a failed build can restore them.
	migratedSources map[string]migratedSource
	// migrated is set once the extractor migration ran, so that a plugin is
	// migrated at most once per build.
	migrated bool
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
			warnLogf(cmd, `"go mod tidy" failed, retry...`)
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

		// To replace net.Listen and net.Dialer.DialContext and crypto/x509.Certificate.Verify and exec.Command.Start etc.
		// Without doing this, it is not possible to use http(s) and exec.Command feature in the wasm plugin.
		overlayFile, err := wasiext.CreateOverlay(ctx, wasiext.WithGoCommandPath(goCmd))
		if err != nil {
			return fmt.Errorf("failed to create overlay file: %w", err)
		}
		defer overlayFile.Close()

		envs = append(envs, "GOOS=wasip1", "GOARCH=wasm")
		if _, err := executeWithEnvs(ctx, envs, pb.dir, goCmd, "build", "-overlay", overlayFile.Path(), "-o", pb.out); err != nil {
			// The generated main file is part of the package being
			// analyzed, so this runs before the deferred removal.
			if retry := pb.migrateExtractorCalls(cmd, goCmd, envs, []string{"-overlay", overlayFile.Path()}, []string{mainPath}, pb.dir, "", opts); retry != nil {
				return retry
			}
			return pb.buildError(fmt.Errorf(`"go build -overlay %s -o %s" failed: %w`, overlayFile.Path(), pb.out, err))
		}
		return nil
	}

	if _, err := executeWithEnvs(ctx, envs, pb.dir, goCmd, "build", "-buildmode=plugin", "-o", pb.out, pb.src); err != nil {
		if retry := pb.migrateExtractorCalls(cmd, goCmd, envs, nil, nil, pb.buildPkgDir(), pb.buildTarget(), opts); retry != nil {
			return retry
		}
		return pb.buildError(fmt.Errorf(`"go build -buildmode=plugin -o %s %s" failed: %w`, pb.out, pb.src, err))
	}
	return nil
}

// buildPkgDir returns the directory of the package the build compiles, which
// is not pb.dir when the plugin lives in a subdirectory of its module.
func (pb *pluginBuilder) buildPkgDir() string {
	path := filepath.Join(pb.dir, pb.src)
	info, err := os.Stat(path)
	if err != nil {
		return pb.dir
	}
	if info.IsDir() {
		return path
	}
	return filepath.Dir(path)
}

// buildTarget returns the source file the native build compiles when the
// plugin src is a file, or an empty string when it compiles the package in
// pb.dir.
func (pb *pluginBuilder) buildTarget() string {
	path := filepath.Join(pb.dir, pb.src)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

// migrateExtractorCalls rewrites the plugin sources that still call
// scenarigo's extractors with the query-go v1 signatures and asks for one
// rebuild. It returns nil when there is nothing to rewrite, leaving the build
// error to be reported as it is.
func (pb *pluginBuilder) migrateExtractorCalls(cmd *cobra.Command, goCmd string, envs, buildFlags, skipFiles []string, pkgDir, target string, opts *buildOpts) error {
	if (opts != nil && opts.skipMigration) || pb.migrated {
		return nil
	}
	pb.migrated = true
	res, err := migrateExtractorCalls(ctx(cmd), &migration{
		dir:        pb.dir,
		goCmd:      goCmd,
		pkgDir:     pkgDir,
		env:        commandEnv(envs),
		buildFlags: buildFlags,
		skipFiles:  skipFiles,
		target:     target,
	})
	pb.rememberMigratedSources(res.originals)
	if err != nil {
		warnLogf(cmd, "%s: failed to migrate the extractor calls: %s", pb.name, err)
		return nil
	}
	for _, m := range res.manual {
		warnLogf(cmd, "%s: %s", pb.name, m)
	}
	if len(res.changes) == 0 {
		return nil
	}
	for _, c := range res.changes {
		warnLogf(cmd, "%s: %s", pb.name, c)
	}
	warnLogf(cmd, "%s: rewrote %d call(s) in place because query-go v2 changed the extractor methods to ExtractByKey(ctx context.Context, key string) (any, error); pass --skip-migration to leave the sources untouched", pb.name, len(res.changes))
	return &retriableError{
		reason: fmt.Sprintf("migrated %d extractor call(s)", len(res.changes)),
	}
}

// buildError reports a build failure, naming the automatic migration when the
// sources were rewritten before this attempt: the compiler errors the user is
// about to read then come from code they did not write.
func (pb *pluginBuilder) buildError(err error) error {
	if pb.migrated && len(pb.migratedSources) > 0 {
		return fmt.Errorf("the build failed again after migrating the sources: %w", err)
	}
	return err
}

func (pb *pluginBuilder) rememberMigratedSources(originals map[string]migratedSource) {
	if len(originals) == 0 {
		return
	}
	if pb.migratedSources == nil {
		pb.migratedSources = map[string]migratedSource{}
	}
	for path, s := range originals {
		if _, ok := pb.migratedSources[path]; !ok {
			pb.migratedSources[path] = s
		}
	}
}

// restoreMigratedSources puts back every source file the migration rewrote.
// A build that failed anyway must leave the plugin as it found it.
func restoreMigratedSources(cmd *cobra.Command, pbs []*pluginBuilder) {
	for _, pb := range pbs {
		pb.restoreMigratedSources(cmd)
	}
}

func (pb *pluginBuilder) restoreMigratedSources(cmd *cobra.Command) {
	dir, err := filepath.Abs(pb.dir)
	if err != nil {
		dir = pb.dir
	}
	realDir := realPath(dir)
	for _, path := range slices.Sorted(maps.Keys(pb.migratedSources)) {
		s := pb.migratedSources[path]
		// The file was inside the plugin directory when it was rewritten;
		// make sure it still is before writing through it.
		if resolved, err := filepath.EvalSymlinks(path); err != nil || !within(resolved, realDir) {
			warnLogf(cmd, "%s: not restoring %s because it no longer resolves inside the plugin directory", pb.name, path)
			continue
		}
		if err := os.WriteFile(path, s.content, s.mode); err != nil {
			warnLogf(cmd, "%s: failed to restore %s: %s", pb.name, path, err)
			continue
		}
		warnLogf(cmd, "%s: restored %s because the build failed after the migration", pb.name, path)
	}
	pb.migratedSources = nil
}

func execute(ctx context.Context, wd, name string, args ...string) (string, error) {
	return executeWithEnvs(ctx, nil, wd, name, args...)
}

// commandEnv returns the environment every go command runs with: the process
// environment, envs, and the toolchain selection.
func commandEnv(envs []string) []string {
	return append(os.Environ(), append(slices.Clone(envs), fmt.Sprintf("GOTOOLCHAIN=%s", toolchain))...)
}

func executeWithEnvs(ctx context.Context, envs []string, wd, name string, args ...string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = commandEnv(envs)
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
				warnLogf(cmd, "%s: remove toolchain by scenarigo", pb.name)
				gomod.DropToolchainStmt()
			}
			return nil
		}

		switch compareVers(gomod.Go.Version, toolchain) {
		case -1: // go.mod < scenarigo go version
			if gomod.Toolchain == nil {
				warnLogf(cmd, "%s: add toolchain %s by scenarigo", pb.name, toolchain)
			} else if gomod.Toolchain.Name != toolchain {
				warnLogf(cmd, "%s: change toolchain %s ==> %s by scenarigo", pb.name, gomod.Toolchain.Name, toolchain)
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
			if require == nil || require.Mod.Path == "" {
				continue
			}
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
					warnLogf(cmd, "%s: add require %s %s by %s", name, k, diff.new.Mod.Version, requiredBy)
				} else {
					warnLogf(cmd, "%s: add require %s %s", name, k, diff.new.Mod.Version)
				}
			}
		case diff.new.Mod.Path == "":
			if !diff.old.Indirect {
				warnLogf(cmd, "%s: remove require %s %s", name, k, diff.old.Mod.Version)
			}
		case diff.old.Mod.Version != diff.new.Mod.Version:
			if !diff.old.Indirect || !diff.new.Indirect {
				if o := overrides[k]; o != nil {
					_, requiredBy, _, _ := o.requireReplace()
					warnLogf(cmd, "%s: change require %s %s ==> %s by %s", name, k, diff.old.Mod.Version, diff.new.Mod.Version, requiredBy)
				} else {
					warnLogf(cmd, "%s: change require %s %s ==> %s", name, k, diff.old.Mod.Version, diff.new.Mod.Version)
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
				warnLogf(cmd, "%s: add replace %s => %s by %s", name, replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version), by)
			} else {
				warnLogf(cmd, "%s: add replace %s => %s", name, replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version))
			}
		case diff.new.Old.Path == "":
			warnLogf(cmd, "%s: remove replace %s => %s", name, replacePathVersion(k, diff.old.Old.Version), replacePathVersion(diff.old.New.Path, diff.old.New.Version))
		case diff.old.New.Path != diff.new.New.Path || diff.old.New.Version != diff.new.New.Version:
			if o := overrides[k]; o != nil {
				_, by, replace, replaceBy := o.requireReplace()
				if replace != nil {
					by = replaceBy
				}
				warnLogf(cmd, "%s: change replace %s => %s ==> %s => %s by %s", name, replacePathVersion(k, diff.old.Old.Version), replacePathVersion(diff.old.New.Path, diff.old.New.Version), replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version), by)
			} else {
				warnLogf(cmd, "%s: change replace %s => %s ==> %s => %s", name, replacePathVersion(k, diff.old.Old.Version), replacePathVersion(diff.old.New.Path, diff.old.New.Version), replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version))
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

func warnLogf(cmd *cobra.Command, format string, a ...any) {
	fmt.Fprintf(cmd.ErrOrStderr(), fmt.Sprintf("%s: %s\n", warnColor.Sprint("WARN"), format), a...)
}

func debugLogf(cmd *cobra.Command, format string, a ...any) {
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
	if info.Main.Path == newScenarigoModPath && info.Main.Version != "" && info.Main.Version != develVersion {
		return info.Main.Version
	}

	// Check dependencies
	for _, dep := range info.Deps {
		if dep.Path == newScenarigoModPath && dep.Version != "" && dep.Version != develVersion {
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
