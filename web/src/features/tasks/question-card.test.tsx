import { expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QuestionCard } from './question-card';
import type { ChatQuestion } from './question-model';

const question: ChatQuestion = {
  id: 'question',
  runId: 'run',
  sessionId: 'native',
  taskSessionId: 'conversation',
  commandId: 'command',
  message: 'Select a package manager',
  status: 'pending',
  schema: {
    properties: {
      manager: {
        type: 'string',
        title: 'Package manager',
        oneOf: [
          { const: 'npm', title: 'NPM' },
          { const: 'pnpm', title: 'PNPM' },
        ],
      },
      note: { type: 'string', title: 'Additional answer' },
    },
    required: ['manager'],
  },
};

it('sends exact option values and custom text, retaining the submission ID across a failed retry', async () => {
  const user = userEvent.setup();
  const respond = vi
    .fn()
    .mockRejectedValueOnce(new Error('Connection lost'))
    .mockResolvedValue(undefined);
  render(<QuestionCard question={question} canAnswer respond={respond} />);
  await user.selectOptions(
    screen.getByLabelText('Package manager *'),
    screen.getByRole('option', { name: 'PNPM' }),
  );
  await user.type(screen.getByLabelText('Additional answer'), 'Use this workspace');
  await user.click(screen.getByRole('button', { name: 'Send answer' }));
  await screen.findByText('Connection lost');
  expect(screen.getByLabelText('Additional answer')).toHaveValue('Use this workspace');
  await user.click(screen.getByRole('button', { name: 'Send answer' }));
  await waitFor(() => expect(respond).toHaveBeenCalledTimes(2));
  expect(respond.mock.calls[0]).toEqual(respond.mock.calls[1]);
  expect(respond.mock.calls[0]?.[0]).toEqual({
    action: 'accept',
    content: { manager: 'pnpm', note: 'Use this workspace' },
  });
  await screen.findByText('Answer recorded');
});

it('lets users decline without supplying required field values', async () => {
  const respond = vi.fn().mockResolvedValue(undefined);
  render(<QuestionCard question={question} canAnswer respond={respond} />);
  await userEvent.setup().click(screen.getByRole('button', { name: 'Decline' }));
  expect(respond.mock.calls[0]?.[0]).toEqual({ action: 'decline' });
});

it('shows viewers the pending question without answer controls', () => {
  render(<QuestionCard question={question} canAnswer={false} respond={vi.fn()} />);
  expect(screen.getByText('Select a package manager')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Send answer' })).not.toBeInTheDocument();
  expect(screen.getByText('Waiting for an answer')).toBeInTheDocument();
});

it.each(['enum', 'oneOf'] as const)('preserves an empty-string %s choice', async (kind) => {
  const respond = vi.fn().mockResolvedValue(undefined);
  const field =
    kind === 'enum'
      ? { type: 'string' as const, enum: ['', 'pnpm'], enumNames: ['None', 'PNPM'] }
      : {
          type: 'string' as const,
          oneOf: [
            { const: '', title: 'None' },
            { const: 'pnpm', title: 'PNPM' },
          ],
        };
  render(
    <QuestionCard
      question={{ ...question, schema: { properties: { manager: field }, required: ['manager'] } }}
      canAnswer
      respond={respond}
    />,
  );
  await userEvent
    .setup()
    .selectOptions(
      screen.getByLabelText('manager *'),
      screen.getByRole('option', { name: 'None' }),
    );
  await userEvent.setup().click(screen.getByRole('button', { name: 'Send answer' }));
  expect(respond.mock.calls[0]?.[0]).toEqual({ action: 'accept', content: { manager: '' } });
});
