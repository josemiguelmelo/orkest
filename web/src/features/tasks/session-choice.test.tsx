import { expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { useSessionChoice } from './session-choice';

function Choice() {
  const choice = useSessionChoice();
  const [result, setResult] = useState('waiting');
  return (
    <>
      <button onClick={() => void choice.choose().then((mode) => setResult(mode ?? 'cancelled'))}>
        Start
      </button>
      <output>{result}</output>
      {choice.dialog}
    </>
  );
}

it('requires an explicit session choice before starting another step', async () => {
  const user = userEvent.setup();
  render(<Choice />);
  await user.click(screen.getByRole('button', { name: 'Start' }));
  expect(screen.getByRole('status', { hidden: true })).toHaveTextContent('waiting');
  await user.click(screen.getByRole('button', { name: 'Continue session' }));
  expect(screen.getByRole('status', { hidden: true })).toHaveTextContent('continue');
  await user.click(screen.getByRole('button', { name: 'Start' }));
  await user.click(screen.getByRole('button', { name: 'New session' }));
  expect(screen.getByRole('status', { hidden: true })).toHaveTextContent('new');
});

it('cancels starting work when the session dialog is dismissed', async () => {
  const user = userEvent.setup();
  render(<Choice />);
  await user.click(screen.getByRole('button', { name: 'Start' }));
  await user.keyboard('{Escape}');
  expect(screen.getByRole('status', { hidden: true })).toHaveTextContent('cancelled');
});
