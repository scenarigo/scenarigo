// A language server for the client test: it answers `lsp --help`, records
// its arguments and pid when started, answers initialize (with an error when
// FAKE_INITIALIZE_ERROR is set, staying alive) and shutdown, and exits on
// exit. FAKE_GARBAGE writes text that is not a message first, as a wrapper
// script that prints something would. FAKE_CRASH_ONCE names a file: when it
// does not exist yet, the server creates it and exits after initialized.
// FAKE_EXIT_IF names a file: when it exists, the server exits at once.
const fs = require('node:fs');

// The extension checks for the lsp command first.
if (process.argv.includes('--help')) {
  process.stdout.write('Starts the language server.\n');
  process.exit(0);
}

fs.appendFileSync(process.env.FAKE_SERVER_LOG, `${JSON.stringify({ pid: process.pid, argv: process.argv.slice(2) })}\n`);

if (process.env.FAKE_EXIT_IF && fs.existsSync(process.env.FAKE_EXIT_IF)) {
  process.exit(1);
}

if (process.env.FAKE_GARBAGE) {
  process.stdout.write(`${process.env.FAKE_GARBAGE}\n`);
}

const send = (message) => {
  const body = JSON.stringify({ jsonrpc: '2.0', ...message });
  process.stdout.write(`Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`);
};

let buffer = Buffer.alloc(0);
process.stdin.on('data', (data) => {
  buffer = Buffer.concat([buffer, data]);
  for (;;) {
    const end = buffer.indexOf('\r\n\r\n');
    if (end < 0) {
      return;
    }
    const length = Number(/Content-Length: *(\d+)/i.exec(buffer.subarray(0, end).toString())[1]);
    if (buffer.length < end + 4 + length) {
      return;
    }
    const message = JSON.parse(buffer.subarray(end + 4, end + 4 + length).toString());
    buffer = buffer.subarray(end + 4 + length);
    if (message.method === 'initialize' && process.env.FAKE_INITIALIZE_ERROR) {
      send({ id: message.id, error: { code: -32603, message: process.env.FAKE_INITIALIZE_ERROR } });
    } else if (message.method === 'initialize') {
      send({ id: message.id, result: { capabilities: {} } });
    } else if (message.method === 'shutdown') {
      send({ id: message.id, result: null });
    } else if (message.method === 'initialized' && process.env.FAKE_CRASH_ONCE && !fs.existsSync(process.env.FAKE_CRASH_ONCE)) {
      fs.writeFileSync(process.env.FAKE_CRASH_ONCE, '');
      process.exit(1);
    } else if (message.method === 'exit') {
      process.exit(0);
    }
  }
});
