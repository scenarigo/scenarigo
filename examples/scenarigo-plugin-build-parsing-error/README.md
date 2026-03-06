# Plugin Build go.mod Parsing Error

This example reproduces a bug in `updateRequireDirectives` (`cmd/scenarigo/cmd/plugin/build.go`) where a `replace` directive without a corresponding `require` causes an invalid entry to be written to `go.mod`, resulting in a `go mod tidy` parsing error.

## Description

When `plugin/src/go.mod` has a `replace` directive for `google.golang.org/genproto` but does NOT `require` it, `selectUnifiedVersions` creates an override entry with `require: nil` and `replace: non-nil`.

`updateRequireDirectives` then calls `requireReplace()`, which returns `&modfile.Require{Mod: replace.New}` (in this case `{Path: "./genproto-stub", Version: ""}`). This is passed to `AddRequire("./genproto-stub", "")`, writing an invalid require line to the generated `go.mod`.

Note: `google.golang.org/genproto/googleapis/rpc` — the module involved in the original bug — cannot be used directly because it is part of scenarigo's own `go.mod`. `requiredModulesByScenarigo()` overwrites the overrides entry for that module before `updateRequireDirectives` runs. The parent module `google.golang.org/genproto` (a separate Go module) is used instead to demonstrate the identical code path.

## Running

```bash
cd examples/scenarigo-plugin-build-parsing-error
scenarigo plugin build
```

## Expected Output (after fix)

```
scenarigo plugin build succeeds (the invalid require is skipped)
```

## Actual Output (bug)

```
failed to build plugin ...: failed to edit require directive: ...: "go mod tidy" failed: go: errors parsing go.mod:
go.mod:...: usage: require module/path v1.2.3
```

## Fix

Add a guard in `updateRequireDirectives` to skip entries where `require.Mod.Path` is empty or invalid:

```go
// build.go — updateRequireDirectives
for _, o := range overrides {
    require, _, _, _ := o.requireReplace()
    if require == nil || require.Mod.Path == "" {
        continue
    }
    if err := gomod.AddRequire(require.Mod.Path, require.Mod.Version); err != nil {
        return fmt.Errorf("%s: %w", pb.gomodPath, err)
    }
}
```
