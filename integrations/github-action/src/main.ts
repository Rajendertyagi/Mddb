import * as core from '@actions/core';
import { readInputs, type ActionInputs } from './inputs';
import { walk, type WalkedFile } from './walker';
import { buildDocument } from './document';
import { MddbClient, MddbHttpError } from './client';

export interface RunResult {
  scanned: number;
  added: number;
  failed: number;
}

export interface RunDependencies {
  /** Resolve glob patterns to files. */
  walker: typeof walk;
  /** Construct an MDDB HTTP client. */
  createClient: (inputs: ActionInputs) => MddbClient;
  /** Resolve repo slug + ref from the environment. */
  envContext: () => { repository?: string; ref?: string };
}

const DEFAULT_DEPS: RunDependencies = {
  walker: walk,
  createClient: (inputs) =>
    new MddbClient({
      baseUrl: inputs.mddbUrl,
      apiKey: inputs.apiKey,
      timeoutSeconds: inputs.timeoutSeconds,
      verifySsl: inputs.verifySsl,
    }),
  envContext: () => ({
    repository: process.env.GITHUB_REPOSITORY,
    ref: process.env.GITHUB_SHA ?? process.env.GITHUB_REF,
  }),
};

/**
 * Sync repository files to MDDB. Returns counts; throws only on hard failures
 * (input validation, ping failure). Per-document failures are accumulated.
 */
export async function run(deps: RunDependencies = DEFAULT_DEPS): Promise<RunResult> {
  const inputs = readInputs();
  const ctx = deps.envContext();

  core.info(`MDDB target:    ${inputs.mddbUrl}`);
  core.info(`Collection:     ${inputs.collection}`);
  core.info(`Patterns:       ${inputs.patterns.join(', ')}`);
  if (inputs.ignore.length > 0) core.info(`Ignore:         ${inputs.ignore.join(', ')}`);
  core.info(
    `Key strategy:   ${inputs.keyStrategy}${inputs.keyPrefix ? ' (prefix=' + inputs.keyPrefix + ')' : ''}`,
  );
  core.info(`Dry run:        ${inputs.dryRun}`);

  const files = await deps.walker({
    workingDirectory: inputs.workingDirectory,
    patterns: inputs.patterns,
    ignore: inputs.ignore,
  });
  core.info(`Scanned ${files.length} files.`);

  if (files.length === 0) {
    return finalise({ scanned: 0, added: 0, failed: 0 }, inputs);
  }

  if (inputs.dryRun) {
    for (const f of files.slice(0, 20)) {
      core.info(`  • ${f.relativePath} (${f.size} bytes)`);
    }
    if (files.length > 20) core.info(`  … and ${files.length - 20} more.`);
    return finalise({ scanned: files.length, added: 0, failed: 0 }, inputs);
  }

  const client = deps.createClient(inputs);
  await client.ping();
  core.info('Ping OK.');

  const result = await syncFiles(client, files, inputs, ctx);
  return finalise(result, inputs);
}

async function syncFiles(
  client: MddbClient,
  files: WalkedFile[],
  inputs: ActionInputs,
  ctx: { repository?: string; ref?: string },
): Promise<RunResult> {
  let added = 0;
  let failed = 0;

  const queue = files.slice();
  const workers: Promise<void>[] = [];
  const workerCount = Math.min(inputs.concurrency, queue.length);

  for (let i = 0; i < workerCount; i++) {
    workers.push(
      (async () => {
        for (;;) {
          const file = queue.shift();
          if (!file) return;
          try {
            const doc = await buildDocument({
              collection: inputs.collection,
              language: inputs.language,
              keyStrategy: inputs.keyStrategy,
              keyPrefix: inputs.keyPrefix,
              relativePath: file.relativePath,
              absolutePath: file.absolutePath,
              repository: ctx.repository,
              ref: ctx.ref,
            });
            await client.addDocument(doc);
            added++;
            core.info(`  ✓ ${file.relativePath} → key=${doc.key}`);
          } catch (err) {
            failed++;
            const message =
              err instanceof MddbHttpError
                ? `${err.message} body=${err.body.slice(0, 200)}`
                : err instanceof Error
                  ? err.message
                  : String(err);
            core.warning(`  ✗ ${file.relativePath}: ${message}`);
          }
        }
      })(),
    );
  }

  await Promise.all(workers);
  return { scanned: files.length, added, failed };
}

function finalise(result: RunResult, inputs: ActionInputs): RunResult {
  core.setOutput('documents-scanned', String(result.scanned));
  core.setOutput('documents-added', String(result.added));
  core.setOutput('documents-failed', String(result.failed));

  const summary = `Scanned ${result.scanned}, added ${result.added}, failed ${result.failed}.`;
  if (result.failed > 0) {
    if (inputs.failOnError) {
      core.setFailed(summary);
    } else {
      core.warning(summary);
    }
  } else {
    core.info(summary);
  }
  return result;
}

/* istanbul ignore next */
if (require.main === module) {
  run().catch((err: unknown) => {
    const message = err instanceof Error ? err.message : String(err);
    core.setFailed(message);
  });
}
