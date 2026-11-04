// Tests the logic of the Run lens that does not need VS Code.
const assert = require('node:assert/strict');
const { findRunDirectory, quote, runCommandLine } = require('../out/run.js');

const exists = (files) => (p) => files.includes(p);

// The nearest directory with a scenarigo.yaml, up to the workspace folder.
assert.equal(findRunDirectory('/ws/app/scenarios/a.yaml', '/ws', exists(['/ws/app/scenarigo.yaml', '/ws/scenarigo.yaml'])), '/ws/app');
assert.equal(findRunDirectory('/ws/scenarios/a.yaml', '/ws', exists(['/ws/scenarigo.yaml'])), '/ws');
// Not above the workspace folder.
assert.equal(findRunDirectory('/ws/scenarios/a.yaml', '/ws', exists(['/scenarigo.yaml'])), '/ws');
// Without a folder: up to the root, else the file's directory.
assert.equal(findRunDirectory('/tmp/x/a.yaml', undefined, exists(['/tmp/scenarigo.yaml'])), '/tmp');
assert.equal(findRunDirectory('/tmp/x/a.yaml', undefined, exists([])), '/tmp/x');

assert.equal(quote("it's.yaml", 'linux'), `'it'\\''s.yaml'`);
assert.equal(quote('say "hi".yaml', 'win32'), '"say ""hi"".yaml"');
assert.equal(runCommandLine('/usr/bin/scenarigo', '/ws/scenarios/a b.yaml', '/ws', 'darwin'), `'/usr/bin/scenarigo' run 'scenarios/a b.yaml'`);

console.log('run ok');
