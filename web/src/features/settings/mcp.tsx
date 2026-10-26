import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import type { z } from 'zod';
import { Network } from 'lucide-react';
import { useApp } from '../../app/context';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { McpServer } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { mcpQuery, path, useRefresh } from '../../shared/api/queries';
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
import { headerInput, mcpSchema } from './mcp-model';
export function McpPage() {
  const { scope, permissions } = useApp();
  const servers = useQuery(mcpQuery(scope));
  const refresh = useRefresh();
  const [editing, setEditing] = useState<McpServer | null | undefined>();
  return (
    <>
      <PageHeader
        title="MCP servers"
        description="HTTP capabilities your agents can use in future runs."
      >
        {permissions.editWorkspace && (
          <CreateButton onClick={() => setEditing(null)}>New MCP server</CreateButton>
        )}
      </PageHeader>
      {servers.isPending ? (
        <Loading />
      ) : servers.isError ? (
        <ErrorState error={servers.error} />
      ) : servers.data.length ? (
        <div className="grid">
          {servers.data.map((item) => (
            <article className="resource-card" key={item.id}>
              <div className="split">
                <Network size={22} className="card-icon" />
                <Badge>{item.hasHeaders ? 'Headers configured' : 'No headers'}</Badge>
              </div>
              <h3>{item.name}</h3>
              <p className="muted">{item.url}</p>
              {permissions.editWorkspace && (
                <div className="actions">
                  <Button variant="outline" size="sm" onClick={() => setEditing(item)}>
                    Edit
                  </Button>
                  <ConfirmButton
                    title="Delete MCP server?"
                    description="This also removes the server from agent selections. Existing runs retain their startup configuration."
                    onConfirm={async () => {
                      await data(
                        sdk.deleteMcpServer({
                          ...command,
                          path: { ...path(scope), serverID: item.id },
                        }),
                      );
                      await refresh(scope);
                    }}
                  />
                </div>
              )}
            </article>
          ))}
        </div>
      ) : (
        <Empty title="No MCP servers yet">
          Register an HTTP server, then select it on an agent definition.
        </Empty>
      )}
      {editing !== undefined && (
        <McpEditor
          key={editing?.id ?? 'new'}
          item={editing}
          onClose={() => setEditing(undefined)}
        />
      )}
    </>
  );
}
function McpEditor({ item, onClose }: { item: McpServer | null; onClose: () => void }) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const form = useForm({
    resolver: zodResolver(mcpSchema),
    defaultValues: {
      name: item?.name ?? '',
      url: item?.url ?? '',
      mode: 'preserve' as z.infer<typeof mcpSchema>['mode'],
      headers: '',
    },
  });
  return (
    <Dialog
      title={item ? 'Edit MCP server' : 'New MCP server'}
      open
      onOpenChange={(v) => {
        if (!v && !action.pending) onClose();
      }}
    >
      <Form
        form={form}
        error={action.error}
        onSubmit={async (values) => {
          if (
            await action.run(async () => {
              const body = {
                name: values.name,
                url: values.url,
                ...headerInput(values.mode, values.headers),
              };
              if (item)
                await data(
                  sdk.updateMcpServer({
                    ...command,
                    path: { ...path(scope), serverID: item.id },
                    body,
                  }),
                );
              else await data(sdk.createMcpServer({ ...command, path: path(scope), body }));
              await refresh(scope);
            })
          )
            onClose();
        }}
      >
        <Field label="Name" error={form.formState.errors.name?.message}>
          <input {...form.register('name')} />
        </Field>
        <Field label="Endpoint URL" error={form.formState.errors.url?.message}>
          <input {...form.register('url')} placeholder="https://example.com/mcp" />
        </Field>
        <Field label="Request headers" hint="Stored values are write-only and cannot be read back.">
          <select {...form.register('mode')}>
            <option value="preserve">{item ? 'Keep stored headers' : 'No headers'}</option>
            <option value="replace">Replace headers</option>
            <option value="clear">Clear all headers</option>
          </select>
        </Field>
        {form.watch('mode') === 'replace' && (
          <Field label="New headers (JSON object)" error={form.formState.errors.headers?.message}>
            <textarea
              {...form.register('headers')}
              autoComplete="off"
              spellCheck={false}
              placeholder={'{"Authorization":"Bearer …"}'}
            />
          </Field>
        )}
      </Form>
    </Dialog>
  );
}
