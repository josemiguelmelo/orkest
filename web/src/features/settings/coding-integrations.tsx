import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { CodingIntegration, CodingIntegrationInputWritable } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { codingIntegrationsQuery, path, useRefresh } from '../../shared/api/queries';
import { Badge, ConfirmButton, CreateButton, Empty, ErrorState, Loading } from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Field } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';

export function CodingIntegrationsPanel() {
  const { scope, permissions } = useApp();
  const items = useQuery(codingIntegrationsQuery(scope));
  const [editing, setEditing] = useState(false);
  const refresh = useRefresh();
  return <section className="panel">
    <div className="panel-heading"><div><h2>Coding integrations</h2><p className="muted">Encrypted GitHub credentials used to browse and check out workspace repositories.</p></div>
      {permissions.workspaceAdmin && <CreateButton onClick={() => setEditing(true)}>Add integration</CreateButton>}
    </div>
    {items.isPending ? <Loading /> : items.isError ? <ErrorState error={items.error} retry={() => void items.refetch()} /> : items.data.length ? <div className="grid">
      {items.data.map((item) => <IntegrationCard key={item.id} item={item} />)}
    </div> : <Empty title="No coding integrations">Add a GitHub connection to browse repositories, including private repositories.</Empty>}
    {editing && <IntegrationEditor onClose={() => setEditing(false)} onSaved={async () => { await refresh(scope); setEditing(false); }} />}
  </section>;
}

function IntegrationCard({ item }: { item: CodingIntegration }) {
  const { scope, permissions } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const [rotation, setRotation] = useState('');
  return <article className="resource-card">
    <div className="split"><h3>{item.name}</h3><Badge>{item.providerName}</Badge></div>
    <p className="muted">Credential stored · hidden</p>
    <div className="actions">
      <Button size="sm" variant="outline" disabled={action.pending} onClick={() => void action.run(() => data(sdk.validateCodingIntegration({ ...command, path: { ...path(scope), integrationID: item.id } })), 'Connection validated.')}>Validate connection</Button>
      {permissions.workspaceAdmin && <ConfirmButton title="Delete coding integration?" description="Repositories registered through this integration must be removed first." onConfirm={async () => { await data(sdk.deleteCodingIntegration({ ...command, path: { ...path(scope), integrationID: item.id } })); await refresh(scope); }} />}
    </div>
    {permissions.workspaceAdmin && <form className="form-stack" onSubmit={(event)=>{event.preventDefault();if(!rotation)return;void action.run(async()=>{await data(sdk.updateCodingIntegration({...command,path:{...path(scope),integrationID:item.id},body:{name:item.name,providerName:item.providerName,providerConfig:item.providerConfig,credential:rotation}}));setRotation('');await refresh(scope);},'Credential rotated.');}}>
      <Field label="Rotate personal access token"><input type="password" autoComplete="new-password" value={rotation} onChange={(event)=>setRotation(event.target.value)} /></Field>
      <Button type="submit" size="sm" variant="outline" disabled={!rotation||action.pending}>Save new token</Button>
    </form>}
    {action.error !== undefined && <ErrorState error={action.error} />}{action.message && <p role="status" className="muted">{action.message}</p>}
  </article>;
}

function IntegrationEditor({ onClose, onSaved }: { onClose: () => void; onSaved: () => Promise<void> }) {
  const { scope } = useApp();
  const providers = useQuery({ queryKey: ['coding-providers'], queryFn: ({ signal }) => data(sdk.listCodingProviders({ ...command, signal })) });
  const action = useAction();
  const [name, setName] = useState('');
  const [credential, setCredential] = useState('');
  const [providerName, setProviderName] = useState('github');
  const [providerConfig, setProviderConfig] = useState<Record<string, unknown>>({});
  const selected = providers.data?.find((provider) => provider.providerName === providerName);
  const fields = (selected?.configSchema.properties ?? {}) as Record<string, { type?: string; title?: string; description?: string; default?: unknown }>;
  return <form className="form-stack" onSubmit={(event) => { event.preventDefault(); void action.run(async () => {
    const input: CodingIntegrationInputWritable = { name, providerName, credential, providerConfig };
    await data(sdk.createCodingIntegration({ ...command, path: path(scope), body: input }));
    setCredential(''); await onSaved();
  }); }}>
    {providers.isError && <ErrorState error={providers.error} />}
    <Field label="Integration name"><input required maxLength={200} value={name} onChange={(event) => setName(event.target.value)} placeholder="Engineering GitHub" /></Field>
    <Field label="Provider"><select value={providerName} onChange={(event) => { setProviderName(event.target.value); setProviderConfig({}); }}>{providers.data?.map((provider) => <option key={provider.providerName} value={provider.providerName}>{provider.providerName}</option>)}</select></Field>
    {Object.entries(fields).map(([key, schema]) => <Field key={key} label={schema.title ?? key}><input value={textValue(providerConfig[key] ?? schema.default)} placeholder={schema.description ?? ''} onChange={(event) => setProviderConfig((current) => ({ ...current, [key]: event.target.value }))} /></Field>)}
    <Field label="Personal access token"><input required type="password" autoComplete="new-password" value={credential} onChange={(event) => setCredential(event.target.value)} /></Field>
    {action.error !== undefined && <ErrorState error={action.error} />}
    <div className="actions"><Button type="submit" disabled={action.pending || providers.isPending}>{action.pending ? 'Connecting…' : 'Connect GitHub'}</Button><Button type="button" variant="outline" onClick={onClose}>Cancel</Button></div>
  </form>;
}

function textValue(value: unknown): string {
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return `${value}`;
  return '';
}
