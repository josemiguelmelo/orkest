import { jobInput } from './model';
import { useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import { keys, path } from '../../shared/api/queries';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { ScheduledJob } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { Button } from '../../shared/ui/button';
import { Empty, ErrorState, Loading, PageHeader, Badge } from '../../shared/ui/common';
import { useAction } from '../../shared/use-action';
import { JobEditor } from './editor';
import { JobHistory } from './history';

export function ScheduledJobsPage() {
  const { scope, permissions } = useApp();
  const cache = useQueryClient();
  const action = useAction();
  const [editing, setEditing] = useState<ScheduledJob | 'new' | null>(null);
  const [selected, setSelected] = useState<ScheduledJob | null>(null);
  const invocation = useRef<{ job: string; key: string } | null>(null);
  const jobs = useQuery({
    queryKey: [...keys.workspace(scope), 'scheduled-jobs'],
    queryFn: ({ signal }) => data(sdk.listScheduledJobs({ ...command, path: path(scope), signal })),
    refetchInterval: 3000,
  });
  const refresh = () =>
    cache.invalidateQueries({ queryKey: [...keys.workspace(scope), 'scheduled-jobs'] });
  if (jobs.isPending) return <Loading />;
  if (jobs.isError) return <ErrorState error={jobs.error} />;
  return (
    <div className="page">
      <PageHeader
        title="Scheduled jobs"
        description="Repeat agent work on a schedule, with independent runs and results."
      >
        {permissions.editWorkspace && (
          <Button onClick={() => setEditing('new')}>New scheduled job</Button>
        )}
      </PageHeader>
      {!!action.error && <ErrorState error={action.error} />}
      {!jobs.data.length && (
        <Empty title="No scheduled jobs">
          Schedule repository reviews, documentation updates, or work through your agent’s connected
          tools.
        </Empty>
      )}
      <div className="resource-list">
        {jobs.data.map((job) => (
          <article className="card" key={job.id}>
            <h2>
              {job.name} <Badge>{job.enabled ? 'Enabled' : 'Paused'}</Badge>
            </h2>
            <p>
              {job.schedule.frequency === 'cron'
                ? `Cron · ${job.schedule.cronExpression}`
                : `${job.schedule.frequency === 'daily' ? 'Daily' : 'Weekly'} at ${String(job.schedule.hour).padStart(2, '0')}:${String(job.schedule.minute).padStart(2, '0')}`}{' '}
              · {job.schedule.timeZone}
            </p>
            <p className="muted">
              Next: {new Date(job.nextDueAt).toLocaleString()} ·{' '}
              {job.repositoryScope === 'all'
                ? 'All workspace repositories'
                : job.repositoryScope === 'selected'
                  ? `${job.repositoryIds.length} selected repositories`
                  : 'Configured agent tools'}
            </p>
            <div className="actions">
              <Button variant="outline" onClick={() => setSelected(job)}>
                Run history
              </Button>
              {permissions.editWorkspace && (
                <>
                  <Button variant="outline" onClick={() => setEditing(job)}>
                    Edit
                  </Button>
                  <Button
                    disabled={action.pending}
                    onClick={() => {
                      if (invocation.current?.job !== job.id)
                        invocation.current = { job: job.id, key: crypto.randomUUID() };
                      const commandId = invocation.current.key;
                      void action.run(async () => {
                        await data(
                          sdk.runScheduledJob({
                            ...command,
                            path: { ...path(scope), jobID: job.id },
                            body: { commandId },
                          }),
                        );
                        invocation.current = null;
                        setSelected(job);
                        await refresh();
                      });
                    }}
                  >
                    Run now
                  </Button>
                  <Button
                    variant="outline"
                    disabled={action.pending}
                    onClick={() =>
                      void action.run(async () => {
                        await data(
                          sdk.updateScheduledJob({
                            ...command,
                            path: { ...path(scope), jobID: job.id },
                            body: {
                              revision: job.revision,
                              input: { ...jobInput(job), enabled: !job.enabled },
                            },
                          }),
                        );
                        await refresh();
                      })
                    }
                  >
                    {job.enabled ? 'Pause' : 'Enable'}
                  </Button>
                </>
              )}
            </div>
          </article>
        ))}
      </div>
      {editing && (
        <JobEditor
          key={editing === 'new' ? 'new' : editing.id}
          job={editing === 'new' ? undefined : editing}
          close={() => setEditing(null)}
          saved={refresh}
        />
      )}
      {selected && <JobHistory key={selected.id} job={selected} close={() => setSelected(null)} />}
    </div>
  );
}
