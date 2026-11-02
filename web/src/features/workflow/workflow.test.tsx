import { afterEach, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppContext } from '../../app/context';
import { capabilities } from '../../app/permissions';
import type { WorkspaceWorkflow } from '../../shared/api/generated/types.gen';
const api = vi.hoisted(() => ({
  listWorkspaceWorkflows: vi.fn(),
  createWorkspaceWorkflow: vi.fn(),
  deleteWorkspaceWorkflow: vi.fn(),
  saveWorkspaceWorkflow: vi.fn(),
  listAgents: vi.fn(),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => api);
import { WorkflowPage } from './workflow';
const definition: WorkspaceWorkflow = {
  id: 'workspace',
  name: 'Default',
  version: 3,
  stages: [
    {
      id: 'ready',
      name: 'Ready',
      description: 'Ready to work',
      responsibleAgentId: '',
      autoStart: false,
      autoAdvance: false,
    },
    {
      id: 'closed',
      name: 'Closed',
      description: 'Finished',
      responsibleAgentId: '',
      autoStart: false,
      autoAdvance: false,
    },
  ],
};
afterEach(() => vi.clearAllMocks());
function setup(role: 'admin' | 'viewer') {
  api.listWorkspaceWorkflows.mockResolvedValue({ data: [definition] });
  api.listAgents.mockResolvedValue({ data: [] });
  api.saveWorkspaceWorkflow.mockResolvedValue({ data: { ...definition, version: 4 } });
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
        <WorkflowPage />
      </AppContext.Provider>
    </QueryClientProvider>,
  );
}
it('saves stable stage IDs, ordering and defaults for a newly created stage', async () => {
  setup('admin');
  expect(screen.queryByLabelText('New tasks start in')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Closed source tasks go to')).not.toBeInTheDocument();
  expect(await screen.findByText('Starts here')).toBeInTheDocument();
  expect(screen.getByText('Final stage')).toBeInTheDocument();
  const user = userEvent.setup();
  await user.click(await screen.findByRole('button', { name: 'Add stage' }));
  const names = screen.getAllByLabelText('Name');
  await user.clear(names[0]!);
  await user.type(names[0]!, 'Backlog');
  await user.click(screen.getAllByRole('button', { name: 'Move earlier' })[2]!);
  await user.click(screen.getByRole('button', { name: 'Save workflow' }));
  await waitFor(() => expect(api.saveWorkspaceWorkflow).toHaveBeenCalled());
  const call = api.saveWorkspaceWorkflow.mock.calls[0]?.[0] as { body: WorkspaceWorkflow };
  expect(call.body.version).toBe(3);
  expect(call.body.stages.map((s) => s.name)).toEqual(['Backlog', 'New stage', 'Closed']);
  expect(call.body.stages[0]?.id).toBe('ready');
  expect(call.body.stages[1]?.autoStart).toBe(false);
  expect(call.body.stages[1]?.autoAdvance).toBe(false);
});
it('renders workflow settings read-only for viewers', async () => {
  setup('viewer');
  await screen.findAllByDisplayValue('Ready');
  expect(screen.getAllByLabelText('Name')[0]).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Save workflow' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Add stage' })).not.toBeInTheDocument();
});
