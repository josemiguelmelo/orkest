import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
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
import { Empty, ErrorState, Loading, Badge } from '../../shared/ui/common';
import { Markdown } from '../../shared/ui/markdown';
import { useAction } from '../../shared/use-action';
import { QuestionCard } from '../tasks/question-card';
import { conversationQuestions } from '../tasks/question-model';

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
          query: { limit: 25, offset },
          signal,
        }),
      ),
    refetchInterval: 2000,
  });
  const run = runs.data?.find((r) => r.id === selected?.id) ?? selected;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
      title={`${job.name} — run history`}
    >
      {runs.isPending && <Loading />}
      {runs.error && <ErrorState error={runs.error} />}
      <p className="muted">Outcomes and external effects are agent-reported.</p>
      {runs.data?.length === 0 && <Empty title="No runs yet" />}
      {runs.data?.map((r) => (
        <div className="card" key={r.id}>
          <Button variant="outline" onClick={() => setSelected(r)}>
            {new Date(r.scheduledFor).toLocaleString()} · {r.state}
          </Button>
          {r.reason && <p>{r.reason.replaceAll('_', ' ')}</p>}
        </div>
      ))}
      <div className="actions">
        <Button
          variant="outline"
          disabled={offset === 0 || runs.isPending}
          onClick={() => {
            setOffset(Math.max(0, offset - 25));
            setSelected(null);
          }}
        >
          Newer runs
        </Button>
        <Button
          variant="outline"
          disabled={runs.isPending || (runs.data?.length ?? 0) < 25}
          onClick={() => {
            setOffset(offset + 25);
            setSelected(null);
          }}
        >
          Older runs
        </Button>
      </div>
      {run && <RunActivity key={run.id} run={run} canWrite={permissions.editWorkspace} />}
    </Dialog>
  );
}
function RunActivity({ run, canWrite }: { run: ScheduledJobRun; canWrite: boolean }) {
  const { scope } = useApp();
  const action = useAction();
  const events = useQuery({
    queryKey: [...keys.workspace(scope), 'scheduled-job-events', run.id],
    queryFn: ({ signal }) =>
      data(
        sdk.listScheduledJobRunEvents({
          ...command,
          path: { ...path(scope), jobID: run.jobId, occurrenceID: run.id },
          signal,
        }),
      ),
    refetchInterval: 2000,
    enabled: ['starting', 'running', 'succeeded', 'failed', 'cancelled'].includes(run.state),
  });
  const runPath = { ...path(scope), jobID: run.jobId, occurrenceID: run.id };
  const activity = events.data ?? [];
  const questions = conversationQuestions(activity);
  const terminal = [
    'succeeded',
    'failed',
    'cancelled',
    'skipped',
    'missed',
    'needs_attention',
  ].includes(run.state);
  const pendingPermissions = activity.filter(
    (e) =>
      e.payload.type === 'permission_requested' &&
      !activity.some(
        (answer) =>
          answer.payload.type === 'permission_response' &&
          answer.payload.requestId === e.payload.requestId,
      ),
  );
  return (
    <section>
      <h3>
        Run activity <Badge>{run.state}</Badge>
      </h3>
      {canWrite && !terminal && (
        <Button
          variant="destructive"
          disabled={action.pending}
          onClick={() =>
            void action.run(() => data(sdk.cancelScheduledJobRun({ ...command, path: runPath })))
          }
        >
          Cancel invocation
        </Button>
      )}
      {events.error && <ErrorState error={events.error} />}
      {!!action.error && <ErrorState error={action.error} />}
      {questions.map((q) => (
        <QuestionCard
          key={`${q.runId}:${q.id}`}
          question={q}
          canAnswer={canWrite && !terminal}
          respond={async (answer, submissionId) => {
            await data(
              sdk.respondScheduledJobQuestion({
                ...command,
                path: { ...runPath, questionID: q.id },
                body: {
                  taskSessionId: q.taskSessionId,
                  sessionId: q.sessionId,
                  commandId: q.commandId,
                  submissionId,
                  answer,
                },
              }),
            );
            await events.refetch();
          }}
        />
      ))}
      {!terminal &&
        pendingPermissions.map((e) => (
          <Permission
            key={e.sequence}
            event={e}
            disabled={!canWrite || action.pending}
            respond={async (optionId) => {
              await action.run(async () => {
                await data(
                  sdk.respondScheduledJobPermission({
                    ...command,
                    path: runPath,
                    body: {
                      requestId: String(e.payload.requestId),
                      sessionId: String(e.payload.sessionId),
                      optionId,
                    },
                  }),
                );
                await events.refetch();
              });
            }}
          />
        ))}
      {activity.map((e) => {
        const update = e.payload.update as { text?: string; title?: string } | undefined;
        const text = update?.text ?? e.payload.text;
        return (
          <div key={e.sequence}>
            {typeof text === 'string' && text ? (
              <Markdown text={text} />
            ) : (
              <p className="muted">{update?.title ?? String(e.payload.type)}</p>
            )}
          </div>
        );
      })}
    </section>
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
      <p>Approval required: {permission?.title ?? 'Agent tool request'}</p>
      {permission?.options?.map((option) => (
        <Button key={option.id} disabled={disabled} onClick={() => void respond(option.id)}>
          {option.name}
        </Button>
      ))}
    </div>
  );
}
