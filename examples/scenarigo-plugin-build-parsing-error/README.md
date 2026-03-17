# Plugin Build go.mod Parsing Error

This example reproduces a bug in `updateRequireDirectives` (`cmd/scenarigo/cmd/plugin/build.go`) where an empty `require` entry (`""`) is written to `go.mod`, resulting in a `go mod tidy` parsing error.

## Description

When building two plugins that share overlapping `replace` directives, the first build iteration may trigger a `retriableError` in `updateReplaceDirectives`. This causes `editGoMod` to return early **without calling `Cleanup()`**, leaving zeroed-out `Replace{}` entries (created by `DropReplace`) in the in-memory `pb.gomod.Replace` slice.

On the second iteration, `selectUnifiedVersions` reads these zeroed entries, creating an `overrides[""]` entry with an all-empty replace. `updateRequireDirectives` then calls `requireReplace()` which returns `&modfile.Require{Mod: replace.New}` — a require with empty `Mod.Path`. `AddRequire("", "")` writes an invalid line to `go.mod`, and the subsequent `go mod tidy` fails.

## Setup

- **plugin.so**: Minimal plugin (`import _ "github.com/scenarigo/scenarigo"`) with 5 `replace` directives
- **shared-plugin.so**: Minimal plugin with cloud/grpc imports and 2 `replace` directives (carvel + genproto/rpc)
- Both plugins share `replace carvel.dev/ytt` and `replace google.golang.org/genproto/googleapis/rpc`

## Running

```bash
cd examples/scenarigo-plugin-build-parsing-error
scenarigo plugin build
```

## Expected Output (after fix)

```
scenarigo plugin build succeeds
```

## Actual Output (bug)

```
failed to build plugin plugin.so: failed to edit require directive: ...: "go mod tidy" failed: go: errors parsing go.mod:
go.mod:...: usage: require module/path v1.2.3
```

## Root Cause

1. `DropReplace()` in `golang.org/x/mod/modfile` zeros out entries (`*r = Replace{}`) but doesn't remove them from the slice
2. `Cleanup()` is supposed to filter out zeroed entries, but it's **not called** when `editGoMod` returns a `retriableError`
3. The zeroed entries survive to the next iteration where they're interpreted as a replace with empty `Old.Path`

## Fix

Add a guard in `updateRequireDirectives` to skip entries where `require.Mod.Path` is empty:

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
