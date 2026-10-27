import type { Task, TaskRun, TaskRunEvent } from '../../shared/api/generated/types.gen';

export type TurnStatus =
  'queued' | 'running' | 'completed' | 'failed' | 'interrupted' | 'cancelled' | 'discarded';
export type ToolActivity = { id: string; title: string; status: string };
export type ChatBlock = { kind: 'text'; text: string } | { kind: 'tools'; tools: ToolActivity[] };
export type ChatTurn = {
  id: string;
  runId: string;
  commandId: string;
  text: string;
  blocks: ChatBlock[];
  status: TurnStatus;
  error?: string;
};
export type PendingPermission = {
  id: string;
  sessionId: string;
  title: string;
  options: { id: string; name: string; kind: string }[];
};

const string = (value: unknown) => (typeof value === 'string' ? value : '');
const object = (value: unknown): Record<string, unknown> =>
  value && typeof value === 'object' ? (value as Record<string, unknown>) : {};

// Stream chunks belong to a turn, not to individual chat bubbles. Tool updates
// revise the original activity in place while preserving text/tool ordering.
export function conversationTurns(events: TaskRunEvent[]): ChatTurn[] {
  const turns: ChatTurn[] = [];
  const byCommand = new Map<string, ChatTurn>();
  for (const event of events) {
    const payload = event.payload;
    const type = string(payload.type);
    const commandId = string(payload.commandId);
    const id = `${event.runId}:${commandId}`;
    if (type === 'prompt_accepted' && !byCommand.has(id)) {
      const turn: ChatTurn = {
        id,
        runId: event.runId,
        commandId,
        text: string(payload.text),
        blocks: [],
        status: 'queued',
      };
      turns.push(turn);
      byCommand.set(id, turn);
    }
    const turn = byCommand.get(id);
    if (type === 'queue_discarded') {
      for (const queued of turns)
        if (queued.runId === event.runId && queued.status === 'queued') queued.status = 'discarded';
    }
    if (!turn) continue;
    if (type === 'prompt_started' || type === 'prompt_continued') turn.status = 'running';
    if (type === 'prompt_completed')
      turn.status = payload.stopReason === 'cancelled' ? 'cancelled' : 'completed';
    if (type === 'prompt_failed') turn.status = 'failed';
    if (type === 'prompt_interrupted' || type === 'runtime_stopped') turn.status = 'interrupted';
    if (payload.error) turn.error = string(payload.error);
    if (type !== 'update') continue;
    turn.status = 'running';
    const update = object(payload.update);
    if (update.kind === 'text') {
      const text = string(update.text);
      const last = turn.blocks.at(-1);
      if (last?.kind === 'text') last.text += text;
      else if (text) turn.blocks.push({ kind: 'text', text });
    } else if (update.kind === 'tool_call' || update.kind === 'tool_call_update') {
      const toolId = string(update.toolCallId);
      const existing = turn.blocks
        .flatMap((block) => (block.kind === 'tools' ? block.tools : []))
        .find((tool) => tool.id === toolId);
      if (existing) {
        if (update.title) existing.title = string(update.title);
        if (update.status) existing.status = string(update.status);
      } else {
        const tool = {
          id: toolId || `${event.sequence}`,
          title: string(update.title) || 'Tool activity',
          status: string(update.status) || 'pending',
        };
        const last = turn.blocks.at(-1);
        if (last?.kind === 'tools') last.tools.push(tool);
        else turn.blocks.push({ kind: 'tools', tools: [tool] });
      }
    }
  }
  return turns;
}

export function pendingPermissions(events: TaskRunEvent[]): PendingPermission[] {
  const pending = new Map<string, PendingPermission>();
  for (const { payload } of events) {
    const type = string(payload.type);
    if (
      [
        'session_restored',
        'runtime_stopped',
        'prompt_interrupted',
        'prompt_failed',
        'prompt_completed',
        'run_completed',
        'run_failed',
      ].includes(type)
    )
      pending.clear();
    const id = string(payload.requestId);
    if (type === 'permission_response') pending.delete(id);
    if (type === 'permission_requested') {
      const permission = object(payload.permission);
      const options = Array.isArray(permission.options)
        ? permission.options
            .map(object)
            .filter((option) => option.id && option.name && option.kind)
            .map((option) => ({
              id: string(option.id),
              name: string(option.name),
              kind: string(option.kind),
            }))
        : [];
      pending.set(id, {
        id,
        sessionId: string(payload.sessionId),
        title: string(permission.title) || 'Agent permission',
        options,
      });
    }
  }
  return [...pending.values()];
}

export function chatExecution(task: Task, latest: TaskRun | undefined, events: TaskRunEvent[]) {
  const ended = events.some(({ payload }) =>
    ['run_completed', 'run_failed'].includes(string(payload.type)),
  );
  const paused =
    !!latest?.recoveryBlocked ||
    !!latest?.queuePaused ||
    events.reduce(
      (value, { payload }) =>
        payload.type === 'queue_paused'
          ? true
          : ['queue_resumed', 'queue_discarded', 'run_completed', 'run_failed'].includes(
                string(payload.type),
              )
            ? false
            : value,
      false,
    );
  const live = latest?.status === 'ready' && !ended;
  const finishing =
    !!latest &&
    (['completed_pending_close', 'failed_pending_close'].includes(latest.status) ||
      (latest.status === 'ready' && ended));
  const starting = !!latest && ['starting', 'process_started'].includes(latest.status);
  const canSend =
    !!task.assigneeAgentId &&
    task.status !== 'done' &&
    !!latest &&
    (live || ['completed', 'failed'].includes(latest.status)) &&
    !paused &&
    !finishing &&
    !starting;
  const label =
    task.status === 'done'
      ? 'Task done'
      : paused
        ? 'Needs attention'
        : finishing
          ? 'Finishing turn'
          : starting
            ? 'Starting agent'
            : live
              ? 'Agent connected'
              : 'Ready for follow-up';
  return { live, paused, finishing, starting, canSend, label };
}
