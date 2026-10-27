import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Check, CircleAlert, GitBranch, KeyRound, Plus, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { CodingIntegration, CodingIntegrationInputWritable } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { codingIntegrationsQuery, path, useRefresh } from '../../shared/api/queries';
import { Badge, Empty, ErrorState, Loading } from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Field } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';
import { useApp } from '../../app/context';

export function CodingIntegrationsPanel() {
  const { scope, permissions } = useApp();
  const items = useQuery(codingIntegrationsQuery(scope));
  const [editing, setEditing] = useState(false);
  const refresh = useRefresh();
  return (
    <section className="panel integration-panel">
      <div className="panel-heading">
        <div>
          <p className="eyebrow">REPOSITORY ACCESS</p>
          <h2>Coding integrations</h2>
          <p className="muted">Connect GitHub once to browse and securely check out workspace repositories.</p>
        </div>
        {permissions.workspaceAdmin && <Button onClick={() => setEditing(true)}><Plus size={16} /> Add connection</Button>}
      </div>
      <div className="integration-trust-note"><ShieldCheck size={17} /><span>Credentials are encrypted and never shown again after saving.</span></div>
      {items.isPending ? <Loading /> : items.isError ? <ErrorState error={items.error} retry={() => void items.refetch()} /> : items.data.length ? (
        <div className="integration-grid">{items.data.map((item) => <IntegrationCard key={item.id} item={item} />)}</div>
      ) : (
        <Empty title="Connect a coding provider">A connection lets workspace members find repositories they can access. Your credential stays private to workspace administrators.</Empty>
      )}
      {editing && <IntegrationEditor onClose={() => setEditing(false)} onSaved={async () => { await refresh(scope); setEditing(false); }} />}
    </section>
  );
}

function IntegrationCard({ item }: { item: CodingIntegration }) {
  const { scope, permissions } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const [rotation, setRotation] = useState('');
  const [rotating, setRotating] = useState(false);
  return (
    <article className="integration-card">
      <div className="integration-card-top">
        <div className="integration-provider-icon"><GitBranch size={21} /></div>
        <div className="integration-card-title"><h3>{item.name}</h3><p>{item.providerName} · credential saved</p></div>
        <Badge><span className="status-dot" /> Configured</Badge>
      </div>
      <div className="integration-card-actions">
        {permissions.workspaceAdmin && <Button size="sm" variant="outline" disabled={action.pending} onClick={() => void action.run(() => data(sdk.validateCodingIntegration({ ...command, path: { ...path(scope), integrationID: item.id } })), 'Connection verified.') }>
          <Check size={14} /> Test connection
        </Button>}
        {permissions.workspaceAdmin && <Button size="sm" variant="ghost" onClick={() => setRotating((value) => !value)}><RefreshCw size={14} /> Rotate key</Button>}
        {permissions.workspaceAdmin && <DeleteIntegration item={item} />}
      </div>
      {rotating && permissions.workspaceAdmin && <form className="rotation-form" onSubmit={(event) => { event.preventDefault(); if (!rotation) return; void action.run(async () => { await data(sdk.updateCodingIntegration({ ...command, path: { ...path(scope), integrationID: item.id }, body: { name: item.name, providerName: item.providerName, providerConfig: item.providerConfig, credential: rotation } })); setRotation(''); setRotating(false); await refresh(scope); }, 'Credential updated.'); }}>
        <Field label="New personal access token" hint="The saved token cannot be viewed. Paste a replacement to rotate it.">
          <input type="password" autoComplete="new-password" value={rotation} onChange={(event) => setRotation(event.target.value)} placeholder="Paste new token" />
        </Field>
        <div className="actions"><Button size="sm" type="submit" disabled={!rotation || action.pending}>{action.pending ? 'Saving…' : 'Save new token'}</Button><Button size="sm" variant="outline" onClick={() => { setRotation(''); setRotating(false); }}>Cancel</Button></div>
      </form>}
      {action.error !== undefined && <ErrorState error={action.error} />}{action.message && <p role="status" className="success-message"><Check size={14} /> {action.message}</p>}
    </article>
  );
}

function DeleteIntegration({ item }: { item: CodingIntegration }) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<unknown>();
  const [pending, setPending] = useState(false);
  async function remove() {
    setPending(true); setError(undefined);
    try { await data(sdk.deleteCodingIntegration({ ...command, path: { ...path(scope), integrationID: item.id } })); await refresh(scope); setOpen(false); }
    catch (reason) { setError(reason); }
    finally { setPending(false); }
  }
  return <>
    <Button size="sm" variant="ghost" className="integration-delete" aria-label={`Delete ${item.name}`} onClick={() => setOpen(true)}><Trash2 size={14} /></Button>
    <Dialog title="Delete this connection?" description="Repositories registered through this connection must be removed first." open={open} onOpenChange={(value) => { if (!pending) setOpen(value); }}>
      {error !== undefined && <ErrorState error={error} />}
      <div className="form-actions"><Button variant="outline" disabled={pending} onClick={() => setOpen(false)}>Keep connection</Button><Button variant="destructive" disabled={pending} onClick={() => void remove()}>{pending ? 'Deleting…' : 'Delete connection'}</Button></div>
    </Dialog>
  </>;
}

function IntegrationEditor({ onClose, onSaved }: { onClose: () => void; onSaved: () => Promise<void> }) {
  const { scope } = useApp();
  const providers = useQuery({ queryKey: ['coding-providers'], queryFn: ({ signal }) => data(sdk.listCodingProviders({ ...command, signal })) });
  const action = useAction();
  const [name, setName] = useState('');
  const [credential, setCredential] = useState('');
  const providerName = 'github';
  const [providerConfig, setProviderConfig] = useState<Record<string, unknown>>({});
  const selected = providers.data?.find((provider) => provider.providerName === providerName);
  const fields = (selected?.configSchema.properties ?? {}) as Record<string, { type?: string; title?: string; description?: string; default?: unknown }>;
  return (
    <Dialog title="Connect GitHub" description="Add a workspace connection so your team can browse and register repositories." open onOpenChange={(open) => { if (!open && !action.pending) onClose(); }}>
      <form className="form-stack" onSubmit={(event) => { event.preventDefault(); void action.run(async () => {
        const input: CodingIntegrationInputWritable = { name: name.trim(), providerName, credential, providerConfig };
        await data(sdk.createCodingIntegration({ ...command, path: path(scope), body: input }));
        setCredential(''); await onSaved();
      }); }}>
        <div className="provider-callout"><div className="integration-provider-icon"><GitBranch size={22} /></div><div><strong>GitHub Cloud</strong><p>Use a fine-grained personal access token with repository metadata read access.</p></div></div>
        {providers.isError && <ErrorState error={providers.error} retry={() => void providers.refetch()} />}
        <Field label="Connection name" hint="Choose a name your team will recognize, such as Engineering GitHub."><input required maxLength={200} value={name} onChange={(event) => setName(event.target.value)} placeholder="Engineering GitHub" autoFocus /></Field>
        {selected && Object.entries(fields).map(([key, schema]) => <Field key={key} label={schema.title ?? key} {...(schema.description ? { hint: schema.description } : {})}><input type={schema.type === 'string' && /secret|token/i.test(key) ? 'password' : 'text'} value={textValue(providerConfig[key] ?? schema.default)} onChange={(event) => setProviderConfig((current) => ({ ...current, [key]: event.target.value }))} /></Field>)}
        <Field label="Personal access token" hint="GitHub requires Metadata: read. Contents: read is needed to clone private repositories."><div className="input-with-icon"><KeyRound size={16} /><input required type="password" autoComplete="new-password" value={credential} onChange={(event) => setCredential(event.target.value)} placeholder="github_pat_…" /></div></Field>
        <div className="integration-security-note"><CircleAlert size={15} /><span>Only workspace administrators can manage this credential. Members can use it to browse accessible repositories.</span></div>
        {action.error !== undefined && <ErrorState error={action.error} />}
        <div className="form-actions"><Button type="button" variant="outline" onClick={onClose}>Cancel</Button><Button type="submit" disabled={action.pending || providers.isPending}>{action.pending ? 'Connecting…' : 'Connect integration'}</Button></div>
      </form>
    </Dialog>
  );
}

function textValue(value: unknown): string {
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return `${value}`;
  return '';
}
