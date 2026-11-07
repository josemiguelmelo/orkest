import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import { path, useRefresh } from '../../shared/api/queries';
import { QuestionCard } from './question-card';
import type { ChatQuestion } from './question-model';

type Props = {
  questions: ChatQuestion[];
  taskId: string;
  activeRunId: string | undefined;
  canAnswer: boolean;
  direct: boolean;
};

export function QuestionList({ questions, taskId, activeRunId, canAnswer, direct }: Props) {
  const { scope } = useApp();
  const refresh = useRefresh();
  return questions.map((question) => (
    <QuestionCard
      key={`${question.runId}:${question.id}`}
      question={question}
      canAnswer={canAnswer && question.runId === activeRunId}
      respond={async (answer, submissionId) => {
        const body = {
          taskSessionId: question.taskSessionId,
          sessionId: question.sessionId,
          commandId: question.commandId,
          submissionId,
          answer,
        };
        const request = {
          ...command,
          path: { ...path(scope), runID: question.runId, questionID: question.id },
          body,
        };
        if (direct)
          await data(
            sdk.respondDirectAgentConversationQuestion({
              ...request,
              path: { ...request.path, conversationID: taskId },
            }),
          );
        else
          await data(
            sdk.respondTaskRunQuestion({ ...request, path: { ...request.path, taskID: taskId } }),
          );
        await refresh(scope);
      }}
    />
  ));
}
