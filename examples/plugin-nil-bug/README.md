# Plugin Nil Argument Bug Reproduction

This example demonstrates a bug where `null` values passed to plugin functions are incorrectly converted to `*plugin.Context` instead of `nil`.

## Bug Description

When passing `null` variables to plugin functions:

```yaml
vars:
  nil: null
steps:
- vars:
    result: '{{ plugins.main.UnaryFun(vars.nil) }}'
  ref: '{{ plugins.main.Nop }}'
```

**Expected**: `UnaryFun` receives `nil`
**Actual**: `UnaryFun` receives `*plugin.Context`

## How to Run

```bash
make test/examples/plugin-nil-bug
```

## Output

```
UnaryFun(vars.nil)
  arg == nil: false
  type: *context.Context
  🐛 BUG: received *plugin.Context instead of nil

UnaryFun(vars.notNil)
  arg == nil: false
  type: string

BinaryFun(vars.nil, vars.nil)
  arg1 == nil: false, type: *context.Context
  🐛 BUG: arg1 received *plugin.Context instead of nil
  arg2 == nil: false, type: *context.Context
  🐛 BUG: arg2 received *plugin.Context instead of nil

BinaryFun(vars.nil, vars.notNil)
  arg1 == nil: false, type: *context.Context
  🐛 BUG: arg1 received *plugin.Context instead of nil
  arg2 == nil: false, type: *context.Context
  🐛 BUG: arg2 received *plugin.Context instead of nil

BinaryFun(vars.notNil, vars.nil)
  arg1 == nil: false, type: string
  arg2 == nil: false, type: *context.Context
  🐛 BUG: arg2 received *plugin.Context instead of nil
```

## Key Findings

1. **UnaryFun(vars.nil)**: Receives `*context.Context` instead of `nil`
2. **BinaryFun(vars.nil, vars.nil)**: Both arguments receive `*context.Context`
3. **BinaryFun(vars.nil, vars.notNil)**: Both arguments receive `*context.Context` (even the non-null one!)
4. **BinaryFun(vars.notNil, vars.nil)**: Second argument receives `*context.Context`

The most interesting case is #3, where the first null argument causes the second non-null argument to also be converted to `*context.Context`.
