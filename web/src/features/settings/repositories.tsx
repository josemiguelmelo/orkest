import { useEffect, useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { ArrowUpRight, FolderGit2, GitBranch, Globe2, LockKeyhole, Search } from 'lucide-react';
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
    <PageHeader title="Repositories" description="Manage the codebases your workspace uses for tasks and agent checkouts.">
      {permissions.editWorkspace && <CreateButton onClick={() => setCreate(true)}>Register repository</CreateButton>}
    </PageHeader>
    {repos.isPending ? <Loading /> : repos.isError ? <ErrorState error={repos.error} retry={() => void repos.refetch()} /> : repos.data.length ? <>
      <div className="repository-list-heading"><span>{repos.data.length} {repos.data.length === 1 ? 'repository' : 'repositories'}</span><span className="muted">Available to workspace tasks</span></div>
      <div className="grid">{repos.data.map((repo) => <RepositoryCard key={repo.id} item={repo} />)}</div>
    </> : <div className="repository-empty"><div className="repository-empty-icon"><FolderGit2 size={24} /></div><h2>Start with a repository</h2><p className="muted">Connect a coding provider to choose a repository. Agents use registered repositories when preparing task workspaces.</p>{permissions.editWorkspace && <Button onClick={() => setCreate(true)}>Add your first repository</Button>}</div>}
    {create && <RepositoryEditor onClose={() => setCreate(false)} />}
  </>;
}

function RepositoryCard({ item }: { item: CodeRepository }) {
  const { scope, permissions } = useApp(); const refresh = useRefresh(); const action = useAction();
  return <article className="repository-card">
    <div className="repository-card-heading"><div className="repository-card-icon"><FolderGit2 size={19} /></div><div className="repository-card-title"><h3>{item.name || item.url.replace(/^https:\/\/[^/]+\//, '').replace(/\.git$/, '')}</h3><a className="text-link" href={item.url.replace(/\.git$/, '')} target="_blank" rel="noreferrer">Open on {providerLabel(item.providerName)} <ArrowUpRight size={13} /></a></div><Badge>{item.private ? <><LockKeyhole size={11} /> Private</> : <><Globe2 size={11} /> Public</>}</Badge></div>
    <div className="repository-branch"><GitBranch size={14} /><span>Default branch</span><code>{item.defaultBranch || 'Detected on sync'}</code></div>
    {permissions.editWorkspace && <div className="actions">
      <Button variant="outline" size="sm" disabled={action.pending} onClick={() => void action.run(() => data(sdk.cloneCodeRepository({ ...command, path: { ...path(scope), repositoryID: item.id } })), 'Repository cloned.')}>Clone checkout</Button>
      <ConfirmButton triggerVariant="outline" label="Sync" title={`Sync checkout to ${item.defaultBranch || 'provider default branch'}?`} description="Sync resets the shared checkout to the repository’s default branch and discards local changes in that shared checkout." pending={action.pending} onConfirm={async () => { await data(sdk.syncCodeRepository({ ...command, path: { ...path(scope), repositoryID: item.id } })); }} />
      <ConfirmButton title="Remove repository?" description="This removes the repository registration. Tasks linked to it must be updated first." onConfirm={async () => { await data(sdk.deleteCodeRepository({ ...command, path: { ...path(scope), repositoryID: item.id } })); await refresh(scope); }} />
    </div>}
    {action.error !== undefined && <ErrorState error={action.error} />}{action.message && <p role="status" className="muted">{action.message}</p>}
  </article>;
}

function RepositoryEditor({ onClose }: { onClose: () => void }) {
  const { scope } = useApp(); const refresh = useRefresh(); const action = useAction();
  const integrations = useQuery(codingIntegrationsQuery(scope));
  const [integrationID, setIntegrationID] = useState(''); const [selected, setSelected] = useState<CodingRepository[]>([]);
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState(search.trim());
  useEffect(() => {
    const timeout = window.setTimeout(() => setDebouncedSearch(search.trim()), 300);
    return () => window.clearTimeout(timeout);
  }, [search]);
  const repositories = useInfiniteQuery({ queryKey: ['coding-repository-picker', scope.orgID, scope.workspaceID, integrationID, debouncedSearch], initialPageParam: 1, queryFn: ({ signal, pageParam }) => data(sdk.browseCodingRepositories({ ...command, path: { ...path(scope), integrationID }, query: { page: pageParam, perPage: 50, ...(debouncedSearch ? { search: debouncedSearch } : {}) }, signal })), getNextPageParam: (lastPage) => lastPage.nextPage, enabled: !!integrationID });
  const repositoryItems = repositories.data?.pages.flatMap((page) => page.repositories) ?? [];
  const filteredRepositories = repositoryItems;
  return <Dialog title="Register repository" description="Select a repository visible to this integration." open onOpenChange={(open) => { if (!open && !action.pending) onClose(); }}>
    <form className="form-stack" onSubmit={(event) => { event.preventDefault(); if (!selected.length) return; void action.run(async () => { for (const repo of selected) await data(sdk.createCodeRepository({ ...command, path: path(scope), body: { integrationId: integrationID, providerRepositoryId: repo.id } })); await refresh(scope); onClose(); }); }}>
      {integrations.isError && <ErrorState error={integrations.error} retry={() => void integrations.refetch()} />}
      {!integrations.isPending && integrations.data?.length === 0 ? <div className="picker-empty"><KeyRoundIcon /><strong>No connections yet</strong><span>Ask a workspace administrator to connect a coding provider in Settings.</span></div> : <Field label="1. Choose a connection" hint="Repositories are listed according to the access granted to this connection."><select required value={integrationID} onChange={(event) => { setIntegrationID(event.target.value); setSelected([]); setSearch(''); }}><option value="">Select a provider connection</option>{integrations.data?.map((item) => <option key={item.id} value={item.id}>{item.name} · {providerLabel(item.providerName)}</option>)}</select></Field>}
      {integrationID && repositories.isPending ? <Loading /> : repositories.isError ? <ErrorState error={repositories.error} /> : integrationID && repositoryItems.length ? <>
        <Field label="2. Choose a repository"><div className="input-with-icon"><Search size={15} /><input type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search repositories" /></div></Field>
        <div className="repository-picker-list" aria-label="Available repositories">{filteredRepositories.map((repo) => <button type="button" aria-pressed={selected.some((item) => item.id === repo.id)} className={`repository-picker-option ${selected.some((item) => item.id === repo.id) ? 'selected' : ''}`} key={repo.id} onClick={() => setSelected((items) => items.some((item) => item.id === repo.id) ? items.filter((item) => item.id !== repo.id) : [...items, repo])}><span className="picker-repo-icon"><FolderGit2 size={17} /></span><span className="picker-repo-copy"><strong>{repo.name}</strong><small><GitBranch size={12} /> {repo.defaultBranch}</small></span><Badge>{repo.private ? <><LockKeyhole size={11} /> Private</> : <><Globe2 size={11} /> Public</>}</Badge>{selected.some((item) => item.id === repo.id) && <CheckMark />}</button>)}{filteredRepositories.length === 0 && <p className="picker-no-match">No repositories match “{search}”.</p>}</div>
        <p className="picker-count">Showing {repositoryItems.length}{repositories.hasNextPage ? ' of more' : ''} repositories</p>
        {repositories.hasNextPage && <Button className="picker-load-more" type="button" variant="outline" disabled={repositories.isFetchingNextPage} onClick={() => void repositories.fetchNextPage()}>{repositories.isFetchingNextPage ? 'Loading repositories…' : 'Load more repositories'}</Button>}
        {selected.length > 0 && <div className="selected-repository"><CheckMark /><span><strong>{selected.length} {selected.length === 1 ? 'repository' : 'repositories'}</strong> ready to add</span></div>}
      </> : integrationID ? <Empty title="No repositories available">This connection cannot see any repositories. Check its permissions or connect another provider account.</Empty> : null}
      {action.error !== undefined && <ErrorState error={action.error} />}
      <div className="form-actions"><Button type="button" variant="outline" onClick={onClose}>Cancel</Button><Button type="submit" disabled={!selected.length || action.pending}>{action.pending ? 'Adding…' : `Add ${selected.length ? `${selected.length} ` : ''}${selected.length === 1 ? 'repository' : 'repositories'}`}</Button></div>
    </form>
  </Dialog>;
}

function KeyRoundIcon() { return <LockKeyhole size={18} />; }
function CheckMark() { return <span className="selected-check">✓</span>; }
function providerLabel(provider: string) { return provider === 'github' ? 'GitHub Cloud' : provider === 'gitlab_cloud' ? 'GitLab' : provider === 'gitlab_server' ? 'GitLab Server' : provider; }
