import { createClient } from '@hey-api/openapi-ts';
import fs from 'node:fs/promises';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import config from '../openapi-ts.config.ts';

process.chdir(fileURLToPath(new URL('..', import.meta.url)));
const output = process.argv[2] ?? 'src/shared/api/generated';
if (!['src/shared/api/generated', '.api-check'].includes(output))
  throw new Error('Unsupported generation destination');
await fs.rm(output, { recursive: true, force: true });
await createClient(config);
const source = '.api-source';
const files = (await fs.readdir(source, { recursive: true }))
  .filter((file) => file.endsWith('.ts'))
  .map((file) => path.join(source, file));
// The upstream Fetch runtime does not support exactOptionalPropertyTypes yet.
// Compile it as a library; consumers still check its declarations under the
// application's strict settings. Generated source is never patched by hand.
const result = spawnSync(
  process.execPath,
  [
    'node_modules/typescript/bin/tsc',
    ...files,
    '--outDir',
    output,
    '--rootDir',
    source,
    '--declaration',
    '--target',
    'ES2022',
    '--module',
    'ESNext',
    '--moduleResolution',
    'Bundler',
    '--strict',
    '--skipLibCheck',
    '--noEmitOnError',
  ],
  { stdio: 'inherit' },
);
if (result.status !== 0) process.exit(result.status ?? 1);
// TypeScript's declaration printer can retain spaces after multiline parameters.
// Normalize emitted whitespace deterministically rather than editing artifacts.
for (const file of await fs.readdir(output, { recursive: true })) {
  if (!/\.(js|ts)$/.test(file)) continue;
  const destination = path.join(output, file);
  const contents = await fs.readFile(destination, 'utf8');
  await fs.writeFile(destination, contents.replace(/[\t ]+$/gm, ''));
}
