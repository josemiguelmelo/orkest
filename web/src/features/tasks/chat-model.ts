import type { Task, TaskRun, TaskRunEvent } from '../../shared/api/generated/types.gen';

export type TurnStatus =
  'queued' | 'running' | 'completed' | 'failed' | 'interrupted' | 'cancelled' | 'discarded';
export type ToolContent = {
  kind: string;
  text?: string;
  path?: string;
  oldText?: string;
  newText?: string;
  truncated?: boolean;
};
export type ToolActivity = {
  id: string;
  title: string;
  status: string;
  toolKind?: string;
  input?: string;
  output?: string;
  truncated?: boolean;
  locations?: { path: string; line?: number }[];
  content?: ToolContent[];
  startedAt?: number;
  endedAt?: number;
};
// Prefer runtime locations, falling back to common tool input path fields.
export function toolLabel(tool: ToolActivity): string {
  let file = tool.locations?.[0]?.path;
  if (!file && tool.input) {
    try {
      const input: unknown = JSON.parse(tool.input);
      if (input && typeof input === 'object') {
        const fields = input as Record<string, unknown>;
        for (const key of ['filePath', 'file_path', 'path'])
          if (typeof fields[key] === 'string') {
            file = fields[key];
            break;
          }
      }
    } catch {
      /* Truncated input may not be valid JSON. */
    }
  }
  return file && !tool.title.includes(file) ? `${tool.title} · ${file}` : tool.title;
}

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
      let tool: ToolActivity;
      if (existing) {
        tool = existing;
        if (update.title) existing.title = string(update.title);
        if (update.status) existing.status = string(update.status);
      } else {
        tool = {
          id: toolId || `${event.sequence}`,
          title: string(update.title) || 'Tool activity',
          status: string(update.status) || 'pending',
        };
        const last = turn.blocks.at(-1);
        if (last?.kind === 'tools') last.tools.push(tool);
        else turn.blocks.push({ kind: 'tools', tools: [tool] });
      }
      for (const key of ['toolKind', 'input', 'output'] as const)
        if (typeof update[key] === 'string') tool[key] = update[key];
      if (typeof update.truncated === 'boolean') tool.truncated = update.truncated;
      if (Array.isArray(update.locations))
        tool.locations = update.locations
          .map(object)
          .filter((l) => typeof l.path === 'string')
          .map((l) => ({
            path: string(l.path),
            ...(typeof l.line === 'number' ? { line: l.line } : {}),
          }));
      if (Array.isArray(update.content))
        tool.content = update.content
          .map(object)
          .filter((c) => typeof c.kind === 'string')
          .map((c) => ({
            kind: string(c.kind),
            text: string(c.text),
            path: string(c.path),
            oldText: string(c.oldText),
            newText: string(c.newText),
            truncated: c.truncated === true,
          }));
      const at = Date.parse(string(payload.at));
      if (Number.isFinite(at)) {
        tool.startedAt ??= at;
        if (['completed', 'failed'].includes(tool.status)) tool.endedAt ??= at;
      }
    }
  }
  for (const turn of turns)
    if (turn.status !== 'running' && turn.status !== 'queued')
      for (const block of turn.blocks)
        if (block.kind === 'tools')
          for (const tool of block.tools)
            if (!['completed', 'failed'].includes(tool.status))
              tool.status = turn.status === 'interrupted' ? 'interrupted' : 'outcome_unknown';
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
    task.stageWorkState !== 'queued' &&
    !!task.assigneeAgentId &&
    !(task.closed ?? task.status === 'done') &&
    !!latest &&
    (live || ['completed', 'failed'].includes(latest.status)) &&
    !paused &&
    !finishing &&
    !starting;
  const label =
    (task.closed ?? task.status === 'done')
      ? 'Task done'
      : task.stageWorkState === 'queued'
        ? 'Queued to start'
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
