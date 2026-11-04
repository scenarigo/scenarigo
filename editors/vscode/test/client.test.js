// Runs the compiled extension with the real vscode-languageclient against a
// stub of the vscode API and a fake server (fake-server.js): the server must
// be started as `scenarigo lsp`; a missing binary, a binary without the lsp
// command and a server that exits at once must each give one error and no
// restarts; a setting change must start the server after a failed start;
// quick setting changes must leave one server running; and restarts must
// share one output channel. The Run command must save the file and run
// `scenarigo run <file>` as a process, from the directory of the
// configuration. The fake binaries are shell scripts, so the test does not
// run on Windows.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const Module = require('node:module');
const os = require('node:os');
const path = require('node:path');
const { fileURLToPath, pathToFileURL } = require('node:url');

if (process.platform === 'win32') {
  console.log('client test skipped on Windows');
  process.exit(0);
}

const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'scenarigo-vscode-'));
const log = path.join(tmp, 'servers.log');
const crashLog = path.join(tmp, 'crashes.log');
const fakeServer = path.join(tmp, 'scenarigo');
const binary = (dir, script) => {
  fs.mkdirSync(path.join(tmp, dir));
  fs.writeFileSync(path.join(tmp, dir, 'scenarigo'), `#!/bin/sh\n${script}`, { mode: 0o755 });
  return path.join(tmp, dir, 'scenarigo');
};
// A released scenarigo, without the lsp command.
const noLsp = binary('old', 'echo \'Error: unknown command "lsp" for "scenarigo"\' >&2\nexit 1\n');
// A server that rejects initialize and keeps running.
const brokenLog = path.join(tmp, 'broken.log');
const rejecting = binary(
  'reject',
  `FAKE_SERVER_LOG="${brokenLog}" FAKE_INITIALIZE_ERROR=broken exec "${process.execPath}" "${path.join(__dirname, 'fake-server.js')}" "$@"\n`,
);
// A server that prints something before its first message.
const garbageLog = path.join(tmp, 'garbage.log');
const garbage = binary(
  'garbage',
  `FAKE_SERVER_LOG="${garbageLog}" FAKE_GARBAGE=starting exec "${process.execPath}" "${path.join(__dirname, 'fake-server.js')}" "$@"\n`,
);
// A server that crashes once after initialize, then runs.
const crashOnceLog = path.join(tmp, 'crash-once.log');
const crashOnce = binary(
  'crash-once',
  `FAKE_SERVER_LOG="${crashOnceLog}" FAKE_CRASH_ONCE="${path.join(tmp, 'crashed')}" exec "${process.execPath}" "${path.join(__dirname, 'fake-server.js')}" "$@"\n`,
);
// A server that crashes after initialize, then exits at once when restarted.
const brokenRestartLog = path.join(tmp, 'broken-restart.log');
const brokenRestart = binary(
  'broken-restart',
  `FAKE_SERVER_LOG="${brokenRestartLog}" FAKE_CRASH_ONCE="${path.join(tmp, 'crashed2')}" FAKE_EXIT_IF="${path.join(tmp, 'crashed2')}" exec "${process.execPath}" "${path.join(__dirname, 'fake-server.js')}" "$@"\n`,
);
// A server that has the command but exits as soon as it starts.
const crashing = binary('crash', `[ "$2" = "--help" ] && exit 0\necho crash >> "${crashLog}"\nexit 1\n`);
fs.writeFileSync(log, '');
fs.writeFileSync(crashLog, '');
fs.writeFileSync(fakeServer, `#!/bin/sh\nexec "${process.execPath}" "${path.join(__dirname, 'fake-server.js')}" "$@"\n`, { mode: 0o755 });
process.env.FAKE_SERVER_LOG = log;

// The vscode API: the parts the extension uses, and for the rest a proxy
// that accepts any use, enough for the language client.
const config = { path: path.join(tmp, 'missing', 'scenarigo') };
const configListeners = [];
const errors = [];
let outputChannels = 0;
const commands = {};
const documents = [];
const executed = [];
let workspaceFolder;
const any = (name) =>
  new Proxy(function () {}, {
    get: (_, prop) => (prop === 'then' ? undefined : prop === 'dispose' ? () => {} : any(`${name}.${String(prop)}`)),
    apply: () => any(`${name}()`),
    construct: () => any(`new ${name}`),
  });
class Disposable {
  constructor(fn) {
    this.fn = fn;
  }
  static from(...items) {
    return new Disposable(() => items.forEach((d) => d?.dispose?.()));
  }
  dispose() {
    this.fn?.();
  }
}
class EventEmitter {
  event = () => new Disposable();
  fire() {}
  dispose() {}
}
class ProcessExecution {
  constructor(process, args, options) {
    Object.assign(this, { process, args, options });
  }
}
class Task {
  constructor(definition, scope, name, source, execution) {
    Object.assign(this, { definition, scope, name, source, execution });
  }
}
const Uri = {
  parse: (s) => ({ scheme: new URL(s).protocol.slice(0, -1), fsPath: s.startsWith('file:') ? fileURLToPath(s) : '', toString: () => s }),
};
const withFallback = (target, name) => new Proxy(target, { get: (t, p) => (p in t ? t[p] : any(`${name}.${String(p)}`)) });
const vscode = withFallback(
  {
    Disposable,
    EventEmitter,
    ProcessExecution,
    Task,
    Uri,
    TaskScope: { Global: 1, Workspace: 2 },
    TaskRevealKind: { Always: 1, Silent: 2, Never: 3 },
    TaskPanelKind: { Shared: 1, Dedicated: 2, New: 3 },
    version: '1.95.0',
    commands: withFallback(
      {
        registerCommand: (name, handler) => {
          commands[name] = handler;
          return new Disposable();
        },
      },
      'commands',
    ),
    tasks: withFallback(
      {
        executeTask: async (task) => {
          executed.push(task);
          return {};
        },
      },
      'tasks',
    ),
    workspace: withFallback(
      {
        getConfiguration: () => ({ get: (key) => config[key] }),
        onDidChangeConfiguration: (listener) => {
          configListeners.push(listener);
          return new Disposable();
        },
        get workspaceFolders() {
          return workspaceFolder ? [workspaceFolder] : undefined;
        },
        getWorkspaceFolder: (uri) =>
          workspaceFolder && uri.fsPath.startsWith(workspaceFolder.uri.fsPath + path.sep) ? workspaceFolder : undefined,
        textDocuments: documents,
      },
      'workspace',
    ),
    window: withFallback(
      {
        showErrorMessage: async (message) => {
          errors.push(message);
        },
        createOutputChannel: () => {
          outputChannels++;
          return withFallback(outputChannel(), 'outputChannel');
        },
        activeTextEditor: undefined,
      },
      'window',
    ),
  },
  'vscode',
);
const load = Module._load;
Module._load = function (request, ...rest) {
  return request === 'vscode' ? vscode : load.call(this, request, ...rest);
};

// VS Code's output channel throws once it is closed.
const closedChannelWrites = [];
const outputChannel = () => {
  let closed = false;
  const write = () => {
    if (closed) {
      closedChannelWrites.push(new Error('Channel has been closed').stack);
      throw new Error('Channel has been closed');
    }
  };
  return { append: write, appendLine: write, replace: write, clear: write, show() {}, hide() {}, dispose: () => (closed = true) };
};
// The extension host logs unhandled rejections and uncaught exceptions; the
// test records them and fails on those of the extension: the ones whose
// stack goes through its code. vscode-languageclient 9.0.1 leaves some of
// its own unhandled when a server fails: writes to a gone process (EPIPE),
// requests pending when the connection closes, and the stop of a client
// whose initialize failed (doInitialize), which goes back to the start the
// extension awaits.
const rejections = [];
process.on('unhandledRejection', (err) => rejections.push(err));
process.on('uncaughtException', (err) => rejections.push(err));
const ownRejections = () =>
  rejections.filter((err) => /out[\\/](extension|server)\.js/.test(String(err?.stack)) && !String(err?.stack).includes('doInitialize'));

const servers = () =>
  fs
    .readFileSync(log, 'utf8')
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line));
const alive = (pid) => {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
};
const running = () => servers().filter((s) => alive(s.pid));
const waitFor = async (what, cond) => {
  for (let i = 0; i < 100 && !cond(); i++) {
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  assert.ok(cond(), what);
};
const changePath = (value) => {
  config.path = value;
  return Promise.all(configListeners.map((listener) => listener({ affectsConfiguration: (s) => s === 'scenarigo.path' })));
};

async function main() {
  const extension = require('../out/extension.js');
  const context = { subscriptions: [] };

  // A missing binary: one error that names the setting, no server.
  await extension.activate(context);
  assert.equal(errors.length, 1, JSON.stringify(errors));
  assert.match(errors[0], /scenarigo\.path/);
  assert.deepEqual(servers(), []);

  // A binary without the lsp command: one error that says so, nothing
  // started.
  errors.length = 0;
  await changePath(noLsp);
  assert.equal(errors.length, 1, JSON.stringify(errors));
  assert.match(errors[0], /unknown command "lsp"/);
  assert.deepEqual(servers(), []);

  // A server that exits at once: one error, and no restarts.
  errors.length = 0;
  await changePath(crashing);
  await new Promise((resolve) => setTimeout(resolve, 1500));
  assert.equal(errors.length, 1, JSON.stringify(errors));
  assert.equal(fs.readFileSync(crashLog, 'utf8').split('\n').filter(Boolean).length, 1, 'the server was restarted');

  // A server that rejects initialize: one error. The client whose start
  // failed cannot be stopped, only disposed, which the next start does; the
  // client then ends the process.
  errors.length = 0;
  fs.writeFileSync(brokenLog, '');
  await changePath(rejecting);
  assert.equal(errors.length, 1, JSON.stringify(errors));
  const broken = () => fs.readFileSync(brokenLog, 'utf8').split('\n').filter(Boolean).map((line) => JSON.parse(line).pid);
  assert.equal(broken().length, 1);

  // A server that prints something before its first message: one error, the
  // start settles, and the process ends.
  errors.length = 0;
  fs.writeFileSync(garbageLog, '');
  const settled = await Promise.race([
    changePath(garbage).then(() => true),
    new Promise((resolve) => setTimeout(() => resolve(false), 10_000)),
  ]);
  assert.ok(settled, 'the start never settled');
  assert.equal(errors.length, 1, JSON.stringify(errors));
  const garbageServers = () => fs.readFileSync(garbageLog, 'utf8').split('\n').filter(Boolean).map((line) => JSON.parse(line).pid);
  await waitFor('the server that printed something is still running', () => !garbageServers().some(alive));

  // A server that crashes after initialize is restarted.
  errors.length = 0;
  fs.writeFileSync(crashOnceLog, '');
  await changePath(crashOnce);
  const crashOnceServers = () => fs.readFileSync(crashOnceLog, 'utf8').split('\n').filter(Boolean).map((line) => JSON.parse(line).pid);
  await waitFor('the server was not restarted', () => crashOnceServers().length === 2 && alive(crashOnceServers()[1]));

  // A restarted server that exits before initialize is not restarted again.
  errors.length = 0;
  fs.writeFileSync(brokenRestartLog, '');
  await changePath(brokenRestart);
  await new Promise((resolve) => setTimeout(resolve, 3000));
  assert.ok(errors.length <= 1, JSON.stringify(errors));
  // Started, crashed, restarted once, and not again.
  assert.equal(fs.readFileSync(brokenRestartLog, 'utf8').split('\n').filter(Boolean).length, 2, 'the server was restarted again');

  // Fixing the setting starts the server, as `scenarigo lsp`.
  errors.length = 0;
  await changePath(fakeServer);
  await waitFor('the server did not start', () => running().length === 1);
  await waitFor('the server that rejected initialize is still running', () => !broken().some(alive));
  assert.deepEqual(servers()[0].argv, ['lsp']);
  assert.deepEqual(errors, []);

  // Two changes at once leave one server running.
  await Promise.all([changePath(fakeServer), changePath(fakeServer)]);
  await waitFor(`${running().length} servers are running`, () => running().length === 1);
  assert.equal(servers().length, 3);
  assert.deepEqual(errors, []);
  assert.equal(outputChannels, 1, 'each start made an output channel');

  assert.deepEqual(ownRejections().map(String), []);

  // The Run command saves the file, then runs `scenarigo run <file>` as a
  // process, without a shell to quote for, from the nearest directory with a
  // scenarigo.yaml.
  const project = path.join(tmp, 'ws', `it's "$HOME" 100% #1`);
  const scenario = path.join(project, 'scenarios', 'a b.yaml');
  fs.mkdirSync(path.dirname(scenario), { recursive: true });
  fs.writeFileSync(path.join(project, 'scenarigo.yaml'), 'schemaVersion: config/v1\n');
  fs.writeFileSync(scenario, 'schemaVersion: scenario/v1\n');
  workspaceFolder = { uri: { fsPath: path.join(tmp, 'ws') }, name: 'ws', index: 0 };
  const uri = pathToFileURL(scenario).href;
  let saved = false;
  documents.push({ uri: { toString: () => uri }, isDirty: true, save: async () => (saved = true) });
  await commands['scenarigo.runScenario'](uri);
  assert.ok(saved, 'the file was not saved');
  assert.equal(executed.length, 1, 'no task was run');
  const { execution, scope } = executed[0];
  assert.ok(execution instanceof ProcessExecution, 'not run as a process');
  assert.equal(execution.process, fakeServer);
  assert.deepEqual(execution.args, ['run', scenario]);
  assert.equal(execution.options.cwd, project);
  assert.equal(scope, workspaceFolder);
  assert.deepEqual(errors, []);

  // Deactivating stops it. VS Code disposes the subscriptions right after
  // calling deactivate, without waiting for it.
  const deactivated = extension.deactivate();
  context.subscriptions.forEach((d) => d.dispose());
  await deactivated;
  await waitFor('the server is still running', () => running().length === 0);
  await new Promise((resolve) => setTimeout(resolve, 500));
  assert.deepEqual(closedChannelWrites, []);
  assert.deepEqual(ownRejections().map(String), []);

  console.log('client ok');
}

main()
  .catch((err) => {
    console.error(err);
    process.exitCode = 1;
  })
  .finally(() => {
    running().forEach((s) => process.kill(s.pid));
    fs.rmSync(tmp, { recursive: true, force: true });
    process.exit();
  });
