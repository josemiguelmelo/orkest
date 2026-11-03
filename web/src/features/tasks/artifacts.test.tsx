import { afterEach, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppContext } from '../../app/context';
import { capabilities } from '../../app/permissions';
const api = vi.hoisted(() => ({
  listTaskArtifacts: vi.fn(),
  listArtifactRevisions: vi.fn(),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => api);
import { TaskArtifacts } from './artifacts';
afterEach(() => vi.clearAllMocks());
function setup(role: 'member' | 'viewer') {
  api.listTaskArtifacts.mockResolvedValue({ data: [] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AppContext.Provider
        value={{
          scope: { userID: 'u', orgID: 'org', workspaceID: 'workspace' },
          permissions: capabilities('member', role, false),
          session: {} as never,
          organization: undefined,
          workspace: undefined,
        }}
      >
        <TaskArtifacts taskID="task" />
      </AppContext.Provider>
    </QueryClientProvider>,
  );
}
it('does not expose manual publication to members', async () => {
  setup('member');
  await screen.findByText('No artifacts published yet.');
  expect(screen.queryByRole('button', { name: /publish|revise/i })).not.toBeInTheDocument();
});
it('keeps publication unavailable to viewers', async () => {
  setup('viewer');
  await screen.findByText('No artifacts published yet.');
  expect(screen.queryByRole('button', { name: /publish|revise/i })).not.toBeInTheDocument();
});
