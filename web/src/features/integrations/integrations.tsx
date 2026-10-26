import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Plug, RefreshCw } from 'lucide-react';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type {
  TaskIntegration,
  TaskIntegrationProvider,
} from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { integrationsQuery, path, providersQuery, useRefresh } from '../../shared/api/queries';
import {
  Badge,
  ConfirmButton,
  CreateButton,
  date,
  Empty,
  ErrorState,
  Loading,
  PageHeader,
} from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Field, Form } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';
import {
  configSchema,
  integrationBody,
  providerFields,
  validateConfig,
  type JsonValue,
  type ProviderFields,
} from './schema';
const schema = z.object({
  provider: z.string().min(1, 'Choose a provider.'),
  name: z.string().trim().min(1).max(200),
  enabled: z.boolean(),
  labels: z.string(),
  interval: z
    .string()
    .trim()
    .regex(/^(?:\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h))+$/, 'Use a duration such as 5m or 1h.')
    .refine((value) => !/^0(?:s|m|h)$/.test(value), 'Use a positive duration.'),
  credential: z.string(),
});
export function IntegrationsPage() {
  const { scope, permissions } = useApp();
  const integrations = useQuery(integrationsQuery(scope));
  const providers = useQuery(providersQuery(scope.userID));
  const [editing, setEditing] = useState<TaskIntegration | null | undefined>();
  return (
    <>
      <PageHeader
        title="Integrations"
        description="Import tasks and keep source-managed content synchronized."
      >
        {permissions.workspaceAdmin && (
          <CreateButton onClick={() => setEditing(null)}>Connect source</CreateButton>
        )}
      </PageHeader>
      {integrations.isPending ? (
        <Loading />
      ) : integrations.isError ? (
        <ErrorState error={integrations.error} />
      ) : integrations.data.length ? (
        <div className="grid">
          {integrations.data.map((item) => (
            <IntegrationCard key={item.id} item={item} onEdit={() => setEditing(item)} />
          ))}
        </div>
      ) : (
        <Empty title="No task sources connected">
          Connect GitHub Projects or Jira Server to import tasks.
        </Empty>
      )}
      {editing !== undefined &&
        (providers.isPending ? (
          <Loading />
        ) : providers.isError ? (
          <ErrorState error={providers.error} />
        ) : (
          <IntegrationEditor
            key={editing?.id ?? 'new'}
            item={editing}
            providers={providers.data}
            onClose={() => setEditing(undefined)}
          />
        ))}
    </>
  );
}
function IntegrationCard({ item, onEdit }: { item: TaskIntegration; onEdit: () => void }) {
  const { scope, permissions } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  return (
    <article className="resource-card">
      <div className="split">
        <Plug size={22} className="card-icon" />
        <Badge>{item.enabled ? 'Enabled' : 'Paused'}</Badge>
      </div>
      <h3>{item.name}</h3>
      <p className="muted">
        {item.provider} · every {item.interval}
      </p>
      <div className="tags">
        {item.labels.map((label) => (
          <Badge key={label}>{label}</Badge>
        ))}
      </div>
      <p className="muted">
        Last synchronized: {item.lastSynced ? date(item.lastSynced) : 'Never'}
      </p>
      {item.lastError && (
        <div className="notice error" role="status">
          <strong>Last synchronization failed.</strong>
          <p>{item.lastError}</p>
          <p className="muted">Correct the source configuration if needed, then sync again.</p>
        </div>
      )}
      {permissions.workspaceAdmin && (
        <div className="actions">
          <Button variant="outline" size="sm" onClick={onEdit}>
            Edit
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={action.pending}
            onClick={() => {
              void action.run(async () => {
                await data(
                  sdk.syncTaskIntegrationNow({
                    ...command,
                    path: { ...path(scope), integrationID: item.id },
                  }),
                );
                await refresh(scope);
              }, 'Synchronization completed.');
            }}
          >
            <RefreshCw size={12} />
            {item.lastError ? 'Retry sync' : 'Sync now'}
          </Button>
          <ConfirmButton
            title="Delete integration?"
            description="This permanently removes the source configuration and all tasks imported by this integration."
            onConfirm={async () => {
              await data(
                sdk.deleteTaskIntegration({
                  ...command,
                  path: { ...path(scope), integrationID: item.id },
                }),
              );
              await refresh(scope);
            }}
          />
        </div>
      )}
      {action.error !== undefined && <ErrorState error={action.error} />}{' '}
      {action.message && (
        <p role="status" className="muted">
          {action.message}
        </p>
      )}
    </article>
  );
}
function IntegrationEditor({
  item,
  providers,
  onClose,
}: {
  item: TaskIntegration | null;
  providers: TaskIntegrationProvider[];
  onClose: () => void;
}) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      provider: item?.provider ?? providers[0]?.provider ?? '',
      name: item?.name ?? '',
      enabled: item?.enabled ?? true,
      labels: item?.labels.join(', ') ?? '',
      interval: item?.interval ?? '5m',
      credential: '',
    },
  });
  const provider = providers.find((p) => p.provider === form.watch('provider'));
  const initial = configSchema.safeParse(item?.config ?? {});
  const [config, setConfig] = useState<Record<string, JsonValue>>(
    initial.success ? initial.data : {},
  );
  const parsed = useMemo(() => {
    try {
      return {
        fields: provider ? providerFields(provider.configSchema) : undefined,
        error: undefined,
      };
    } catch (error) {
      return { fields: undefined, error };
    }
  }, [provider]);
  function body(values: z.infer<typeof schema>) {
    if (!provider || !parsed.fields) throw new Error('Choose a supported provider.');
    if (!item && !values.credential) throw new Error('Enter a credential for the new connection.');
    return integrationBody(values, validateConfig(provider.configSchema, config));
  }
  return (
    <Dialog
      title={item ? 'Edit integration' : 'Connect task source'}
      description="Credentials are write-only. Leave the credential blank when editing to preserve its stored value."
      open
      onOpenChange={(v) => {
        if (!v && !action.pending) onClose();
      }}
    >
      <Form
        form={form}
        error={action.error}
        onSubmit={async (values) => {
          if (
            await action.run(async () => {
              const input = body(values);
              if (item)
                await data(
                  sdk.updateTaskIntegration({
                    ...command,
                    path: { ...path(scope), integrationID: item.id },
                    body: input,
                  }),
                );
              else
                await data(
                  sdk.createTaskIntegration({ ...command, path: path(scope), body: input }),
                );
              form.setValue('credential', '');
              await refresh(scope);
            })
          )
            onClose();
        }}
      >
        <Field label="Provider" error={form.formState.errors.provider?.message}>
          <select
            {...form.register('provider', { onChange: () => setConfig({}) })}
            disabled={!!item}
          >
            {providers.map((p) => (
              <option key={p.provider} value={p.provider}>
                {p.provider.replaceAll('_', ' ')}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Connection name" error={form.formState.errors.name?.message}>
          <input {...form.register('name')} />
        </Field>
        {parsed.error !== undefined && <ErrorState error={parsed.error} />}{' '}
        {parsed.fields && (
          <ProviderConfig fields={parsed.fields} config={config} onChange={setConfig} />
        )}
        <Field label={item ? 'Replacement credential (optional)' : 'Credential'}>
          <input type="password" autoComplete="new-password" {...form.register('credential')} />
        </Field>
        <div className="form-grid">
          <Field label="Required labels" hint="Comma-separated; source content stays read-only.">
            <input {...form.register('labels')} />
          </Field>
          <Field label="Polling interval" error={form.formState.errors.interval?.message}>
            <input {...form.register('interval')} />
          </Field>
        </div>
        <label className="actions">
          <input type="checkbox" {...form.register('enabled')} />
          Enable polling
        </label>
        <Button
          variant="outline"
          disabled={action.pending || !form.watch('credential') || !parsed.fields}
          onClick={() => {
            void form.handleSubmit(async (values) => {
              await action.run(async () => {
                await data(
                  sdk.validateTaskIntegration({
                    ...command,
                    path: path(scope),
                    body: body(values),
                  }),
                );
              }, 'Connection validated.');
            })();
          }}
        >
          Validate connection
        </Button>
        {action.message && (
          <p role="status" className="muted">
            {action.message}
          </p>
        )}
      </Form>
    </Dialog>
  );
}
function ProviderConfig({
  fields,
  config,
  onChange,
}: {
  fields: ProviderFields;
  config: Record<string, JsonValue>;
  onChange: (config: Record<string, JsonValue>) => void;
}) {
  return (
    <>
      {Object.entries(fields.properties).map(([key, field]) => (
        <Field
          key={key}
          label={`${field.title ?? key}${fields.required?.includes(key) ? ' *' : ''}`}
          {...(field.description ? { hint: field.description } : {})}
        >
          {field.type === 'string' ? (
            <input
              required={fields.required?.includes(key) ?? false}
              value={typeof config[key] === 'string' ? config[key] : ''}
              onChange={(e) => onChange({ ...config, [key]: e.target.value })}
            />
          ) : (
            <textarea
              value={
                Array.isArray(config[key])
                  ? config[key].filter((v): v is string => typeof v === 'string').join('\n')
                  : ''
              }
              onChange={(e) =>
                onChange({
                  ...config,
                  [key]: e.target.value
                    .split('\n')
                    .map((s) => s.trim())
                    .filter(Boolean),
                })
              }
              placeholder="One value per line"
            />
          )}
        </Field>
      ))}
    </>
  );
}
