import { Markdown } from '../../shared/ui/markdown';
import type { FormEvent, JSX } from 'react';
import { useState } from 'react';
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { ArrowLeft, Bot, CircleStop, LayoutGrid, List, Search, Send } from 'lucide-react';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { Task } from '../../shared/api/generated/types.gen';
import {
  agentsQuery,
  commentsQuery,
  componentsQuery,
  path,
  taskQuery,
  taskRunEventsQuery,
  taskRunsQuery,
  tasksQuery,
  useRefresh,
} from '../../shared/api/queries';
import {
  Badge,
  CreateButton,
  date,
  Empty,
  ErrorState,
  Loading,
  PageHeader,
} from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Field, Form, Selection } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';
import { filterTasks, statuses, statusNames, taskFormSchema, taskInput } from './model';

export function TasksPage(): JSX.Element {
  const { scope, permissions } = useApp();
  const tasks = useQuery(tasksQuery(scope));
  const agents = useQuery(agentsQuery(scope));
  const search = useSearch({ strict: false });
  const navigate = useNavigate();
  const [create, setCreate] = useState(false);
  const state = {
    view: search.view ?? 'board',
    q: search.q ?? '',
    status: search.status ?? 'all',
    agent: search.agent ?? '',
    sort: search.sort ?? 'updated',
  };
  const params = { orgId: scope.orgID, workspaceId: scope.workspaceID };
  function update(values: Partial<typeof state>) {
    void navigate({
      to: '/organizations/$orgId/workspaces/$workspaceId/tasks',
      params,
      search: { ...state, ...values },
      replace: true,
    });
  }
  const visible = filterTasks(tasks.data ?? [], state);
  return (
    <>
      <PageHeader
        title="Tasks"
        description="A clear view of the work ahead, in progress, and ready for review."
      >
        {permissions.editWorkspace && (
          <CreateButton onClick={() => setCreate(true)}>New task</CreateButton>
        )}
      </PageHeader>
      <div className="toolbar">
        <Search size={16} className="muted" />
        <input
          aria-label="Search tasks"
          placeholder="Search tasks…"
          value={state.q}
          onChange={(e) => update({ q: e.target.value })}
        />
        <select
          aria-label="Filter status"
          value={state.status}
          onChange={(e) => {
            const status = statuses.find((s) => s === e.target.value);
            update({ status: status ?? 'all' });
          }}
        >
          <option value="all">All statuses</option>
          {statuses.map((s) => (
            <option key={s} value={s}>
              {statusNames[s]}
            </option>
          ))}
        </select>
        <select
          aria-label="Filter agent"
          value={state.agent}
          onChange={(e) => update({ agent: e.target.value })}
        >
          <option value="">All agents</option>
          {agents.data?.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
        <select
          aria-label="Sort tasks"
          value={state.sort}
          onChange={(e) => update({ sort: e.target.value === 'title' ? 'title' : 'updated' })}
        >
          <option value="updated">Recently updated</option>
          <option value="title">Title</option>
        </select>
        <div className="view-toggle">
          <Button
            variant={state.view === 'board' ? 'outline' : 'ghost'}
            aria-label="Board view"
            aria-pressed={state.view === 'board'}
            onClick={() => update({ view: 'board' })}
          >
            <LayoutGrid size={16} />
          </Button>
          <Button
            variant={state.view === 'list' ? 'outline' : 'ghost'}
            aria-label="List view"
            aria-pressed={state.view === 'list'}
            onClick={() => update({ view: 'list' })}
          >
            <List size={16} />
          </Button>
        </div>
      </div>
      {tasks.isPending ? (
        <Loading />
      ) : tasks.isError ? (
        <ErrorState
          error={tasks.error}
          retry={() => {
            void tasks.refetch();
          }}
        />
      ) : !tasks.data.length ? (
        <Empty title="No tasks yet">Create a task or connect an integration to import work.</Empty>
      ) : !visible.length ? (
        <Empty title="No matching tasks">Try another search or filter.</Empty>
      ) : state.view === 'board' ? (
        <div className="board">
          {statuses.map((status) => (
            <section className="board-column" key={status}>
              <h2 className="column-heading">
                <span className={`status-dot status-${status}`} />
                {statusNames[status]}
                <Badge>{visible.filter((t) => t.status === status).length}</Badge>
              </h2>
              {visible
                .filter((t) => t.status === status)
                .map((task) => (
                  <Link
                    key={task.id}
                    className="task-card"
                    to="/organizations/$orgId/workspaces/$workspaceId/tasks/$taskId"
                    params={{ ...params, taskId: task.id }}
                  >
                    <div className="task-meta">
                      <span>{task.id.slice(0, 8)}</span>
                      <span>{task.sourceProvider === 'manual' ? 'Manual' : 'Imported'}</span>
                    </div>
                    <h3>{task.title}</h3>
                    <div className="tags">
                      {task.labels.map((label) => (
                        <Badge key={label}>{label}</Badge>
                      ))}
                    </div>
                    <div className="task-agent">
                      <Bot size={12} style={{ display: 'inline', marginRight: 5 }} />
                      {agents.data?.find((a) => a.id === task.assigneeAgentId)?.name ??
                        'Unassigned'}
                    </div>
                  </Link>
                ))}
            </section>
          ))}
        </div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Task</th>
                <th>Status</th>
                <th>Agent</th>
                <th>Labels</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((task) => (
                <tr key={task.id}>
                  <td>
                    <Link
                      to="/organizations/$orgId/workspaces/$workspaceId/tasks/$taskId"
                      params={{ ...params, taskId: task.id }}
                    >
                      {task.title}
                    </Link>
                  </td>
                  <td>
                    <Badge>{statusNames[task.status]}</Badge>
                  </td>
                  <td>
                    {agents.data?.find((a) => a.id === task.assigneeAgentId)?.name ?? 'Unassigned'}
                  </td>
                  <td>
                    <div className="tags">
                      {task.labels.map((label) => (
                        <Badge key={label}>{label}</Badge>
                      ))}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {create && <TaskEditor onClose={() => setCreate(false)} />}
    </>
  );
}
export function TaskEditor({ task, onClose }: { task?: Task; onClose: () => void }) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const agents = useQuery(agentsQuery(scope));
  const components = useQuery(componentsQuery(scope));
  const form = useForm({
    resolver: zodResolver(taskFormSchema),
    defaultValues: {
      title: task?.title ?? '',
      description: task?.description ?? '',
      status: task?.status ?? 'todo',
      assigneeAgentId: task?.assigneeAgentId ?? '',
      labels: task?.labels.join(', ') ?? '',
      componentIds: task?.components.map((c) => c.id) ?? [],
    },
  });
  const imported = !!task && task.sourceProvider !== 'manual';
  return (
    <Dialog
      title={task ? 'Edit task' : 'New task'}
      description={
        imported
          ? 'Imported content is managed at its source. Update assignment, components, or local workflow here.'
          : 'Organize the work and assign an agent definition. Assignment does not start execution.'
      }
      open
      onOpenChange={(open) => {
        if (!open && !action.pending) onClose();
      }}
    >
      <Form
        form={form}
        error={action.error}
        onSubmit={async (values) => {
          if (
            await action.run(async () => {
              const body = taskInput(values, task);
              if (task)
                await data(
                  sdk.updateTask({ ...command, path: { ...path(scope), taskID: task.id }, body }),
                );
              else await data(sdk.createTask({ ...command, path: path(scope), body }));
              await refresh(scope);
            })
          )
            onClose();
        }}
      >
        <Field label="Title" error={form.formState.errors.title?.message}>
          <input {...form.register('title')} readOnly={imported} />
        </Field>
        <Field label="Description" error={form.formState.errors.description?.message}>
          <textarea rows={6} {...form.register('description')} readOnly={imported} />
        </Field>
        <div className="form-grid">
          <Field label="Status" hint="Execution starts are unavailable in this UI.">
            <select {...form.register('status')} disabled={!task || task.status === 'done'}>
              {statuses.map((status) => (
                <option key={status} value={status}>
                  {statusNames[status]}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Assigned agent">
            <select
              {...form.register('assigneeAgentId')}
              disabled={agents.isPending || agents.isError}
            >
              <option value="">Unassigned</option>
              {agents.data?.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </Field>
        </div>
        {agents.isError && <ErrorState error={agents.error} />}
        <Field label="Labels" hint="Separate labels with commas.">
          <input {...form.register('labels')} readOnly={imported} />
        </Field>
        {components.isError ? (
          <ErrorState error={components.error} />
        ) : (
          <Selection
            label="Components"
            options={components.data ?? []}
            value={form.watch('componentIds')}
            onChange={(ids) => form.setValue('componentIds', ids, { shouldDirty: true })}
          />
        )}
      </Form>
    </Dialog>
  );
}
export function TaskPage(): JSX.Element {
  const { taskId } = useParams({
    from: '/organizations/$orgId/workspaces/$workspaceId/tasks/$taskId',
  });
  const { scope, permissions } = useApp();
  const task = useQuery(taskQuery(scope, taskId));
  const agents = useQuery(agentsQuery(scope));
  const components = useQuery(componentsQuery(scope));
  const [edit, setEdit] = useState(false);
  const startAction = useAction();
  const refresh = useRefresh();
  if (task.isPending) return <Loading />;
  if (task.isError) return <ErrorState error={task.error} />;
  const item = task.data;
  return (
    <>
      <Link
        className="text-link"
        to="/organizations/$orgId/workspaces/$workspaceId/tasks"
        params={{ orgId: scope.orgID, workspaceId: scope.workspaceID }}
        search={{ view: 'board', q: '', status: 'all', agent: '', sort: 'updated' }}
      >
        <ArrowLeft size={13} style={{ display: 'inline' }} /> All tasks
      </Link>
      <div className="spacer">
        <PageHeader title={item.title} description={`Task ${item.id.slice(0, 8)}`}>
          {permissions.editWorkspace && item.status === 'todo' && item.assigneeAgentId && (
            <Button
              disabled={startAction.pending}
              onClick={() => {
                void startAction.run(async () => {
                  try {
                    await data(
                      sdk.updateTask({
                        ...command,
                        path: { ...path(scope), taskID: item.id },
                        body: {
                          title: item.title,
                          description: item.description,
                          status: 'in_progress',
                          assigneeAgentId: item.assigneeAgentId ?? '',
                          labels: item.labels,
                          componentIds: item.components.map((component) => component.id),
                        },
                      }),
                    );
                  } finally {
                    await refresh(scope);
                  }
                });
              }}
            >
              Start work
            </Button>
          )}
          {permissions.editWorkspace && <Button onClick={() => setEdit(true)}>Edit task</Button>}
        </PageHeader>
      </div>
      {startAction.error && <ErrorState error={startAction.error} />}
      {item.status === 'in_progress' && (
        <div className="notice">
          This task is in progress. Review its run activity and interact with the agent below.
        </div>
      )}
      <div className="detail-layout">
        <div className="task-conversation-column">
          <div className="task-context">
            <details className="task-context-section">
              <summary>
                <span>Description</span>
                <span className="muted">Task context</span>
              </summary>
              <div className="task-context-content">
                {item.description ? (
                  <Markdown text={item.description} />
                ) : (
                  <p className="muted">No description provided.</p>
                )}
              </div>
            </details>
            <details className="task-context-section">
              <summary>
                <span>Source comments</span>
                <span className="muted">Read only</span>
              </summary>
              <div className="task-context-content">
                <Comments taskID={item.id} />
              </div>
            </details>
          </div>
          <RunPanel task={item} />
        </div>
        <aside>
          <section className="panel">
            <dl className="detail-meta">
              {permissions.editWorkspace ? (
                <TaskPropertyControls
                  task={item}
                  agents={agents.data ?? []}
                  components={components.data ?? []}
                  agentsLoading={agents.isPending || agents.isError}
                  componentsLoading={components.isPending || components.isError}
                />
              ) : (
                <>
                  <div>
                    <dt>Status</dt>
                    <dd>
                      <Badge>{statusNames[item.status]}</Badge>
                    </dd>
                  </div>
                  <div>
                    <dt>Assigned agent</dt>
                    <dd>
                      {agents.data?.find((a) => a.id === item.assigneeAgentId)?.name ??
                        'Unassigned'}
                    </dd>
                  </div>
                  <div>
                    <dt>Components</dt>
                    <dd>{item.components.map((c) => c.name).join(', ') || 'None'}</dd>
                  </div>
                </>
              )}
              <div>
                <dt>Labels</dt>
                <dd className="tags">
                  {item.labels.length ? item.labels.map((l) => <Badge key={l}>{l}</Badge>) : 'None'}
                </dd>
              </div>
              <div>
                <dt>Source</dt>
                <dd>
                  {item.sourceUrl && /^https?:\/\//i.test(item.sourceUrl) ? (
                    <a className="text-link" href={item.sourceUrl} target="_blank" rel="noreferrer">
                      {item.sourceProvider} ↗
                    </a>
                  ) : (
                    item.sourceProvider
                  )}
                </dd>
              </div>
              <div>
                <dt>Updated</dt>
                <dd>{date(item.updatedAt)}</dd>
              </div>
              <div>
                <dt>Created</dt>
                <dd>{date(item.createdAt)}</dd>
              </div>
            </dl>
          </section>
        </aside>
      </div>
      {edit && <TaskEditor task={item} onClose={() => setEdit(false)} />}
    </>
  );
}

function TaskPropertyControls({
  task,
  agents,
  components,
  agentsLoading,
  componentsLoading,
}: {
  task: Task;
  agents: { id: string; name: string }[];
  components: { id: string; name: string }[];
  agentsLoading: boolean;
  componentsLoading: boolean;
}): JSX.Element {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  async function update(patch: {
    status?: Task['status'];
    assigneeAgentId?: string;
    componentIds?: string[];
  }) {
    await action.run(async () => {
      try {
        await data(
          sdk.updateTask({
            ...command,
            path: { ...path(scope), taskID: task.id },
            body: {
              title: task.title,
              description: task.description,
              status: patch.status ?? task.status,
              assigneeAgentId: patch.assigneeAgentId ?? task.assigneeAgentId ?? '',
              labels: task.labels,
              componentIds: patch.componentIds ?? task.components.map((component) => component.id),
            },
          }),
        );
      } finally {
        await refresh(scope);
      }
    });
  }
  return (
    <>
      <div>
        <dt>Status</dt>
        <dd>
          <select
            aria-label="Task status"
            value={task.status}
            disabled={action.pending || task.status === 'done'}
            onChange={(event) => void update({ status: event.target.value as Task['status'] })}
          >
            {statuses.map((status) => (
              <option key={status} value={status}>
                {statusNames[status]}
              </option>
            ))}
          </select>
        </dd>
      </div>
      <div>
        <dt>Assigned agent</dt>
        <dd>
          <select
            aria-label="Assigned agent"
            value={task.assigneeAgentId ?? ''}
            disabled={action.pending || agentsLoading}
            onChange={(event) => void update({ assigneeAgentId: event.target.value })}
          >
            <option value="">Unassigned</option>
            {agents.map((agent) => (
              <option key={agent.id} value={agent.id}>
                {agent.name}
              </option>
            ))}
          </select>
        </dd>
      </div>
      <div>
        <dt>Components</dt>
        <dd>
          {componentsLoading ? (
            <span className="muted">Loading components…</span>
          ) : (
            <div className="component-chips" aria-label="Components">
              {components.length ? (
                components.map((component) => {
                  const selected = task.components.some((item) => item.id === component.id);
                  return (
                    <button
                      className={`component-chip${selected ? ' is-selected' : ''}`}
                      type="button"
                      key={component.id}
                      aria-pressed={selected}
                      disabled={action.pending}
                      onClick={() =>
                        void update({
                          componentIds: selected
                            ? task.components
                                .filter((item) => item.id !== component.id)
                                .map((item) => item.id)
                            : [...task.components.map((item) => item.id), component.id],
                        })
                      }
                    >
                      {component.name}
                    </button>
                  );
                })
              ) : (
                <span className="muted">No components available.</span>
              )}
            </div>
          )}
          {action.error ? <ErrorState error={action.error} /> : null}
        </dd>
      </div>
    </>
  );
}

type RuntimeEvent = {
  type?: string | undefined;
  at?: string | undefined;
  commandId?: string | undefined;
  requestId?: string | undefined;
  sessionId?: string | undefined;
  text?: string | undefined;
  error?: string | undefined;
  stopReason?: string | undefined;
  update?:
    | {
        kind?: string | undefined;
        text?: string | undefined;
        title?: string | undefined;
        status?: string | undefined;
      }
    | undefined;
  permission?:
    | {
        title?: string | undefined;
        options?: { id: string; name: string; kind: string }[] | undefined;
      }
    | undefined;
};

function RunPanel({ task }: { task: Task }): JSX.Element {
  const { scope, permissions } = useApp();
  const runs = useQuery(taskRunsQuery(scope, task.id));
  const [selectedRunID, setSelectedRunID] = useState<string>();
  const selected = runs.data?.find((run) => run.runId === selectedRunID) ?? runs.data?.[0];
  const eventsQuery = useQuery(taskRunEventsQuery(scope, task.id, selected?.runId ?? ''));
  const action = useAction();
  const refresh = useRefresh();
  const [text, setText] = useState('');
  const [pendingCommandID, setPendingCommandID] = useState<string | null>(null);
  const events = eventsQuery.data ?? [];
  const payloads = events.map((event) => ({ event, payload: runtimeEvent(event.payload) }));
  const pending = new Map<string, RuntimeEvent>();
  const commands = new Map<string, { accepted: RuntimeEvent; terminal: boolean }>();
  for (const { payload } of payloads) {
    if (payload.type === 'permission_requested' && payload.requestId)
      pending.set(payload.requestId, payload);
    if (payload.type === 'permission_response' && payload.requestId)
      pending.delete(payload.requestId);
    if (payload.commandId && payload.type === 'prompt_accepted')
      commands.set(payload.commandId, { accepted: payload, terminal: false });
    if (payload.commandId && ['prompt_completed', 'prompt_failed'].includes(payload.type ?? '')) {
      const command = commands.get(payload.commandId);
      if (command) command.terminal = true;
    }
  }
  const activeCommand = [...commands.entries()]
    .reverse()
    .find(([, command]) => !command.terminal)?.[0];
  async function submitPrompt(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const prompt = text.trim();
    if (!selected || !prompt) return;
    if (
      await action.run(async () => {
        const commandId = pendingCommandID ?? crypto.randomUUID();
        setPendingCommandID(commandId);
        await data(
          sdk.sendTaskRunPrompt({
            ...command,
            path: { ...path(scope), taskID: task.id, runID: selected.runId },
            body: { text: prompt, commandId },
          }),
        );
        setPendingCommandID(null);
        setText('');
        await refresh(scope);
      })
    )
      setText('');
  }
  return (
    <section className="panel conversation-panel" aria-label="Task chat">
      <div className="split">
        <div>
          <h2>Chat</h2>
          <p className="muted">Conversation with the assigned agent</p>
        </div>
        {runs.data?.length ? (
          <Badge>{selected ? runStatusLabel(selected.status) : ''}</Badge>
        ) : null}
      </div>
      {runs.isPending ? (
        <Loading />
      ) : runs.isError ? (
        <ErrorState error={runs.error} />
      ) : !runs.data.length ? (
        <p className="muted spacer">
          No agent runs yet. Assign an agent and start work to launch one.
        </p>
      ) : (
        <>
          {runs.data.length > 1 && (
            <div className="tags spacer">
              {runs.data.map((run) => (
                <Button
                  key={run.runId}
                  variant={run.runId === selected?.runId ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setSelectedRunID(run.runId)}
                >
                  {run.runId.slice(0, 8)} · {runStatusLabel(run.status)}
                </Button>
              ))}
            </div>
          )}
          {selected?.failureReason && <p className="error spacer">{selected.failureReason}</p>}
          {eventsQuery.isError ? (
            <ErrorState error={eventsQuery.error} retry={() => void eventsQuery.refetch()} />
          ) : eventsQuery.isPending ? (
            <Loading />
          ) : (
            <div className="spacer">
              {payloads.length ? (
                payloads.map(({ event, payload }) => {
                  const userMessage = payload.type === 'prompt_accepted';
                  const agentMessage = payload.type === 'update' && payload.update?.kind === 'text';
                  const message = userMessage
                    ? payload.text
                    : agentMessage
                      ? payload.update?.text
                      : '';
                  return (
                    <article
                      className={`chat-message ${userMessage ? 'chat-message-user' : agentMessage ? 'chat-message-agent' : 'chat-message-activity'}`}
                      key={`${event.runId}-${event.sequence}`}
                    >
                      <div className="chat-message-meta">
                        <strong>
                          {userMessage
                            ? 'You'
                            : agentMessage
                              ? 'Agent'
                              : activityLabel(payload.type)}
                        </strong>
                        <span>{date(payload.at ?? selected?.updatedAt ?? '')}</span>
                      </div>
                      {message && <Markdown text={message} />}
                      {payload.update?.title && (
                        <p>
                          {payload.update.title} {payload.update.status}
                        </p>
                      )}
                      {payload.error && <p className="error">{payload.error}</p>}
                      {payload.stopReason && <p className="muted">{payload.stopReason}</p>}
                      {payload.text && !userMessage && !message && <Markdown text={payload.text} />}
                    </article>
                  );
                })
              ) : (
                <p className="muted">
                  {selected?.failureReason
                    ? 'The agent failed before recording activity.'
                    : 'Waiting for agent activity…'}
                </p>
              )}
            </div>
          )}
          {permissions.editWorkspace &&
            selected?.status === 'ready' &&
            task.status === 'in_progress' && (
              <>
                {[...pending.entries()].map(([requestId, permission]) => (
                  <div className="notice spacer" key={requestId}>
                    <strong>
                      Approval needed: {permission.permission?.title ?? 'Agent permission'}
                    </strong>
                    <div className="actions spacer">
                      {permission.permission?.options?.map((option) => (
                        <Button
                          key={option.id}
                          variant={
                            /reject|deny|cancel/i.test(`${option.id} ${option.name}`)
                              ? 'outline'
                              : 'default'
                          }
                          size="sm"
                          disabled={action.pending}
                          onClick={() =>
                            void action.run(async () => {
                              await data(
                                sdk.respondTaskRunPermission({
                                  ...command,
                                  path: { ...path(scope), taskID: task.id, runID: selected.runId },
                                  body: {
                                    requestId,
                                    sessionId: permission.sessionId ?? '',
                                    optionId: option.id,
                                  },
                                }),
                              );
                              await refresh(scope);
                            })
                          }
                        >
                          {option.name}
                        </Button>
                      ))}
                    </div>
                  </div>
                ))}
                <form className="spacer" onSubmit={(event) => void submitPrompt(event)}>
                  <Field label="Message the agent">
                    <textarea
                      value={text}
                      onChange={(event) => setText(event.target.value)}
                      placeholder="Add context or answer the agent…"
                      disabled={!!activeCommand || !!pendingCommandID || action.pending}
                    />
                  </Field>
                  <div className="actions spacer">
                    <Button
                      type="submit"
                      disabled={!text.trim() || !!activeCommand || action.pending}
                    >
                      <Send size={14} /> Send
                    </Button>
                    {activeCommand && (
                      <Button
                        type="button"
                        variant="outline"
                        disabled={action.pending}
                        onClick={() =>
                          void action.run(async () => {
                            await data(
                              sdk.cancelTaskRunPrompt({
                                ...command,
                                path: { ...path(scope), taskID: task.id, runID: selected.runId },
                                body: { commandId: activeCommand },
                              }),
                            );
                            await refresh(scope);
                          })
                        }
                      >
                        <CircleStop size={14} /> Cancel prompt
                      </Button>
                    )}
                  </div>
                </form>
              </>
            )}
          {action.error && <ErrorState error={action.error} />}
        </>
      )}
    </section>
  );
}

function runStatusLabel(status: string): string {
  switch (status) {
    case 'starting':
    case 'process_started':
      return 'Starting';
    case 'ready':
      return 'Running';
    case 'completed':
    case 'completed_pending_close':
      return 'Completed';
    case 'failed':
    case 'failed_pending_close':
      return 'Failed';
    default:
      return status.replaceAll('_', ' ');
  }
}

function activityLabel(type?: string): string {
  switch (type) {
    case 'permission_requested':
      return 'Approval requested';
    case 'permission_response':
      return 'Approval response';
    case 'prompt_completed':
      return 'Agent finished';
    case 'prompt_failed':
      return 'Agent error';
    case 'cancel_requested':
      return 'Cancellation requested';
    case 'update':
      return 'Agent activity';
    default:
      return type?.replaceAll('_', ' ') ?? 'Activity';
  }
}

function runtimeEvent(value: Record<string, unknown>): RuntimeEvent {
  const updateValue = value.update && typeof value.update === 'object' ? value.update : undefined;
  const permissionValue =
    value.permission && typeof value.permission === 'object' ? value.permission : undefined;
  const update = updateValue ? (updateValue as Record<string, unknown>) : undefined;
  const permission = permissionValue ? (permissionValue as Record<string, unknown>) : undefined;
  const options = Array.isArray(permission?.options)
    ? permission.options.filter(isPermissionOption)
    : undefined;
  return {
    type: typeof value.type === 'string' ? value.type : undefined,
    at: typeof value.at === 'string' ? value.at : undefined,
    commandId: typeof value.commandId === 'string' ? value.commandId : undefined,
    requestId: typeof value.requestId === 'string' ? value.requestId : undefined,
    sessionId: typeof value.sessionId === 'string' ? value.sessionId : undefined,
    text: typeof value.text === 'string' ? value.text : undefined,
    error: typeof value.error === 'string' ? value.error : undefined,
    stopReason: typeof value.stopReason === 'string' ? value.stopReason : undefined,
    update: update
      ? {
          kind: typeof update.kind === 'string' ? update.kind : undefined,
          text: typeof update.text === 'string' ? update.text : undefined,
          title: typeof update.title === 'string' ? update.title : undefined,
          status: typeof update.status === 'string' ? update.status : undefined,
        }
      : undefined,
    permission: permission
      ? {
          title: typeof permission.title === 'string' ? permission.title : undefined,
          options,
        }
      : undefined,
  };
}

function isPermissionOption(value: unknown): value is { id: string; name: string; kind: string } {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate.id === 'string' &&
    typeof candidate.name === 'string' &&
    typeof candidate.kind === 'string'
  );
}

function Comments({ taskID }: { taskID: string }) {
  const { scope, session } = useApp();
  const comments = useQuery(commentsQuery(scope, taskID));
  return (
    <div className="source-comments">
      {comments.isPending ? (
        <Loading />
      ) : comments.isError ? (
        <ErrorState error={comments.error} />
      ) : comments.data.length ? (
        <div>
          {comments.data.map((comment) => (
            <article className="comment" key={comment.id}>
              <div className="comment-meta">
                {comment.authorUserId === session.user.id
                  ? session.user.username
                  : comment.authorUserId}{' '}
                · {date(comment.createdAt)}
              </div>
              <Markdown text={comment.body} />
            </article>
          ))}
        </div>
      ) : (
        <p className="muted">No source comments.</p>
      )}
    </div>
  );
}
