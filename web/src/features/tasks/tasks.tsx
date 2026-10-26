import { Markdown } from '../../shared/ui/markdown';
import type { JSX } from 'react';
import { useState } from 'react';
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { ArrowLeft, Bot, LayoutGrid, List, MessageSquare, Search } from 'lucide-react';
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
                <option
                  key={status}
                  value={status}
                  disabled={status === 'in_progress' && task?.status !== 'in_progress'}
                >
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
  const [edit, setEdit] = useState(false);
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
          {permissions.editWorkspace && <Button onClick={() => setEdit(true)}>Edit task</Button>}
        </PageHeader>
      </div>
      {item.status === 'in_progress' && (
        <div className="notice">
          This task is in progress. Live execution state and agent controls are not available in the
          management UI.
        </div>
      )}
      <div className="detail-layout">
        <div>
          <section className="panel">
            <h2>Description</h2>
            <div className="spacer">
              {item.description ? (
                <Markdown text={item.description} />
              ) : (
                <p className="muted">No description provided.</p>
              )}
            </div>
          </section>
          <Comments taskID={item.id} />
        </div>
        <aside>
          <section className="panel">
            <dl className="detail-meta">
              <div>
                <dt>Status</dt>
                <dd>
                  <Badge>{statusNames[item.status]}</Badge>
                </dd>
              </div>
              <div>
                <dt>Assigned agent</dt>
                <dd>
                  {agents.data?.find((a) => a.id === item.assigneeAgentId)?.name ?? 'Unassigned'}
                </dd>
              </div>
              <div>
                <dt>Labels</dt>
                <dd className="tags">
                  {item.labels.length ? item.labels.map((l) => <Badge key={l}>{l}</Badge>) : 'None'}
                </dd>
              </div>
              <div>
                <dt>Components</dt>
                <dd>{item.components.map((c) => c.name).join(', ') || 'None'}</dd>
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
const commentSchema = z.object({ body: z.string().trim().min(1, 'Write a comment.').max(20000) });
function Comments({ taskID }: { taskID: string }) {
  const { scope, permissions, session } = useApp();
  const comments = useQuery(commentsQuery(scope, taskID));
  const refresh = useRefresh();
  const action = useAction();
  const form = useForm({ resolver: zodResolver(commentSchema), defaultValues: { body: '' } });
  return (
    <section className="panel">
      <h2>
        <MessageSquare size={16} style={{ display: 'inline', marginRight: 8 }} />
        Comments
      </h2>
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
        <p className="muted spacer">No comments yet.</p>
      )}
      {permissions.editWorkspace && (
        <div className="spacer">
          <Form
            form={form}
            error={action.error}
            label="Add comment"
            onSubmit={async (body) => {
              if (
                await action.run(async () => {
                  await data(
                    sdk.createTaskComment({ ...command, path: { ...path(scope), taskID }, body }),
                  );
                  await refresh(scope);
                })
              )
                form.reset();
            }}
          >
            <Field label="Comment" error={form.formState.errors.body?.message}>
              <textarea {...form.register('body')} placeholder="Share context or leave a note…" />
            </Field>
          </Form>
        </div>
      )}
    </section>
  );
}
