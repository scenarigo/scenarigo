import * as vscode from 'vscode';
import {
  CloseAction,
  ErrorAction,
  ErrorHandler,
  LanguageClient,
  LanguageClientOptions,
} from 'vscode-languageclient/node';
import { checkLspCommand, expandHome, findExecutable } from './server';

// The extension starts `scenarigo lsp` for YAML files. The server stays
// silent on files without a schemaVersion key, so it coexists with other
// YAML extensions.

let client: LanguageClient | undefined;

// One output channel for every client, so that restarts do not add more.
let output: vscode.OutputChannel | undefined;

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
  const problem = await checkLspCommand(command);
  if (problem !== undefined) {
    void vscode.window.showErrorMessage(
      `Scenarigo: "${command}" cannot run the language server (${problem}). Set scenarigo.path to a scenarigo that has the lsp command.`,
    );
    return;
  }
  // A server that fails before it has answered initialize is not restarted,
  // and the client reports that failure once; later failures get the
  // client's default handling, which restarts a crashed server a few times.
  let initialized = false;
  let fallback: ErrorHandler | undefined;
  const errorHandler: ErrorHandler = {
    error: (error, message, count) =>
      initialized && fallback ? fallback.error(error, message, count) : { action: ErrorAction.Continue, handled: true },
    closed: () => (initialized && fallback ? fallback.closed() : { action: CloseAction.DoNotRestart, handled: true }),
  };
  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: 'file', language: 'yaml' }],
    outputChannel: output,
    errorHandler,
    initializationFailedHandler: () => false,
  };
  // No transport: with one, the client adds --stdio to the arguments, which
  // `scenarigo lsp` rejects. The server uses stdio anyway.
  const started = new LanguageClient('scenarigo', 'Scenarigo', { command, args: ['lsp'] }, clientOptions);
  fallback = started.createDefaultErrorHandler();
  client = started;
  try {
    await started.start();
    initialized = true;
  } catch {
    // The client has shown "couldn't create connection to server", with a
    // link to the output channel that holds the server's stderr.
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
  output = vscode.window.createOutputChannel('Scenarigo');
  context.subscriptions.push(
    output,
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
