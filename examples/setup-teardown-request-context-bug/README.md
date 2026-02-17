# Setup Teardown RequestContext Bug Example

This example reproduces a bug where `ctx.RequestContext()` is already cancelled when passed to the teardown function registered via `plugin.RegisterSetupEachScenario`.

## Description

When a plugin registers a setup function via `plugin.RegisterSetupEachScenario` and returns a teardown function, the `ctx.RequestContext()` passed to the teardown is already cancelled at the time it is called.

## Running

```bash
make build
make test/examples/setup-teardown-request-context-bug
```

## Expected Output

```
OK: teardown received valid RequestContext
```

## Actual Output

```
BUG: teardown received already-cancelled RequestContext: context canceled
```
