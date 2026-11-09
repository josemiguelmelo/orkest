import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type {
  TaskTriggerIntegration,
  TaskTriggerProvider,
  TaskTriggerRule,
} from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { keys, path, tasksQuery, workflowsQuery } from '../../shared/api/queries';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Field } from '../../shared/ui/form';
import {
  ConfirmButton,
  CreateButton,
  ErrorState,
  Loading,
  PageHeader,
} from '../../shared/ui/common';
import { providerFields, validateConfig } from './schema';

export function TriggerIntegrations() {
  const { scope, permissions } = useApp();
  const [editing, setEditing] = useState<TaskTriggerIntegration | null | undefined>();
  const providers = useQuery({
    queryKey: ['task-trigger-providers', scope.userID],
    queryFn: () => data(sdk.listTaskTriggerIntegrationProviders(command)),
  });
  const integrations = useQuery({
    queryKey: [...keys.workspace(scope), 'task-trigger-integrations'],
    queryFn: () => data(sdk.listTaskTriggerIntegrations({ ...command, path: path(scope) })),
  });
  return (
    <section>
      <PageHeader
        title="Trigger integrations"
        description="Run configured actions for linked tasks when external systems send webhook events."
      >
        {permissions.workspaceAdmin && providers.data?.length ? (
          <CreateButton onClick={() => setEditing(null)}>Connect trigger</CreateButton>
        ) : null}
      </PageHeader>
      {integrations.isPending || providers.isPending ? (
        <Loading />
      ) : integrations.isError || providers.isError ? (
        <ErrorState error={integrations.error ?? providers.error} />
      ) : (
        <div className="grid">
          {!integrations.data.length && (
            <p className="muted">No trigger integrations configured.</p>
          )}
          {integrations.data.map((item) => (
            <TriggerCard
              key={item.id}
              item={item}
              onEdit={() => setEditing(item)}
              onRefresh={() => {
                void integrations.refetch();
              }}
            />
          ))}
        </div>
      )}
      {editing !== undefined && providers.data && (
        <TriggerEditor
          key={editing?.id ?? 'new'}
          item={editing}
          providers={providers.data}
          onClose={() => setEditing(undefined)}
          onSaved={() => {
            setEditing(undefined);
            void integrations.refetch();
          }}
        />
      )}
    </section>
  );
}

function TriggerEditor({
  item,
  providers,
  onClose,
  onSaved,
}: {
  item: TaskTriggerIntegration | null;
  providers: TaskTriggerProvider[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const { scope } = useApp();
  const workflows = useQuery(workflowsQuery(scope));
  const [providerID, setProviderID] = useState(item?.provider ?? providers[0]?.id ?? '');
  const [name, setName] = useState(item?.name ?? '');
  const [enabled, setEnabled] = useState(item?.enabled ?? true);
  const [secret, setSecret] = useState('');
  const [config, setConfig] = useState<Record<string, unknown>>(item?.config ?? {});
  const [rules, setRules] = useState<TaskTriggerRule[]>(item?.rules ?? []);
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const provider = providers.find((p) => p.id === providerID);
  let fields;
  try {
    fields = provider ? providerFields(provider.configSchema) : undefined;
  } catch (cause) {
    return (
      <Dialog
        title="Trigger integration"
        open
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
      >
        <ErrorState error={cause} />
      </Dialog>
    );
  }
  function updateRule(index: number, values: Partial<TaskTriggerRule>) {
    setRules((current) => current.map((r, i) => (i === index ? { ...r, ...values } : r)));
  }
  return (
    <Dialog
      title={item ? 'Edit trigger integration' : 'Connect trigger integration'}
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <form
        className="form"
        onSubmit={async (event) => {
          event.preventDefault();
          setBusy(true);
          setError(undefined);
          try {
            if (!provider) throw new Error('Choose a provider.');
            const body = {
              name,
              enabled,
              provider: providerID,
              config: validateConfig(provider.configSchema, config),
              rules,
              ...(item ? { version: item.version } : {}),
              ...(secret ? { webhookSecret: secret } : {}),
            };
            if (item)
              await sdk.updateTaskTriggerIntegration({
                ...command,
                path: { ...path(scope), integrationID: item.id },
                body,
              });
            else await sdk.createTaskTriggerIntegration({ ...command, path: path(scope), body });
            onSaved();
          } catch (cause) {
            setError(cause);
          } finally {
            setBusy(false);
          }
        }}
      >
        <Field label="Name">
          <input required maxLength={200} value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Provider">
          <select
            disabled={!!item}
            value={providerID}
            onChange={(e) => {
              setProviderID(e.target.value);
              setConfig({});
              setRules([]);
            }}
          >
            {providers.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </Field>
        {fields &&
          Object.entries(fields.properties).map(([key, field]) => (
            <Field key={key} label={field.title ?? key} hint={field.description ?? ''}>
              {field.type === 'array' ? (
                <textarea
                  value={Array.isArray(config[key]) ? config[key].join('\n') : ''}
                  onChange={(e) =>
                    setConfig({ ...config, [key]: e.target.value.split('\n').filter(Boolean) })
                  }
                />
              ) : (
                <input
                  required={fields.required?.includes(key)}
                  value={typeof config[key] === 'string' ? config[key] : ''}
                  onChange={(e) => setConfig({ ...config, [key]: e.target.value })}
                />
              )}
            </Field>
          ))}
        <Field
          label="Webhook secret"
          hint={
            item
              ? 'Leave empty to retain the current secret.'
              : 'Use the same random secret in your provider webhook settings (at least 32 characters).'
          }
        >
          <input
            type="password"
            autoComplete="new-password"
            required={!item}
            minLength={32}
            maxLength={4096}
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
          />
        </Field>
        <Field label="Enabled">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        </Field>
        <h3>Event rules</h3>
        {rules.map((rule, index) => {
          const workflow = workflows.data?.find((w) => w.id === rule.workflowId);
          return (
            <fieldset key={rule.id} className="form">
              <legend>Rule {index + 1}</legend>
              <Field label="Event">
                <select
                  required
                  value={rule.event}
                  onChange={(e) => updateRule(index, { event: e.target.value })}
                >
                  {provider?.events.map((e) => (
                    <option key={e} value={e}>
                      {e}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Workflow">
                <select
                  required
                  value={rule.workflowId}
                  onChange={(e) =>
                    updateRule(index, {
                      workflowId: e.target.value,
                      sourceStageId: '',
                      targetStageId: '',
                    })
                  }
                >
                  <option value="">Choose workflow</option>
                  {workflows.data?.map((w) => (
                    <option key={w.id} value={w.id}>
                      {w.name}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="From stage">
                <select
                  required
                  value={rule.sourceStageId}
                  onChange={(e) => updateRule(index, { sourceStageId: e.target.value })}
                >
                  <option value="">Choose stage</option>
                  {workflow?.stages
                    .filter((s) => s.id !== workflow.closedStageId)
                    .map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name}
                      </option>
                    ))}
                </select>
              </Field>
              <Field label="Action">
                <select
                  value={rule.action ?? 'move_task'}
                  onChange={(e) =>
                    updateRule(index, {
                      action: e.target.value as TaskTriggerRule['action'],
                      targetStageId:
                        e.target.value === 'start_work' ? rule.sourceStageId : '',
                    })
                  }
                >
                  <option value="move_task">Move task</option>
                  <option value="start_work">Start assigned agent in this stage</option>
                </select>
              </Field>
              {(rule.action ?? 'move_task') === 'move_task' ? (
                <Field label="To stage">
                  <select
                    required
                    value={rule.targetStageId}
                    onChange={(e) => updateRule(index, { targetStageId: e.target.value })}
                  >
                    <option value="">Choose stage</option>
                    {workflow?.stages
                      .filter((s) => s.id !== rule.sourceStageId)
                      .map((s) => (
                        <option key={s.id} value={s.id}>
                          {s.name}
                        </option>
                      ))}
                  </select>
                </Field>
              ) : (
                <Field
                  label="Custom agent prompt"
                  hint="The prompt includes the PR link. The assigned agent can use the authenticated GitHub CLI to fetch review comments."
                >
                  <textarea
                    maxLength={32768}
                    value={rule.promptText ?? ''}
                    onChange={(e) => updateRule(index, { promptText: e.target.value })}
                  />
                </Field>
              )}
              <Button
                type="button"
                variant="outline"
                onClick={() => setRules(rules.filter((_, i) => i !== index))}
              >
                Remove rule
              </Button>
            </fieldset>
          );
        })}
        <Button
          type="button"
          variant="outline"
          onClick={() =>
            setRules([
              ...rules,
              {
                id: crypto.randomUUID(),
                event: provider?.events[0] ?? '',
                workflowId: '',
                sourceStageId: '',
                targetStageId: '',
                action: 'move_task',
                promptText: '',
              },
            ])
          }
        >
          Add rule
        </Button>
        {workflows.isError && <ErrorState error={workflows.error} />}
        {error != null && <ErrorState error={error} />}
        <div className="form-actions">
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={busy}>
            Save trigger
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

function TriggerCard({
  item,
  onEdit,
  onRefresh,
}: {
  item: TaskTriggerIntegration;
  onEdit: () => void;
  onRefresh: () => void;
}) {
  const { scope, permissions } = useApp();
  const apiPath = { ...path(scope), integrationID: item.id };
  const [expanded, setExpanded] = useState(false);
  const [taskID, setTaskID] = useState('');
  const [repositoryID, setRepositoryID] = useState('');
  const [resourceID, setResourceID] = useState('');
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const tasks = useQuery(tasksQuery(scope));
  const links = useQuery({
    queryKey: [...keys.workspace(scope), 'trigger-links', item.id],
    queryFn: () => data(sdk.listTaskTriggerResourceLinks({ ...command, path: apiPath })),
    enabled: expanded,
  });
  const deliveries = useQuery({
    queryKey: [...keys.workspace(scope), 'trigger-deliveries', item.id],
    queryFn: () => data(sdk.listTaskTriggerDeliveries({ ...command, path: apiPath })),
    enabled: expanded,
    refetchInterval: expanded ? 5000 : false,
  });
  async function act(action: () => Promise<unknown>) {
    setBusy(true);
    setError(undefined);
    try {
      await action();
      await links.refetch();
      onRefresh();
    } catch (cause) {
      setError(cause);
    } finally {
      setBusy(false);
    }
  }
  return (
    <article className="card">
      <h3>{item.name}</h3>
      <p>
        {item.provider} · {item.enabled ? 'Enabled' : 'Disabled'} · {item.rules.length} rules
      </p>
      <Field label="Webhook URL">
        <input
          readOnly
          value={`${window.location.origin}/webhooks/task-trigger-integrations/${item.id}`}
        />
      </Field>
      <div className="form-actions">
        <Button variant="outline" onClick={() => setExpanded(!expanded)}>
          {expanded ? 'Hide activity' : 'Links and activity'}
        </Button>
        {permissions.workspaceAdmin && (
          <>
            <Button variant="outline" onClick={onEdit}>
              Edit
            </Button>
            <ConfirmButton
              label="Delete"
              title="Delete trigger integration?"
              description="This removes the integration, links, and delivery history."
              onConfirm={() =>
                act(() => sdk.deleteTaskTriggerIntegration({ ...command, path: apiPath }))
              }
            />
          </>
        )}
      </div>
      {error != null && <ErrorState error={error} />}
      {expanded && (
        <>
          <h4>Linked pull requests</h4>
          {links.isError ? (
            <ErrorState error={links.error} />
          ) : (
            links.data?.map((link) => (
              <div key={`${link.repositoryId}:${link.resourceId}`}>
                <span>
                  {tasks.data?.find((t) => t.id === link.taskId)?.title ?? link.taskId} · Repository{' '}
                  {link.repositoryId} · PR {link.resourceId}
                </span>
                {permissions.workspaceAdmin && (
                  <Button
                    variant="outline"
                    disabled={busy}
                    onClick={() =>
                      void act(() =>
                        sdk.unlinkTaskTriggerResource({ ...command, path: apiPath, body: link }),
                      )
                    }
                  >
                    Unlink
                  </Button>
                )}
              </div>
            ))
          )}
          {permissions.workspaceAdmin && (
            <form
              className="form"
              onSubmit={(event) => {
                event.preventDefault();
                void act(() =>
                  sdk.linkTaskTriggerResource({
                    ...command,
                    path: apiPath,
                    body: { taskId: taskID, repositoryId: repositoryID, resourceId: resourceID },
                  }),
                );
              }}
            >
              <Field label="Task">
                <select required value={taskID} onChange={(e) => setTaskID(e.target.value)}>
                  <option value="">Choose task</option>
                  {tasks.data?.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.title}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="External repository ID">
                <input
                  required
                  value={repositoryID}
                  onChange={(e) => setRepositoryID(e.target.value)}
                />
              </Field>
              <Field label="Pull request number">
                <input
                  required
                  value={resourceID}
                  onChange={(e) => setResourceID(e.target.value)}
                />
              </Field>
              <p className="muted">
                The task must select the registered repository. Link before submitting a review or
                merging.
              </p>
              <Button type="submit" disabled={busy}>
                Link pull request
              </Button>
            </form>
          )}
          <h4>Recent deliveries</h4>
          {item.discoveryDiagnostic ? <p role="status">PR discovery: {item.discoveryDiagnostic}</p> : null}
          {deliveries.isError ? (
            <ErrorState error={deliveries.error} />
          ) : (
            <ul>
              {deliveries.data?.map((d) => (
                <li key={d.id}>
                  {d.event.type} · PR {d.event.resourceId} · {d.status}
                  {d.diagnostic ? ` · ${d.diagnostic}` : null}
                </li>
              ))}
            </ul>
          )}
          <p className="muted">
            Blocked events require intervention. They never interrupt active work or retry
            execution.
          </p>
        </>
      )}
    </article>
  );
}
