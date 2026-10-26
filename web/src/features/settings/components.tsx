import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Boxes } from 'lucide-react';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { Component } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { componentsQuery, path, repositoriesQuery, useRefresh } from '../../shared/api/queries';
import {
  Badge,
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
  identifier: z.string().trim().min(1).max(100),
  name: z.string().trim().min(1).max(200),
  repositoryId: z.string().min(1, 'Choose a repository.'),
  description: z.string().max(2000),
});
export function ComponentsPage() {
  const { scope, permissions } = useApp();
  const components = useQuery(componentsQuery(scope));
  const repos = useQuery(repositoriesQuery(scope));
  const [editing, setEditing] = useState<Component | null | undefined>();
  return (
    <>
      <PageHeader
        title="Components"
        description="Connect areas of your project to registered repositories."
      >
        {permissions.editWorkspace && (
          <CreateButton onClick={() => setEditing(null)}>New component</CreateButton>
        )}
      </PageHeader>
      {components.isPending ? (
        <Loading />
      ) : components.isError ? (
        <ErrorState error={components.error} />
      ) : components.data.length ? (
        <div className="grid">
          {components.data.map((item) => (
            <article className="resource-card" key={item.id}>
              <Boxes size={22} className="card-icon" />
              <h3>{item.name}</h3>
              <div>
                <Badge>{item.identifier}</Badge>
              </div>
              <p className="muted">{item.description || 'No description'}</p>
              <p className="muted">
                {repos.data?.find((r) => r.id === item.repositoryId)?.url ?? item.repositoryId}
              </p>
              {permissions.editWorkspace && (
                <div className="actions">
                  <Button variant="outline" size="sm" onClick={() => setEditing(item)}>
                    Edit
                  </Button>
                </div>
              )}
            </article>
          ))}
        </div>
      ) : (
        <Empty title="No components yet">
          Register a repository, then create components to organize task context.
        </Empty>
      )}
      {editing !== undefined && (
        <ComponentEditor
          key={editing?.id ?? 'new'}
          item={editing}
          onClose={() => setEditing(undefined)}
        />
      )}
    </>
  );
}
function ComponentEditor({ item, onClose }: { item: Component | null; onClose: () => void }) {
  const { scope } = useApp();
  const repos = useQuery(repositoriesQuery(scope));
  const refresh = useRefresh();
  const action = useAction();
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      identifier: item?.identifier ?? '',
      name: item?.name ?? '',
      description: item?.description ?? '',
      repositoryId: item?.repositoryId ?? '',
    },
  });
  return (
    <Dialog
      title={item ? 'Edit component' : 'New component'}
      open
      onOpenChange={(v) => {
        if (!v && !action.pending) onClose();
      }}
    >
      <Form
        form={form}
        error={action.error}
        onSubmit={async (body) => {
          if (
            await action.run(async () => {
              if (item)
                await data(
                  sdk.updateComponent({
                    ...command,
                    path: { ...path(scope), componentID: item.id },
                    body,
                  }),
                );
              else await data(sdk.createComponent({ ...command, path: path(scope), body }));
              await refresh(scope);
            })
          )
            onClose();
        }}
      >
        <div className="form-grid">
          <Field label="Identifier" error={form.formState.errors.identifier?.message}>
            <input {...form.register('identifier')} />
          </Field>
          <Field label="Name" error={form.formState.errors.name?.message}>
            <input {...form.register('name')} />
          </Field>
        </div>
        <Field label="Description">
          <textarea {...form.register('description')} />
        </Field>
        {repos.isError ? (
          <ErrorState error={repos.error} />
        ) : (
          <Field label="Repository" error={form.formState.errors.repositoryId?.message}>
            <select {...form.register('repositoryId')}>
              <option value="">Choose repository</option>
              {repos.data?.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.url}
                </option>
              ))}
            </select>
          </Field>
        )}
      </Form>
    </Dialog>
  );
}
