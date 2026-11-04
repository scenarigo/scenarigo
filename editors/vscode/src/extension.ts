import * as vscode from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from 'vscode-languageclient/node';

// The extension starts `scenarigo lsp` for YAML files. The server stays
// silent on files without a schemaVersion key, so it coexists with other
// YAML extensions.

let client: LanguageClient | undefined;

function serverCommand(): string {
  return vscode.workspace.getConfiguration('scenarigo').get<string>('path') || 'scenarigo';
}

function serverOptions(): ServerOptions {
  return { command: serverCommand(), args: ['lsp'], transport: TransportKind.stdio };
}

async function start(context: vscode.ExtensionContext): Promise<void> {
  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: 'file', language: 'yaml' }],
  };
  client = new LanguageClient('scenarigo', 'Scenarigo', serverOptions(), clientOptions);
  try {
    await client.start();
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    void vscode.window.showErrorMessage(
      `Scenarigo: cannot start "${serverCommand()} lsp": ${message}. Set scenarigo.path to the scenarigo binary.`,
    );
  }
  context.subscriptions.push({ dispose: () => void client?.stop() });
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  await start(context);
  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration(async (event) => {
      if (!event.affectsConfiguration('scenarigo.path')) {
        return;
      }
      await client?.stop();
      await start(context);
    }),
  );
}

export async function deactivate(): Promise<void> {
  await client?.stop();
}
