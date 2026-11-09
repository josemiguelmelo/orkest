import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type {
  WorkspaceHostMount,
  WorkspaceHostMountSettings,
} from '../../shared/api/generated/types.gen';
import { keys, path, useRefresh } from '../../shared/api/queries';
import { Button } from '../../shared/ui/button';
import { ConfirmButton, Empty, ErrorState, Loading } from '../../shared/ui/common';
import { Dialog } from '../../shared/ui/dialog';
import { Field, Form } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';

const isClean = (value: string) =>
  !value.includes('\0') &&
  value.split('/').every((part, i) => i === 0 || (part !== '' && part !== '.' && part !== '..'));

const schema = z.object({
  source: z
    .string()
    .min(2, 'Enter an absolute host path.')
    .max(4096)
    .refine(
      (value) => value.startsWith('/') && isClean(value),
      'Use an absolute, clean host path without trailing slashes or dot segments.',
    ),
  target: z
    .string()
    .max(4096)
    .regex(/^(\/mnt\/host\/|~\/)[^/].*/, 'Use a target beneath /mnt/host/ or ~/.')
    .refine(isClean, 'Use a clean path without trailing slashes, empty segments, or dot segments.'),
  readOnly: z.boolean(),
});

export function WorkspaceHostMountsPanel() {
  const { scope, permissions } = useApp();
  const refresh = useRefresh();
  const settings = useQuery({
    queryKey: [...keys.workspace(scope), 'host-mounts'],
    queryFn: ({ signal }) =>
      data(sdk.getWorkspaceHostMounts({ ...command, path: path(scope), signal })),
  });
  const [editing, setEditing] = useState<WorkspaceHostMount | null>();
  const save = async (mounts: WorkspaceHostMount[]) => {
    await data(sdk.putWorkspaceHostMounts({ ...command, path: path(scope), body: { mounts } }));
    await refresh(scope);
  };
  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>Docker host volumes</h2>
          <p className="muted">
            Mount host files or directories under /mnt/host or the agent home using ~/. New sessions
            use these settings; existing sessions retain their saved mounts. Host files are not
            copied or backed up by Orkest.
          </p>
        </div>
        {permissions.workspaceAdmin && settings.data?.enabled && (
          <Button disabled={settings.data.mounts.length >= 32} onClick={() => setEditing(null)}>
            Add volume
          </Button>
        )}
      </div>
      {settings.isPending ? (
        <Loading />
      ) : settings.isError ? (
        <ErrorState error={settings.error} retry={() => void settings.refetch()} />
      ) : (
        <>
          {!settings.data.enabled && (
            <p className="muted">Host volumes are available only with the Docker executor.</p>
          )}
          {settings.data.enabled && (
            <p className="muted">
              Trusted deployments only: workspace admins can expose any path accessible to the
              Docker daemon, including files outside this workspace. Do not mount unrelated
              credentials or the Docker socket. No server environment variable is required.
            </p>
          )}
          {settings.data.mounts.length ? (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Host source</th>
                    <th>Container target</th>
                    <th>Access</th>
                    {permissions.workspaceAdmin && <th>Actions</th>}
                  </tr>
                </thead>
                <tbody>
                  {settings.data.mounts.map((mount) => (
                    <tr key={mount.target}>
                      <td>
                        <code>{mount.source}</code>
                      </td>
                      <td>
                        <code>{mount.target}</code>
                      </td>
                      <td>{mount.readOnly ? 'Read only' : 'Read/write'}</td>
                      {permissions.workspaceAdmin && (
                        <td>
                          <div className="actions">
                            <Button
                              variant="outline"
                              size="sm"
                              disabled={!settings.data.enabled}
                              onClick={() => setEditing(mount)}
                            >
                              Edit
                            </Button>
                            <ConfirmButton
                              title={`Remove ${mount.target}?`}
                              description="New sessions will no longer mount this path. Existing sessions keep their saved mounts. Host files are not deleted."
                              onConfirm={() =>
                                save(
                                  settings.data.mounts.filter(
                                    (entry) => entry.target !== mount.target,
                                  ),
                                )
                              }
                            />
                          </div>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty title="No host volumes configured">
              Add only the files your workspace agents need. Prefer read-only mounts; writable
              mounts allow agents to change or delete host files.
            </Empty>
          )}
          {permissions.workspaceAdmin && settings.data.mounts.length > 1 && (
            <ConfirmButton
              label="Remove all volumes"
              title="Remove all host volumes?"
              description="New sessions will have no host mounts. Existing sessions keep their saved mounts. Host files are not deleted."
              onConfirm={() => save([])}
            />
          )}
          {editing !== undefined && (
            <MountEditor
              settings={settings.data}
              item={editing}
              onClose={() => setEditing(undefined)}
              save={save}
            />
          )}
        </>
      )}
    </section>
  );
}

function MountEditor({
  settings,
  item,
  save,
  onClose,
}: {
  settings: WorkspaceHostMountSettings;
  item: WorkspaceHostMount | null;
  save: (mounts: WorkspaceHostMount[]) => Promise<void>;
  onClose: () => void;
}) {
  const action = useAction();
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: item ?? { source: '', target: '/mnt/host/', readOnly: true },
  });
  return (
    <Dialog
      title={item ? 'Edit host volume' : 'Add host volume'}
      open
      onOpenChange={(open) => {
        if (!open && !action.pending) onClose();
      }}
    >
      <Form
        form={form}
        error={action.error}
        onCancel={onClose}
        onSubmit={async (mount) => {
          if (item && !settings.mounts.some((entry) => entry.target === item.target)) {
            form.setError('target', {
              message: 'This mount changed. Close the editor and refresh before retrying.',
            });
            return;
          }
          if (
            settings.mounts.some(
              (entry) =>
                entry.target !== item?.target &&
                (entry.target === mount.target ||
                  entry.target.startsWith(`${mount.target}/`) ||
                  mount.target.startsWith(`${entry.target}/`)),
            )
          ) {
            form.setError('target', { message: 'Mount targets must not overlap.' });
            return;
          }
          const mounts =
            item === null
              ? [...settings.mounts, mount]
              : settings.mounts.map((entry) => (entry.target === item.target ? mount : entry));
          if (await action.run(() => save(mounts))) onClose();
        }}
      >
        <Field
          label="Host source"
          hint="Absolute file or directory path on the Docker daemon host, not the browser or server container."
          error={form.formState.errors.source?.message}
        >
          <input {...form.register('source')} placeholder="/srv/project-data" />
        </Field>
        <Field
          label="Container target"
          hint="Use ~/.aws, ~/.m2/settings.xml, or ~/.gradle/gradle.properties for credentials in the agent's private session home. Never use /root as the agent home."
          error={form.formState.errors.target?.message}
        >
          <input {...form.register('target')} placeholder="~/.aws or /mnt/host/project-data" />
        </Field>
        <p className="muted">
          Mount Maven and Gradle credential files individually to keep caches writable. JVM tools
          may need explicit settings or user-home flags; changing HOME does not change Java
          user.home. AWS SSO login or token refresh may require write access to its cache.
        </p>
        <label>
          <input type="checkbox" {...form.register('readOnly')} />
          Read only
        </label>
        <p className="muted">
          Disabling read only permits agents to modify and delete files on the host.
        </p>
      </Form>
    </Dialog>
  );
}
