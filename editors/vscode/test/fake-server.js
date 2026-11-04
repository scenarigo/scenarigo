// A language server for the client test: it records its arguments and pid,
// answers initialize and shutdown, and exits on exit.
const fs = require('node:fs');

fs.appendFileSync(process.env.FAKE_SERVER_LOG, `${JSON.stringify({ pid: process.pid, argv: process.argv.slice(2) })}\n`);

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
    if (message.method === 'initialize') {
      send({ id: message.id, result: { capabilities: {} } });
    } else if (message.method === 'shutdown') {
      send({ id: message.id, result: null });
    } else if (message.method === 'exit') {
      process.exit(0);
    }
  }
});
