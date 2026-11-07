import type { TaskRunEvent } from '../../shared/api/generated/types.gen';

export type FormField = {
  type: 'string' | 'boolean' | 'integer' | 'number' | 'array';
  title?: string;
  description?: string;
  enum?: string[];
  enumNames?: string[];
  oneOf?: { const: string; title?: string; description?: string }[];
  items?: FormField;
  minLength?: number;
  maxLength?: number;
  minimum?: number;
  maximum?: number;
};

export type QuestionAnswer = {
  action: 'accept' | 'decline' | 'cancel';
  content?: Record<string, unknown>;
};
export type ChatQuestion = {
  id: string;
  runId: string;
  sessionId: string;
  taskSessionId: string;
  commandId: string;
  message: string;
  schema: { properties: Record<string, FormField>; required?: string[] };
  status: 'pending' | 'answered' | 'cancelled' | 'uncertain';
  answer?: QuestionAnswer;
};

export function conversationQuestions(events: TaskRunEvent[]): ChatQuestion[] {
  const questions = new Map<string, ChatQuestion>();
  const completed = new Set<string>();
  for (const event of events) {
    const payload = event.payload;
    const key = `${event.runId}:${String(payload.requestId)}`;
    if (['prompt_completed', 'prompt_failed', 'prompt_interrupted'].includes(String(payload.type)))
      completed.add(`${event.runId}:${String(payload.commandId)}`);
    if (payload.type === 'question_requested' && payload.question) {
      const question = payload.question as { message: string; schema: ChatQuestion['schema'] };
      questions.set(key, {
        id: String(payload.requestId),
        runId: event.runId,
        sessionId: String(payload.sessionId),
        taskSessionId: typeof payload.taskSessionId === 'string' ? payload.taskSessionId : '',
        commandId: String(payload.commandId),
        message: question.message,
        schema: question.schema,
        status: 'pending',
      });
    }
    const question = questions.get(key);
    if (question && payload.type === 'question_answered') {
      question.answer = (payload.questionResponse as { answer: QuestionAnswer }).answer;
      question.status = 'answered';
    }
    if (question && payload.type === 'question_cancelled' && question.status === 'pending')
      question.status = 'cancelled';
    for (const item of questions.values()) {
      if (item.runId !== event.runId) continue;
      if (payload.type === 'question_delivery_uncertain' && item.id === payload.requestId)
        item.status = 'uncertain';
      if (
        ['runtime_stopped', 'session_restored'].includes(String(payload.type)) &&
        (!payload.commandId || item.commandId === payload.commandId) &&
        !completed.has(`${item.runId}:${item.commandId}`)
      )
        item.status = 'uncertain';
      if (payload.type === 'prompt_interrupted' && item.commandId === payload.commandId)
        item.status = 'uncertain';
      if (
        [
          'prompt_completed',
          'prompt_failed',
          'cancel_requested',
          'run_completed',
          'run_failed',
        ].includes(String(payload.type)) &&
        item.status === 'pending' &&
        (!payload.commandId || item.commandId === payload.commandId)
      )
        item.status = 'cancelled';
    }
  }
  return [...questions.values()];
}

export function fieldOptions(field: FormField) {
  return (
    field.oneOf?.map((option) => ({ value: option.const, label: option.title ?? option.const })) ??
    field.enum?.map((value, index) => ({ value, label: field.enumNames?.[index] ?? value })) ??
    []
  );
}
