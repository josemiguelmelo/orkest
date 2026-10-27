import { describe, expect, it } from 'vitest';
import type { TaskRun, TaskRunEvent } from '../../shared/api/generated/types.gen';
import { task } from '../../test/fixtures';
import { chatExecution, conversationTurns, pendingPermissions } from './chat-model';

const run: TaskRun = {
  runId: 'run-1',
  taskId: task.id,
  taskSessionId: 'session-1',
  status: 'completed',
  lastSequence: 5,
  updatedAt: task.updatedAt,
};
function events(...payloads: Record<string, unknown>[]): TaskRunEvent[] {
  return payloads.map((payload, index) => ({
    runId: run.runId,
    sequence: index + 1,
    payload: { commandId: 'command-1', ...payload },
  }));
}

describe('task conversation', () => {
  it('keeps completed runs available for follow-up before the task status refresh', () => {
    const state = chatExecution({ ...task, status: 'in_progress' }, run, []);
    expect(state.canSend).toBe(true);
    expect(state.live).toBe(false);
    expect(state.label).toBe('Ready for follow-up');
  });
  it('keeps drafts available while cleanup finishes and prevents overlapping work', () => {
    expect(chatExecution(task, { ...run, status: 'completed_pending_close' }, []).finishing).toBe(
      true,
    );
    expect(chatExecution(task, { ...run, status: 'completed_pending_close' }, []).canSend).toBe(
      false,
    );
    expect(
      chatExecution(task, { ...run, status: 'ready' }, events({ type: 'run_completed' })).canSend,
    ).toBe(false);
    expect(chatExecution({ ...task, status: 'done' }, run, []).canSend).toBe(false);
  });
  it('groups streamed chunks into readable replies while preserving tool order', () => {
    const turns = conversationTurns(
      events(
        { type: 'prompt_accepted', text: 'Improve this' },
        { type: 'prompt_started' },
        { type: 'update', update: { kind: 'text', text: 'I will ' } },
        { type: 'update', update: { kind: 'text', text: 'inspect it.' } },
        {
          type: 'update',
          update: {
            kind: 'tool_call',
            toolCallId: 'tool-1',
            title: 'Read app.ts',
            status: 'pending',
          },
        },
        {
          type: 'update',
          update: { kind: 'tool_call_update', toolCallId: 'tool-1', status: 'completed' },
        },
        { type: 'update', update: { kind: 'text', text: 'Here is the result.' } },
        { type: 'prompt_completed' },
      ),
    );
    expect(turns).toHaveLength(1);
    expect(turns[0]?.blocks).toEqual([
      { kind: 'text', text: 'I will inspect it.' },
      { kind: 'tools', tools: [{ id: 'tool-1', title: 'Read app.ts', status: 'completed' }] },
      { kind: 'text', text: 'Here is the result.' },
    ]);
    expect(turns[0]?.status).toBe('completed');
  });
  it('keeps command identities separate across runs and shows discarded queue entries', () => {
    const first = events({ type: 'prompt_accepted', text: 'first' }, { type: 'prompt_completed' });
    const second = events(
      { type: 'prompt_accepted', text: 'second' },
      { type: 'queue_discarded' },
    ).map((event) => ({ ...event, runId: 'run-2' }));
    expect(conversationTurns([...first, ...second]).map((turn) => turn.status)).toEqual([
      'completed',
      'discarded',
    ]);
  });
  it('removes approvals when the runtime connection stops', () => {
    const history = events({
      type: 'permission_requested',
      requestId: 'approval-1',
      sessionId: 'session-1',
      permission: {
        title: 'Edit files',
        options: [{ id: 'allow', name: 'Allow once', kind: 'allow_once' }],
      },
    });
    expect(pendingPermissions(history)).toHaveLength(1);
    expect(pendingPermissions([...history, ...events({ type: 'runtime_stopped' })])).toEqual([]);
  });
});
