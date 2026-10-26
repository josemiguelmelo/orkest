import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Bot } from 'lucide-react';
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
export function AgentsPage() {
  const { scope, permissions } = useApp();
  const agents = useQuery(agentsQuery(scope));
  const refresh = useRefresh();
  const [editing, setEditing] = useState<Agent | null | undefined>();
  return (
    <>
      <PageHeader title="Agents" description="Reusable definitions for the way your agents work.">
        {permissions.editWorkspace && (
          <CreateButton onClick={() => setEditing(null)}>New agent</CreateButton>
        )}
      </PageHeader>
      {agents.isPending ? (
        <Loading />
      ) : agents.isError ? (
        <ErrorState error={agents.error} />
      ) : agents.data.length ? (
        <div className="grid">
          {agents.data.map((item) => (
            <article className="resource-card" key={item.id}>
              <div className="split">
                <Bot className="card-icon" size={24} />
                <Badge>OpenCode</Badge>
              </div>
              <h3>{item.name}</h3>
              <p className="muted">{item.description || 'No description'}</p>
              <div className="tags">
                <Badge>
                  {item.provider} / {item.model}
                </Badge>
                <Badge>{item.skillIds.length} skills</Badge>
                <Badge>{item.mcpServerIds.length} MCP servers</Badge>
              </div>
              {permissions.editWorkspace ? (
                <div className="actions">
                  <Button variant="outline" size="sm" onClick={() => setEditing(item)}>
                    Edit definition
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
                <details>
                  <summary>Instructions</summary>
                  <p style={{ whiteSpace: 'pre-wrap' }}>
                    {item.systemPrompt || 'No system prompt.'}
                  </p>
                </details>
              )}
            </article>
          ))}
        </div>
      ) : (
        <Empty title="No agent definitions yet">
          Create an agent with its model, instructions, skills, and MCP connections.
        </Empty>
      )}
      {editing !== undefined && (
        <AgentEditor
          key={editing?.id ?? 'new'}
          item={editing}
          onClose={() => setEditing(undefined)}
        />
      )}
    </>
  );
}
function AgentEditor({ item, onClose }: { item: Agent | null; onClose: () => void }) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const servers = useQuery(mcpQuery(scope));
  const skills = useQuery(skillsQuery(scope));
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
        <div className="form-grid">
          <Field label="Model provider" error={form.formState.errors.provider?.message}>
            <input {...form.register('provider')} placeholder="Provider identifier" />
          </Field>
          <Field label="Model identifier" error={form.formState.errors.model?.message}>
            <input {...form.register('model')} />
          </Field>
        </div>
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
