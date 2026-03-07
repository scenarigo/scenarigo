# scenarigo LSP Server

Language Server Protocol implementation for scenarigo configuration files (`scenarigo.yaml`) and test scenario files (`.yaml`).

## Usage

```bash
scenarigo lsp
```

The LSP server communicates over stdio using JSON-RPC 2.0. Configure your editor to use it as an LSP client.

## Feature Matrix

### LSP Methods

| Method | Status | Description |
|---|---|---|
| `initialize` / `shutdown` | Supported | Lifecycle management |
| `textDocument/didOpen` | Supported | Document management (full sync) |
| `textDocument/didChange` | Supported | Document update |
| `textDocument/didClose` | Supported | Document close |
| `textDocument/completion` | Supported | Key, value, and template completion |
| `textDocument/hover` | Supported | Field description, type, and enum values |
| `textDocument/definition` | Supported | Jump to included files |
| `textDocument/publishDiagnostics` | Supported | Unknown key warnings, enum validation, YAML syntax errors |
| `textDocument/documentSymbol` | Supported | Hierarchical outline tree |
| `textDocument/codeAction` | Supported | "Did you mean?" quick fix for unknown fields |
| `textDocument/formatting` | Not yet | YAML formatting |
| `textDocument/references` | Not yet | Find references to variables |
| `textDocument/rename` | Not yet | Rename symbols |
| `textDocument/signatureHelp` | Not yet | Template function signatures |
| `workspace/symbol` | Not yet | Workspace-wide symbol search |

### Completion

| Feature | Status | Description |
|---|---|---|
| YAML key completion | Supported | Schema-based key suggestions; already-present keys are excluded |
| Enum value completion | Supported | e.g., `protocol: ` suggests `http`, `grpc` |
| Protocol-aware completion | Supported | `request`/`expect` fields change based on `protocol` value |
| Template variable completion | Supported | Inside `{{`, suggests `vars`, `secrets`, `assert`, etc. |
| Template dot completion | Supported | `{{assert.` suggests `contains`, `notZero`, etc. |
| Partial match filtering | Supported | Filters candidates by prefix as you type |
| File path completion | Not yet | Path candidates for `include`, `plugins`, `scenarios` |
| Plugin export completion | Not yet | Variables/functions exported by plugins |

### Diagnostics

| Feature | Status | Description |
|---|---|---|
| Unknown key detection | Supported | Warns on keys not in the schema |
| Enum value validation | Supported | Warns on values not in the allowed set (lists allowed values) |
| YAML syntax errors | Supported | Reports parse failures |
| Type checking | Not yet | Type mismatches (string/int/bool/duration) |
| Required field validation | Not yet | Warns when required fields are missing |

### Other Features

| Feature | Status | Description |
|---|---|---|
| Hover | Supported | Shows field name, type, description, and allowed values in Markdown |
| Definition | Supported | Jump from `include` value to the target file; also works for `plugins`/`scenarios` values |
| Document Symbol | Supported | Hierarchical symbol tree from YAML structure; steps use their `title` as the symbol name |
| Code Action | Supported | Suggests quick fixes for unknown fields using Levenshtein distance (<= 3) |
| Schema auto-detection | Supported | Detects config vs. scenario from the `schemaVersion` value |
| Broken YAML handling | Supported | Caches last successful AST + text-based analysis hybrid |

## Supported Schemas

| Schema | Version | Target Files |
|---|---|---|
| Scenario | `scenario/v1` | Test scenario YAML files |
| Config | `config/v1` | `scenarigo.yaml` configuration files |

## Directory Structure

```
internal/lsp/
  server.go             # LSP server (handlers, completion, hover, diagnostics, codeAction)
  protocol.go           # LSP protocol type definitions (JSON-RPC, LSP types)
  document.go           # Document management (open/change/close + AST cache)
  schema/
    schema.go           # Schema type definitions (FieldInfo, FindField, ChildFields)
    scenarigo.go        # scenarigo-specific schema (Config, Scenario, HTTP, gRPC)
  yamlutil/
    position.go         # YAML AST analysis (FindNodeAtPosition, GetCursorContext)
```

## Testing

```
internal/lsp/
  server_test.go        # testClient infrastructure, TestServer_Initialize, TestServer_Fixtures
  fixture_test.go       # YAML fixture loader, parseCursorMarker, per-operation runners
  integration_test.go   # Session-level integration tests (open -> edit -> complete -> hover -> close)
  server_fuzz_test.go   # FuzzGetTemplateContext, FuzzCompleteTemplate
  testdata/
    completion/         # 9 completion test fixtures
    diagnostics/        # 3 diagnostics test fixtures
    definition/         # 1 definition test fixture
    hover/              # 2 hover test fixtures
    documentSymbol/     # 1 document symbol test fixture
  yamlutil/
    position_test.go    # GetCursorContext unit tests
    position_fuzz_test.go # FuzzGetCursorContext
```

### Adding a Test

Add a YAML file under `testdata/{operation}/` and `TestServer_Fixtures` will pick it up automatically. No code changes needed.

```yaml
---
name: "test name"
document: |
  schemaVersion: scenario/v1
  steps:
    - protocol: $0
operation: completion  # completion | diagnostics | definition | hover | documentSymbol
expect:
  completionLabels:
    contains: [http, grpc]
    excludes: [url]
```

`$0` marks the cursor position (required for completion, hover, and definition operations).
