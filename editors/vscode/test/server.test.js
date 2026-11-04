// Tests finding the scenarigo binary (src/server.ts).
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { checkLspCommand, expandHome, findExecutable } = require('../out/server.js');

assert.equal(expandHome('~', '/home/u'), '/home/u');
assert.equal(expandHome('~/go/bin/scenarigo', '/home/u'), path.join('/home/u', 'go/bin/scenarigo'));
assert.equal(expandHome('~other/scenarigo', '/home/u'), '~other/scenarigo');
assert.equal(expandHome('/usr/bin/scenarigo', '/home/u'), '/usr/bin/scenarigo');
assert.equal(expandHome('scenarigo', '/home/u'), 'scenarigo');

// The lookup on this platform uses executable bits; Windows has none.
if (process.platform === 'win32') {
  console.log('server test skipped on Windows');
  process.exit(0);
}

const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'scenarigo-find-'));
try {
  const bin = path.join(tmp, 'bin');
  fs.mkdirSync(path.join(bin, 'dir'), { recursive: true });
  fs.writeFileSync(path.join(bin, 'scenarigo'), '#!/bin/sh\n', { mode: 0o755 });
  fs.writeFileSync(path.join(bin, 'plain'), '', { mode: 0o644 });
  const env = { PATH: ['/nonexistent', bin].join(path.delimiter) };

  assert.equal(findExecutable('scenarigo', env, 'linux'), path.join(bin, 'scenarigo'));
  assert.equal(findExecutable(path.join(bin, 'scenarigo'), {}, 'linux'), path.join(bin, 'scenarigo'));
  assert.equal(findExecutable('plain', env, 'linux'), undefined, 'not executable');
  assert.equal(findExecutable('dir', env, 'linux'), undefined, 'a directory');
  assert.equal(findExecutable('missing', env, 'linux'), undefined);
  assert.equal(findExecutable('bin/scenarigo', env, 'linux'), undefined, 'a relative path');
  assert.equal(findExecutable('scenarigo', {}, 'linux'), undefined, 'no PATH');

  // Windows: a name gets .exe, which wins over a file without an extension
  // (it cannot be started), and .cmd or .bat files are not taken (Node
  // starts them only through a shell).
  fs.writeFileSync(path.join(bin, 'tool'), '');
  fs.writeFileSync(path.join(bin, 'tool.exe'), '');
  fs.writeFileSync(path.join(bin, 'script.cmd'), '');
  assert.equal(findExecutable('tool', env, 'win32'), path.join(bin, 'tool.exe'));
  assert.equal(findExecutable('tool.exe', env, 'win32'), path.join(bin, 'tool.exe'));
  assert.equal(findExecutable('script', { ...env, PATHEXT: '.CMD' }, 'win32'), undefined);
  // A name with a dot is still a name.
  fs.writeFileSync(path.join(bin, 'scenarigo-0.29.0.exe'), '');
  assert.equal(findExecutable('scenarigo-0.29.0', env, 'win32'), path.join(bin, 'scenarigo-0.29.0.exe'));
} finally {
  fs.rmSync(tmp, { recursive: true, force: true });
}

console.log('server ok');

// A binary that ignores SIGTERM and hangs: the check times out and says so.
(async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'scenarigo-check-'));
  try {
    const hang = path.join(dir, 'hang');
    fs.writeFileSync(hang, "#!/bin/sh\ntrap '' TERM\nwhile :; do sleep 1; done\n", { mode: 0o755 });
    const started = Date.now();
    const problem = await checkLspCommand(hang, 500);
    assert.ok(Date.now() - started < 5000, 'the check did not end');
    assert.match(problem, /timed out/);
    // A binary that reads its input gets none.
    const reader = path.join(dir, 'reader');
    fs.writeFileSync(reader, '#!/bin/sh\ncat >/dev/null\nexit 0\n', { mode: 0o755 });
    assert.equal(await checkLspCommand(reader, 5000), undefined);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
  console.log('check ok');
})();
