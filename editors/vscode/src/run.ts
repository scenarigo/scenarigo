import * as path from 'path';

// The logic of the Run lens that does not need VS Code, so that it can be
// tested with Node alone.

// findRunDirectory returns the directory to run a scenario from: the nearest
// directory from the file up to the workspace folder that has a
// scenarigo.yaml, because `scenarigo run` reads the configuration of the
// current directory; otherwise the workspace folder, or the file's directory.
export function findRunDirectory(file: string, workspaceFolder: string | undefined, exists: (p: string) => boolean): string {
  let dir = path.dirname(file);
  for (;;) {
    if (exists(path.join(dir, 'scenarigo.yaml'))) {
      return dir;
    }
    const parent = path.dirname(dir);
    if (dir === workspaceFolder || parent === dir || (workspaceFolder !== undefined && !isWithin(parent, workspaceFolder))) {
      break;
    }
    dir = parent;
  }
  return workspaceFolder ?? path.dirname(file);
}

function isWithin(dir: string, root: string): boolean {
  const rel = path.relative(root, dir);
  return rel === '' || (rel !== '..' && !rel.startsWith(`..${path.sep}`) && !path.isAbsolute(rel));
}
