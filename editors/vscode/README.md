# Scenarigo for Visual Studio Code

A minimal extension that starts the scenarigo language server (`scenarigo lsp`) for YAML files: completion, hover, diagnostics, definitions, references, renames and signature help for `scenarigo.yaml` and scenario files. It also highlights `{{ }}` template expressions inside YAML.

The server stays silent on YAML files without a `schemaVersion` key, so the extension coexists with other YAML extensions.

## Requirements

`scenarigo` installed, with the `lsp` command:

```shell
go install github.com/scenarigo/scenarigo/cmd/scenarigo@latest
```

## Settings

| Setting | Default | Description |
|---|---|---|
| `scenarigo.path` | `scenarigo` | Path of the `scenarigo` binary. Changing it restarts the server. |

## Build and install locally

The extension is not published to the Marketplace. Build a package and install it:

```shell
cd editors/vscode
npm ci
npm run compile
npx @vscode/vsce package
code --install-extension scenarigo-*.vsix
```

## Development

```shell
npm run check         # type check
npm run test:grammar  # tokenize template expressions with the injection grammar
```
