import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { FolderGit2 } from 'lucide-react';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { CodeRepository } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { path, repositoriesQuery, useRefresh } from '../../shared/api/queries';
import {
  Badge,
  ConfirmButton,
  CreateButton,
  Empty,
  ErrorState,
  Loading,
  PageHeader,
} from '../../shared/ui/common';
import { Button } from '../../shared/ui/button';
import { Dialog } from '../../shared/ui/dialog';
import { Field, Form } from '../../shared/ui/form';
import { useAction } from '../../shared/use-action';
const schema = z.object({
  url: z.url().refine((value) => {
    const url = new URL(value);
    return (
      url.protocol === 'https:' &&
      url.hostname === 'github.com' &&
      !url.username &&
      !url.password &&
      !url.search &&
      !url.hash &&
      url.pathname.split('/').filter(Boolean).length === 2
    );
  }, 'Enter a public GitHub HTTPS repository URL.'),
});
export function RepositoriesPage() {
  const { scope, permissions } = useApp();
  const repos = useQuery(repositoriesQuery(scope));
  const [create, setCreate] = useState(false);
  return (
    <>
      <PageHeader
        title="Repositories"
        description="Register public GitHub repositories for this workspace."
      >
        {permissions.editWorkspace && (
          <CreateButton onClick={() => setCreate(true)}>Register repository</CreateButton>
        )}
      </PageHeader>
      {repos.isPending ? (
        <Loading />
      ) : repos.isError ? (
        <ErrorState error={repos.error} />
      ) : repos.data.length ? (
        <div className="grid">
          {repos.data.map((repo) => (
            <RepositoryCard key={repo.id} item={repo} />
          ))}
        </div>
      ) : (
        <Empty title="No repositories registered">
          Add a public GitHub repository to connect your code.
        </Empty>
      )}
      {create && <RepositoryEditor onClose={() => setCreate(false)} />}
    </>
  );
}
function RepositoryCard({ item }: { item: CodeRepository }) {
  const { scope, permissions } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  return (
    <article className="resource-card">
      <div className="split">
        <FolderGit2 size={22} className="card-icon" />
        <Badge>{item.providerName}</Badge>
      </div>
      <h3>{item.url.replace('https://github.com/', '').replace(/\.git$/, '')}</h3>
      <a className="text-link" href={item.url} target="_blank" rel="noreferrer">
        Open repository ↗
      </a>
      <p className="muted">ID: {item.id}</p>
      {permissions.editWorkspace && (
        <div className="actions">
          <Button
            variant="outline"
            size="sm"
            disabled={action.pending}
            onClick={() => {
              void action.run(
                () =>
                  data(
                    sdk.cloneCodeRepository({
                      ...command,
                      path: { ...path(scope), repositoryID: item.id },
                    }),
                  ),
                'Repository cloned.',
              );
            }}
          >
            Clone
          </Button>
          <ConfirmButton
            label="Sync main"
            title="Reset checkout to remote main?"
            description="This fetches remote main and hard-resets the server checkout, discarding local changes."
            pending={action.pending}
            onConfirm={async () => {
              await data(
                sdk.syncCodeRepositoryMain({
                  ...command,
                  path: { ...path(scope), repositoryID: item.id },
                }),
              );
            }}
          />
          <ConfirmButton
            title="Remove repository?"
            description="This removes the repository registration. Referencing components may prevent removal."
            onConfirm={async () => {
              await data(
                sdk.deleteCodeRepository({
                  ...command,
                  path: { ...path(scope), repositoryID: item.id },
                }),
              );
              await refresh(scope);
            }}
          />
        </div>
      )}
      {action.error !== undefined && <ErrorState error={action.error} />}{' '}
      {action.message && (
        <p role="status" className="muted">
          {action.message}
        </p>
      )}
    </article>
  );
}
function RepositoryEditor({ onClose }: { onClose: () => void }) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { url: '' } });
  return (
    <Dialog
      title="Register repository"
      description="Public GitHub HTTPS repositories are supported. Private repository credentials are not implemented."
      open
      onOpenChange={(v) => {
        if (!v && !action.pending) onClose();
      }}
    >
      <Form
        form={form}
        error={action.error}
        label="Register repository"
        onSubmit={async (values) => {
          if (
            await action.run(async () => {
              await data(
                sdk.createCodeRepository({
                  ...command,
                  path: path(scope),
                  body: { providerName: 'github', url: values.url },
                }),
              );
              await refresh(scope);
            })
          )
            onClose();
        }}
      >
        <Field label="Repository URL" error={form.formState.errors.url?.message}>
          <input
            placeholder="https://github.com/organization/repository"
            {...form.register('url')}
          />
        </Field>
      </Form>
    </Dialog>
  );
}
