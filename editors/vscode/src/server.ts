import { execFile } from 'node:child_process';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';

// Finding the scenarigo binary. This module does not use the vscode API, so
// that node can test it alone.

// expandHome expands a leading ~ to the home directory, as a shell does.
export function expandHome(p: string, home: string = os.homedir()): string {
  if (p === '~') {
    return home;
  }
  if (p.startsWith('~/') || p.startsWith('~\\')) {
    return path.join(home, p.slice(2));
  }
  return p;
}

// findExecutable returns the absolute path of the program that a command
// names: an absolute path, or a name looked up in PATH. On Windows a name
// gets .exe unless it ends with it (scenarigo-0.29.0 is a name too): a file without one cannot be started, and
// Node starts .cmd and .bat files only through a shell. A relative path is
// not resolved, since the working directory of the extension host means
// nothing to the user.
export function findExecutable(
  command: string,
  env: NodeJS.ProcessEnv = process.env,
  platform: NodeJS.Platform = process.platform,
): string | undefined {
  const windows = platform === 'win32';
  const extensions = windows && !command.toLowerCase().endsWith('.exe') ? ['.exe'] : [''];
  const executable = (p: string): boolean => {
    try {
      if (!fs.statSync(p).isFile()) {
        return false;
      }
      if (!windows) {
        fs.accessSync(p, fs.constants.X_OK);
      }
      return true;
    } catch {
      return false;
    }
  };
  const find = (p: string) => extensions.map((ext) => p + ext).find(executable);
  if (command.includes('/') || (windows && command.includes('\\'))) {
    return path.isAbsolute(command) ? find(command) : undefined;
  }
  for (const dir of (env.PATH ?? '').split(path.delimiter).filter(Boolean)) {
    const found = find(path.join(dir, command));
    if (found !== undefined) {
      return found;
    }
  }
  return undefined;
}

// checkLspCommand runs `<command> lsp --help` and returns why the binary
// cannot run the language server, or undefined when it can. A scenarigo
// without the lsp command, such as a release before it, exits with
// "unknown command"; starting it as a server would fail over and over.
// The binary gets no input, and one that does not answer in time is
// killed, also when it ignores SIGTERM.
export function checkLspCommand(command: string, timeoutMs = 10_000): Promise<string | undefined> {
  return new Promise((resolve) => {
    const child = execFile(
      command,
      ['lsp', '--help'],
      { timeout: timeoutMs, killSignal: 'SIGKILL', windowsHide: true },
      (err, _stdout, stderr) => {
        if (err === null) {
          resolve(undefined);
          return;
        }
        if (err.killed) {
          resolve(`"lsp --help" timed out after ${timeoutMs / 1000} seconds`);
          return;
        }
        const output = String(stderr).trim().replace(/^Error: /, '').split('\n')[0];
        resolve(output || err.message.trim());
      },
    );
    child.stdin?.end();
  });
}
