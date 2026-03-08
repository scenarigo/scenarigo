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

## Configuration

The server accepts `initializationOptions` to configure features.

| Option | Type | Default | Description |
|---|---|---|---|
| `formatting` | `bool` | `false` | Enable `textDocument/formatting` (schema key ordering) |

### Neovim example

```lua
lspconfig.scenarigo.setup({
  init_options = {
    formatting = true, -- enable schema-based key ordering
  },
})
```

### Vim (vim-lsp) example

```vim
au User lsp_setup call lsp#register_server(#{
  \ name: 'scenarigo',
  \ cmd: ['scenarigo', 'lsp'],
  \ allowlist: ['yaml'],
  \ initialization_options: {'formatting': v:true},
  \ })
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
| `textDocument/publishDiagnostics` | Supported | Unknown key warnings, enum validation, YAML syntax errors |
| `textDocument/documentSymbol` | Supported | Hierarchical outline tree |
| `textDocument/codeAction` | Supported | "Did you mean?" quick fix for unknown fields |
| `textDocument/formatting` | Supported | Key reordering to match schema field order |
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
| Template variable completion | Supported | Inside `{{`, suggests `vars`, `secrets`, `assert`, etc. |
| Template dot completion | Supported | `{{assert.` suggests `contains`, `notZero`, etc.; `{{vars.`/`{{secrets.`/`{{steps.` suggests keys from document and config |
| Partial match filtering | Supported | Filters candidates by prefix as you type |
| File path completion | Supported | Filesystem-based candidates for `include`, `plugins`, `scenarios`, `pluginDirectory` |
| Plugin export completion | Not yet | Variables/functions exported by plugins |

### Diagnostics

| Feature | Status | Description |
|---|---|---|
| Unknown key detection | Supported | Warns on keys not in the schema |
| Enum value validation | Supported | Warns on values not in the allowed set (lists allowed values) |
| YAML syntax errors | Supported | Reports parse failures |
| Type checking | Supported | Type mismatches (string/int/bool/object/array); template expressions are accepted |
| Required field validation | Supported | Warns when required fields are missing (e.g., `method`/`url` in HTTP request) |

### Other Features

| Feature | Status | Description |
|---|---|---|
| Hover | Supported | Shows field name, type, description, and allowed values in Markdown |
| Definition | Supported | Jump from `include` value to the target file; also works for `plugins`/`scenarios` values |
| Document Symbol | Supported | Hierarchical symbol tree from YAML structure; steps use their `title` as the symbol name |
| Code Action | Supported | Suggests quick fixes for unknown fields using Levenshtein distance (<= 3) |
| Schema auto-detection | Supported | Detects config vs. scenario from the `schemaVersion` value |
| Non-scenarigo YAML coexistence | Supported | Silent when `schemaVersion` key is absent; skips files with `# yaml-language-server:` modeline |
| Broken YAML handling | Supported | Caches last successful AST + text-based analysis hybrid |

## Schema Detection

The LSP uses the `schemaVersion` key to decide how to handle a YAML file:

| Condition | Behavior |
|---|---|
| `schemaVersion: scenario/v1` | Full LSP features using the scenario schema |
| `schemaVersion: config/v1` | Full LSP features using the config schema |
| `schemaVersion:` present, value empty or unrecognized | Defaults to scenario schema (most common type) so that completion and diagnostics remain active while the user is typing |
| `schemaVersion` key absent | File is treated as non-scenarigo YAML — all features are disabled (no completions, diagnostics, hover, etc.) |
| `# yaml-language-server: ...` modeline present | File is skipped entirely (not stored in memory), avoiding conflicts with other YAML language servers |

This design means the server can be registered for all `*.yaml` files without interfering with non-scenarigo YAML. When another YAML language server (e.g., [yaml-language-server](https://github.com/redhat-developer/yaml-language-server)) is also active, both servers can coexist: scenarigo stays silent on files it does not own, and files with a `yaml-language-server` modeline are fully ignored.

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

The LSP server has three tiers of tests, each with a different purpose and cost.

### Test Tiers

| Tier | Files | Run with | Purpose |
|---|---|---|---|
| **Unit / Fuzz** | `server_fuzz_test.go`, `yamlutil/*_test.go` | `go test ./internal/lsp/...` | Individual function correctness, edge-case detection |
| **Fixture + Session** | `fixture_test.go` + `testdata/`, `integration_test.go` | `go test ./internal/lsp/...` | Protocol-level behavior via in-process `io.Pipe` |
| **E2E (binary + Neovim)** | `e2e_test.go` + `testdata/e2e_nvim/` | `make test/lsp-e2e` | Real binary over OS pipes; real editor integration |

#### Unit / Fuzz Tests

Pure function calls without starting the server. Fuzz tests (`FuzzGetTemplateContext`, `FuzzCompleteTemplate`, `FuzzGetCursorContext`) detect panics and edge cases via random input.

#### Fixture Tests (data-driven)

Single-operation tests declared in YAML under `testdata/{operation}/`. Each fixture specifies a document (with `$0` cursor marker), an operation, and expected results. **New tests should be added here whenever possible** — no Go code changes needed.

Supported operations: `completion`, `diagnostics`, `hover`, `definition`, `documentSymbol`, `formatting`, `signatureHelp`, `references`.

#### Session Tests (multi-step Go tests)

`integration_test.go` contains tests that require multi-step interactions impossible to express in YAML fixtures: editing a document and re-completing, opening multiple documents simultaneously, verifying diagnostics update after edits, code action flows that depend on diagnostics, and full lifecycle (initialize → operations → close → shutdown).

#### E2E Tests

Gated behind the `e2e_lsp` build tag. These tests build the real `scenarigo` binary via `make build` and test:

- **Binary stdio tests** — launch `scenarigo lsp` as a subprocess, communicate via stdin/stdout pipes. Tests full lifecycle, shutdown/exit behavior, stdin close handling, config file reads from disk, and formatting.
- **Neovim integration** (`TestE2E_Neovim`) — launches headless Neovim (`nvim --headless --clean`) with a Lua test script that uses `vim.lsp.start` to connect to the server. Tests client attach, completion, hover, diagnostics, and diagnostics-after-edit through Neovim's real LSP client. Skipped if `nvim` is not available.

### File Layout

```
internal/lsp/
  server_test.go          # testClient (helpers, typed methods), TestServer_Initialize
  fixture_test.go         # YAML fixture loader, parseCursorMarker, per-operation runners
  integration_test.go     # Multi-step session tests (TestEditorSession_*)
  server_fuzz_test.go     # FuzzGetTemplateContext, FuzzCompleteTemplate
  e2e_test.go             # [e2e_lsp] Binary subprocess + Neovim headless tests
  testdata/
    completion/           # Completion fixtures
    diagnostics/          # Diagnostics fixtures
    definition/           # Definition fixtures
    hover/                # Hover fixtures
    documentSymbol/       # Document symbol fixtures
    formatting/           # Formatting fixtures
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

### Running Tests

```bash
# Unit + Fixture + Session tests (default, fast)
go test ./internal/lsp/...

# E2E tests (builds binary, launches subprocesses)
make test/lsp-e2e

# All together
go test -tags e2e_lsp ./internal/lsp/...
```
