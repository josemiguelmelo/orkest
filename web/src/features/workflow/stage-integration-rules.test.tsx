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

Object.defineProperty(HTMLElement.prototype, 'hasPointerCapture', {
  configurable: true,
  value: () => false,
});
Object.defineProperty(HTMLElement.prototype, 'setPointerCapture', {
  configurable: true,
  value: () => undefined,
});
Object.defineProperty(HTMLElement.prototype, 'releasePointerCapture', {
  configurable: true,
  value: () => undefined,
});
Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
  configurable: true,
  value: () => undefined,
});

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
    {
      id: 'pr_merged',
      name: 'PR merged',
      parametersSchema: {
        type: 'object',
        properties: {
          artifactInputName: { type: 'string', title: 'PR input' },
          mergePolicy: {
            type: 'string',
            title: 'Merge policy',
            enum: ['all', 'any'],
            default: 'all',
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
function setup(disabled = false, initialDefinition = initial, noConnections = false) {
  api.listWorkflowStageIntegrations.mockResolvedValue({
    data: noConnections
      ? []
      : [{ id: 'connection', name: 'Review events', provider: 'example', enabled: true }],
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
  await user.click(await screen.findByRole('combobox', { name: 'Integration connection' }));
  await user.click(await screen.findByRole('option', { name: /Review events/ }));
  await user.click(screen.getByRole('combobox', { name: 'PR input' }));
  await user.click(screen.getByRole('option', { name: 'prs' }));
  expect(screen.getByLabelText('Current head')).toBeChecked();
  await user.click(screen.getByRole('button', { name: 'Review states' }));
  expect(screen.getByRole('menuitemcheckbox', { name: 'Changes requested' })).toBeInTheDocument();
  expect(screen.queryByRole('menuitemcheckbox', { name: 'plan' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('menuitemcheckbox', { name: 'Changes requested' }));
  await user.keyboard('{Escape}');
  await user.click(screen.getByRole('combobox', { name: 'Callback target stage' }));
  expect(screen.getByRole('group', { name: 'Keep task in this stage' })).toContainElement(
    screen.getByRole('option', { name: /Work/ }),
  );
  expect(screen.getByRole('group', { name: 'Move task to another stage' })).toContainElement(
    screen.getByRole('option', { name: 'Review' }),
  );
  expect(screen.getByRole('group', { name: 'Complete task' })).toContainElement(
    screen.getByRole('option', { name: /Done/ }),
  );
  await user.click(screen.getByRole('option', { name: 'Review' }));
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
      parameters: {
        artifactInputName: 'prs',
        requireCurrentHead: true,
        reviewStates: ['changes_requested'],
      },
    },
  });
  await user.click(screen.getByRole('combobox', { name: 'Workflow callback' }));
  await user.click(screen.getByRole('option', { name: /Call assigned agent/ }));
  await user.type(screen.getByLabelText('Agent callback prompt'), 'Address review feedback.');
  rules = JSON.parse(screen.getByTestId('rules').textContent ?? '[]') as WorkflowAutomationRule[];
  expect(rules[0]).toMatchObject({
    action: 'call_agent',
    promptText: 'Address review feedback.',
    sessionPolicy: 'continue_or_create_if_absent',
  });
  expect(rules[0]?.targetStageId).toBeUndefined();
  await user.click(screen.getByRole('combobox', { name: 'Workflow callback' }));
  await user.click(screen.getByRole('option', { name: /Move task to a stage/ }));
  rules = JSON.parse(screen.getByTestId('rules').textContent ?? '[]') as WorkflowAutomationRule[];
  expect(rules[0]?.promptText).toBeUndefined();
  expect(rules[0]?.sessionPolicy).toBeUndefined();
});

it('explains merge policy choices in plain language', async () => {
  const user = userEvent.setup();
  setup();
  await user.click(await screen.findByRole('button', { name: 'Add integration callback' }));
  await user.click(await screen.findByRole('combobox', { name: 'Integration connection' }));
  await user.click(await screen.findByRole('option', { name: /Review events/ }));
  await user.click(screen.getByRole('combobox', { name: 'Integration action' }));
  await user.click(screen.getByRole('option', { name: 'PR merged' }));
  await user.click(screen.getByRole('combobox', { name: 'PR input' }));
  await user.click(screen.getByRole('option', { name: 'prs' }));
  await user.click(screen.getByRole('combobox', { name: 'Merge policy' }));
  expect(screen.getByRole('option', { name: 'All linked PRs must be merged' })).toBeInTheDocument();
  await user.click(screen.getByRole('option', { name: 'Any linked PR can be merged' }));
  const rules = JSON.parse(
    screen.getByTestId('rules').textContent ?? '[]',
  ) as WorkflowAutomationRule[];
  expect(rules[0]).toMatchObject({ integration: { parameters: { mergePolicy: 'any' } } });
});
it('prevents viewers adding callbacks', async () => {
  setup(true);
  expect(await screen.findByRole('button', { name: 'Add integration callback' })).toBeDisabled();
});

it('links workflow editors to connection setup when none are available', async () => {
  setup(false, initial, true);
  const link = await screen.findByRole('link', { name: 'Open workspace settings' });
  expect(link).toHaveAttribute('href', '/organizations/o/workspaces/w/settings');
  expect(screen.getByRole('button', { name: 'Add integration callback' })).toBeDisabled();
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
  expect(await screen.findByRole('combobox', { name: 'Integration connection' })).toHaveTextContent(
    /Unavailable connection/,
  );
  expect(screen.getByRole('combobox', { name: 'Integration action' })).toHaveTextContent(
    /Unavailable action/,
  );
  expect(screen.getByLabelText('Agent callback prompt')).toHaveValue('Keep saved prompt');
  expect(JSON.parse(screen.getByTestId('rules').textContent ?? '[]')).toEqual(definition.rules);
});
