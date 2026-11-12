import { afterEach, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppContext } from '../../app/context';
import { capabilities } from '../../app/permissions';
import type { ScheduledJob } from '../../shared/api/generated/types.gen';
import { jobInput } from './model';
const api = vi.hoisted(() => ({
  listScheduledJobs: vi.fn(),
  listAgents: vi.fn(),
  listCodeRepositories: vi.fn(),
  createScheduledJob: vi.fn(),
  previewJobSchedule: vi.fn(),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => api);
import { ScheduledJobsPage } from './jobs';
afterEach(() => vi.clearAllMocks());
function setup(role: 'member' | 'viewer') {
  api.listScheduledJobs.mockResolvedValue({ data: [] });
  api.listAgents.mockResolvedValue({ data: [{ id: 'agent', name: 'Review agent' }] });
  api.listCodeRepositories.mockResolvedValue({ data: [] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AppContext.Provider
        value={{
          scope: { userID: 'u', orgID: 'org', workspaceID: 'workspace' },
          permissions: capabilities('member', role, false),
          session: {} as never,
          organization: undefined,
          workspace: undefined,
        }}
      >
        <ScheduledJobsPage />
      </AppContext.Provider>
    </QueryClientProvider>,
  );
}
it('keeps creation and execution unavailable to viewers', async () => {
  setup('viewer');
  await screen.findByText('No scheduled jobs');
  expect(screen.queryByRole('button', { name: 'New scheduled job' })).not.toBeInTheDocument();
});
it('creates repeatable work without a workflow selector', async () => {
  setup('member');
  await userEvent.click(await screen.findByRole('button', { name: 'New scheduled job' }));
  expect(screen.getByLabelText('Instructions')).toBeInTheDocument();
  expect(screen.getByLabelText('Repository scope')).toBeInTheDocument();
  expect(screen.queryByLabelText(/workflow/i)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Preview next runs' })).toBeInTheDocument();
});
it('strips server metadata from edit and pause payloads', () => {
  const job = {
    id: 'job',
    revision: 5,
    workspaceId: 'workspace',
    authorizedBy: 'u',
    createdAt: 'now',
    nextDueAt: 'later',
    name: 'Review',
    instructions: 'Review code',
    agentId: 'a',
    repositoryScope: 'none',
    repositoryIds: [],
    schedule: { frequency: 'daily', timeZone: 'UTC', hour: 9, minute: 0, weekday: 1 },
    enabled: false,
    maxDurationMinutes: 60,
  } satisfies ScheduledJob;
  expect(jobInput(job)).not.toHaveProperty('id');
  expect(jobInput(job)).not.toHaveProperty('workflowId');
  expect(jobInput(job).instructions).toBe('Review code');
});

it('saves a job definition without board-task fields', async () => {
  api.createScheduledJob.mockResolvedValue({ data: {} });
  setup('member');
  await userEvent.click(await screen.findByRole('button', { name: 'New scheduled job' }));
  await userEvent.type(screen.getByLabelText('Name'), 'Daily review');
  await userEvent.type(screen.getByLabelText('Instructions'), 'Review all repositories');
  await userEvent.selectOptions(screen.getByLabelText('Agent'), 'agent');
  await userEvent.click(screen.getByRole('button', { name: 'Save scheduled job' }));
  expect(api.createScheduledJob).toHaveBeenCalledOnce();
  const payload = api.createScheduledJob.mock.calls[0]?.[0].body;
  expect(payload.commandId).toBeTruthy();
  expect(payload.input.instructions).toBe('Review all repositories');
  expect(payload.input).not.toHaveProperty('workflowId');
  expect(payload.input).not.toHaveProperty('status');
});
