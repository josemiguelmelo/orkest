import { TaskChat } from './task-chat';
import { Markdown } from '../../shared/ui/markdown';
import type { JSX } from 'react';
import { useState } from 'react';
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { useSessionChoice } from './session-choice';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { ArrowLeft, Bot, GitBranch, LayoutGrid, List, Search, X } from 'lucide-react';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { Task } from '../../shared/api/generated/types.gen';
import {
  agentsQuery,
  commentsQuery,
  repositoriesQuery,
  path,
  taskQuery,
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
  const refreshTasks = useRefresh();
  const agents = useQuery(agentsQuery(scope));
  const search = useSearch({ strict: false });
  const navigate = useNavigate();
  const [create, setCreate] = useState(false);
  const [selectedTask, setSelectedTask] = useState<string>();
  const [dragTarget, setDragTarget] = useState<string>();
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
      <div className="toolbar task-toolbar">
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
        <>
          <div className="board-hint">
            <span>Move work through your workflow</span>
            <span>Drag a card to change its lane</span>
          </div>
          <div className="board">
            {statuses.map((status) => (
              <section
                className={`board-column${dragTarget === status ? ' is-drop-target' : ''}`}
                data-status={status}
                key={status}
                onDragEnter={(event) => {
                  if (permissions.editWorkspace) {
                    event.preventDefault();
                    setDragTarget(status);
                  }
                }}
                onDragLeave={(event) => {
                  if (event.currentTarget === event.target) setDragTarget(undefined);
                }}
                onDragOver={(event) => {
                  if (permissions.editWorkspace) event.preventDefault();
                }}
                onDrop={(event) => {
                  event.preventDefault();
                  setDragTarget(undefined);
                  const id = event.dataTransfer.getData('text/task-id');
                  const dragged = visible.find((item) => item.id === id);
                  if (dragged && dragged.status !== status && dragged.status !== 'done')
                    void updateTaskValues(scope, dragged, { status }).then(() =>
                      refreshTasks(scope),
                    );
                }}
              >
                <h2 className="column-heading">
                  <span className={`column-status-icon status-${status}`} />
                  <span>{statusNames[status]}</span>
                  <span className="column-count">
                    {visible.filter((t) => t.status === status).length}
                  </span>
                </h2>
                <p className="column-description">
                  {
                    {
                      todo: 'Ready to pick up',
                      in_progress: 'Work in progress',
                      in_review: 'Ready for review',
                      done: 'Completed work',
                    }[status]
                  }
                </p>
                {visible
                  .filter((t) => t.status === status)
                  .map((task) => (
                    <TaskCard
                      key={task.id}
                      task={task}
                      agents={agents.data ?? []}
                      canEdit={permissions.editWorkspace}
                      onOpen={() => setSelectedTask(task.id)}
                    />
                  ))}
                {!visible.some((task) => task.status === status) && (
                  <div className="column-empty">No tasks here yet</div>
                )}
              </section>
            ))}
          </div>
        </>
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
                    <button
                      className="text-link task-title-button"
                      onClick={() => setSelectedTask(task.id)}
                    >
                      {task.title}
                    </button>
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
      {selectedTask && (
        <TaskDrawer taskId={selectedTask} onClose={() => setSelectedTask(undefined)} />
      )}
    </>
  );
}

function TaskCard({
  task,
  agents,
  canEdit,
  onOpen,
}: {
  task: Task;
  agents: { id: string; name: string }[];
  canEdit: boolean;
  onOpen: () => void;
}): JSX.Element {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const runs = useQuery(taskRunsQuery(scope, task.id));
  const latest = runs.data?.[0];
  const execution = latest?.executionState;
  const badge =
    latest?.queuePaused || latest?.recoveryBlocked
      ? { label: 'On hold', color: 'blue' }
      : execution === 'waiting_for_permission'
        ? { label: 'Waiting for input', color: 'yellow' }
        : execution === 'running' || latest?.status === 'ready'
          ? { label: 'In progress', color: 'green' }
          : undefined;
  return (
    <article
      className="task-card"
      draggable={canEdit && task.status !== 'done'}
      onDragStart={(event) => event.dataTransfer.setData('text/task-id', task.id)}
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === 'Enter') onOpen();
      }}
      tabIndex={0}
      aria-label={`Open task ${task.title}`}
    >
      <div className="task-meta">
        <span>{task.id.slice(0, 8)}</span>
        {task.sourceProvider === 'manual' ? (
          <span>Manual</span>
        ) : (
          <span className="task-source">
            <SourceProviderIcon provider={task.sourceProvider} />
            {task.sourceId && <span>{task.sourceId}</span>}
          </span>
        )}
      </div>
      <div className="task-status-row">
        <span className={`task-status-pill ${badge ? `is-${badge.color}` : ''}`}>
          <span
            className={`status-dot ${badge ? `status-${badge.color}` : `status-${task.status}`}`}
          />
          {badge?.label ?? statusNames[task.status]}
        </span>
      </div>
      <h3>{task.title}</h3>
      <div className="tags">
        {task.labels.map((label) => (
          <Badge key={label}>{label}</Badge>
        ))}
      </div>
      <div className="task-agent">
        <Bot size={12} style={{ display: 'inline', marginRight: 5 }} />
        {canEdit ? (
          <select
            aria-label={`Assign ${task.title}`}
            value={task.assigneeAgentId ?? ''}
            onClick={(event) => event.stopPropagation()}
            onChange={(event) =>
              void action.run(async () => {
                try {
                  await data(
                    sdk.updateTask({
                      ...command,
                      path: { ...path(scope), taskID: task.id },
                      body: {
                        title: task.title,
                        description: task.description,
                        status: task.status,
                        assigneeAgentId: event.target.value,
                        labels: task.labels,
                        repositoryIds: task.repositories.map((r) => r.id),
                      },
                    }),
                  );
                } finally {
                  await refresh(scope);
                }
              })
            }
          >
            <option value="">Unassigned</option>
            {agents.map((agent) => (
              <option key={agent.id} value={agent.id}>
                {agent.name}
              </option>
            ))}
          </select>
        ) : (
          (agents.find((a) => a.id === task.assigneeAgentId)?.name ?? 'Unassigned')
        )}
      </div>
    </article>
  );
}

async function updateTaskValues(
  scope: ReturnType<typeof useApp>['scope'],
  task: Task,
  patch: { status: Task['status'] },
) {
  await data(
    sdk.updateTask({
      ...command,
      path: { ...path(scope), taskID: task.id },
      body: {
        title: task.title,
        description: task.description,
        status: patch.status,
        assigneeAgentId: task.assigneeAgentId ?? '',
        labels: task.labels,
        repositoryIds: task.repositories.map((r) => r.id),
      },
    }),
  );
}

function TaskDrawer({ taskId, onClose }: { taskId: string; onClose: () => void }): JSX.Element {
  const { scope } = useApp();
  const task = useQuery(taskQuery(scope, taskId));
  const agents = useQuery(agentsQuery(scope));
  return (
    <div
      className="task-drawer-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <aside className="task-drawer" role="dialog" aria-modal="true" aria-label="Task details">
        <button className="task-drawer-close" onClick={onClose} aria-label="Close task">
          <X size={18} />
        </button>
        {task.isPending ? (
          <Loading />
        ) : task.isError ? (
          <ErrorState error={task.error} />
        ) : (
          <>
            <div className="task-drawer-heading">
              <span className="muted">Task {task.data.id.slice(0, 8)}</span>
              <h2>{task.data.title}</h2>
            </div>
            <Badge>{statusNames[task.data.status]}</Badge>
            {task.data.sourceProvider !== 'manual' && (
              <div className="task-drawer-section">
                <h3>External issue</h3>
                <p className="task-source">
                  <SourceProviderIcon provider={task.data.sourceProvider} />
                  {task.data.sourceUrl && /^https?:\/\//i.test(task.data.sourceUrl) ? (
                    <a
                      className="text-link"
                      href={task.data.sourceUrl}
                      target="_blank"
                      rel="noreferrer"
                    >
                      {task.data.sourceId || task.data.sourceProvider} ↗
                    </a>
                  ) : (
                    task.data.sourceId || task.data.sourceProvider
                  )}
                </p>
              </div>
            )}
            <div className="task-drawer-section">
              <h3>Assigned agent</h3>
              <p>
                {agents.data?.find((a) => a.id === task.data.assigneeAgentId)?.name ?? 'Unassigned'}
              </p>
            </div>
            <div className="task-drawer-section">
              <h3>Description</h3>
              {task.data.description ? (
                <Markdown text={task.data.description} />
              ) : (
                <p className="muted">No description provided.</p>
              )}
            </div>
            <div className="task-drawer-section">
              <h3>Task chat</h3>
              <TaskChat key={task.data.id} task={task.data} />
            </div>
            <Link
              className="text-link"
              to="/organizations/$orgId/workspaces/$workspaceId/tasks/$taskId"
              params={{ orgId: scope.orgID, workspaceId: scope.workspaceID, taskId }}
              onClick={onClose}
            >
              Open full task
            </Link>
          </>
        )}
      </aside>
    </div>
  );
}

export function TaskEditor({ task, onClose }: { task?: Task; onClose: () => void }) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const sessionChoice = useSessionChoice();
  const runs = useQuery({ ...taskRunsQuery(scope, task?.id ?? ''), enabled: !!task });
  const agents = useQuery(agentsQuery(scope));
  const repositories = useQuery(repositoriesQuery(scope));
  const form = useForm({
    resolver: zodResolver(taskFormSchema),
    defaultValues: {
      title: task?.title ?? '',
      description: task?.description ?? '',
      status: task?.status ?? 'todo',
      assigneeAgentId: task?.assigneeAgentId ?? '',
      labels: task?.labels.join(', ') ?? '',
      repositoryIds: task?.repositories.map((repository) => repository.id) ?? [],
    },
  });
  const imported = !!task && task.sourceProvider !== 'manual';
  return (
    <Dialog
      title={task ? 'Edit task' : 'New task'}
      description={
        imported
          ? 'Imported content is managed at its source. Update assignment, repositories, or local workflow here.'
          : 'Organize the work and assign an agent definition. Assignment does not start execution.'
      }
      open
      onOpenChange={(open) => {
        if (!open && !action.pending) onClose();
      }}
    >
      {sessionChoice.dialog}
      <Form
        form={form}
        error={action.error}
        onSubmit={async (values) => {
          const starting = task && task.status !== 'in_progress' && values.status === 'in_progress';
          const sessionMode =
            starting && runs.data?.some((run) => run.taskSessionId)
              ? await sessionChoice.choose()
              : undefined;
          if (sessionMode === null) return;
          if (
            await action.run(async () => {
              const body = { ...taskInput(values, task), ...(sessionMode ? { sessionMode } : {}) };
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
          <Field
            label="Status"
            hint="Starting work lets you choose whether to continue the saved session."
          >
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
        {repositories.isError ? (
          <ErrorState error={repositories.error} />
        ) : (
          <Selection
            label="Repositories"
            options={
              repositories.data?.map((repository) => ({
                id: repository.id,
                name: repository.name || repository.url,
              })) ?? []
            }
            value={form.watch('repositoryIds')}
            onChange={(ids) => form.setValue('repositoryIds', ids, { shouldDirty: true })}
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
  const repositories = useQuery(repositoriesQuery(scope));
  const [edit, setEdit] = useState(false);
  const startAction = useAction();
  const [startCommandID, setStartCommandID] = useState<string>();
  const sessionChoice = useSessionChoice();
  const runs = useQuery(taskRunsQuery(scope, taskId));
  const refresh = useRefresh();
  if (task.isPending) return <Loading />;
  if (task.isError) return <ErrorState error={task.error} />;
  const item = task.data;
  return (
    <>
      {sessionChoice.dialog}
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
          {permissions.editWorkspace &&
            (['todo', 'in_review'].includes(item.status) ||
              (item.status === 'in_progress' &&
                (!runs.data?.length || runs.data[0]?.status === 'failed'))) &&
            item.assigneeAgentId && (
              <Button
                disabled={startAction.pending}
                onClick={() => {
                  void startAction.run(async () => {
                    const sessionMode = runs.data?.some((run) => run.taskSessionId)
                      ? await sessionChoice.choose()
                      : undefined;
                    if (sessionMode === null) return;
                    const commandId = startCommandID ?? crypto.randomUUID();
                    setStartCommandID(commandId);
                    try {
                      await data(
                        sdk.updateTask({
                          ...command,
                          path: { ...path(scope), taskID: item.id },
                          body: {
                            title: item.title,
                            description: item.description,
                            status: 'in_progress',
                            startWork: true,
                            commandId,
                            ...(sessionMode ? { sessionMode } : {}),
                            assigneeAgentId: item.assigneeAgentId ?? '',
                            labels: item.labels,
                            repositoryIds: item.repositories.map((repository) => repository.id),
                          },
                        }),
                      );
                      setStartCommandID(undefined);
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
          <TaskChat key={item.id} task={item} />
        </div>
        <aside>
          <section className="panel">
            <dl className="detail-meta">
              {permissions.editWorkspace ? (
                <TaskPropertyControls
                  task={item}
                  agents={agents.data ?? []}
                  repositories={
                    repositories.data?.map((repository) => ({
                      id: repository.id,
                      name: repository.name || repository.url,
                    })) ?? []
                  }
                  agentsLoading={agents.isPending || agents.isError}
                  repositoriesLoading={repositories.isPending || repositories.isError}
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
                    <dt>Repositories</dt>
                    <dd>
                      {item.repositories.map((repository) => repository.name).join(', ') || 'None'}
                    </dd>
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
                      <SourceProviderIcon provider={item.sourceProvider} />
                      {item.sourceId || item.sourceProvider} ↗
                    </a>
                  ) : (
                    <>
                      <SourceProviderIcon provider={item.sourceProvider} />{' '}
                      {item.sourceId || item.sourceProvider}
                    </>
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

function SourceProviderIcon({ provider }: { provider: string }): JSX.Element {
  if (provider === 'github_cloud') return <GitBranch size={13} aria-label="GitHub" />;
  if (provider === 'jira_server')
    return (
      <span className="task-source-jira" aria-label="Jira">
        J
      </span>
    );
  return (
    <span className="task-source-generic" aria-label={provider}>
      {provider.slice(0, 1).toUpperCase()}
    </span>
  );
}

function TaskPropertyControls({
  task,
  agents,
  repositories,
  agentsLoading,
  repositoriesLoading,
}: {
  task: Task;
  agents: { id: string; name: string }[];
  repositories: { id: string; name: string }[];
  agentsLoading: boolean;
  repositoriesLoading: boolean;
}): JSX.Element {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  async function update(patch: {
    status?: Task['status'];
    assigneeAgentId?: string;
    repositoryIds?: string[];
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
              repositoryIds:
                patch.repositoryIds ?? task.repositories.map((repository) => repository.id),
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
        <dt>Repositories</dt>
        <dd>
          {repositoriesLoading ? (
            <span className="muted">Loading repositories…</span>
          ) : (
            <div className="repository-chips" aria-label="Repositories">
              {repositories.length ? (
                repositories.map((repository) => {
                  const selected = task.repositories.some((item) => item.id === repository.id);
                  return (
                    <button
                      className={`repository-chip${selected ? ' is-selected' : ''}`}
                      type="button"
                      key={repository.id}
                      aria-pressed={selected}
                      disabled={action.pending}
                      onClick={() =>
                        void update({
                          repositoryIds: selected
                            ? task.repositories
                                .filter((item) => item.id !== repository.id)
                                .map((item) => item.id)
                            : [...task.repositories.map((item) => item.id), repository.id],
                        })
                      }
                    >
                      {repository.name}
                    </button>
                  );
                })
              ) : (
                <span className="muted">No repositories available.</span>
              )}
            </div>
          )}
          {action.error ? <ErrorState error={action.error} /> : null}
        </dd>
      </div>
    </>
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
