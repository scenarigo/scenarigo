// Runs the compiled extension with the real vscode-languageclient against a
// stub of the vscode API and a fake server (fake-server.js): the server must
// be started as `scenarigo lsp`; a missing binary, a binary without the lsp
// command and a server that exits at once must each give one error and no
// restarts; a setting change must start the server after a failed start;
// quick setting changes must leave one server running; and restarts must
// share one output channel. The fake binaries are shell scripts, so the
// test does not run on Windows.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const Module = require('node:module');
const os = require('node:os');
const path = require('node:path');

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
const withFallback = (target, name) => new Proxy(target, { get: (t, p) => (p in t ? t[p] : any(`${name}.${String(p)}`)) });
const vscode = withFallback(
  {
    Disposable,
    EventEmitter,
    version: '1.95.0',
    workspace: withFallback(
      {
        getConfiguration: () => ({ get: (key) => config[key] }),
        onDidChangeConfiguration: (listener) => {
          configListeners.push(listener);
          return new Disposable();
        },
        workspaceFolders: undefined,
        textDocuments: [],
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
          return withFallback({}, 'outputChannel');
        },
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

// The extension host logs unhandled rejections; the test records them and
// fails on those of the extension. Two come from vscode-languageclient
// 9.0.1 itself when a server exits during initialize, and the extension
// cannot catch them: its writes to the gone process fail with EPIPE, and it
// stops the starting client without handling the rejection (doInitialize).
const rejections = [];
process.on('unhandledRejection', (err) => rejections.push(err));
const ownRejections = () =>
  rejections.filter((err) => !['EPIPE', 'ERR_STREAM_DESTROYED'].includes(err?.code) && !String(err?.stack).includes('doInitialize'));

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

  // Deactivating stops it.
  await extension.deactivate();
  context.subscriptions.forEach((d) => d.dispose());
  await waitFor('the server is still running', () => running().length === 0);

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
