import { useEffect, useState } from 'react';
import { Check, ChevronDown, FileText, Pencil, Search, Terminal, X } from 'lucide-react';
import { toolLabel, type ToolActivity } from './chat-model';

function Preview({ label, text }: { label: string; text: string }) {
  let formatted = text;
  try {
    formatted = JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    /* Plain text preview. */
  }
  return (
    <section className="agent-tool-preview">
      <strong>{label}</strong>
      <pre>{formatted}</pre>
    </section>
  );
}

export function ToolCard({ tool }: { tool: ToolActivity }) {
  const [now, setNow] = useState(() => Date.now());
  const active = ['pending', 'in_progress'].includes(tool.status);
  useEffect(() => {
    if (!active || tool.startedAt === undefined) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [active, tool.startedAt]);
  const Icon =
    tool.toolKind === 'read'
      ? FileText
      : tool.toolKind === 'edit'
        ? Pencil
        : tool.toolKind === 'search'
          ? Search
          : Terminal;
  const elapsed =
    tool.startedAt !== undefined && (active || tool.endedAt !== undefined)
      ? Math.max(0, Math.round(((tool.endedAt ?? now) - tool.startedAt) / 1000))
      : undefined;
  return (
    <details className="agent-tool-card">
      <summary className="agent-tool-row">
        <Icon size={13} />
        <span>{toolLabel(tool)}</span>
        <small>
          {tool.status.replaceAll('_', ' ')}
          {elapsed !== undefined ? ` · ${elapsed}s` : ''}
        </small>
        {tool.status === 'completed' ? (
          <Check size={12} />
        ) : tool.status === 'failed' ? (
          <X size={12} />
        ) : (
          <ChevronDown size={12} />
        )}
      </summary>
      <div className="agent-tool-body">
        {tool.truncated && <p role="note">Preview truncated. Full output is not retained.</p>}
        {tool.input !== undefined && <Preview label="Input" text={tool.input} />}
        {tool.output !== undefined && <Preview label="Result" text={tool.output} />}
        {tool.locations?.length ? (
          <section>
            <strong>Affected files</strong>
            <ul>
              {tool.locations.map((l, i) => (
                <li key={i}>
                  <code>
                    {l.path}
                    {l.line !== undefined ? `:${l.line}` : ''}
                  </code>
                </li>
              ))}
            </ul>
          </section>
        ) : null}
        {tool.content?.map((content, i) => (
          <section key={i}>
            {content.truncated && <p role="note">File or content preview truncated.</p>}
            {content.kind === 'diff' ? (
              <>
                <strong>{content.path || 'File change'}</strong>
                <div className="agent-tool-diff">
                  <Preview label="Before" text={content.oldText ?? ''} />
                  <Preview label="After" text={content.newText ?? ''} />
                </div>
              </>
            ) : content.kind === 'text' ? (
              <Preview label="Output" text={content.text ?? ''} />
            ) : (
              <p>Unsupported tool content.</p>
            )}
          </section>
        ))}
        {tool.input === undefined &&
          tool.output === undefined &&
          !tool.content?.length &&
          !tool.locations?.length && <p>No additional details available.</p>}
      </div>
    </details>
  );
}
