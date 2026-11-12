import { jobInput } from './model';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import { agentsQuery, repositoriesQuery, path } from '../../shared/api/queries';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { ScheduledJob, ScheduledJobInput } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
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
  const [preview, setPreview] = useState<string[]>([]);
  const agents = useQuery(agentsQuery(scope));
  const repositories = useQuery(repositoriesQuery(scope));
  const action = useAction();
  const update = (value: Partial<ScheduledJobInput>) => {
    setInput((current) => ({ ...current, ...value }));
    setPreview([]);
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
            onChange={(e) =>
              update({
                schedule: { ...input.schedule, frequency: e.target.value as 'daily' | 'weekly' },
              })
            }
          >
            <option value="daily">Daily</option>
            <option value="weekly">Weekly</option>
          </select>
        </Field>
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
        <Button
          type="button"
          variant="outline"
          disabled={action.pending}
          onClick={() =>
            void action.run(async () =>
              setPreview(
                await data(
                  sdk.previewJobSchedule({ ...command, path: path(scope), body: input.schedule }),
                ),
              ),
            )
          }
        >
          Preview next runs
        </Button>
        {preview.length > 0 && (
          <ul>
            {preview.map((t) => (
              <li key={t}>
                {new Date(t).toLocaleString(undefined, { timeZone: input.schedule.timeZone })}{' '}
                {input.schedule.timeZone} · {t}
              </li>
            ))}
          </ul>
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
