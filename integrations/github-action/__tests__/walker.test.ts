import * as os from 'node:os';
import * as path from 'node:path';
import * as fs from 'node:fs/promises';
import { walk } from '../src/walker';

async function writeTree(root: string, files: Record<string, string>): Promise<void> {
  for (const [rel, content] of Object.entries(files)) {
    const abs = path.join(root, rel);
    await fs.mkdir(path.dirname(abs), { recursive: true });
    await fs.writeFile(abs, content, 'utf8');
  }
}

describe('walk', () => {
  let tmp: string;

  beforeEach(async () => {
    tmp = await fs.mkdtemp(path.join(os.tmpdir(), 'mddb-action-walk-'));
  });

  afterEach(async () => {
    await fs.rm(tmp, { recursive: true, force: true });
  });

  it('matches files by glob and returns deterministic order', async () => {
    await writeTree(tmp, {
      'b.md': 'b',
      'a.md': 'a',
      'sub/c.md': 'c',
      'sub/skip.txt': 'x',
    });

    const result = await walk({ workingDirectory: tmp, patterns: ['**/*.md'], ignore: [] });
    expect(result.map((f) => f.relativePath)).toEqual(['a.md', 'b.md', 'sub/c.md']);
    expect(result[0].size).toBeGreaterThan(0);
  });

  it('respects ignore patterns', async () => {
    await writeTree(tmp, {
      'README.md': 'r',
      'node_modules/foo/bar.md': 'b',
    });

    const result = await walk({
      workingDirectory: tmp,
      patterns: ['**/*.md'],
      ignore: ['node_modules/**'],
    });
    expect(result.map((f) => f.relativePath)).toEqual(['README.md']);
  });

  it('throws for a non-existent working directory', async () => {
    await expect(
      walk({ workingDirectory: path.join(tmp, 'nope'), patterns: ['**/*'], ignore: [] }),
    ).rejects.toThrow(/working-directory/);
  });

  it('deduplicates files matched by multiple patterns', async () => {
    await writeTree(tmp, { 'a.md': 'x' });
    const result = await walk({
      workingDirectory: tmp,
      patterns: ['**/*.md', 'a.md'],
      ignore: [],
    });
    expect(result).toHaveLength(1);
  });

  it('passes absolute patterns through unchanged', async () => {
    await writeTree(tmp, { 'a.md': 'a' });
    const result = await walk({
      workingDirectory: tmp,
      patterns: [path.join(tmp, '*.md')],
      ignore: [path.join(tmp, 'b.md')],
    });
    expect(result.map((f) => f.relativePath)).toEqual(['a.md']);
  });

  it('supports inline negated patterns', async () => {
    await writeTree(tmp, {
      'docs/a.md': 'a',
      'docs/draft/b.md': 'b',
    });
    const result = await walk({
      workingDirectory: tmp,
      patterns: ['**/*.md', '!docs/draft/**'],
      ignore: [],
    });
    expect(result.map((f) => f.relativePath)).toEqual(['docs/a.md']);
  });
});
