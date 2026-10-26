import { defineConfig } from '@hey-api/openapi-ts';
export default defineConfig({
  input: '../internal/app/httpapi/openapi.yaml',
  output: { path: '.api-source' },
  plugins: [
    '@hey-api/typescript',
    { name: 'zod', requests: true, responses: true },
    { name: '@hey-api/sdk', validator: { request: 'zod', response: 'zod' } },
    '@hey-api/client-fetch',
  ],
});
