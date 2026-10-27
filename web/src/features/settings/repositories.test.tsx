import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppContext } from '../../app/context';
import { capabilities } from '../../app/permissions';

const api = vi.hoisted(() => ({
  listCodeRepositories: vi.fn(),
  listCodingIntegrations: vi.fn(),
  browseCodingRepositories: vi.fn(),
  createCodeRepository: vi.fn(),
  cloneCodeRepository: vi.fn(),
  syncCodeRepository: vi.fn(),
  deleteCodeRepository: vi.fn(),
}));
vi.mock('../../shared/api/generated/sdk.gen', () => api);

import { RepositoriesPage } from './repositories';

afterEach(() => { cleanup(); vi.clearAllMocks(); });

it('clears a repository selection when integration changes and registers provider identity', async () => {
  const user = userEvent.setup();
  api.listCodeRepositories.mockResolvedValue({ data: [] });
  api.listCodingIntegrations.mockResolvedValue({ data: [
    { id: 'first', name: 'First', providerName: 'github' },
    { id: 'second', name: 'Second', providerName: 'github' },
  ] });
  api.browseCodingRepositories.mockImplementation(({ path }: { path: { integrationID: string } }) => Promise.resolve({ data: {
    repositories: [{ id: path.integrationID === 'first' ? '42' : '84', name: `acme/${path.integrationID}`, cloneUrl: 'https://github.com/acme/project.git', private: true, defaultBranch: 'trunk' }],
  } }));
  api.createCodeRepository.mockResolvedValue({ data: {} });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const value = {
    scope: { userID: 'user', orgID: 'org', workspaceID: 'workspace' },
    permissions: capabilities('owner', undefined, false),
    session: {} as never,
    organization: undefined,
    workspace: undefined,
  };
  render(<QueryClientProvider client={client}><AppContext.Provider value={value as never}><RepositoriesPage /></AppContext.Provider></QueryClientProvider>);
  await user.click(await screen.findByRole('button', { name: 'Register repository' }));
  const integrationSelect = await screen.findByLabelText('Coding integration');
  await user.selectOptions(integrationSelect, 'first');
  await user.selectOptions(await screen.findByLabelText('Repository'), '42');
  await user.selectOptions(integrationSelect, 'second');
  expect(screen.getByRole('button', { name: /^Register repository$/ })).toBeDisabled();
  await user.selectOptions(await screen.findByLabelText('Repository'), '84');
  await user.click(screen.getByRole('button', { name: /^Register repository$/ }));
  await waitFor(() => expect(api.createCodeRepository).toHaveBeenCalledWith(expect.objectContaining({ body: { integrationId: 'second', providerRepositoryId: '84' } })));
});
