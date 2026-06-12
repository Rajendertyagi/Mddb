import * as os from 'node:os';
import * as path from 'node:path';
import * as fs from 'node:fs/promises';
import * as core from '@actions/core';
import { run, type RunDependencies } from '../src/main';
import { MddbClient, MddbHttpError } from '../src/client';

jest.mock('@actions/core');

const mockCore = core as jest.Mocked<typeof core>;

function stubInputs(values: Record<string, string>): void {
  mockCore.getInput.mockImplementation((name: string) => values[name] ?? '');
}

interface FakeClient {
  ping: jest.Mock;
  addDocument: jest.Mock;
}

function buildDeps(
  client: FakeClient,
  files: { absolutePath: string; relativePath: string; size: number }[],
): RunDependencies {
  return {
    walker: jest.fn(async () => files) as RunDependencies['walker'],
    createClient: jest.fn(() => client as unknown as MddbClient),
    envContext: jest.fn(() => ({ repository: 'tradik/mddb', ref: 'abc123' })),
  };
}

describe('run', () => {
  let tmp: string;

  beforeEach(async () => {
    tmp = await fs.mkdtemp(path.join(os.tmpdir(), 'mddb-action-main-'));
    mockCore.getInput.mockReset();
    mockCore.setOutput.mockReset();
    mockCore.setFailed.mockReset();
    mockCore.warning.mockReset();
    mockCore.info.mockReset();
  });

  afterEach(async () => {
    await fs.rm(tmp, { recursive: true, force: true });
  });

  it('uploads every matched file and records counts', async () => {
    const file = path.join(tmp, 'a.md');
    await fs.writeFile(file, '# Hi', 'utf8');
    stubInputs({ collection: 'docs' });

    const client: FakeClient = { ping: jest.fn(), addDocument: jest.fn() };
    const deps = buildDeps(client, [{ absolutePath: file, relativePath: 'a.md', size: 4 }]);

    const result = await run(deps);
    expect(client.ping).toHaveBeenCalledTimes(1);
    expect(client.addDocument).toHaveBeenCalledTimes(1);
    expect(result).toEqual({ scanned: 1, added: 1, failed: 0 });
    expect(mockCore.setFailed).not.toHaveBeenCalled();
    expect(mockCore.setOutput).toHaveBeenCalledWith('documents-added', '1');
  });

  it('counts failures without aborting and fails the job when fail-on-error is true', async () => {
    const a = path.join(tmp, 'a.md');
    const b = path.join(tmp, 'b.md');
    await fs.writeFile(a, 'a', 'utf8');
    await fs.writeFile(b, 'b', 'utf8');
    stubInputs({ collection: 'docs', concurrency: '1' });

    const client: FakeClient = {
      ping: jest.fn(),
      addDocument: jest
        .fn()
        .mockResolvedValueOnce(undefined)
        .mockRejectedValueOnce(new Error('boom')),
    };
    const deps = buildDeps(client, [
      { absolutePath: a, relativePath: 'a.md', size: 1 },
      { absolutePath: b, relativePath: 'b.md', size: 1 },
    ]);

    const result = await run(deps);
    expect(result).toEqual({ scanned: 2, added: 1, failed: 1 });
    expect(mockCore.setFailed).toHaveBeenCalledWith(expect.stringMatching(/failed 1/));
    expect(mockCore.warning).toHaveBeenCalled();
  });

  it('downgrades failures to warnings when fail-on-error is false', async () => {
    const file = path.join(tmp, 'a.md');
    await fs.writeFile(file, 'a', 'utf8');
    stubInputs({ collection: 'docs', 'fail-on-error': 'false' });

    const client: FakeClient = {
      ping: jest.fn(),
      addDocument: jest.fn().mockRejectedValue(new Error('boom')),
    };
    const deps = buildDeps(client, [{ absolutePath: file, relativePath: 'a.md', size: 1 }]);

    const result = await run(deps);
    expect(result.failed).toBe(1);
    expect(mockCore.setFailed).not.toHaveBeenCalled();
    expect(mockCore.warning).toHaveBeenCalled();
  });

  it('INT-004: never logs the server response body, only the status', async () => {
    const file = path.join(tmp, 'a.md');
    await fs.writeFile(file, 'a', 'utf8');
    stubInputs({ collection: 'docs', 'fail-on-error': 'false' });

    const secretBody = 'SECRET-token=vk_should_never_be_logged';
    const client: FakeClient = {
      ping: jest.fn(),
      addDocument: jest
        .fn()
        .mockRejectedValue(new MddbHttpError('HTTP 500', 500, secretBody)),
    };
    const deps = buildDeps(client, [{ absolutePath: file, relativePath: 'a.md', size: 1 }]);

    await run(deps);

    expect(mockCore.warning).toHaveBeenCalled();
    const logged = mockCore.warning.mock.calls.map((c) => String(c[0])).join('\n');
    expect(logged).not.toContain(secretBody);
    expect(logged).toContain('status=500');
  });

  it('returns immediately when no files match', async () => {
    stubInputs({ collection: 'docs' });
    const client: FakeClient = { ping: jest.fn(), addDocument: jest.fn() };
    const deps = buildDeps(client, []);
    const result = await run(deps);
    expect(result).toEqual({ scanned: 0, added: 0, failed: 0 });
    expect(client.ping).not.toHaveBeenCalled();
    expect(client.addDocument).not.toHaveBeenCalled();
  });

  it('skips network calls in dry-run mode', async () => {
    const file = path.join(tmp, 'a.md');
    await fs.writeFile(file, 'a', 'utf8');
    stubInputs({ collection: 'docs', 'dry-run': 'true' });

    const client: FakeClient = { ping: jest.fn(), addDocument: jest.fn() };
    const deps = buildDeps(client, [{ absolutePath: file, relativePath: 'a.md', size: 1 }]);

    const result = await run(deps);
    expect(result).toEqual({ scanned: 1, added: 0, failed: 0 });
    expect(client.ping).not.toHaveBeenCalled();
    expect(client.addDocument).not.toHaveBeenCalled();
  });

  it('truncates the dry-run preview when there are many files', async () => {
    stubInputs({ collection: 'docs', 'dry-run': 'true' });
    const files = Array.from({ length: 25 }, (_, i) => ({
      absolutePath: path.join(tmp, `f${i}.md`),
      relativePath: `f${i}.md`,
      size: 1,
    }));
    const client: FakeClient = { ping: jest.fn(), addDocument: jest.fn() };
    const deps = buildDeps(client, files);
    const result = await run(deps);
    expect(result.scanned).toBe(25);
    expect(mockCore.info).toHaveBeenCalledWith(expect.stringContaining('… and 5 more'));
  });
});
