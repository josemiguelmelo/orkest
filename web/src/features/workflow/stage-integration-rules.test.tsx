import { useState } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppContext } from '../../app/context';
import { capabilities } from '../../app/permissions';
import type {
  WorkflowAutomationRule,
  WorkspaceWorkflow,
} from '../../shared/api/generated/types.gen';
const api = vi.hoisted(() => ({
  listWorkflowStageIntegrations: vi.fn(),
  listWorkflowStageIntegrationProviders: vi.fn(),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => api);
import { StageIntegrationRules } from './stage-integration-rules';

const initial: WorkspaceWorkflow = {
  id: 'flow',
  name: 'Flow',
  version: 1,
  initialStageId: 'work',
  closedStageId: 'done',
  rules: [],
  stages: [
    {
      id: 'produce',
      name: 'Produce',
      description: '',
      responsibleAgentId: '',
      artifactOutputs: [
        { name: 'prs', kind: 'code_review' },
        { name: 'plan', kind: 'plan' },
      ],
    },
    {
      id: 'work',
      name: 'Work',
      description: '',
      responsibleAgentId: '',
      artifactInputs: [
        { name: 'prs', sourceStageId: 'produce', outputName: 'prs', required: true },
        { name: 'plan', sourceStageId: 'produce', outputName: 'plan', required: false },
      ],
    },
    { id: 'review', name: 'Review', description: '', responsibleAgentId: '' },
    { id: 'done', name: 'Done', description: '', responsibleAgentId: '' },
  ],
};
const provider = {
  provider: 'example',
  configSchema: { type: 'object', properties: {} },
  actions: [
    {
      id: 'review_added',
      name: 'Review added',
      parametersSchema: {
        type: 'object',
        properties: {
          artifactInputName: { type: 'string', title: 'PR input' },
          requireCurrentHead: { type: 'boolean', title: 'Current head', default: true },
          reviewStates: {
            type: 'array',
            title: 'Review states',
            items: { type: 'string', enum: ['approved', 'changes_requested'] },
          },
        },
        required: ['artifactInputName'],
      },
    },
  ],
};
function Harness({
  disabled = false,
  initialDefinition = initial,
}: {
  disabled?: boolean;
  initialDefinition?: WorkspaceWorkflow;
}) {
  const [definition, setDefinition] = useState(initialDefinition);
  return (
    <>
      <StageIntegrationRules
        definition={definition}
        stageID="work"
        disabled={disabled}
        onChange={(rules) => setDefinition({ ...definition, rules })}
      />
      <output data-testid="rules">{JSON.stringify(definition.rules)}</output>
    </>
  );
}
function setup(disabled = false, initialDefinition = initial) {
  api.listWorkflowStageIntegrations.mockResolvedValue({
    data: [{ id: 'connection', name: 'Review events', provider: 'example', enabled: true }],
  });
  api.listWorkflowStageIntegrationProviders.mockResolvedValue({ data: [provider] });
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <AppContext.Provider
        value={{
          scope: { userID: 'u', orgID: 'o', workspaceID: 'w' },
          permissions: capabilities('member', disabled ? 'viewer' : 'admin', false),
          session: {} as never,
          organization: undefined,
          workspace: undefined,
        }}
      >
        <Harness disabled={disabled} initialDefinition={initialDefinition} />
      </AppContext.Provider>
    </QueryClientProvider>,
  );
}
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it('configures provider parameters and switches between movement and a custom agent prompt', async () => {
  const user = userEvent.setup();
  setup();
  await user.click(await screen.findByRole('button', { name: 'Add integration callback' }));
  await user.selectOptions(screen.getByLabelText('Integration connection'), 'connection');
  await user.selectOptions(screen.getByLabelText('PR input'), 'prs');
  expect(screen.getByLabelText('Current head')).toBeChecked();
  expect(screen.queryByRole('option', { name: 'plan' })).not.toBeInTheDocument();
  await user.selectOptions(screen.getByLabelText('Callback target stage'), 'review');
  expect(screen.queryByRole('option', { name: 'Done' })).not.toBeInTheDocument();
  let rules = JSON.parse(
    screen.getByTestId('rules').textContent ?? '[]',
  ) as WorkflowAutomationRule[];
  expect(rules[0]).toMatchObject({
    event: 'integration_action',
    sourceStageId: 'work',
    action: 'move_task',
    targetStageId: 'review',
    integration: {
      connectionId: 'connection',
      actionId: 'review_added',
      parameters: { artifactInputName: 'prs', requireCurrentHead: true },
    },
  });
  await user.selectOptions(screen.getByLabelText('Workflow callback'), 'call_agent');
  await user.type(screen.getByLabelText('Agent callback prompt'), 'Address review feedback.');
  rules = JSON.parse(screen.getByTestId('rules').textContent ?? '[]') as WorkflowAutomationRule[];
  expect(rules[0]).toMatchObject({
    action: 'call_agent',
    promptText: 'Address review feedback.',
    sessionPolicy: 'continue_or_create_if_absent',
  });
  expect(rules[0]?.targetStageId).toBeUndefined();
  await user.selectOptions(screen.getByLabelText('Workflow callback'), 'move_task');
  rules = JSON.parse(screen.getByTestId('rules').textContent ?? '[]') as WorkflowAutomationRule[];
  expect(rules[0]?.promptText).toBeUndefined();
  expect(rules[0]?.sessionPolicy).toBeUndefined();
});
it('prevents viewers adding callbacks', async () => {
  setup(true);
  expect(await screen.findByRole('button', { name: 'Add integration callback' })).toBeDisabled();
});

it('keeps unavailable connection and action identities visible until explicitly changed', async () => {
  const definition: WorkspaceWorkflow = {
    ...initial,
    rules: [
      {
        id: 'stored',
        enabled: false,
        event: 'integration_action',
        sourceStageId: 'work',
        action: 'call_agent',
        promptText: 'Keep saved prompt',
        sessionPolicy: 'continue_or_create_if_absent',
        integration: {
          connectionId: 'unavailable',
          actionId: 'unknown_action',
          parameters: { artifactInputName: 'prs' },
        },
      },
    ],
  };
  setup(false, definition);
  expect(
    await screen.findByRole('option', { name: 'Unavailable connection (unavailable)' }),
  ).toHaveProperty('selected', true);
  expect(
    screen.getByRole('option', { name: 'Unavailable action (unknown_action)' }),
  ).toHaveProperty('selected', true);
  expect(screen.getByLabelText('Agent callback prompt')).toHaveValue('Keep saved prompt');
  expect(JSON.parse(screen.getByTestId('rules').textContent ?? '[]')).toEqual(definition.rules);
});
