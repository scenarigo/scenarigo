# Test Report Example

This example demonstrates Scenarigo's test report features, including JSON and JUnit XML output formats for CI/CD integration.

## Overview

Scenarigo can generate test reports in multiple formats:
- **JSON format**: Detailed test results for programmatic analysis
- **JUnit XML format**: CI/CD tool integration (Jenkins, GitLab CI, GitHub Actions, etc.)
- **Console summary**: Test execution summary displayed in terminal

This example uses a local HTTP server (started via plugin) instead of external APIs, making it runnable offline.

## File Structure

```
examples/report/
├── scenarigo.yaml           # Configuration with report settings
├── plugin/
│   └── main.go              # HTTP server plugin for testing
├── scenarios/
│   ├── success.yaml         # All passing tests
│   ├── failed.yaml          # Intentionally failing test
│   └── conditional.yaml     # Conditional execution with skip
├── gen/                     # Built plugins (auto-generated)
│   └── server.so
├── report.json             # Generated JSON report (after run)
└── junit.xml               # Generated JUnit XML report (after run)
```

## Usage

```bash
cd examples/report
make run-with-fail
```

This will generate comprehensive reports including success, failure, and skip examples.
After execution, `report.json` and `junit.xml` will be generated in the current directory.

> **Note**: The default configuration (`scenarigo run` without make) excludes `failed.yaml` to prevent CI failures.

## Report Formats

### JSON Report

```json
{
  "result": "failed",
  "files": [
    {
      "name": "scenarios/success.yaml",
      "result": "passed",
      "duration": "0.123s",
      "scenarios": [
        {
          "name": "Successful API Tests",
          "result": "passed",
          "steps": [...]
        }
      ]
    }
  ]
}
```

### JUnit XML Report

```xml
<testsuites>
  <testsuite name="scenarios/success.yaml" time="0.123" tests="1" failures="0">
    <testcase name="Successful API Tests" file="scenarios/success.yaml" time="0.123"/>
  </testsuite>
  <testsuite name="scenarios/failed.yaml" time="0.067" tests="1" failures="1">
    <testcase name="Failed API Tests" file="scenarios/failed.yaml" time="0.067">
      <failure message="This step will fail - wrong status code">
        expected "404" but got "OK"
      </failure>
    </testcase>
  </testsuite>
</testsuites>
```
