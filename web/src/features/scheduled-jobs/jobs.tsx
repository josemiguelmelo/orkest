import { jobInput } from './model';
import { useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  CalendarClock,
  Clock3,
  FolderGit2,
  History,
  Pause,
  Pencil,
  Play,
  Plus,
  Search,
} from 'lucide-react';
import { useApp } from '../../app/context';
import { keys, path } from '../../shared/api/queries';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { ScheduledJob } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { Button } from '../../shared/ui/button';
import { ErrorState, Loading, PageHeader } from '../../shared/ui/common';
import { useAction } from '../../shared/use-action';
import { JobEditor } from './editor';
import { JobHistory } from './history';

export function ScheduledJobsPage() {
  const { scope, permissions } = useApp();
  const cache = useQueryClient();
  const action = useAction();
  const [editing, setEditing] = useState<ScheduledJob | 'new' | null>(null);
  const [selected, setSelected] = useState<ScheduledJob | null>(null);
  const [filter, setFilter] = useState<'all' | 'enabled' | 'paused'>('all');
  const [search, setSearch] = useState('');
  const invocation = useRef<{ job: string; key: string } | null>(null);
  const jobs = useQuery({
    queryKey: [...keys.workspace(scope), 'scheduled-jobs'],
    queryFn: ({ signal }) => data(sdk.listScheduledJobs({ ...command, path: path(scope), signal })),
    refetchInterval: 3000,
  });
  const refresh = () =>
    cache.invalidateQueries({ queryKey: [...keys.workspace(scope), 'scheduled-jobs'] });
  const enabledCount = jobs.data?.filter((job) => job.enabled).length ?? 0;
  const pausedCount = (jobs.data?.length ?? 0) - enabledCount;
  const visibleJobs = (jobs.data ?? [])
    .filter((job) => filter === 'all' || job.enabled === (filter === 'enabled'))
    .filter((job) => {
      const searchText = `${job.name} ${job.instructions} ${job.schedule.cronExpression ?? ''}`;
      return searchText.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase());
    })
    .sort((a, b) => Number(b.enabled) - Number(a.enabled) || a.name.localeCompare(b.name));
  if (jobs.isPending) return <Loading />;
  if (jobs.isError) return <ErrorState error={jobs.error} />;
  return (
    <div className="page">
      <PageHeader
        title="Scheduled jobs"
        description="Repeat agent work on a schedule, with independent runs and results."
      >
        {permissions.editWorkspace && !!jobs.data.length && (
          <Button onClick={() => setEditing('new')}>New scheduled job</Button>
        )}
      </PageHeader>
      {!!action.error && <ErrorState error={action.error} />}
      {!jobs.data.length && (
        <section className="scheduled-jobs-empty">
          <div className="scheduled-jobs-empty-icon">
            <CalendarClock size={24} />
          </div>
          <p className="eyebrow">REPEATABLE AGENT WORK</p>
          <h2>No scheduled jobs yet</h2>
          <p>
            Set up recurring repository reviews, documentation updates, or work through your agent’s
            connected tools. Each run gets its own activity and results.
          </p>
          {permissions.editWorkspace && (
            <Button onClick={() => setEditing('new')}>
              <Plus size={16} />
              Create your first job
            </Button>
          )}
        </section>
      )}
      {!!jobs.data.length && (
        <>
          <section className="scheduled-jobs-overview" aria-label="Scheduled job totals">
            <OverviewCount label="Enabled" count={enabledCount} />
            <OverviewCount label="Paused" count={pausedCount} />
            <OverviewCount label="All jobs" count={jobs.data.length} />
          </section>
          <div className="scheduled-jobs-toolbar">
            <label className="scheduled-jobs-search">
              <Search size={17} aria-hidden="true" />
              <span className="sr-only">Search scheduled jobs</span>
              <input
                type="search"
                aria-label="Search scheduled jobs"
                placeholder="Search by name, instructions, or cron expression"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </label>
            <div className="scheduled-jobs-filters" role="group" aria-label="Filter scheduled jobs">
              {(['all', 'enabled', 'paused'] as const).map((value) => (
                <button
                  type="button"
                  key={value}
                  aria-pressed={filter === value}
                  onClick={() => setFilter(value)}
                >
                  {value === 'all' ? 'All' : value === 'enabled' ? 'Enabled' : 'Paused'}
                  <span>
                    {value === 'all'
                      ? jobs.data.length
                      : value === 'enabled'
                        ? enabledCount
                        : pausedCount}
                  </span>
                </button>
              ))}
            </div>
          </div>
          {visibleJobs.length ? (
            <div className="scheduled-job-list">
              {visibleJobs.map((job) => (
                <ScheduledJobCard
                  key={job.id}
                  job={job}
                  canEdit={permissions.editWorkspace}
                  pending={action.pending}
                  edit={() => setEditing(job)}
                  history={() => setSelected(job)}
                  toggle={() =>
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
                  run={() => {
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
                />
              ))}
            </div>
          ) : (
            <div className="scheduled-jobs-no-results">
              <Search size={19} />
              <strong>No matching jobs</strong>
              <span>Try another search or change the status filter.</span>
              <Button
                variant="ghost"
                onClick={() => {
                  setSearch('');
                  setFilter('all');
                }}
              >
                Clear filters
              </Button>
            </div>
          )}
        </>
      )}
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

function OverviewCount({ label, count }: { label: string; count: number }) {
  return (
    <div className="scheduled-jobs-overview-item">
      <strong>{count}</strong>
      <span>{label}</span>
    </div>
  );
}

function ScheduledJobCard({
  job,
  canEdit,
  pending,
  edit,
  history,
  toggle,
  run,
}: {
  job: ScheduledJob;
  canEdit: boolean;
  pending: boolean;
  edit: () => void;
  history: () => void;
  toggle: () => void;
  run: () => void;
}) {
  return (
    <article className={`scheduled-job-card${job.enabled ? '' : ' is-paused'}`}>
      <div className="scheduled-job-card-main">
        <div className="scheduled-job-card-heading">
          <div className="scheduled-job-icon">
            <CalendarClock size={19} aria-hidden="true" />
          </div>
          <div className="scheduled-job-title">
            <div>
              <h2>{job.name}</h2>
              <span className={`scheduled-job-state ${job.enabled ? 'enabled' : 'paused'}`}>
                {job.enabled ? 'Enabled' : 'Paused'}
              </span>
            </div>
            <p>{job.instructions}</p>
          </div>
        </div>
        <div className="scheduled-job-details">
          <div className="scheduled-job-detail schedule">
            <CalendarClock size={16} aria-hidden="true" />
            <span>
              <small>Schedule</small>
              <strong>{scheduleLabel(job)}</strong>
            </span>
          </div>
          <div className="scheduled-job-detail next">
            <Clock3 size={16} aria-hidden="true" />
            <span>
              <small>{job.enabled ? 'Next run' : 'Schedule status'}</small>
              <strong>
                {job.enabled
                  ? formatNextRun(job.nextDueAt, job.schedule.timeZone)
                  : 'Paused until enabled'}
              </strong>
              {job.enabled && <small>{relativeNextRun(job.nextDueAt)}</small>}
            </span>
          </div>
          <div className="scheduled-job-detail resources">
            <FolderGit2 size={16} aria-hidden="true" />
            <span>
              <small>Resources</small>
              <strong>{resourceScope(job)}</strong>
            </span>
          </div>
        </div>
      </div>
      <div className="scheduled-job-card-actions">
        {canEdit && (
          <>
            <Button onClick={run} disabled={pending}>
              <Play size={15} />
              Run now
            </Button>
            <Button variant="outline" onClick={edit}>
              <Pencil size={15} />
              Edit
            </Button>
            <Button variant="outline" onClick={toggle} disabled={pending}>
              {job.enabled ? <Pause size={15} /> : <Play size={15} />}
              {job.enabled ? 'Pause' : 'Enable'}
            </Button>
          </>
        )}
        <Button variant="ghost" onClick={history}>
          <History size={15} />
          History
        </Button>
      </div>
    </article>
  );
}

function scheduleLabel(job: ScheduledJob): string {
  const schedule = job.schedule;
  if (schedule.frequency === 'cron') return `Cron · ${schedule.cronExpression ?? ''}`;
  const time = `${String(schedule.hour).padStart(2, '0')}:${String(schedule.minute).padStart(2, '0')}`;
  if (schedule.frequency === 'daily') return `Every day · ${time}`;
  const weekdays = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
  return `Every ${weekdays[schedule.weekday] ?? 'week'} · ${time}`;
}

function resourceScope(job: ScheduledJob): string {
  if (job.repositoryScope === 'all') return 'All workspace repositories';
  if (job.repositoryScope === 'selected')
    return `${job.repositoryIds.length} selected repositories`;
  return 'Connected agent tools';
}

function formatNextRun(value: string, timeZone: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return 'Time unavailable';
  return new Intl.DateTimeFormat(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    timeZoneName: 'short',
    timeZone,
  }).format(date);
}

function relativeNextRun(value: string): string {
  const minutes = Math.round((new Date(value).valueOf() - Date.now()) / 60_000);
  if (!Number.isFinite(minutes)) return '';
  if (minutes < 0) return 'Overdue';
  if (minutes < 60) return minutes <= 1 ? 'In less than a minute' : `In ${minutes} minutes`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return hours === 1 ? 'In about an hour' : `In about ${hours} hours`;
  const days = Math.floor(hours / 24);
  if (days === 1) return 'Tomorrow';
  if (days < 7) return `In ${days} days`;
  const weeks = Math.floor(days / 7);
  return weeks === 1 ? 'In about a week' : `In about ${weeks} weeks`;
}
