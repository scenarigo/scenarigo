import * as vscode from 'vscode';
import { LanguageClient, LanguageClientOptions } from 'vscode-languageclient/node';
import { expandHome, findExecutable } from './server';

// The extension starts `scenarigo lsp` for YAML files. The server stays
// silent on files without a schemaVersion key, so it coexists with other
// YAML extensions.

let client: LanguageClient | undefined;

// Starts and stops run one after another, so that quick setting changes do
// not leave several servers running.
let queue: Promise<void> = Promise.resolve();

function enqueue(task: () => Promise<void>): Promise<void> {
  queue = queue.then(task, task);
  return queue;
}

async function startClient(): Promise<void> {
  const configured = vscode.workspace.getConfiguration('scenarigo').get<string>('path') || 'scenarigo';
  const command = findExecutable(expandHome(configured));
  if (command === undefined) {
    void vscode.window.showErrorMessage(
      `Scenarigo: cannot find "${configured}". Set scenarigo.path to the absolute path of the scenarigo binary, or put scenarigo on the PATH.`,
    );
    return;
  }
  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: 'file', language: 'yaml' }],
  };
  // No transport: with one, the client adds --stdio to the arguments, which
  // `scenarigo lsp` rejects. The server uses stdio anyway.
  const started = new LanguageClient('scenarigo', 'Scenarigo', { command, args: ['lsp'] }, clientOptions);
  client = started;
  try {
    await started.start();
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    void vscode.window.showErrorMessage(
      `Scenarigo: cannot start "${command} lsp": ${message}. Check that this scenarigo has the lsp command.`,
    );
  }
}

async function stopClient(): Promise<void> {
  const stopped = client;
  client = undefined;
  if (stopped === undefined) {
    return;
  }
  try {
    // A client whose start failed cannot be stopped, only disposed.
    await (stopped.isRunning() ? stopped.stop() : stopped.dispose());
  } catch {
    // Stopped already.
  }
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  context.subscriptions.push(
    { dispose: () => void enqueue(stopClient) },
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration('scenarigo.path')) {
        return enqueue(async () => {
          await stopClient();
          await startClient();
        });
      }
      return undefined;
    }),
  );
  await enqueue(startClient);
}

export function deactivate(): Promise<void> {
  return enqueue(stopClient);
}
