# scenarigo LSP Server

Language Server Protocol implementation for scenarigo configuration files (`scenarigo.yaml`) and test scenario files (`.yaml`).

## Usage

```bash
scenarigo lsp
```

The LSP server communicates over stdio using JSON-RPC 2.0. Configure your editor to use it as an LSP client.

## Editor Setup

### Neovim (nvim-lspconfig)

Add the following to your Neovim configuration (e.g., `~/.config/nvim/init.lua` or a file loaded by it):

```lua
local lspconfig = require("lspconfig")
local configs = require("lspconfig.configs")

if not configs.scenarigo then
  configs.scenarigo = {
    default_config = {
      cmd = { "scenarigo", "lsp" },
      filetypes = { "yaml" },
      root_dir = lspconfig.util.root_pattern("scenarigo.yaml", ".git"),
      settings = {},
    },
  }
end

lspconfig.scenarigo.setup({})
```

If you only want the LSP active for scenarigo-related YAML files, use `root_dir` to scope it — the server will only start when `scenarigo.yaml` or `.git` is found in a parent directory.

### Vim (vim-lsp)

Using [vim-lsp](https://github.com/prabirshrestha/vim-lsp), add the following to your `.vimrc`:

```vim
if executable('scenarigo')
  au User lsp_setup call lsp#register_server(#{
    \ name: 'scenarigo',
    \ cmd: ['scenarigo', 'lsp'],
    \ allowlist: ['yaml'],
    \ root_uri: {server_info->
    \   lsp#utils#path_to_uri(
    \     lsp#utils#find_nearest_parent_file_directory(
    \       lsp#utils#get_buffer_path(),
    \       ['scenarigo.yaml', '.git']))},
    \ })
endif
```

### Neovim (manual, without nvim-lspconfig)

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = "yaml",
  callback = function()
    vim.lsp.start({
      name = "scenarigo",
      cmd = { "scenarigo", "lsp" },
      root_dir = vim.fs.root(0, { "scenarigo.yaml", ".git" }),
    })
  end,
})
```

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
| `textDocument/publishDiagnostics` | Supported | Unknown key warnings, enum validation, YAML syntax errors, template expression errors |
| `textDocument/documentSymbol` | Supported | Hierarchical outline tree |
| `textDocument/codeAction` | Supported | "Did you mean?" quick fix for unknown fields |
| `textDocument/references` | Supported | Find references to variables |
| `textDocument/rename` | Not yet | Rename symbols |
| `textDocument/signatureHelp` | Supported | Template function signatures (e.g., `assert.contains <- expected`) |
| `workspace/symbol` | Not yet | Workspace-wide symbol search |

### Completion

| Feature | Status | Description |
|---|---|---|
| YAML key completion | Supported | Schema-based key suggestions; already-present keys are excluded |
| Enum value completion | Supported | e.g., `protocol: ` suggests `http`, `grpc` |
| Protocol-aware completion | Supported | `request`/`expect` fields change based on `protocol` value |
| Template variable completion | Supported | Inside `{{`, suggests `vars`, `secrets`, `assert`, etc.; `{` and `.` are trigger characters inside `{{` only, so candidates appear without a manual request while typing `{{vars.` |
| Template dot completion | Supported | `{{assert.` suggests `contains`, `notZero`, etc.; `{{vars.`/`{{secrets.`/`{{steps.` suggests keys from document and config; `{{plugins.` suggests plugin names |
| Partial match filtering | Supported | Filters candidates by prefix as you type |
| File path completion | Supported | Filesystem-based candidates for `include`, `plugins`, `scenarios`, `pluginDirectory` |
| Plugin name completion | Supported | `{{plugins.` suggests plugin names from document and config |
| Plugin export completion | Supported | `{{plugins.grpc.` suggests exported functions/variables by parsing Go source (resolved via `PluginConfig.Src` in `scenarigo.yaml`) |

### Diagnostics

| Feature | Status | Description |
|---|---|---|
| Unknown key detection | Supported | Warns on keys not in the schema |
| Enum value validation | Supported | Warns on values not in the allowed set (lists allowed values); an empty value is accepted where scenarigo reads it as the default (`schemaVersion`, HTTP `method`, `telemetry.tracing.exporter`); `protocol` and HTTP `method` only get an information diagnostic, because plugins can register more protocols and the runner sends any method |
| YAML syntax errors | Supported | Reports parse failures; a text estimated to cost the parser too much memory or stack counts as one |
| Type checking | Supported | Type mismatches (string/int/bool/object/array); template expressions are accepted |
| Required field validation | Supported | Warns when required fields are missing (e.g., `url` in HTTP request) |
| Template syntax errors | Supported | Parses `{{ }}` with scenarigo's own template parser and reports an error where `scenarigo run` would fail, including an operand the parser leaves empty (`{{a ==}}`); an operand missing on a branch that may not run (`{{a ?? }}`, `{{t ? 1 : }}`) is a warning, and everything else in such a branch, other than a syntax error, is one level lower; a template over 1 MiB is not parsed; only fields scenarigo evaluates are checked (`vars` and `secrets` of a scenario, the steps' `if`, `vars`, `secrets`, `bind` and, except on an `include` step, `request`, `expect`, `ref`, and `vars`, `secrets`, `protocols` of `scenarigo.yaml`) |
| Template names | Supported | Warns on unknown root names (other than `ctx`, `vars`, `secrets`, `plugins`, `steps`, `request`, `response`, `env`, `assert`, `$` and the built-in functions), unknown `assert.x`, and `steps.<id>` without a step of that id that has run before; an undefined `vars.x`/`secrets.x`, and `plugins.<name>` not declared in the scenario or a scenario that includes it, are only information because a plugin setup can add them (the plugins of `scenarigo.yaml` are not template names). Names are checked in scenarigo's evaluation order (a step's `if` and `vars` cannot use its own `vars`; `bind` and ids are visible to later steps), aliases and merge keys are resolved (what they bring is checked once, where it is first used), and the `bind` of an `include` step sees the included scenario's steps, vars, secrets and plugins. The scenarios that include a file (found by one walk of the workspace, then updated from the open buffers) lend it their plugins, vars and secrets, including those of the include step; a workspace that cannot be read whole does not prove a name undefined, a scenario outside the workspace root is not seen, and one behind a symlinked directory or in a hidden directory, `vendor` or `node_modules` is seen only once it has been opened. In a file with `#@` annotations, `vars`, `secrets`, `steps` and `plugins` names are not checked, because ytt can fill them in. The `protocols` options of `scenarigo.yaml` are evaluated in each step's context, so only their syntax, root names and `assert.x` are checked. A path directly on the left of `??` or inside `defined()` may be missing; `defined()` of anything else is an error |
| ytt templates | Supported | `ytt/v1` documents have their own schema, also next to scenario documents in one file; a null value followed by a `#@` annotation (`title: #@ params.title`) is filled in by ytt and not type-checked; a file with `#@` that does not parse is not reported, because it is YAML only after ytt evaluates it |

### Other Features

| Feature | Status | Description |
|---|---|---|
| Hover | Supported | Shows field name, type, description, and allowed values in Markdown |
| Definition | Supported | Jump from `include` to target file, `{{vars.x}}`/`{{secrets.x}}` to declaration (including bind, last-write-wins), `{{plugins.name}}` to plugin declaration, `{{plugins.name.Symbol}}` to Go source |
| Document Symbol | Supported | Hierarchical symbol tree from YAML structure; steps use their `title` as the symbol name |
| Code Action | Supported | Suggests quick fixes for unknown fields using Levenshtein distance (<= 3) |
| Schema auto-detection | Supported | Detects config, scenario, or ytt input from the `schemaVersion` value |
| Non-scenarigo YAML coexistence | Supported | Silent when `schemaVersion` key is absent; skips files with `# yaml-language-server:` modeline |
| Broken YAML handling | Supported | Caches last successful AST + text-based analysis hybrid |
| Position encoding | Supported | Columns are exchanged in UTF-16 code units (the LSP default), or in bytes when the client offers `utf-8` in `general.positionEncodings` |

## Schema Detection

The LSP uses the `schemaVersion` key to decide how to handle a YAML file:

| Condition | Behavior |
|---|---|
| `schemaVersion: scenario/v1` | Full LSP features using the scenario schema |
| `schemaVersion: config/v1` | Full LSP features using the config schema |
| `schemaVersion: ytt/v1` | Completion and diagnostics using the ytt input schema (`files`) |
| `schemaVersion:` present, value empty or unrecognized | Defaults to scenario schema (most common type) so that completion and diagnostics remain active while the user is typing |
| `schemaVersion` key absent | File is treated as non-scenarigo YAML — all features are disabled (no completions, diagnostics, hover, etc.) |
| `# yaml-language-server: ...` modeline present | File is skipped entirely (not stored in memory), avoiding conflicts with other YAML language servers |

This design means the server can be registered for all `*.yaml` files without interfering with non-scenarigo YAML. When another YAML language server (e.g., [yaml-language-server](https://github.com/redhat-developer/yaml-language-server)) is also active, both servers can coexist: scenarigo stays silent on files it does not own, and files with a `yaml-language-server` modeline are fully ignored.

## Supported Schemas

| Schema | Version | Target Files |
|---|---|---|
| Scenario | `scenario/v1` | Test scenario YAML files |
| Config | `config/v1` | `scenarigo.yaml` configuration files |
| ytt input | `ytt/v1` | Documents listing the ytt files evaluated into scenarios |

## Directory Structure

```
internal/lsp/
  server.go             # LSP server (handlers, completion, hover, diagnostics, codeAction)
  protocol.go           # LSP protocol type definitions (JSON-RPC, LSP types)
  document.go           # Document management (open/change/close + AST cache)
  transport.go          # JSON-RPC framing over stdio
  position.go           # Column unit conversions (bytes, runes, UTF-16)
  uri.go                # file URI <-> path conversions
  template_diagnostics.go # Template expression diagnostics ({{ }})
  yamlview.go           # Alias and merge key resolution
  include.go            # Included and including scenarios
  ytt.go                # ytt annotation handling
  yamlutil/
    position.go         # YAML AST analysis (FindNodeAtPosition, GetCursorContext)

internal/yamlschema/
  schema.go             # Field schema types (FieldInfo, FindField, ChildFields)
  scenarigo.go          # Config and scenario field definitions shared with other tools
```

## Testing

The LSP server has three tiers of tests, each with a different purpose and cost.

### Test Tiers

| Tier | Files | Run with | Purpose |
|---|---|---|---|
| **Unit / Fuzz** | `position_test.go`, `template_diagnostics_test.go`, `template_bounds_test.go`, `yamlview_test.go`, `transport_test.go`, `uri_test.go`, `server_fuzz_test.go`, `yamlutil/*_test.go` | `go test ./internal/lsp/...` | Individual function correctness, edge-case detection |
| **Fixture + Session** | `fixture_test.go` + `testdata/`, `integration_test.go`, `transport_test.go` | `go test ./internal/lsp/...` | Protocol-level behavior via in-process `io.Pipe` |
| **E2E (binary + Neovim)** | `e2e_test.go` + `testdata/e2e_nvim/` | `make test/lsp-e2e` | Real binary over OS pipes; real editor integration |

#### Unit / Fuzz Tests

Pure function calls without starting the server. Fuzz tests (`FuzzGetTemplateContext`, `FuzzCompleteTemplate`, `FuzzTemplateDiagnostics`, `FuzzGetCursorContext`) detect panics and edge cases via random input. The drift tests in `template_diagnostics_test.go` compare the template names the server knows with `assert/template_helpers.go`, `context/extract.go` and the template package.

#### Fixture Tests (data-driven)

Single-operation tests declared in YAML under `testdata/{operation}/`. Each fixture specifies a document (with `$0` cursor marker), an operation, and expected results. **New tests should be added here whenever possible** — no Go code changes needed.

Supported operations: `completion`, `diagnostics`, `hover`, `definition`, `documentSymbol`, `signatureHelp`, `references`.

#### Session Tests (multi-step Go tests)

`integration_test.go` contains tests that require multi-step interactions impossible to express in YAML fixtures: editing a document and re-completing, opening multiple documents simultaneously, verifying diagnostics update after edits, code action flows that depend on diagnostics, and full lifecycle (initialize → operations → close → shutdown).

#### E2E Tests

Gated behind the `e2e_lsp` build tag. These tests build the real `scenarigo` binary via `make build` and test:

- **Binary stdio tests** — launch `scenarigo lsp` as a subprocess, communicate via stdin/stdout pipes. Tests full lifecycle, shutdown/exit behavior, stdin close handling, and config file reads from disk.
- **Neovim integration** (`TestE2E_Neovim`) — launches headless Neovim (`nvim --headless --clean`) with a Lua test script that uses `vim.lsp.start` to connect to the server. Tests client attach, completion, hover, diagnostics, and diagnostics-after-edit through Neovim's real LSP client. Skipped if `nvim` is not available.

### File Layout

```
internal/lsp/
  server_test.go          # testClient (helpers, typed methods), TestServer_Initialize
  fixture_test.go         # YAML fixture loader, parseCursorMarker, per-operation runners
  integration_test.go     # Multi-step session tests (TestEditorSession_*)
  server_fuzz_test.go     # FuzzGetTemplateContext, FuzzCompleteTemplate, FuzzTemplateDiagnostics
  template_diagnostics_test.go # Drift tests: assert helpers, context roots, built-in functions; diagnostics scale linearly
  template_bounds_test.go # Hostile templates, files and alias fan-out
  yamlview_test.go        # Aliases, merge keys and key names
  position_test.go        # Column conversions between bytes, runes and UTF-16
  transport_test.go       # Message framing and protocol error responses
  uri_test.go             # file URI <-> path conversions
  e2e_test.go             # [e2e_lsp] Binary subprocess + Neovim headless tests
  testdata/
    completion/           # Completion fixtures
    diagnostics/          # Diagnostics fixtures
    definition/           # Definition fixtures
    hover/                # Hover fixtures
    documentSymbol/       # Document symbol fixtures
    signatureHelp/        # Signature help fixtures
    references/           # References fixtures
    e2e_nvim/
      lsp_test.lua        # Neovim Lua test script
  yamlutil/
    position_test.go      # GetCursorContext unit tests
    position_fuzz_test.go # FuzzGetCursorContext
```

### Adding a Fixture Test

Add a YAML file under `testdata/{operation}/` and `TestServer_Fixtures` will pick it up automatically. No code changes needed.

```yaml
---
name: "test name"
document: |
  schemaVersion: scenario/v1
  steps:
    - protocol: $0
operation: completion
expect:
  completionLabels:
    contains: [http, grpc]
    excludes: [url]
```

`$0` marks the cursor position (required for completion, hover, definition, signatureHelp, and references).

A diagnostics fixture can check the severity and range of a diagnostic, and create files next to the document (for example `scenarigo.yaml`). Lines and characters are 0-based; characters count UTF-16 code units.

```yaml
---
name: "unknown root name"
files:
  scenarigo.yaml: |
    schemaVersion: config/v1
document: |
  schemaVersion: scenario/v1
  vars:
    a: '{{hots}}'
operation: diagnostics
expect:
  diagnostics:
    - message: 'unknown template name "hots"'
      severity: 2
      startLine: 2
      startChar: 8
      endLine: 2
      endChar: 12
```

### Running Tests

```bash
# Unit + Fixture + Session tests (default, fast)
go test ./internal/lsp/...

# E2E tests (builds binary, launches subprocesses)
make test/lsp-e2e

# All together
go test -tags e2e_lsp ./internal/lsp/...
```
