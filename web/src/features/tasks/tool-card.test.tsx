import { expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ToolCard } from './tool-card';

it('expands escaped inputs, results, locations and file previews safely', async () => {
  const user = userEvent.setup();
  const { container } = render(
    <ToolCard
      tool={{
        id: '1',
        title: 'Edit app.ts',
        status: 'completed',
        toolKind: 'edit',
        input: '{"path":"app.ts"}',
        output: '<script>alert(1)</script>',
        truncated: true,
        startedAt: 1000,
        endedAt: 3000,
        locations: [{ path: 'app.ts', line: 12 }],
        content: [
          { kind: 'diff', path: 'app.ts', oldText: 'old', newText: 'new', truncated: true },
        ],
      }}
    />,
  );
  const summary = screen.getByText('Edit app.ts').closest('summary')!;
  await user.click(summary);
  expect(summary.parentElement).toHaveAttribute('open');
  expect(screen.getByText('completed · 2s')).toBeInTheDocument();
  expect(screen.getByText('app.ts:12')).toBeInTheDocument();
  expect(screen.getByText('Before')).toBeInTheDocument();
  expect(screen.getByText('After')).toBeInTheDocument();
  expect(screen.getByText('<script>alert(1)</script>')).toBeInTheDocument();
  expect(container.querySelector('script')).toBeNull();
  expect(screen.getAllByRole('note')).toHaveLength(2);
});

it('keeps legacy calls readable and preserves expansion across updates', async () => {
  const user = userEvent.setup();
  const tool = { id: '1', title: 'Run command', status: 'pending' };
  const view = render(<ToolCard tool={tool} />);
  await user.click(screen.getByText('Run command'));
  view.rerender(<ToolCard tool={{ ...tool, status: 'failed', output: 'command failed' }} />);
  expect(screen.getByText('Run command').closest('details')).toHaveAttribute('open');
  expect(screen.getByText('failed')).toBeInTheDocument();
  expect(screen.getByText('command failed')).toBeInTheDocument();
});

it('shows the read path in its summary before opening the details', () => {
  render(
    <ToolCard
      tool={{
        id: 'read',
        title: 'read',
        status: 'completed',
        input: '{"filePath":"/workspace/README.md"}',
      }}
    />,
  );
  expect(screen.getByText('read · /workspace/README.md').closest('summary')).toBeInTheDocument();
});
