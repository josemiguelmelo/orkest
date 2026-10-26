import { StrictMode, useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';

type ACPEvent = {
  at: string;
  type: string;
  sequence: number;
  text?: string;
  stopReason?: string;
  update?: {
    kind: string;
    text?: string;
    title?: string;
    status?: string;
    toolCallId?: string;
  };
};

type TranscriptItem =
  | { kind: 'user' | 'assistant'; text: string; sequence: number }
  | { kind: 'tool'; id: string; title: string; status: string; sequence: number };

function buildTranscript(events: ACPEvent[]): TranscriptItem[] {
  const items: TranscriptItem[] = [];
  const tools = new Map<string, Extract<TranscriptItem, { kind: 'tool' }>>();

  for (const event of events) {
    if (event.type === 'prompt_accepted') {
      items.push({ kind: 'user', text: event.text ?? '', sequence: event.sequence });
      continue;
    }
    if (event.type === 'prompt_completed') continue;

    const update = event.update;
    if (update?.kind === 'text' && update.text) {
      const last = items.at(-1);
      if (last?.kind === 'assistant') last.text += update.text;
      else items.push({ kind: 'assistant', text: update.text, sequence: event.sequence });
      continue;
    }

    if (update?.kind === 'tool_call' || update?.kind === 'tool_call_update') {
      const id = update.toolCallId ?? `event-${event.sequence}`;
      let tool = tools.get(id);
      if (!tool) {
        tool = {
          kind: 'tool',
          id,
          title: update.title ?? 'Tool call',
          status: update.status ?? 'pending',
          sequence: event.sequence,
        };
        tools.set(id, tool);
        items.push(tool);
      }
      if (update.title && update.title !== 'shell' && update.title !== 'read') tool.title = update.title;
      if (update.status) tool.status = update.status;
    }
  }
  return items;
}

function App() {
  const [events, setEvents] = useState<ACPEvent[]>([]);
  const [error, setError] = useState('');

  useEffect(() => {
    fetch('/test.json')
      .then((response) => {
        if (!response.ok) throw new Error(`Could not load test.json (${response.status})`);
        return response.text();
      })
      .then((body) => setEvents(body.split('\n').filter(Boolean).map((line) => JSON.parse(line) as ACPEvent)))
      .catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Could not load ACP transcript'));
  }, []);

  const transcript = useMemo(() => buildTranscript(events), [events]);
  const prompt = events.find((event) => event.type === 'prompt_accepted');
  const finished = events.some((event) => event.type === 'prompt_completed');

  return (
    <main style={{ maxWidth: 900, margin: '0 auto', padding: '32px 24px', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', color: '#202124', lineHeight: 1.6 }}>
      <header style={{ borderBottom: '1px solid #ddd', paddingBottom: 16, marginBottom: 24 }}>
        <div style={{ fontSize: 13, color: '#666' }}>ORKEST / TASK xxxx / RUN run-2</div>
        <h1 style={{ fontSize: 23, margin: '8px 0' }}>Agent conversation</h1>
        <div style={{ fontSize: 13, color: '#666' }}>OpenCode · ACP transcript · {events.length || '…'} events · {finished ? 'Completed' : 'Loading'}</div>
      </header>
      {error && <p role="alert">Error: {error}</p>}
      {!events.length && !error && <p>Loading transcript…</p>}
      <section aria-label="Conversation transcript">
        {transcript.map((item, index) => (
          <article key={item.kind === 'tool' ? item.id : `${item.kind}-${item.sequence}`} style={{ margin: '0 0 20px', paddingLeft: 16, borderLeft: item.kind === 'user' ? '3px solid #5574c9' : item.kind === 'tool' ? '3px solid #bbb' : '3px solid #4b9b76' }}>
            <div style={{ fontSize: 12, color: '#666', marginBottom: 5 }}>
              {item.kind === 'tool' ? `TOOL · ${item.status.toUpperCase()}` : item.kind === 'user' ? 'YOU' : 'OPENCODE'}
              {' · '}{item.sequence.toString().padStart(3, '0')}
            </div>
            {item.kind === 'tool' ? (
              <details>
                <summary style={{ cursor: 'pointer', whiteSpace: 'pre-wrap' }}>{item.title}</summary>
                <div style={{ padding: '8px 0 0 16px', color: '#666' }}>Tool call ID: {item.id}<br />Status: {item.status}</div>
              </details>
            ) : (
              <div style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{item.text}</div>
            )}
            {index === transcript.length - 1 && !finished && <span aria-label="assistant is responding"> ▍</span>}
          </article>
        ))}
      </section>
      {finished && <footer style={{ borderTop: '1px solid #ddd', paddingTop: 14, marginTop: 28, color: '#666', fontSize: 13 }}>Turn finished · {events.at(-1)?.stopReason ?? 'end_turn'}</footer>}
      {!prompt && !error && <p>Waiting for prompt…</p>}
    </main>
  );
}

createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>);
