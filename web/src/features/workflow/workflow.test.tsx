import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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
  listWorkflowStageIntegrations: vi.fn(),
  listWorkflowStageIntegrationProviders: vi.fn(),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => api);
import { WorkflowPage } from './workflow';
const definition: WorkspaceWorkflow = {
  id: 'workspace',
  name: 'Default',
  version: 3,
  initialStageId: 'ready',
  closedStageId: 'closed',
  rules: [],
  stages: [
    {
      id: 'ready',
      name: 'Ready',
      description: 'Ready to work',
      responsibleAgentId: '',
    },
    {
      id: 'closed',
      name: 'Closed',
      description: 'Finished',
      responsibleAgentId: '',
    },
  ],
};
afterEach(() => vi.clearAllMocks());
function setup(role: 'admin' | 'viewer') {
  api.listWorkspaceWorkflows.mockResolvedValue({ data: [definition] });
  api.listAgents.mockResolvedValue({ data: [] });
  api.listWorkflowStageIntegrations.mockResolvedValue({ data: [] });
  api.listWorkflowStageIntegrationProviders.mockResolvedValue({ data: [] });
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
  expect(call.body.rules).toEqual([]);
  expect(call.body.initialStageId).toBe('ready');
  expect(call.body.closedStageId).toBe('closed');
});
it('renders workflow settings read-only for viewers', async () => {
  setup('viewer');
  await screen.findAllByDisplayValue('Ready');
  expect(screen.getAllByLabelText('Name')[0]).toBeDisabled();
  expect(screen.getAllByLabelText('Custom prompt')[0]).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Save workflow' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Add stage' })).not.toBeInTheDocument();
});

it('keeps explicit automation destinations and endpoints when columns move', async () => {
  setup('admin');
  await screen.findByText('Starts here');
  const user = userEvent.setup();
  await user.selectOptions(screen.getAllByLabelText('After successful run')[0]!, 'closed');
  await user.click(screen.getAllByRole('button', { name: 'Move earlier' })[1]!);
  await user.click(screen.getByRole('button', { name: 'Save workflow' }));
  await waitFor(() => expect(api.saveWorkspaceWorkflow).toHaveBeenCalled());
  const call = api.saveWorkspaceWorkflow.mock.calls[0]?.[0] as { body: WorkspaceWorkflow };
  expect(call.body.stages.map((s) => s.id)).toEqual(['closed', 'ready']);
  expect(call.body.initialStageId).toBe('ready');
  expect(call.body.closedStageId).toBe('closed');
  expect(call.body.rules).toHaveLength(1);
  expect(call.body.rules[0]?.id).toMatch(/^[0-9a-f-]{36}$/);
  expect(call.body.rules[0]).toMatchObject({
    enabled: true,
    event: 'run_succeeded',
    sourceStageId: 'ready',
    action: 'move_task',
    targetStageId: 'closed',
  });
});

it('saves multiline custom stage prompts and clears them', async () => {
  setup('admin');
  const user = userEvent.setup();
  const prompts = await screen.findAllByLabelText('Custom prompt');
  await user.type(prompts[0]!, 'Analyse the task{enter}Create a plan');
  await user.click(screen.getByRole('button', { name: 'Save workflow' }));
  await waitFor(() => expect(api.saveWorkspaceWorkflow).toHaveBeenCalled());
  const call = api.saveWorkspaceWorkflow.mock.calls[0]?.[0] as { body: WorkspaceWorkflow };
  expect(call.body.stages[0]?.customPrompt).toBe('Analyse the task\nCreate a plan');
  await user.clear(screen.getAllByLabelText('Custom prompt')[0]!);
  await user.type(screen.getAllByLabelText('Custom prompt')[0]!, 'Temporary');
  await user.clear(screen.getAllByLabelText('Custom prompt')[0]!);
  await user.click(screen.getByRole('button', { name: 'Save workflow' }));
  await waitFor(() => expect(api.saveWorkspaceWorkflow).toHaveBeenCalledTimes(2));
  const cleared = api.saveWorkspaceWorkflow.mock.calls[1]?.[0] as { body: WorkspaceWorkflow };
  expect(cleared.body.stages[0]?.customPrompt).toBe('');
});

it('rejects custom prompts exceeding the UTF-8 byte limit', async () => {
  setup('admin');
  const prompts = await screen.findAllByLabelText('Custom prompt');
  fireEvent.change(prompts[0]!, { target: { value: 'é'.repeat(16385) } });
  expect(screen.getByRole('alert')).toHaveTextContent('32,768 UTF-8 bytes');
  expect(screen.getByRole('button', { name: 'Save workflow' })).toBeDisabled();
});

it('configures stage artifact outputs and required inputs', async () => {
  setup('admin');
  const user = userEvent.setup();
  const artifactSections = await screen.findAllByText('Artifacts');
  const artifacts = artifactSections[0]?.closest('details');
  if (!artifacts) throw new Error('Expected artifact editor');
  await user.click(artifactSections[0]!);
  await user.click(within(artifacts).getByRole('button', { name: 'Add output' }));
  await user.click(within(artifacts).getByRole('button', { name: 'Add input' }));
  await user.type(within(artifacts).getByLabelText('Output name'), '-report');
  await user.click(within(artifacts).getByRole('checkbox'));
  await user.click(screen.getByRole('button', { name: 'Save workflow' }));
  await waitFor(() => expect(api.saveWorkspaceWorkflow).toHaveBeenCalled());
  const call = api.saveWorkspaceWorkflow.mock.calls[0]?.[0] as { body: WorkspaceWorkflow };
  expect(call.body.stages[0]?.artifactOutputs).toEqual([{ name: 'output-report', kind: 'plan' }]);
  expect(call.body.stages[0]?.artifactInputs).toEqual([
    { name: 'input', sourceStageId: '', outputName: '', required: false },
  ]);
});
