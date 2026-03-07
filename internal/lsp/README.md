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

The server accepts `initializationOptions` to configure features. All options default to enabled (`true`).

| Option | Type | Default | Description |
|---|---|---|---|
| `formatting` | `bool` | `true` | Enable `textDocument/formatting` (schema key ordering) |

### Neovim example

```lua
lspconfig.scenarigo.setup({
  init_options = {
    formatting = false, -- disable formatting if you use another YAML formatter
  },
})
```

### Vim (vim-lsp) example

```vim
au User lsp_setup call lsp#register_server(#{
  \ name: 'scenarigo',
  \ cmd: ['scenarigo', 'lsp'],
  \ allowlist: ['yaml'],
  \ initialization_options: {'formatting': v:false},
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
| `textDocument/references` | Not yet | Find references to variables |
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
| Template dot completion | Supported | `{{assert.` suggests `contains`, `notZero`, etc. |
| Partial match filtering | Supported | Filters candidates by prefix as you type |
| File path completion | Supported | Filesystem-based candidates for `include`, `plugins`, `scenarios`, `pluginDirectory` |
| Plugin export completion | Not yet | Variables/functions exported by plugins |

### Diagnostics

| Feature | Status | Description |
|---|---|---|
| Unknown key detection | Supported | Warns on keys not in the schema |
| Enum value validation | Supported | Warns on values not in the allowed set (lists allowed values) |
| YAML syntax errors | Supported | Reports parse failures |
| Type checking | Not yet | Type mismatches (string/int/bool/duration) |
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

```
internal/lsp/
  server_test.go        # testClient infrastructure, TestServer_Initialize, TestServer_Fixtures
  fixture_test.go       # YAML fixture loader, parseCursorMarker, per-operation runners
  integration_test.go   # Session-level integration tests (open -> edit -> complete -> hover -> close)
  server_fuzz_test.go   # FuzzGetTemplateContext, FuzzCompleteTemplate
  testdata/
    completion/         # 15 completion test fixtures
    diagnostics/        # 4 diagnostics test fixtures
    definition/         # 1 definition test fixture
    hover/              # 3 hover test fixtures
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
