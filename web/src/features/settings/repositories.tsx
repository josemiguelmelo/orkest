import { useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { FolderGit2 } from 'lucide-react';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { CodeRepository, CodingRepository } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { codingIntegrationsQuery, path, repositoriesQuery, useRefresh } from '../../shared/api/queries';
import { Badge, ConfirmButton, CreateButton, Empty, ErrorState, Loading, PageHeader } from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Field } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';

export function RepositoriesPage() {
  const { scope, permissions } = useApp();
  const repos = useQuery(repositoriesQuery(scope));
  const [create, setCreate] = useState(false);
  return <>
    <PageHeader title="Repositories" description="Register repositories from a connected coding integration.">
      {permissions.editWorkspace && <CreateButton onClick={() => setCreate(true)}>Register repository</CreateButton>}
    </PageHeader>
    {repos.isPending ? <Loading /> : repos.isError ? <ErrorState error={repos.error} /> : repos.data.length ? <div className="grid">{repos.data.map((repo) => <RepositoryCard key={repo.id} item={repo} />)}</div> : <Empty title="No repositories registered">Connect a coding integration in workspace settings, then choose repositories here.</Empty>}
    {create && <RepositoryEditor onClose={() => setCreate(false)} />}
  </>;
}

function RepositoryCard({ item }: { item: CodeRepository }) {
  const { scope, permissions } = useApp(); const refresh = useRefresh(); const action = useAction();
  return <article className="resource-card"><div className="split"><FolderGit2 size={22} className="card-icon"/><Badge>{item.private ? 'Private' : 'Public'}</Badge></div>
    <h3>{item.name || item.url.replace('https://github.com/', '').replace(/\.git$/, '')}</h3>
    <a className="text-link" href={item.url} target="_blank" rel="noreferrer">Open repository ↗</a>
    <p className="muted">Default branch: <code>{item.defaultBranch || 'detected at sync'}</code></p>
    {permissions.editWorkspace && <div className="actions">
      <Button variant="outline" size="sm" disabled={action.pending} onClick={() => void action.run(() => data(sdk.cloneCodeRepository({ ...command, path: { ...path(scope), repositoryID: item.id } })), 'Repository cloned.')}>Clone</Button>
      <ConfirmButton label="Sync" title={`Sync checkout to ${item.defaultBranch || 'GitHub default branch'}?`} description="This fetches the registered default branch (or detects GitHub's default branch for older registrations) and hard-resets the shared checkout, discarding local changes." pending={action.pending} onConfirm={async () => { await data(sdk.syncCodeRepository({ ...command, path: { ...path(scope), repositoryID: item.id } })); }} />
      <ConfirmButton title="Remove repository?" description="This removes the repository registration. Referencing components may prevent removal." onConfirm={async () => { await data(sdk.deleteCodeRepository({ ...command, path: { ...path(scope), repositoryID: item.id } })); await refresh(scope); }} />
    </div>}
    {action.error !== undefined && <ErrorState error={action.error} />}{action.message && <p role="status" className="muted">{action.message}</p>}
  </article>;
}

function RepositoryEditor({ onClose }: { onClose: () => void }) {
  const { scope } = useApp(); const refresh = useRefresh(); const action = useAction();
  const integrations = useQuery(codingIntegrationsQuery(scope));
  const [integrationID, setIntegrationID] = useState(''); const [selected, setSelected] = useState<CodingRepository | null>(null);
  const repositories = useInfiniteQuery({ queryKey: ['coding-repository-picker', scope.orgID, scope.workspaceID, integrationID], initialPageParam: 1, queryFn: ({ signal, pageParam }) => data(sdk.browseCodingRepositories({ ...command, path: { ...path(scope), integrationID }, query: { page: pageParam, perPage: 50 }, signal })), getNextPageParam: (lastPage) => lastPage.nextPage, enabled: !!integrationID });
  const repositoryItems = repositories.data?.pages.flatMap((page) => page.repositories) ?? [];
  return <Dialog title="Register repository" description="Select a repository visible to this integration." open onOpenChange={(open) => { if (!open && !action.pending) onClose(); }}>
    <form className="form-stack" onSubmit={(event) => { event.preventDefault(); if (!selected) return; void action.run(async () => { await data(sdk.createCodeRepository({ ...command, path: path(scope), body: { integrationId: integrationID, providerRepositoryId: selected.id } })); await refresh(scope); onClose(); }); }}>
      {integrations.isError && <ErrorState error={integrations.error} />}
      <Field label="Coding integration"><select required value={integrationID} onChange={(event) => { setIntegrationID(event.target.value); setSelected(null); }}><option value="">Choose integration</option>{integrations.data?.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.providerName}</option>)}</select></Field>
      {integrationID && repositories.isPending ? <Loading /> : repositories.isError ? <ErrorState error={repositories.error} /> : integrationID && repositoryItems.length ? <>
        <Field label="Repository"><select required value={selected?.id ?? ''} onChange={(event) => setSelected(repositoryItems.find((repo) => repo.id === event.target.value) ?? null)}><option value="">Choose repository</option>{repositoryItems.map((repo) => <option key={repo.id} value={repo.id}>{repo.name} · {repo.private ? 'Private' : 'Public'} · {repo.defaultBranch}</option>)}</select></Field>
        {repositories.hasNextPage && <Button type="button" variant="outline" disabled={repositories.isFetchingNextPage} onClick={() => void repositories.fetchNextPage()}>{repositories.isFetchingNextPage ? 'Loading…' : 'Load more repositories'}</Button>}
      </> : integrationID ? <Empty title="No repositories found">This integration cannot currently see any repositories.</Empty> : null}
      {action.error !== undefined && <ErrorState error={action.error} />}
      <Button type="submit" disabled={!selected || action.pending}>{action.pending ? 'Registering…' : 'Register repository'}</Button>
    </form>
  </Dialog>;
}
