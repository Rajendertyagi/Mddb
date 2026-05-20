import * as path from 'node:path';
import * as fs from 'node:fs/promises';
import * as glob from '@actions/glob';

export interface WalkedFile {
  absolutePath: string;
  relativePath: string;
  size: number;
}

export interface WalkOptions {
  workingDirectory: string;
  patterns: string[];
  ignore: string[];
}

/**
 * Resolve glob patterns to a deduplicated, ignore-filtered list of files.
 *
 * - `patterns` may include negated entries (prefixed `!`) per @actions/glob.
 * - `ignore` is appended as negated globs.
 * - Directories and non-regular files are skipped.
 */
export async function walk(opts: WalkOptions): Promise<WalkedFile[]> {
  const cwd = path.resolve(opts.workingDirectory);
  const cwdStat = await fs.stat(cwd).catch(() => null);
  if (!cwdStat || !cwdStat.isDirectory()) {
    throw new Error(`working-directory does not exist or is not a directory: ${cwd}`);
  }

  const combined = [
    ...opts.patterns.map((p) => resolvePattern(cwd, p)),
    ...opts.ignore.map((p) => '!' + resolvePattern(cwd, p)),
  ];

  const globber = await glob.create(combined.join('\n'), {
    followSymbolicLinks: false,
    implicitDescendants: true,
    matchDirectories: false,
  });

  const matches = await globber.glob();
  const files: WalkedFile[] = [];
  const seen = new Set<string>();

  for (const absolutePath of matches) {
    /* istanbul ignore if — defensive: globber.glob() already deduplicates. */
    if (seen.has(absolutePath)) continue;
    seen.add(absolutePath);

    const stat = await fs.stat(absolutePath).catch(() => null);
    /* istanbul ignore if — defensive: globber returns existing entries; matchDirectories=false. */
    if (!stat || !stat.isFile()) continue;

    files.push({
      absolutePath,
      relativePath: path.relative(cwd, absolutePath),
      size: stat.size,
    });
  }

  files.sort((a, b) => a.relativePath.localeCompare(b.relativePath));
  return files;
}

function resolvePattern(cwd: string, pattern: string): string {
  if (path.isAbsolute(pattern)) return pattern;
  if (pattern.startsWith('!')) {
    return '!' + path.join(cwd, pattern.slice(1));
  }
  return path.join(cwd, pattern);
}
