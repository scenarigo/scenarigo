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

Neovim announces the dynamic registration of file watchers on macOS and Windows only. To let the server follow files changed outside the editor (`scenarigo.yaml`, included scenarios, plugin sources) on Linux too, add the capability (Neovim then watches with `inotifywait` when it is installed, and with one watcher per directory otherwise):

```lua
lspconfig.scenarigo.setup({
  capabilities = vim.tbl_deep_extend("force", vim.lsp.protocol.make_client_capabilities(), {
    workspace = { didChangeWatchedFiles = { dynamicRegistration = true } },
  }),
})
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
| `textDocument/definition` | Supported | Jump to included files, variables, steps, plugins |
| `textDocument/publishDiagnostics` | Supported | Unknown key warnings, enum validation, YAML syntax errors, template expression errors |
| `textDocument/documentSymbol` | Supported | Hierarchical outline tree |
| `textDocument/codeAction` | Supported | "Did you mean?" quick fix for unknown fields |
| `textDocument/references` | Supported | Find references to vars, secrets and steps, including names inside calls and operators (`size(vars.x)`, `vars.x ?? 0`) |
| `textDocument/rename` | Supported | Rename a variable, secret or step id in one YAML document of a scenario: its declarations (top level, steps, `bind`) and the references `textDocument/references` finds. A rename that the server cannot complete is refused with the reason: the name is declared through an alias or merge key, used as an index (`vars["x"]`), or in a template the text scan cannot locate; it belongs to a configuration file or is also defined in `scenarigo.yaml`; the scenario includes another one (vars and secrets) or another scenario of the workspace includes it; there is no workspace root. The new name must be a template name (a letter first, then letters, digits, `-`, `_`) other than `nil`, `defined` and any spelling of `true`, `false` and `null`, and neither defined nor already used in the scenario or `scenarigo.yaml` |
| `textDocument/prepareRename` | Supported | Selects the name under the cursor, or reports why it cannot be renamed |
| `textDocument/signatureHelp` | Supported | Template function signatures (e.g., `assert.contains <- expected`) |
| `workspace/symbol` | Not yet | Workspace-wide symbol search |
| `workspace/didChangeWatchedFiles` | Supported | A change on disk drops what the server derived from the file: the symbols of a plugin source directory, the include index entry of a scenario, and for a directory event everything under the directory; a proto file change drops the compiled proto files, which are also checked against the files they were compiled from before each use, so that changes nothing reports (an import outside the workspace folders, a moved directory) are seen too. Every change, `scenarigo.yaml` and proto files included, publishes the diagnostics of the open documents again. The watchers (`**/*.{yaml,yml}`, `**/*.go`, `**/*.proto`, and `**/*` for creations and deletions, which catches directories) are registered with `client/registerCapability` when the client supports dynamic registration, and the registration is replaced when folders change; plugin sources outside the workspace folders are not watched |
| `workspace/didChangeWorkspaceFolders` | Supported | Folders can be added and removed while the server runs |

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
| gRPC from proto files | Supported | For a gRPC step that reads proto files as scenarigo does (no `client`, reflection disabled, and `files` listed in the step's `proto` options or, when the step has no `proto` options at all, in those of `scenarigo.yaml`: a step's `proto` replaces the whole `proto` of `scenarigo.yaml`; the options are read as scenarigo reads them: an alias resolves to the last anchor of its name in its document, or else in the documents before it, as scenarigo's loader resolves it, and merge keys, tags and block scalars read as at run time; null options are no options; options whose aliases expand to more than 100,000 values get no proto support; paths relative to the scenario, and without `imports` the files a proto file imports relative to the workspace root, where `scenarigo run` runs): service and method names, which replace the whole value with its quotes (`test.v1.Gr` becomes `test.v1.Greeter`), and the fields of request and expected messages, also nested, repeated, map values, oneofs and streamed `messages`, by JSON name, or by proto name when that is what the typed prefix matches; fields already set in the mapping, above or below the cursor and by any of their names, are left out, and in a request message so are the other members of a oneof that is set. Reflection is never used |
| Plugin export completion | Supported | `{{plugins.grpc.` suggests exported functions/variables by parsing Go source (resolved via `PluginConfig.Src` in `scenarigo.yaml`) |

### Diagnostics

| Feature | Status | Description |
|---|---|---|
| Unknown key detection | Supported | Warns on keys not in the schema |
| Enum value validation | Supported | Warns on values not in the allowed set (lists allowed values); an empty value is accepted where scenarigo reads it as the default (`schemaVersion`, HTTP `method`, `telemetry.tracing.exporter`); `protocol` only gets an information diagnostic because plugins can register more protocols |
| YAML syntax errors | Supported | Reports parse failures |
| Type checking | Supported | Type mismatches (string/int/bool/object/array); template expressions are accepted |
| gRPC from proto files | Supported | Warns on unknown services, methods and message fields (proto, text and JSON names are accepted, as scenarigo does), and in a request message on a field set twice by two of its names and on two members of a oneof set to values (null counts for a `google.protobuf.Value` member), which protojson rejects; a key that a merge key brings in and an explicit key overrides is not set twice; well-known messages with a JSON form of their own (`Timestamp`, `Duration`, `Struct`, `Any`, wrappers) are not checked, other ones such as `Empty` are; a template value and a mapping with a template key (an assertion) are not checked; proto files that do not compile give no diagnostics, and a failed compilation is tried again after 5 seconds (a minute after a timeout) |
| Required field validation | Supported | Warns when required fields are missing (e.g., `method`/`url` in HTTP request) |
| Template syntax errors | Supported | Parses `{{ }}` with scenarigo's own template parser and reports an error where `scenarigo run` would fail, including an operand the parser leaves empty (`{{a ==}}`); only fields scenarigo evaluates are checked (`vars` and `secrets` of a scenario, the steps' `if`, `vars`, `secrets`, `request`, `expect`, `bind`, `ref`, and `vars`, `secrets`, `protocols` of `scenarigo.yaml`) |
| Template names | Supported | Warns on unknown root names (other than `ctx`, `vars`, `secrets`, `plugins`, `steps`, `request`, `response`, `env`, `assert`, `$` and the built-in functions), unknown `assert.x`, `steps.<id>` without a step of that id that has run before, and `plugins.<name>` not declared in the file, `scenarigo.yaml` or a scenario that includes the file; an undefined `vars.x`/`secrets.x` is only information because a plugin setup can add variables. Names are checked in scenarigo's evaluation order (a step's `if` and `vars` cannot use its own `vars`; `bind` and ids are visible to later steps), aliases and merge keys are resolved, and the `bind` of an `include` step sees the included scenario's steps and vars. The scenarios that include a file (found by one walk of the workspace, then updated from the open buffers) lend it their plugins and vars, including the vars of the include step. The `protocols` options of `scenarigo.yaml` are evaluated in each step's context, so only their syntax and root names are checked. A path directly on the left of `??` or inside `defined()` may be missing; `defined()` of anything else is an error |
| ytt templates | Supported | `ytt/v1` documents have their own schema, also next to scenario documents in one file; a null value followed by a `#@` annotation (`title: #@ params.title`) is filled in by ytt and not type-checked |

### Other Features

| Feature | Status | Description |
|---|---|---|
| Hover | Supported | Shows field name, type, description, and allowed values in Markdown |
| Definition | Supported | Jump from `include` to target file, `{{vars.x}}`/`{{secrets.x}}` to declaration (including bind, last-write-wins), `{{steps.id}}` to the step's `id`, `{{plugins.name}}` to plugin declaration, `{{plugins.name.Symbol}}` to Go source |
| gRPC hover | Supported | The type and comments of a message field, and the comments of a service or a method, from proto files |
| Document Symbol | Supported | Hierarchical symbol tree from YAML structure; steps use their `title` as the symbol name |
| Code Action | Supported | Suggests quick fixes for unknown fields using Levenshtein distance (<= 3) |
| Schema auto-detection | Supported | Detects config, scenario, or ytt input from the `schemaVersion` value |
| Non-scenarigo YAML coexistence | Supported | Silent when `schemaVersion` key is absent; skips files with `# yaml-language-server:` modeline |
| Broken YAML handling | Supported | Caches last successful AST + text-based analysis hybrid |
| Workspace folders | Supported | Several folders (`workspaceFolders` in `initialize`, `workspace/didChangeWorkspaceFolders`). A document uses the `scenarigo.yaml` of the innermost folder that contains it and has one; scenarios that include it are searched in the outermost folder that contains it. A document outside every folder uses no `scenarigo.yaml`, except when the client gave `rootUri` alone, as before |
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
  rename.go             # textDocument/rename and prepareRename
  workspace.go          # Workspace folders and watched files
  grpcproto.go          # gRPC services, methods and messages from proto files
  navigation.go         # Name chains, step definitions and document spans for navigation
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
| **Unit / Fuzz** | `position_test.go`, `transport_test.go`, `uri_test.go`, `navigation_test.go`, `template_diagnostics_test.go`, `server_fuzz_test.go`, `yamlutil/*_test.go` | `go test ./internal/lsp/...` | Individual function correctness, edge-case detection |
| **Fixture + Session** | `fixture_test.go` + `testdata/`, `integration_test.go` | `go test ./internal/lsp/...` | Protocol-level behavior via in-process `io.Pipe` |
| **E2E (binary + Neovim)** | `e2e_test.go` + `testdata/e2e_nvim/` | `make test/lsp-e2e` | Real binary over OS pipes; real editor integration |

#### Unit / Fuzz Tests

Pure function calls without starting the server. Fuzz tests (`FuzzGetTemplateContext`, `FuzzCompleteTemplate`, `FuzzTemplateDiagnostics`, `FuzzGetCursorContext`) detect panics and edge cases via random input. The drift tests in `template_diagnostics_test.go` compare the template names the server knows with `assert/template_helpers.go`, `context/extract.go` and the template package.

#### Fixture Tests (data-driven)

Single-operation tests declared in YAML under `testdata/{operation}/`. Each fixture specifies a document (with `$0` cursor marker), an operation, and expected results. **New tests should be added here whenever possible** — no Go code changes needed.

Supported operations: `completion`, `diagnostics`, `hover`, `definition`, `documentSymbol`, `signatureHelp`, `references`, `rename`, `prepareRename`.

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
  template_diagnostics_test.go # Drift tests: assert helpers, context roots, built-in functions
  position_test.go        # Column conversions between bytes, runes and UTF-16
  transport_test.go       # Message framing and protocol error responses
  uri_test.go             # file URI <-> path conversions
  navigation_test.go      # Name chains, YAML escapes, value alignment fallback
  e2e_test.go             # [e2e_lsp] Binary subprocess + Neovim headless tests
  testdata/
    completion/           # Completion fixtures
    diagnostics/          # Diagnostics fixtures
    definition/           # Definition fixtures
    hover/                # Hover fixtures
    documentSymbol/       # Document symbol fixtures
    signatureHelp/        # Signature help fixtures
    references/           # References fixtures
    rename/               # Rename fixtures
    prepareRename/        # Prepare rename fixtures
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
