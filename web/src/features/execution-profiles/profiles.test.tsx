import { beforeEach, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ProfilesPage } from './profiles';

const state = vi.hoisted(() => ({ admin: true, create: vi.fn() }));
vi.mock('../../app/context', () => ({
  useApp: () => ({
    scope: { userID: 'u', orgID: 'o', workspaceID: 'w' },
    permissions: { workspaceAdmin: state.admin },
  }),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => ({
  listExecutionProfiles: vi.fn(() => Promise.resolve({ data: [] })),
  createExecutionProfile: state.create,
}));
beforeEach(() => {
  state.admin = true;
  state.create.mockReset().mockResolvedValue({ data: { id: 'p' } });
});
function page() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ProfilesPage />
    </QueryClientProvider>,
  );
}
it('queues preparation with the selected runtime and toolchain', async () => {
  const user = userEvent.setup();
  page();
  await user.click(screen.getByRole('button', { name: 'New profile' }));
  await user.type(screen.getByLabelText('Name'), 'Java backend');
  await user.type(screen.getByLabelText('Toolchain image'), 'example.com/java:21');
  await user.selectOptions(screen.getByLabelText('Agent runtime'), 'claude-code');
  await user.click(screen.getByRole('button', { name: 'Create and build' }));
  await waitFor(() => expect(state.create).toHaveBeenCalledTimes(1));
  expect(state.create.mock.calls[0]?.[0] as unknown).toMatchObject({
    path: { orgID: 'o', workspaceID: 'w' },
    body: { name: 'Java backend', runtime: 'claude-code', toolchainImage: 'example.com/java:21' },
  });
});
it('does not offer build creation to read-only users', async () => {
  state.admin = false;
  page();
  await screen.findByText('No execution profiles');
  expect(screen.queryByRole('button', { name: 'New profile' })).not.toBeInTheDocument();
});
