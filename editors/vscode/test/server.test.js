// Tests finding the scenarigo binary (src/server.ts).
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { expandHome, findExecutable } = require('../out/server.js');

assert.equal(expandHome('~', '/home/u'), '/home/u');
assert.equal(expandHome('~/go/bin/scenarigo', '/home/u'), path.join('/home/u', 'go/bin/scenarigo'));
assert.equal(expandHome('~other/scenarigo', '/home/u'), '~other/scenarigo');
assert.equal(expandHome('/usr/bin/scenarigo', '/home/u'), '/usr/bin/scenarigo');
assert.equal(expandHome('scenarigo', '/home/u'), 'scenarigo');

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

  // Windows: the extensions of PATHEXT.
  // Windows file names are case-insensitive; this test runs where they are not.
  fs.writeFileSync(path.join(bin, 'tool.EXE'), '');
  assert.equal(findExecutable('tool', { ...env, PATHEXT: '.EXE;.CMD' }, 'win32'), path.join(bin, 'tool.EXE'));
} finally {
  fs.rmSync(tmp, { recursive: true, force: true });
}

console.log('server ok');
