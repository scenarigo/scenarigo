// Runs the compiled extension with the real vscode-languageclient against a
// stub of the vscode API and a fake server (fake-server.js): the server must
// be started as `scenarigo lsp`, a setting change must start it after a
// failed start, and quick setting changes must leave one server running.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const Module = require('node:module');
const os = require('node:os');
const path = require('node:path');

const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'scenarigo-vscode-'));
const log = path.join(tmp, 'servers.log');
const fakeServer = path.join(tmp, 'scenarigo');
fs.writeFileSync(log, '');
fs.writeFileSync(fakeServer, `#!/bin/sh\nexec "${process.execPath}" "${path.join(__dirname, 'fake-server.js')}" "$@"\n`, { mode: 0o755 });
process.env.FAKE_SERVER_LOG = log;

// The vscode API: the parts the extension uses, and for the rest a proxy
// that accepts any use, enough for the language client.
const config = { path: path.join(tmp, 'missing', 'scenarigo') };
const configListeners = [];
const errors = [];
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
        createOutputChannel: () => withFallback({}, 'outputChannel'),
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

  // Fixing the setting starts the server, as `scenarigo lsp`.
  errors.length = 0;
  await changePath(fakeServer);
  await waitFor('the server did not start', () => running().length === 1);
  assert.deepEqual(servers()[0].argv, ['lsp']);
  assert.deepEqual(errors, []);

  // Two changes at once leave one server running.
  await Promise.all([changePath(fakeServer), changePath(fakeServer)]);
  await waitFor(`${running().length} servers are running`, () => running().length === 1);
  assert.equal(servers().length, 3);
  assert.deepEqual(errors, []);

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
