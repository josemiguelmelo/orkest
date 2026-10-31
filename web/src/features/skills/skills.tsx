import { Markdown } from '../../shared/ui/markdown';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { parse } from 'yaml';
import { NotebookText } from 'lucide-react';
import { useApp } from '../../app/context';
import { canManageSkill } from '../../app/permissions';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { Skill } from '../../shared/api/generated/types.gen';
import { command, data } from '../../shared/api/client';
import { path, skillsQuery, useRefresh } from '../../shared/api/queries';
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
const metadata = z.object({
  name: z
    .string()
    .regex(/^[a-z0-9]+(-[a-z0-9]+)*$/)
    .min(1)
    .max(64),
  description: z.string().trim().min(1).max(1024),
});
export function skillName(content: string): string {
  const match = /^---\n([\s\S]*?)\n---(?:\n|$)([\s\S]*)$/.exec(content.trim());
  if (!match?.[1] || !match[2]?.trim())
    throw new Error(
      'Include YAML frontmatter with name and description, followed by Markdown instructions.',
    );
  const value: unknown = parse(match[1], { maxAliasCount: 10 });
  return metadata.parse(value).name;
}
function SkillPreview({ content }: { content: string }) {
  const match = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)([\s\S]*)$/.exec(content.trim());
  let details: z.infer<typeof metadata> | undefined;
  if (match?.[1]) {
    try {
      const result = metadata.safeParse(parse(match[1], { maxAliasCount: 10 }));
      if (result.success) details = result.data;
    } catch {
      // Keep the instructions preview available while frontmatter is being edited.
    }
  }
  return (
    <section className="skill-preview" aria-label="Skill preview">
      {details ? (
        <header className="skill-preview-meta">
          <span className="skill-preview-label">Skill</span>
          <h3>{details.name}</h3>
          <p>{details.description}</p>
        </header>
      ) : (
        <p className="skill-preview-notice">
          Add valid YAML frontmatter with a name and description to complete this skill.
        </p>
      )}
      <div className="skill-preview-instructions">
        <h3>Instructions</h3>
        <Markdown text={match?.[2] ?? content} />
      </div>
    </section>
  );
}
const schema = z.object({
  skillMD: z
    .string()
    .trim()
    .min(1)
    .max(100000)
    .superRefine((content, ctx) => {
      try {
        skillName(content);
      } catch {
        ctx.addIssue({
          code: 'custom',
          message:
            'Include valid frontmatter (lowercase name and description) and Markdown instructions.',
        });
      }
    }),
});
const initial =
  '---\nname: my-skill\ndescription: When this skill should be used.\n---\n\n# Instructions\n\nDescribe how the agent should perform this work.\n';
export function SkillsPage() {
  const app = useApp();
  const { scope, permissions } = app;
  const skills = useQuery(skillsQuery(scope));
  const refresh = useRefresh();
  const [editing, setEditing] = useState<Skill | null | undefined>();
  const [viewing, setViewing] = useState<Skill>();
  const target = scope.workspaceID ? 'workspace' : scope.orgID ? 'organization' : 'global';
  const writable =
    target === 'workspace'
      ? permissions.workspaceAdmin
      : target === 'organization'
        ? permissions.organizationAdmin
        : permissions.platformAdmin;
  const canManage = (skill: Skill) => canManageSkill(skill, { ...scope, permissions });
  return (
    <>
      <PageHeader
        title={`${target[0]?.toUpperCase()}${target.slice(1)} skills`}
        description="Reusable instructions, shared at the right scope. Inherited skills remain visible."
      >
        {writable && <CreateButton onClick={() => setEditing(null)}>New skill</CreateButton>}
      </PageHeader>
      {skills.isPending ? (
        <Loading />
      ) : skills.isError ? (
        <ErrorState error={skills.error} />
      ) : skills.data.length ? (
        <div className="grid">
          {skills.data.map((item) => (
            <article className="resource-card" key={item.id}>
              <div className="split">
                <NotebookText className="card-icon" size={22} />
                <Badge>{item.scope}</Badge>
              </div>
              <h3>{item.id}</h3>
              <div className="actions">
                <Button variant="outline" size="sm" onClick={() => setViewing(item)}>
                  View instructions
                </Button>
                {canManage(item) && (
                  <>
                    <Button variant="outline" size="sm" onClick={() => setEditing(item)}>
                      Edit
                    </Button>
                    <ConfirmButton
                      title="Delete skill?"
                      description="This removes the skill from the catalog and agent selections."
                      onConfirm={async () => {
                        if (item.scope === 'workspace')
                          await data(
                            sdk.deleteWorkspaceSkill({
                              ...command,
                              path: { ...path(scope), skillID: item.id },
                            }),
                          );
                        else if (item.scope === 'organization')
                          await data(
                            sdk.deleteOrganizationSkill({
                              ...command,
                              path: { orgID: scope.orgID, skillID: item.id },
                            }),
                          );
                        else
                          await data(
                            sdk.deleteGlobalSkill({ ...command, path: { skillID: item.id } }),
                          );
                        await refresh(scope);
                      }}
                    />
                  </>
                )}
              </div>
            </article>
          ))}
        </div>
      ) : (
        <Empty title="No skills available">
          Skills are Markdown instructions with a name and description in YAML frontmatter.
        </Empty>
      )}
      {editing !== undefined && (
        <SkillEditor
          key={editing?.id ?? 'new'}
          item={editing}
          target={target}
          onClose={() => setEditing(undefined)}
        />
      )}{' '}
      {viewing && (
        <Dialog
          title={viewing.id}
          description={`${viewing.scope} skill`}
          open
          onOpenChange={(open) => {
            if (!open) setViewing(undefined);
          }}
        >
          <SkillPreview content={viewing.skillMD} />
        </Dialog>
      )}
    </>
  );
}
function SkillEditor({
  item,
  target,
  onClose,
}: {
  item: Skill | null;
  target: Skill['scope'];
  onClose: () => void;
}) {
  const { scope } = useApp();
  const refresh = useRefresh();
  const action = useAction();
  const [preview, setPreview] = useState(false);
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: { skillMD: item?.skillMD ?? initial },
  });
  const effective = item?.scope ?? target;
  return (
    <Dialog
      title={item ? `Edit ${item.id}` : `New ${effective} skill`}
      description="Skill names are globally unique and cannot be renamed after creation."
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
              if (item && skillName(body.skillMD) !== item.id)
                throw new Error('The skill name cannot be changed.');
              if (effective === 'workspace') {
                if (item)
                  await data(
                    sdk.updateWorkspaceSkill({
                      ...command,
                      path: { ...path(scope), skillID: item.id },
                      body,
                    }),
                  );
                else await data(sdk.createWorkspaceSkill({ ...command, path: path(scope), body }));
              } else if (effective === 'organization') {
                if (item)
                  await data(
                    sdk.updateOrganizationSkill({
                      ...command,
                      path: { orgID: scope.orgID, skillID: item.id },
                      body,
                    }),
                  );
                else
                  await data(
                    sdk.createOrganizationSkill({ ...command, path: { orgID: scope.orgID }, body }),
                  );
              } else {
                if (item)
                  await data(
                    sdk.updateGlobalSkill({ ...command, path: { skillID: item.id }, body }),
                  );
                else await data(sdk.createGlobalSkill({ ...command, body }));
              }
              await refresh(scope);
            })
          )
            onClose();
        }}
      >
        <div className="actions">
          <Button variant={!preview ? 'outline' : 'ghost'} onClick={() => setPreview(false)}>
            Source
          </Button>
          <Button variant={preview ? 'outline' : 'ghost'} onClick={() => setPreview(true)}>
            Preview
          </Button>
        </div>
        <div hidden={preview}>
          <Field label="SKILL.md" error={form.formState.errors.skillMD?.message}>
            <textarea rows={15} className="code-input" {...form.register('skillMD')} />
          </Field>
        </div>
        {preview && <SkillPreview content={form.watch('skillMD')} />}
      </Form>
    </Dialog>
  );
}
