import { useState, type ChangeEvent } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Bot, Brain, Cpu, Eye, FileText, Sparkles, Wrench, X } from 'lucide-react';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { Agent } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import {
  agentsQuery,
  profilesQuery,
  agentRuntimeProfilesQuery,
  mcpQuery,
  path,
  skillsQuery,
  useRefresh,
} from '../../shared/api/queries';
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
const schema = z
  .object({
    name: z.string().trim().min(1).max(200),
    description: z.string().max(2000),
    runtime: z.enum(['opencode', 'claude-code', 'codex', 'github-copilot', 'kiro']),
    provider: z.string().trim().min(1).max(200),
    model: z.string().trim().min(1).max(200),
    systemPrompt: z.string().max(100000),
    permissionPolicy: z.enum(['ask', 'autonomous']),
    mcpServerIds: z.array(z.string()),
    skillIds: z.array(z.string()),
    executionProfileId: z.string(),
    runtimeProfileId: z.string(),
  })
  .superRefine((value, context) => {
    if (value.runtime === 'claude-code' && value.provider.toLowerCase() !== 'anthropic') {
      context.addIssue({
        code: 'custom',
        path: ['provider'],
        message: 'Claude Code requires Anthropic.',
      });
    }
    if (value.runtime === 'codex' && value.provider.toLowerCase() !== 'openai') {
      context.addIssue({ code: 'custom', path: ['provider'], message: 'Codex requires OpenAI.' });
    }
    if (value.runtime === 'github-copilot' && value.provider.toLowerCase() !== 'github-copilot') {
      context.addIssue({ code: 'custom', path: ['provider'], message: 'GitHub Copilot requires the GitHub Copilot provider.' });
    }
    if (value.runtime === 'kiro' && value.provider.toLowerCase() !== 'kiro') {
      context.addIssue({ code: 'custom', path: ['provider'], message: 'Kiro requires the Kiro provider.' });
    }
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
function runtimeName(runtime: Agent['runtime']) {
  if (runtime === 'claude-code') return 'Claude Code';
  if (runtime === 'codex') return 'Codex';
  if (runtime === 'github-copilot') return 'GitHub Copilot';
  if (runtime === 'kiro') return 'Kiro';
  return 'OpenCode';
}
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
                <Badge>{runtimeName(item.runtime)}</Badge>
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
          Create an agent with its model, instructions, skills, and MCP connections to get started.
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
  const profiles = useQuery(profilesQuery(scope));
  const runtimeProfiles = useQuery(agentRuntimeProfilesQuery(scope));
  const skills = useQuery(skillsQuery(scope));
  const catalog = useQuery(modelCatalogQuery);
  const [customModel, setCustomModel] = useState(item?.runtime === 'github-copilot' || item?.runtime === 'kiro');
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      name: item?.name ?? '',
      description: item?.description ?? '',
      runtime: item?.runtime ?? 'opencode',
      provider: item?.runtime === 'claude-code' ? 'anthropic' : item?.runtime === 'codex' ? 'openai' : item?.runtime === 'github-copilot' ? 'github-copilot' : item?.runtime === 'kiro' ? 'kiro' : (item?.provider ?? ''),
      model: item?.model ?? '',
      systemPrompt: item?.systemPrompt ?? '',
      permissionPolicy: item?.permissionPolicy ?? 'ask',
      mcpServerIds: item?.mcpServerIds ?? [],
      skillIds: item?.skillIds ?? [],
      executionProfileId: item?.executionProfileId ?? '',
      runtimeProfileId: item?.runtimeProfileId ?? '',
    },
  });
  const runtimeField = form.register('runtime');
  return (
    <Dialog
      title={item ? 'Edit agent definition' : 'Create new agent'}
      description="Configure your agent's identity, model, tools, skills, and system instructions."
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
        <div className="agent-form-section">
          <div className="agent-form-section-header">
            <Bot size={16} />
            <span>Basic Information</span>
          </div>
          <div className="agent-form-section-content">
            <Field label="Name" error={form.formState.errors.name?.message}>
              <input {...form.register('name')} placeholder="e.g., Code Reviewer" />
            </Field>
            <Field label="Description" error={form.formState.errors.description?.message}>
              <textarea
                {...form.register('description')}
                placeholder="Describe this agent's purpose and capabilities..."
              />
            </Field>
          </div>
        </div>
        <div className="agent-form-section">
          <div className="agent-form-section-header">
            <Cpu size={16} />
            <span>Model Configuration</span>
          </div>
          <div className="agent-form-section-content">
            <Field label="Runtime profile">
              <select
                value={form.watch('runtimeProfileId')}
                onChange={(event) => {
                  const id = event.target.value;
                  form.setValue('runtimeProfileId', id, { shouldDirty: true });
                  const profile = runtimeProfiles.data?.find((p) => p.id === id);
                  if (profile) {
                    form.setValue('runtime', profile.runtime, { shouldDirty: true });
                    form.setValue('provider', profile.modelProvider, { shouldDirty: true });
                    form.setValue('model', profile.modelIdentifier, { shouldDirty: true });
                    setCustomModel(true);
                  }
                }}
              >
                <option value="">Configure runtime and model manually</option>
                {runtimeProfiles.data?.map((profile) => (
                  <option key={profile.id} value={profile.id}>
                    {profile.name} ({runtimeName(profile.runtime)} · {profile.modelProvider}/{profile.modelIdentifier})
                  </option>
                ))}
              </select>
              {runtimeProfiles.isError && <ErrorState error={runtimeProfiles.error} />}
              {runtimeProfiles.data?.length === 0 && <p className="muted">No runtime profiles yet. Create one in Runtime profiles.</p>}
            </Field>
            <Field label="Execution profile">
              <select
                {...form.register('executionProfileId')}
                onChange={(e) => {
                  form.setValue('executionProfileId', e.target.value, { shouldDirty: true });
                  const profile = profiles.data?.find((p) => p.id === e.target.value);
                  if (profile) {
                    form.setValue('runtime', profile.runtime, { shouldDirty: true });
                    if (profile.runtime === 'claude-code')
                      form.setValue('provider', 'anthropic', { shouldDirty: true });
                    if (profile.runtime === 'codex')
                      form.setValue('provider', 'openai', { shouldDirty: true });
                    if (profile.runtime === 'github-copilot' || profile.runtime === 'kiro') {
                      form.setValue('provider', profile.runtime, { shouldDirty: true });
                      setCustomModel(true);
                    } else {
                      setCustomModel(false);
                    }
                    if (profile.runtime !== 'opencode')
                      form.setValue('model', '', { shouldDirty: true });
                  }
                }}
              >
                <option value="">Default environment</option>
                {profiles.data?.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} ({p.runtime}, {p.status})
                  </option>
                ))}
              </select>
              {profiles.isError && <ErrorState error={profiles.error} />}
            </Field>
            <Field label="Runtime">
              <select
                {...runtimeField}
                disabled={!!form.watch('runtimeProfileId')}
                onChange={(event: ChangeEvent<HTMLSelectElement>) => {
                  void runtimeField.onChange(event);
                  if (event.currentTarget.value === 'claude-code') {
                    form.setValue('provider', 'anthropic', { shouldDirty: true });
                    form.setValue('model', '', { shouldDirty: true });
                  }
                  if (event.currentTarget.value === 'codex') {
                    form.setValue('provider', 'openai', { shouldDirty: true });
                    form.setValue('model', '', { shouldDirty: true });
                  }
                  if (event.currentTarget.value === 'github-copilot') {
                    form.setValue('provider', 'github-copilot', { shouldDirty: true });
                    form.setValue('model', '', { shouldDirty: true });
                    setCustomModel(true);
                  }
                  if (event.currentTarget.value === 'kiro') {
                    form.setValue('provider', 'kiro', { shouldDirty: true });
                    form.setValue('model', '', { shouldDirty: true });
                    setCustomModel(true);
                  }
                }}
              >
                <option value="opencode">OpenCode</option>
                <option value="claude-code">Claude Code</option>
                <option value="codex">Codex</option>
                <option value="github-copilot">GitHub Copilot</option>
                <option value="kiro">Kiro</option>
              </select>
            </Field>
            {customModel || form.watch('runtime') === 'github-copilot' || form.watch('runtime') === 'kiro' ? (
              <div className="form-grid">
                <Field label="Model provider" error={form.formState.errors.provider?.message}>
                  <input
                    {...form.register('provider')}
                    placeholder="e.g., anthropic"
                    disabled={form.watch('runtime') !== 'opencode' || !!form.watch('runtimeProfileId')}
                  />
                </Field>
                <Field label="Model identifier" error={form.formState.errors.model?.message}>
                  <input {...form.register('model')} placeholder="e.g., claude-sonnet-4" disabled={!!form.watch('runtimeProfileId')} />
                </Field>
              </div>
            ) : (
              <>
                <div className="form-grid">
                  <Field label="Model provider" error={form.formState.errors.provider?.message}>
                    <select
                      value={
                        form.watch('runtime') === 'claude-code'
                          ? 'anthropic'
                          : form.watch('runtime') === 'codex'
                            ? 'openai'
                          : form.watch('provider')
                      }
                      disabled={form.watch('runtime') !== 'opencode' || !!form.watch('runtimeProfileId')}
                      onChange={(event) => {
                        form.setValue('provider', event.target.value, { shouldDirty: true });
                        form.setValue('model', '', { shouldDirty: true });
                      }}
                    >
                      <option value="">Select a provider</option>
                      {Object.entries(catalog.data ?? {}).filter(([id]) => form.watch('runtime') === 'opencode' || (form.watch('runtime') === 'codex' && id === 'openai')).map(([id, provider]) => (
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
                      disabled={!form.watch('provider') || !!form.watch('runtimeProfileId')}
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
                          <option value={form.watch('model')}>
                            {form.watch('model')} (current)
                          </option>
                        )}
                    </select>
                  </Field>
                </div>
                {catalog.isError && (
                  <p className="muted">Model catalog unavailable. You can enter a custom model.</p>
                )}
              </>
            )}
            <label className="agent-form-toggle">
              <input
                type="checkbox"
                checked={customModel}
                disabled={!!form.watch('runtimeProfileId')}
                onChange={(event) => setCustomModel(event.target.checked)}
              />
              <span>Enter a custom provider and model</span>
            </label>
          </div>
        </div>
        <div className="agent-form-section">
          <div className="agent-form-section-header">
            <FileText size={16} />
            <span>Instructions</span>
          </div>
          <div className="agent-form-section-content">
            <Field
              label="System instructions"
              error={form.formState.errors.systemPrompt?.message}
              hint="Define how this agent should behave and approach tasks."
            >
              <textarea
                rows={10}
                className="code-input"
                {...form.register('systemPrompt')}
                placeholder="You are an expert code reviewer. Focus on..."
              />
            </Field>
          </div>
        </div>
        <div className="agent-form-section">
          <div className="agent-form-section-header">
            <Wrench size={16} />
            <span>Permission policy</span>
          </div>
          <div className="agent-form-section-content">
            <Field label="Agent autonomy" hint="Autonomous mode skips approval prompts. Provider controls vary; Codex uses full access and Copilot allows all tools inside the isolated task environment.">
              <select {...form.register('permissionPolicy')}>
                <option value="ask">Ask for permission</option>
                <option value="autonomous">Autonomous</option>
              </select>
            </Field>
          </div>
        </div>
        <div className="agent-form-section">
          <div className="agent-form-section-header">
            <Wrench size={16} />
            <span>Tools & Capabilities</span>
          </div>
          <div className="agent-form-section-content">
            {servers.isError ? (
              <ErrorState error={servers.error} />
            ) : (
              <Selection
                label="MCP servers"
                options={servers.data ?? []}
                value={form.watch('mcpServerIds')}
                onChange={(ids) => form.setValue('mcpServerIds', ids, { shouldDirty: true })}
              />
            )}
            {skills.isError ? (
              <ErrorState error={skills.error} />
            ) : (
              <Selection
                label="Skills"
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
          </div>
        </div>
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
              <Badge>{runtimeName(item.runtime)}</Badge>
            </div>
            <div className="agent-viewer-row">
              <span className="agent-viewer-label">Provider / Model</span>
              <span>
                {item.provider} / {item.model}
              </span>
            </div>
            <div className="agent-viewer-row">
              <span className="agent-viewer-label">Permission policy</span>
              <span>{item.permissionPolicy === 'autonomous' ? 'Autonomous' : 'Ask for permission'}</span>
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
