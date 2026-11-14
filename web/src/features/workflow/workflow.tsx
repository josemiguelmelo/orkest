import { useState } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  Bot,
  Check,
  FileOutput,
  Plus,
  Save,
  Trash2,
  Workflow as WorkflowIcon,
} from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import type {
  ArtifactInput,
  ArtifactOutput,
  WorkspaceWorkflow,
  WorkflowStage,
} from '../../shared/api/generated/types.gen';
import { agentsQuery, path, useRefresh, workflowsQuery } from '../../shared/api/queries';
import { Button } from '../../shared/ui/button';
import { ConfirmButton, ErrorState, Loading, PageHeader } from '../../shared/ui/common';
import { Field } from '../../shared/ui/form';
import { Dialog } from '../../shared/ui/dialog';
import { useAction } from '../../shared/use-action';
import { StageIntegrationRules } from './stage-integration-rules';

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
  const initialStageId = draft.stages[0]?.id ?? '';
  const closedStageId = draft.stages.at(-1)?.id ?? '';
  function patch(id: string, values: Partial<WorkflowStage>) {
    setDraft((d) => ({
      ...d,
      stages: d.stages.map((s) => (s.id === id ? { ...s, ...values } : s)),
    }));
  }
  function setRule(
    stageId: string,
    event: 'stage_entered' | 'run_succeeded' | 'stage_completed',
    target: string,
  ) {
    setDraft((d) => ({
      ...d,
      rules: [
        ...d.rules.filter((r) => !(r.sourceStageId === stageId && r.event === event)),
        ...(target
          ? [
              {
                id:
                  d.rules.find((r) => r.sourceStageId === stageId && r.event === event)?.id ??
                  crypto.randomUUID(),
                enabled: true,
                event,
                sourceStageId: stageId,
                action:
                  event === 'stage_entered' ? ('start_work' as const) : ('move_task' as const),
                ...(event === 'stage_entered' ? {} : { targetStageId: target }),
              },
            ]
          : []),
      ],
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
      return withDerivedStageBoundaries({ ...d, stages });
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
              disabled={
                action.pending ||
                !dirty ||
                draft.stages.some(
                  (s) => new TextEncoder().encode((s.customPrompt ?? '').trim()).length > 32768,
                )
              }
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
          <select
            className="workflow-select"
            value={definition.id}
            onChange={(event) => onSelect(event.target.value)}
          >
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
            <span>Tasks start in the first column and close in the last.</span>
          </div>
        </div>
      </div>
      <div className="workflow-board-hint">
        <span>Column order controls display</span>
        <span>Rules move tasks to explicit destinations; failed work stays in its stage.</span>
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
              {stage.id === initialStageId && (
                <span className="workflow-marker marker-start">Starts here</span>
              )}
              {stage.id === closedStageId && (
                <span className="workflow-marker marker-closed">Final stage</span>
              )}
              {stage.id !== initialStageId && stage.id !== closedStageId && (
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
            <Field
              label="Custom prompt"
              hint="Sent with task details at every work start, including continued sessions."
              error={
                new TextEncoder().encode((stage.customPrompt ?? '').trim()).length > 32768
                  ? 'Custom prompt must be at most 32,768 UTF-8 bytes.'
                  : undefined
              }
            >
              <textarea
                disabled={disabled}
                value={stage.customPrompt ?? ''}
                rows={5}
                onChange={(e) => patch(stage.id, { customPrompt: e.target.value })}
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
            <StageArtifacts
              stage={stage}
              stages={draft.stages}
              disabled={disabled}
              onChange={(values) => patch(stage.id, values)}
            />
            <div className="workflow-automation">
              <div className="workflow-automation-heading">
                <Bot size={14} />
                <span>Automation</span>
              </div>
              <label className="workflow-toggle">
                <input
                  type="checkbox"
                  disabled={disabled || stage.id === closedStageId}
                  checked={draft.rules.some(
                    (r) => r.enabled && r.sourceStageId === stage.id && r.event === 'stage_entered',
                  )}
                  onChange={(e) =>
                    setRule(stage.id, 'stage_entered', e.target.checked ? 'start' : '')
                  }
                />{' '}
                <span>
                  <strong>Start on entry</strong>
                  <small>Run the assigned agent automatically</small>
                </span>
              </label>
              {(['run_succeeded', 'stage_completed'] as const).map((event) => (
                <Field
                  key={event}
                  label={
                    event === 'run_succeeded' ? 'After successful run' : 'After manual completion'
                  }
                >
                  <select
                    disabled={disabled || stage.id === closedStageId}
                    value={
                      draft.rules.find(
                        (r) => r.enabled && r.sourceStageId === stage.id && r.event === event,
                      )?.targetStageId ?? ''
                    }
                    onChange={(e) => setRule(stage.id, event, e.target.value)}
                  >
                    <option value="">Stay in this stage</option>
                    {draft.stages
                      .filter((s) => s.id !== stage.id)
                      .map((s) => (
                        <option key={s.id} value={s.id}>
                          Move to {s.name}
                        </option>
                      ))}
                  </select>
                </Field>
              ))}
            </div>
            <StageIntegrationRules
              definition={draft}
              stageID={stage.id}
              disabled={disabled}
              onChange={(rules) => setDraft((current) => ({ ...current, rules }))}
            />
            {permissions.workspaceAdmin && (
              <div className="workflow-stage-footer">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={
                    disabled ||
                    draft.stages.length === 1 ||
                    stage.id === initialStageId ||
                    stage.id === closedStageId
                  }
                  onClick={() =>
                    setDraft(
                      withDerivedStageBoundaries({
                        ...draft,
                        stages: draft.stages.filter((s) => s.id !== stage.id),
                        rules: draft.rules.filter(
                          (r) => r.sourceStageId !== stage.id && r.targetStageId !== stage.id,
                        ),
                      }),
                    )
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
              setDraft(
                withDerivedStageBoundaries({
                  ...draft,
                  stages: [
                    ...draft.stages,
                    {
                      id: crypto.randomUUID(),
                      name: 'New stage',
                      description: '',
                      responsibleAgentId: '',
                    },
                  ],
                }),
              )
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

function StageArtifacts({
  stage,
  stages,
  disabled,
  onChange,
}: {
  stage: WorkflowStage;
  stages: WorkflowStage[];
  disabled: boolean;
  onChange: (values: Partial<WorkflowStage>) => void;
}) {
  const outputs = stage.artifactOutputs ?? [];
  const inputs = stage.artifactInputs ?? [];
  const total = outputs.length + inputs.length;
  const availableOutputs = stages
    .filter((candidate) => candidate.id !== stage.id)
    .flatMap((candidate) =>
      (candidate.artifactOutputs ?? []).map((output) => ({
        stageID: candidate.id,
        stageName: candidate.name,
        output,
      })),
    );
  function updateOutput(index: number, values: Partial<ArtifactOutput>) {
    onChange({
      artifactOutputs: outputs.map((output, current) =>
        current === index ? { ...output, ...values } : output,
      ),
    });
  }
  function updateInput(index: number, values: Partial<ArtifactInput>) {
    onChange({
      artifactInputs: inputs.map((input, current) =>
        current === index ? { ...input, ...values } : input,
      ),
    });
  }
  return (
    <details className="workflow-artifacts">
      <summary>
        <span className="workflow-artifacts-summary">
          <FileOutput size={14} /> Artifacts
        </span>
        <span className="workflow-artifact-count">{total}</span>
      </summary>
      <div className="workflow-artifacts-intro">
        <p>Define the files this stage produces and the files it needs from other stages.</p>
        <small>Artifact names are used by agents and workflow integrations.</small>
      </div>
      <section className="workflow-artifact-section" aria-labelledby={`${stage.id}-outputs`}>
        <div className="workflow-artifact-section-heading">
          <div>
            <h3 id={`${stage.id}-outputs`}>Outputs</h3>
            <p>Results published when work in this stage completes.</p>
          </div>
          <Button
            size="sm"
            variant="outline"
            disabled={disabled || outputs.length >= 50}
            onClick={() =>
              onChange({ artifactOutputs: [...outputs, { name: 'output', kind: 'plan' }] })
            }
          >
            <Plus size={14} /> Add output
          </Button>
        </div>
        {outputs.length === 0 ? (
          <p className="workflow-artifact-empty">No outputs configured yet.</p>
        ) : (
          outputs.map((output, index) => (
            <div className="workflow-artifact-card" key={index}>
              <div className="workflow-artifact-card-heading">
                <strong>Output {index + 1}</strong>
                <Button
                  size="icon"
                  variant="ghost"
                  disabled={disabled}
                  aria-label={`Remove output ${index + 1}`}
                  title="Remove output"
                  onClick={() =>
                    onChange({ artifactOutputs: outputs.filter((_, current) => current !== index) })
                  }
                >
                  <Trash2 size={14} />
                </Button>
              </div>
              <div className="workflow-artifact-fields">
                <Field label="Output name" hint="A short name agents can refer to.">
                  <input
                    disabled={disabled}
                    value={output.name}
                    onChange={(event) => updateOutput(index, { name: event.target.value })}
                  />
                </Field>
                <Field label="Output kind" hint="For example, plan or pull_request.">
                  <input
                    disabled={disabled}
                    value={output.kind}
                    onChange={(event) => updateOutput(index, { kind: event.target.value })}
                  />
                </Field>
              </div>
            </div>
          ))
        )}
      </section>
      <section className="workflow-artifact-section" aria-labelledby={`${stage.id}-inputs`}>
        <div className="workflow-artifact-section-heading">
          <div>
            <h3 id={`${stage.id}-inputs`}>Inputs</h3>
            <p>Bring an output from another stage into this stage.</p>
          </div>
          <Button
            size="sm"
            variant="outline"
            disabled={disabled || inputs.length >= 50}
            onClick={() =>
              onChange({
                artifactInputs: [
                  ...inputs,
                  { name: 'input', sourceStageId: '', outputName: '', required: true },
                ],
              })
            }
          >
            <Plus size={14} /> Add input
          </Button>
        </div>
        {inputs.length === 0 ? (
          <p className="workflow-artifact-empty">No inputs configured yet.</p>
        ) : (
          inputs.map((input, index) => (
            <div className="workflow-artifact-card" key={index}>
              <div className="workflow-artifact-card-heading">
                <strong>Input {index + 1}</strong>
                <Button
                  size="icon"
                  variant="ghost"
                  disabled={disabled}
                  aria-label={`Remove input ${index + 1}`}
                  title="Remove input"
                  onClick={() =>
                    onChange({ artifactInputs: inputs.filter((_, current) => current !== index) })
                  }
                >
                  <Trash2 size={14} />
                </Button>
              </div>
              <div className="workflow-artifact-fields">
                <Field label="Input name" hint="The local name used by this stage.">
                  <input
                    disabled={disabled}
                    value={input.name}
                    onChange={(event) => updateInput(index, { name: event.target.value })}
                  />
                </Field>
                <Field label="Source output">
                  <select
                    className="workflow-select"
                    disabled={disabled || availableOutputs.length === 0}
                    value={JSON.stringify([input.sourceStageId, input.outputName])}
                    onChange={(event) => {
                      const [sourceStageId, outputName] = JSON.parse(event.target.value) as [
                        string,
                        string,
                      ];
                      updateInput(index, { sourceStageId, outputName });
                    }}
                  >
                    <option value={JSON.stringify(['', ''])}>
                      {availableOutputs.length === 0 ? 'Add an output first' : 'Select output'}
                    </option>
                    {availableOutputs.map(({ stageID, stageName, output }) => (
                      <option
                        key={`${stageID}:${output.name}`}
                        value={JSON.stringify([stageID, output.name])}
                      >
                        {stageName} / {output.name}
                      </option>
                    ))}
                  </select>
                </Field>
              </div>
              <label className="workflow-artifact-required">
                <input
                  type="checkbox"
                  disabled={disabled}
                  checked={input.required}
                  onChange={(event) => updateInput(index, { required: event.target.checked })}
                />
                <span>
                  <strong>Required before starting</strong>
                  <small>Prevent this stage from starting until the input exists.</small>
                </span>
              </label>
            </div>
          ))
        )}
      </section>
    </details>
  );
}

function normalizeDefinition(definition: WorkspaceWorkflow): WorkspaceWorkflow {
  return withDerivedStageBoundaries({
    ...definition,
    stages: structuredClone(definition.stages),
    rules: structuredClone(definition.rules),
  });
}

function withDerivedStageBoundaries(definition: WorkspaceWorkflow): WorkspaceWorkflow {
  const closedStageId = definition.stages.at(-1)?.id ?? '';
  return {
    ...definition,
    initialStageId: definition.stages[0]?.id ?? '',
    closedStageId,
    rules: definition.rules.filter((rule) => rule.sourceStageId !== closedStageId),
  };
}
