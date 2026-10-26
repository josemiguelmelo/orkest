import fs from 'node:fs/promises';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
process.chdir(fileURLToPath(new URL('..', import.meta.url)));
const output = '.api-check';
await fs.rm(output, { recursive: true, force: true });
const result = spawnSync(process.execPath, ['scripts/generate-api.mjs', output], {
  stdio: 'inherit',
});
if (result.status !== 0) process.exit(result.status ?? 1);
async function contents(directory) {
  const files = (await fs.readdir(directory, { recursive: true }))
    .filter((file) => /\.(js|ts)$/.test(file))
    .sort();
  return Promise.all(
    files.map(async (file) => [file, await fs.readFile(`${directory}/${file}`, 'utf8')]),
  );
}
const actual = await contents('src/shared/api/generated');
const expected = await contents(output);
await fs.rm(output, { recursive: true, force: true });
if (JSON.stringify(actual) !== JSON.stringify(expected)) {
  console.error('Generated API is stale. Run npm run generate:api in web/.');
  process.exit(1);
}
console.log('Generated API matches the platform contract.');
