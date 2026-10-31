import { useState } from 'react';
import { Bot, Cpu, Pencil } from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { AgentRuntimeProfile } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { agentRuntimeProfilesQuery, path, useRefresh } from '../../shared/api/queries';
import { Badge, ConfirmButton, CreateButton, Empty, ErrorState, Loading, PageHeader } from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Field, Form } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';

const schema = z.object({
  name: z.string().trim().min(1).max(200),
  runtime: z.enum(['opencode', 'claude-code', 'codex']),
  modelProvider: z.string().trim().min(1).max(200),
  modelIdentifier: z.string().trim().min(1).max(200),
});
type Values = z.infer<typeof schema>;

export function AgentRuntimeProfilesPage() {
  const { scope, permissions } = useApp();
  const profiles = useQuery(agentRuntimeProfilesQuery(scope));
  const refresh = useRefresh();
  const [editing, setEditing] = useState<AgentRuntimeProfile | null | undefined>();
  return <>
    <PageHeader title="Runtime profiles" description="Save runtime and model settings as reusable agent presets. Updating a profile applies the new settings to all linked agents.">
      {permissions.editWorkspace && <CreateButton onClick={() => setEditing(null)}>New runtime profile</CreateButton>}
    </PageHeader>
    {profiles.isPending ? <Loading /> : profiles.isError ? <ErrorState error={profiles.error} /> : profiles.data.length === 0 ? (
      <Empty title="No runtime profiles yet">Create a profile for a runtime, model provider, and model identifier, then select it when creating or editing agents.</Empty>
    ) : <div className="grid">
      {profiles.data.map((profile) => <article className="profile-card" key={profile.id}>
        <div className="profile-card-heading"><div className="repository-card-icon"><Bot size={19} /></div><div className="profile-card-title"><h3>{profile.name}</h3><p>{profile.runtime}</p></div></div>
        <div className="profile-runtime"><Cpu size={14} /><span>Model</span><code>{profile.modelProvider}/{profile.modelIdentifier}</code></div>
        <div className="agent-card-actions">
          {permissions.editWorkspace && <>
            <Button variant="outline" size="sm" onClick={() => setEditing(profile)}><Pencil size={14} />Edit</Button>
            <ConfirmButton label="Delete" title="Delete runtime profile?" description="Linked agents keep their current runtime and model settings and become manually configured." onConfirm={async () => { await data(sdk.deleteAgentRuntimeProfile({ ...command, path: { ...path(scope), profileID: profile.id } })); await refresh(scope); }} />
          </>}
        </div>
      </article>)}
    </div>}
    {editing !== undefined && <ProfileEditor key={editing?.id ?? 'new'} profile={editing} onClose={() => setEditing(undefined)} />}
  </>;
}

function ProfileEditor({ profile, onClose }: { profile: AgentRuntimeProfile | null; onClose: () => void }) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: {
    name: profile?.name ?? '', runtime: profile?.runtime ?? 'opencode',
    modelProvider: profile?.modelProvider ?? '', modelIdentifier: profile?.modelIdentifier ?? '',
  } });
  return <Dialog title={profile ? 'Edit runtime profile' : 'Create runtime profile'} description="Agents linked to this profile receive its updated settings together." open onOpenChange={(open) => { if (!open && !action.pending) onClose(); }}>
    <Form form={form} error={action.error} onSubmit={async (body) => {
      if (await action.run(async () => {
        if (profile) await data(sdk.updateAgentRuntimeProfile({ ...command, path: { ...path(scope), profileID: profile.id }, body }));
        else await data(sdk.createAgentRuntimeProfile({ ...command, path: path(scope), body }));
        await refresh(scope);
      })) onClose();
    }}>
      <Field label="Name" error={form.formState.errors.name?.message}><input {...form.register('name')} placeholder="e.g., Claude through LiteLLM" /></Field>
      <Field label="Runtime"><select {...form.register('runtime')}><option value="opencode">OpenCode</option><option value="claude-code">Claude Code</option><option value="codex">Codex</option></select></Field>
      <Field label="Model provider" error={form.formState.errors.modelProvider?.message} hint="Use the gateway provider name configured for this runtime."><input {...form.register('modelProvider')} placeholder="e.g., anthropic or litellm" /></Field>
      <Field label="Model identifier" error={form.formState.errors.modelIdentifier?.message}><input {...form.register('modelIdentifier')} placeholder="e.g., claude-sonnet-4" /></Field>
      {profile && <Badge>Changes apply to linked agents</Badge>}
    </Form>
  </Dialog>;
}
