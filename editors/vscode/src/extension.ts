import * as fs from 'fs';
import * as path from 'path';
import * as vscode from 'vscode';
import {
  CloseAction,
  ErrorAction,
  ErrorHandler,
  LanguageClient,
  LanguageClientOptions,
} from 'vscode-languageclient/node';
import { findRunDirectory } from './run';
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

// closableChannel wraps an output channel so that writes after dispose are
// dropped: the client logs the exit of its server process once it has
// stopped, and VS Code's channels throw when written to after dispose.
function closableChannel(channel: vscode.OutputChannel): vscode.OutputChannel {
  let closed = false;
  const unlessClosed =
    <A extends unknown[]>(write: (...args: A) => void) =>
    (...args: A): void => {
      if (!closed) {
        write(...args);
      }
    };
  return {
    name: channel.name,
    append: unlessClosed((value: string) => channel.append(value)),
    appendLine: unlessClosed((value: string) => channel.appendLine(value)),
    replace: unlessClosed((value: string) => channel.replace(value)),
    clear: unlessClosed(() => channel.clear()),
    show: unlessClosed((column?: vscode.ViewColumn | boolean, preserveFocus?: boolean) =>
      typeof column === 'boolean' || column === undefined ? channel.show(column) : channel.show(column, preserveFocus),
    ),
    hide: unlessClosed(() => channel.hide()),
    dispose: () => {
      closed = true;
      channel.dispose();
    },
  };
}

function configuredPath(): string {
  return vscode.workspace.getConfiguration('scenarigo').get<string>('path') || 'scenarigo';
}

async function startClient(): Promise<void> {
  const configured = configuredPath();
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
  // A server process that fails before it has answered initialize, the
  // first one or a restarted one, is shut down and not restarted, and the
  // client reports that failure once; a client that a setting change
  // replaced is not restarted either. Other failures get the client's
  // default handling, which restarts a crashed server a few times.
  let started: LanguageClient | undefined;
  let fallback: ErrorHandler | undefined;
  const current = (): ErrorHandler | undefined =>
    started !== undefined && client === started && started.isRunning() ? fallback : undefined;
  const errorHandler: ErrorHandler = {
    error: (error, message, count) =>
      current()?.error(error, message, count) ?? { action: ErrorAction.Shutdown, handled: true },
    closed: () => current()?.closed() ?? { action: CloseAction.DoNotRestart, handled: true },
  };
  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: 'file', language: 'yaml' }],
    outputChannel: output,
    errorHandler,
    initializationFailedHandler: () => false,
  };
  // No transport: with one, the client adds --stdio to the arguments, which
  // `scenarigo lsp` rejects. The server uses stdio anyway.
  started = new LanguageClient('scenarigo', 'Scenarigo', { command, args: ['lsp'] }, clientOptions);
  fallback = started.createDefaultErrorHandler();
  client = started;
  try {
    await started.start();
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

// runScenario runs `scenarigo run <file>` as a task. The language server
// shows it as the Run lens of a scenario file; it runs only when the user
// clicks the lens or calls the command. The task runs the binary as a
// process, with the file as an argument: there is no shell to quote for,
// whatever the user's shell is, and each run gets its own directory.
async function runScenario(uri?: string): Promise<void> {
  const target = uri ? vscode.Uri.parse(uri) : vscode.window.activeTextEditor?.document.uri;
  if (!target || target.scheme !== 'file') {
    return;
  }
  const doc = vscode.workspace.textDocuments.find((d) => d.uri.toString() === target.toString());
  if (doc?.isDirty && !(await doc.save())) {
    return;
  }
  const configured = configuredPath();
  const command = findExecutable(expandHome(configured));
  if (command === undefined) {
    void vscode.window.showErrorMessage(
      `Scenarigo: cannot find "${configured}". Set scenarigo.path to the absolute path of the scenarigo binary, or put scenarigo on the PATH.`,
    );
    return;
  }
  const folder = vscode.workspace.getWorkspaceFolder(target);
  if (folder === undefined && vscode.workspace.workspaceFolders === undefined) {
    void vscode.window.showErrorMessage('Scenarigo: open a folder to run scenarios; VS Code runs tasks in a workspace.');
    return;
  }
  // scenarigo run reads scenarigo.yaml from the current directory.
  const cwd = findRunDirectory(target.fsPath, folder?.uri.fsPath, fs.existsSync);
  const task = new vscode.Task(
    { type: 'scenarigo', file: target.fsPath },
    folder ?? vscode.TaskScope.Workspace,
    `run ${path.basename(target.fsPath)}`,
    'scenarigo',
    new vscode.ProcessExecution(command, ['run', target.fsPath], { cwd }),
  );
  task.presentationOptions = { reveal: vscode.TaskRevealKind.Always, panel: vscode.TaskPanelKind.Dedicated, clear: true };
  await vscode.tasks.executeTask(task);
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  // The output channel is closed after the server has stopped, by
  // deactivate: VS Code disposes the subscriptions without waiting for
  // deactivate, and the client writes to the channel while it stops.
  output = closableChannel(vscode.window.createOutputChannel('Scenarigo'));
  context.subscriptions.push(
    vscode.commands.registerCommand('scenarigo.runScenario', runScenario),
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
  return enqueue(async () => {
    await stopClient();
    output?.dispose();
    output = undefined;
  });
}
