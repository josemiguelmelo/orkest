import { useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { ArrowUpRight, Folder, Layers, Users } from 'lucide-react';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { Membership } from '../../shared/api/generated/types.gen';
import {
  membersQuery,
  organizationsQuery,
  path,
  useRefresh,
  workspacesQuery,
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
import { Field, Form } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';

const nameSchema = z.object({ name: z.string().trim().min(1, 'Enter a name.').max(200) });
function NameDialog({
  title,
  name = '',
  open,
  onClose,
  save,
}: {
  title: string;
  name?: string;
  open: boolean;
  onClose: () => void;
  save: (name: string) => Promise<unknown>;
}) {
  const form = useForm({ resolver: zodResolver(nameSchema), defaultValues: { name } });
  const action = useAction();
  return (
    <Dialog
      title={title}
      open={open}
      onOpenChange={(value) => {
        if (!value && !action.pending) onClose();
      }}
    >
      <Form
        form={form}
        error={action.error}
        onSubmit={async (values) => {
          if (await action.run(() => save(values.name))) onClose();
        }}
      >
        <Field label="Name" error={form.formState.errors.name?.message}>
          <input autoFocus {...form.register('name')} />
        </Field>
      </Form>
    </Dialog>
  );
}
export function OrganizationsPage() {
  const { scope } = useApp();
  const organizations = useQuery(organizationsQuery(scope.userID));
  const refresh = useRefresh();
  const [creating, setCreating] = useState(false);
  const navigate = useNavigate();
  return (
    <>
      <PageHeader
        title="Organizations"
        description="Choose where you want to work, or create a new organization."
      >
        <CreateButton onClick={() => setCreating(true)}>New organization</CreateButton>
      </PageHeader>
      {organizations.isPending ? (
        <Loading />
      ) : organizations.isError ? (
        <ErrorState error={organizations.error} />
      ) : organizations.data.length ? (
        <div className="grid">
          {organizations.data.map((org) => (
            <Link
              key={org.id}
              to="/organizations/$orgId"
              params={{ orgId: org.id }}
              className="resource-card"
            >
              <div className="split">
                <Layers className="card-icon" size={22} />
                <ArrowUpRight size={16} />
              </div>
              <h3>{org.name}</h3>
              <div>
                <Badge>{org.role ?? 'member'}</Badge>
              </div>
            </Link>
          ))}
        </div>
      ) : (
        <Empty title="Your first organization">
          Create an organization to add workspaces, agents, and tasks.
        </Empty>
      )}
      {creating && (
        <NameDialog
          title="New organization"
          open
          onClose={() => setCreating(false)}
          save={async (name) => {
            const org = await data(sdk.createOrganization({ ...command, body: { name } }));
            await refresh(scope);
            await navigate({ to: '/organizations/$orgId', params: { orgId: org.id } });
          }}
        />
      )}
    </>
  );
}
export function OrganizationPage() {
  const { scope, organization, permissions } = useApp();
  const workspaces = useQuery(workspacesQuery(scope));
  const refresh = useRefresh();
  const navigate = useNavigate();
  const [dialog, setDialog] = useState<'rename' | 'create' | null>(null);
  if (!organization) return <Loading />;
  return (
    <>
      <PageHeader
        title={organization.name}
        description="Manage your workspaces and organization membership."
      >
        {permissions.organizationAdmin && (
          <>
            <Button variant="outline" onClick={() => setDialog('rename')}>
              Rename organization
            </Button>
            <CreateButton onClick={() => setDialog('create')}>New workspace</CreateButton>
          </>
        )}
      </PageHeader>
      <section className="panel">
        <div className="panel-heading">
          <h2>Workspaces</h2>
          <Folder size={18} />
        </div>
        {workspaces.isPending ? (
          <Loading />
        ) : workspaces.isError ? (
          <ErrorState error={workspaces.error} />
        ) : workspaces.data.length ? (
          <div className="grid">
            {workspaces.data.map((ws) => (
              <Link
                className="resource-card"
                key={ws.id}
                to="/organizations/$orgId/workspaces/$workspaceId/tasks"
                params={{ orgId: scope.orgID, workspaceId: ws.id }}
                search={{ view: 'board', q: '', status: 'all', agent: '', sort: 'updated' }}
              >
                <h3>{ws.name}</h3>
                <div>
                  <Badge>{ws.role ?? 'member'}</Badge>
                </div>
              </Link>
            ))}
          </div>
        ) : (
          <Empty title="No workspaces yet">
            {permissions.organizationAdmin
              ? 'Create a workspace to start organizing your project.'
              : 'Ask an organization administrator to add you to a workspace.'}
          </Empty>
        )}
      </section>
      <Members workspace={false} />
      {organization.role === 'owner' && (
        <section className="panel">
          <h2>Delete organization</h2>
          <p className="muted spacer">
            Permanently delete this organization and its workspace data.
          </p>
          <div className="spacer">
            <ConfirmButton
              title="Delete organization?"
              description="All organization and workspace data will be removed. This cannot be undone."
              onConfirm={async () => {
                await data(sdk.deleteOrganization({ ...command, path: { orgID: scope.orgID } }));
                await refresh(scope);
                await navigate({ to: '/' });
              }}
            />
          </div>
        </section>
      )}
      {dialog && (
        <NameDialog
          key={dialog}
          title={dialog === 'rename' ? 'Rename organization' : 'New workspace'}
          name={dialog === 'rename' ? organization.name : ''}
          open
          onClose={() => setDialog(null)}
          save={async (name) => {
            if (dialog === 'rename')
              await data(
                sdk.updateOrganization({
                  ...command,
                  path: { orgID: scope.orgID },
                  body: { name },
                }),
              );
            else
              await data(
                sdk.createWorkspace({ ...command, path: { orgID: scope.orgID }, body: { name } }),
              );
            await refresh(scope);
          }}
        />
      )}
    </>
  );
}
export function WorkspaceSettingsPage() {
  const { scope, workspace, permissions } = useApp();
  const refresh = useRefresh();
  const navigate = useNavigate();
  const [rename, setRename] = useState(false);
  if (!workspace) return <Loading />;
  return (
    <>
      <PageHeader
        title="Settings & members"
        description={`Manage ${workspace.name} and who can access it.`}
      >
        {permissions.workspaceAdmin && (
          <Button variant="outline" onClick={() => setRename(true)}>
            Rename workspace
          </Button>
        )}
      </PageHeader>
      <Members workspace />
      {permissions.workspaceAdmin && (
        <section className="panel">
          <h2>Delete workspace</h2>
          <p className="muted spacer">Permanently remove this workspace and its project data.</p>
          <div className="spacer">
            <ConfirmButton
              title="Delete workspace?"
              description="Tasks, agents, and workspace configuration will be permanently removed."
              onConfirm={async () => {
                await data(sdk.deleteWorkspace({ ...command, path: path(scope) }));
                await refresh(scope);
                await navigate({ to: '/organizations/$orgId', params: { orgId: scope.orgID } });
              }}
            />
          </div>
        </section>
      )}
      {rename && (
        <NameDialog
          title="Rename workspace"
          name={workspace.name}
          open
          onClose={() => setRename(false)}
          save={async (name) => {
            await data(sdk.updateWorkspace({ ...command, path: path(scope), body: { name } }));
            await refresh(scope);
          }}
        />
      )}
    </>
  );
}
const memberSchema = z.object({
  userID: z.string().trim().min(1, 'Choose a user.'),
  role: z.enum(['owner', 'admin', 'member', 'viewer']),
});
function Members({ workspace }: { workspace: boolean }) {
  const { scope, organization, permissions, session } = useApp();
  const members = useQuery(membersQuery(scope, workspace));
  const orgMembers = useQuery({ ...membersQuery(scope, false), enabled: workspace });
  const refresh = useRefresh();
  const [editing, setEditing] = useState<Membership | null | undefined>();
  const manageable = workspace ? permissions.workspaceAdmin : permissions.organizationAdmin;
  const owners = members.data?.filter((m) => m.role === 'owner').length ?? 0;
  const canEdit = (member: Membership) =>
    manageable && (workspace || organization?.role === 'owner' || member.role !== 'owner');
  return (
    <section className="panel">
      <div className="panel-heading">
        <h2>
          <Users size={17} style={{ display: 'inline', marginRight: 8 }} />
          {workspace ? 'Workspace' : 'Organization'} members
        </h2>
        {manageable && <CreateButton onClick={() => setEditing(null)}>Add member</CreateButton>}
      </div>
      {members.isPending ? (
        <Loading />
      ) : members.isError ? (
        <ErrorState error={members.error} />
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>User</th>
                <th>Email</th>
                <th>Role</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {members.data.map((member) => (
                <tr key={member.user.id}>
                  <td>
                    {member.user.username}
                    {member.user.id === session.user.id ? ' (you)' : ''}
                    <p className="muted">{member.user.id}</p>
                  </td>
                  <td>{member.user.email}</td>
                  <td>
                    <Badge>{member.role}</Badge>
                  </td>
                  <td>
                    <div className="actions">
                      {canEdit(member) && (
                        <>
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={!workspace && member.role === 'owner' && owners === 1}
                            onClick={() => setEditing(member)}
                          >
                            Change role
                          </Button>
                          <ConfirmButton
                            title="Remove member?"
                            description="This user will lose the access granted by this membership."
                            pending={!workspace && member.role === 'owner' && owners === 1}
                            onConfirm={async () => {
                              if (workspace)
                                await data(
                                  sdk.removeWorkspaceMember({
                                    ...command,
                                    path: { ...path(scope), userID: member.user.id },
                                  }),
                                );
                              else
                                await data(
                                  sdk.removeOrganizationMember({
                                    ...command,
                                    path: { orgID: scope.orgID, userID: member.user.id },
                                  }),
                                );
                              await refresh(scope);
                            }}
                          />
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing !== undefined && (
        <MemberDialog
          key={editing?.user.id ?? 'new'}
          member={editing}
          workspace={workspace}
          choices={orgMembers.data ?? []}
          onClose={() => setEditing(undefined)}
        />
      )}
    </section>
  );
}
function MemberDialog({
  member,
  workspace,
  choices,
  onClose,
}: {
  member: Membership | null;
  workspace: boolean;
  choices: Membership[];
  onClose: () => void;
}) {
  const { scope, organization } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const form = useForm({
    resolver: zodResolver(memberSchema),
    defaultValues: { userID: member?.user.id ?? '', role: member?.role ?? 'member' },
  });
  return (
    <Dialog
      title={member ? 'Change member role' : 'Add member'}
      description={
        workspace
          ? 'Choose an existing organization member.'
          : 'Enter the internal user ID of someone who has already signed in.'
      }
      open
      onOpenChange={(open) => {
        if (!open && !action.pending) onClose();
      }}
    >
      <Form
        form={form}
        error={action.error}
        onSubmit={async (values) => {
          if (
            await action.run(async () => {
              if (workspace) {
                if (values.role === 'owner') throw new Error('Owner is an organization role.');
                await data(
                  sdk.setWorkspaceMember({
                    ...command,
                    path: { ...path(scope), userID: values.userID },
                    body: { role: values.role },
                  }),
                );
              } else {
                if (values.role === 'viewer') throw new Error('Viewer is a workspace role.');
                await data(
                  sdk.setOrganizationMember({
                    ...command,
                    path: { orgID: scope.orgID, userID: values.userID },
                    body: { role: values.role },
                  }),
                );
              }
              await refresh(scope);
            })
          )
            onClose();
        }}
      >
        <Field label="User" error={form.formState.errors.userID?.message}>
          {workspace ? (
            <select {...form.register('userID')} disabled={!!member}>
              <option value="">Choose member</option>
              {choices.map((m) => (
                <option key={m.user.id} value={m.user.id}>
                  {m.user.username} · {m.user.email}
                </option>
              ))}
            </select>
          ) : (
            <input {...form.register('userID')} disabled={!!member} />
          )}
        </Field>
        <Field label="Role">
          <select {...form.register('role')}>
            {!workspace && organization?.role === 'owner' && <option value="owner">Owner</option>}
            <option value="admin">Admin</option>
            <option value="member">Member</option>
            {workspace && <option value="viewer">Viewer</option>}
          </select>
        </Field>
      </Form>
    </Dialog>
  );
}
