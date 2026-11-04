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
// names: an absolute path, or a name looked up in PATH (with the extensions
// of PATHEXT on Windows). A relative path is not resolved, since the working
// directory of the extension host means nothing to the user.
export function findExecutable(
  command: string,
  env: NodeJS.ProcessEnv = process.env,
  platform: NodeJS.Platform = process.platform,
): string | undefined {
  const windows = platform === 'win32';
  const extensions = windows ? ['', ...(env.PATHEXT ?? '.COM;.EXE;.BAT;.CMD').split(';').filter(Boolean)] : [''];
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
