import { afterEach, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppContext } from '../../app/context';
import { capabilities } from '../../app/permissions';
import type {
  TaskTriggerIntegration,
  TaskTriggerIntegrationInput,
} from '../../shared/api/generated/types.gen';
const api = vi.hoisted(() => ({
  listTaskTriggerIntegrationProviders: vi.fn(),
  listTaskTriggerIntegrations: vi.fn(),
  listWorkspaceWorkflows: vi.fn(),
  listTasks: vi.fn(),
  updateTaskTriggerIntegration: vi.fn(),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => api);
import { TriggerIntegrations } from './trigger-integrations';
const integration: TaskTriggerIntegration = {
  id: 'integration',
  workspaceId: 'workspace',
  name: 'Review events',
  provider: 'example',
  config: { repositoryId: '123' },
  enabled: true,
  version: 7,
  rules: [
    {
      id: 'rule',
      event: 'review.published',
      workflowId: 'workflow',
      sourceStageId: 'review',
      targetStageId: 'address',
      action: 'move_task',
    },
  ],
};
afterEach(() => vi.clearAllMocks());
function setup(role: 'admin' | 'viewer') {
  api.listTaskTriggerIntegrationProviders.mockResolvedValue({
    data: [
      {
        id: 'example',
        name: 'Example provider',
        events: ['review.published'],
        configSchema: {
          type: 'object',
          properties: { repositoryId: { type: 'string', title: 'Repository ID' } },
          required: ['repositoryId'],
          additionalProperties: false,
        },
      },
    ],
  });
  api.listTaskTriggerIntegrations.mockResolvedValue({ data: [integration] });
  api.listTasks.mockResolvedValue({ data: [] });
  api.listWorkspaceWorkflows.mockResolvedValue({
    data: [
      {
        id: 'workflow',
        name: 'Default',
        version: 1,
        initialStageId: 'review',
        closedStageId: 'done',
        rules: [],
        stages: [
          { id: 'review', name: 'Review' },
          { id: 'address', name: 'Address review' },
          { id: 'done', name: 'Done' },
        ],
      },
    ],
  });
  api.updateTaskTriggerIntegration.mockResolvedValue({ data: { ...integration, version: 8 } });
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <AppContext.Provider
        value={{
          scope: { userID: 'u', orgID: 'org', workspaceID: 'workspace' },
          permissions: capabilities('member', role, false),
          session: {} as never,
          organization: undefined,
          workspace: undefined,
        }}
      >
        <TriggerIntegrations />
      </AppContext.Provider>
    </QueryClientProvider>,
  );
}
it('renders trigger configuration read-only for viewers', async () => {
  setup('viewer');
  await screen.findByText('Review events');
  expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Connect trigger' })).not.toBeInTheDocument();
  expect(screen.getByLabelText('Webhook URL')).toHaveValue(
    `${window.location.origin}/webhooks/task-trigger-integrations/integration`,
  );
});
it('uses provider descriptors and retains version, rules and the write-only secret on update', async () => {
  setup('admin');
  const user = userEvent.setup();
  await user.click(await screen.findByRole('button', { name: 'Edit' }));
  expect(await screen.findByDisplayValue('Address review')).toBeInTheDocument();
  expect(screen.getByLabelText('Repository ID')).toHaveValue('123');
  expect(screen.getByLabelText('Webhook secret')).toHaveValue('');
  await user.click(screen.getByRole('button', { name: 'Save trigger' }));
  await waitFor(() => expect(api.updateTaskTriggerIntegration).toHaveBeenCalled());
  expect(api.updateTaskTriggerIntegration.mock.calls[0]?.[0]).toMatchObject({
    path: { integrationID: 'integration' },
    body: {
      version: 7,
      provider: 'example',
      rules: integration.rules,
      config: { repositoryId: '123' },
    },
  });
  const call = api.updateTaskTriggerIntegration.mock.calls[0]?.[0] as {
    body: TaskTriggerIntegrationInput;
  };
  expect(call.body).not.toHaveProperty('webhookSecret');
});
