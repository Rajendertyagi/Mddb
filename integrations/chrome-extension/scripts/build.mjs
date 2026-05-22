// Bundle the extension into ./build using esbuild.
// Produces: background.js, popup.js, options.js + verbatim copy of public/.
import { build } from 'esbuild';
import { cp, mkdir, readFile, writeFile, rm, readdir, stat } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '..');
const buildDir = join(root, 'build');
const publicDir = join(root, 'public');

async function copyPublic() {
  const entries = await readdir(publicDir);
  for (const entry of entries) {
    const src = join(publicDir, entry);
    const dest = join(buildDir, entry);
    const st = await stat(src);
    if (st.isDirectory()) {
      await cp(src, dest, { recursive: true });
    } else {
      await cp(src, dest);
    }
  }
}

async function syncVersion() {
  const pkg = JSON.parse(await readFile(join(root, 'package.json'), 'utf8'));
  const manifestPath = join(buildDir, 'manifest.json');
  const manifest = JSON.parse(await readFile(manifestPath, 'utf8'));
  manifest.version = pkg.version;
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
}

async function main() {
  await rm(buildDir, { recursive: true, force: true });
  await mkdir(buildDir, { recursive: true });
  await copyPublic();
  await syncVersion();

  const common = {
    bundle: true,
    minify: true,
    sourcemap: true,
    target: ['chrome120'],
    format: 'esm',
    platform: 'browser',
    legalComments: 'none',
  };

  await build({
    ...common,
    entryPoints: [join(root, 'src/background.ts')],
    outfile: join(buildDir, 'background.js'),
  });
  await build({
    ...common,
    entryPoints: [join(root, 'src/popup-entry.ts')],
    outfile: join(buildDir, 'popup.js'),
  });
  await build({
    ...common,
    entryPoints: [join(root, 'src/options-entry.ts')],
    outfile: join(buildDir, 'options.js'),
  });

  console.log(`Built MDDB Browser extension into ${buildDir}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
