import { beforeEach, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppContext } from '../../app/context';
import { capabilities } from '../../app/permissions';
import { session, organization, workspace, task } from '../../test/fixtures';
import * as sdk from '../../shared/api/generated/sdk.gen';
import { TaskChat } from './task-chat';

vi.mock('../../shared/api/generated/sdk.gen', () => ({
  listTaskRuns: vi.fn(),
  listTaskRunEvents: vi.fn(),
  updateTask: vi.fn(),
  sendTaskRunPrompt: vi.fn(),
}));
const run = {
  runId: 'run-1',
  taskSessionId: 'session-1',
  taskId: task.id,
  status: 'completed',
  lastSequence: 4,
  updatedAt: task.updatedAt,
};
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(sdk.listTaskRuns).mockResolvedValue({ data: [run] } as never);
  vi.mocked(sdk.listTaskRunEvents).mockResolvedValue({
    data: [
      {
        runId: run.runId,
        sequence: 1,
        payload: { type: 'prompt_accepted', commandId: 'previous', text: 'Make a change' },
      },
      {
        runId: run.runId,
        sequence: 2,
        payload: {
          type: 'update',
          commandId: 'previous',
          update: { kind: 'text', text: 'The change is ' },
        },
      },
      {
        runId: run.runId,
        sequence: 3,
        payload: {
          type: 'update',
          commandId: 'previous',
          update: { kind: 'text', text: 'ready.' },
        },
      },
      {
        runId: run.runId,
        sequence: 4,
        payload: { type: 'prompt_completed', commandId: 'previous' },
      },
    ],
  } as never);
  vi.mocked(sdk.updateTask).mockResolvedValue({
    data: { ...task, status: 'in_progress' },
  } as never);
  vi.mocked(sdk.sendTaskRunPrompt).mockResolvedValue({ data: {} } as never);
});
function setup(status: typeof task.status = 'in_progress', viewer = false) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const context = {
    session,
    organization,
    workspace,
    scope: { userID: session.user.id, orgID: organization.id, workspaceID: workspace.id },
    permissions: capabilities(viewer ? 'member' : 'owner', viewer ? 'viewer' : 'admin', false),
  };
  const view = render(
    <QueryClientProvider client={client}>
      <AppContext.Provider value={context}>
        <TaskChat task={{ ...task, status }} />
      </AppContext.Provider>
    </QueryClientProvider>,
  );
  return { ...view, client };
}
it('keeps the composer available after completion and sends through a new run in the same session', async () => {
  const user = userEvent.setup();
  setup();
  await screen.findByText('The change is ready.');
  const composer = screen.getByRole('textbox', { name: 'Message the agent' });
  expect(composer).toBeEnabled();
  await user.type(composer, 'Now add a test');
  await user.click(screen.getByRole('button', { name: 'Send message' }));
  await waitFor(() =>
    expect(vi.mocked(sdk.updateTask).mock.calls[0]?.[0]?.body).toMatchObject({
      startWork: true,
      sessionMode: 'continue',
      status: 'in_progress',
      promptText: 'Now add a test',
    }),
  );
  expect(sdk.sendTaskRunPrompt).not.toHaveBeenCalled();
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
});
it('sends to a live run and supports Shift+Enter without submitting', async () => {
  vi.mocked(sdk.listTaskRuns).mockResolvedValue({ data: [{ ...run, status: 'ready' }] } as never);
  const user = userEvent.setup();
  setup();
  await screen.findByText('The change is ready.');
  const composer = screen.getByRole('textbox', { name: 'Message the agent' });
  await user.type(composer, 'Another request');
  await user.keyboard('{Shift>}{Enter}{/Shift}');
  expect(sdk.sendTaskRunPrompt).not.toHaveBeenCalled();
  await user.keyboard('{Enter}');
  await waitFor(() =>
    expect(vi.mocked(sdk.sendTaskRunPrompt).mock.calls[0]?.[0]).toMatchObject({
      path: { runID: 'run-1' },
      body: { text: 'Another request' },
    }),
  );
  expect(sdk.updateTask).not.toHaveBeenCalled();
});
it('offers an explicit fresh session while keeping previous chat visible', async () => {
  const user = userEvent.setup();
  setup('in_review');
  await screen.findByText('The change is ready.');
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Session for next message' }),
    'new',
  );
  await user.type(screen.getByRole('textbox', { name: 'Message the agent' }), 'Start fresh');
  await user.click(screen.getByRole('button', { name: 'Send message' }));
  await waitFor(() =>
    expect(vi.mocked(sdk.updateTask).mock.calls[0]?.[0]?.body).toMatchObject({
      sessionMode: 'new',
    }),
  );
});
it('keeps completed task and viewer conversations read-only', async () => {
  const { unmount } = setup('done');
  await screen.findByText('The change is ready.');
  expect(screen.getByRole('textbox', { name: 'Message the agent' })).toBeDisabled();
  unmount();
  setup('in_review', true);
  await screen.findByText('The change is ready.');
  expect(screen.getByRole('textbox', { name: 'Message the agent' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled();
});

it('keeps the composer editable while a completed run is cleaning up', async () => {
  vi.mocked(sdk.listTaskRuns).mockResolvedValue({
    data: [{ ...run, status: 'completed_pending_close' }],
  } as never);
  const user = userEvent.setup();
  setup();
  await screen.findByText('The change is ready.');
  const composer = screen.getByRole('textbox', { name: 'Message the agent' });
  await user.type(composer, 'Keep this draft');
  expect(composer).toHaveValue('Keep this draft');
  expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled();
  expect(screen.getByText('Finishing turn', { exact: true })).toBeInTheDocument();
});

it('preserves the draft when sending fails and allows correcting it', async () => {
  vi.mocked(sdk.updateTask).mockRejectedValue(new Error('Could not send'));
  const user = userEvent.setup();
  setup();
  await screen.findByText('The change is ready.');
  const composer = screen.getByRole('textbox', { name: 'Message the agent' });
  await user.type(composer, 'Keep this message');
  await user.click(screen.getByRole('button', { name: 'Send message' }));
  await screen.findByText('Could not send');
  expect(composer).toBeEnabled();
  expect(composer).toHaveValue('Keep this message');
  await user.type(composer, ' and this correction');
  expect(composer).toHaveValue('Keep this message and this correction');
});

it.each(['pending', 'answered'] as const)(
  'keeps a %s question between the messages surrounding its request',
  async (status) => {
    const payloads = [
      { type: 'prompt_accepted', text: 'Plan this work' },
      { type: 'update', update: { kind: 'text', text: 'Here is my plan.' } },
      {
        type: 'question_requested', requestId: 'question-1', sessionId: 'native-session',
        question: {
          message: 'Approve this plan?',
          schema: { type: 'object', properties: { decision: { type: 'string' } } },
        },
      },
      ...(status === 'answered' ? [{
        type: 'question_answered', requestId: 'question-1',
        questionResponse: { answer: { action: 'accept', content: { decision: 'Proceed' } } },
      }] : []),
      { type: 'update', update: { kind: 'text', text: 'The next agent message.' } },
      ...(status === 'answered' ? [{ type: 'prompt_completed' }] : []),
      { type: 'prompt_accepted', commandId: 'next', text: 'A later user message' },
    ];
    vi.mocked(sdk.listTaskRuns).mockResolvedValue({ data: [{ ...run, status: 'ready' }] } as never);
    vi.mocked(sdk.listTaskRunEvents).mockResolvedValue({
      data: payloads.map((payload, index) => ({
        runId: run.runId, sequence: index + 1,
        payload: { commandId: 'previous', ...payload },
      })),
    } as never);
    setup();
    const question = await screen.findByRole('region', { name: 'Agent question' });
    const before = screen.getByText('Here is my plan.');
    const after = screen.getByText('The next agent message.');
    const later = screen.getByText('A later user message');
    expect(before.compareDocumentPosition(question) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(question.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(question.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getAllByRole('region', { name: 'Agent question' })).toHaveLength(1);
    if (status === 'answered') expect(question).toHaveTextContent('Proceed');
    else expect(screen.getByRole('textbox', { name: 'decision' })).toBeEnabled();
  },
);
