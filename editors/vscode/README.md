# Scenarigo for Visual Studio Code

A minimal extension that starts the scenarigo language server (`scenarigo lsp`) for YAML files: completion, hover, diagnostics, definitions, references, renames and signature help for `scenarigo.yaml` and scenario files. It also highlights `{{ }}` template expressions inside YAML scalars (plain, quoted and block), but not in comments or after an escape (`\{{`). A template that spans lines is highlighted on its first line only, so that a template being typed does not take the rest of the file with it. It also runs a scenario file from the "Run" lens the server shows on it.

The lens and the **Scenarigo: Run Scenario** command run `scenarigo run <file>` as a task, in the terminal panel, from the nearest directory with a `scenarigo.yaml` between the file and its workspace folder, so that the configuration is used. The task runs the binary of `scenarigo.path` directly, without a shell. An unsaved file is saved first. Nothing runs until you click the lens or call the command. VS Code runs tasks in a workspace, so the file must be in an open folder or workspace. VS Code replaces `${...}` in the paths of a task, so a file whose path contains `${` is not run.

The server stays silent on YAML files without a `schemaVersion` key, so the extension coexists with other YAML extensions.

## Requirements

A `scenarigo` binary with the `lsp` command. No release has it yet; build one from a checkout of this repository:

```shell
go install ./cmd/scenarigo
```

## Settings

| Setting | Default | Description |
|---|---|---|
| `scenarigo.path` | `scenarigo` | The `scenarigo` binary: a name looked up in the `PATH`, or an absolute path (a leading `~` is the home directory). Relative paths are not supported. Changing it restarts the server. |

Before starting the server, the extension runs `scenarigo lsp --help` to check that the binary has the command, and reports a binary that does not (a released scenarigo) once. A server that exits before it has answered `initialize` is reported once and not restarted; a server that crashes later is restarted a few times by the language client. The server's messages go to the **Scenarigo** output channel.

## Build and install locally

The extension is not published to the Marketplace. Build a package and install it; packaging needs Node.js 20 or later:

```shell
cd editors/vscode
npm ci
npx @vscode/vsce@3.9.2 package
code --install-extension scenarigo-0.0.1.vsix
```

`vsce package` compiles the extension and copies the repository's `LICENSE` into the package.

## Development

```shell
npm run check  # type check
npm test       # compile, then test the grammar, the binary lookup, the client and the Run logic
```

`npm test` tokenizes YAML with the TextMate YAML grammar of the `tm-grammars` package, which VS Code ships in 1.82 to 1.91 and 1.95 to 1.96, and the injection grammars. Other VS Code versions ship another YAML grammar (`extensions/yaml/syntaxes` in the VS Code repository); to test with it, put its files in a directory and run `YAML_GRAMMAR_DIR=<directory> node test/grammar.test.js`. The client test starts the extension with the real `vscode-languageclient` against a stub of the VS Code API and fake servers; it uses shell scripts, so it is skipped on Windows.
