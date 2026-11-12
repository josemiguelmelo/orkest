import { useState } from 'react';
import { useQuery, queryOptions } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type {
  WorkflowStageIntegration,
  WorkflowStageIntegrationInputWritable,
} from '../../shared/api/generated/types.gen';
import {
  codingIntegrationsQuery,
  keys,
  path,
  repositoriesQuery,
  type WorkspaceScope,
} from '../../shared/api/queries';
import { Button } from '../../shared/ui/button';
import { Badge, ConfirmButton, Empty, ErrorState, Loading } from '../../shared/ui/common';
import { Dialog } from '../../shared/ui/dialog';
import { Field } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';
import { IntegrationParameters, parameterDefaults } from '../workflow/integration-parameters';
import { validateConfig, type JsonValue } from '../integrations/schema';

export const stageConnectionsQuery = (scope: WorkspaceScope) =>
  queryOptions({
    queryKey: [...keys.workspace(scope), 'workflow-stage-integrations'],
    queryFn: ({ signal }) =>
      data(sdk.listWorkflowStageIntegrations({ ...command, path: path(scope), signal })),
  });
export const stageProvidersQuery = (userID: string) =>
  queryOptions({
    queryKey: [...keys.user(userID), 'workflow-stage-integration-providers'],
    queryFn: ({ signal }) =>
      data(sdk.listWorkflowStageIntegrationProviders({ ...command, signal })),
  });

const callbackErrors: Record<string, string> = {
  busy: 'The task is busy. Retry after its current work or interaction finishes.',
  execution_busy: 'The task is busy. Retry after its current work or interaction finishes.',
  stage_changed: 'The task has left the stage this callback was watching.',
  configuration_changed:
    'The workflow or connection changed. New events use the current configuration.',
  authorization_revoked:
    'The configuring administrator no longer has permission. Ask an administrator to review the connection and workflow.',
  callback_prerequisites:
    'The task cannot start this callback. Check its assigned agent and execution configuration.',
  verification_or_reconciliation_failed:
    'External review information could not be verified. Check repository access, published review links and provider connectivity.',
};

export function WorkflowStageIntegrationsPanel() {
  const { scope, permissions } = useApp();
  const items = useQuery(stageConnectionsQuery(scope));
  const [editor, setEditor] = useState<WorkflowStageIntegration | 'new'>();
  return (
    <section className="panel integration-panel">
      <div className="panel-heading">
        <div>
          <p className="eyebrow">WORKFLOW EVENTS</p>
          <h2>Workflow stage integrations</h2>
          <p className="muted">
            Let external review and merge events trigger configured stage callbacks.
          </p>
        </div>
        {permissions.workspaceAdmin && (
          <Button onClick={() => setEditor('new')}>Add stage connection</Button>
        )}
      </div>
      {items.isPending ? (
        <Loading />
      ) : items.isError ? (
        <ErrorState error={items.error} retry={() => void items.refetch()} />
      ) : items.data.length === 0 ? (
        <Empty title="No stage connections">
          Add a connection, then configure its actions in the workflow editor.
        </Empty>
      ) : (
        <div className="integration-grid">
          {items.data.map((item) => (
            <StageConnectionCard
              key={item.id}
              item={item}
              onEdit={() => setEditor(item)}
              refresh={() => items.refetch()}
            />
          ))}
        </div>
      )}
      {editor && (
        <StageConnectionEditor
          item={editor === 'new' ? undefined : editor}
          onClose={() => setEditor(undefined)}
          onSaved={async () => {
            setEditor(undefined);
            await items.refetch();
          }}
        />
      )}
    </section>
  );
}

function StageConnectionCard({
  item,
  onEdit,
  refresh,
}: {
  item: WorkflowStageIntegration;
  onEdit: () => void;
  refresh: () => Promise<unknown>;
}) {
  const { scope, permissions } = useApp();
  const action = useAction();
  const [showStatus, setShowStatus] = useState(false);
  const status = useQuery({
    queryKey: [...keys.workspace(scope), 'workflow-stage-integrations', item.id, 'status'],
    queryFn: ({ signal }) =>
      data(
        sdk.listWorkflowStageIntegrationStatus({
          ...command,
          path: { ...path(scope), connectionID: item.id },
          signal,
        }),
      ),
    enabled: showStatus,
    refetchInterval: showStatus ? 5000 : false,
  });
  const callbackURL = new URL(
    `/callbacks/workflow-stage-integrations/${encodeURIComponent(item.id)}`,
    window.location.origin,
  ).href;
  return (
    <article className="integration-card">
      <div className="integration-card-top">
        <div>
          <h3>{item.name}</h3>
          <p>{item.provider}</p>
        </div>
        <Badge>{item.enabled ? 'Enabled' : 'Disabled'}</Badge>
      </div>
      <Field
        label="Webhook callback URL"
        hint="Configure this URL at the provider with the same webhook secret. Use HTTPS and subscribe to the required review and merge events."
      >
        <input readOnly value={callbackURL} />
      </Field>
      <div className="integration-card-actions">
        {permissions.workspaceAdmin && (
          <>
            <Button size="sm" variant="outline" onClick={onEdit}>
              Edit connection
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={action.pending}
              onClick={() =>
                void action.run(
                  () =>
                    data(
                      sdk.validateWorkflowStageIntegration({
                        ...command,
                        path: { ...path(scope), connectionID: item.id },
                      }),
                    ),
                  'Connection verified.',
                )
              }
            >
              Validate connection
            </Button>
            <ConfirmButton
              title={`Delete ${item.name}?`}
              description="Remove references from workflow rules before deleting this connection."
              onConfirm={async () => {
                await data(
                  sdk.deleteWorkflowStageIntegration({
                    ...command,
                    path: { ...path(scope), connectionID: item.id },
                  }),
                );
                await refresh();
              }}
            />
          </>
        )}
        <Button size="sm" variant="ghost" onClick={() => setShowStatus(!showStatus)}>
          {showStatus ? 'Hide activity' : 'Show activity'}
        </Button>
      </div>
      {action.error !== undefined && <ErrorState error={action.error} />}{' '}
      {action.message && <p role="status">{action.message}</p>}
      {showStatus &&
        (status.isPending ? (
          <Loading />
        ) : status.isError ? (
          <ErrorState error={status.error} />
        ) : status.data.length === 0 ? (
          <p className="muted">No observations or callback effects yet.</p>
        ) : (
          <ul>
            {status.data.map((event) => (
              <li key={event.id}>
                <strong>{event.state}</strong> · Task {event.taskId} ·{' '}
                {event.actionId || event.ruleId}
                {event.error && (
                  <p role="status">
                    {callbackErrors[event.error] ??
                      'This callback needs attention. Review the connection and task configuration.'}
                  </p>
                )}
                {permissions.workspaceAdmin &&
                  event.state === 'blocked' &&
                  [
                    'busy',
                    'execution_busy',
                    'authorization_revoked',
                    'callback_prerequisites',
                  ].includes(event.error) && (
                    <Button
                      size="sm"
                      disabled={action.pending}
                      onClick={() =>
                        void action.run(async () => {
                          await data(
                            sdk.retryWorkflowStageIntegrationEvent({
                              ...command,
                              path: { ...path(scope), eventID: event.id },
                            }),
                          );
                          await status.refetch();
                        }, 'Retry requested.')
                      }
                    >
                      Retry callback
                    </Button>
                  )}
              </li>
            ))}
          </ul>
        ))}
    </article>
  );
}

function StageConnectionEditor({
  item,
  onClose,
  onSaved,
}: {
  item: WorkflowStageIntegration | undefined;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const { scope } = useApp();
  const providers = useQuery(stageProvidersQuery(scope.userID));
  const coding = useQuery(codingIntegrationsQuery(scope));
  const repositories = useQuery(repositoriesQuery(scope));
  const [provider, setProvider] = useState(item?.provider ?? '');
  const [name, setName] = useState(item?.name ?? '');
  const [enabled, setEnabled] = useState(item?.enabled ?? false);
  const [codingID, setCodingID] = useState(item?.codingIntegrationId ?? '');
  const [repositoryIDs, setRepositoryIDs] = useState(item?.repositoryIds ?? []);
  const [secret, setSecret] = useState('');
  const [config, setConfig] = useState<Record<string, JsonValue>>(
    structuredClone(item?.config ?? {}) as Record<string, JsonValue>,
  );
  const action = useAction();
  const selected = providers.data?.find((p) => p.provider === provider);
  return (
    <Dialog
      title={item ? 'Edit stage connection' : 'Add stage connection'}
      description="Credentials stay private. Saving does not trigger work or replay historical events."
      open
      onOpenChange={(open) => {
        if (!open && !action.pending) onClose();
      }}
    >
      {providers.isPending || coding.isPending || repositories.isPending ? (
        <Loading />
      ) : providers.isError || coding.isError || repositories.isError ? (
        <ErrorState error={providers.error ?? coding.error ?? repositories.error} />
      ) : (
        <form
          className="form"
          onSubmit={(event) => {
            event.preventDefault();
            void action.run(async () => {
              if (!selected) throw new Error('Choose an integration provider.');
              const body: WorkflowStageIntegrationInputWritable = {
                provider,
                name,
                enabled,
                codingIntegrationId: codingID,
                repositoryIds: repositoryIDs,
                config: validateConfig(selected.configSchema, config),
                version: item?.version ?? 0,
                ...(secret ? { webhookSecret: secret } : {}),
              };
              if (item)
                await data(
                  sdk.updateWorkflowStageIntegration({
                    ...command,
                    path: { ...path(scope), connectionID: item.id },
                    body,
                  }),
                );
              else
                await data(
                  sdk.createWorkflowStageIntegration({ ...command, path: path(scope), body }),
                );
              setSecret('');
              await onSaved();
            });
          }}
        >
          <Field label="Connection name">
            <input
              required
              maxLength={200}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <Field label="Stage integration provider">
            <select
              required
              disabled={!!item}
              value={provider}
              onChange={(event) => {
                setProvider(event.target.value);
                setConfig(
                  parameterDefaults(
                    providers.data.find((p) => p.provider === event.target.value)?.configSchema ??
                      {},
                  ),
                );
              }}
            >
              <option value="">Choose provider</option>
              {item && !selected && (
                <option value={item.provider}>Unavailable provider ({item.provider})</option>
              )}
              {providers.data.map((p) => (
                <option key={p.provider} value={p.provider}>
                  {p.provider}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Coding credentials">
            <select
              required
              value={codingID}
              onChange={(event) => {
                setCodingID(event.target.value);
                setRepositoryIDs([]);
              }}
            >
              <option value="">Choose workspace coding connection</option>
              {coding.data.map((connection) => (
                <option key={connection.id} value={connection.id}>
                  {connection.name} ({connection.providerName})
                </option>
              ))}
            </select>
          </Field>
          <Field
            label="Allowed repositories"
            hint="Only selected registered repositories can trigger task callbacks."
          >
            <select
              multiple
              required
              value={repositoryIDs}
              onChange={(event) =>
                setRepositoryIDs(Array.from(event.target.selectedOptions, (option) => option.value))
              }
            >
              {repositories.data
                .filter((repository) => repository.integrationId === codingID)
                .map((repository) => (
                  <option key={repository.id} value={repository.id}>
                    {repository.name || repository.url}
                  </option>
                ))}
            </select>
          </Field>
          {selected && (
            <IntegrationParameters
              schema={selected.configSchema}
              value={config}
              onChange={setConfig}
              disabled={action.pending}
            />
          )}
          <Field
            label="Webhook secret"
            hint={
              item
                ? 'Leave empty to preserve the stored secret.'
                : 'Use the same high-entropy secret when configuring the provider webhook.'
            }
          >
            <input
              type="password"
              autoComplete="new-password"
              required={!item}
              minLength={32}
              value={secret}
              onChange={(event) => setSecret(event.target.value)}
            />
          </Field>
          <Field label="Enable connection">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(event) => setEnabled(event.target.checked)}
            />
          </Field>
          {action.error !== undefined && <ErrorState error={action.error} />}
          <div className="form-actions">
            <Button variant="outline" disabled={action.pending} onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={action.pending}>
              {action.pending ? 'Saving…' : 'Save stage connection'}
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
