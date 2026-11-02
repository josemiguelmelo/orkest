import { useState } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  Bot,
  Check,
  Plus,
  Save,
  Trash2,
  Workflow as WorkflowIcon,
} from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type { WorkspaceWorkflow, WorkflowStage } from '../../shared/api/generated/types.gen';
import { agentsQuery, path, useRefresh, workflowsQuery } from '../../shared/api/queries';
import { Button } from '../../shared/ui/button';
import { ConfirmButton, ErrorState, Loading, PageHeader } from '../../shared/ui/common';
import { Field } from '../../shared/ui/form';
import { Dialog } from '../../shared/ui/dialog';
import { useAction } from '../../shared/use-action';

export function useWorkflows() {
  const { scope } = useApp();
  return useQuery(workflowsQuery(scope));
}
export function useWorkflow(workflowID?: string) {
  const query = useWorkflows();
  const definition = query.data?.find((item) => item.id === workflowID) ?? query.data?.[0];
  const stages = definition?.stages ?? [];
  return {
    ...query,
    data: definition,
    workflows: query.data ?? [],
    stages,
    statuses: stages.map((s) => s.id),
    statusNames: Object.fromEntries(stages.map((s) => [s.id, s.name])) as Record<string, string>,
  };
}
export function WorkflowPage() {
  const query = useWorkflows();
  const [selectedID, setSelectedID] = useState<string>();
  if (query.isPending) return <Loading />;
  if (query.isError)
    return (
      <ErrorState
        error={query.error}
        retry={() => {
          void query.refetch();
        }}
      />
    );
  const selected = query.data.find((item) => item.id === selectedID) ?? query.data[0];
  if (!selected) return <p className="muted">This workspace has no workflows yet.</p>;
  return (
    <WorkflowEditor
      key={selected.id}
      definition={selected}
      workflows={query.data}
      onSelect={setSelectedID}
    />
  );
}
function WorkflowEditor({
  definition,
  workflows,
  onSelect,
}: {
  definition: WorkspaceWorkflow;
  workflows: WorkspaceWorkflow[];
  onSelect: (id: string) => void;
}) {
  const { scope, permissions } = useApp();
  const [draft, setDraft] = useState(() => normalizeDefinition(definition));
  const [creating, setCreating] = useState(false);
  const [createName, setCreateName] = useState('');
  const agents = useQuery(agentsQuery(scope));
  const refresh = useRefresh();
  const action = useAction();
  const disabled = !permissions.workspaceAdmin || action.pending;
  const dirty = JSON.stringify(draft) !== JSON.stringify(definition);
  function patch(id: string, values: Partial<WorkflowStage>) {
    setDraft((d) => ({
      ...d,
      stages: d.stages.map((s) => (s.id === id ? { ...s, ...values } : s)),
    }));
  }
  function reorder(index: number, delta: number) {
    setDraft((d) => {
      const stages = [...d.stages];
      const target = index + delta;
      const stage = stages[index];
      if (!stage || target < 0 || target >= stages.length) return d;
      stages.splice(index, 1);
      stages.splice(target, 0, stage);
      const final = stages[stages.length - 1];
      if (final) {
        final.autoStart = false;
        final.autoAdvance = false;
      }
      return { ...d, stages };
    });
  }
  return (
    <>
      <PageHeader title="Workflows" description="Manage task flows and their stage sequences.">
        {permissions.workspaceAdmin && (
          <div className="task-actions">
            <Button onClick={() => setCreating(true)}>
              <Plus size={15} /> New workflow
            </Button>
            {workflows.length > 1 && (
              <ConfirmButton
                triggerSize="default"
                title={`Delete ${definition.name}?`}
                description="A workflow cannot be deleted while tasks or source integrations use it."
                onConfirm={async () => {
                  await data(
                    sdk.deleteWorkspaceWorkflow({
                      ...command,
                      path: { ...path(scope), workflowID: definition.id },
                    }),
                  );
                  onSelect(workflows.find((item) => item.id !== definition.id)?.id ?? '');
                  await refresh(scope);
                }}
                pending={action.pending}
              />
            )}
            <Button
              disabled={action.pending || !dirty}
              onClick={() => {
                void action.run(async () => {
                  const saved = await data(
                    sdk.saveWorkspaceWorkflow({
                      ...command,
                      path: { ...path(scope), workflowID: definition.id },
                      body: draft,
                    }),
                  );
                  setDraft(normalizeDefinition(saved));
                  await refresh(scope);
                });
              }}
            >
              {action.pending ? <Save size={15} className="spin" /> : <Save size={15} />}
              Save workflow
            </Button>
          </div>
        )}
      </PageHeader>
      {action.error && <ErrorState error={action.error} />}
      <div className="workflow-toolbar">
        <Field label="Select workflow">
          <select className="workflow-select" value={definition.id} onChange={(event) => onSelect(event.target.value)}>
            {workflows.map((workflow) => (
              <option key={workflow.id} value={workflow.id}>
                {workflow.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Workflow name">
          <input
            value={draft.name}
            disabled={disabled}
            onChange={(event) => setDraft({ ...draft, name: event.target.value })}
          />
        </Field>
      </div>
      {creating && (
        <Dialog title="Create workflow" open onOpenChange={(open) => !open && setCreating(false)}>
          <Field label="Workflow name">
            <input value={createName} onChange={(event) => setCreateName(event.target.value)} />
          </Field>
          <Button
            disabled={!createName.trim() || action.pending}
            onClick={() => {
              void action.run(async () => {
                const created = await data(
                  sdk.createWorkspaceWorkflow({
                    ...command,
                    path: path(scope),
                    body: { name: createName.trim() },
                  }),
                );
                setCreating(false);
                setCreateName('');
                onSelect(created.id);
                await refresh(scope);
              });
            }}
          >
            Create workflow
          </Button>
        </Dialog>
      )}
      {agents.isError && <ErrorState error={agents.error} />}
      <div className="workflow-toolbar">
        <div className="workflow-toolbar-title">
          <WorkflowIcon size={17} />
          <div>
            <strong>Stage sequence</strong>
            <span>Tasks start in the first stage and source-closed tasks go to the final stage.</span>
          </div>
        </div>
      </div>
      <div className="workflow-board-hint">
        <span>Tasks flow from left to right</span>
        <span>Successful work advances only where enabled; failed work stays in its stage.</span>
      </div>
      <div className="workflow-board">
        {draft.stages.map((stage, index) => (
          <article className="workflow-stage-column" key={stage.id}>
            <div className="workflow-stage-topline">
              <span className="workflow-stage-number">{String(index + 1).padStart(2, '0')}</span>
              <div className="workflow-stage-order">
                <Button
                  variant="destructive"
                  size="sm"
                  aria-label="Move earlier"
                  title="Move earlier"
                  disabled={disabled || index === 0}
                  onClick={() => reorder(index, -1)}
                >
                  <ArrowLeft size={15} />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label="Move later"
                  title="Move later"
                  disabled={disabled || index === draft.stages.length - 1}
                  onClick={() => reorder(index, 1)}
                >
                  <ArrowRight size={15} />
                </Button>
              </div>
            </div>
            <div className="workflow-stage-markers">
              {index === 0 && (
                <span className="workflow-marker marker-start">Starts here</span>
              )}
              {index === draft.stages.length - 1 && (
                <span className="workflow-marker marker-closed">Final stage</span>
              )}
              {index !== 0 && index !== draft.stages.length - 1 && (
                <span className="workflow-marker-placeholder">&nbsp;</span>
              )}
            </div>
            <Field label="Name">
              <input
                className="workflow-stage-name"
                disabled={disabled}
                value={stage.name}
                maxLength={200}
                onChange={(e) => patch(stage.id, { name: e.target.value })}
              />
            </Field>
            <Field label="Description">
              <textarea
                className="workflow-stage-description"
                disabled={disabled}
                value={stage.description}
                maxLength={2000}
                rows={2}
                onChange={(e) => patch(stage.id, { description: e.target.value })}
              />
            </Field>
            <Field label="Responsible agent">
              <select
                className="workflow-select"
                disabled={disabled || agents.isPending}
                value={stage.responsibleAgentId}
                onChange={(e) => patch(stage.id, { responsibleAgentId: e.target.value })}
              >
                <option value="">Keep task assignment</option>
                {agents.data?.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
            </Field>
            <div className="workflow-automation">
              <div className="workflow-automation-heading">
                <Bot size={14} />
                <span>Automation</span>
              </div>
              <label className="workflow-toggle">
                <input
                  type="checkbox"
                  disabled={disabled || index === draft.stages.length - 1}
                  checked={stage.autoStart}
                  onChange={(e) => patch(stage.id, { autoStart: e.target.checked })}
                />{' '}
                <span>
                  <strong>Start on entry</strong>
                  <small>Run the assigned agent automatically</small>
                </span>
              </label>
              <label className="workflow-toggle">
                <input
                  type="checkbox"
                  disabled={disabled || index === draft.stages.length - 1}
                  checked={stage.autoAdvance}
                  onChange={(e) => patch(stage.id, { autoAdvance: e.target.checked })}
                />{' '}
                <span>
                  <strong>Advance on success</strong>
                  <small>Move to the next stage after a successful run</small>
                </span>
              </label>
            </div>
            {permissions.workspaceAdmin && (
              <div className="workflow-stage-footer">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={
                    disabled ||
                    draft.stages.length === 1
                  }
                  onClick={() =>
                    setDraft({ ...draft, stages: draft.stages.filter((s) => s.id !== stage.id) })
                  }
                  aria-label={`Remove ${stage.name}`}
                  title="Remove stage"
                >
                  <Trash2 size={14} />
                </Button>
                <span>Stage {index + 1}</span>
              </div>
            )}
          </article>
        ))}
        {permissions.workspaceAdmin && (
          <button
            type="button"
            className="workflow-add-column"
            aria-label="Add stage"
            disabled={disabled || draft.stages.length >= 50}
            onClick={() =>
              setDraft({
                ...draft,
                stages: [
                  ...draft.stages,
                  {
                    id: crypto.randomUUID(),
                    name: 'New stage',
                    description: '',
                    responsibleAgentId: '',
                    autoStart: false,
                    autoAdvance: false,
                  },
                ],
              })
            }
          >
            <span className="workflow-add-icon">
              <Plus size={17} />
            </span>
            <strong>Add a stage</strong>
            <small>New stages start with automation off</small>
          </button>
        )}
      </div>
      <div className="workflow-footer-note">
        <Check size={14} /> Tasks containing a stage prevent that stage from being removed.
      </div>
      {permissions.workspaceAdmin && dirty && (
        <div className="workflow-unsaved">
          <span /> Unsaved changes
        </div>
      )}
    </>
  );
}

function normalizeDefinition(definition: WorkspaceWorkflow): WorkspaceWorkflow {
  const stages = definition.stages.map((stage) => ({ ...stage }));
  const final = stages[stages.length - 1];
  if (final) {
    final.autoStart = false;
    final.autoAdvance = false;
  }
  return { ...definition, stages };
}
