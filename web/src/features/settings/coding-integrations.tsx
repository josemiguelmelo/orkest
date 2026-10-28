import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Check, CircleAlert, GitBranch, KeyRound, Plus, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { CodingIntegration, CodingIntegrationInputWritable, CodingProvider } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { codingIntegrationsQuery, path, useRefresh } from '../../shared/api/queries';
import { Badge, Empty, ErrorState, Loading } from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Field } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';
import { useApp } from '../../app/context';

const knownCodingProviders: CodingProvider[] = [
  { providerName: 'github', configSchema: { type: 'object', properties: { host: { type: 'string', default: 'github.com' } } } },
  { providerName: 'gitlab_cloud', configSchema: { type: 'object', properties: {} } },
  { providerName: 'gitlab_server', configSchema: { type: 'object', required: ['baseUrl'], properties: { baseUrl: { type: 'string', format: 'uri', title: 'GitLab Server URL', description: 'HTTPS base URL of your self-hosted GitLab instance.' } } } },
];

export function CodingIntegrationsPanel() {
  const { scope, permissions } = useApp();
  const items = useQuery(codingIntegrationsQuery(scope));
  const providers = useQuery({ queryKey: ['coding-providers'], queryFn: ({ signal }) => data(sdk.listCodingProviders({ ...command, signal })) });
  const [editing, setEditing] = useState(false);
  const refresh = useRefresh();
  const providerCatalog = mergeCodingProviders(providers.data);
  const availableProviders = providerCatalog.filter((provider) => !items.data?.some((item) => item.providerName === provider.providerName));
  return (
    <section className="panel integration-panel">
      <div className="panel-heading">
        <div>
          <p className="eyebrow">REPOSITORY ACCESS</p>
          <h2>Coding integrations</h2>
          <p className="muted">Connect one account per provider to browse and securely check out workspace repositories.</p>
        </div>
        {permissions.workspaceAdmin && items.isSuccess && <Button disabled={availableProviders.length === 0} onClick={() => setEditing(true)}><Plus size={16} /> Add connection</Button>}
      </div>
      <div className="integration-trust-note"><ShieldCheck size={17} /><span>Credentials are encrypted and never shown again after saving.</span></div>
      {providers.isError && <ErrorState error={providers.error} retry={() => void providers.refetch()} />}
      {items.isPending ? <Loading /> : items.isError ? <ErrorState error={items.error} retry={() => void items.refetch()} /> : items.data.length ? (
        <div className="integration-grid">{items.data.map((item) => <IntegrationCard key={item.id} item={item} />)}</div>
      ) : (
        <Empty title="Connect a coding provider">A connection lets workspace members find repositories they can access. Your credential stays private to workspace administrators.</Empty>
      )}
      {editing && <IntegrationEditor onClose={() => setEditing(false)} onSaved={async () => { await refresh(scope); await providers.refetch(); setEditing(false); }} />}
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
        <div className="integration-card-title"><h3>{item.name}</h3><p>{providerLabel(item.providerName)} · credential saved</p></div>
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
  const [providerName, setProviderName] = useState<CodingIntegrationInputWritable['providerName'] | ''>('');
  const [providerConfig, setProviderConfig] = useState<Record<string, unknown>>({});
  const existing = useQuery(codingIntegrationsQuery(scope));
  const providerCatalog = mergeCodingProviders(providers.data);
  const available = providerCatalog.filter((provider) => !existing.data?.some((item) => item.providerName === provider.providerName));
  const effectiveProviderName = available.some((provider) => provider.providerName === providerName) ? providerName : available[0]?.providerName ?? '';
  const selected = providerCatalog.find((provider) => provider.providerName === effectiveProviderName);
  const fields = (selected?.configSchema.properties ?? {}) as Record<string, { type?: string; title?: string; description?: string; default?: unknown }>;
  const requiredFields = (selected?.configSchema.required ?? []) as string[];
  const label = providerLabel(effectiveProviderName);
  const gitlab = effectiveProviderName.startsWith('gitlab_');
  return (
    <Dialog title="Add coding connection" description="Choose a provider, then add a private workspace connection for repository access." open onOpenChange={(open) => { if (!open && !action.pending) onClose(); }}>
      <form className="form-stack" onSubmit={(event) => { event.preventDefault(); void action.run(async () => {
        if (!effectiveProviderName) return;
        const input: CodingIntegrationInputWritable = { name: name.trim(), providerName: effectiveProviderName, credential, providerConfig };
        await data(sdk.createCodingIntegration({ ...command, path: path(scope), body: input }));
        setCredential(''); await onSaved();
      }); }}>
        <div className="provider-picker-label">Choose a provider</div>
        <div className="provider-choice-grid" role="radiogroup" aria-label="Coding provider">
          {providerCatalog.map((provider) => {
            const connected = existing.data?.some((item) => item.providerName === provider.providerName) ?? false;
            const selectedProvider = effectiveProviderName === provider.providerName;
            return <button key={provider.providerName} type="button" role="radio" aria-checked={selectedProvider} disabled={connected || existing.isPending} className={`provider-choice ${selectedProvider ? 'selected' : ''}`} onClick={() => { setProviderName(provider.providerName); setProviderConfig({}); }}>
              <ProviderIcon provider={provider.providerName} />
              <span className="provider-choice-copy"><strong>{providerLabel(provider.providerName)}</strong><small>{providerDescription(provider.providerName)}</small>{connected && <small className="provider-connected">Already connected</small>}</span>
              {selectedProvider && <span className="provider-selected-mark"><Check size={13} /></span>}
            </button>;
          })}
        </div>
        <div className="provider-config-section">
          <div className="provider-config-heading"><ProviderIcon provider={effectiveProviderName} /><div><strong>Connection details</strong><span>{gitlab ? 'Personal access token authentication' : 'Fine-grained personal access token authentication'}</span></div></div>
        </div>
        {providers.isError && <ErrorState error={providers.error} retry={() => void providers.refetch()} />}
        <Field label="Connection name" hint={`Choose a name your team will recognize, such as Engineering ${label}.`}><input required maxLength={200} value={name} onChange={(event) => setName(event.target.value)} placeholder={`Engineering ${label}`} autoFocus /></Field>
        {selected && Object.entries(fields).map(([key, schema]) => <Field key={key} label={schema.title ?? key} {...(schema.description ? { hint: schema.description } : {})}><input required={requiredFields.includes(key)} type={schema.type === 'string' && /secret|token/i.test(key) ? 'password' : 'text'} value={textValue(providerConfig[key] ?? schema.default)} onChange={(event) => setProviderConfig((current) => ({ ...current, [key]: event.target.value }))} /></Field>)}
        <Field label="Personal access token" hint={gitlab ? 'The token must allow API access and read/write repository access.' : 'GitHub requires Metadata: read. Contents: read is needed to clone private repositories.'}><div className="input-with-icon"><KeyRound size={16} /><input required type="password" autoComplete="new-password" value={credential} onChange={(event) => setCredential(event.target.value)} placeholder={gitlab ? 'glpat-…' : 'github_pat_…'} /></div></Field>
        <div className="integration-security-note"><CircleAlert size={15} /><span>Only workspace administrators can manage this credential. Members can use it to browse accessible repositories.</span></div>
        {action.error !== undefined && <ErrorState error={action.error} />}
        <div className="form-actions"><Button type="button" variant="outline" onClick={onClose}>Cancel</Button><Button type="submit" disabled={action.pending || providers.isPending || existing.isPending || !effectiveProviderName}>{action.pending ? 'Connecting…' : 'Connect integration'}</Button></div>
      </form>
    </Dialog>
  );
}

function providerLabel(provider: string): string {
  if (provider === 'github') return 'GitHub Cloud';
  if (provider === 'gitlab_cloud') return 'GitLab';
  if (provider === 'gitlab_server') return 'GitLab Server';
  return provider;
}

function providerDescription(provider: string): string {
  if (provider === 'github') return 'Connect repositories on github.com';
  if (provider === 'gitlab_cloud') return 'Connect repositories on gitlab.com';
  if (provider === 'gitlab_server') return 'Connect your self-managed GitLab instance';
  return provider;
}

function ProviderIcon({ provider }: { provider: string }) {
  if (provider === 'github') return <span className="provider-brand-icon github-brand" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="M12 2.1a9.9 9.9 0 0 0-3.13 19.29c.5.09.68-.22.68-.48v-1.7c-2.78.61-3.37-1.18-3.37-1.18-.45-1.16-1.11-1.47-1.11-1.47-.91-.62.07-.61.07-.61 1 .07 1.53 1.03 1.53 1.03.9 1.52 2.35 1.08 2.92.83.09-.65.35-1.08.63-1.33-2.22-.25-4.55-1.11-4.55-4.94 0-1.09.39-1.98 1.03-2.68-.1-.25-.45-1.27.1-2.64 0 0 .84-.27 2.72 1.02a9.46 9.46 0 0 1 4.96 0c1.89-1.29 2.72-1.02 2.72-1.02.55 1.37.2 2.39.1 2.64.64.7 1.03 1.59 1.03 2.68 0 3.84-2.34 4.69-4.57 4.94.36.31.68.92.68 1.86v2.57c0 .27.18.58.69.48A9.9 9.9 0 0 0 12 2.1Z" /></svg></span>;
  return <span className={`provider-brand-icon gitlab-brand ${provider === 'gitlab_server' ? 'gitlab-server-brand' : ''}`} aria-hidden="true"><svg viewBox="0 0 24 24"><path d="m12 21.2 4.1-12.6H7.9L12 21.2Z"/><path d="m12 21.2-8.5-6.1a1 1 0 0 1-.36-1.12L5.7 6.4c.2-.6 1.05-.6 1.25 0l1 3.1h8.1l1-3.1c.2-.6 1.05-.6 1.25 0l2.56 7.58a1 1 0 0 1-.36 1.12L12 21.2Z"/></svg>{provider === 'gitlab_server' && <i />}</span>;
}

function mergeCodingProviders(fromAPI?: CodingProvider[]): CodingProvider[] {
  const providers = new Map(knownCodingProviders.map((provider) => [provider.providerName, provider]));
  for (const provider of fromAPI ?? []) providers.set(provider.providerName, provider);
  return [...providers.values()].sort((a, b) => a.providerName.localeCompare(b.providerName));
}

function textValue(value: unknown): string {
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return `${value}`;
  return '';
}
