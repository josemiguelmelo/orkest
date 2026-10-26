import { test, expect, type Page } from '@playwright/test';
import {
  agent,
  organization,
  session,
  task,
  workspace,
  githubProvider,
} from '../src/test/fixtures';
test.beforeEach(({ page }) => {
  page.on('pageerror', (error) => console.error(error.message));
  page.on('console', (message) => {
    if (message.type() === 'error') console.error(message.text());
  });
  page.on('response', (response) => {
    if (response.status() >= 400) console.error(response.status(), response.url());
  });
});
const base = '/api/organizations/org-1/workspaces/ws-1';
async function mock(page: Page, viewer = false) {
  let tasks = [task];
  await page.route(
    (url) => url.pathname.startsWith('/api/'),
    async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const path = url.pathname;
      if (path === '/api/me')
        return route.fulfill({
          json: {
            ...session,
            organizations: [{ ...organization, role: viewer ? 'member' : 'owner' }],
          },
        });
      if (path === '/api/organizations')
        return route.fulfill({ json: [{ ...organization, role: viewer ? 'member' : 'owner' }] });
      if (path === '/api/organizations/org-1')
        return route.fulfill({ json: { ...organization, role: viewer ? 'member' : 'owner' } });
      if (path === '/api/organizations/org-1/workspaces')
        return route.fulfill({
          json: [
            { ...workspace, role: viewer ? 'viewer' : 'admin' },
            {
              ...workspace,
              id: 'ws-2',
              name: 'Other workspace',
              role: viewer ? 'viewer' : 'admin',
            },
          ],
        });
      if (path === base)
        return route.fulfill({ json: { ...workspace, role: viewer ? 'viewer' : 'admin' } });
      if (path === base + '/tasks') {
        if (request.method() === 'POST') {
          const body: unknown = request.postDataJSON();
          const { zTaskInput } = await import('../src/shared/api/generated/zod.gen');
          const input = zTaskInput.parse(body);
          const created = { ...task, ...input, id: 'task-2', title: input.title };
          tasks = [...tasks, created];
          return route.fulfill({ status: 201, json: created });
        }
        return route.fulfill({ json: tasks });
      }
      if (path === base + '/tasks/task-1') return route.fulfill({ json: task });
      if (path === base + '/agents') return route.fulfill({ json: [agent] });
      if (path === '/api/task-integration-providers')
        return route.fulfill({ json: [githubProvider] });
      if (path.startsWith(base)) return route.fulfill({ json: [] });
      if (path === '/api/organizations/org-1/workspaces/ws-2')
        return route.fulfill({ json: { ...workspace, id: 'ws-2', name: 'Other workspace' } });
      if (path === '/api/organizations/org-1/workspaces/ws-2/tasks')
        return route.fulfill({ json: [] });
      if (path.startsWith('/api/organizations/org-1/workspaces/ws-2/'))
        return route.fulfill({ json: [] });
      return route.fulfill({ status: 404, json: { error: 'not_found' } });
    },
  );
}
test('create a task, preserve URL filters and refresh a deep link', async ({ page }) => {
  await mock(page);
  await page.goto('/organizations/org-1/workspaces/ws-1/tasks');
  await expect(page.getByRole('heading', { name: 'Tasks', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'New task' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Title', { exact: true }).fill('Improve onboarding');
  await expect(dialog.getByRole('option', { name: 'In progress', exact: true })).toBeDisabled();
  await dialog.getByRole('button', { name: 'Save changes' }).click();
  await expect(dialog).not.toBeVisible();
  await expect(
    page.getByRole('heading', { name: 'Improve onboarding', exact: true }),
  ).toBeVisible();
  await page.getByRole('textbox', { name: 'Search tasks' }).fill('workbench');
  await expect(page).toHaveURL(/q=workbench/);
  await page.reload();
  await expect(page.getByRole('textbox', { name: 'Search tasks' })).toHaveValue('workbench');
  await page.getByRole('heading', { name: task.title, exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Description', exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('heading', { name: task.title, exact: true })).toBeVisible();
});
test('viewer can read but cannot mutate', async ({ page }) => {
  await mock(page, true);
  await page.goto('/organizations/org-1/workspaces/ws-1/tasks');
  await expect(page.getByRole('heading', { name: task.title, exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'New task' })).toHaveCount(0);
  await page.goto('/organizations/org-1/workspaces/ws-1/tasks/task-1');
  await expect(page.getByRole('heading', { name: task.title, exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Edit task' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Add comment' })).toHaveCount(0);
});
test('workspace switching does not retain tasks from the previous scope', async ({
  page,
}, info) => {
  await mock(page);
  await page.goto('/organizations/org-1/workspaces/ws-1/tasks');
  await expect(page.getByRole('heading', { name: task.title, exact: true })).toBeVisible();
  if (info.project.name === 'mobile')
    await page.getByRole('button', { name: 'Open navigation' }).click();
  await page.getByLabel('Workspace', { exact: true }).selectOption('ws-2');
  await expect(page.getByRole('heading', { name: 'No tasks yet' })).toBeVisible();
  await expect(page.getByRole('heading', { name: task.title, exact: true })).toHaveCount(0);
});
test('session expiry clears the workbench', async ({ page }) => {
  await mock(page);
  await page.goto('/organizations/org-1/workspaces/ws-1/tasks');
  await expect(page.getByRole('heading', { name: task.title, exact: true })).toBeVisible();
  await page.route(
    (url) => url.pathname.startsWith('/api/'),
    (route) => route.fulfill({ status: 401, json: { error: 'unauthorized' } }),
  );
  await page.goto('/organizations/org-1/workspaces/ws-1/agents');
  await expect(page.getByRole('link', { name: /Sign in with Keycloak/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: task.title, exact: true })).toHaveCount(0);
});
test('keyboard focus enters a dialog and Escape closes it', async ({ page }) => {
  await mock(page);
  await page.goto('/organizations/org-1/workspaces/ws-1/tasks');
  const trigger = page.getByRole('button', { name: 'New task' });
  await trigger.click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByRole('dialog').getByRole('button', { name: 'Close' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
});
