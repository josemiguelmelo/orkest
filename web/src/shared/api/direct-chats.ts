import type { TaskRun, TaskRunEvent } from './generated/types.gen';
import type { WorkspaceScope } from './queries';

export type DirectConversation = { id: string; agentId: string; createdAt: string };
const base = (scope: WorkspaceScope) =>
  `/api/organizations/${encodeURIComponent(scope.orgID)}/workspaces/${encodeURIComponent(scope.workspaceID)}/agent-conversations`;

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { credentials: 'same-origin', ...init, headers: { 'Content-Type': 'application/json', ...init?.headers } });
  if (!response.ok) {
    if (init?.method === 'DELETE' && response.status === 409) {
      throw new Error('This chat still has an active or closing run. Wait for it to finish before removing it.');
    }
    throw new Error(`Direct conversation request failed (${response.status}).`);
  }
  if (response.status === 204 || response.status === 202) return undefined as T;
  return (await response.json()) as T;
}

export const directChats = {
  list: (scope: WorkspaceScope) => request<DirectConversation[]>(base(scope)),
  create: (scope: WorkspaceScope, agentId: string) =>
    request<DirectConversation>(base(scope), { method: 'POST', body: JSON.stringify({ agentId }) }),
  remove: (scope: WorkspaceScope, id: string) =>
    request<void>(`${base(scope)}/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  runs: (scope: WorkspaceScope, id: string) => request<TaskRun[]>(`${base(scope)}/${encodeURIComponent(id)}/runs`),
  events: (scope: WorkspaceScope, id: string, runId: string) =>
    request<TaskRunEvent[]>(`${base(scope)}/${encodeURIComponent(id)}/runs/${encodeURIComponent(runId)}/events`),
  prompt: (scope: WorkspaceScope, id: string, runId: string, commandId: string, text: string) =>
    request<void>(`${base(scope)}/${encodeURIComponent(id)}/runs/${encodeURIComponent(runId)}/prompts`, { method: 'POST', body: JSON.stringify({ commandId, text }) }),
  cancel: (scope: WorkspaceScope, id: string, runId: string, commandId: string) =>
    request<void>(`${base(scope)}/${encodeURIComponent(id)}/runs/${encodeURIComponent(runId)}/cancel`, { method: 'POST', body: JSON.stringify({ commandId }) }),
  queue: (scope: WorkspaceScope, id: string, runId: string, discard: boolean) =>
    request<void>(`${base(scope)}/${encodeURIComponent(id)}/runs/${encodeURIComponent(runId)}/queue/${discard ? 'discard' : 'resume'}`, { method: 'POST' }),
  respondPermission: (scope: WorkspaceScope, id: string, runId: string, requestId: string, sessionId: string, optionId: string) =>
    request<void>(`${base(scope)}/${encodeURIComponent(id)}/runs/${encodeURIComponent(runId)}/permissions/respond`, { method: 'POST', body: JSON.stringify({ requestId, sessionId, optionId }) }),
};
