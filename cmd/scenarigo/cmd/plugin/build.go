package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/build"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"github.com/zoncoen/scenarigo"
	"github.com/zoncoen/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/zoncoen/scenarigo/internal/filepathutil"
	"github.com/zoncoen/scenarigo/version"
)

var (
	goVer        string
	tip          bool
	goMajorMinor string
	gomodVer     string
)

func init() {
	goVer = runtime.Version()
	if strings.HasPrefix(goVer, "devel ") {
		// gotip
		goVer = strings.Split(strings.TrimPrefix(goVer, "devel "), "-")[0]
		tip = true
	}
	e := strings.Split(strings.TrimPrefix(goVer, "go"), ".")
	if len(e) < 2 {
		panic(fmt.Sprintf("%q is invalid Go version", goVer))
	}
	goMajorMinor = strings.Join(e[:2], ".")
	gomod, err := modfile.Parse("go.mod", scenarigo.GoModBytes, nil)
	if err != nil {
		panic(fmt.Errorf("failed to parse go.mod of scenarigo: %w", err))
	}
	gomodVer = gomod.Go.Version
}

var buildCmd = &cobra.Command{
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

var warnColor = color.New(color.Bold, color.FgYellow)

type overrideModule struct {
	require          *modfile.Require
	requiredBy       string
	replace          *modfile.Replace
	replacedBy       string
	replaceLocalPath string
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

func buildRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		return errors.New("config file not found")
	}

	goCmd, err := findGoCmd(ctx(cmd), tip)
	if err != nil {
		return err
	}

	pbs := make([]*pluginBuilder, 0, cfg.Plugins.Len())
	pluginModules := map[string]*overrideModule{}
	pluginDir := filepathutil.From(cfg.Root, cfg.PluginDirectory)
	for _, item := range cfg.Plugins.ToSlice() {
		out := item.Key
		mod := filepathutil.From(cfg.Root, item.Value.Src)
		var src string
		if _, err := os.Stat(mod); err != nil {
			m, s, r, clean, err := downloadModule(ctx(cmd), goCmd, item.Value.Src)
			defer clean()
			if err != nil {
				return fmt.Errorf("failed to build plugin %s: %w", out, err)
			}
			mod = m
			src = s
			pluginModules[r.Mod.Path] = &overrideModule{
				require:    r,
				requiredBy: out,
			}
		}
		// NOTE: All module names must be unique and different from the standard modules.
		defaultModName := filepath.Join("plugins", strings.TrimSuffix(out, ".so"))
		pb, err := newPluginBuilder(cmd, goCmd, out, mod, src, filepathutil.From(pluginDir, out), defaultModName)
		if err != nil {
			return fmt.Errorf("failed to build plugin %s: %w", out, err)
		}
		pbs = append(pbs, pb)
	}

	overrides, err := selectUnifiedVersions(pbs)
	if err != nil {
		return fmt.Errorf("failed to build plugin: %w", err)
	}

	for m, o := range pluginModules {
		overrides[m] = &overrideModule{
			require:    o.require,
			requiredBy: o.requiredBy,
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
		}
	}

	overrideKeys := make([]string, 0, len(overrides))
	for k := range overrides {
		overrideKeys = append(overrideKeys, k)
	}
	sort.Strings(overrideKeys)

	for _, pb := range pbs {
		if err := pb.build(cmd, goCmd, overrideKeys, overrides); err != nil {
			return fmt.Errorf("failed to build plugin %s: %w", pb.name, err)
		}
	}

	return nil
}

func findGoCmd(ctx context.Context, tip bool) (string, error) {
	goCmd, err := exec.LookPath("go")
	var verr error
	if err == nil {
		verr = checkGoVersion(ctx, goCmd, gomodVer)
		if verr == nil {
			return goCmd, nil
		}
	}
	if tip {
		if goCmd, err := exec.LookPath("gotip"); err == nil {
			if err := checkGoVersion(ctx, goCmd, goVer); err == nil {
				return goCmd, nil
			}
		}
	}
	if err == nil {
		return "", verr
	}
	return "", fmt.Errorf("go command required: %w", err)
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
			if semver.Compare(o.require.Mod.Version, r.Mod.Version) < 0 {
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
		if len(items) > 4 && items[2] == "devel" {
			// gotip
			items[2] = strings.Split(items[3], "-")[0]
		} else {
			return errors.New("invalid version output or scenarigo bug")
		}
	}
	ver := strings.TrimPrefix(items[2], "go")
	if semver.Compare("v"+ver, "v"+minVer) == -1 {
		return fmt.Errorf(`required go %s or later but installed %s`, minVer, ver)
	}
	return nil
}

func downloadModule(ctx context.Context, goCmd, p string) (string, string, *modfile.Require, func(), error) {
	tempDir, err := os.MkdirTemp("", "scenarigo-plugin-")
	if err != nil {
		return "", "", nil, func() {}, fmt.Errorf("failed to create a temporary directory: %w", err)
	}

	clean := func() {
		os.RemoveAll(tempDir)
	}

	if err := execute(ctx, tempDir, goCmd, "mod", "init", "download_module"); err != nil {
		return "", "", nil, clean, fmt.Errorf("failed to initialize go.mod: %w", err)
	}
	if err := execute(ctx, tempDir, goCmd, downloadCmd(p)...); err != nil {
		return "", "", nil, clean, fmt.Errorf("failed to download %s: %w", p, err)
	}
	mod, src, req, err := modSrcPath(tempDir, p)
	if err != nil {
		return "", "", nil, clean, fmt.Errorf("failed to get module path: %w", err)
	}

	clean = func() {
		os.RemoveAll(tempDir)
		// The downloaded plugin module should be removed because its go.mod may have been modified.
		if err := filepath.Walk(mod, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			return os.Chmod(path, 0o777)
		}); err == nil {
			os.RemoveAll(mod)
		}
	}

	if err := os.Chmod(mod, 0o755); err != nil {
		return "", "", nil, clean, err
	}
	if err := os.Chmod(filepath.Join(mod, "go.mod"), 0o644); err != nil {
		if !os.IsNotExist(err) {
			return "", "", nil, clean, err
		}
	}
	if err := os.Chmod(filepath.Join(mod, "go.sum"), 0o644); err != nil {
		if !os.IsNotExist(err) {
			return "", "", nil, clean, err
		}
	}

	return mod, src, req, clean, nil
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

type pluginBuilder struct {
	name      string
	dir       string
	src       string
	gomodPath string
	gomod     *modfile.File
	out       string
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
		if err := execute(ctx, dir, goCmd, "mod", "init"); err != nil {
			// ref. https://github.com/golang/go/wiki/Modules#why-does-go-mod-init-give-the-error-cannot-determine-module-path-for-source-directory
			if strings.Contains(err.Error(), "cannot determine module path") {
				if err := execute(ctx, dir, goCmd, "mod", "init", defaultModName); err != nil {
					return nil, fmt.Errorf("failed to initialize go.mod: %w", err)
				}
			} else {
				return nil, fmt.Errorf("failed to initialize go.mod: %w", err)
			}
		}
	}

	if err := modTidy(ctx, dir, goCmd); err != nil {
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

	return &pluginBuilder{
		name:      name,
		dir:       dir,
		src:       src,
		gomodPath: gomodPath,
		gomod:     gomod,
		out:       out,
	}, nil
}

func modTidy(ctx context.Context, dir, goCmd string) error {
	if cmd := os.Getenv("GO_MOD_TIDY"); cmd != "" {
		if err := execute(ctx, dir, goCmd, strings.Split(cmd, " ")...); err != nil {
			return err
		}
		return nil
	}
	if err := execute(ctx, dir, goCmd, "mod", "tidy", fmt.Sprintf("-compat=%s", goMajorMinor)); err != nil {
		return fmt.Errorf(`"go mod tidy" failed: %w`, err)
	}
	return nil
}

func (pb *pluginBuilder) build(cmd *cobra.Command, goCmd string, overrideKeys []string, overrides map[string]*overrideModule) error {
	ctx := ctx(cmd)
	if err := updateGoMod(cmd, goCmd, pb.name, pb.gomodPath, overrideKeys, overrides); err != nil {
		return err
	}
	if err := os.RemoveAll(pb.out); err != nil {
		return fmt.Errorf("failed to delete the old plugin %s: %w", pb.out, err)
	}
	if err := execute(ctx, pb.dir, goCmd, "build", "-buildmode=plugin", "-o", pb.out, pb.src); err != nil {
		return fmt.Errorf(`"go build -buildmode=plugin -o %s %s" failed: %w`, pb.out, pb.src, err)
	}
	return nil
}

func execute(ctx context.Context, wd, name string, args ...string) error {
	return executeWithEnvs(ctx, nil, wd, name, args...)
}

func executeWithEnvs(ctx context.Context, envs []string, wd, name string, args ...string) error {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	if tip {
		envs = append(envs, "GOTOOLCHAIN=local")
	} else {
		envs = append(envs, fmt.Sprintf("GOTOOLCHAIN=%s", goVer))
	}
	cmd.Env = append(os.Environ(), envs...)
	if wd != "" {
		cmd.Dir = wd
	}
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.New(strings.TrimSuffix(stderr.String(), "\n"))
	}
	return nil
}

func updateGoMod(cmd *cobra.Command, goCmd, name, gomodPath string, overrideKeys []string, overrides map[string]*overrideModule) error {
	if err := editGoMod(cmd, goCmd, gomodPath, func(gomod *modfile.File) error {
		gomod.DropToolchainStmt()
		return nil
	}); err != nil {
		return fmt.Errorf("failed to edit toolchain directive: %w", err)
	}

	initialRequires, initialReplaces, err := updateRequireDirectives(cmd, goCmd, gomodPath, overrides)
	if err != nil {
		return err
	}
	if err := updateReplaceDirectives(cmd, goCmd, gomodPath, overrides, overrideKeys); err != nil {
		return err
	}
	if err := printUpdatedResult(cmd, goCmd, name, gomodPath, overrides, initialRequires, initialReplaces); err != nil {
		return err
	}
	return nil
}

func updateRequireDirectives(cmd *cobra.Command, goCmd, gomodPath string, overrides map[string]*overrideModule) (map[string]modfile.Require, map[string]modfile.Replace, error) {
	initialRequires := map[string]modfile.Require{}
	initialReplaces := map[string]modfile.Replace{}
	if err := editGoMod(cmd, goCmd, gomodPath, func(gomod *modfile.File) error {
		for _, r := range gomod.Require {
			initialRequires[r.Mod.Path] = *r
		}
		for _, r := range gomod.Replace {
			initialReplaces[r.Old.Path] = *r
		}
		if semver.Compare(gomod.Go.Version, gomodVer) < 0 {
			if err := gomod.AddGoStmt(gomodVer); err != nil {
				return fmt.Errorf("%s: %w", gomodPath, err)
			}
		}
		for _, o := range overrides {
			require, _, _, _ := o.requireReplace()
			if err := gomod.AddRequire(require.Mod.Path, require.Mod.Version); err != nil {
				return fmt.Errorf("%s: %w", gomodPath, err)
			}
		}
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to edit require directive: %w", err)
	}
	return initialRequires, initialReplaces, nil
}

func updateReplaceDirectives(cmd *cobra.Command, goCmd, gomodPath string, overrides map[string]*overrideModule, overrideKeys []string) error {
	if err := editGoMod(cmd, goCmd, gomodPath, func(gomod *modfile.File) error {
		requires := map[string]string{}
		for _, r := range gomod.Require {
			requires[r.Mod.Path] = r.Mod.Version
		}
		replaces := map[string]modfile.Replace{}
		for _, r := range gomod.Replace {
			if _, ok := requires[r.Old.Path]; !ok {
				if err := gomod.DropReplace(r.Old.Path, r.Old.Version); err != nil {
					return fmt.Errorf("%s: %w", gomodPath, err)
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
					return fmt.Errorf("%s: %w", gomodPath, err)
				}
			}
			if replace != nil {
				if v, ok := requires[replace.Old.Path]; ok {
					path := replace.New.Path
					if o.replaceLocalPath != "" {
						rel, err := filepath.Rel(filepath.Dir(gomodPath), o.replaceLocalPath)
						if err != nil {
							return fmt.Errorf("%s: %w", gomodPath, err)
						}
						// must be rooted or staring with ./ or ../
						if sep := string(filepath.Separator); !strings.Contains(rel, sep) {
							rel += sep
						}
						path = rel
					}
					if err := gomod.AddReplace(replace.Old.Path, v, path, replace.New.Version); err != nil {
						return fmt.Errorf("%s: %w", gomodPath, err)
					}
				}
			} else {
				if v, ok := requires[require.Mod.Path]; ok {
					if require.Mod.Version != v {
						if err := gomod.AddReplace(require.Mod.Path, v, require.Mod.Path, require.Mod.Version); err != nil {
							return fmt.Errorf("%s: %w", gomodPath, err)
						}
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

func printUpdatedResult(cmd *cobra.Command, goCmd, name, gomodPath string, overrides map[string]*overrideModule, initialRequires map[string]modfile.Require, initialReplaces map[string]modfile.Replace) error {
	gomod, err := parseGoMod(cmd, goCmd, gomodPath)
	if err != nil {
		return err
	}
	printUpdatedRequires(cmd, name, overrides, initialRequires, gomod)
	printUpdatedReplaces(cmd, name, overrides, initialReplaces, gomod)
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
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: add require %s %s by %s\n", warnColor.Sprint("WARN"), name, k, diff.new.Mod.Version, requiredBy)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: add require %s %s\n", warnColor.Sprint("WARN"), name, k, diff.new.Mod.Version)
				}
			}
		case diff.new.Mod.Path == "":
			if !diff.old.Indirect {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: remove require %s %s\n", warnColor.Sprint("WARN"), name, k, diff.old.Mod.Version)
			}
		case diff.old.Mod.Version != diff.new.Mod.Version:
			if !diff.old.Indirect || !diff.new.Indirect {
				if o := overrides[k]; o != nil {
					_, requiredBy, _, _ := o.requireReplace()
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: change require %s %s ==> %s by %s\n", warnColor.Sprint("WARN"), name, k, diff.old.Mod.Version, diff.new.Mod.Version, requiredBy)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: change require %s %s ==> %s\n", warnColor.Sprint("WARN"), name, k, diff.old.Mod.Version, diff.new.Mod.Version)
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
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: add replace %s => %s by %s\n", warnColor.Sprint("WARN"), name, replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version), by)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: add replace %s => %s\n", warnColor.Sprint("WARN"), name, replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version))
			}
		case diff.new.Old.Path == "":
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: remove replace %s => %s\n", warnColor.Sprint("WARN"), name, replacePathVersion(k, diff.old.Old.Version), replacePathVersion(diff.old.New.Path, diff.old.New.Version))
		case diff.old.New.Path != diff.new.New.Path || diff.old.New.Version != diff.new.New.Version:
			if o := overrides[k]; o != nil {
				_, by, replace, replaceBy := o.requireReplace()
				if replace != nil {
					by = replaceBy
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: change replace %s => %s ==> %s => %s by %s\n", warnColor.Sprint("WARN"), name, replacePathVersion(k, diff.old.Old.Version), replacePathVersion(diff.old.New.Path, diff.old.New.Version), replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version), by)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s: change replace %s => %s ==> %s => %s\n", warnColor.Sprint("WARN"), name, replacePathVersion(k, diff.old.Old.Version), replacePathVersion(diff.old.New.Path, diff.old.New.Version), replacePathVersion(k, diff.new.Old.Version), replacePathVersion(diff.new.New.Path, diff.new.New.Version))
			}
		}
	}
}

func editGoMod(cmd *cobra.Command, goCmd, gomodPath string, edit func(*modfile.File) error) error {
	gomod, err := parseGoMod(cmd, goCmd, gomodPath)
	if err != nil {
		return err
	}

	if err := edit(gomod); err != nil {
		return fmt.Errorf("failed to edit %s: %w", gomodPath, err)
	}
	gomod.Cleanup()
	edited, err := gomod.Format()
	if err != nil {
		return fmt.Errorf("failed to edit %s: %w", gomodPath, err)
	}

	f, err := os.Create(gomodPath)
	if err != nil {
		return fmt.Errorf("failed to edit %s: %w", gomodPath, err)
	}
	defer f.Close()
	if _, err := f.Write(edited); err != nil {
		return fmt.Errorf("failed to edit %s: %w", gomodPath, err)
	}
	if err := modTidy(ctx(cmd), filepath.Dir(gomodPath), goCmd); err != nil {
		return err
	}

	return nil
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
				Path:    "github.com/zoncoen/scenarigo",
				Version: v,
			},
		}}, gomod.Require...), nil
	}
	return gomod.Require, nil
}
