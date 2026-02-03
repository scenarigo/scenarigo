# Scenario-Level Retry ScenarioFilepath Bug Example

This example reproduces a bug where `ctx.ScenarioFilepath()` becomes empty during scenario-level retry.

## Description

When a scenario has scenario-level retry configuration and retry is triggered, `ctx.ScenarioFilepath()` accessible from plugin functions becomes empty on retry attempts, even though it should contain the path to the scenario file.

## Running

```bash
make build
make test/examples/scenario-retry-scenariofilepath-bug
```

## Expected Output

```
Captured os.Stdout output:
CheckFilepath called - attempt 1
  ScenarioFilepath: '/path/to/scenarigo/examples/scenario-retry-scenariofilepath-bug/scenarios/filepath-check.yaml'
CheckFilepath called - attempt 2
  ScenarioFilepath: '/path/to/scenarigo/examples/scenario-retry-scenariofilepath-bug/scenarios/filepath-check.yaml'
```

## Actual Output

```
Captured os.Stdout output:
CheckFilepath called - attempt 1
  ScenarioFilepath: '/path/to/scenarigo/examples/scenario-retry-scenariofilepath-bug/scenarios/filepath-check.yaml'
CheckFilepath called - attempt 2
  ScenarioFilepath: ''
  WARNING: ScenarioFilepath is EMPTY!
```

**Bug**: On attempt 1 (first execution), ScenarioFilepath contains the correct path. On attempt 2 (retry), ScenarioFilepath is empty.
