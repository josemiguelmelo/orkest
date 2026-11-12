import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppContext } from '../../app/context';
import { capabilities } from '../../app/permissions';

const api = vi.hoisted(() => ({
  listWorkflowStageIntegrations: vi.fn(),
  listWorkflowStageIntegrationProviders: vi.fn(),
  createWorkflowStageIntegration: vi.fn(),
  updateWorkflowStageIntegration: vi.fn(),
  listWorkflowStageIntegrationStatus: vi.fn(),
  retryWorkflowStageIntegrationEvent: vi.fn(),
  listCodingIntegrations: vi.fn(),
  listCodeRepositories: vi.fn(),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => api);
import { WorkflowStageIntegrationsPanel } from './workflow-stage-integrations';

const connection = {
  id: 'connection',
  workspaceId: 'workspace',
  provider: 'example',
  name: 'Review events',
  enabled: true,
  codingIntegrationId: 'coding',
  repositoryIds: ['repository'],
  config: {},
  version: 2,
  configuredBy: 'user',
  updatedAt: '2026-11-12T00:00:00Z',
};
function setup(role: 'admin' | 'viewer', connected = false) {
  api.listWorkflowStageIntegrations.mockResolvedValue({ data: connected ? [connection] : [] });
  api.listWorkflowStageIntegrationProviders.mockResolvedValue({
    data: [
      {
        provider: 'example',
        configSchema: { type: 'object', properties: {}, additionalProperties: false },
        actions: [],
      },
    ],
  });
  api.listCodingIntegrations.mockResolvedValue({
    data: [{ id: 'coding', name: 'Repository access', providerName: 'example' }],
  });
  api.listCodeRepositories.mockResolvedValue({
    data: [
      { id: 'repository', name: 'Backend', integrationId: 'coding' },
      {
        id: 'foreign-repository',
        name: 'Other connection repository',
        integrationId: 'other-coding',
      },
    ],
  });
  api.createWorkflowStageIntegration.mockResolvedValue({ data: connection });
  api.updateWorkflowStageIntegration.mockResolvedValue({ data: connection });
  api.listWorkflowStageIntegrationStatus.mockResolvedValue({
    data: [
      {
        id: 'event',
        taskId: 'task',
        actionId: 'review_added',
        ruleId: 'rule',
        state: 'blocked',
        error: 'busy',
      },
    ],
  });
  api.retryWorkflowStageIntegrationEvent.mockResolvedValue({ data: undefined });
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <AppContext.Provider
        value={{
          scope: { userID: 'user', orgID: 'org', workspaceID: 'workspace' },
          permissions: capabilities('member', role, false),
          session: {} as never,
          organization: undefined,
          workspace: undefined,
        }}
      >
        <WorkflowStageIntegrationsPanel />
      </AppContext.Provider>
    </QueryClientProvider>,
  );
}
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

it('creates a scoped connection using only repositories from selected coding credentials', async () => {
  const user = userEvent.setup();
  setup('admin');
  await user.click(await screen.findByRole('button', { name: 'Add stage connection' }));
  await user.type(await screen.findByLabelText('Connection name'), 'Review events');
  await user.selectOptions(screen.getByLabelText('Stage integration provider'), 'example');
  await user.selectOptions(screen.getByLabelText('Coding credentials'), 'coding');
  expect(
    screen.queryByRole('option', { name: 'Other connection repository' }),
  ).not.toBeInTheDocument();
  await user.selectOptions(screen.getByLabelText('Allowed repositories'), 'repository');
  const secret = 'a'.repeat(32);
  await user.type(screen.getByLabelText('Webhook secret'), secret);
  await user.click(screen.getByLabelText('Enable connection'));
  await user.click(screen.getByRole('button', { name: 'Save stage connection' }));
  await waitFor(() =>
    expect(api.createWorkflowStageIntegration).toHaveBeenCalledWith(
      expect.objectContaining({
        path: { orgID: 'org', workspaceID: 'workspace' },
        body: {
          provider: 'example',
          name: 'Review events',
          enabled: true,
          codingIntegrationId: 'coding',
          repositoryIds: ['repository'],
          config: {},
          version: 0,
          webhookSecret: secret,
        },
      }),
    ),
  );
});

it('preserves write-only webhook secrets when editing and requests retry explicitly', async () => {
  const user = userEvent.setup();
  setup('admin', true);
  await user.click(await screen.findByRole('button', { name: 'Edit connection' }));
  expect(await screen.findByLabelText('Webhook secret')).toHaveValue('');
  await user.click(screen.getByRole('button', { name: 'Save stage connection' }));
  await waitFor(() =>
    expect(api.updateWorkflowStageIntegration).toHaveBeenCalledWith(
      expect.objectContaining({
        body: {
          provider: 'example',
          name: 'Review events',
          enabled: true,
          codingIntegrationId: 'coding',
          repositoryIds: ['repository'],
          config: {},
          version: 2,
        },
      }),
    ),
  );
  await user.click(await screen.findByRole('button', { name: 'Show activity' }));
  expect(
    await screen.findByText(
      'The task is busy. Retry after its current work or interaction finishes.',
    ),
  ).toBeInTheDocument();
  expect(api.retryWorkflowStageIntegrationEvent).not.toHaveBeenCalled();
  await user.click(screen.getByRole('button', { name: 'Retry callback' }));
  await waitFor(() =>
    expect(api.retryWorkflowStageIntegrationEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        path: { orgID: 'org', workspaceID: 'workspace', eventID: 'event' },
      }),
    ),
  );
});

it('allows viewers to inspect activity without configuration or retry controls', async () => {
  const user = userEvent.setup();
  setup('viewer', true);
  await screen.findByText('Review events');
  expect(screen.queryByRole('button', { name: 'Add stage connection' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Edit connection' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Show activity' }));
  expect(
    await screen.findByText(
      'The task is busy. Retry after its current work or interaction finishes.',
    ),
  ).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Retry callback' })).not.toBeInTheDocument();
});

it('keeps stale stage and configuration effects as history without retry controls', async () => {
  const user = userEvent.setup();
  setup('admin', true);
  api.listWorkflowStageIntegrationStatus.mockResolvedValue({
    data: [
      { id: 'old-stage', taskId: 'task', ruleId: 'rule', state: 'blocked', error: 'stage_changed' },
      {
        id: 'old-config',
        taskId: 'task',
        ruleId: 'rule',
        state: 'blocked',
        error: 'configuration_changed',
      },
    ],
  });
  await user.click(await screen.findByRole('button', { name: 'Show activity' }));
  expect(
    await screen.findByText('The task has left the stage this callback was watching.'),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      'The workflow or connection changed. New events use the current configuration.',
    ),
  ).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Retry callback' })).not.toBeInTheDocument();
});
