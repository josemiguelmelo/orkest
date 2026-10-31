import { useState } from 'react';
import { Boxes, Cpu, Layers3 } from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { ExecutionProfile } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { path, profilesQuery, useRefresh } from '../../shared/api/queries';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { PageHeader, Loading, ErrorState, Badge, CreateButton } from '../../shared/ui/common';
import { Field } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';

export function ProfilesPage() {
  const { scope, permissions } = useApp();
  const profiles = useQuery(profilesQuery(scope));
  const [editing, setEditing] = useState<ExecutionProfile | null | undefined>();
  const [history, setHistory] = useState<ExecutionProfile>();
  return (
    <>
      <PageHeader
        title="Execution profiles"
        description="Prepare toolchains for your agents. Updated images apply after active work finishes."
      >
        {permissions.workspaceAdmin && (
          <CreateButton onClick={() => setEditing(null)}>New profile</CreateButton>
        )}
      </PageHeader>
      {profiles.isPending ? (
        <Loading />
      ) : profiles.isError ? (
        <ErrorState error={profiles.error} />
      ) : !profiles.data.length ? (
        <div className="repository-empty">
          <div className="repository-empty-icon">
            <Boxes size={24} />
          </div>
          <h2>No execution profiles</h2>
          <p className="muted">
            Create a profile with the toolchain image and agent runtime your workspace needs.
          </p>
          {permissions.workspaceAdmin && (
            <CreateButton onClick={() => setEditing(null)}>Create your first profile</CreateButton>
          )}
        </div>
      ) : (
        <>
          <div className="repository-list-heading">
            <span>
              {profiles.data.length}{' '}
              {profiles.data.length === 1 ? 'execution profile' : 'execution profiles'}
            </span>
            <span className="muted">Toolchains available to workspace agents</span>
          </div>
          <div className="grid">
            {profiles.data.map((p) => (
              <article className="profile-card" key={p.id}>
                <div className="profile-card-heading">
                  <div className="repository-card-icon">
                    <Boxes size={19} />
                  </div>
                  <div className="profile-card-title">
                    <h3>{p.name}</h3>
                    <p>{p.description || 'No description provided'}</p>
                  </div>
                  <Badge>
                    <span className={`profile-status-dot profile-status-${p.status}`} />
                    {p.status}
                  </Badge>
                </div>
                <div className="profile-runtime">
                  <Cpu size={14} />
                  <span>Agent runtime</span>
                  <code>{p.runtime}</code>
                </div>
                <div className="profile-image">
                  <Layers3 size={14} />
                  <code title={p.toolchainImage}>{p.toolchainImage}</code>
                </div>
                {p.buildError && (
                  <div className="notice error" role="alert">
                    <strong>Build failed</strong>
                    <p>{p.buildError}</p>
                  </div>
                )}
                {p.executionImage && (
                  <p className="profile-prepared muted">
                    A prepared image is available
                    {p.status !== 'ready' ? ' while changes are being prepared' : ''}.
                  </p>
                )}
                <div className="profile-card-actions">
                  {permissions.workspaceAdmin && (
                    <Button variant="outline" size="sm" onClick={() => setEditing(p)}>
                      Edit / rebuild
                    </Button>
                  )}
                  <Button variant="outline" size="sm" onClick={() => setHistory(p)}>
                    Build history
                  </Button>
                </div>
              </article>
            ))}
          </div>
        </>
      )}
      {editing !== undefined && (
        <ProfileEditor
          key={editing?.id ?? 'new'}
          profile={editing}
          close={() => setEditing(undefined)}
        />
      )}
      {history && <BuildHistory profile={history} close={() => setHistory(undefined)} />}
    </>
  );
}

function ProfileEditor({
  profile,
  close,
}: {
  profile: ExecutionProfile | null;
  close: () => void;
}) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const [name, setName] = useState(profile?.name ?? '');
  const [description, setDescription] = useState(profile?.description ?? '');
  const [runtime, setRuntime] = useState<'opencode' | 'claude-code' | 'codex' | 'github-copilot' | 'kiro'>(
    profile?.runtime ?? 'opencode',
  );
  const [toolchainImage, setImage] = useState(profile?.toolchainImage ?? '');
  return (
    <Dialog
      open
      title={profile ? 'Edit / rebuild profile' : 'Create execution profile'}
      description="Set the agent runtime and toolchain image used to prepare task environments."
      onOpenChange={(v) => {
        if (!v && !action.pending) close();
      }}
    >
      <form
        className="form-stack profile-form"
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            const body = {
              name,
              description,
              runtime,
              toolchainImage,
              generation: profile?.generation ?? 0,
            };
            if (profile)
              await data(
                sdk.updateExecutionProfile({
                  ...command,
                  path: { ...path(scope), profileID: profile.id },
                  body,
                }),
              );
            else await data(sdk.createExecutionProfile({ ...command, path: path(scope), body }));
            await refresh(scope);
            close();
          });
        }}
      >
        <div className="profile-form-section">
          <h3>Profile details</h3>
          <p className="muted">Give your team a recognizable name and a short description.</p>
        </div>
        <Field label="Name">
          <input
            required
            maxLength={200}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="For example, Backend development"
            autoFocus
          />
        </Field>
        <Field
          label="Description"
          hint="Optional. Describe the tools or project this profile supports."
        >
          <textarea
            maxLength={2000}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="A short summary for workspace members"
          />
        </Field>
        <div className="profile-form-section">
          <h3>Execution environment</h3>
          <p className="muted">Choose the runtime and container image agents will use.</p>
        </div>
        <Field
          label="Agent runtime"
          hint={
            profile
              ? 'Runtime cannot be changed after the profile is created.'
              : 'The runtime determines which agent protocol is launched.'
          }
        >
          <select
            disabled={!!profile}
            value={runtime}
            onChange={(e) => setRuntime(e.target.value as typeof runtime)}
          >
            <option value="opencode">OpenCode</option>
            <option value="claude-code">Claude Code</option>
            <option value="codex">Codex</option>
            <option value="github-copilot">GitHub Copilot</option>
                <option value="kiro">Kiro</option>
          </select>
        </Field>
        <Field
          label="Toolchain image"
          hint="Use an image based on a supported Orkest foundation. Saving queues a new image build; running agents continue unchanged."
        >
          <input
            required
            value={toolchainImage}
            onChange={(e) => setImage(e.target.value)}
            placeholder="registry.example.com/team/java-tools:21"
          />
        </Field>
        {!!action.error && <ErrorState error={action.error} />}
        <div className="form-actions">
          <Button type="button" variant="outline" disabled={action.pending} onClick={close}>
            Cancel
          </Button>
          <Button type="submit" disabled={action.pending}>
            {action.pending ? 'Saving…' : profile ? 'Save and rebuild' : 'Create and build'}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

function BuildHistory({ profile, close }: { profile: ExecutionProfile; close: () => void }) {
  const { scope } = useApp();
  const builds = useQuery({
    queryKey: [...profilesQuery(scope).queryKey, profile.id, 'builds'],
    queryFn: ({ signal }) =>
      data(
        sdk.listExecutionProfileBuilds({
          ...command,
          path: { ...path(scope), profileID: profile.id },
          signal,
        }),
      ),
    refetchInterval: (q) =>
      q.state.data?.some((b) => ['queued', 'building', 'reconciling'].includes(b.status))
        ? 2000
        : false,
  });
  return (
    <Dialog
      open
      title={`${profile.name} builds`}
      onOpenChange={(v) => {
        if (!v) close();
      }}
    >
      {builds.isPending ? (
        <Loading />
      ) : builds.isError ? (
        <ErrorState error={builds.error} />
      ) : (
        <ul>
          {builds.data.map((b) => (
            <li key={b.id}>
              <Badge>{b.status}</Badge> {new Date(b.createdAt).toLocaleString()}
              <p>{b.buildError || b.executionImage || b.toolchainImage}</p>
            </li>
          ))}
        </ul>
      )}
    </Dialog>
  );
}
