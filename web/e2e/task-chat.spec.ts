import { test, expect, type Page } from '@playwright/test';
import { agent, organization, session, task, workspace } from '../src/test/fixtures';

const base = '/api/organizations/org-1/workspaces/ws-1';
const completedRun = {
  runId: 'run-1',
  taskId: task.id,
  taskSessionId: 'session-1',
  status: 'completed',
  lastSequence: 9,
  updatedAt: task.updatedAt,
};
const initial = [
  { type: 'prompt_accepted', text: 'Make the task chat easier to use.', commandId: 'turn-1' },
  { type: 'prompt_started', commandId: 'turn-1' },
  {
    type: 'update',
    commandId: 'turn-1',
    update: { kind: 'text', text: 'I’ll keep the composer available and ' },
  },
  {
    type: 'update',
    commandId: 'turn-1',
    update: { kind: 'text', text: 'make each reply easier to follow.' },
  },
  {
    type: 'update',
    commandId: 'turn-1',
    update: {
      kind: 'tool_call',
      toolCallId: 'tool-1',
      title: 'Read task-chat.tsx',
      status: 'pending',
    },
  },
  {
    type: 'update',
    commandId: 'turn-1',
    update: { kind: 'tool_call_update', toolCallId: 'tool-1', status: 'completed' },
  },
  {
    type: 'update',
    commandId: 'turn-1',
    update: {
      kind: 'text',
      text: 'The conversation is ready for your next message.\n\n- Your task context is preserved.\n- Tool activity is grouped below each response.\n- Send a follow-up whenever you’re ready.',
    },
  },
  { type: 'prompt_completed', commandId: 'turn-1' },
  { type: 'run_completed' },
];
async function mockChat(page: Page) {
  let runs = [completedRun];
  const updates: Record<string, unknown>[] = [];
  const histories: Record<string, Record<string, unknown>[]> = { 'run-1': initial };
  await page.route(
    (url) => url.pathname.startsWith('/api/'),
    async (route) => {
      const request = route.request();
      const pathname = new URL(request.url()).pathname;
      if (pathname === '/api/me') return route.fulfill({ json: session });
      if (pathname === '/api/organizations') return route.fulfill({ json: [organization] });
      if (pathname === '/api/organizations/org-1') return route.fulfill({ json: organization });
      if (pathname === '/api/organizations/org-1/workspaces')
        return route.fulfill({ json: [workspace] });
      if (pathname === base) return route.fulfill({ json: workspace });
      if (pathname === base + '/agents') return route.fulfill({ json: [agent] });
      if (pathname === base + '/tasks/task-1') {
        if (request.method() === 'PUT') {
          const body = request.postDataJSON() as Record<string, unknown>;
          updates.push(body);
          const runId = `run-${runs.length + 1}`;
          runs = [{ ...completedRun, runId }, ...runs];
          histories[runId] = [
            { type: 'prompt_accepted', commandId: body.commandId, text: body.promptText },
            {
              type: 'update',
              commandId: body.commandId,
              update: {
                kind: 'text',
                text: 'The follow-up is complete. Your context is still here.',
              },
            },
            { type: 'prompt_completed', commandId: body.commandId },
            { type: 'run_completed' },
          ];
        }
        return route.fulfill({ json: { ...task, status: 'in_progress' } });
      }
      if (pathname === base + '/tasks/task-1/runs') return route.fulfill({ json: runs });
      const match = pathname.match(/\/runs\/(run-\d+)\/events$/);
      if (match?.[1])
        return route.fulfill({
          json: (histories[match[1]] ?? []).map((payload, index) => ({
            runId: match[1],
            sequence: index + 1,
            payload,
          })),
        });
      if (pathname.startsWith(base)) return route.fulfill({ json: [] });
      return route.fulfill({ status: 404, json: { error: 'not_found' } });
    },
  );
  return updates;
}

test('completed runs keep the conversation available for repeated follow-ups', async ({
  page,
}, info) => {
  const updates = await mockChat(page);
  await page.goto('/organizations/org-1/workspaces/ws-1/tasks/task-1');
  const chat = page.getByRole('region', { name: 'Task chat' });
  const composer = chat.getByRole('textbox', { name: 'Message the agent' });
  await expect(chat.getByText('Ready for follow-up', { exact: true })).toBeVisible();
  await expect(
    chat.getByText('I’ll keep the composer available and make each reply easier to follow.'),
  ).toBeVisible();
  await expect(composer).toBeEnabled();
  await composer.fill('Add keyboard shortcuts too.');
  await composer.press('Enter');
  await expect.poll(() => updates.length).toBe(1);
  expect(updates[0]).toMatchObject({
    sessionMode: 'continue',
    startWork: true,
    promptText: 'Add keyboard shortcuts too.',
  });
  await expect(composer).toHaveValue('');
  await expect(composer).toBeEnabled();
  await composer.fill('Now make the empty state clearer.');
  await chat.getByRole('button', { name: 'Send message' }).click();
  await expect.poll(() => updates.length).toBe(2);
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await composer.scrollIntoViewIfNeeded();
  await page.screenshot({ path: info.outputPath('task-chat.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});

test('tool activity is collapsed without splitting streamed assistant text', async ({ page }) => {
  await mockChat(page);
  await page.goto('/organizations/org-1/workspaces/ws-1/tasks/task-1');
  const chat = page.getByRole('region', { name: 'Task chat' });
  await expect(chat.getByLabel('Agent response')).toHaveCount(1);
  const tools = chat.locator('details.agent-tools');
  await expect(tools).not.toHaveAttribute('open');
  await tools.locator('summary').click();
  await expect(tools.getByText('completed', { exact: true })).toBeVisible();
});
