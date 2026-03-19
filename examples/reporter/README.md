# reporter

This example demonstrates how a Go plugin can interact with scenarigo's `Reporter` API to emit logs (both `Print*` and `Log*`) from plugin code such as setup hooks, helper functions, and teardown routines.

## What it does

- `plugin/src/main.go` provides an `Increment` helper that writes the current counter value using both `Reporter.Printf` (always shown) and `Reporter.Log` (shown only when tests fail or verbose output is enabled). Functions that take `*plugin.Context` as the first argument automatically receive the current context. Test scenarios can call them without explicitly passing `ctx`.
- The scenario at `scenarios/echo.yaml` loads the plugin, calls `Increment`, and sends a request to the plugin-managed server.
- Avoid writing directly to `os.Stdout`/`os.Stderr` inside plugins; use `Reporter().Print*`/`Log*` instead so scenarigo can manage and display logs consistently.

## How to run

1. Build the plugin

   ```bash
   scenarigo plugin build
   ```

2. Execute the scenario to see the reporter output coming from the plugin

   ```bash
   scenarigo run
   ```

The command prints the `Reporter.Printf` messages (e.g., `count: 1`) regardless of the test result, while the `Reporter.Log` output appears only when the scenario fails or you add `-v/--verbose`.
