import { expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { WorkflowStage } from '../../shared/api/generated/types.gen';
import { StageArtifacts } from './stage-artifacts';

for (const method of [
  'hasPointerCapture',
  'setPointerCapture',
  'releasePointerCapture',
  'scrollIntoView',
]) {
  Object.defineProperty(HTMLElement.prototype, method, { configurable: true, value: () => false });
}
const source: WorkflowStage = {
  id: 'plan',
  name: 'Planning',
  description: '',
  responsibleAgentId: '',
  artifactOutputs: [{ name: 'report', kind: 'plan' }],
};
const stage: WorkflowStage = {
  id: 'build',
  name: 'Build',
  description: '',
  responsibleAgentId: '',
  artifactInputs: [{ name: 'brief', sourceStageId: 'plan', outputName: 'removed', required: true }],
};
it('shows unavailable bindings and lets the user select a replacement without losing input settings', async () => {
  const onChange = vi.fn();
  render(
    <StageArtifacts stage={stage} stages={[source, stage]} disabled={false} onChange={onChange} />,
  );
  const user = userEvent.setup();
  await user.click(screen.getByText('Artifacts'));
  expect(screen.getByLabelText('Source output')).toHaveTextContent('Unavailable output (removed)');
  await user.click(screen.getByLabelText('Source output'));
  expect(screen.getByText('Kind: plan')).toBeInTheDocument();
  await user.click(screen.getByRole('option', { name: 'Planning / report' }));
  expect(onChange).toHaveBeenCalledWith({
    artifactInputs: [
      { name: 'brief', sourceStageId: 'plan', outputName: 'report', required: true },
    ],
  });
});
it('keeps artifact editing and removal disabled for read-only users', async () => {
  render(
    <StageArtifacts
      stage={{ ...stage, artifactOutputs: source.artifactOutputs ?? [] }}
      stages={[source, stage]}
      disabled
      onChange={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByText('Artifacts'));
  for (const label of ['Output name', 'Output kind', 'Input name', 'Source output']) {
    expect(screen.getByLabelText(label)).toBeDisabled();
  }
  for (const name of ['Add output', 'Add input', 'Remove output 1', 'Remove input 1']) {
    expect(screen.getByRole('button', { name })).toBeDisabled();
  }
  expect(screen.getByRole('checkbox')).toBeDisabled();
});
