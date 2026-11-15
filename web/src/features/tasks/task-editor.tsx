import { queryOptions, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Square, Code2, LoaderCircle } from 'lucide-react';
import type { JSX } from 'react';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { TaskEditorStatus } from '../../shared/api/generated/types.gen';
import { path, type WorkspaceScope } from '../../shared/api/queries';
import { Button } from '../../shared/ui/button';
import { ErrorState } from '../../shared/ui/common';
import { useAction } from '../../shared/use-action';

export function taskEditorQuery(scope: WorkspaceScope, taskID: string) {
  return queryOptions({
    queryKey: ['editor', scope.orgID, scope.workspaceID, taskID],
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      data(sdk.getTaskEditor({ ...command, path: { ...path(scope), taskID }, signal })),
    refetchInterval: (query) => (query.state.data?.state === 'ready' ? 2000 : false),
  });
}

function editorStatus(status?: TaskEditorStatus): string {
  if (!status) return 'Checking editor availability…';
  if (status.state === 'ready')
    return status.mode === 'ro'
      ? 'VS Code is open in read-only mode while agent work is active.'
      : 'VS Code is open with read and write access.';
  if (status.state === 'unavailable') {
    const detail = status.missing?.length
      ? `No checkout is available for ${status.missing.map((repository) => repository.name).join(', ')}.`
      : 'This task needs a durable workspace and the Docker executor.';
    return `VS Code is unavailable. ${detail}`;
  }
  if (status.state === 'failed') return 'VS Code could not start. Try again.';
  return 'VS Code is stopped.';
}

export function TaskEditorAccess({
  taskID,
  active,
  activation,
}: {
  taskID: string;
  active: boolean;
  activation: number;
}): JSX.Element | null {
  const { scope, permissions } = useApp();
  const queryClient = useQueryClient();
  const query = useQuery({
    ...taskEditorQuery(scope, taskID),
    enabled: permissions.editWorkspace,
  });
  const action = useAction();
  const [launchUrl, setLaunchUrl] = useState<string>();
  const editorActive = useRef(false);
  const attemptedActivation = useRef(-1);
  const opening = useRef(false);
  const openEditorRef = useRef<() => void>(() => {});
  const status = query.data;
  const ready = status?.state === 'ready';
  const missing = status?.missing ?? [];

  const queryKey = taskEditorQuery(scope, taskID).queryKey;

  function openEditor() {
    if (opening.current || status?.state === 'unavailable') return;
    opening.current = true;
    editorActive.current = true;
    void action.run(async () => {
      try {
        const result = await data(
          sdk.openTaskEditor({ ...command, path: { ...path(scope), taskID } }),
        );
        queryClient.setQueryData(queryKey, result);
        setLaunchUrl(result.launchUrl);
      } finally {
        opening.current = false;
      }
    });
  }
  function stopEditor() {
    void action.run(async () => {
      const result = await data(
        sdk.stopTaskEditor({ ...command, path: { ...path(scope), taskID } }),
      );
      editorActive.current = false;
      setLaunchUrl(undefined);
      queryClient.setQueryData(queryKey, result);
    });
  }

  const stopOnExit = useCallback(() => {
    if (!editorActive.current) return;
    editorActive.current = false;
    const url = `/api/organizations/${encodeURIComponent(scope.orgID)}/workspaces/${encodeURIComponent(scope.workspaceID)}/tasks/${encodeURIComponent(taskID)}/editor`;
    void fetch(url, { method: 'DELETE', credentials: 'same-origin', keepalive: true }).catch(
      () => undefined,
    );
  }, [scope.orgID, scope.workspaceID, taskID]);

  useEffect(() => {
    openEditorRef.current = openEditor;
  });

  useEffect(() => {
    if (!permissions.editWorkspace || !active || attemptedActivation.current === activation) return;
    attemptedActivation.current = activation;
    openEditorRef.current();
  }, [active, activation, permissions.editWorkspace]);

  useEffect(() => {
    if (!permissions.editWorkspace || !ready) return;
    const timer = window.setTimeout(() => {
      editorActive.current = true;
    }, 0);
    return () => window.clearTimeout(timer);
  }, [permissions.editWorkspace, ready]);

  useEffect(() => {
    if (!permissions.editWorkspace) return;
    window.addEventListener('pagehide', stopOnExit);
    return () => {
      window.removeEventListener('pagehide', stopOnExit);
      stopOnExit();
    };
  }, [permissions.editWorkspace, stopOnExit]);

  if (!permissions.editWorkspace) return null;

  return (
    <section className="task-editor-shell" aria-label="VS Code task workspace">
      <header className="task-editor-chrome">
        <div className="task-editor-workspace-label">
          <Code2 size={17} aria-hidden="true" />
          <span>
            <strong>Task workspace</strong>
            <small>/task</small>
          </span>
        </div>
        <div className="task-editor-chrome-actions">
          <p
            className={`task-editor-status ${
              query.isPending ? 'is-loading' : ready ? 'is-ready' : (status?.state ?? 'is-stopped')
            }`}
            role="status"
          >
            {query.isPending ? 'Checking…' : ready ? 'Online' : editorStatus(status)}
          </p>
          {ready && (
            <Button variant="outline" size="sm" disabled={action.pending} onClick={stopEditor}>
              <Square size={13} /> Stop
            </Button>
          )}
        </div>
      </header>
      {query.isError && <ErrorState error={query.error} retry={() => void query.refetch()} />}
      {missing.length > 0 && (
        <p className="muted task-editor-notice">
          No checkout is available for {missing.map((repository) => repository.name).join(', ')}.
          Opening VS Code will not clone repositories.
        </p>
      )}
      {Boolean(action.error) && <ErrorState error={action.error} />}
      {launchUrl ? (
        <iframe
          className="task-editor-surface"
          title="VS Code task workspace"
          src={launchUrl}
          referrerPolicy="no-referrer"
          allow="clipboard-read; clipboard-write"
        />
      ) : (
        <div className="task-editor-connecting" role="status">
          {action.pending ? (
            <LoaderCircle size={24} className="spin" aria-hidden="true" />
          ) : status?.state === 'unavailable' || status?.state === 'failed' ? (
            <Code2 size={24} aria-hidden="true" />
          ) : (
            <LoaderCircle size={24} className="spin" aria-hidden="true" />
          )}
          <strong>
            {action.pending
              ? 'Connecting to VS Code…'
              : status?.state === 'unavailable'
                ? 'VS Code is unavailable'
                : status?.state === 'failed'
                  ? 'VS Code could not start'
                  : 'Opening your task workspace…'}
          </strong>
          <span>{editorStatus(status)}</span>
          {Boolean(action.error) && (
            <Button variant="outline" onClick={openEditor}>
              Try again
            </Button>
          )}
        </div>
      )}
    </section>
  );
}
