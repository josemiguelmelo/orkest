import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Bot, Brain, Cpu, Eye, FileText, Sparkles, Wrench, X } from 'lucide-react';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { Agent } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { agentsQuery, mcpQuery, path, skillsQuery, useRefresh } from '../../shared/api/queries';
import {
  Badge,
  ConfirmButton,
  CreateButton,
  Empty,
  ErrorState,
  Loading,
  PageHeader,
} from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Field, Form, Selection } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';
const schema = z.object({
  name: z.string().trim().min(1).max(200),
  description: z.string().max(2000),
  runtime: z.literal('opencode'),
  provider: z.string().trim().min(1).max(200),
  model: z.string().trim().min(1).max(200),
  systemPrompt: z.string().max(100000),
  mcpServerIds: z.array(z.string()),
  skillIds: z.array(z.string()),
});
type ModelCatalog = Record<string, { name?: string; models?: Record<string, { name?: string }> }>;
const modelCatalogQuery = {
  queryKey: ['runtime-model-catalog', 'opencode'],
  queryFn: async (): Promise<ModelCatalog> => {
    const response = await fetch('https://models.dev/api.json');
    if (!response.ok) throw new Error('Model catalog is temporarily unavailable.');
    return (await response.json()) as ModelCatalog;
  },
  staleTime: 60 * 60 * 1000,
};
export function AgentsPage() {
  const { scope, permissions } = useApp();
  const agents = useQuery(agentsQuery(scope));
  const refresh = useRefresh();
  const [editing, setEditing] = useState<Agent | null | undefined>();
  const [viewing, setViewing] = useState<Agent>();
  return (
    <>
      <PageHeader
        title="Agents"
        description="Build intelligent agent definitions with custom models, tools, skills, and instructions."
      >
        {permissions.editWorkspace && (
          <CreateButton onClick={() => setEditing(null)}>New agent</CreateButton>
        )}
      </PageHeader>
      <div className="agents-hint">
        <Sparkles size={14} />
        <span>Reusable agents power your workspace tasks with specialized capabilities</span>
      </div>
      {agents.isPending ? (
        <Loading />
      ) : agents.isError ? (
        <ErrorState error={agents.error} />
      ) : agents.data.length ? (
        <div className="agents-grid">
          {agents.data.map((item) => (
            <article className="agent-card" key={item.id}>
              <div className="agent-card-header">
                <div className="agent-card-icon">
                  <Bot size={26} />
                </div>
                <Badge>OpenCode</Badge>
              </div>
              <div className="agent-card-body">
                <h3>{item.name}</h3>
                <p className="agent-card-description">
                  {item.description || 'A configurable agent definition ready for your tasks.'}
                </p>
                <div className="agent-card-model">
                  <Cpu size={14} />
                  <span>
                    {item.provider} / {item.model}
                  </span>
                </div>
                <div className="agent-card-tags">
                  <span className="agent-tag">
                    <Wrench size={12} />
                    {item.skillIds.length} skills
                  </span>
                  <span className="agent-tag">
                    <Brain size={12} />
                    {item.mcpServerIds.length} MCP servers
                  </span>
                </div>
              </div>
              {permissions.editWorkspace ? (
                <div className="agent-card-actions">
                  <Button variant="outline" size="sm" onClick={() => setViewing(item)}>
                    <Eye size={14} />
                    View
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setEditing(item)}>
                    Edit
                  </Button>
                  <ConfirmButton
                    title="Delete agent?"
                    description="Task assignments to this definition will be cleared. Tasks are preserved."
                    onConfirm={async () => {
                      await data(
                        sdk.deleteAgent({ ...command, path: { ...path(scope), agentID: item.id } }),
                      );
                      await refresh(scope);
                    }}
                  />
                </div>
              ) : (
                <div className="agent-card-actions">
                  <Button variant="outline" size="sm" onClick={() => setViewing(item)}>
                    <FileText size={14} />
                    View instructions
                  </Button>
                </div>
              )}
            </article>
          ))}
        </div>
      ) : (
        <Empty title="No agent definitions yet">
          Create an agent with its model, instructions, skills, and MCP connections to get
          started.
        </Empty>
      )}
      {editing !== undefined && (
        <AgentEditor
          key={editing?.id ?? 'new'}
          item={editing}
          onClose={() => setEditing(undefined)}
        />
      )}
      {viewing && <AgentViewer item={viewing} onClose={() => setViewing(undefined)} />}
    </>
  );
}
function AgentEditor({ item, onClose }: { item: Agent | null; onClose: () => void }) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const servers = useQuery(mcpQuery(scope));
  const skills = useQuery(skillsQuery(scope));
  const catalog = useQuery(modelCatalogQuery);
  const [customModel, setCustomModel] = useState(false);
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      name: item?.name ?? '',
      description: item?.description ?? '',
      runtime: 'opencode' as const,
      provider: item?.provider ?? '',
      model: item?.model ?? '',
      systemPrompt: item?.systemPrompt ?? '',
      mcpServerIds: item?.mcpServerIds ?? [],
      skillIds: item?.skillIds ?? [],
    },
  });
  return (
    <Dialog
      title={item ? 'Edit agent definition' : 'New agent definition'}
      description="Definitions configure agents; they do not provide execution slots or start tasks."
      open
      onOpenChange={(v) => {
        if (!v && !action.pending) onClose();
      }}
    >
      <Form
        form={form}
        error={action.error}
        onSubmit={async (body) => {
          if (
            await action.run(async () => {
              if (item)
                await data(
                  sdk.updateAgent({ ...command, path: { ...path(scope), agentID: item.id }, body }),
                );
              else await data(sdk.createAgent({ ...command, path: path(scope), body }));
              await refresh(scope);
            })
          )
            onClose();
        }}
      >
        <Field label="Name" error={form.formState.errors.name?.message}>
          <input {...form.register('name')} />
        </Field>
        <Field label="Description" error={form.formState.errors.description?.message}>
          <textarea {...form.register('description')} />
        </Field>
        <Field label="Runtime">
          <select {...form.register('runtime')}>
            <option value="opencode">OpenCode</option>
          </select>
        </Field>
        {customModel ? (
          <div className="form-grid">
            <Field label="Model provider" error={form.formState.errors.provider?.message}>
              <input {...form.register('provider')} placeholder="Provider identifier" />
            </Field>
            <Field label="Model identifier" error={form.formState.errors.model?.message}>
              <input {...form.register('model')} />
            </Field>
          </div>
        ) : (
          <>
            <div className="form-grid">
              <Field label="Model provider" error={form.formState.errors.provider?.message}>
                <select
                  value={form.watch('provider')}
                  onChange={(event) => {
                    form.setValue('provider', event.target.value, { shouldDirty: true });
                    form.setValue('model', '', { shouldDirty: true });
                  }}
                >
                  <option value="">Select a provider</option>
                  {Object.entries(catalog.data ?? {}).map(([id, provider]) => (
                    <option value={id} key={id}>
                      {provider.name ?? id}
                    </option>
                  ))}
                  {form.watch('provider') && !catalog.data?.[form.watch('provider')] && (
                    <option value={form.watch('provider')}>
                      {form.watch('provider')} (current)
                    </option>
                  )}
                </select>
              </Field>
              <Field label="Model identifier" error={form.formState.errors.model?.message}>
                <select
                  value={form.watch('model')}
                  onChange={(event) =>
                    form.setValue('model', event.target.value, { shouldDirty: true })
                  }
                  disabled={!form.watch('provider')}
                >
                  <option value="">Select a model</option>
                  {Object.entries(catalog.data?.[form.watch('provider')]?.models ?? {}).map(
                    ([id, model]) => (
                      <option value={id} key={id}>
                        {model.name ?? id}
                      </option>
                    ),
                  )}
                  {form.watch('model') &&
                    !catalog.data?.[form.watch('provider')]?.models?.[form.watch('model')] && (
                      <option value={form.watch('model')}>{form.watch('model')} (current)</option>
                    )}
                </select>
              </Field>
            </div>
            {catalog.isError && (
              <p className="muted">Model catalog unavailable. You can enter a custom model.</p>
            )}
          </>
        )}
        <label className="muted">
          <input
            type="checkbox"
            checked={customModel}
            onChange={(event) => setCustomModel(event.target.checked)}
          />
          Enter a custom provider and model
        </label>
        <Field label="System instructions" error={form.formState.errors.systemPrompt?.message}>
          <textarea rows={8} className="code-input" {...form.register('systemPrompt')} />
        </Field>
        {servers.isError ? (
          <ErrorState error={servers.error} />
        ) : (
          <Selection
            label="MCP servers"
            options={servers.data ?? []}
            value={form.watch('mcpServerIds')}
            onChange={(ids) => form.setValue('mcpServerIds', ids, { shouldDirty: true })}
          />
        )}{' '}
        {skills.isError ? (
          <ErrorState error={skills.error} />
        ) : (
          <Selection
            label="Skills visible to this workspace"
            options={
              skills.data?.map((skill) => ({
                id: skill.id,
                name: `${skill.id} (${skill.scope})`,
              })) ?? []
            }
            value={form.watch('skillIds')}
            onChange={(ids) => form.setValue('skillIds', ids, { shouldDirty: true })}
          />
        )}
      </Form>
    </Dialog>
  );
}

function AgentViewer({ item, onClose }: { item: Agent; onClose: () => void }) {
  return (
    <Dialog
      title={item.name}
      description={item.description || 'Agent definition details'}
      open
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
    >
      <div className="agent-viewer">
        <div className="agent-viewer-section">
          <div className="agent-viewer-header">
            <Bot size={18} />
            <span>Runtime & Model</span>
          </div>
          <div className="agent-viewer-content">
            <div className="agent-viewer-row">
              <span className="agent-viewer-label">Runtime</span>
              <Badge>OpenCode</Badge>
            </div>
            <div className="agent-viewer-row">
              <span className="agent-viewer-label">Provider / Model</span>
              <span>
                {item.provider} / {item.model}
              </span>
            </div>
          </div>
        </div>
        <div className="agent-viewer-section">
          <div className="agent-viewer-header">
            <Wrench size={18} />
            <span>Capabilities</span>
          </div>
          <div className="agent-viewer-content">
            <div className="agent-viewer-row">
              <span className="agent-viewer-label">Skills</span>
              <span>{item.skillIds.length} skills attached</span>
            </div>
            <div className="agent-viewer-row">
              <span className="agent-viewer-label">MCP Servers</span>
              <span>{item.mcpServerIds.length} servers connected</span>
            </div>
          </div>
        </div>
        <div className="agent-viewer-section">
          <div className="agent-viewer-header">
            <FileText size={18} />
            <span>System Instructions</span>
          </div>
          <div className="agent-viewer-instructions">
            {item.systemPrompt ? (
              <pre>{item.systemPrompt}</pre>
            ) : (
              <p className="muted">No system instructions configured.</p>
            )}
          </div>
        </div>
      </div>
      <div className="form-actions">
        <Button variant="outline" onClick={onClose}>
          <X size={14} />
          Close
        </Button>
      </div>
    </Dialog>
  );
}
