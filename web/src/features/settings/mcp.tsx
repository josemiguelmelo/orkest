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
import { argumentInput, headerInput, stdioInput, mcpSchema } from './mcp-model';
export function McpPage() {
  const { scope, permissions } = useApp();
  const servers = useQuery(mcpQuery(scope));
  const refresh = useRefresh();
  const [editing, setEditing] = useState<McpServer | null | undefined>();
  return (
    <>
      <PageHeader
        title="MCP servers"
        description="HTTP and stdio tools your agents can use in new sessions."
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
                <Badge>
                  {item.transport === 'stdio' ? 'Stdio' : 'HTTP'}
                  {item.hasHeaders || item.hasEnvironment ? ' · Credentials configured' : ''}
                </Badge>
              </div>
              <h3>{item.name}</h3>
              <p className="muted">{item.transport === 'stdio' ? item.command : item.url}</p>
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
          Register an HTTP or stdio server, then select it on an agent definition.
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
      transport: item ? item.transport : 'http',
      command: item?.command ?? '',
      args: JSON.stringify(item?.args ?? []),
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
        onSubmit={async (rawValues) => {
          const values = mcpSchema.parse(rawValues);
          if (
            await action.run(async () => {
              const body = {
                name: values.name,
                transport: values.transport,
                ...(values.transport === 'stdio'
                  ? {
                      command: values.command,
                      args: argumentInput(values.args),
                      ...stdioInput(values.mode, values.headers),
                    }
                  : { url: values.url, ...headerInput(values.mode, values.headers) }),
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
        <Field
          label="Transport"
          hint={
            item
              ? 'Transport cannot be changed after creation.'
              : 'Select how the agent connects to this server.'
          }
        >
          {item ? (
            <input {...form.register('transport')} readOnly />
          ) : (
            <select {...form.register('transport')}>
              <option value="http">HTTP</option>
              <option value="stdio">Stdio</option>
            </select>
          )}
        </Field>
        {form.watch('transport') === 'stdio' ? (
          <>
            <Field
              label="Executable"
              error={form.formState.errors.command?.message}
              hint="Absolute path inside the agent execution environment. Install the server in its execution image. The trusted local executor runs it on the host."
            >
              <input {...form.register('command')} placeholder="/opt/mcp/server" />
            </Field>
            <Field
              label="Arguments (JSON array)"
              error={form.formState.errors.args?.message}
              hint="Literal arguments, without shell expansion. Put credentials in environment variables."
            >
              <textarea
                {...form.register('args')}
                spellCheck={false}
                placeholder={'["--root", "/workspace"]'}
              />
            </Field>
          </>
        ) : (
          <Field label="Endpoint URL" error={form.formState.errors.url?.message}>
            <input {...form.register('url')} placeholder="https://example.com/mcp" />
          </Field>
        )}
        <Field
          label={form.watch('transport') === 'stdio' ? 'Environment variables' : 'Request headers'}
          hint="Stored values are encrypted and write-only."
        >
          <select {...form.register('mode')}>
            <option value="preserve">{item ? 'Keep stored values' : 'No values'}</option>
            <option value="replace">Replace values</option>
            <option value="clear">Clear all values</option>
          </select>
        </Field>
        {form.watch('mode') === 'replace' && (
          <Field label="New values (JSON object)" error={form.formState.errors.headers?.message}>
            <textarea
              {...form.register('headers')}
              autoComplete="off"
              spellCheck={false}
              placeholder={
                form.watch('transport') === 'stdio'
                  ? '{"API_TOKEN":"…"}'
                  : '{"Authorization":"Bearer …"}'
              }
            />
          </Field>
        )}
      </Form>
    </Dialog>
  );
}
