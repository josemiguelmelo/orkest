import { expect, it } from 'vitest';
import type { TaskRunEvent } from '../../shared/api/generated/types.gen';
import { conversationQuestions } from './question-model';

const request = {
  type: 'question_requested',
  commandId: 'command',
  requestId: 'question',
  sessionId: 'native',
  question: {
    message: 'Choose',
    schema: { type: 'object', properties: { choice: { type: 'string' } } },
  },
};
function events(...payloads: Record<string, unknown>[]): TaskRunEvent[] {
  return payloads.map((payload, index) => ({ runId: 'run', sequence: index + 1, payload }));
}

it('reconstructs answered questions after reconnect without showing another pending form', () => {
  const questions = conversationQuestions(
    events(request, {
      type: 'question_answered',
      requestId: 'question',
      questionResponse: { answer: { action: 'accept', content: { choice: 'npm' } } },
    }),
  );
  expect(questions).toHaveLength(1);
  expect(questions[0]?.status).toBe('answered');
  expect(questions[0]?.answer?.content).toEqual({ choice: 'npm' });
});

it('preserves unresolved questions visibly when the runtime stops', () => {
  expect(conversationQuestions(events(request, { type: 'runtime_stopped' }))[0]?.status).toBe(
    'uncertain',
  );
  expect(
    conversationQuestions(events(request, { type: 'cancel_requested', commandId: 'command' }))[0]
      ?.status,
  ).toBe('cancelled');
});

it('does not mark answers from finished commands uncertain during a later restart', () => {
  const questions = conversationQuestions(
    events(
      request,
      {
        type: 'question_answered',
        requestId: 'question',
        questionResponse: { answer: { action: 'accept', content: { choice: 'npm' } } },
      },
      { type: 'prompt_completed', commandId: 'command' },
      { type: 'runtime_stopped' },
    ),
  );
  expect(questions[0]?.status).toBe('answered');
});

it('scopes repeated question IDs to their original runs', () => {
  const questions = conversationQuestions([
    ...events(request),
    { runId: 'another', sequence: 1, payload: request },
  ]);
  expect(questions).toHaveLength(2);
});
