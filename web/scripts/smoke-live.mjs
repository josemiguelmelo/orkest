// Opt-in: run only against a disposable local database. Creates and removes a
// dedicated organization through the real API; never starts agent execution.
import os from 'node:os';
import path from 'node:path';
import { chromium, expect } from '@playwright/test';
import { zCurrentUser, zOrganization, zWorkspace } from '../src/shared/api/generated/zod.gen.js';
const base = process.env.ORKEST_LIVE_BASE_URL ?? 'http://localhost:8080';
const login = process.env.ORKEST_LIVE_LOGIN_URL ?? `${base}/auth/login`;
const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage();
page.setDefaultTimeout(15000);
page.setDefaultNavigationTimeout(15000);
let orgID;
async function authenticate() {
  await page.goto(login);
  const field = page.getByRole('textbox', { name: /username/i });
  const needsLogin = await Promise.race([
    field.waitFor({ state: 'visible' }).then(() => true),
    page.waitForURL((url) => url.pathname === '/' && url.port !== '8082').then(() => false),
  ]);
  if (needsLogin) {
    await field.fill(process.env.ORKEST_LIVE_USERNAME ?? 'local');
    await page
      .getByLabel('Password', { exact: true })
      .fill(process.env.ORKEST_LIVE_PASSWORD ?? 'local');
    await page.getByRole('button', { name: /sign in/i }).click();
    await page.waitForURL((url) => url.pathname === '/' && url.port !== '8082');
  }
}
try {
  await authenticate();
  console.log('Real OIDC callback completed.');
  await page.goto(base);
  await expect(page.getByRole('heading', { name: 'Organizations', exact: true })).toBeVisible();
  const me = await context.request.get(`${base}/api/me`);
  expect(me.status()).toBe(200);
  zCurrentUser.parse(await me.json());
  await page.getByRole('button', { name: 'New organization' }).click();
  await page
    .getByRole('dialog')
    .getByLabel('Name', { exact: true })
    .fill('Webapp smoke verification');
  const createdOrg = page.waitForResponse(
    (r) => new URL(r.url()).pathname === '/api/organizations' && r.request().method() === 'POST',
  );
  await page.getByRole('dialog').getByRole('button', { name: 'Save changes' }).click();
  console.log('Organization submitted.');
  const orgResponse = await createdOrg;
  expect(orgResponse.status()).toBe(201);
  orgID = zOrganization.parse(await orgResponse.json()).id;
  await page.getByRole('button', { name: 'New workspace' }).click();
  await page.getByRole('dialog').getByLabel('Name', { exact: true }).fill('Smoke project');
  const createdWorkspace = page.waitForResponse(
    (r) => new URL(r.url()).pathname.endsWith('/workspaces') && r.request().method() === 'POST',
  );
  await page.getByRole('dialog').getByRole('button', { name: 'Save changes' }).click();
  console.log('Workspace submitted.');
  const wsResponse = await createdWorkspace;
  expect(wsResponse.status()).toBe(201);
  const ws = zWorkspace.parse(await wsResponse.json());
  await page.getByRole('link', { name: /Smoke project/ }).click();
  await page.getByRole('button', { name: 'New task' }).click();
  await page
    .getByRole('dialog')
    .getByLabel('Title', { exact: true })
    .fill('Verify real task persistence');
  await page
    .getByRole('dialog')
    .getByLabel('Description', { exact: true })
    .fill('Created through the management UI against PostgreSQL.');
  await page.getByRole('dialog').getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  console.log('Task created.');
  await page.getByRole('heading', { name: 'Verify real task persistence', exact: true }).click();
  await page.getByLabel('Comment', { exact: true }).fill('Real API comment verification.');
  await page.getByRole('button', { name: 'Add comment' }).click();
  await expect(page.getByText('Real API comment verification.', { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByText('Real API comment verification.', { exact: true })).toBeVisible();
  await page.goto(`${base}/organizations/${orgID}/workspaces/${ws.id}/agents`);
  await page.getByRole('button', { name: 'New agent' }).click();
  await page.getByRole('dialog').getByLabel('Name', { exact: true }).fill('Smoke agent');
  await page.getByRole('dialog').getByLabel('Model provider', { exact: true }).fill('opencode');
  await page
    .getByRole('dialog')
    .getByLabel('Model identifier', { exact: true })
    .fill('configured-model');
  await page.getByRole('dialog').getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Smoke agent', exact: true })).toBeVisible();
  await page.goto(`${base}/organizations/${orgID}/workspaces/${ws.id}/mcp-servers`);
  await page.getByRole('button', { name: 'New MCP server' }).click();
  await page.getByRole('dialog').getByLabel('Name', { exact: true }).fill('Smoke MCP');
  await page
    .getByRole('dialog')
    .getByLabel('Endpoint URL', { exact: true })
    .fill('https://example.test/mcp');
  await page.getByRole('dialog').getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Smoke MCP', exact: true })).toBeVisible();
  await page.goto(`${base}/organizations/${orgID}/workspaces/${ws.id}/repositories`);
  await page.getByRole('button', { name: 'Register repository' }).click();
  await page
    .getByRole('dialog')
    .getByLabel('Repository URL', { exact: true })
    .fill('https://github.com/octocat/Hello-World');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Register repository', exact: true })
    .click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.goto(`${base}/organizations/${orgID}/workspaces/${ws.id}/components`);
  await page.getByRole('button', { name: 'New component' }).click();
  await page.getByRole('dialog').getByLabel('Identifier', { exact: true }).fill('frontend');
  await page.getByRole('dialog').getByLabel('Name', { exact: true }).fill('Frontend');
  await page
    .getByRole('dialog')
    .getByLabel('Repository', { exact: true })
    .selectOption({ label: 'https://github.com/octocat/Hello-World.git' });
  await page.getByRole('dialog').getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Frontend', exact: true })).toBeVisible();
  await page.goto(`${base}/organizations/${orgID}/workspaces/${ws.id}/skills`);
  await page.getByRole('button', { name: 'New skill' }).click();
  await page
    .getByRole('dialog')
    .getByLabel('SKILL.md', { exact: true })
    .fill(
      `---\nname: smoke-${Date.now()}\ndescription: Verify the management UI.\n---\n\nFollow project instructions carefully.`,
    );
  await page.getByRole('dialog').getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'View instructions' })).toBeVisible();
  await page.goto(`${base}/organizations/${orgID}/workspaces/${ws.id}/integrations`);
  await page.getByRole('button', { name: 'Connect source' }).click();
  await expect(page.getByRole('dialog').getByLabel('projectUrl *', { exact: true })).toBeVisible();
  await page
    .getByRole('dialog')
    .getByLabel('Provider', { exact: true })
    .selectOption('jira_server');
  await expect(page.getByRole('dialog').getByLabel('projects *', { exact: true })).toBeVisible();
  await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();
  await page.goto(`${base}/organizations/${orgID}/workspaces/${ws.id}/tasks`);
  await expect(
    page.getByRole('heading', { name: 'Verify real task persistence', exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: path.join(os.tmpdir(), 'orkest-webapp-desktop.png'),
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: path.join(os.tmpdir(), 'orkest-webapp-mobile.png'),
    fullPage: true,
  });
  await context.request.post(`${base}/auth/logout`);
  await page.reload();
  await expect(page.getByRole('link', { name: /Sign in with Keycloak/ })).toBeVisible();
  console.log(
    'Real Keycloak login/logout, organization/workspace creation, task/comment persistence, agent/MCP/skill/repository/component creation, real provider catalog forms, and Go-served SPA deep links verified.',
  );
} catch (error) {
  await page.screenshot({
    path: path.join(os.tmpdir(), 'orkest-webapp-live-failure.png'),
    fullPage: true,
  });
  console.error('Live smoke failed:', error.message);
  process.exitCode = 1;
} finally {
  try {
    if (orgID) {
      let response = await context.request.delete(`${base}/api/organizations/${orgID}`);
      if (response.status() === 401) {
        await authenticate();
        response = await context.request.delete(`${base}/api/organizations/${orgID}`);
      }
      if (response.status() !== 204) throw new Error('Smoke organization cleanup failed.');
      console.log('Smoke organization removed.');
    }
  } finally {
    await browser.close();
  }
}
