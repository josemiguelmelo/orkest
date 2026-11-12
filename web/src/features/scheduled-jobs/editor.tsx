import { jobInput } from './model';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import { agentsQuery, repositoriesQuery, path } from '../../shared/api/queries';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { ScheduledJob, ScheduledJobInput } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { CalendarClock, Clock3 } from 'lucide-react';
import { Button } from '../../shared/ui/button';
import { Field } from '../../shared/ui/form';
import { Dialog } from '../../shared/ui/dialog';
import { ErrorState } from '../../shared/ui/common';
import { useAction } from '../../shared/use-action';

const defaults = (): ScheduledJobInput => ({
  name: '',
  instructions: '',
  agentId: '',
  repositoryScope: 'none',
  repositoryIds: [],
  enabled: false,
  maxDurationMinutes: 60,
  schedule: {
    frequency: 'daily',
    timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    hour: 9,
    minute: 0,
    weekday: 1,
  },
});
export function JobEditor({
  job,
  close,
  saved,
}: {
  job: ScheduledJob | undefined;
  close: () => void;
  saved: () => Promise<unknown>;
}) {
  const { scope } = useApp();
  const [input, setInput] = useState<ScheduledJobInput>(job ? jobInput(job) : defaults());
  const [commandId] = useState(() => crypto.randomUUID());
  const [preview, setPreview] = useState<string[] | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const agents = useQuery(agentsQuery(scope));
  const repositories = useQuery(repositoriesQuery(scope));
  const action = useAction();
  const update = (value: Partial<ScheduledJobInput>) => {
    setInput((current) => ({ ...current, ...value }));
    setPreview(null);
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
      title={job ? 'Edit scheduled job' : 'New scheduled job'}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void action.run(async () => {
            const body = { commandId, revision: job?.revision ?? 0, input };
            if (job)
              await data(
                sdk.updateScheduledJob({
                  ...command,
                  path: { ...path(scope), jobID: job.id },
                  body,
                }),
              );
            else await data(sdk.createScheduledJob({ ...command, path: path(scope), body }));
            await saved();
            close();
          });
        }}
      >
        <Field label="Name">
          <input
            required
            maxLength={200}
            value={input.name}
            onChange={(e) => update({ name: e.target.value })}
          />
        </Field>
        <Field label="Instructions">
          <textarea
            required
            rows={6}
            maxLength={60000}
            value={input.instructions}
            onChange={(e) => update({ instructions: e.target.value })}
            placeholder="Review all selected repositories and summarize findings…"
          />
        </Field>
        <Field
          label="Agent"
          hint="Uses this agent’s configured skills, tools and account permissions."
        >
          <select
            required
            value={input.agentId}
            onChange={(e) => update({ agentId: e.target.value })}
          >
            <option value="">Select an agent</option>
            {agents.data?.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Repository scope">
          <select
            value={input.repositoryScope}
            onChange={(e) =>
              update({
                repositoryScope: e.target.value as ScheduledJobInput['repositoryScope'],
                repositoryIds: [],
              })
            }
          >
            <option value="none">None — use configured tools</option>
            <option value="selected">Selected repositories</option>
            <option value="all">All workspace repositories, including future additions</option>
          </select>
        </Field>
        {input.repositoryScope === 'selected' && (
          <Field label="Repositories">
            <select
              multiple
              required
              value={input.repositoryIds}
              onChange={(e) =>
                update({
                  repositoryIds: Array.from(e.target.selectedOptions, (option) => option.value),
                })
              }
            >
              {repositories.data?.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name || r.url}
                </option>
              ))}
            </select>
          </Field>
        )}
        <Field label="Frequency">
          <select
            value={input.schedule.frequency}
            onChange={(e) => {
              const frequency = e.target.value as 'daily' | 'weekly' | 'cron';
              const { cronExpression, ...baseSchedule } = input.schedule;
              update({
                schedule:
                  frequency === 'cron'
                    ? {
                        ...input.schedule,
                        frequency,
                        cronExpression: cronExpression || '0 9 * * 1-5',
                      }
                    : { ...baseSchedule, frequency },
              });
            }}
          >
            <option value="daily">Daily</option>
            <option value="weekly">Weekly</option>
            <option value="cron">Custom crontab</option>
          </select>
        </Field>
        {input.schedule.frequency === 'cron' ? (
          <Field
            label="Cron expression"
            hint="Five fields: minute, hour, day of month, month, weekday. Lists, ranges, steps, names, and Sunday as 0 or 7 are supported. Example: 0 9 * * 1-5 runs weekdays at 09:00. If both day-of-month and weekday are restricted, either can match."
          >
            <input
              required
              maxLength={128}
              spellCheck={false}
              autoComplete="off"
              placeholder="0 9 * * 1-5"
              value={input.schedule.cronExpression ?? ''}
              onChange={(e) =>
                update({
                  schedule: { ...input.schedule, cronExpression: e.target.value },
                })
              }
            />
          </Field>
        ) : null}
        {input.schedule.frequency === 'weekly' && (
          <Field label="Weekday">
            <select
              value={input.schedule.weekday}
              onChange={(e) =>
                update({ schedule: { ...input.schedule, weekday: Number(e.target.value) } })
              }
            >
              {['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'].map(
                (day, i) => (
                  <option key={day} value={i}>
                    {day}
                  </option>
                ),
              )}
            </select>
          </Field>
        )}
        {input.schedule.frequency !== 'cron' && (
          <Field label="Local time">
            <input
              type="time"
              required
              value={`${String(input.schedule.hour).padStart(2, '0')}:${String(input.schedule.minute).padStart(2, '0')}`}
              onChange={(e) => {
                const [hour, minute] = e.target.value.split(':').map(Number);
                update({ schedule: { ...input.schedule, hour: hour ?? 0, minute: minute ?? 0 } });
              }}
            />
          </Field>
        )}
        <Field label="IANA time zone">
          <input
            required
            value={input.schedule.timeZone}
            onChange={(e) => update({ schedule: { ...input.schedule, timeZone: e.target.value } })}
          />
        </Field>
        <Field label="Maximum duration (minutes)">
          <input
            type="number"
            min={1}
            max={1440}
            required
            value={input.maxDurationMinutes}
            onChange={(e) => update({ maxDurationMinutes: Number(e.target.value) })}
          />
        </Field>
        <label>
          <input
            type="checkbox"
            checked={input.enabled}
            onChange={(e) => update({ enabled: e.target.checked })}
          />{' '}
          Enable and authorize future runs using these instructions and resources
        </label>
        <p className="muted">
          Saving authorizes future runs as you. Each run has a fresh session. Overlaps are skipped;
          late schedules and capacity waits have a one-hour grace period. Pausing leaves
          queued/active runs intact. Email jobs require tools connected to the intended account.
        </p>
        <div className="job-preview-actions">
          <Button
            type="button"
            variant="outline"
            disabled={action.pending}
            onClick={() => {
              setPreviewing(true);
              void action
                .run(async () =>
                  setPreview(
                    await data(
                      sdk.previewJobSchedule({
                        ...command,
                        path: path(scope),
                        body: input.schedule,
                      }),
                    ),
                  ),
                )
                .finally(() => setPreviewing(false));
            }}
          >
            <CalendarClock size={16} />
            {previewing ? 'Finding times…' : 'Preview next runs'}
          </Button>
          <span>Check the upcoming dates before you enable this schedule.</span>
        </div>
        {preview !== null && (
          <section className="job-schedule-preview" aria-live="polite">
            <div className="job-schedule-preview-heading">
              <div className="job-schedule-preview-icon">
                <Clock3 size={17} />
              </div>
              <div>
                <h3>
                  Next {preview.length} scheduled {preview.length === 1 ? 'run' : 'runs'}
                </h3>
                <p>Shown in {input.schedule.timeZone}; UTC is included for reference.</p>
              </div>
            </div>
            {preview.length ? (
              <ol>
                {preview.map((timestamp, index) => (
                  <li key={timestamp}>
                    <span className="job-schedule-preview-number">
                      {String(index + 1).padStart(2, '0')}
                    </span>
                    <div className="job-schedule-preview-local">
                      <strong>{formatScheduleTime(timestamp, input.schedule.timeZone)}</strong>
                      <small>
                        {relativeScheduleTime(timestamp)} · {input.schedule.timeZone}
                      </small>
                    </div>
                    <time dateTime={timestamp}>
                      <small>UTC</small>
                      {formatScheduleTime(timestamp, 'UTC')}
                    </time>
                  </li>
                ))}
              </ol>
            ) : (
              <p className="muted">No upcoming runs were returned for this schedule.</p>
            )}
          </section>
        )}
        {(agents.error || repositories.error) && (
          <ErrorState error={agents.error ?? repositories.error} />
        )}
        {!!action.error && <ErrorState error={action.error} />}
        <div className="actions">
          <Button type="button" variant="outline" onClick={close}>
            Cancel
          </Button>
          <Button type="submit" disabled={action.pending}>
            Save scheduled job
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

function formatScheduleTime(value: string, timeZone: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return value;
  return new Intl.DateTimeFormat(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    timeZone,
  }).format(date);
}

function relativeScheduleTime(value: string): string {
  const minutes = Math.round((new Date(value).valueOf() - Date.now()) / 60_000);
  if (minutes < 60) return minutes <= 1 ? 'In less than a minute' : `In ${minutes} minutes`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return hours === 1 ? 'In about an hour' : `In about ${hours} hours`;
  const days = Math.floor(hours / 24);
  if (days === 1) return 'Tomorrow';
  if (days < 7) return `In ${days} days`;
  const weeks = Math.floor(days / 7);
  return weeks === 1 ? 'In about a week' : `In about ${weeks} weeks`;
}
