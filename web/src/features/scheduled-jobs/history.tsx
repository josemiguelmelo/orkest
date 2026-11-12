import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  AlertCircle,
  Ban,
  CalendarClock,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Clock3,
  FolderGit2,
  LoaderCircle,
  RotateCcw,
  Wrench,
  XCircle,
} from 'lucide-react';
import { useApp } from '../../app/context';
import { keys, path } from '../../shared/api/queries';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type {
  ScheduledJob,
  ScheduledJobRun,
  TaskRunEvent,
} from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Empty, ErrorState, Loading } from '../../shared/ui/common';
import { Markdown } from '../../shared/ui/markdown';
import { useAction } from '../../shared/use-action';
import { QuestionCard } from '../tasks/question-card';
import { conversationQuestions } from '../tasks/question-model';

const PAGE_SIZE = 25;
const activeStates = ['pending', 'starting', 'running', 'delivery_uncertain'];

export function JobHistory({ job, close }: { job: ScheduledJob; close: () => void }) {
  const { scope, permissions } = useApp();
  const [selected, setSelected] = useState<ScheduledJobRun | null>(null);
  const [offset, setOffset] = useState(0);
  const runs = useQuery({
    queryKey: [...keys.workspace(scope), 'scheduled-job-runs', job.id, offset],
    queryFn: ({ signal }) =>
      data(
        sdk.listScheduledJobRuns({
          ...command,
          path: { ...path(scope), jobID: job.id },
          query: { limit: PAGE_SIZE, offset },
          signal,
        }),
      ),
    refetchInterval: (query) =>
      query.state.data?.some((run) => activeStates.includes(run.state)) ? 2000 : false,
  });
  const run = runs.data?.find((item) => item.id === selected?.id) ?? selected ?? runs.data?.[0];
  const attentionCount = runs.data?.filter((item) => needsAttention(item.state)).length ?? 0;

  return (
    <Dialog
      open
      className="job-history-dialog"
      onOpenChange={(open) => {
        if (!open) close();
      }}
      title={`${job.name} · run history`}
      description="Review what ran, when it ran, and what the agent reported."
    >
      <div className="job-history">
        <div className="job-history-note">
          <CircleHelp size={16} aria-hidden="true" />
          <span>
            Results and external changes are reported by the agent and aren’t independently
            verified.
          </span>
        </div>

        {runs.isPending && <Loading />}
        {runs.error && <ErrorState error={runs.error} />}
        {runs.data?.length === 0 && (
          <Empty title="No runs yet">Runs will appear here after this job starts.</Empty>
        )}

        {!!runs.data?.length && (
          <div className="job-history-browser">
            <aside className="job-run-sidebar" aria-label="Run history">
              <div className="job-run-sidebar-heading">
                <div>
                  <h3>Recent runs</h3>
                  <p>{runs.data.length} shown · newest first</p>
                </div>
                {attentionCount > 0 && (
                  <span className="job-attention-count">
                    <AlertCircle size={14} aria-hidden="true" />
                    {attentionCount} need attention
                  </span>
                )}
              </div>
              <div className="job-run-list" role="list">
                {runs.data.map((item) => (
                  <button
                    type="button"
                    role="listitem"
                    aria-pressed={run?.id === item.id}
                    className={`job-run-list-item${run?.id === item.id ? ' selected' : ''}`}
                    key={item.id}
                    onClick={() => setSelected(item)}
                  >
                    <span className="job-run-list-topline">
                      <RunStatus state={item.state} />
                      <span className="job-run-list-date">{formatDate(item.scheduledFor)}</span>
                    </span>
                    <span className="job-run-list-meta">
                      <span>
                        {item.repositoryIds.length
                          ? `${item.repositoryIds.length} ${plural(item.repositoryIds.length, 'repository')}`
                          : 'No repositories'}
                      </span>
                      {item.reason && <span>{formatReason(item.reason)}</span>}
                    </span>
                  </button>
                ))}
              </div>
              <div className="job-run-pagination">
                <span>
                  {offset + 1}–{offset + (runs.data?.length ?? 0)}
                </span>
                <div>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Newer runs"
                    title="Newer runs"
                    disabled={offset === 0 || runs.isFetching}
                    onClick={() => {
                      setOffset(Math.max(0, offset - PAGE_SIZE));
                      setSelected(null);
                    }}
                  >
                    <ChevronLeft size={18} />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Older runs"
                    title="Older runs"
                    disabled={runs.isFetching || (runs.data?.length ?? 0) < PAGE_SIZE}
                    onClick={() => {
                      setOffset(offset + PAGE_SIZE);
                      setSelected(null);
                    }}
                  >
                    <ChevronRight size={18} />
                  </Button>
                </div>
              </div>
            </aside>

            <main className="job-run-detail" aria-live="polite">
              {run ? (
                <RunDetails key={run.id} run={run} canWrite={permissions.editWorkspace} />
              ) : null}
            </main>
          </div>
        )}
      </div>
    </Dialog>
  );
}

function RunDetails({ run, canWrite }: { run: ScheduledJobRun; canWrite: boolean }) {
  const { scope } = useApp();
  const action = useAction();
  const runPath = { ...path(scope), jobID: run.jobId, occurrenceID: run.id };
  const events = useQuery({
    queryKey: [...keys.workspace(scope), 'scheduled-job-events', run.id],
    queryFn: ({ signal }) =>
      data(sdk.listScheduledJobRunEvents({ ...command, path: runPath, signal })),
    refetchInterval: activeStates.includes(run.state) ? 2000 : false,
    enabled: !['pending', 'skipped', 'missed', 'needs_attention'].includes(run.state),
  });
  const activity = events.data ?? [];
  const questions = conversationQuestions(activity);
  const output = activity
    .map(readUpdate)
    .filter((update) => update?.kind === 'text' && update.text)
    .map((update) => update?.text ?? '')
    .join('');
  const tools = activity.filter((event) => {
    const kind = readUpdate(event)?.kind;
    return kind === 'tool_call' || kind === 'tool_call_update';
  });
  const pendingPermissions = activity.filter(
    (event) =>
      event.payload.type === 'permission_requested' &&
      !activity.some(
        (answer) =>
          answer.payload.type === 'permission_response' &&
          answer.payload.requestId === event.payload.requestId,
      ),
  );

  return (
    <article className="job-run-report">
      <header className="job-run-report-header">
        <div>
          <RunStatus state={run.state} />
          <h3>{stateTitle(run.state)}</h3>
          <p>Scheduled for {formatDate(run.scheduledFor)}</p>
        </div>
        {canWrite && activeStates.includes(run.state) && (
          <Button
            variant="outline"
            disabled={action.pending}
            onClick={() =>
              void action.run(() => data(sdk.cancelScheduledJobRun({ ...command, path: runPath })))
            }
          >
            <Ban size={16} />
            Cancel run
          </Button>
        )}
      </header>

      {run.reason && (
        <div className={`job-run-reason${needsAttention(run.state) ? ' attention' : ''}`}>
          {needsAttention(run.state) ? <AlertCircle size={17} /> : <CircleHelp size={17} />}
          <div>
            <strong>{needsAttention(run.state) ? 'Needs attention' : 'Run note'}</strong>
            <p>{formatReason(run.reason)}</p>
          </div>
        </div>
      )}

      <div className="job-run-facts">
        <div>
          <CalendarClock size={17} aria-hidden="true" />
          <span>
            <small>Scheduled</small>
            <strong>{formatDate(run.scheduledFor)}</strong>
          </span>
        </div>
        <div>
          <FolderGit2 size={17} aria-hidden="true" />
          <span>
            <small>Repositories</small>
            <strong>
              {run.repositoryIds.length
                ? plural(run.repositoryIds.length, 'repository')
                : 'Agent tools only'}
            </strong>
          </span>
        </div>
        <div>
          <Clock3 size={17} aria-hidden="true" />
          <span>
            <small>Added to history</small>
            <strong>{formatDate(run.createdAt)}</strong>
          </span>
        </div>
      </div>

      <details className="job-run-instructions">
        <summary>Instructions for this run</summary>
        <p>{run.input.instructions}</p>
      </details>

      {!!action.error && <ErrorState error={action.error} />}
      {events.error && <ErrorState error={events.error} />}
      {questions.map((question) => (
        <QuestionCard
          key={`${question.runId}:${question.id}`}
          question={question}
          canAnswer={canWrite && !isTerminal(run.state)}
          respond={async (answer, submissionId) => {
            await data(
              sdk.respondScheduledJobQuestion({
                ...command,
                path: { ...runPath, questionID: question.id },
                body: {
                  taskSessionId: question.taskSessionId,
                  sessionId: question.sessionId,
                  commandId: question.commandId,
                  submissionId,
                  answer,
                },
              }),
            );
            await events.refetch();
          }}
        />
      ))}
      {!isTerminal(run.state) &&
        pendingPermissions.map((event) => (
          <Permission
            key={event.sequence}
            event={event}
            disabled={!canWrite || action.pending}
            respond={async (optionId) => {
              await action.run(async () => {
                await data(
                  sdk.respondScheduledJobPermission({
                    ...command,
                    path: runPath,
                    body: {
                      requestId: String(event.payload.requestId),
                      sessionId: String(event.payload.sessionId),
                      optionId,
                    },
                  }),
                );
                await events.refetch();
              });
            }}
          />
        ))}

      <section className="job-run-result" aria-label="Run result">
        <div className="job-run-section-heading">
          <div>
            <h4>
              {run.state === 'running' || run.state === 'starting'
                ? 'Agent update'
                : 'Agent summary'}
            </h4>
            <p>
              {output
                ? 'The latest response from the agent'
                : resultHint(run.state, events.isPending)}
            </p>
          </div>
          {activeStates.includes(run.state) && (
            <span className="job-live-indicator">
              <span /> Live
            </span>
          )}
        </div>
        {output ? (
          <div className="job-run-output">
            <Markdown text={output} />
          </div>
        ) : (
          <p className="job-run-empty-result">{resultHint(run.state, events.isPending)}</p>
        )}
      </section>

      <details className="job-run-activity">
        <summary>
          <span>Run activity</span>
          <span className="job-run-activity-count">
            {tools.length} {plural(tools.length, 'tool action')}
          </span>
        </summary>
        {tools.length ? (
          <ol>
            {tools.map((event) => (
              <ToolActivity key={event.sequence} event={event} />
            ))}
          </ol>
        ) : (
          <p className="muted">No tool activity recorded for this run.</p>
        )}
      </details>
    </article>
  );
}

function ToolActivity({ event }: { event: TaskRunEvent }) {
  const update = readUpdate(event);
  const title = update?.title || update?.toolKind || 'Agent tool';
  return (
    <li className="job-tool-activity">
      <Wrench size={15} aria-hidden="true" />
      <div>
        <strong>{title}</strong>
        <span>{update?.status ? humanize(update.status) : 'Activity recorded'}</span>
        {update?.output && (
          <details>
            <summary>View tool output</summary>
            <pre>{update.output}</pre>
          </details>
        )}
      </div>
      {eventTime(event) && <time>{formatDate(eventTime(event)!)}</time>}
    </li>
  );
}

function RunStatus({ state }: { state: string }) {
  return (
    <span className={`job-run-status ${statusTone(state)}`}>
      {statusIcon(state)}
      {stateTitle(state)}
    </span>
  );
}

function Permission({
  event,
  disabled,
  respond,
}: {
  event: TaskRunEvent;
  disabled: boolean;
  respond: (optionId: string) => Promise<void>;
}) {
  const permission = event.payload.permission as
    { title?: string; options?: { id: string; name: string }[] } | undefined;
  return (
    <div className="notice">
      <AlertCircle size={18} aria-hidden="true" />
      <div className="job-permission-content">
        <strong>Approval needed: {permission?.title ?? 'Agent tool request'}</strong>
        <div>
          {permission?.options?.map((option) => (
            <Button key={option.id} disabled={disabled} onClick={() => void respond(option.id)}>
              {option.name}
            </Button>
          ))}
        </div>
      </div>
    </div>
  );
}

type ActivityUpdate = {
  kind?: string;
  text?: string;
  title?: string;
  status?: string;
  toolKind?: string;
  output?: string;
};

function readUpdate(event: TaskRunEvent): ActivityUpdate | undefined {
  return event.payload.update as ActivityUpdate | undefined;
}

function eventTime(event: TaskRunEvent): string | undefined {
  const at = event.payload.at;
  return typeof at === 'string' ? at : undefined;
}

function resultHint(state: string, loading: boolean): string {
  if (loading) return 'Loading the agent’s response…';
  if (state === 'pending') return 'This run is waiting to start.';
  if (state === 'starting' || state === 'running')
    return 'The agent is working. Updates appear here automatically.';
  if (state === 'succeeded') return 'The agent finished without leaving a text summary.';
  if (state === 'delivery_uncertain')
    return 'The start of this run could not be confirmed. Check the workspace before retrying.';
  if (state === 'needs_attention')
    return 'Review the reason above and update the job or its access before the next run.';
  if (state === 'missed') return 'This occurrence expired before it could start.';
  if (state === 'skipped')
    return 'A previous run was still unresolved when this occurrence was due.';
  if (state === 'cancelled') return 'This run was cancelled.';
  if (state === 'failed') return 'The run failed before a summary was recorded.';
  return 'No summary was recorded for this run.';
}

function stateTitle(state: string): string {
  const titles: Record<string, string> = {
    pending: 'Waiting',
    starting: 'Starting',
    running: 'Running',
    succeeded: 'Completed',
    failed: 'Failed',
    cancelled: 'Cancelled',
    skipped: 'Skipped',
    missed: 'Missed',
    needs_attention: 'Needs attention',
    delivery_uncertain: 'Start unconfirmed',
  };
  return titles[state] ?? humanize(state);
}

function statusIcon(state: string) {
  if (state === 'succeeded') return <CheckCircle2 size={15} aria-hidden="true" />;
  if (
    state === 'failed' ||
    state === 'missed' ||
    state === 'needs_attention' ||
    state === 'delivery_uncertain'
  )
    return <AlertCircle size={15} aria-hidden="true" />;
  if (state === 'cancelled') return <Ban size={15} aria-hidden="true" />;
  if (state === 'skipped') return <RotateCcw size={15} aria-hidden="true" />;
  if (state === 'running' || state === 'starting')
    return <LoaderCircle size={15} aria-hidden="true" />;
  if (state === 'pending') return <Clock3 size={15} aria-hidden="true" />;
  return <XCircle size={15} aria-hidden="true" />;
}

function statusTone(state: string): string {
  if (state === 'succeeded') return 'success';
  if (needsAttention(state)) return 'attention';
  if (state === 'running' || state === 'starting') return 'active';
  if (state === 'cancelled' || state === 'skipped') return 'neutral';
  return 'pending';
}

function needsAttention(state: string): boolean {
  return ['failed', 'missed', 'needs_attention', 'delivery_uncertain'].includes(state);
}

function isTerminal(state: string): boolean {
  return ['succeeded', 'failed', 'cancelled', 'skipped', 'missed', 'needs_attention'].includes(
    state,
  );
}

function formatReason(reason: string): string {
  const descriptions: Record<string, string> = {
    authorization_revoked: 'The person who enabled this job no longer has access.',
    repository_unavailable: 'One or more selected repositories are no longer available.',
    outside_lateness_window: 'This run was more than one hour late and was skipped.',
    previous_invocation_unresolved: 'A previous run was still active, so this one was skipped.',
    capacity_wait_expired: 'The workspace stayed busy until this run expired.',
    acceptance_not_established: 'The platform could not confirm that the agent accepted this run.',
  };
  return descriptions[reason] ?? humanize(reason);
}

function humanize(value: string): string {
  return value.replaceAll('_', ' ').replace(/^\w/, (letter) => letter.toUpperCase());
}

function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? value
    : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function plural(count: number, word: string): string {
  return `${count} ${word}${count === 1 ? '' : 's'}`;
}
