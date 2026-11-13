import { useId, useState } from 'react';
import { Check, Copy, Search } from 'lucide-react';
import { useQuery, queryOptions } from '@tanstack/react-query';
import './workflow-stage-integrations.css';
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
import { ConfirmButton, ErrorState, Loading } from '../../shared/ui/common';
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

function providerLabel(provider: string) {
  if (provider === 'github_pr') return 'GitHub pull requests';
  return provider
    .split(/[_-]/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

function activityLabel(value: string) {
  const labels: Record<string, string> = {
    observing: 'Watching pull requests',
    pending: 'Waiting to process',
    processed: 'Processed',
    blocked: 'Needs attention',
    queued: 'Queued',
    running: 'Running',
    completed: 'Callback completed',
    failed: 'Callback failed',
  };
  return labels[value] ?? value.replaceAll('_', ' ');
}

function integrationActionLabel(value: string) {
  const labels: Record<string, string> = {
    review_added: 'Review added',
    pr_merged: 'PR merged',
  };
  return labels[value] ?? value.replaceAll('_', ' ');
}

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
        <div className="stage-integration-onboarding">
          <div>
            <p className="stage-integration-kicker">GET STARTED</p>
            <h3>Connect a code host to your workflow</h3>
            <p className="muted">
              A connection verifies linked pull requests and listens for review or merge activity.
            </p>
          </div>
          <ol className="stage-integration-steps">
            <li>
              <span>1</span>
              <div>
                <strong>Create a connection</strong>
                <small>Choose credentials and the repositories Orkest can observe.</small>
              </div>
            </li>
            <li>
              <span>2</span>
              <div>
                <strong>Configure the webhook</strong>
                <small>Send provider events to the callback URL shown after setup.</small>
              </div>
            </li>
            <li>
              <span>3</span>
              <div>
                <strong>Attach an action to a stage</strong>
                <small>Choose a PR artifact input and what Orkest should do.</small>
              </div>
            </li>
          </ol>
          {permissions.workspaceAdmin && (
            <Button onClick={() => setEditor('new')}>Create your first connection</Button>
          )}
        </div>
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
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState(false);
  const callbackURLID = useId();
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
        <div className="integration-card-title">
          <h3>{item.name}</h3>
          <p>
            {providerLabel(item.provider)} · {item.repositoryIds.length}{' '}
            {item.repositoryIds.length === 1 ? 'repository' : 'repositories'}
          </p>
        </div>
        <span className={`badge stage-connection-badge ${item.enabled ? 'is-enabled' : ''}`}>
          <span className={`stage-connection-dot ${item.enabled ? 'is-enabled' : ''}`} />
          {item.enabled ? 'Enabled' : 'Paused'}
        </span>
      </div>
      <div className="stage-connection-events">
        <strong>Webhook setup</strong>
        {item.provider === 'github_pr' ? (
          <>
            <span>Pull requests · closed</span>
            <span>Pull request reviews · submitted</span>
            <small>Only merged pull requests trigger the merge action.</small>
          </>
        ) : (
          <span>Subscribe to the events required by {providerLabel(item.provider)} actions.</span>
        )}
      </div>
      <div className="field">
        <label htmlFor={callbackURLID}>Webhook callback URL</label>
        <div className="stage-callback-url">
          <input
            id={callbackURLID}
            readOnly
            value={callbackURL}
            onFocus={(event) => event.currentTarget.select()}
          />
          <Button
            size="sm"
            variant="outline"
            aria-label="Copy webhook callback URL"
            onClick={() => {
              setCopyError(false);
              if (!navigator.clipboard?.writeText) {
                setCopyError(true);
                return;
              }
              void navigator.clipboard.writeText(callbackURL).then(
                () => {
                  setCopied(true);
                  window.setTimeout(() => setCopied(false), 1800);
                },
                () => setCopyError(true),
              );
            }}
          >
            {copied ? <Check size={14} /> : <Copy size={14} />}
            {copied ? 'Copied' : 'Copy URL'}
          </Button>
        </div>
        <small className="muted">
          Add this URL to your provider webhook settings. The secret is stored privately.
        </small>
        {copyError && (
          <small role="status">Copy was unavailable. Select and copy the URL manually.</small>
        )}
      </div>
      <p className="stage-connection-scope">
        Events are accepted only for the {item.repositoryIds.length} selected{' '}
        {item.repositoryIds.length === 1 ? 'repository' : 'repositories'}.
      </p>
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
              Check repository access
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
              <li className="stage-activity-row" key={event.id}>
                <div className="stage-activity-summary">
                  <strong>{activityLabel(event.state)}</strong>
                  <span>·</span>
                  <a
                    href={`/organizations/${encodeURIComponent(scope.orgID)}/workspaces/${encodeURIComponent(scope.workspaceID)}/tasks/${encodeURIComponent(event.taskId)}`}
                  >
                    Task {event.taskId.slice(0, 8)}
                  </a>
                  <span>·</span>
                  <span>{integrationActionLabel(event.actionId || event.ruleId)}</span>
                </div>
                {Number.isFinite(Date.parse(event.createdAt)) && (
                  <time dateTime={event.createdAt}>
                    {new Date(event.createdAt).toLocaleString()}
                  </time>
                )}
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
  const [repositorySearch, setRepositorySearch] = useState('');
  const [secret, setSecret] = useState('');
  const [showSecret, setShowSecret] = useState(false);
  const [config, setConfig] = useState<Record<string, JsonValue>>(
    structuredClone(item?.config ?? {}) as Record<string, JsonValue>,
  );
  const action = useAction();
  const selected = providers.data?.find((p) => p.provider === provider);
  const availableRepositories = (repositories.data ?? []).filter(
    (repository) => repository.integrationId === codingID,
  );
  const visibleRepositories = availableRepositories.filter((repository) =>
    `${repository.name} ${repository.url}`.toLowerCase().includes(repositorySearch.toLowerCase()),
  );
  const unavailableRepositoryCount = repositoryIDs.filter(
    (id) => !availableRepositories.some((repository) => repository.id === id),
  ).length;
  const providerProperties = selected?.configSchema.properties;
  const hasProviderSettings =
    providerProperties !== null &&
    typeof providerProperties === 'object' &&
    Object.keys(providerProperties).length > 0;
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
          className="form stage-integration-form"
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
          <section className="stage-integration-form-section">
            <div className="stage-integration-section-heading">
              <span>1</span>
              <div>
                <h3>Connection details</h3>
                <p>Choose a provider and name this connection.</p>
              </div>
            </div>
            <Field label="Connection name">
              <input
                required
                maxLength={200}
                value={name}
                disabled={action.pending}
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <Field label="Provider">
              <select
                required
                disabled={!!item || action.pending}
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
                  <option value={item.provider}>
                    Unavailable provider ({providerLabel(item.provider)})
                  </option>
                )}
                {providers.data.map((p) => (
                  <option key={p.provider} value={p.provider}>
                    {providerLabel(p.provider)}
                  </option>
                ))}
              </select>
            </Field>
          </section>
          <section className="stage-integration-form-section">
            <div className="stage-integration-section-heading">
              <span>2</span>
              <div>
                <h3>Credentials and repositories</h3>
                <p>Limit this connection to repositories it should observe.</p>
              </div>
            </div>
            {coding.data.length === 0 ? (
              <div className="stage-integration-inline-notice" role="status">
                <strong>No coding credentials are set up yet.</strong>
                <span>Add a coding integration in Workspace settings, then return here.</span>
              </div>
            ) : (
              <Field
                label="Coding credentials"
                hint="Uses the selected workspace connection to verify linked pull requests."
              >
                <select
                  required
                  value={codingID}
                  disabled={action.pending}
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
            )}
            <fieldset className="stage-repository-picker" disabled={!codingID || action.pending}>
              <legend>
                Allowed repositories <span>{repositoryIDs.length} selected</span>
              </legend>
              <p className="muted">
                Only selected registered repositories can trigger task callbacks.
              </p>
              <div className="stage-repository-tools">
                <label className="stage-repository-search">
                  <Search size={15} aria-hidden="true" />
                  <input
                    type="search"
                    aria-label="Search repositories"
                    placeholder="Search repositories"
                    value={repositorySearch}
                    onChange={(event) => setRepositorySearch(event.target.value)}
                  />
                </label>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  disabled={!visibleRepositories.length}
                  onClick={() =>
                    setRepositoryIDs([
                      ...new Set([
                        ...repositoryIDs,
                        ...visibleRepositories.map((repository) => repository.id),
                      ]),
                    ])
                  }
                >
                  Select visible
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  disabled={!repositoryIDs.length}
                  onClick={() => setRepositoryIDs([])}
                >
                  Clear
                </Button>
              </div>
              <div className="stage-repository-list">
                {!codingID ? (
                  <p className="stage-repository-empty">
                    Choose coding credentials to see repositories.
                  </p>
                ) : visibleRepositories.length === 0 ? (
                  <p className="stage-repository-empty">
                    {availableRepositories.length
                      ? 'No repositories match this search.'
                      : 'No registered repositories found for these credentials.'}
                  </p>
                ) : (
                  visibleRepositories.map((repository) => (
                    <label className="stage-repository-row" key={repository.id}>
                      <input
                        type="checkbox"
                        checked={repositoryIDs.includes(repository.id)}
                        onChange={(event) =>
                          setRepositoryIDs(
                            event.target.checked
                              ? [...repositoryIDs, repository.id]
                              : repositoryIDs.filter((id) => id !== repository.id),
                          )
                        }
                      />
                      <span>
                        <strong>{repository.name || repository.url}</strong>
                        <small>{repository.url}</small>
                      </span>
                    </label>
                  ))
                )}
              </div>
              {codingID && unavailableRepositoryCount > 0 && (
                <p className="stage-integration-inline-notice" role="status">
                  {unavailableRepositoryCount} saved{' '}
                  {unavailableRepositoryCount === 1 ? 'repository is' : 'repositories are'} no
                  longer available. Clear the selection or choose another repository.
                </p>
              )}
            </fieldset>
          </section>
          {selected && hasProviderSettings && (
            <section className="stage-integration-form-section">
              <div className="stage-integration-section-heading">
                <span>3</span>
                <div>
                  <h3>Provider settings</h3>
                  <p>Set any provider-specific options.</p>
                </div>
              </div>
              <IntegrationParameters
                schema={selected.configSchema}
                value={config}
                onChange={setConfig}
                disabled={action.pending}
              />
            </section>
          )}
          <section className="stage-integration-form-section">
            <div className="stage-integration-section-heading">
              <span>{hasProviderSettings ? '4' : '3'}</span>
              <div>
                <h3>Webhook and activation</h3>
                <p>Paste the same secret into your provider webhook settings after saving.</p>
              </div>
            </div>
            <Field
              label="Webhook secret"
              hint={
                item
                  ? 'Leave empty to keep the saved secret. Enter a new value only when rotating it.'
                  : 'Use a high-entropy value, at least 32 characters long, or generate one below.'
              }
            >
              <input
                type={showSecret ? 'text' : 'password'}
                autoComplete="new-password"
                required={!item}
                minLength={32}
                value={secret}
                disabled={action.pending}
                onChange={(event) => setSecret(event.target.value)}
              />
            </Field>
            <div className="stage-secret-actions">
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={action.pending}
                onClick={() => {
                  const bytes = window.crypto.getRandomValues(new Uint8Array(32));
                  setSecret(
                    Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join(''),
                  );
                  setShowSecret(true);
                }}
              >
                Generate secret
              </Button>
              {secret && (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setShowSecret(!showSecret)}
                >
                  {showSecret ? 'Hide secret' : 'Show secret'}
                </Button>
              )}
            </div>
            <label className="stage-connection-enable">
              <input
                type="checkbox"
                checked={enabled}
                disabled={action.pending}
                onChange={(event) => setEnabled(event.target.checked)}
              />
              <span>
                <strong>Enable this connection</strong>
                <small>Disabled connections do not accept callbacks or run observations.</small>
              </span>
            </label>
          </section>
          {action.error !== undefined && <ErrorState error={action.error} />}
          <div className="form-actions">
            <small className="muted stage-save-hint">
              {item
                ? 'Choose credentials and at least one repository. Leave the secret blank to keep it unchanged.'
                : 'Choose a provider, credentials and at least one repository, then enter or generate a 32+ character secret.'}
            </small>
            <Button type="button" variant="outline" disabled={action.pending} onClick={onClose}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={
                action.pending ||
                !selected ||
                !codingID ||
                repositoryIDs.length === 0 ||
                unavailableRepositoryCount > 0 ||
                !name.trim() ||
                (!item && secret.length < 32)
              }
            >
              {action.pending ? 'Saving…' : 'Save stage connection'}
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
