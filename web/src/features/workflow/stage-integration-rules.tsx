import { ChevronDown, GitPullRequest, Plus, Trash2, Zap } from 'lucide-react';
import '../settings/workflow-stage-integrations.css';
import './stage-integration-rules.css';
import { useQuery } from '@tanstack/react-query';
import { useApp } from '../../app/context';
import type {
  WorkflowAutomationRule,
  WorkspaceWorkflow,
} from '../../shared/api/generated/types.gen';
import { Button } from '../../shared/ui/button';
import { Field } from '../../shared/ui/form';
import { ErrorState, Loading } from '../../shared/ui/common';
import { stageConnectionsQuery, stageProvidersQuery } from '../../shared/api/queries';
import { IntegrationParameters, parameterDefaults } from './integration-parameters';
import type { JsonValue } from '../integrations/schema';
import { WorkflowChoiceSelect, type WorkflowChoice } from './workflow-choice-select';

export function StageIntegrationRules({
  definition,
  stageID,
  disabled,
  onChange,
}: {
  definition: WorkspaceWorkflow;
  stageID: string;
  disabled: boolean;
  onChange: (rules: WorkflowAutomationRule[]) => void;
}) {
  const { scope } = useApp();
  const connections = useQuery(stageConnectionsQuery(scope));
  const providers = useQuery(stageProvidersQuery(scope.userID));
  const stage = definition.stages.find((item) => item.id === stageID);
  const stageName = stage?.name ?? stageID;
  const completionStage = definition.stages.find((item) => item.id === definition.closedStageId);
  const otherTargetStages = definition.stages.filter(
    (item) => item.id !== stageID && item.id !== definition.closedStageId,
  );
  const artifactInputs = (stage?.artifactInputs ?? [])
    .filter((input) => {
      const source = definition.stages.find((item) => item.id === input.sourceStageId);
      const output = source?.artifactOutputs?.find((item) => item.name === input.outputName);
      return output?.kind === 'code_review' || output?.kind === 'pull_request';
    })
    .map((input) => input.name);
  const rules = definition.rules.filter(
    (rule) => rule.sourceStageId === stageID && rule.event === 'integration_action',
  );
  function patch(id: string, values: Partial<WorkflowAutomationRule>) {
    onChange(definition.rules.map((rule) => (rule.id === id ? { ...rule, ...values } : rule)));
  }
  return (
    <details className="workflow-stage-integrations">
      <summary>
        <Zap size={16} aria-hidden="true" />
        <span>External integrations</span>
        <span className="workflow-integration-count">{rules.length}</span>
        <ChevronDown className="workflow-integration-chevron" size={16} aria-hidden="true" />
      </summary>
      {connections.isPending || providers.isPending ? (
        <Loading />
      ) : connections.isError || providers.isError ? (
        <ErrorState error={connections.error ?? providers.error} />
      ) : (
        <>
          <div className="workflow-integration-intro">
            <p>Turn external events into the next step for tasks in {stageName}.</p>
            <small>
              Save the workflow to apply changes. Only new events will trigger these rules.
            </small>
          </div>
          {rules.length === 0 &&
            connections.data.length > 0 &&
            stageID !== definition.closedStageId && (
              <div className="workflow-integration-empty">
                <GitPullRequest size={24} aria-hidden="true" />
                <strong>No integration rules yet</strong>
                <p>
                  Move a task when its pull request is merged, or ask its agent to address a review.
                </p>
                <small>Choose a connection, an event, and what should happen next.</small>
              </div>
            )}
          {stageID === definition.closedStageId && (
            <p className="workflow-integration-hint">
              External integrations cannot start from the final stage.
            </p>
          )}
          {rules.map((rule, index) => {
            const connection = connections.data.find(
              (item) => item.id === rule.integration?.connectionId,
            );
            const provider = providers.data.find((item) => item.provider === connection?.provider);
            const selectedAction = provider?.actions.find(
              (item) => item.id === rule.integration?.actionId,
            );
            const connectionOptions: WorkflowChoice[] = [
              ...(!connection && rule.integration?.connectionId
                ? [
                    {
                      value: rule.integration.connectionId,
                      label: `Unavailable connection (${rule.integration.connectionId})`,
                      description: 'This saved connection is no longer available.',
                    },
                  ]
                : []),
              ...connections.data.map((item) => ({
                value: item.id,
                label: item.name,
                description: [
                  item.provider?.replaceAll('_', ' '),
                  item.repositoryIds?.length
                    ? `${item.repositoryIds.length} ${item.repositoryIds.length === 1 ? 'repository' : 'repositories'}`
                    : undefined,
                  item.enabled ? undefined : 'Paused',
                ]
                  .filter(Boolean)
                  .join(' · '),
              })),
            ];
            const actionOptions: WorkflowChoice[] = [
              ...(!selectedAction && rule.integration?.actionId
                ? [
                    {
                      value: rule.integration.actionId,
                      label: `Unavailable action (${rule.integration.actionId})`,
                      description: 'This saved action is not provided by the selected connection.',
                    },
                  ]
                : []),
              ...(provider?.actions.map((action) => ({
                value: action.id,
                label: action.name,
              })) ?? []),
            ];
            const targetGroups = [
              ...(stage
                ? [
                    {
                      label: 'Keep task in this stage',
                      options: [
                        {
                          value: stage.id,
                          label: stage.name,
                          description: 'Run the callback without changing the task stage.',
                        },
                      ],
                    },
                  ]
                : []),
              ...(otherTargetStages.length
                ? [
                    {
                      label: 'Move task to another stage',
                      options: otherTargetStages.map((item) => ({
                        value: item.id,
                        label: item.name,
                        ...(item.description ? { description: item.description } : {}),
                      })),
                    },
                  ]
                : []),
              ...(completionStage
                ? [
                    {
                      label: 'Complete task',
                      options: [
                        {
                          value: completionStage.id,
                          label: completionStage.name,
                          description: 'Marks the task as done.',
                        },
                      ],
                    },
                  ]
                : []),
            ];
            const target = definition.stages.find((item) => item.id === rule.targetStageId);
            const outcome =
              rule.action === 'call_agent'
                ? 'prompt the assigned agent'
                : target
                  ? target.id === stageID
                    ? `keep the task in ${target.name}`
                    : `move to ${target.name}`
                  : 'choose a destination';
            return (
              <div key={rule.id} className="workflow-integration-rule">
                <div className="workflow-integration-rule-heading">
                  <div>
                    <strong>
                      Rule {index + 1} · {selectedAction?.name ?? 'Choose an event'}
                    </strong>
                    <small>
                      {connection?.name ?? 'Select a connection'} ·{' '}
                      {rule.enabled ? 'Enabled' : 'Paused'}
                    </small>
                  </div>
                  <Field label="Enabled">
                    <input
                      type="checkbox"
                      disabled={disabled}
                      checked={rule.enabled}
                      onChange={(event) => patch(rule.id, { enabled: event.target.checked })}
                    />
                  </Field>
                </div>
                <fieldset className="workflow-integration-step">
                  <legend>
                    <span>1</span> When this happens
                  </legend>
                  <p className="workflow-integration-step-description">
                    Watch for an event while the task is in {stageName}.
                  </p>
                  <Field
                    label="Connection"
                    hint="Connections and repository access are managed in workspace settings."
                  >
                    <WorkflowChoiceSelect
                      disabled={disabled}
                      value={rule.integration?.connectionId ?? ''}
                      placeholder="Choose a connection"
                      aria-required
                      options={connectionOptions}
                      onChange={(value) => {
                        const next = connections.data.find((item) => item.id === value);
                        const action = providers.data.find(
                          (item) => item.provider === next?.provider,
                        )?.actions[0];
                        patch(rule.id, {
                          integration: {
                            connectionId: value,
                            actionId: action?.id ?? '',
                            parameters: parameterDefaults(action?.parametersSchema ?? {}),
                          },
                        });
                      }}
                    />
                  </Field>
                  <Field label="Event">
                    <WorkflowChoiceSelect
                      disabled={disabled || !connection}
                      value={rule.integration?.actionId ?? ''}
                      placeholder="Choose an event"
                      aria-required
                      options={actionOptions}
                      onChange={(value) => {
                        const action = provider?.actions.find((item) => item.id === value);
                        patch(rule.id, {
                          integration: {
                            connectionId: rule.integration?.connectionId ?? '',
                            actionId: value,
                            parameters: parameterDefaults(action?.parametersSchema ?? {}),
                          },
                        });
                      }}
                    />
                  </Field>
                  {selectedAction && rule.integration && (
                    <IntegrationParameters
                      schema={selectedAction.parametersSchema}
                      value={rule.integration.parameters as Record<string, JsonValue>}
                      artifactInputs={artifactInputs}
                      disabled={disabled}
                      onChange={(parameters) =>
                        patch(rule.id, { integration: { ...rule.integration!, parameters } })
                      }
                    />
                  )}
                  {selectedAction && artifactInputs.length === 0 && (
                    <p className="workflow-integration-hint" role="status">
                      This stage has no pull request or code review artifact input yet. Add one to
                      the stage before configuring this action.
                    </p>
                  )}
                  {connection && !connection.enabled && (
                    <p role="status" className="workflow-integration-hint">
                      This connection is disabled. Its callbacks will not run until it is enabled.
                    </p>
                  )}
                  {!selectedAction && (
                    <p role="status" className="workflow-integration-hint">
                      Choose an available connection and action. Existing settings are preserved
                      until you change them.
                    </p>
                  )}
                </fieldset>
                <fieldset className="workflow-integration-step">
                  <legend>
                    <span>2</span> Then do this
                  </legend>
                  <p className="workflow-integration-step-description">
                    Choose how this task should respond to the event.
                  </p>
                  <Field label="Response">
                    <WorkflowChoiceSelect
                      disabled={disabled}
                      value={rule.action}
                      placeholder="Choose a response"
                      options={[
                        {
                          value: 'move_task',
                          label: 'Move task to a stage',
                          description: 'Change the task’s workflow stage.',
                        },
                        {
                          value: 'call_agent',
                          label: 'Call assigned agent with a prompt',
                          description: 'Send review context to the assigned agent.',
                        },
                      ]}
                      onChange={(value) => {
                        if (value === 'call_agent') {
                          const rest = { ...rule };
                          delete rest.targetStageId;
                          onChange(
                            definition.rules.map((item) =>
                              item.id === rule.id
                                ? {
                                    ...rest,
                                    action: 'call_agent',
                                    promptText: rule.promptText ?? '',
                                    sessionPolicy: 'continue_or_create_if_absent',
                                  }
                                : item,
                            ),
                          );
                        } else {
                          const rest = { ...rule };
                          delete rest.promptText;
                          delete rest.sessionPolicy;
                          onChange(
                            definition.rules.map((item) =>
                              item.id === rule.id
                                ? { ...rest, action: 'move_task', targetStageId: '' }
                                : item,
                            ),
                          );
                        }
                      }}
                    />
                  </Field>
                  {rule.action === 'call_agent' ? (
                    <>
                      <Field
                        label="Agent prompt"
                        hint="External review context accompanies this prompt. The task stays in its current stage when the callback starts."
                      >
                        <textarea
                          disabled={disabled}
                          required
                          maxLength={32768}
                          value={rule.promptText ?? ''}
                          onChange={(event) => patch(rule.id, { promptText: event.target.value })}
                        />
                      </Field>
                      <Field label="Conversation">
                        <WorkflowChoiceSelect
                          disabled={disabled}
                          value={rule.sessionPolicy ?? 'continue_or_create_if_absent'}
                          placeholder="Choose a conversation"
                          options={[
                            {
                              value: 'continue_or_create_if_absent',
                              label: 'Continue this conversation',
                              description: 'Create one only if the task has no conversation yet.',
                            },
                            {
                              value: 'new',
                              label: 'Start a new conversation',
                              description: 'Keep this callback separate from earlier agent work.',
                            },
                          ]}
                          onChange={(value) =>
                            patch(rule.id, {
                              sessionPolicy: value as 'continue_or_create_if_absent' | 'new',
                            })
                          }
                        />
                      </Field>
                    </>
                  ) : (
                    <Field
                      label="Destination stage"
                      hint="Choose where the task should go when this provider event arrives. The current stage keeps it here; the closed stage marks the task done."
                    >
                      <WorkflowChoiceSelect
                        disabled={disabled}
                        value={rule.targetStageId ?? ''}
                        groups={targetGroups}
                        aria-required
                        onChange={(value) => patch(rule.id, { targetStageId: value })}
                        placeholder="Choose where the task should go"
                      />
                    </Field>
                  )}
                </fieldset>
                <div className="workflow-integration-preview">
                  <Zap size={14} aria-hidden="true" />
                  <p>
                    {rule.enabled ? 'When' : 'Paused: when'}{' '}
                    <strong>{selectedAction?.name ?? 'an event occurs'}</strong>, {outcome}.
                  </p>
                </div>
                <div className="workflow-integration-rule-footer">
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={disabled}
                    onClick={() => onChange(definition.rules.filter((item) => item.id !== rule.id))}
                  >
                    <Trash2 size={14} aria-hidden="true" /> Remove callback
                  </Button>
                </div>
              </div>
            );
          })}
          {stageID !== definition.closedStageId && (
            <Button
              variant="outline"
              disabled={disabled || connections.data.length === 0}
              onClick={() =>
                onChange([
                  ...definition.rules,
                  {
                    id: crypto.randomUUID(),
                    enabled: true,
                    event: 'integration_action',
                    sourceStageId: stageID,
                    action: 'move_task',
                    targetStageId: '',
                    integration: { connectionId: '', actionId: '', parameters: {} },
                  },
                ])
              }
            >
              <Plus size={15} aria-hidden="true" /> Add integration callback
            </Button>
          )}
          {connections.data.length === 0 && (
            <div className="workflow-integration-no-connection">
              <strong>Connect a provider to get started</strong>
              <span>
                Create a workflow stage connection in workspace settings, then return here to choose
                an event and response.
              </span>
              <a
                href={`/organizations/${encodeURIComponent(scope.orgID)}/workspaces/${encodeURIComponent(scope.workspaceID)}/settings`}
              >
                Open workspace settings
              </a>
            </div>
          )}
        </>
      )}
    </details>
  );
}
