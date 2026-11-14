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
for (const method of [
  'hasPointerCapture',
  'setPointerCapture',
  'releasePointerCapture',
  'scrollIntoView',
]) {
  Object.defineProperty(HTMLElement.prototype, method, { configurable: true, value: () => false });
}
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
function setup(role: 'admin' | 'viewer', workflow = definition) {
  api.listWorkspaceWorkflows.mockResolvedValue({ data: [workflow] });
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
  expect(screen.queryByLabelText('New tasks start in')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Closed source tasks go to')).not.toBeInTheDocument();
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
  expect(call.body.initialStageId).toBe(call.body.stages[0]?.id);
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

it('removes outgoing automation from the new final stage while preserving incoming rules', async () => {
  const incoming = {
    id: 'incoming',
    enabled: true,
    event: 'run_succeeded' as const,
    sourceStageId: 'ready',
    action: 'move_task' as const,
    targetStageId: 'working',
  };
  setup('admin', {
    ...definition,
    stages: [
      definition.stages[0]!,
      { id: 'working', name: 'Working', description: '', responsibleAgentId: '' },
      definition.stages[1]!,
    ],
    rules: [
      incoming,
      {
        id: 'outgoing',
        enabled: true,
        event: 'run_succeeded',
        sourceStageId: 'working',
        action: 'move_task',
        targetStageId: 'closed',
      },
      {
        id: 'disabled',
        enabled: false,
        event: 'stage_entered',
        sourceStageId: 'working',
        action: 'start_work',
      },
    ],
  });
  await screen.findByText('Starts here');
  const user = userEvent.setup();
  await user.click(screen.getAllByRole('button', { name: 'Move earlier' })[2]!);
  await user.click(screen.getByRole('button', { name: 'Save workflow' }));
  await waitFor(() => expect(api.saveWorkspaceWorkflow).toHaveBeenCalled());
  const call = api.saveWorkspaceWorkflow.mock.calls[0]?.[0] as { body: WorkspaceWorkflow };
  expect(call.body.stages.map((s) => s.id)).toEqual(['ready', 'closed', 'working']);
  expect(call.body.initialStageId).toBe('ready');
  expect(call.body.closedStageId).toBe('working');
  expect(call.body.rules).toEqual([incoming]);
});

it('cleans outgoing rules when loading a workflow with a different configured final stage', async () => {
  setup('admin', {
    ...definition,
    stages: [...definition.stages].reverse(),
    rules: [
      {
        id: 'outgoing',
        enabled: true,
        event: 'stage_entered',
        sourceStageId: 'ready',
        action: 'start_work',
      },
    ],
  });
  await screen.findByText('Final stage');
  const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'Save workflow' }));
  await waitFor(() => expect(api.saveWorkspaceWorkflow).toHaveBeenCalled());
  const call = api.saveWorkspaceWorkflow.mock.calls[0]?.[0] as { body: WorkspaceWorkflow };
  expect(call.body.initialStageId).toBe('closed');
  expect(call.body.closedStageId).toBe('ready');
  expect(call.body.rules).toEqual([]);
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
  api.saveWorkspaceWorkflow.mockImplementation(({ body }: { body: WorkspaceWorkflow }) =>
    Promise.resolve({ data: { ...body, version: body.version + 1 } }),
  );
  const user = userEvent.setup();
  const artifactSections = await screen.findAllByText('Artifacts');
  const sourceArtifacts = artifactSections[0]?.closest('details');
  const targetArtifacts = artifactSections[1]?.closest('details');
  if (!sourceArtifacts || !targetArtifacts) throw new Error('Expected stage artifact editors');
  await user.click(artifactSections[0]!);
  await user.click(within(sourceArtifacts).getByRole('button', { name: 'Add output' }));
  await user.type(within(sourceArtifacts).getByLabelText('Output name'), '-report');
  await user.click(artifactSections[1]!);
  await user.click(within(targetArtifacts).getByRole('button', { name: 'Add input' }));
  await user.click(within(targetArtifacts).getByLabelText('Source output'));
  await user.click(screen.getByRole('option', { name: 'Ready / output-report' }));
  expect(within(targetArtifacts).getByRole('checkbox')).toBeChecked();
  await user.click(screen.getByRole('button', { name: 'Save workflow' }));
  await waitFor(() => expect(api.saveWorkspaceWorkflow).toHaveBeenCalled());
  const call = api.saveWorkspaceWorkflow.mock.calls[0]?.[0] as { body: WorkspaceWorkflow };
  expect(call.body.stages[0]?.artifactOutputs).toEqual([{ name: 'output-report', kind: 'plan' }]);
  expect(call.body.stages[1]?.artifactInputs).toEqual([
    { name: 'input', sourceStageId: 'ready', outputName: 'output-report', required: true },
  ]);
  await user.click(within(targetArtifacts).getByRole('checkbox'));
  await user.click(screen.getByRole('button', { name: 'Save workflow' }));
  await waitFor(() => expect(api.saveWorkspaceWorkflow).toHaveBeenCalledTimes(2));
  const optional = api.saveWorkspaceWorkflow.mock.calls[1]?.[0] as { body: WorkspaceWorkflow };
  expect(optional.body.stages[1]?.artifactInputs).toEqual([
    { name: 'input', sourceStageId: 'ready', outputName: 'output-report', required: false },
  ]);
});
