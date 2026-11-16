import { beforeEach, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { PlatformAdminPage } from './admin';

const state = vi.hoisted(() => ({
  overview: vi.fn(),
  configuration: vi.fn(),
}));
vi.mock('../../shared/api/queries', () => ({
  meQuery: { queryKey: ['session'], queryFn: () => ({ platformAdmin: true }) },
}));
vi.mock('../../shared/api/client', () => ({
  command: {},
  data: async (request: Promise<{ data: unknown }>) => (await request).data,
}));
vi.mock('../../shared/api/generated/sdk.gen', () => ({
  getPlatformAdminOverview: state.overview,
  getPlatformRuntimeConfiguration: state.configuration,
}));

const analytics = {
  enabledUsers: 12,
  organizations: 3,
  workspaces: 5,
  runsLast30Days: 8,
};
const runtimeConfig = {
  executor: 'docker',
  agentctlImages: {
    opencode: 'orkest/agentctl-opencode:2.1.4',
    codex: 'orkest/agentctl-codex:2.3.0',
  },
  agentMcpUrl: 'http://orkest:8080',
  agentMcpSource: 'environment_override',
  dataDirectory: '/var/lib/orkest',
  codexAuth: { configured: false, type: '' },
  loadedAt: '2026-11-16T12:00:00Z',
};
beforeEach(() => {
  state.overview.mockReset().mockResolvedValue({ data: analytics });
  state.configuration.mockReset().mockResolvedValue({ data: runtimeConfig });
});
function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <PlatformAdminPage />
    </QueryClientProvider>,
  );
}

it('shows the platform metrics and runtime settings in readable sections', async () => {
  renderPage();
  expect(await screen.findByText('12')).toBeInTheDocument();
  expect(screen.getByText('Runs · last 30 days')).toBeInTheDocument();
  expect(screen.getByText('Rolling 30-day window')).toBeInTheDocument();
  expect(screen.getByText('http://orkest:8080')).toBeInTheDocument();
  expect(screen.getByText('/var/lib/orkest')).toBeInTheDocument();
  expect(screen.getByText('orkest/agentctl-opencode:2.1.4')).toBeInTheDocument();
});

it('refreshes analytics and runtime configuration together', async () => {
  const user = userEvent.setup();
  renderPage();
  await screen.findByText('12');
  await user.click(screen.getByRole('button', { name: 'Refresh dashboard data' }));
  await waitFor(() => {
    expect(state.overview).toHaveBeenCalledTimes(2);
    expect(state.configuration).toHaveBeenCalledTimes(2);
  });
});

it('copies deployment values and confirms the copy accessibly', async () => {
  const user = userEvent.setup();
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
  renderPage();
  await screen.findByText('http://orkest:8080');
  await user.click(screen.getByRole('button', { name: 'Copy MCP URL' }));
  expect(writeText).toHaveBeenCalledWith('http://orkest:8080');
  expect(await screen.findByRole('status')).toHaveTextContent('Copied');
});
