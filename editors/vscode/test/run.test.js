// Tests the logic of the Run lens that does not need VS Code.
const assert = require('node:assert/strict');
const { findRunDirectory } = require('../out/run.js');

const exists = (files) => (p) => files.includes(p);

// The nearest directory with a scenarigo.yaml, up to the workspace folder.
assert.equal(findRunDirectory('/ws/app/scenarios/a.yaml', '/ws', exists(['/ws/app/scenarigo.yaml', '/ws/scenarigo.yaml'])), '/ws/app');
assert.equal(findRunDirectory('/ws/scenarios/a.yaml', '/ws', exists(['/ws/scenarigo.yaml'])), '/ws');
// Not above the workspace folder.
assert.equal(findRunDirectory('/ws/scenarios/a.yaml', '/ws', exists(['/scenarigo.yaml'])), '/ws');
// Without a folder: up to the root, else the file's directory.
assert.equal(findRunDirectory('/tmp/x/a.yaml', undefined, exists(['/tmp/scenarigo.yaml'])), '/tmp');
assert.equal(findRunDirectory('/tmp/x/a.yaml', undefined, exists([])), '/tmp/x');

// A directory whose name starts with .. is inside the folder.
assert.equal(findRunDirectory('/ws/..data/sub/a.yaml', '/ws', exists(['/ws/..data/scenarigo.yaml'])), '/ws/..data');

console.log('run ok');
