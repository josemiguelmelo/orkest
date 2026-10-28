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
const syncErrorMessages: Record<string, string> = {
  integration_sync_failed:
    'The sync failed while saving imported tasks. Retry, and contact your administrator if it keeps failing.',
  integration_validation_failed:
    'GitHub could not validate the project connection. Check the token and project access.',
  github_api_unavailable: 'Could not reach GitHub. Check the server network connection and retry.',
  github_project_response_invalid: 'GitHub returned an unexpected response. Retry later.',
  github_project_permission_denied:
    'GitHub denied access to this project. Confirm the token can read Projects and has access to the project.',
  github_project_not_found_or_not_accessible:
    'The GitHub project is unavailable with this token. Check the project URL and token access.',
  github_graphql_query_failed:
    'GitHub could not read this project. Confirm the token permissions and project configuration.',
  github_token_invalid: 'The GitHub token is invalid or revoked. Replace it in workspace settings.',
  github_api_status_401:
    'The GitHub token is invalid or revoked. Replace it in workspace settings.',
  github_api_status_403:
    'GitHub denied access or rate limited the request. Check token permissions and retry later.',
  github_api_status_429: 'GitHub rate limited the request. Retry later.',
  github_api_status_500: 'GitHub is temporarily unavailable. Retry later.',
  github_api_status_502: 'GitHub is temporarily unavailable. Retry later.',
  github_api_status_503: 'GitHub is temporarily unavailable. Retry later.',
};

function syncErrorMessage(code: string) {
  return syncErrorMessages[code] ?? 'The source sync failed. Check the connection and retry.';
}

export function IntegrationsPage() {
  const { scope, permissions } = useApp();
  const integrations = useQuery(integrationsQuery(scope));
  const providers = useQuery(providersQuery(scope.userID));
  const [editing, setEditing] = useState<TaskIntegration | null | undefined>();
  return (
    <>
      <PageHeader
        title="Source integrations"
        description="Connect task sources to import and sync issues automatically."
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
        <>
          <div className="repository-list-heading">
            <span>
              {integrations.data.length}{' '}
              {integrations.data.length === 1 ? 'source integration' : 'source integrations'}
            </span>
            <span className="muted">Imported tasks stay in sync with their source</span>
          </div>
          <div className="grid">
            {integrations.data.map((item) => (
              <IntegrationCard key={item.id} item={item} onEdit={() => setEditing(item)} />
            ))}
          </div>
        </>
      ) : (
        <div className="repository-empty">
          <div className="repository-empty-icon">
            <Plug size={24} />
          </div>
          <h2>Connect a task source</h2>
          <p className="muted">
            Connect GitHub Projects or Jira to import tasks and keep them synchronized with your
            source of truth.
          </p>
          {permissions.workspaceAdmin && (
            <CreateButton onClick={() => setEditing(null)}>Connect source</CreateButton>
          )}
        </div>
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
  const providerLabel = item.provider.replaceAll('_', ' ');
  return (
    <article className="integration-card">
      <div className="integration-card-top">
        <div className="integration-provider-icon">
          <Plug size={21} />
        </div>
        <div className="integration-card-title">
          <h3>{item.name}</h3>
          <p>
            {providerLabel} · sync every {item.interval}
          </p>
        </div>
        <Badge>
          <span className={`status-dot ${item.enabled ? '' : 'status-paused'}`} />
          {item.enabled ? 'Enabled' : 'Paused'}
        </Badge>
      </div>
      {item.labels.length > 0 && (
        <div className="tags">
          {item.labels.map((label) => (
            <Badge key={label}>{label}</Badge>
          ))}
        </div>
      )}
      <p className="muted" style={{ fontSize: '11px' }}>
        Last synchronized: {item.lastSynced ? date(item.lastSynced) : 'Never'}
      </p>
      {item.lastError && (
        <div className="notice error" role="status">
          <strong>Last synchronization failed</strong>
          <p>{syncErrorMessage(item.lastError)}</p>
          <p className="muted">Correct the source configuration if needed, then sync again.</p>
        </div>
      )}
      {permissions.workspaceAdmin && (
        <div className="integration-card-actions">
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
            <RefreshCw size={14} />
            {item.lastError ? 'Retry sync' : 'Sync now'}
          </Button>
          <ConfirmButton
            title="Delete source integration?"
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
        <p role="status" className="success-message">
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
      title={item ? 'Edit source integration' : 'Connect a task source'}
      description={
        item
          ? 'Update your source connection settings. Credentials are write-only.'
          : 'Connect GitHub Projects or Jira to import tasks automatically.'
      }
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
        <div className="form-grid">
          <Field
            label="Provider"
            hint={item ? 'Provider cannot be changed after creation' : undefined}
            error={form.formState.errors.provider?.message}
          >
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
          <Field
            label="Connection name"
            hint="A friendly name for this source"
            error={form.formState.errors.name?.message}
          >
            <input {...form.register('name')} placeholder="e.g., Team GitHub Projects" />
          </Field>
        </div>
        {parsed.error !== undefined && <ErrorState error={parsed.error} />}{' '}
        {parsed.fields && (
          <section className="panel" style={{ padding: '16px', gap: '12px' }}>
            <div>
              <p style={{ fontSize: '12px', fontWeight: 600 }}>Source configuration</p>
              <p className="muted" style={{ fontSize: '11px' }}>
                Configure how tasks are imported from this source
              </p>
            </div>
            <ProviderConfig fields={parsed.fields} config={config} onChange={setConfig} />
          </section>
        )}
        <section className="panel" style={{ padding: '16px', gap: '12px' }}>
          <div>
            <p style={{ fontSize: '12px', fontWeight: 600 }}>Authentication</p>
            <p className="muted" style={{ fontSize: '11px' }}>
              Credentials are encrypted and never displayed after saving
            </p>
          </div>
          <Field
            label={item ? 'Replacement credential' : 'Credential *'}
            hint={
              item
                ? 'Leave blank to keep existing credential. Paste a new one to rotate.'
                : 'Personal access token or API key with read access to the project'
            }
          >
            <input
              type="password"
              autoComplete="new-password"
              {...form.register('credential')}
              placeholder={item ? 'Paste new credential to rotate' : 'Enter your credential'}
            />
          </Field>
        </section>
        <section className="panel" style={{ padding: '16px', gap: '12px' }}>
          <div>
            <p style={{ fontSize: '12px', fontWeight: 600 }}>Sync settings</p>
            <p className="muted" style={{ fontSize: '11px' }}>
              Control which tasks are imported and how often
            </p>
          </div>
          <div className="form-grid">
            <Field
              label="Filter by labels"
              hint="Comma-separated. Leave empty to import all tasks."
            >
              <input {...form.register('labels')} placeholder="bug, enhancement" />
            </Field>
            <Field
              label="Sync interval"
              hint="e.g., 5m, 1h"
              error={form.formState.errors.interval?.message}
            >
              <input {...form.register('interval')} placeholder="5m" />
            </Field>
          </div>
          <label style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '8px 0' }}>
            <input type="checkbox" {...form.register('enabled')} />
            <span style={{ fontSize: '12px' }}>Enable automatic synchronization</span>
          </label>
        </section>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            paddingTop: '8px',
            borderTop: '1px solid var(--border)',
            gap: '12px',
            flexWrap: 'wrap',
          }}
        >
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
                }, 'Connection validated successfully.');
              })();
            }}
          >
            Test connection
          </Button>
          {action.message && (
            <p role="status" className="success-message" style={{ margin: 0 }}>
              {action.message}
            </p>
          )}
        </div>
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
