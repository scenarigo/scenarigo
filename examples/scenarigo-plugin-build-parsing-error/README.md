# Plugin Build go.mod Parsing Error

This example reproduces a bug in `scenarigo plugin build` where a `replace` directive with a local path and no corresponding `require` causes an invalid entry to be written to go.mod, resulting in a `go mod tidy` parsing error.

## Bug

`plugin/src/go.mod` has the following replace directive:

```
replace google.golang.org/genproto v0.0.0-20251202230838-ff82c1b0f217 => ./genproto-local
```

This module (`google.golang.org/genproto`) is **not** in the plugin's `require` section and is **not** in scenarigo's own `go.mod`.

During `scenarigo plugin build`, `selectUnifiedVersions()` creates an override entry with `require: nil` and `replace: non-nil`. Since the module is not in scenarigo's `go.mod`, `requiredModulesByScenarigo()` does not overwrite it.

`updateRequireDirectives()` then calls `requireReplace()`, which returns `replace.New` (`{Path: "./genproto-local", Version: ""}`). This is passed to `AddRequire("./genproto-local", "")`, writing an invalid require line to go.mod.

The relevant code in `cmd/scenarigo/cmd/plugin/build.go`:

```go
func (o *overrideModule) requireReplace() (*modfile.Require, string, *modfile.Replace, string) {
    if o.replace != nil {
        if o.require == nil || o.replace.Old.Path == o.replace.New.Path {
            return &modfile.Require{
                Mod:      o.replace.New, // {Path: "./genproto-local", Version: ""}
                Indirect: false,
                Syntax:   nil,
            }, o.replacedBy, nil, ""
        }
    }
    return o.require, o.requiredBy, o.replace, o.replacedBy
}
```

## Reproduce

```bash
cd examples/scenarigo-plugin-build-parsing-error
scenarigo plugin build
```

## Expected Output (after fix)

Build succeeds (the invalid require is skipped or handled correctly).

## Actual Output (bug)

```
ERRO failed to build plugin plugin.so: failed to edit require directive: ...: "go mod tidy" failed: go: errors parsing go.mod:
go.mod:...: usage: require module/path v1.2.3
```

## Fix

Add a guard in `updateRequireDirectives` to skip entries where the version is empty (local path replace without a corresponding require):

```go
for _, o := range overrides {
    require, _, _, _ := o.requireReplace()
    if require.Mod.Version == "" {
        continue
    }
    if err := gomod.AddRequire(require.Mod.Path, require.Mod.Version); err != nil {
        return fmt.Errorf("%s: %w", pb.gomodPath, err)
    }
}
```
