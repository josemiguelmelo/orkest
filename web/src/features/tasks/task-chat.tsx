import { useEffect, useRef, useState, type ReactNode, type FormEvent } from 'react';
import { useQueries, useQuery } from '@tanstack/react-query';
import {
  ArrowDown,
  ArrowUp,
  Bot,
  Check,
  ChevronDown,
  CircleStop,
  LoaderCircle,
  MessageSquare,
  ShieldCheck,
  Terminal,
} from 'lucide-react';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { Task } from '../../shared/api/generated/types.gen';
import { directChats } from '../../shared/api/direct-chats';
import { directChatEventsQuery, directChatRunsQuery, path, taskRunEventsQuery, taskRunsQuery, useRefresh } from '../../shared/api/queries';
import { Button } from '../../shared/ui/button';
import { ErrorState } from '../../shared/ui/common';
import { Markdown } from '../../shared/ui/markdown';
import { useAction } from '../../shared/use-action';
import {
  chatExecution,
  conversationTurns,
  pendingPermissions,
  toolLabel,
  type ChatTurn,
} from './chat-model';
import { ToolCard } from './tool-card';
import { QuestionList } from './question-list';
import { conversationQuestions, type ChatQuestion } from './question-model';
import type { SessionMode } from './session-choice';

export function TaskChat({ task, direct = false }: { task: Task; direct?: boolean }) {
  const { scope, permissions } = useApp();
  const runs = useQuery(direct ? directChatRunsQuery(scope, task.id) : taskRunsQuery(scope, task.id));
  const latest = runs.data?.[0];
  const latestEvents = useQuery(direct ? directChatEventsQuery(scope, task.id, latest?.runId ?? '') : taskRunEventsQuery(scope, task.id, latest?.runId ?? ''));
  const [selectedSession, setSelectedSession] = useState<string | null>(null);
  const [text, setText] = useState('');
  const [sessionMode, setSessionMode] = useState<SessionMode>('continue');
  const [showJump, setShowJump] = useState(false);
  const timeline = useRef<HTMLDivElement>(null);
  const textarea = useRef<HTMLTextAreaElement>(null);
  const follow = useRef(true);
  const pendingCommand = useRef<{ id: string; text: string; target: string } | null>(null);
  const action = useAction();
  const refresh = useRefresh();
  const sessions = (runs.data ?? []).filter(
    (run, index, all) =>
      all.findIndex(
        (other) => (other.taskSessionId ?? other.runId) === (run.taskSessionId ?? run.runId),
      ) === index,
  );
  const currentSession = latest?.taskSessionId ?? latest?.runId;
  const selectedKey = selectedSession ?? currentSession;
  const archived = !!selectedKey && selectedKey !== currentSession;
  const sessionRuns = (runs.data ?? [])
    .filter((run) => (run.taskSessionId ?? run.runId) === selectedKey)
    .slice()
    .reverse();
  const history = useQueries({
    queries: sessionRuns.map((run) => ({
      ...(direct ? directChatEventsQuery(scope, task.id, run.runId) : taskRunEventsQuery(scope, task.id, run.runId)),
      refetchInterval: run.runId === latest?.runId ? 1500 : false,
    })),
  });
  const events = history.flatMap((query) => query.data ?? []);
  const turns = conversationTurns(events);
  const questions = conversationQuestions(events);
  const currentTurns = conversationTurns(latestEvents.data ?? []);
  const active = [...currentTurns].reverse().find((turn) => turn.status === 'running');
  const queued = currentTurns.filter((turn) => turn.status === 'queued').length;
  const approvals = pendingPermissions(latestEvents.data ?? []);
  const execution = chatExecution(task, latest, latestEvents.data ?? []);
  const nextMode = execution.live ? 'continue' : sessionMode;
  const canSend = permissions.editWorkspace && !archived && execution.canSend && !action.pending;
  const eventSignature = events.map((event) => `${event.runId}:${event.sequence}`).join('|');
  const historyError = history.find((query) => query.error)?.error;
  const loadingHistory = history.some((query) => query.isPending);

  useEffect(() => {
    const element = timeline.current;
    if (element && follow.current) element.scrollTop = element.scrollHeight;
  }, [eventSignature, selectedKey]);

  useEffect(() => {
    const element = textarea.current;
    if (!element) return;
    element.style.height = 'auto';
    element.style.height = `${Math.min(200, Math.max(76, element.scrollHeight))}px`;
  }, [text]);

  function jumpToLatest() {
    follow.current = true;
    setShowJump(false);
    if (timeline.current) timeline.current.scrollTop = timeline.current.scrollHeight;
  }

  async function submitPrompt(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const prompt = text.trim();
    if (!canSend || !prompt || !latest) return;
    const target = execution.live ? latest.runId : nextMode;
    const previous = pendingCommand.current;
    const commandId =
      previous?.text === prompt && previous.target === target ? previous.id : crypto.randomUUID();
    pendingCommand.current = { id: commandId, text: prompt, target };
    follow.current = true;
    await action.run(async () => {
      if (direct) {
        await directChats.prompt(scope, task.id, latest.runId, commandId, prompt);
      } else if (execution.live) {
        await data(
          sdk.sendTaskRunPrompt({
            ...command,
            path: { ...path(scope), taskID: task.id, runID: latest.runId },
            body: { commandId, text: prompt },
          }),
        );
      } else {
        // A run can end before the task-status refresh arrives. Explicitly start
        // another work step in the current workflow stage using the saved session.
        await data(
          sdk.updateTask({
            ...command,
            path: { ...path(scope), taskID: task.id },
            body: {
              title: task.title,
              description: task.description,
              status: task.status,
              stageVersion: task.stageVersion ?? 1,
              startWork: true,
              assigneeAgentId: task.assigneeAgentId ?? '',
              labels: task.labels,
              repositoryIds: task.repositories.map((repository) => repository.id),
              sessionMode: nextMode,
              promptText: prompt,
              commandId,
            },
          }),
        );
      }
      pendingCommand.current = null;
      setText('');
      setSessionMode('continue');
      await refresh(scope);
    });
  }

  async function queueAction(discard: boolean) {
    if (!latest) return;
    await action.run(async () => {
      if (direct) {
        await directChats.queue(scope, task.id, latest.runId, discard);
        await refresh(scope);
        return;
      }
      const request = {
        ...command,
        path: { ...path(scope), taskID: task.id, runID: latest.runId },
      };
      await data(discard ? sdk.discardTaskRunQueue(request) : sdk.resumeTaskRunQueue(request));
      await refresh(scope);
    });
  }

  const hint = archived
    ? 'You are viewing an earlier session.'
    : !permissions.editWorkspace
      ? 'You have view-only access to this conversation.'
      : (task.closed ?? task.status === 'done')
        ? 'This task is done. Its conversation is read-only.'
        : !task.assigneeAgentId
          ? 'Assign an agent to continue this conversation.'
          : !latest
            ? 'Start work above to begin. You can draft your message here.'
            : execution.paused
              ? 'Resolve the paused work before sending another message.'
              : execution.finishing
                ? 'Finishing this turn. Your next message will continue the conversation.'
                : execution.starting
                  ? 'Connecting the agent. Your draft stays here.'
                  : active
                    ? 'Your message will be queued after the current turn.'
                    : nextMode === 'new'
                      ? 'Your next message starts a fresh session. Existing chat and files are kept.'
                      : 'Continue the conversation with the same task context.';
  const waitingForInput = execution.live && !execution.paused && questions.some(
    (question) => question.status === 'pending' && question.runId === latest?.runId,
  );
  const status = archived
    ? 'Previous session'
    : approvals.length && execution.live
      ? 'Approval needed'
      : waitingForInput
        ? 'Waiting for your input'
        : active && execution.live
          ? 'Working'
          : !latest
            ? 'No session yet'
            : execution.label;

  return (
    <section className="agent-chat" aria-label={direct ? 'Direct agent chat' : 'Task chat'}>
      <header className="agent-chat-header">
        <div className="agent-chat-title">
          <span className="agent-avatar">
            <Bot size={19} />
          </span>
          <div>
            <h2>Conversation</h2>
            <p>{direct ? 'Agent · Direct conversation' : 'Agent · Task context preserved'}</p>
            {latest?.executionImage && (
              <p title={latest.executionImage}>
                Prepared execution image · {latest.executionImage.split('@')[1]?.slice(0, 19)}
              </p>
            )}
          </div>
        </div>
        <span
          className={`agent-chat-status ${active && execution.live && !archived ? 'is-working' : ''}`}
          role="status"
        >
          <span />
          {status}
        </span>
        {sessions.length > 1 && (
          <label className="agent-history-select">
            <MessageSquare size={14} />
            <span className="sr-only">Conversation history</span>
            <select
              aria-label="Conversation history"
              value={selectedKey ?? ''}
              onChange={(event) => {
                setSelectedSession(
                  event.target.value === currentSession ? null : event.target.value,
                );
                follow.current = true;
                setShowJump(false);
              }}
            >
              {sessions.map((run, index) => (
                <option key={run.taskSessionId ?? run.runId} value={run.taskSessionId ?? run.runId}>
                  {index === 0 ? 'Current session' : `Previous session ${sessions.length - index}`}{' '}
                  · {new Date(run.updatedAt).toLocaleDateString()}
                </option>
              ))}
            </select>
            <ChevronDown size={12} />
          </label>
        )}
      </header>
      <div className="agent-chat-body">
        <div
          className="agent-chat-timeline"
          role="log"
          aria-label="Conversation messages"
          aria-live="polite"
          aria-relevant="additions text"
          ref={timeline}
          onScroll={(event) => {
            const element = event.currentTarget;
            const atBottom = element.scrollHeight - element.scrollTop - element.clientHeight < 80;
            follow.current = atBottom;
            setShowJump(!atBottom);
          }}
        >
          {runs.error && <ErrorState error={runs.error} retry={() => void runs.refetch()} />}
          {historyError && (
            <ErrorState
              error={historyError}
              retry={() => {
                for (const query of history) void query.refetch();
              }}
            />
          )}
          {(runs.isPending || loadingHistory) && !turns.length ? (
            <div className="agent-chat-empty">
              <LoaderCircle className="chat-spin" size={22} />
              <p>Loading conversation…</p>
            </div>
          ) : (
            !turns.length &&
            !historyError && (
              <div className="agent-chat-empty">
                <span className="agent-empty-icon">
                  <Terminal size={26} />
                </span>
                <h3>{direct ? 'Start a conversation' : 'A conversation that stays with your task'}</h3>
                <p>
                  {direct
                    ? 'Ask this agent a question or discuss an idea. No task is created.'
                    : 'Ask questions, review the agent’s work, and iterate here. Your context carries into every follow-up.'}
                </p>
              </div>
            )
          )}
          {turns.map((turn) => (
            <ConversationTurn
              key={turn.id}
              turn={turn}
              renderQuestion={(question) => (
                <QuestionList
                  questions={[question]}
                  taskId={task.id}
                  activeRunId={latest?.runId}
                  canAnswer={!archived && permissions.editWorkspace && execution.live && !execution.paused}
                  direct={direct}
                />
              )}
            />
          ))}
          {!archived && execution.finishing && (
            <p className="agent-turn-notice">
              <LoaderCircle size={13} className="chat-spin" />
              Saving this turn. The conversation stays open.
            </p>
          )}
          {!archived && latest?.failureReason && (
            <p className="agent-turn-error">{latest.failureReason}</p>
          )}
        </div>
        {showJump && (
          <Button className="agent-jump" variant="outline" size="sm" onClick={jumpToLatest}>
            <ArrowDown size={13} />
            Latest messages
          </Button>
        )}
      </div>
      <footer className="agent-chat-footer">
        {archived && (
          <div className="agent-chat-banner">
            <span>You’re browsing a previous session.</span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setSelectedSession(null);
                follow.current = true;
                setShowJump(false);
              }}
            >
              Return to current session
            </Button>
          </div>
        )}
        {!archived &&
          permissions.editWorkspace &&
          execution.live &&
          approvals.map((approval) => (
            <div className="agent-approval" key={approval.id}>
              <div>
                <ShieldCheck size={17} />
                <strong>Approval needed</strong>
              </div>
              <p>{approval.title}</p>
              <div className="actions">
                {approval.options.map((option) => (
                  <Button
                    size="sm"
                    key={option.id}
                    variant={/reject|deny|cancel/i.test(option.kind) ? 'outline' : 'default'}
                    disabled={action.pending}
                    onClick={() =>
                      void action.run(async () => {
                        if (!latest) return;
                        if (direct) {
                          await directChats.respondPermission(scope, task.id, latest.runId, approval.id, approval.sessionId, option.id);
                        } else await data(
                          sdk.respondTaskRunPermission({
                            ...command,
                            path: { ...path(scope), taskID: task.id, runID: latest.runId },
                            body: {
                              requestId: approval.id,
                              sessionId: approval.sessionId,
                              optionId: option.id,
                            },
                          }),
                        );
                        await refresh(scope);
                      })
                    }
                  >
                    {option.name}
                  </Button>
                ))}
              </div>
            </div>
          ))}
        {!archived && permissions.editWorkspace && execution.paused && (
          <div className="agent-chat-banner agent-chat-paused">
            <div>
              <strong>Work is paused</strong>
              <p>Check interrupted work and uncertain effects before continuing.</p>
            </div>
            <div className="actions">
              <Button
                size="sm"
                variant="outline"
                disabled={action.pending || latest?.recoveryBlocked}
                onClick={() => void queueAction(false)}
              >
                Resume queue
              </Button>
              <Button
                size="sm"
                variant="ghost"
                disabled={action.pending}
                onClick={() => void queueAction(true)}
              >
                Discard queued prompts
              </Button>
            </div>
          </div>
        )}
        <form
          className="agent-composer"
          onSubmit={(event) => void submitPrompt(event)}
          aria-label="Chat composer"
        >
          <label className="sr-only" htmlFor={`task-message-${task.id}`}>
            Message the agent
          </label>
          <textarea
            id={`task-message-${task.id}`}
            ref={textarea}
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder="Ask a question or describe what to change…"
            disabled={
              !permissions.editWorkspace || archived || (task.closed ?? task.status === 'done') || action.pending
            }
            aria-describedby={`chat-hint-${task.id}`}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }}
          />
          <div className="agent-composer-toolbar">
            <div className="agent-composer-context">
              <span className="agent-context-chip">
                <Terminal size={12} />
                {direct ? 'Direct chat' : 'Task context'}
              </span>
              {!direct && latest && !archived && permissions.editWorkspace && !(task.closed ?? task.status === 'done') && (
                <select
                  aria-label="Session for next message"
                  value={nextMode}
                  disabled={
                    execution.live ||
                    execution.starting ||
                    execution.finishing ||
                    execution.paused ||
                    action.pending
                  }
                  onChange={(event) => setSessionMode(event.target.value as SessionMode)}
                >
                  <option value="continue">Continue session</option>
                  <option value="new">New session</option>
                </select>
              )}
              {queued > 0 && !archived && (
                <span className="agent-queue-count">{queued} queued</span>
              )}
            </div>
            <div className="agent-composer-actions">
              {active &&
                !archived &&
                execution.live &&
                !execution.paused &&
                permissions.editWorkspace && (
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Stop agent"
                    title="Stop agent"
                    disabled={action.pending}
                    onClick={() =>
                      void action.run(async () => {
                        if (!latest) return;
                        if (direct) await directChats.cancel(scope, task.id, latest.runId, active.commandId);
                        else await data(
                          sdk.cancelTaskRunPrompt({
                            ...command,
                            path: { ...path(scope), taskID: task.id, runID: latest.runId },
                            body: { commandId: active.commandId },
                          }),
                        );
                        await refresh(scope);
                      })
                    }
                  >
                    <CircleStop size={18} />
                  </Button>
                )}
              <Button
                className="agent-send"
                type="submit"
                size="icon"
                aria-label={active && execution.live ? 'Queue message' : 'Send message'}
                title={active && execution.live ? 'Queue message' : 'Send message'}
                disabled={!canSend || !text.trim()}
              >
                {action.pending ? (
                  <LoaderCircle size={17} className="chat-spin" />
                ) : (
                  <ArrowUp size={18} />
                )}
              </Button>
            </div>
          </div>
        </form>
        <div className="agent-composer-hint">
          <span id={`chat-hint-${task.id}`}>{hint}</span>
          <span className="agent-keyboard-hint">
            Enter to send <span>·</span> Shift + Enter for a new line
          </span>
        </div>
        {!!action.error && <ErrorState error={action.error} />}
      </footer>
    </section>
  );
}

function ConversationTurn({ turn, renderQuestion }: {
  turn: ChatTurn;
  renderQuestion: (question: ChatQuestion) => ReactNode;
}) {
  const taskContext = turn.text.startsWith('Work on the following Orkest task.');
  return (
    <div className="agent-turn">
      {taskContext ? (
        <details className="agent-task-instructions">
          <summary>
            <Terminal size={13} />
            Task instructions
            <ChevronDown size={12} />
          </summary>
          <Markdown text={turn.text} />
        </details>
      ) : (
        <div className="agent-user-message">
          <span className="sr-only">You: </span>
          <Markdown text={turn.text} />
          {turn.status === 'queued' && <span className="agent-message-queued">Queued</span>}
          {turn.status === 'discarded' && <span className="agent-message-queued">Discarded</span>}
        </div>
      )}
      {(turn.blocks.length > 0 || !['queued', 'discarded'].includes(turn.status)) && (
        <article className="agent-response" aria-label="Agent response">
          <span className="agent-response-avatar">
            <Bot size={16} />
          </span>
          <div className="agent-response-content">
            {turn.blocks.map((block, index) =>
              block.kind === 'text' ? (
                <Markdown key={index} text={block.text} />
              ) : block.kind === 'question' ? (
                <div key={`question:${block.question.runId}:${block.question.id}`}>
                  {renderQuestion(block.question)}
                </div>
              ) : (
                <details className="agent-tools" key={index}>
                  <summary>
                    <Terminal size={13} />
                    <span>
                      {block.tools.length === 1
                        ? block.tools[0] && toolLabel(block.tools[0])
                        : `${block.tools.length} tool actions`}
                    </span>
                    <ChevronDown size={12} />
                  </summary>
                  <div>
                    {block.tools.map((tool) => (
                      <ToolCard key={tool.id} tool={tool} />
                    ))}
                  </div>
                </details>
              ),
            )}
            {turn.status === 'running' && (
              <div className="agent-working">
                <LoaderCircle size={13} className="chat-spin" />
                <span>{turn.blocks.length ? 'Working…' : 'Thinking…'}</span>
              </div>
            )}
            {turn.error && <p className="agent-turn-error">{turn.error}</p>}
            {turn.status === 'completed' && (
              <div className="agent-turn-complete">
                <Check size={12} />
                Turn finished<span>Context saved</span>
              </div>
            )}
            {turn.status === 'cancelled' && (
              <p className="agent-turn-notice">Stopped. You can continue the conversation.</p>
            )}
            {turn.status === 'interrupted' && !turn.error && (
              <p className="agent-turn-notice">Interrupted. Your conversation is preserved.</p>
            )}
          </div>
        </article>
      )}
    </div>
  );
}
