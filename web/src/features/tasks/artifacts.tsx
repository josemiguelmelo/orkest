import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { ArtifactRevision } from '../../shared/api/generated/types.gen';
import { path } from '../../shared/api/queries';
import { ErrorState, Loading } from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Markdown } from '../../shared/ui/markdown';

export function TaskArtifacts({ taskID }: { taskID: string }) {
  const { scope } = useApp();
  const apiPath = { ...path(scope), taskID };
  const query = useQuery({
    queryKey: ['artifacts', scope.orgID, scope.workspaceID, taskID],
    queryFn: () => data(sdk.listTaskArtifacts({ ...command, path: apiPath })),
    refetchInterval: 5000,
  });
  return (
    <section className="panel">
      <h3>Artifacts</h3>
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorState error={query.error} />
      ) : query.data.length === 0 ? (
        <p className="muted">No artifacts published yet.</p>
      ) : (
        query.data.map((revision) => (
          <ArtifactCard
            key={revision.artifactId}
            taskID={taskID}
            revision={revision}
          />
        ))
      )}
    </section>
  );
}
function ArtifactCard({
  taskID,
  revision,
}: {
  taskID: string;
  revision: ArtifactRevision;
}) {
  const { scope } = useApp();
  const [history, setHistory] = useState(false);
  const query = useQuery({
    queryKey: [
      'artifact-history',
      scope.orgID,
      scope.workspaceID,
      taskID,
      revision.artifactId,
      revision.number,
    ],
    queryFn: () =>
      data(
        sdk.listArtifactRevisions({
          ...command,
          path: { ...path(scope), taskID, artifactID: revision.artifactId },
        }),
      ),
    enabled: history,
  });
  return (
    <details className="task-context-section">
      <summary>
        {revision.title} · {revision.kind} · v{revision.number}
      </summary>
      <div className="task-context-content">
        <ArtifactContent revision={revision} />
        <p className="muted">
          Published in {revision.stageId}
          {revision.outputName ? ` · ${revision.outputName}` : ''}
        </p>
        <Button variant="ghost" onClick={() => setHistory(!history)}>
          Revision history
        </Button>
        {history &&
          (query.isPending ? (
            <Loading />
          ) : query.isError ? (
            <ErrorState error={query.error} />
          ) : (
            query.data.map((item) => (
              <details key={item.id}>
                <summary>
                  v{item.number} · {item.title}
                </summary>
                <ArtifactContent revision={item} />
              </details>
            ))
          ))}
      </div>
    </details>
  );
}

function ArtifactContent({ revision }: { revision: ArtifactRevision }) {
  const content = revision.content;
  if (content.url)
    return (
      <a href={content.url} target="_blank" rel="noreferrer">
        Open {revision.title}
      </a>
    );
  if (content.mediaType.split(';')[0] === 'text/markdown')
    return <Markdown text={content.text ?? ''} />;
  return <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{content.text}</pre>;
}
