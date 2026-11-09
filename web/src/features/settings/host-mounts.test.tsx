import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppContext } from '../../app/context';
import { capabilities } from '../../app/permissions';
import type { WorkspaceHostMountSettings } from '../../shared/api/generated/types.gen';

const api = vi.hoisted(() => ({
  getWorkspaceHostMounts: vi.fn(),
  putWorkspaceHostMounts: vi.fn(),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => api);

import { WorkspaceHostMountsPanel } from './host-mounts';

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function renderPanel(settings: WorkspaceHostMountSettings, role: 'admin' | 'viewer' = 'admin') {
  api.getWorkspaceHostMounts.mockResolvedValue({ data: settings });
  api.putWorkspaceHostMounts.mockResolvedValue({ data: { status: 'updated' } });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AppContext.Provider
        value={{
          scope: { userID: 'user', orgID: 'org', workspaceID: 'workspace' },
          permissions: capabilities('member', role, false),
          session: {
            user: { id: 'user', username: 'User', email: 'user@example.test' },
            organizations: [],
            platformAdmin: false,
          },
          organization: undefined,
          workspace: undefined,
        }}
      >
        <WorkspaceHostMountsPanel />
      </AppContext.Provider>
    </QueryClientProvider>,
  );
}

it('adds a custom read-only volume without environment configuration', async () => {
  const user = userEvent.setup();
  renderPanel({
    enabled: true,
    mounts: [],
  });
  await user.click(await screen.findByRole('button', { name: 'Add volume' }));
  await user.type(screen.getByLabelText('Host source'), '/srv/input');
  const readOnly = screen.getByRole('checkbox', { name: 'Read only' });
  expect(readOnly).toBeChecked();
  expect(readOnly).toBeEnabled();
  await user.type(screen.getByLabelText('Container target'), 'input');
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  await waitFor(() =>
    expect(api.putWorkspaceHostMounts).toHaveBeenCalledWith(
      expect.objectContaining({
        path: { orgID: 'org', workspaceID: 'workspace' },
        body: { mounts: [{ source: '/srv/input', target: '/mnt/host/input', readOnly: true }] },
      }),
    ),
  );
});

it.each(['~/.aws', '~/.m2/settings.xml', '~/.gradle/gradle.properties'])(
  'saves home-relative credential target %s without environment configuration',
  async (target) => {
    const user = userEvent.setup();
    renderPanel({ enabled: true, mounts: [] });
    await user.click(await screen.findByRole('button', { name: 'Add volume' }));
    await user.type(screen.getByLabelText('Host source'), '/srv/credentials');
    const input = screen.getByLabelText('Container target');
    await user.clear(input);
    await user.type(input, target);
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() =>
      expect(api.putWorkspaceHostMounts).toHaveBeenCalledWith(
        expect.objectContaining({
          body: { mounts: [{ source: '/srv/credentials', target, readOnly: true }] },
        }),
      ),
    );
  },
);

it.each(['~/', '~/../session.json', '~/.aws/', '~//.aws', '/root/.aws'])(
  'rejects invalid credential target %s',
  async (target) => {
    const user = userEvent.setup();
    renderPanel({ enabled: true, mounts: [] });
    await user.click(await screen.findByRole('button', { name: 'Add volume' }));
    await user.type(screen.getByLabelText('Host source'), '/srv/credentials');
    const input = screen.getByLabelText('Container target');
    await user.clear(input);
    await user.type(input, target);
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText(/Use a (target beneath|clean path)/)).toBeInTheDocument();
    expect(api.putWorkspaceHostMounts).not.toHaveBeenCalled();
  },
);

it('rejects relative host paths before saving', async () => {
  const user = userEvent.setup();
  renderPanel({
    enabled: true,
    mounts: [],
  });
  await user.click(await screen.findByRole('button', { name: 'Add volume' }));
  await user.type(screen.getByLabelText('Host source'), 'relative/path');
  await user.type(screen.getByLabelText('Container target'), 'input');
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  expect(await screen.findByText(/Use an absolute, clean host path/)).toBeInTheDocument();
  expect(api.putWorkspaceHostMounts).not.toHaveBeenCalled();
});

it('rejects overlapping container targets before saving', async () => {
  const user = userEvent.setup();
  renderPanel({
    enabled: true,
    mounts: [{ source: '/srv/input', target: '/mnt/host/input', readOnly: true }],
  });
  await user.click(await screen.findByRole('button', { name: 'Add volume' }));
  await user.type(screen.getByLabelText('Host source'), '/srv/input');
  await user.type(screen.getByLabelText('Container target'), 'input/nested');
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  expect(await screen.findByText('Mount targets must not overlap.')).toBeInTheDocument();
  expect(api.putWorkspaceHostMounts).not.toHaveBeenCalled();
});

it('shows volumes without mutation controls to viewers', async () => {
  renderPanel(
    {
      enabled: true,
      mounts: [{ source: '/srv/input', target: '/mnt/host/input', readOnly: true }],
    },
    'viewer',
  );
  expect(await screen.findByText('/mnt/host/input')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Add volume' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
});

it('allows additions immediately and explains the trusted deployment boundary', async () => {
  renderPanel({ enabled: true, mounts: [] });
  expect(await screen.findByText(/Trusted deployments only/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Add volume' })).toBeEnabled();
});

it('reports a non-Docker executor without offering additions', async () => {
  renderPanel({ enabled: false, mounts: [] });
  expect(await screen.findByText(/available only with the Docker executor/)).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Add volume' })).not.toBeInTheDocument();
});

it('surfaces save errors and keeps the editor open', async () => {
  const user = userEvent.setup();
  renderPanel({
    enabled: true,
    mounts: [],
  });
  api.putWorkspaceHostMounts.mockRejectedValue(new Error('Could not save host volumes.'));
  await user.click(await screen.findByRole('button', { name: 'Add volume' }));
  await user.type(screen.getByLabelText('Host source'), '/srv/input');
  await user.type(screen.getByLabelText('Container target'), 'input');
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  expect(await screen.findByText('Could not save host volumes.')).toBeInTheDocument();
  expect(screen.getByRole('dialog')).toBeInTheDocument();
});

it('edits an existing volume and saves write access directly', async () => {
  const user = userEvent.setup();
  renderPanel({
    enabled: true,
    mounts: [{ source: '/srv/output', target: '/mnt/host/output', readOnly: true }],
  });
  await user.click(await screen.findByRole('button', { name: 'Edit' }));
  await user.click(screen.getByRole('checkbox', { name: 'Read only' }));
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  await waitFor(() =>
    expect(api.putWorkspaceHostMounts).toHaveBeenCalledWith(
      expect.objectContaining({
        body: { mounts: [{ source: '/srv/output', target: '/mnt/host/output', readOnly: false }] },
      }),
    ),
  );
});

it('removes a volume without deleting host files', async () => {
  const user = userEvent.setup();
  renderPanel({
    enabled: true,
    mounts: [{ source: '/srv/input', target: '/mnt/host/input', readOnly: true }],
  });
  await user.click(await screen.findByRole('button', { name: 'Delete' }));
  expect(screen.getByText(/Host files are not deleted/)).toBeInTheDocument();
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Delete' }));
  await waitFor(() =>
    expect(api.putWorkspaceHostMounts).toHaveBeenCalledWith(
      expect.objectContaining({
        body: { mounts: [] },
      }),
    ),
  );
});

it('can clear all volumes from the UI', async () => {
  const user = userEvent.setup();
  renderPanel({
    enabled: true,
    mounts: [
      { source: '/srv/input', target: '/mnt/host/input', readOnly: true },
      { source: '/srv/output', target: '/mnt/host/output', readOnly: false },
    ],
  });
  await user.click(await screen.findByRole('button', { name: 'Remove all volumes' }));
  await user.click(
    within(screen.getByRole('dialog')).getByRole('button', { name: 'Remove all volumes' }),
  );
  await waitFor(() =>
    expect(api.putWorkspaceHostMounts).toHaveBeenCalledWith(
      expect.objectContaining({
        body: { mounts: [] },
      }),
    ),
  );
});
